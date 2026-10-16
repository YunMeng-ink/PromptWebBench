#!/usr/bin/env bash
# G-01：每日逻辑备份，保留 7 天（开发方案 §8.4）。
#
# 用法：
#   deploy/backup.sh                 # 备份到默认目录 /var/backups/prompt-benchmark
#   BACKUP_DIR=/data/backups deploy/backup.sh
#
# 依赖：pg_dump（PostgreSQL 18）。连接参数取自环境变量，与后端服务同源：
#   DB_HOST DB_PORT DB_NAME DB_USER DB_PASSWORD
#
# 部署方式：把本脚本放到 cron 或 systemd timer 中每日执行一次。
# 例（每日 03:30）：
#   30 3 * * * /opt/prompt-benchmark/deploy/backup.sh >> /var/log/pb-backup.log 2>&1

set -euo pipefail

DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-prompt_benchmark}"
DB_USER="${DB_USER:-postgres}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/prompt-benchmark}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"

if ! command -v pg_dump >/dev/null 2>&1; then
	echo "未找到 pg_dump，请先安装 PostgreSQL 客户端" >&2
	exit 1
fi

mkdir -p "$BACKUP_DIR"

STAMP="$(date -u +%Y%m%d-%H%M%S)"
TARGET="${BACKUP_DIR}/${DB_NAME}-${STAMP}.dump"
TMP="${TARGET}.part"

# 自定义格式（-Fc）：支持 pg_restore 选择性恢复与并行恢复，体积小于纯文本。
# 先写 .part 再改名：中断的备份不会以完整文件名出现，避免恢复时选到半截文件。
PGPASSWORD="${DB_PASSWORD:-}" pg_dump \
	--host="$DB_HOST" \
	--port="$DB_PORT" \
	--username="$DB_USER" \
	--dbname="$DB_NAME" \
	--format=custom \
	--no-owner \
	--file="$TMP"

mv "$TMP" "$TARGET"
echo "备份完成：${TARGET}"

# 清理超过保留期的备份。仅删除本脚本命名的 .dump 与 .dump.part 文件。
find "$BACKUP_DIR" -maxdepth 1 -type f \
	\( -name "${DB_NAME}-*.dump" -o -name "${DB_NAME}-*.dump.part" \) \
	-mtime "+${RETENTION_DAYS}" -delete
echo "已清理 ${RETENTION_DAYS} 天前的备份"

find "$BACKUP_DIR" -maxdepth 1 -type f -name "${DB_NAME}-*.dump" -printf '%f\n' | sort | tail -5
