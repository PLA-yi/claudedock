#!/usr/bin/env bash
#
# 以伪装身份启动 Claude Code
#
# 用法：
#   ./claude-spoofed.sh                           # 默认伪装
#   SPOOF_HOSTNAME=my-vm ./claude-spoofed.sh      # 自定义主机名
#   SPOOF_DEBUG=1 ./claude-spoofed.sh             # 打印伪装信息
#
# 可配合代理使用（推荐）：
#   HTTPS_PROXY=http://127.0.0.1:8888 ./claude-spoofed.sh
#

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SPOOF_SCRIPT="$SCRIPT_DIR/spoof-fingerprint.js"
DNS_GUARD="$SCRIPT_DIR/dns-guard.js"

if [ ! -f "$SPOOF_SCRIPT" ]; then
  echo "Error: spoof-fingerprint.js not found at $SPOOF_SCRIPT"
  exit 1
fi

# 如果没有设置伪装 hostname，生成一个稳定的
export SPOOF_HOSTNAME="${SPOOF_HOSTNAME:-cloud-vm-$(echo -n "$(whoami)-$(date +%Y%m)" | shasum | cut -c1-6)}"

# 将 spoof 脚本 + DNS guard 注入到 Node.js 启动流程
export NODE_OPTIONS="--require $SPOOF_SCRIPT --require $DNS_GUARD ${NODE_OPTIONS:-}"
# Bun 二进制（Claude Code standalone）使用 BUN_OPTIONS --preload
export BUN_OPTIONS="--preload $SPOOF_SCRIPT --preload $DNS_GUARD ${BUN_OPTIONS:-}"

# ── 遥测阻断环境变量 ─────────────────────────────────────────
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

exec claude "$@"
