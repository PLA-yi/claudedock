#!/bin/bash
# ClaudeDock 一键启动器（macOS 双击运行）
# 作用：自动打开 Docker Desktop → 等待就绪 → docker compose up -d → 健康检查 → 打开后台
set -uo pipefail

# 双击时 launchd 给的 PATH 很精简，手动补上 docker 常见安装位置
export PATH="/usr/local/bin:/opt/homebrew/bin:/Applications/Docker.app/Contents/Resources/bin:$PATH"

# 定位到脚本所在目录（即项目根目录）
PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR" || { echo "找不到项目目录"; exit 1; }

c()    { printf "\033[%sm" "$1"; }
info() { echo "$(c 36)[ClaudeDock]$(c 0) $*"; }
ok()   { echo "$(c 32)[ClaudeDock]$(c 0) $*"; }
err()  { echo "$(c 31)[ClaudeDock]$(c 0) $*" >&2; }
pause_exit() { echo; echo "按回车键关闭本窗口..."; read -r; exit "${1:-0}"; }

echo "=================================================="
echo "            ClaudeDock 一键启动"
echo "=================================================="
info "项目目录: $PROJECT_DIR"

# 1. 检查 docker CLI
if ! command -v docker >/dev/null 2>&1; then
  err "未找到 docker 命令。请先安装 Docker Desktop："
  err "  https://www.docker.com/products/docker-desktop/"
  pause_exit 1
fi

# 2. 确保 Docker 守护进程在跑；没跑就打开 Docker Desktop 并等待
if ! docker info >/dev/null 2>&1; then
  info "Docker 未运行，正在启动 Docker Desktop..."
  open -a Docker 2>/dev/null || open -a "Docker Desktop" 2>/dev/null || {
    err "无法自动启动 Docker Desktop，请手动打开后重试。"
    pause_exit 1
  }
  info "等待 Docker 就绪（最多约 120 秒）..."
  for _ in $(seq 1 60); do
    docker info >/dev/null 2>&1 && break
    sleep 2; printf "."
  done
  echo
  if ! docker info >/dev/null 2>&1; then
    err "Docker 启动超时。请等 Docker Desktop 完全启动（鲸鱼图标不再闪动）后再双击本脚本。"
    pause_exit 1
  fi
fi
ok "Docker 已就绪"

# 3. 选择 compose 命令（优先 v2 插件）
if docker compose version >/dev/null 2>&1; then
  COMPOSE="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE="docker-compose"
else
  err "未找到 docker compose，请升级 Docker Desktop。"
  pause_exit 1
fi

# 3.2 本地构建：叠加 build 覆盖文件，避免拉取私有 ghcr 镜像；按本机架构构建
export COMPOSE_FILE="docker-compose.yml:docker-compose.build.yaml:docker-compose.macbuild.yml"
case "$(uname -m)" in
  arm64|aarch64) export CD_PLATFORM="linux/arm64" ;;
  *)             export CD_PLATFORM="linux/amd64" ;;
esac
info "运行模式: 本地源码构建（不拉取私有镜像） · 架构 $CD_PLATFORM"

# 3.5 自动挑选未被占用的宿主机端口（绝不动其它容器；旧实例占着 8080/2222 就自动换到空端口）
port_in_use() {
  local p="$1"
  lsof -nP -iTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1 && return 0
  docker ps --format '{{.Ports}}' 2>/dev/null | grep -q ":$p->" && return 0
  return 1
}
find_free_port() {
  local p="$1" max="$2"
  while port_in_use "$p"; do
    p=$((p + 1))
    [ "$p" -gt "$max" ] && { echo ""; return 1; }
  done
  echo "$p"
}
ADMIN_HOST_PORT="$(find_free_port 8080 8180)"
SSH_PROXY_PORT="$(find_free_port 2222 2320)"
if [ -z "$ADMIN_HOST_PORT" ] || [ -z "$SSH_PROXY_PORT" ]; then
  err "在预设范围内找不到可用端口，请释放一些端口后重试。"
  pause_exit 1
fi
export ADMIN_HOST_PORT SSH_PROXY_PORT
if [ "$ADMIN_HOST_PORT" = "8080" ]; then info "后台端口: 8080"; else info "后台端口 8080 被占用，自动改用 $ADMIN_HOST_PORT"; fi
if [ "$SSH_PROXY_PORT" = "2222" ]; then info "SSH 端口: 2222"; else info "SSH 端口 2222 被占用，自动改用 $SSH_PROXY_PORT"; fi

# 4. 确保 .env 存在（缺则自动生成随机密码，避免双击时卡在交互输入）
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

# 5. 本地构建控制面镜像并启动（不拉取私有镜像）
info "本地构建控制面镜像（首次较慢，需在容器内编译前端 + Go，请耐心等待）..."
if ! $COMPOSE build control-plane; then
  err "镜像构建失败。查看详细输出：cd \"$PROJECT_DIR\" && $COMPOSE build control-plane"
  pause_exit 1
fi
info "启动控制面服务..."
if ! $COMPOSE up -d control-plane; then
  err "启动失败。若在 macOS 上，常见原因："
  echo "   · 本 compose 为 Linux 宿主机设计，含 /var/lib/claudedock 绑定挂载与 pid:host，"
  echo "     在 Docker Desktop 上可能需在「Settings → Resources → File Sharing」共享该路径，"
  echo "     或直接部署到 Linux 服务器。"
  echo "   · 查看详细日志：$COMPOSE logs"
  pause_exit 1
fi

# 6. 等待健康检查（用实际挑中的宿主机端口）
info "等待服务健康（http://localhost:${ADMIN_HOST_PORT}/healthz，最多约 60 秒）..."
HEALTHY=0
for _ in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:${ADMIN_HOST_PORT}/healthz" >/dev/null 2>&1; then HEALTHY=1; break; fi
  sleep 2; printf "."
done
echo
if [ "$HEALTHY" = "1" ]; then
  ok "服务已就绪 🎉"
  info "管理后台: http://localhost:${ADMIN_HOST_PORT}   （账号见 .env 里的 ADMIN_USERNAME / ADMIN_PASSWORD）"
  open "http://localhost:${ADMIN_HOST_PORT}" 2>/dev/null || true
else
  err "健康检查暂未通过，服务可能仍在启动。可运行： $COMPOSE logs -f 查看进度。"
fi

echo
echo "常用命令（在项目目录执行；注意带上构建配置）："
echo "  export COMPOSE_FILE=$COMPOSE_FILE"
echo "  $COMPOSE ps        # 查看状态"
echo "  $COMPOSE logs -f   # 实时日志"
echo "  $COMPOSE down      # 停止并移除容器"
echo "  重新构建更新: $COMPOSE build control-plane && $COMPOSE up -d control-plane"
pause_exit 0
