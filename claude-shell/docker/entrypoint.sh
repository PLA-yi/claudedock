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

# 生成 per-container unique machine-id
setup_fingerprint() {
  local h; h="$(hostname)"
  local t; t="$(cat /proc/uptime 2>/dev/null | tr -d ' .')"
  local mid; mid="$(echo -n "${h}-${t}" | sha256sum | cut -c1-32)"
  echo "$mid" > /etc/machine-id
  echo "$mid" > /var/lib/dbus/machine-id 2>/dev/null || true
  chmod 444 /etc/machine-id
  log "step=fingerprint machine-id=$mid"
}

# 容器检测绕过 + 遥测阻断环境变量
setup_anti_detect() {
  rm -f /.dockerenv
  export DISABLE_TELEMETRY=1
  export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
  export CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=
  export DO_NOT_TRACK=1
  export OTEL_SDK_DISABLED=true
  export OTEL_TRACES_EXPORTER=none
  export OTEL_METRICS_EXPORTER=none
  export OTEL_LOGS_EXPORTER=none
  export SENTRY_DSN=
  export DISABLE_ERROR_REPORTING=1
  export TELEMETRY_DISABLED=1
  log "step=anti_detect /.dockerenv removed, telemetry blocked"
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
