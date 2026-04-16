#!/usr/bin/env bash
set -euo pipefail

# Cloud Claude CLI installer
# Usage: curl -fsSL https://raw.githubusercontent.com/ZaneL1u/cloud-cli-proxy/main/scripts/install.sh | bash

REPO="ZaneL1u/cloud-cli-proxy"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY="cloud-claude"

info()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn()  { printf '\033[1;33m警告:\033[0m %s\n' "$*"; }
error() { printf '\033[1;31m错误:\033[0m %s\n' "$*" >&2; exit 1; }

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "${os}" in
    linux)  os="linux" ;;
    darwin) os="darwin" ;;
    *)      error "不支持的操作系统: ${os}" ;;
  esac

  case "${arch}" in
    x86_64|amd64)  arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *)             error "不支持的架构: ${arch}" ;;
  esac

  echo "${os}-${arch}"
}

get_latest_version() {
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' \
    || error "无法获取最新版本号，请检查网络连接"
}

main() {
  local platform version archive url tmp

  platform="$(detect_platform)"
  version="${1:-$(get_latest_version)}"
  archive="${BINARY}-${platform}.tar.gz"
  url="https://github.com/${REPO}/releases/download/${version}/${archive}"
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp}"' EXIT

  info "下载 ${BINARY} ${version} (${platform})..."
  curl -fsSL -o "${tmp}/${archive}" "${url}" \
    || error "下载失败: ${url}"

  info "校验完整性..."
  local sha_url="${url}.sha256"
  if curl -fsSL -o "${tmp}/${archive}.sha256" "${sha_url}" 2>/dev/null; then
    (cd "${tmp}" && sha256sum -c "${archive}.sha256" --quiet 2>/dev/null) \
      || (cd "${tmp}" && shasum -a 256 -c "${archive}.sha256" --quiet 2>/dev/null) \
      || warn "sha256 校验失败，继续安装"
  fi

  tar xzf "${tmp}/${archive}" -C "${tmp}"
  chmod +x "${tmp}/${BINARY}-${platform}"

  info "安装到 ${INSTALL_DIR}/${BINARY}..."
  if [ -w "${INSTALL_DIR}" ]; then
    mv "${tmp}/${BINARY}-${platform}" "${INSTALL_DIR}/${BINARY}"
  else
    sudo mv "${tmp}/${BINARY}-${platform}" "${INSTALL_DIR}/${BINARY}"
  fi

  if command -v "${BINARY}" &>/dev/null; then
    info "安装成功! $("${BINARY}" --version 2>/dev/null || echo "${version}")"
  else
    warn "${BINARY} 已安装到 ${INSTALL_DIR}/${BINARY}，但不在 PATH 中"
  fi

  echo ""
  info "开始使用："
  echo "  cloud-claude init    # 配置网关与凭证"
  echo "  cloud-claude         # 启动 Claude Code 会话"
}

main "$@"
