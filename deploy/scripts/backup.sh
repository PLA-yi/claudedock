#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/var/backups/claudedock}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"
DB_PATH="${DB_PATH:-/data/claudedock.db}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/claudedock_${TIMESTAMP}.db"

mkdir -p "$BACKUP_DIR"

echo "[backup] Starting SQLite backup: ${DB_PATH}"
sqlite3 "$DB_PATH" ".backup '${BACKUP_FILE}'"
echo "[backup] Backup saved: $BACKUP_FILE ($(du -h "$BACKUP_FILE" | cut -f1))"

find "$BACKUP_DIR" -name "claudedock_*.db" -mtime +"$RETENTION_DAYS" -delete
echo "[backup] Cleaned backups older than ${RETENTION_DAYS} days"
