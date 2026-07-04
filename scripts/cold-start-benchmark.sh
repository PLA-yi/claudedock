#!/usr/bin/env bash
# scripts/cold-start-benchmark.sh — Phase 35 BASE-02 首连 ≤ 8s + 三段式中文进度断言
#
# 默认 5 次 attempt，每次：起 managed-user 容器 → 跑 claudedock → 探测 prompt → 关容器。
# 同时断言 stderr 含三段式中文进度（REQ-F1-B 锁定字符串）。
# threshold_seconds 默认 8 秒 (CONTEXT 锁定)；min-pass 默认 4 (5 次中 ≥ 4 次 ≤ 8s)。
#
# 退出码：
#   0  通过（pass ≥ min-pass 且 progress_matches_all=true）
#   1  FAIL（达标次数不足或缺三段式进度）
#   2  SKIP（无 docker / CI 无特权 / 缺受管镜像）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BENCH_DIR="${PROJECT_ROOT}/.planning/phases/35-e2e/benchmarks"

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0
SKIP_COUNT=0

pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
warn() { echo "[WARN]  $1"; WARN_COUNT=$((WARN_COUNT + 1)); }
info() { echo "[INFO]  $1"; }
skip() { echo "[SKIP]  $1: $2"; SKIP_COUNT=$((SKIP_COUNT + 1)); }

usage() {
  cat <<'EOF'
cold-start-benchmark.sh — Phase 35 BASE-02 (首连 ≤ 8s + 三段式进度) 基准

用法: scripts/cold-start-benchmark.sh [--attempts=N] [--threshold-seconds=N]
                                       [--min-pass=N] [--output-dir=DIR] [--ci-mode] [--help]

选项:
  --attempts=5            attempt 次数（默认 5，CONTEXT 锁定）
  --threshold-seconds=8   首连阈值 ≤ 8s（默认 8，CONTEXT 锁定）
  --min-pass=4            5 次中至少达标次数（默认 4，"5 次中 ≥ 4 次 ≤ 8s"）
  --output-dir=DIR        报告输出目录（默认 .planning/phases/35-e2e/benchmarks）
  --ci-mode               CI 模式：is_ci && skip BASE-02 → exit 2 不阻塞
  --help, -h              显示本帮助

裁决：
  - summary.pass >= --min-pass 且 summary.progress_matches_all == true → exit 0
  - 否则 → exit 1

三段式中文进度断言（REQ-F1-B 锁定字符串）：
  "初始化文件映射 (1/3) 热同步源码中"
  "(2/3) 启动冷兜底"
  "(3/3) 合并视图"
EOF
}

ATTEMPTS=5
THRESHOLD_SECONDS=8
MIN_PASS=4
OUTPUT_DIR="$BENCH_DIR"
CI_MODE=false

for arg in "$@"; do
  case "$arg" in
    --attempts=*) ATTEMPTS="${arg#--attempts=}" ;;
    --threshold-seconds=*) THRESHOLD_SECONDS="${arg#--threshold-seconds=}" ;;
    --min-pass=*) MIN_PASS="${arg#--min-pass=}" ;;
    --output-dir=*) OUTPUT_DIR="${arg#--output-dir=}" ;;
    --ci-mode) CI_MODE=true ;;
    --help|-h) usage; exit 0 ;;
    *) fail "未知参数: $arg"; usage; exit 1 ;;
  esac
done

THRESHOLD_MS=$((THRESHOLD_SECONDS * 1000))
HARD_TIMEOUT_MS=15000

# ms 时钟（macOS BSD date 无 %3N，回退到 perl Time::HiRes）
ms_now() {
  if date +%s%3N 2>/dev/null | grep -qE '^[0-9]{13,}$'; then
    date +%s%3N
  else
    perl -MTime::HiRes -e 'printf "%d", Time::HiRes::time() * 1000'
  fi
}

is_ci() { [[ "${CI:-}" == "true" ]]; }

has_docker_privileged() {
  command -v docker >/dev/null 2>&1 || return 1
  # 尝试启动一个最小特权容器探测 SYS_ADMIN+/dev/fuse 是否可用
  docker run --rm \
    --cap-add SYS_ADMIN --device /dev/fuse \
    --security-opt apparmor=unconfined \
    alpine:3 true >/dev/null 2>&1
}

# CI 感知 SKIP（Pattern B）：is_ci && skip
if [ "$CI_MODE" = true ] && is_ci && ! has_docker_privileged; then
  skip "BASE-02" "CI 无 FUSE 特权（缺 SYS_ADMIN / /dev/fuse），本档仅真机执行"
  exit 2
fi

if ! command -v docker >/dev/null 2>&1; then
  skip "BASE-02" "docker engine 未安装，无法测量首连"
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  fail "缺少 jq (brew install jq / apt install jq)"
  exit 2
fi

IMAGE_NAME="$(awk -F': ' '$1 == "local_dev_image_name" { print $2 }' \
  "${PROJECT_ROOT}/deploy/docker/managed-user/image.lock")"
if [ -z "$IMAGE_NAME" ]; then
  fail "image.lock 解析 local_dev_image_name 失败"
  exit 2
fi
info "使用受管镜像: $IMAGE_NAME"

mkdir -p "$OUTPUT_DIR"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
REPORT_JSON="${OUTPUT_DIR}/cold-start-${TIMESTAMP}.json"
REPORT_MD="${OUTPUT_DIR}/cold-start-${TIMESTAMP}.md"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

ATTEMPTS_JSON="$WORK/attempts.json"
echo '[]' > "$ATTEMPTS_JSON"

PASS_TOTAL=0
FAIL_TOTAL=0
PROGRESS_ALL=true

for idx in $(seq 1 "$ATTEMPTS"); do
  info "attempt #${idx}/${ATTEMPTS} 启动容器"
  CTR="cold-start-bench-$$-${idx}"

  if ! docker run -d --name "$CTR" \
    --cap-add SYS_ADMIN --device /dev/fuse \
    --security-opt apparmor=unconfined \
    "$IMAGE_NAME" sleep 600 >/dev/null 2>&1; then
    fail "attempt #${idx}: 容器启动失败"
    PROGRESS_ALL=false
    FAIL_TOTAL=$((FAIL_TOTAL + 1))
    jq --argjson idx "$idx" \
       '. + [{ idx: $idx, duration_ms: -1, stderr_progress_matches: false, outcome: "fail" }]' \
       "$ATTEMPTS_JSON" > "$WORK/.tmp.json" && mv "$WORK/.tmp.json" "$ATTEMPTS_JSON"
    continue
  fi

  STDERR_FILE="${WORK}/attempt-${idx}.stderr"
  : > "$STDERR_FILE"

  T0=$(ms_now)
  # 后台启动 claudedock，不阻塞探测循环
  docker exec "$CTR" claudedock --mount-mode=auto --json 2> "$STDERR_FILE" >/dev/null &
  CLI_PID=$!

  # Pitfall 3：tmux race，初次 capture-pane 前 sleep 2，最多 3 次重试
  sleep 2

  T1=-1
  ELAPSED_MS=0
  while [ "$ELAPSED_MS" -lt "$HARD_TIMEOUT_MS" ]; do
    # 200ms 一次探测；3 次重试在 capture-pane race 时
    READY=false
    for retry in 1 2 3; do
      if docker exec "$CTR" tmux capture-pane -p 2>/dev/null \
         | grep -qE '(^>|claude> |➜ )'; then
        READY=true
        break
      fi
      sleep 0.05
    done
    if [ "$READY" = true ]; then
      T1=$(ms_now)
      break
    fi
    sleep 0.2
    NOW=$(ms_now)
    ELAPSED_MS=$((NOW - T0))
  done

  if [ "$T1" = "-1" ]; then
    DURATION_MS=-1
    info "attempt #${idx}: 硬超时 ${HARD_TIMEOUT_MS}ms 未探测到 prompt"
  else
    DURATION_MS=$((T1 - T0))
    info "attempt #${idx}: duration_ms=${DURATION_MS}"
  fi

  # 三段式中文进度断言（REQ-F1-B 锁定 verbatim 字符串，不用 regex 偷懒）
  STAGE_1=false; STAGE_2=false; STAGE_3=false
  if grep -qF '初始化文件映射 (1/3) 热同步源码中' "$STDERR_FILE" 2>/dev/null; then STAGE_1=true; fi
  if grep -qF '(2/3) 启动冷兜底' "$STDERR_FILE" 2>/dev/null; then STAGE_2=true; fi
  if grep -qF '(3/3) 合并视图' "$STDERR_FILE" 2>/dev/null; then STAGE_3=true; fi

  STDERR_PROGRESS_MATCHES=false
  if [ "$STAGE_1" = true ] && [ "$STAGE_2" = true ] && [ "$STAGE_3" = true ]; then
    STDERR_PROGRESS_MATCHES=true
  else
    PROGRESS_ALL=false
  fi

  OUTCOME="fail"
  if [ "$DURATION_MS" -ge 0 ] && [ "$DURATION_MS" -le "$THRESHOLD_MS" ]; then
    OUTCOME="pass"
    PASS_TOTAL=$((PASS_TOTAL + 1))
  else
    FAIL_TOTAL=$((FAIL_TOTAL + 1))
  fi

  jq --argjson idx "$idx" \
     --argjson dur "$DURATION_MS" \
     --argjson sm "$STDERR_PROGRESS_MATCHES" \
     --arg outcome "$OUTCOME" \
     '. + [{ idx: $idx, duration_ms: $dur, stderr_progress_matches: $sm, outcome: $outcome }]' \
     "$ATTEMPTS_JSON" > "$WORK/.tmp.json" && mv "$WORK/.tmp.json" "$ATTEMPTS_JSON"

  kill "$CLI_PID" 2>/dev/null || true
  docker rm -f "$CTR" >/dev/null 2>&1 || true
done

# 聚合 JSON 报告（schema_version=1 锁定 + summary 块）
jq --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
   --argjson attempts_total "$ATTEMPTS" \
   --argjson pass_total "$PASS_TOTAL" \
   --argjson fail_total "$FAIL_TOTAL" \
   --argjson threshold_ms "$THRESHOLD_MS" \
   --argjson min_pass "$MIN_PASS" \
   --argjson progress_all "$PROGRESS_ALL" \
   --arg image "$IMAGE_NAME" \
   '{
     schema_version: 1,
     kind: "cold-start-benchmark",
     timestamp: $ts,
     image: $image,
     config: { attempts: $attempts_total, threshold_ms: $threshold_ms, min_pass: $min_pass },
     attempts: .,
     summary: {
       total: $attempts_total,
       pass: $pass_total,
       fail: $fail_total,
       threshold_ms: $threshold_ms,
       progress_matches_all: $progress_all
     }
   }' "$ATTEMPTS_JSON" > "$REPORT_JSON"

# Markdown 报告
{
  echo "# BASE-02 cold-start 报告 — ${TIMESTAMP}"
  echo ""
  echo "> 适用版本：v3.0+；脚本：scripts/cold-start-benchmark.sh"
  echo "> 配置：attempts=${ATTEMPTS}, threshold_seconds=${THRESHOLD_SECONDS}, min_pass=${MIN_PASS}"
  echo ""
  echo "## attempts 明细"
  echo ""
  echo "| # | duration(ms) | progress | outcome |"
  echo "|---|--------------|----------|---------|"
  jq -r '.[] | "| \(.idx) | \(.duration_ms) | \(.stderr_progress_matches) | \(.outcome) |"' "$ATTEMPTS_JSON"
  echo ""
  echo "## summary"
  echo ""
  echo "- total: ${ATTEMPTS}"
  echo "- pass: ${PASS_TOTAL}"
  echo "- fail: ${FAIL_TOTAL}"
  echo "- threshold_ms: ${THRESHOLD_MS}"
  echo "- min_pass: ${MIN_PASS}"
  echo "- progress_matches_all: ${PROGRESS_ALL}"
} > "$REPORT_MD"

info "JSON 报告: $REPORT_JSON"
info "MD   报告: $REPORT_MD"

# 裁决闸门
EXIT_CODE=0
if [ "$PASS_TOTAL" -lt "$MIN_PASS" ]; then
  fail "BASE-02 首连 ≤ ${THRESHOLD_SECONDS}s 达标次数 ${PASS_TOTAL}/${ATTEMPTS} 低于要求 ${MIN_PASS}"
  EXIT_CODE=1
fi
if [ "$PROGRESS_ALL" != "true" ]; then
  fail "REQ-F1-B 三段式进度缺失（progress_matches_all=${PROGRESS_ALL}）"
  EXIT_CODE=1
fi
if [ "$EXIT_CODE" -eq 0 ]; then
  pass "BASE-02 ok：${PASS_TOTAL}/${ATTEMPTS} 达标 + 三段式进度全命中"
fi

echo ""
echo "========================================"
echo "cold-start-benchmark: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${WARN_COUNT} WARN, ${SKIP_COUNT} SKIP"
exit "$EXIT_CODE"
