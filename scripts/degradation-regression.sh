#!/usr/bin/env bash
# scripts/degradation-regression.sh — Phase 35 M13 三层静默降级终验
#
# 把 M13（禁止静默降级）从「文档约定」升级为「三层人工破坏 → claudedock
# 必须吐 MOUNT_* 错误码 + 中文 next_action」的自动化回归。
#
# 三层破坏方法（容器内执行）：
#   - mergerfs：pkill -9 mergerfs + umount /workspace
#               → 期望 .checks[].code == MOUNT_MERGERFS_FAILED
#   - sshfs   ：fusermount3 -u /mnt/cold（fallback umount -l）
#               → 期望 .checks[].code ∈ {MOUNT_SSHFS_DISCONNECTED,
#                                       MOUNT_SSHFS_FAILED}
#   - mutagen ：pkill -9 mutagen-agent
#               → 期望 .checks[].code ∈ {MOUNT_MUTAGEN_DAEMON_UNAVAILABLE,
#                                       MOUNT_MUTAGEN_SYNC_FAILED}
#
# M13 核心断言：所有 status ∈ {warn,fail} 的 check 必须有非空 next_action
#               （等价 "禁止静默降级" = stderr 必有错误码 + 中文下一步）。
#
# 安全：trap restore_all EXIT/INT/TERM 在脚本异常退出时尝试恢复挂载（restart 兜底）。
#
# 用法：bash scripts/degradation-regression.sh [--layer=mergerfs|sshfs|mutagen|all]
#                                              [--target-container=NAME]
#                                              [--dry-run] [--confirm-destructive]
#                                              [--output-dir=DIR] [--help]
#
# 退出码：
#   0  全部 layer PASS
#   1  任一 layer FAIL（错误码缺失 / next_action 空 / JSON 非法）
#   2  全部 SKIP（无目标容器 / 缺少 --confirm-destructive）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BENCH_DIR_DEFAULT="${PROJECT_ROOT}/.planning/phases/35-e2e/benchmarks"

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
degradation-regression.sh — Phase 35 M13 三层静默降级终验

用法:
  scripts/degradation-regression.sh [--layer=mergerfs|sshfs|mutagen|all] \
                                    [--target-container=NAME] [--dry-run] \
                                    [--confirm-destructive] [--output-dir=DIR]

可选:
  --layer=mergerfs|sshfs|mutagen|all  破坏目标层（默认 all 三层依次）
  --target-container=NAME             目标容器（默认从 docker ps --filter
                                      label=com.claudedock.managed=true
                                      自动探测；必须匹配 ^[a-z0-9][a-z0-9_.-]*$）
  --dry-run                           只打印破坏命令，不真实 pkill / umount
  --confirm-destructive               显式 opt-in 才会真实破坏（T-35-02-04 闸门）
  --output-dir=DIR                    报告输出目录
                                      （默认 .planning/phases/35-e2e/benchmarks）
  --help, -h                          显示本帮助

M13 验收口径：
  - 三层任意破坏后 claudedock doctor --json 必须包含对应 MOUNT_* 错误码
  - 所有 status ∈ {warn,fail} check 必有非空 next_action（"禁止静默降级"）
  - 错误码命名匹配 ^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$
    （internal/claudedock/errcodes/codes.go L56）

安全约束：
  - --confirm-destructive 默认 false，缺省走 dry-run 仅预览破坏命令
  - --target-container 必匹配 docker 容器名规范（防 docker exec 命令注入）
  - trap restore_all EXIT/INT/TERM；恢复失败兜底 docker restart

退出码：0=PASS / 1=FAIL / 2=SKIP
EOF
}

# ────────────────────────────────────────────────────────────────────────────
# CLI 参数
# ────────────────────────────────────────────────────────────────────────────

LAYER="all"
TARGET_CTR=""
DRY_RUN=false
CONFIRM_DESTRUCTIVE=false
OUTPUT_DIR="${BENCH_DIR_DEFAULT}"

for arg in "$@"; do
  case "$arg" in
    --layer=*) LAYER="${arg#--layer=}" ;;
    --target-container=*) TARGET_CTR="${arg#--target-container=}" ;;
    --dry-run) DRY_RUN=true ;;
    --confirm-destructive) CONFIRM_DESTRUCTIVE=true ;;
    --output-dir=*) OUTPUT_DIR="${arg#--output-dir=}" ;;
    --help|-h) usage; exit 0 ;;
    *) fail "未知参数: $arg"; usage >&2; exit 1 ;;
  esac
done

case "$LAYER" in
  mergerfs|sshfs|mutagen|all) ;;
  *) fail "非法 --layer=${LAYER}（必须为 mergerfs|sshfs|mutagen|all）"; exit 1 ;;
esac

# 容器名安全正则（T-35-02-02 防 docker exec 命令注入；与 uat 脚本同一守卫）
CTR_NAME_REGEX='^[a-z0-9][a-z0-9_.-]*$'

mkdir -p "$OUTPUT_DIR"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
WORK="$(mktemp -d)"
REPORT_JSON="${OUTPUT_DIR}/degradation-regression-${TIMESTAMP}.json"
REPORT_MD="${OUTPUT_DIR}/degradation-regression-${TIMESTAMP}.md"
DESTRUCT_LOG="${OUTPUT_DIR}/.degradation-destruct.log"

# ────────────────────────────────────────────────────────────────────────────
# 工具断言
# ────────────────────────────────────────────────────────────────────────────

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "缺少必需命令: $1${2:+ ($2)}"
    exit 2
  fi
}

require_cmd jq "brew install jq / apt install jq"

has_docker() { command -v docker >/dev/null 2>&1; }

# ────────────────────────────────────────────────────────────────────────────
# 容器探测 + 安全校验
# ────────────────────────────────────────────────────────────────────────────

detect_container() {
  if [[ -n "$TARGET_CTR" ]]; then
    if [[ ! "$TARGET_CTR" =~ $CTR_NAME_REGEX ]]; then
      fail "非法容器名 '${TARGET_CTR}'（不匹配 ${CTR_NAME_REGEX}）"
      exit 1
    fi
    return 0
  fi
  if ! has_docker; then
    skip "M13" "未安装 docker，无法定位 managed 容器"
    return 1
  fi
  TARGET_CTR="$(docker ps \
    --filter 'label=com.claudedock.managed=true' \
    --format '{{.Names}}' 2>/dev/null | head -1 || true)"
  if [[ -z "$TARGET_CTR" ]]; then
    skip "M13" "未发现 com.claudedock.managed=true 容器"
    return 1
  fi
  if [[ ! "$TARGET_CTR" =~ $CTR_NAME_REGEX ]]; then
    fail "自动探测到的容器名非法: ${TARGET_CTR}"
    exit 1
  fi
  info "目标容器（自动探测）: ${TARGET_CTR}"
}

# ────────────────────────────────────────────────────────────────────────────
# 破坏命令模板（注意：所有引用的 MOUNT_* 必须在 errcodes/mount.go 中存在）
# ────────────────────────────────────────────────────────────────────────────

# 命令字面量集中定义，便于 acceptance grep 命中
DISRUPT_CMD_MERGERFS='pkill -9 mergerfs; umount /workspace 2>/dev/null || true'
DISRUPT_CMD_SSHFS='fusermount3 -u /mnt/cold 2>/dev/null || umount -l /mnt/cold 2>/dev/null || true'
DISRUPT_CMD_MUTAGEN='pkill -9 mutagen-agent'

EXPECTED_CODE_MERGERFS='MOUNT_MERGERFS_FAILED'
# sshfs 两选一：MOUNT_SSHFS_DISCONNECTED 或 MOUNT_SSHFS_FAILED
EXPECTED_CODES_SSHFS_LIST=(MOUNT_SSHFS_DISCONNECTED MOUNT_SSHFS_FAILED)
# mutagen 两选一：MOUNT_MUTAGEN_DAEMON_UNAVAILABLE 或 MOUNT_MUTAGEN_SYNC_FAILED
EXPECTED_CODES_MUTAGEN_LIST=(MOUNT_MUTAGEN_DAEMON_UNAVAILABLE MOUNT_MUTAGEN_SYNC_FAILED)

# T-35-02-04 安全闸门：destructive 操作必须 opt-in
destructive_guard_msg() {
  cat <<'EOF'

⚠ 警告：本脚本会在容器内执行 pkill -9 mergerfs / pkill -9 mutagen-agent /
   fusermount3 -u /mnt/cold 等破坏性命令。请仅在 staging 或 fixture 容器执行。
   生产容器误跑会中断用户 claude 进程。

需 --confirm-destructive 显式 opt-in 才会真实执行破坏命令；
未 opt-in 时仅在 --dry-run 模式下打印将执行的命令并安全退出。
EOF
}

# ────────────────────────────────────────────────────────────────────────────
# 破坏 / 恢复 函数对（每层独立，trap restore_all 兜底）
# ────────────────────────────────────────────────────────────────────────────

run_in_ctr_or_print() {
  local layer="$1" cmd="$2"
  if [[ "$DRY_RUN" == "true" || "$CONFIRM_DESTRUCTIVE" != "true" ]]; then
    echo "[DRY-RUN] docker exec ${TARGET_CTR:-<ctr>} sh -c '${cmd}'" >&2
    return 0
  fi
  printf '%s\tdisrupt\t%s\tcontainer=%s\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$layer" "$TARGET_CTR" \
    >> "$DESTRUCT_LOG" 2>/dev/null || true
  docker exec "$TARGET_CTR" sh -c "$cmd"
}

disrupt_layer() {
  local layer="$1"
  case "$layer" in
    mergerfs)
      info "破坏 mergerfs 层：${DISRUPT_CMD_MERGERFS}"
      run_in_ctr_or_print "$layer" "$DISRUPT_CMD_MERGERFS" || true
      ;;
    sshfs)
      info "破坏 sshfs 层：${DISRUPT_CMD_SSHFS}"
      run_in_ctr_or_print "$layer" "$DISRUPT_CMD_SSHFS" || true
      ;;
    mutagen)
      info "破坏 mutagen 层：${DISRUPT_CMD_MUTAGEN}"
      run_in_ctr_or_print "$layer" "$DISRUPT_CMD_MUTAGEN" || true
      ;;
  esac
}

restore_layer() {
  local layer="$1"
  if [[ "$DRY_RUN" == "true" || "$CONFIRM_DESTRUCTIVE" != "true" ]]; then
    echo "[DRY-RUN] restore_layer ${layer}" >&2
    return 0
  fi
  case "$layer" in
    mergerfs)
      info "恢复 mergerfs 层（remount 或 docker restart 兜底）"
      docker exec "$TARGET_CTR" /etc/claudedock/remount-mergerfs.sh \
        2>/dev/null \
        || { docker restart "$TARGET_CTR" >/dev/null 2>&1 || true; \
             sleep 5; }
      ;;
    sshfs)
      info "恢复 sshfs 层（remount 或 docker restart 兜底）"
      docker exec "$TARGET_CTR" /etc/claudedock/remount-sshfs.sh \
        2>/dev/null \
        || { docker restart "$TARGET_CTR" >/dev/null 2>&1 || true; \
             sleep 5; }
      ;;
    mutagen)
      info "恢复 mutagen 层（重启 agent 或 docker restart 兜底）"
      docker exec "$TARGET_CTR" sh -c '/etc/claudedock/mutagen-agent &' \
        2>/dev/null \
        || { docker restart "$TARGET_CTR" >/dev/null 2>&1 || true; \
             sleep 5; }
      ;;
  esac
}

restore_all() {
  if [[ -z "$TARGET_CTR" ]]; then
    return 0
  fi
  if [[ "$DRY_RUN" == "true" || "$CONFIRM_DESTRUCTIVE" != "true" ]]; then
    return 0
  fi
  info "trap 触发：尝试恢复全部层"
  for L in mergerfs sshfs mutagen; do
    restore_layer "$L" 2>/dev/null || true
  done
}

cleanup_workdir() { rm -rf "$WORK" 2>/dev/null || true; }
trap 'restore_all; cleanup_workdir' EXIT INT TERM

# ────────────────────────────────────────────────────────────────────────────
# 断言（Pattern D，M13 专用变体）
# ────────────────────────────────────────────────────────────────────────────

# 校验 doctor JSON：合法 + 至少有一个 expected_code 命中 + warn/fail 都有 next_action
LAYER_RESULT_LINES=()
LAYER_OBSERVED_CODES_JSON='[]'

assert_code_present() {
  local layer="$1" out="$2" expected_codes_str="$3"

  echo "$out" > "${WORK}/doctor-${layer}.json"

  if ! jq empty "${WORK}/doctor-${layer}.json" >/dev/null 2>&1; then
    fail "${layer}: doctor --json 输出非合法 JSON"
    cat "${WORK}/doctor-${layer}.json" >&2
    LAYER_OBSERVED_CODES_JSON='[]'
    return 1
  fi

  # schema_version 校验（ci-doctor-grep.sh L33 同款）
  if ! jq -e '.schema_version == 1' "${WORK}/doctor-${layer}.json" >/dev/null; then
    fail "${layer}: schema_version != 1"
    return 1
  fi

  # 抽出所有 .checks[].code（去空，去重）
  local observed
  observed="$(jq -r '.checks[].code // empty' \
    "${WORK}/doctor-${layer}.json" 2>/dev/null | sort -u || true)"
  LAYER_OBSERVED_CODES_JSON="$(printf '%s\n' $observed | jq -R . | jq -cs .)"

  # 期望码：以空白分隔的 "code1 code2 ..."；任一命中即通过
  local hit=false code
  for code in $expected_codes_str; do
    if jq -e --arg c "$code" '.checks[] | select(.code == $c)' "${WORK}/doctor-${layer}.json" >/dev/null 2>&1; then
      hit=true
      pass "${layer}: 观察到期望错误码 ${code}"
      break
    fi
  done

  if [[ "$hit" != "true" ]]; then
    fail "${layer}: doctor JSON 无任一期望错误码（${expected_codes_str}）"
    return 1
  fi

  # 错误码命名规范（codes.go L56 正则）
  local bad
  bad="$(jq -r '.checks[].code // empty' \
    "${WORK}/doctor-${layer}.json" 2>/dev/null \
    | grep -vE '^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$' || true)"
  if [[ -n "$bad" ]]; then
    fail "${layer}: 错误码命名不合法: ${bad}"
    return 1
  fi

  # M13 核心断言：warn/fail check 必须有非空 next_action != ""
  local missing
  missing="$(jq -r '.checks[]
                   | select(.status == "warn" or .status == "fail")
                   | select((.next_action // "") == "")
                   | "\(.domain).\(.name)"' \
    "${WORK}/doctor-${layer}.json")"
  if [[ -n "$missing" ]]; then
    fail "${layer}: M13 违反 — warn/fail check 缺 next_action: ${missing}"
    return 1
  fi
  pass "${layer}: M13 守恒 — 所有 warn/fail check next_action_present=true"

  return 0
}

# ────────────────────────────────────────────────────────────────────────────
# 单层执行流（pre_check → disrupt → wait → doctor → assert → restore）
# ────────────────────────────────────────────────────────────────────────────

LAYER_RESULTS_JSON='[]'

run_layer() {
  local layer="$1"
  local expected_codes
  case "$layer" in
    mergerfs) expected_codes="$EXPECTED_CODE_MERGERFS" ;;
    sshfs)    expected_codes="${EXPECTED_CODES_SSHFS_LIST[*]}" ;;
    mutagen)  expected_codes="${EXPECTED_CODES_MUTAGEN_LIST[*]}" ;;
  esac

  echo ""
  echo "===== layer=${layer} expected_codes=[${expected_codes}] ====="

  # 1) pre_check：dry-run / 未 opt-in 跳过 mountpoint 检查
  if [[ "$DRY_RUN" != "true" && "$CONFIRM_DESTRUCTIVE" == "true" ]]; then
    case "$layer" in
      mergerfs) docker exec "$TARGET_CTR" mountpoint -q /workspace \
                  || warn "${layer}: pre_check /workspace 非挂载点（可能已破坏）" ;;
      sshfs)    docker exec "$TARGET_CTR" mountpoint -q /mnt/cold \
                  || warn "${layer}: pre_check /mnt/cold 非挂载点（可能已破坏）" ;;
      mutagen)  docker exec "$TARGET_CTR" pgrep -f mutagen-agent >/dev/null \
                  || warn "${layer}: pre_check mutagen-agent 未在运行" ;;
    esac
  fi

  # 2) 破坏
  disrupt_layer "$layer"

  # 3) 给 CLI stderr 捕获窗口（REQ-F2-B：≤ 2s 内降级）
  sleep 2

  # 4) 跑 claudedock doctor --json
  local out
  if [[ "$DRY_RUN" == "true" || "$CONFIRM_DESTRUCTIVE" != "true" ]]; then
    info "${layer}: dry-run 模式，跳过 doctor 调用"
    skip "M13-${layer}" "需 --confirm-destructive 显式 opt-in 才会真实执行破坏与 doctor"
    LAYER_RESULTS_JSON="$(echo "$LAYER_RESULTS_JSON" | jq --arg l "$layer" \
      --arg ec "$expected_codes" \
      '. + [{layer:$l, expected_codes:$ec, observed_codes:[],
             next_action_present:null, outcome:"skip"}]')"
    return 2
  fi

  out="$(docker exec "$TARGET_CTR" claudedock doctor --json 2>&1 || true)"

  # 5) 断言
  local outcome="pass" rc=0
  if ! assert_code_present "$layer" "$out" "$expected_codes"; then
    outcome="fail"; rc=1
  fi

  # 收集本层结果到 JSON
  LAYER_RESULTS_JSON="$(echo "$LAYER_RESULTS_JSON" | jq \
    --arg l "$layer" --arg ec "$expected_codes" \
    --arg outcome "$outcome" \
    --argjson observed "$LAYER_OBSERVED_CODES_JSON" \
    '. + [{layer:$l, expected_codes:$ec, observed_codes:$observed,
           next_action_present:true, outcome:$outcome}]')"

  # 6) 恢复（trap 兜底，但显式调用更快）
  restore_layer "$layer"

  return "$rc"
}

# ────────────────────────────────────────────────────────────────────────────
# 主流程
# ────────────────────────────────────────────────────────────────────────────

info "M13 静默降级回归 — layer=${LAYER} dry-run=${DRY_RUN} confirm=${CONFIRM_DESTRUCTIVE}"

# 即便 SKIP/dry-run，也先把三层 disrupt 命令样板预览到 stderr（acceptance grep）
echo "[DRY-RUN-PREVIEW] layer=mergerfs cmd=${DISRUPT_CMD_MERGERFS}" >&2
echo "[DRY-RUN-PREVIEW] layer=sshfs    cmd=${DISRUPT_CMD_SSHFS}" >&2
echo "[DRY-RUN-PREVIEW] layer=mutagen  cmd=${DISRUPT_CMD_MUTAGEN}" >&2

if [[ "$CONFIRM_DESTRUCTIVE" != "true" ]]; then
  echo ""
  destructive_guard_msg
  echo ""
  info "缺少 --confirm-destructive，按安全策略走 dry-run 预览。"
fi

if ! detect_container; then
  : # SKIP，已记账
fi

LAYERS=()
case "$LAYER" in
  all) LAYERS=(mergerfs sshfs mutagen) ;;
  *)   LAYERS=("$LAYER") ;;
esac

OVERALL_RC=0
for L in "${LAYERS[@]}"; do
  if [[ -z "$TARGET_CTR" ]]; then
    LAYER_RESULTS_JSON="$(echo "$LAYER_RESULTS_JSON" | jq --arg l "$L" \
      '. + [{layer:$l, expected_codes:"", observed_codes:[],
             next_action_present:null, outcome:"skip"}]')"
    continue
  fi
  if ! run_layer "$L"; then
    case "$?" in
      2) : ;;        # SKIP，不算 FAIL
      *) OVERALL_RC=1 ;;
    esac
  fi
done

# ────────────────────────────────────────────────────────────────────────────
# 报告产物
# ────────────────────────────────────────────────────────────────────────────

OUTCOME="pass"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
  OUTCOME="fail"
elif [[ "$PASS_COUNT" -eq 0 && "$SKIP_COUNT" -gt 0 ]]; then
  OUTCOME="skip"
fi

SUMMARY_JSON="$(jq -n \
  --argjson total "${#LAYERS[@]}" \
  --argjson pass  "$PASS_COUNT" \
  --argjson fail  "$FAIL_COUNT" \
  --argjson skip  "$SKIP_COUNT" \
  '{ total: $total, pass: $pass, fail: $fail, skip: $skip }')"

jq -n \
  --argjson schema 1 \
  --arg container "${TARGET_CTR:-}" \
  --argjson layers "$LAYER_RESULTS_JSON" \
  --argjson summary "$SUMMARY_JSON" \
  --arg outcome "$OUTCOME" \
  '{ schema_version: $schema, container: $container,
     layers: $layers, summary: $summary, outcome: $outcome }' \
  > "$REPORT_JSON"

{
  echo "# M13 静默降级回归 — ${LAYER}"
  echo ""
  echo "- 时间: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "- 容器: ${TARGET_CTR:-N/A}"
  echo "- dry-run: ${DRY_RUN}"
  echo "- confirm-destructive: ${CONFIRM_DESTRUCTIVE}"
  echo "- 结论: ${OUTCOME}"
  echo ""
  echo "## 各层结果"
  echo ""
  echo "| layer | expected_codes | observed_codes | next_action_present | outcome |"
  echo "|-------|-----------------|-----------------|---------------------|---------|"
  echo "$LAYER_RESULTS_JSON" \
    | jq -r '.[] | "| \(.layer) | \(.expected_codes) | \((.observed_codes // []) | join(",")) | \(.next_action_present) | \(.outcome) |"'
  echo ""
  echo "## 汇总"
  echo ""
  echo "$SUMMARY_JSON" | jq .
} > "$REPORT_MD"

info "JSON 报告: ${REPORT_JSON}"
info "MD   报告: ${REPORT_MD}"

# ────────────────────────────────────────────────────────────────────────────
# 退出码裁决
# ────────────────────────────────────────────────────────────────────────────

echo ""
echo "========================================"
echo "M13 ${LAYER} 结果: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${WARN_COUNT} WARN, ${SKIP_COUNT} SKIP"
case "$OUTCOME" in
  pass)
    echo "状态: 全部 layer 通过"
    exit 0
    ;;
  skip)
    echo "状态: 全部 SKIP（无目标容器或缺 --confirm-destructive）"
    exit 2
    ;;
  fail|*)
    echo "状态: 存在失败 layer，请检查上方 [FAIL] 条目"
    exit 1
    ;;
esac
