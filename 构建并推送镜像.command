#!/bin/bash
# 构建 ClaudeDock 镜像并推送到你们自己的 registry（方案 B）。
# 你双击运行一次 → 同事设同样的 CONTAINER_REGISTRY 后 `docker compose pull` 即可，无需本地构建。
set -uo pipefail

export PATH="/usr/local/bin:/opt/homebrew/bin:/Applications/Docker.app/Contents/Resources/bin:$PATH"
PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR" || { echo "找不到项目目录"; exit 1; }

c(){ printf "\033[%sm" "$1"; }
info(){ echo "$(c 36)[镜像]$(c 0) $*"; }
ok(){ echo "$(c 32)[镜像]$(c 0) $*"; }
err(){ echo "$(c 31)[镜像]$(c 0) $*" >&2; }
pause_exit(){ echo; echo "按回车键关闭本窗口..."; read -r; exit "${1:-0}"; }

echo "=================================================="
echo "        ClaudeDock 构建并推送镜像（方案 B）"
echo "=================================================="
info "项目目录: $PROJECT_DIR"

# 1. Docker 就绪检查
command -v docker >/dev/null 2>&1 || { err "未找到 docker，请先装 Docker Desktop 并打开。"; pause_exit 1; }
if ! docker info >/dev/null 2>&1; then
  info "Docker 未运行，尝试打开 Docker Desktop..."
  open -a Docker 2>/dev/null || open -a "Docker Desktop" 2>/dev/null
  for _ in $(seq 1 60); do docker info >/dev/null 2>&1 && break; sleep 2; printf "."; done; echo
  docker info >/dev/null 2>&1 || { err "Docker 未就绪，请等它完全启动后重试。"; pause_exit 1; }
fi
ok "Docker 已就绪"

# 2. 选择 compose 命令
if docker compose version >/dev/null 2>&1; then COMPOSE="docker compose";
elif command -v docker-compose >/dev/null 2>&1; then COMPOSE="docker-compose";
else err "未找到 docker compose。"; pause_exit 1; fi

# 3. registry 地址（默认 ghcr.io/PLA-yi，可改）
DEFAULT_REG="ghcr.io/PLA-yi"
printf "推送到哪个 registry？[回车用默认 %s]: " "$DEFAULT_REG"
read -r REG_INPUT
export CONTAINER_REGISTRY="${REG_INPUT:-$DEFAULT_REG}"
REG_HOST="${CONTAINER_REGISTRY%%/*}"   # 取第一个 / 之前，如 ghcr.io
info "目标 registry: $CONTAINER_REGISTRY  (登录服务器: $REG_HOST)"

# 4. 确认已登录 registry（未登录则引导登录）
if ! grep -q "\"$REG_HOST\"" "$HOME/.docker/config.json" 2>/dev/null; then
  info "检测到尚未登录 $REG_HOST，现在登录（ghcr 需要 GitHub 用户名 + Token，Token 需勾选 write:packages 权限）"
  printf "GitHub 用户名: "; read -r GH_USER
  printf "粘贴 GitHub Token（输入时不显示）: "; read -rs GH_TOKEN; echo
  if ! printf '%s' "$GH_TOKEN" | docker login "$REG_HOST" -u "$GH_USER" --password-stdin; then
    err "登录 $REG_HOST 失败，请检查用户名/Token。"; pause_exit 1
  fi
  ok "已登录 $REG_HOST"
else
  ok "已登录 $REG_HOST（复用现有凭证）"
fi

# 5. 本机架构（Apple Silicon 走 arm64，避免模拟 amd64 巨慢）
case "$(uname -m)" in arm64|aarch64) export CD_PLATFORM="linux/arm64";; *) export CD_PLATFORM="linux/amd64";; esac
export COMPOSE_FILE="docker-compose.yml:docker-compose.build.yaml:docker-compose.macbuild.yml"
info "构建架构: $CD_PLATFORM"

# 6. 构建（控制面 + 用户镜像；用户镜像较大，首次很慢）
info "构建 control-plane + managed-user（首次较慢，请耐心）..."
if ! $COMPOSE --profile build-only build control-plane managed-user; then
  err "构建失败，详见上面输出。"; pause_exit 1
fi
ok "构建完成"

# 7. 推送
info "推送到 $CONTAINER_REGISTRY ..."
if ! $COMPOSE --profile build-only push control-plane managed-user; then
  err "推送失败。常见原因：Token 无 write:packages 权限，或 registry 地址/账号不对。"; pause_exit 1
fi
ok "推送完成 🎉"

echo
echo "=================================================="
echo "  同事那边这样用（无需再本地构建）："
echo "=================================================="
echo "  cd claudedock-dev"
echo "  export CONTAINER_REGISTRY=$CONTAINER_REGISTRY"
echo "  export COMPOSE_FILE=docker-compose.yml:docker-compose.build.yaml"
echo "  docker login $REG_HOST      # 若包设为私有需登录；设为公开则免登录"
echo "  docker compose pull"
echo "  docker compose up -d control-plane"
echo
echo "  提示：想让同事免登录直接拉，去 GitHub 把这两个 package 设为 Public："
echo "    $CONTAINER_REGISTRY/claudedock/claudedock/control-plane"
echo "    $CONTAINER_REGISTRY/claudedock/claudedock/managed-user"
echo "  注意：镜像是 $CD_PLATFORM 架构，同事需同架构（都用 Apple Silicon 即可）。"
pause_exit 0
