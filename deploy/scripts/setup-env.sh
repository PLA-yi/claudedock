#!/usr/bin/env bash
# 生产环境 .env 初始化脚本
# 自动生成所有密码和密钥，使用 SQLite 单文件数据库
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="$REPO_ROOT/.env"

log()  { printf "\033[36m[setup]\033[0m %s\n" "$*"; }
warn() { printf "\033[33m[setup]\033[0m %s\n" "$*"; }
err()  { printf "\033[31m[setup]\033[0m %s\n" "$*" >&2; }

rand_password() { head -c 32 /dev/urandom | base64 | tr -d '=+/' | head -c "$1"; }

if [[ -f "$ENV_FILE" ]]; then
  warn ".env 文件已存在: $ENV_FILE"
  printf "覆盖现有文件? [y/N] "
  read -r confirm
  if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    log "已取消"
    exit 0
  fi
  cp "$ENV_FILE" "${ENV_FILE}.bak.$(date +%s)"
  log "已备份旧文件"
fi

# ── 数据库配置 ─────────────────────────────────────────────

echo ""
log "正在生成生产环境配置..."
echo ""

# SQLite 单文件数据库，无需交互
DATABASE_URL="${DATABASE_URL:-file:/data/claudedock.db}"
log "使用 SQLite 数据库: ${DATABASE_URL}"

# ── 镜像源检测 ──────────────────────────────────────────────

CONTAINER_REGISTRY=""
if command -v curl &>/dev/null; then
  echo ""
  log "正在检测 ghcr.io 连通性..."
  if curl -sSf --connect-timeout 5 --max-time 10 "https://ghcr.io" >/dev/null 2>&1; then
    log "ghcr.io 直接可达"
  else
    warn "ghcr.io 不可达（可能在防火墙后），建议使用镜像源 ghcr.1ms.run"
    echo ""
    printf "是否将镜像源切换为 ghcr.1ms.run（毫秒镜像）? [Y/n]: "
    read -r USE_MIRROR
    USE_MIRROR="${USE_MIRROR:-y}"
    if [[ "$USE_MIRROR" == "y" || "$USE_MIRROR" == "Y" ]]; then
      CONTAINER_REGISTRY="ghcr.1ms.run"
      log "将使用中国大陆镜像源"
    fi
  fi
else
  warn "未检测到 curl，跳过镜像源检测"
fi

# ── 控制面和管理员 ──────────────────────────────────────────

echo ""
printf "管理员用户名 [admin]: "
read -r ADMIN_USER
ADMIN_USER="${ADMIN_USER:-admin}"

ADMIN_PASSWORD="$(rand_password 20)"
ADMIN_JWT_SECRET="$(rand_password 48)"

# ── 写入 .env ───────────────────────────────────────────────

{
  cat <<EOF
# ============================================================
# ClaudeDock — 生产环境配置
# 由 setup-env.sh 自动生成于 $(date -u '+%Y-%m-%d %H:%M:%S UTC')
# ============================================================

# Database (SQLite, WAL 模式)
DATABASE_URL=${DATABASE_URL}

# Control Plane (API + Admin UI + SSH Proxy 统一在 :8080)
CONTROL_PLANE_ADDR=:8080
ADMIN_USERNAME=${ADMIN_USER}
ADMIN_PASSWORD=${ADMIN_PASSWORD}
ADMIN_JWT_SECRET=${ADMIN_JWT_SECRET}

# Container Registry
CONTAINER_REGISTRY=${CONTAINER_REGISTRY}

# Logging
LOG_FORMAT=json
LOG_LEVEL=info
EOF
} > "$ENV_FILE"

chmod 600 "$ENV_FILE"

# ── 输出摘要 ────────────────────────────────────────────────

echo ""
log "========================================="
log ".env 已生成: $ENV_FILE"
log "========================================="
echo ""
echo "  数据库:   SQLite (${DATABASE_URL})"
echo "  管理员用户名:   ${ADMIN_USER}"
echo "  管理员密码:     ${ADMIN_PASSWORD}"
echo "  JWT Secret:     ${ADMIN_JWT_SECRET:0:12}... (已写入 .env)"
echo ""
warn "请立即保存以上密码，此处仅显示一次！"
echo ""
log "下一步:"
log "  docker compose up -d --build"
log "  管理后台: http://YOUR_HOST:8080"
