#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# claude-shell entrypoint — step-function orchestration
# Order: network → fingerprint → anti_detect → claude (D-08)
# ---------------------------------------------------------------------------

readonly EXIT_NETWORK=10
readonly EXIT_FINGERPRINT=20
readonly EXIT_ANTI_DETECT=30
readonly EXIT_CLAUDE=40

# ---------------------------------------------------------------------------
# 日志工具函数
# ---------------------------------------------------------------------------
log() { echo "[entrypoint] $(date -u +%Y-%m-%dT%H:%M:%SZ) $*"; }
die() { log "FATAL: $1"; exit "${2:-1}"; }

# ---------------------------------------------------------------------------
# 步骤函数
# ---------------------------------------------------------------------------

# Phase 18 将实现: sing-box tun + nftables
setup_network() {
  log "step=network status=placeholder"
}

# Phase 21 将实现: machine-id, /proc overrides
setup_fingerprint() {
  log "step=fingerprint status=placeholder"
}

# Phase 21 将实现: /.dockerenv cleanup, cgroup mask
setup_anti_detect() {
  log "step=anti_detect status=placeholder"
}

# 查找 claude 二进制，exec 替换为 PID 1（D-10）
start_claude() {
  log "step=claude status=starting"
  local claude_bin
  claude_bin="$(command -v claude 2>/dev/null)" \
    || die "claude binary not found in PATH" $EXIT_CLAUDE
  exec "$claude_bin" "$@"
}

# ---------------------------------------------------------------------------
# 主流程（D-08 顺序：网络 → 指纹 → 反检测 → Claude）
# ---------------------------------------------------------------------------
setup_network      || die "network setup failed"      $EXIT_NETWORK
setup_fingerprint  || die "fingerprint setup failed"  $EXIT_FINGERPRINT
setup_anti_detect  || die "anti-detect setup failed"  $EXIT_ANTI_DETECT
start_claude "$@"
