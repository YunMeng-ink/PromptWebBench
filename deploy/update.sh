#!/usr/bin/env bash
# 从 GitHub Release 安装二进制：校验和校验、原子替换、重启与健康检查，失败自动回滚。
#
# 用法：
#   deploy/update.sh                更新到最新正式版；已是最新时不做任何事
#   TAG=v0.3.1 deploy/update.sh     安装指定标签
#   FORCE=1 deploy/update.sh        跳过版本比较，重新安装或回退到指定标签
#   deploy/update.sh --check        只检查不安装；有新版本时退出码 10
#
# 配置：环境变量或 /etc/prompt-benchmark/update.env，已设置的变量优先。
#   REPO          GitHub 仓库 owner/name
#   ASSET         发布资产名
#   INSTALL_DIR   安装目录，二进制位于 $INSTALL_DIR/dist/$ASSET
#   RESTART_MODE  systemd（默认）或 script（调用 deploy/pb.sh restart）
#   SERVICE       systemd 单元名
#   ENV_FILE      script 模式传给 pb.sh 的环境文件
#   HEALTH_URL    重启后的健康检查地址
#   STATE_DIR     记录已安装版本与更新锁的目录

set -euo pipefail

CHECK_ONLY=0

usage() {
	sed -n '2,20p' "$0"
}

for arg in "$@"; do
	case "$arg" in
	--check) CHECK_ONLY=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		printf '未知参数：%s\n' "$arg" >&2
		exit 2
		;;
	esac
done

CONFIG_FILE="${CONFIG_FILE:-/etc/prompt-benchmark/update.env}"
if [ -f "$CONFIG_FILE" ]; then
	# 只补齐未设置的变量，使命令行上的 TAG、FORCE 等取值优先。
	while IFS= read -r line || [ -n "$line" ]; do
		case "$line" in
		'' | '#'*) continue ;;
		esac
		key="${line%%=*}"
		value="${line#*=}"
		key="$(printf '%s' "$key" | tr -d '[:space:]')"
		[ -n "$key" ] || continue
		if printenv "$key" >/dev/null 2>&1; then
			continue
		fi
		case "$value" in
		\"*\") value="${value#\"}" && value="${value%\"}" ;;
		\'*\') value="${value#\'}" && value="${value%\'}" ;;
		esac
		export "$key=$value"
	done <"$CONFIG_FILE"
fi

REPO="${REPO:-YunMeng-ink/PromptWebBench}"
ASSET="${ASSET:-prompt-benchmark}"
INSTALL_DIR="${INSTALL_DIR:-/opt/prompt-benchmark}"
RESTART_MODE="${RESTART_MODE:-systemd}"
SERVICE="${SERVICE:-prompt-benchmark}"
ENV_FILE="${ENV_FILE:-/etc/prompt-benchmark/env}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/api/v1/health}"
STATE_DIR="${STATE_DIR:-/var/lib/prompt-benchmark}"
TAG="${TAG:-}"
FORCE="${FORCE:-0}"

BIN="${INSTALL_DIR}/dist/${ASSET}"
VERSION_FILE="${STATE_DIR}/version"

info() { printf '%s\n' "$*"; }
die() {
	printf '错误：%s\n' "$*" >&2
	exit 1
}

command -v curl >/dev/null 2>&1 || die "缺少 curl 命令"
command -v jq >/dev/null 2>&1 || die "缺少 jq 命令（apt install -y jq）"

# release_api 返回目标版本的发布接口地址。TAG 为空时取最新正式版。
release_api() {
	if [ -n "$TAG" ]; then
		printf 'https://api.github.com/repos/%s/releases/tags/%s' "$REPO" "$TAG"
	else
		printf 'https://api.github.com/repos/%s/releases/latest' "$REPO"
	fi
}

installed_version() {
	if [ -s "$VERSION_FILE" ]; then
		cat "$VERSION_FILE"
		return 0
	fi
	if [ -x "$BIN" ]; then
		"$BIN" --version 2>/dev/null || true
	fi
}

# version_newer 判断第一个参数是否高于第二个参数（按版本号排序，不是字典序）。
version_newer() {
	local remote="$1" current="$2"
	[ -n "$remote" ] || return 1
	[ -n "$current" ] || return 0
	[ "$remote" = "$current" ] && return 1
	[ "$(printf '%s\n%s\n' "$current" "$remote" | sort -V | tail -n 1)" = "$remote" ]
}

api_url="$(release_api)"
info "读取发布信息：${api_url}"
release_json="$(curl -fsSL -H 'Accept: application/vnd.github+json' "$api_url")" ||
	die "无法读取发布信息，请检查 TAG、网络与仓库是否公开"

remote_tag="$(printf '%s' "$release_json" | jq -r '.tag_name // empty')"
[ -n "$remote_tag" ] || die "发布信息中没有标签名"

asset_url="$(printf '%s' "$release_json" |
	jq -r --arg name "$ASSET" '.assets[] | select(.name == $name) | .browser_download_url' |
	head -n 1)"
checksum_url="$(printf '%s' "$release_json" |
	jq -r --arg name "${ASSET}.sha256" '.assets[] | select(.name == $name) | .browser_download_url' |
	head -n 1)"
[ -n "$asset_url" ] || die "发布 ${remote_tag} 中没有资产 ${ASSET}"
[ -n "$checksum_url" ] || die "发布 ${remote_tag} 中没有校验文件 ${ASSET}.sha256"

installed="$(installed_version)"
info "当前版本：${installed:-未安装}；最新发布：${remote_tag}"

if [ -n "$installed" ] && [ "$remote_tag" = "$installed" ] && [ "$FORCE" != "1" ]; then
	info "已是 ${installed}，无需更新"
	exit 0
fi
if [ -n "$installed" ] && ! version_newer "$remote_tag" "$installed" && [ "$FORCE" != "1" ]; then
	info "已安装 ${installed}，不低于发布 ${remote_tag}；如需重装或回退，设置 FORCE=1"
	exit 0
fi

if [ "$CHECK_ONLY" = "1" ]; then
	info "可更新：${installed:-未安装} -> ${remote_tag}"
	exit 10
fi

[ "$(uname -s)" = "Linux" ] || die "安装步骤只支持 Linux；其他平台请使用 deploy/pb.sh build"

install -d -m 755 "$INSTALL_DIR/dist" "$STATE_DIR"

# 同一时间只允许一个更新进程，避免定时器与手动执行交错替换二进制。
if command -v flock >/dev/null 2>&1; then
	exec 9>"${STATE_DIR}/update.lock"
	flock -n 9 || {
		info "另一个更新进程正在运行，跳过本次"
		exit 0
	}
fi

tmp_dir="$(mktemp -d)"
cleanup() { rm -rf "$tmp_dir"; }
trap cleanup EXIT

info "下载 ${remote_tag}……"
curl -fsSL -o "${tmp_dir}/${ASSET}" "$asset_url"
curl -fsSL -o "${tmp_dir}/${ASSET}.sha256" "$checksum_url"
(
	cd "$tmp_dir"
	sha256sum -c "${ASSET}.sha256" >/dev/null
) || die "校验和不匹配，已放弃安装"
info "校验和通过"

# 先写临时文件再改名：中断的下载不会留下半截二进制。
install -m 755 "${tmp_dir}/${ASSET}" "${BIN}.new"
if [ -x "$BIN" ]; then
	cp -f "$BIN" "${BIN}.previous"
fi
mv -f "${BIN}.new" "$BIN"

restart_service() {
	case "$RESTART_MODE" in
	systemd) systemctl restart "$SERVICE" ;;
	script) "${INSTALL_DIR}/deploy/pb.sh" restart "$ENV_FILE" ;;
	*) die "RESTART_MODE 只能是 systemd 或 script" ;;
	esac
}

healthy() {
	local _
	for _ in $(seq 1 30); do
		if curl -fsS --connect-timeout 2 --max-time 3 -o /dev/null "$HEALTH_URL" 2>/dev/null; then
			return 0
		fi
		sleep 1
	done
	return 1
}

info "重启服务（${RESTART_MODE}）……"
if ! restart_service; then
	printf '重启失败，回滚到上一版本\n' >&2
	if [ -f "${BIN}.previous" ]; then
		mv -f "${BIN}.previous" "$BIN"
		restart_service || true
	fi
	die "更新失败"
fi

if ! healthy; then
	printf '健康检查未通过，回滚到上一版本\n' >&2
	if [ -f "${BIN}.previous" ]; then
		mv -f "${BIN}.previous" "$BIN"
		restart_service || true
		healthy || printf '回滚后的健康检查仍未通过，请检查服务日志\n' >&2
	fi
	die "更新失败"
fi

printf '%s\n' "$remote_tag" >"$VERSION_FILE"
rm -f "${BIN}.previous"
info "已更新到 ${remote_tag}"
