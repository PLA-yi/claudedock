#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/var/backups/cloud-cli-proxy}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"
DB_PATH="${DB_PATH:-/data/cloud-cli-proxy.db}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/cloud-cli-proxy_${TIMESTAMP}.db"

mkdir -p "$BACKUP_DIR"

echo "[backup] Starting SQLite backup: ${DB_PATH}"
sqlite3 "$DB_PATH" ".backup '${BACKUP_FILE}'"
echo "[backup] Backup saved: $BACKUP_FILE ($(du -h "$BACKUP_FILE" | cut -f1))"

find "$BACKUP_DIR" -name "cloud-cli-proxy_*.db" -mtime +"$RETENTION_DAYS" -delete
echo "[backup] Cleaned backups older than ${RETENTION_DAYS} days"
