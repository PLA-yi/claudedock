#!/usr/bin/env bash
# 拉取 Mutagen v0.18.1 4 平台 release tarball、解包出 mutagen 二进制并 sha256 校验。
# 用法：scripts/fetch-mutagen-bins.sh [--check-only]
#   默认：拉取并写入 internal/cloudclaude/mutagen_bin/<plat>/mutagen
#   --check-only：只校验已存在文件的 sha256，不联网
set -euo pipefail

VERSION="v0.18.1"
BASE_URL="https://github.com/mutagen-io/mutagen/releases/download/${VERSION}"
OUT_DIR="$(cd "$(dirname "$0")/.." && pwd)/internal/cloudclaude/mutagen_bin"
SUMS="${OUT_DIR}/SHA256SUMS"

# macOS 默认无 sha256sum，提供回退到 shasum -a 256（输出格式兼容）
if ! command -v sha256sum >/dev/null 2>&1; then
  sha256sum() { shasum -a 256 "$@"; }
  export -f sha256sum
fi

PLATFORMS=(
  "darwin_amd64:mutagen_darwin_amd64_v0.18.1.tar.gz"
  "darwin_arm64:mutagen_darwin_arm64_v0.18.1.tar.gz"
  "linux_amd64:mutagen_linux_amd64_v0.18.1.tar.gz"
  "linux_arm64:mutagen_linux_arm64_v0.18.1.tar.gz"
)

mkdir -p "${OUT_DIR}"

if [[ "${1:-}" == "--check-only" ]]; then
  if [[ ! -f "${SUMS}" ]]; then
    echo "SHA256SUMS 不存在: ${SUMS}" >&2
    exit 1
  fi
  if grep -q '^PENDING-FETCH ' "${SUMS}"; then
    echo "SHA256SUMS 仍含 PENDING-FETCH 占位，请先在联网环境运行不带 --check-only 的脚本" >&2
    exit 1
  fi
  (cd "${OUT_DIR}" && sha256sum -c SHA256SUMS)
  exit $?
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

: > "${SUMS}.new"

for entry in "${PLATFORMS[@]}"; do
  plat="${entry%%:*}"
  tarball="${entry#*:}"
  echo "=== ${plat}: 拉取 ${tarball}"
  curl -fsSL --retry 3 -o "${tmp}/${tarball}" "${BASE_URL}/${tarball}"
  mkdir -p "${OUT_DIR}/${plat}"
  tar -xzf "${tmp}/${tarball}" -C "${tmp}" mutagen
  install -m 0755 "${tmp}/mutagen" "${OUT_DIR}/${plat}/mutagen"
  (cd "${OUT_DIR}" && sha256sum "${plat}/mutagen") >> "${SUMS}.new"
  rm -f "${tmp}/mutagen"
done

mv "${SUMS}.new" "${SUMS}"
echo "=== 完成。SHA256SUMS:"
cat "${SUMS}"
