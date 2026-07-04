#!/bin/bash
# ClaudeDock 一键启动（公司成员双击即可，一个脚本搞定全部）：
#   检查/启动 Docker → 生成环境配置(.env) → 获取镜像(优先从公司 registry 拉取，拉不到自动本地源码构建)
#   → 启动控制面 → 健康检查 → 打开管理后台
# 进阶：REBUILD=1 双击本脚本 = 跳过拉取、强制本地干净重建（--no-cache）。
set -uo pipefail

# 双击时 launchd 给的 PATH 很精简，手动补上 docker 常见安装位置
export PATH="/usr/local/bin:/opt/homebrew/bin:/Applications/Docker.app/Contents/Resources/bin:$PATH"
PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR" || { echo "找不到项目目录"; exit 1; }

# ★ 公司镜像仓库地址：改成你们自己的 ghcr 组织即可（成员无需改，跟着走）
# 注意：Docker 镜像名必须全小写，这里务必用小写（GitHub 组织 PLA-yi → 写成 pla-yi）
REGISTRY_DEFAULT="ghcr.io/pla-yi"

c(){ printf "\033[%sm" "$1"; }
info(){ echo "$(c 36)[ClaudeDock]$(c 0) $*"; }
ok(){ echo "$(c 32)[ClaudeDock]$(c 0) $*"; }
err(){ echo "$(c 31)[ClaudeDock]$(c 0) $*" >&2; }
pause_exit(){ echo; echo "按回车键关闭本窗口..."; read -r; exit "${1:-0}"; }

echo "=================================================="
echo "            ClaudeDock 一键启动"
echo "=================================================="
info "项目目录: $PROJECT_DIR"

# 1. docker CLI
command -v docker >/dev/null 2>&1 || { err "未找到 docker，请先安装 Docker Desktop：https://www.docker.com/products/docker-desktop/"; pause_exit 1; }

# 2. docker daemon（没跑就打开 Docker Desktop 并等待）
if ! docker info >/dev/null 2>&1; then
  info "Docker 未运行，正在启动 Docker Desktop..."
  open -a Docker 2>/dev/null || open -a "Docker Desktop" 2>/dev/null || { err "无法自动启动 Docker Desktop，请手动打开后重试。"; pause_exit 1; }
  info "等待 Docker 就绪（最多约 120 秒）..."
  for _ in $(seq 1 60); do docker info >/dev/null 2>&1 && break; sleep 2; printf "."; done; echo
  docker info >/dev/null 2>&1 || { err "Docker 启动超时。请等 Docker Desktop 完全启动后再双击本脚本。"; pause_exit 1; }
fi
ok "Docker 已就绪"

# 3. 选择 compose 命令
if docker compose version >/dev/null 2>&1; then COMPOSE="docker compose";
elif command -v docker-compose >/dev/null 2>&1; then COMPOSE="docker-compose";
else err "未找到 docker compose，请升级 Docker Desktop。"; pause_exit 1; fi

# 3.2 registry + 本机架构 + compose 文件组合
export CONTAINER_REGISTRY="${CONTAINER_REGISTRY:-$REGISTRY_DEFAULT}"
# Docker 镜像名必须全小写，自动转小写兜底（防止大写导致 "repository name must be lowercase"）
export CONTAINER_REGISTRY="$(printf '%s' "$CONTAINER_REGISTRY" | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in arm64|aarch64) export CD_PLATFORM="linux/arm64";; *) export CD_PLATFORM="linux/amd64";; esac
PULL_FILES="docker-compose.yml:docker-compose.platform.yml"
BUILD_FILES="docker-compose.yml:docker-compose.build.yaml:docker-compose.platform.yml"
info "镜像仓库: $CONTAINER_REGISTRY · 架构: $CD_PLATFORM"

# 3.5 自动挑选未被占用的宿主机端口
port_in_use(){ lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1 && return 0; docker ps --format '{{.Ports}}' 2>/dev/null | grep -q ":$1->" && return 0; return 1; }
find_free_port(){ local p="$1" max="$2"; while port_in_use "$p"; do p=$((p+1)); [ "$p" -gt "$max" ] && { echo ""; return 1; }; done; echo "$p"; }
ADMIN_HOST_PORT="$(find_free_port 8080 8180)"; SSH_PROXY_PORT="$(find_free_port 2222 2320)"
if [ -z "$ADMIN_HOST_PORT" ] || [ -z "$SSH_PROXY_PORT" ]; then err "在预设范围内找不到可用端口，请释放一些端口后重试。"; pause_exit 1; fi
export ADMIN_HOST_PORT SSH_PROXY_PORT
[ "$ADMIN_HOST_PORT" = "8080" ] && info "后台端口: 8080" || info "后台端口 8080 被占用，自动改用 $ADMIN_HOST_PORT"
[ "$SSH_PROXY_PORT" = "2222" ] && info "SSH 端口: 2222" || info "SSH 端口 2222 被占用，自动改用 $SSH_PROXY_PORT"

# 4. 确保 .env（缺则自动生成随机管理员密码，避免双击时卡在交互输入）
if [ ! -f "$PROJECT_DIR/.env" ]; then
  info ".env 不存在，自动生成随机管理员密码与 JWT 密钥..."
  GEN_PW="$(head -c 24 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 20)"
  GEN_JWT="$(head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 48)"
  cat > "$PROJECT_DIR/.env" <<EOF
DATABASE_URL=file:/data/claudedock.db?_texttotime=true
CONTROL_PLANE_ADDR=:8080
ADMIN_USERNAME=admin
ADMIN_PASSWORD=$GEN_PW
ADMIN_JWT_SECRET=$GEN_JWT
LOG_FORMAT=text
LOG_LEVEL=info
EOF
  chmod 600 "$PROJECT_DIR/.env"
  ok "已生成 .env → 管理员 admin / 密码 $(c 33)$GEN_PW$(c 0)"
  echo "   （密码也保存在 $PROJECT_DIR/.env，请妥善记录）"
fi

# 5. 获取镜像：先从公司 registry 拉取；拉不到（未授权/未公开/离线）则本地源码构建
if [ "${REBUILD:-0}" = "1" ]; then
  info "REBUILD=1：跳过拉取，强制本地干净重建（--no-cache，较慢）..."
  export COMPOSE_FILE="$BUILD_FILES"
  if ! $COMPOSE build --no-cache control-plane; then err "构建失败。"; pause_exit 1; fi
  ok "本地重建完成"
elif COMPOSE_FILE="$PULL_FILES" $COMPOSE pull control-plane >/dev/null 2>&1; then
  export COMPOSE_FILE="$PULL_FILES"
  ok "已从 $CONTAINER_REGISTRY 拉取预构建镜像（免本地构建，快）"
  docker pull "$CONTAINER_REGISTRY/claudedock/claudedock/managed-user:latest" >/dev/null 2>&1 \
    && ok "已预拉用户容器镜像" || info "用户容器镜像将在创建主机时自动拉取"
else
  info "拉取不可用（未授权/未公开/离线），改为本地源码构建（首次较慢：容器内编译前端 + Go）..."
  export COMPOSE_FILE="$BUILD_FILES"
  if ! $COMPOSE build control-plane; then err "构建失败。查看详细输出：$COMPOSE build control-plane"; pause_exit 1; fi
  ok "本地构建完成"
fi

# 6. 启动控制面
info "启动控制面服务..."
if ! $COMPOSE up -d control-plane; then
  err "启动失败。若在 macOS Docker Desktop 上，可能需在「Settings → Resources → File Sharing」共享 /var/lib/claudedock，或部署到 Linux 服务器。查看日志：$COMPOSE logs"
  pause_exit 1
fi

# 7. 健康检查并打开后台
info "等待服务健康（http://localhost:${ADMIN_HOST_PORT}/healthz，最多约 60 秒）..."
HEALTHY=0
for _ in $(seq 1 30); do curl -sf "http://127.0.0.1:${ADMIN_HOST_PORT}/healthz" >/dev/null 2>&1 && { HEALTHY=1; break; }; sleep 2; printf "."; done; echo
if [ "$HEALTHY" = "1" ]; then
  ok "服务已就绪 🎉"
  info "管理后台: http://localhost:${ADMIN_HOST_PORT}   （账号 admin，密码见 .env 里的 ADMIN_PASSWORD）"
  open "http://localhost:${ADMIN_HOST_PORT}" 2>/dev/null || true
else
  err "健康检查暂未通过，服务可能仍在启动。可运行：$COMPOSE logs -f 查看进度。"
fi

echo
echo "常用命令（在项目目录执行）："
echo "  export COMPOSE_FILE=$COMPOSE_FILE"
echo "  export CONTAINER_REGISTRY=$CONTAINER_REGISTRY"
echo "  $COMPOSE ps        # 查看状态"
echo "  $COMPOSE logs -f   # 实时日志"
echo "  $COMPOSE down      # 停止并移除容器"
echo "  强制干净重建再启动:  REBUILD=1 双击本脚本"
pause_exit 0
