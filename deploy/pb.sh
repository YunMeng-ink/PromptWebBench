#!/usr/bin/env bash
# Prompt Benchmark 一键打包与进程控制脚本。
#
# 用法：
#   deploy/pb.sh build               打包：构建前端并嵌入后端，产出单文件二进制
#   deploy/pb.sh start [环境文件]    后台启动（默认 deploy/pb.env）
#   deploy/pb.sh stop                停止
#   deploy/pb.sh restart [环境文件]  重启
#   deploy/pb.sh status              运行状态与健康检查
#   deploy/pb.sh logs [行数]         查看日志（默认最近 50 行）

set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEFAULT_ENV="$ROOT/deploy/pb.env"
BIN="$ROOT/dist/prompt-benchmark"
PID_FILE="$ROOT/dist/pb.pid"
LOG_FILE="$ROOT/dist/pb.log"
ENV_RECORD="$ROOT/dist/pb.envfile"
START_RECORD="$ROOT/dist/pb.start"

# Windows（Git Bash）下 go build 会自动补充 .exe 后缀。
case "$(uname -s)" in
MINGW* | MSYS* | CYGWIN*) BIN="$BIN.exe" ;;
esac

die() {
	printf '错误：%s\n' "$*" >&2
	exit 1
}

info() {
	printf '%s\n' "$*"
}

remove_records() {
	rm -f "$PID_FILE" "$START_RECORD" "$ENV_RECORD"
}

# start_clock 读取进程启动时刻（jiffies）。Linux 下读 /proc 用于识别 PID 复用；
# 其余平台（Windows Git Bash）无此信息，返回空串并跳过身份校验。
start_clock() {
	local pid=$1
	case "$(uname -s)" in
	Linux*)
		awk '{print $22}' "/proc/$pid/stat" 2>/dev/null
		;;
	*)
		printf '\n'
		;;
	esac
}

# is_running 核对 PID 文件的进程：存活，且（Linux 下）启动时刻与记录一致。
# 时刻不一致说明 PID 已被系统回收并分配给其他进程，视为未运行并清理陈旧记录，
# 避免 stop 误杀无关进程。
is_running() {
	[ -f "$PID_FILE" ] || return 1
	local pid recorded
	pid="$(cat "$PID_FILE")"
	case "$pid" in
	'' | *[!0-9]*) return 1 ;;
	esac
	kill -0 "$pid" 2>/dev/null || return 1
	if [ -s "$START_RECORD" ]; then
		recorded="$(cat "$START_RECORD")"
		local current
		current="$(start_clock "$pid")"
		if [ -n "$recorded" ] && [ -n "$current" ] && [ "$current" != "$recorded" ]; then
			remove_records
			return 1
		fi
	fi
}

# env_of 求启动用的环境文件：位置参数优先于 PB_ENV_FILE，再退到默认值。
env_of() {
	if [ "$#" -gt 0 ]; then
		printf '%s\n' "$1"
	elif [ -n "${PB_ENV_FILE:-}" ]; then
		printf '%s\n' "$PB_ENV_FILE"
	else
		printf '%s\n' "$DEFAULT_ENV"
	fi
}

# port_of 从环境文件读取 PORT。只提取该键的取值，不执行整个文件，
# 避免口令行里的特殊字符在 shell 求值中产生副作用。
port_of() {
	local port
	port="$(grep -E '^PORT=' "$1" | tail -n 1 | cut -d= -f2- | tr -d ' \r"'"'"'')"
	printf '%s\n' "${port:-8080}"
}

# terminate 先发 SIGTERM 等待退出，超时后强制终止。
terminate() {
	local pid=$1
	kill "$pid" 2>/dev/null || true
	local _
	for _ in $(seq 1 20); do
		if ! kill -0 "$pid" 2>/dev/null; then
			return 0
		fi
		sleep 0.5
	done
	kill -9 "$pid" 2>/dev/null || true
	info "进程未在 10 秒内退出，已强制终止"
}

cmd_build() {
	command -v go >/dev/null 2>&1 || die "缺少 go 命令"
	command -v pnpm >/dev/null 2>&1 || die "缺少 pnpm 命令"

	info "构建前端……"
	(cd "$ROOT/frontend" && pnpm build)

	info "嵌入前端产物……"
	rm -rf "$ROOT/backend/internal/web/dist"
	mkdir -p "$ROOT/backend/internal/web/dist"
	cp -R "$ROOT/frontend/dist/." "$ROOT/backend/internal/web/dist/"

	info "编译后端（embedweb）……"
	mkdir -p "$ROOT/dist"
	(
		cd "$ROOT/backend"
		go build -tags embedweb -trimpath -o "$BIN" ./cmd/server
	)
	info "产物：$BIN"
}

cmd_start() {
	local env_file url
	env_file="$(env_of "$@")"
	[ -x "$BIN" ] || die "未找到二进制 $BIN，请先执行 deploy/pb.sh build"
	[ -f "$env_file" ] || die "缺少环境文件 $env_file（参考 backend/.env.example 创建）"
	command -v curl >/dev/null 2>&1 || die "缺少 curl 命令，无法进行健康检查"
	mkdir -p "$ROOT/dist"

	if is_running; then
		info "服务已在运行（PID $(cat "$PID_FILE")），如需重启请执行 restart"
		return 0
	fi

	nohup "$BIN" -env-file "$env_file" >>"$LOG_FILE" 2>&1 &
	printf '%s\n' "$!" >"$PID_FILE"
	printf '%s\n' "$(start_clock "$!")" >"$START_RECORD"
	printf '%s\n' "$env_file" >"$ENV_RECORD"
	url="http://127.0.0.1:$(port_of "$env_file")/api/v1/health"
	info "已启动（PID $!），等待健康检查 $url"

	local _
	for _ in $(seq 1 60); do
		if ! is_running; then
			printf '进程提前退出，最近日志：\n' >&2
			tail -n 20 "$LOG_FILE" >&2 || true
			remove_records
			die "服务启动失败"
		fi
		if curl -fsS --connect-timeout 2 --max-time 3 -o /dev/null "$url" 2>/dev/null; then
			info "健康检查通过"
			return 0
		fi
		sleep 0.5
	done

	# 假死（端口可达但健康检查不响应）时不留半启动状态：终止进程并清理记录。
	printf '健康检查超时（30 秒），最近日志：\n' >&2
	tail -n 20 "$LOG_FILE" >&2 || true
	terminate "$(cat "$PID_FILE")"
	remove_records
	die "服务启动失败"
}

cmd_stop() {
	if ! is_running; then
		remove_records
		info "服务未在运行"
		return 0
	fi

	local pid
	pid="$(cat "$PID_FILE")"
	info "正在停止（PID $pid）……"
	terminate "$pid"
	info "已停止"
	remove_records
}

cmd_restart() {
	cmd_stop
	cmd_start "$@"
}

cmd_status() {
	if ! is_running; then
		remove_records
		info "状态：未运行"
		info "二进制：$BIN"
		return 1
	fi

	local env_file url
	# 以启动时记录的环境文件为准，避免与默认文件不一致时探测错端口。
	if [ -f "$ENV_RECORD" ]; then
		env_file="$(cat "$ENV_RECORD")"
	else
		env_file="$(env_of)"
	fi
	info "状态：运行中（PID $(cat "$PID_FILE")）"
	info "二进制：$BIN"
	if [ -f "$env_file" ]; then
		url="http://127.0.0.1:$(port_of "$env_file")/api/v1/health"
		if curl -fsS --connect-timeout 2 --max-time 3 -o /dev/null "$url" 2>/dev/null; then
			info "健康检查：通过（$url）"
		else
			info "健康检查：未通过（$url）"
			return 1
		fi
	fi
	info "日志：$LOG_FILE"
}

cmd_logs() {
	[ -f "$LOG_FILE" ] || die "日志文件不存在：$LOG_FILE"
	tail -n "${1:-50}" "$LOG_FILE"
}

main() {
	case "${1:-}" in
	build) cmd_build ;;
	start) shift && cmd_start "$@" ;;
	stop) cmd_stop ;;
	restart) shift && cmd_restart "$@" ;;
	status) cmd_status ;;
	logs) shift && cmd_logs "$@" ;;
	'') die "缺少子命令，可用：build / start / stop / restart / status / logs" ;;
	*) die "未知子命令 $1，可用：build / start / stop / restart / status / logs" ;;
	esac
}

main "$@"
