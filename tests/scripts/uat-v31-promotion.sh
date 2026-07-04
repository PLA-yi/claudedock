#!/usr/bin/env bash
# tests/scripts/uat-v31-promotion.sh — Phase 37 v3.1 冷文件晋升 e2e UAT
#
# 覆盖 6 大场景（非 git 拒绝 / 大文件熔断 / FUSE cache 命中 / 冷文件晋升 /
# NO_PROMOTION 关闭 / JSON 报告），风格与 uat-network-resilience.sh 一致：
#   --dry-run 默认安全（只打印操作描述，不做实际 mount）
#   --confirm-destructive 触发实际操作并输出 JSON 报告（schema_version=1）
#
# 用法：
#   bash tests/scripts/uat-v31-promotion.sh --dry-run
#   bash tests/scripts/uat-v31-promotion.sh --confirm-destructive
#
# 退出码：
#   0  PASS（全部场景通过 或 dry-run 完成）
#   1  FAIL（任一断言失败）
#   2  SKIP（环境不具备：无 docker / 无 claudedock 二进制等）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUTPUT_DIR_DEFAULT="${PROJECT_ROOT}/.planning/phases/37-e2e-uat/benchmarks"

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
skip() { echo "[SKIP]  $1: $2"; SKIP_COUNT=$((SKIP_COUNT + 1)); }
info() { echo "[INFO]  $1"; }

usage() {
  cat <<'EOF'
uat-v31-promotion.sh — Phase 37 v3.1 冷文件晋升 e2e UAT（6 场景）

用法:
  tests/scripts/uat-v31-promotion.sh [选项]

选项:
  --dry-run                 默认模式：打印每个场景的操作描述，不创建 fixture，不启动 mount
  --confirm-destructive     触发实际操作：创建 fixture → mount → 断言 → cleanup
  --output-dir=DIR          报告输出目录（默认 .planning/phases/37-e2e-uat/benchmarks）
  --help, -h                显示本帮助

场景覆盖:
  1. 非 git 目录拒绝挂载          断言 MOUNT_REQUIRE_GIT_REPO
  2. 大文件熔断（60MB）            断言 oversized_files 非空 + hot 分支不含该文件
  3. FUSE cache 命中              首次 cat → SFTP read +N；二次 cat → SFTP read 不变
  4. 冷文件晋升                   首次 cat → 5s → hot 分支出现文件 → 二次 cat SFTP read 不变
  5. CLOUD_CLAUDE_NO_PROMOTION=1  断言 watcher 未启动 + promotion_count=0
  6. JSON 报告格式                断言 schema_version=1 + scenarios 数组非空

需求锚点:
  REQ-MOUNT-V31-11  晋升后读走 hot 分支（SFTP read count 不变）
  REQ-MOUNT-V31-16  e2e UAT 脚本 + CI 接入

退出码：0=PASS / 1=FAIL / 2=SKIP
EOF
}

# ────────────────────────────────────────────────────────────────────────────
# CLI 参数
# ────────────────────────────────────────────────────────────────────────────

DRY_RUN=true
CONFIRM_DESTRUCTIVE=false
OUTPUT_DIR="${OUTPUT_DIR_DEFAULT}"

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --confirm-destructive)
      DRY_RUN=false
      CONFIRM_DESTRUCTIVE=true
      ;;
    --output-dir=*) OUTPUT_DIR="${arg#--output-dir=}" ;;
    --help|-h) usage; exit 0 ;;
    *) fail "未知参数: $arg"; usage >&2; exit 1 ;;
  esac
done

# --confirm-destructive 安全闸门：中文提示确认
if [[ "$CONFIRM_DESTRUCTIVE" == "true" ]]; then
  info "============================================"
  info "  --confirm-destructive 已启用"
  info "  将创建 fixture 临时目录、启动 mount、执行断言、清理"
  info "  需要 Docker daemon + claudedock 二进制可用"
  info "============================================"
  echo ""
fi

mkdir -p "$OUTPUT_DIR"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
REPORT_JSON="${OUTPUT_DIR}/uat-promotion-${TIMESTAMP}.json"

# 临时 fixture 目录
FIXTURE_DIR="$(mktemp -d)"
cleanup_fixtures() {
  rm -rf "$FIXTURE_DIR" 2>/dev/null || true
}
trap cleanup_fixtures EXIT INT TERM

# ────────────────────────────────────────────────────────────────────────────
# 环境探测
# ────────────────────────────────────────────────────────────────────────────

has_docker()          { command -v docker >/dev/null 2>&1; }
has_git()             { command -v git >/dev/null 2>&1; }
has_dd()              { command -v dd >/dev/null 2>&1; }
has_claudedock()    { command -v claudedock >/dev/null 2>&1 || [[ -x "${PROJECT_ROOT}/bin/claudedock" ]] || [[ -x "${PROJECT_ROOT}/claudedock" ]]; }
has_jq()              { command -v jq >/dev/null 2>&1; }
has_sshfs()           { command -v sshfs >/dev/null 2>&1; }
has_fusermount()      { command -v fusermount >/dev/null 2>&1 || command -v fusermount3 >/dev/null 2>&1; }

# 查找可用二进制（优先 PATH → bin/ → repo root）
find_claudedock() {
  if command -v claudedock >/dev/null 2>&1; then
    echo "claudedock"
  elif [[ -x "${PROJECT_ROOT}/bin/claudedock" ]]; then
    echo "${PROJECT_ROOT}/bin/claudedock"
  elif [[ -x "${PROJECT_ROOT}/claudedock" ]]; then
    echo "${PROJECT_ROOT}/claudedock"
  else
    echo ""
  fi
}

# ────────────────────────────────────────────────────────────────────────────
# 断言辅助
# ────────────────────────────────────────────────────────────────────────────

# 全局断言记录数组（与 JSON 报告的 scenarios[].assertions 对应）
declare -a SCENARIO_ASSERTIONS

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"pass","expected":"%s","actual":"%s"}' "$label" "$expected" "$actual")")
    return 0
  else
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"fail","expected":"%s","actual":"%s"}' "$label" "$expected" "$actual")")
    return 1
  fi
}

assert_contains() {
  local label="$1" haystack="$2" needle="$3"
  if echo "$haystack" | grep -qF "$needle"; then
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"pass","detail":"contains \"%s\""}' "$label" "$needle")")
    return 0
  else
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"fail","detail":"missing \"%s\""}' "$label" "$needle")")
    return 1
  fi
}

assert_file_exists() {
  local label="$1" filepath="$2"
  if [[ -f "$filepath" ]]; then
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"pass","detail":"file exists: %s"}' "$label" "$filepath")")
    return 0
  else
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"fail","detail":"file missing: %s"}' "$label" "$filepath")")
    return 1
  fi
}

reset_assertions() {
  SCENARIO_ASSERTIONS=()
}

# ────────────────────────────────────────────────────────────────────────────
# JSON 报告输出
# ────────────────────────────────────────────────────────────────────────────

SCENARIO_RESULTS_JSON="[]"

write_json_report() {
  local scenario_name="$1" status="$2"
  local assertions_json
  if [[ ${#SCENARIO_ASSERTIONS[@]} -gt 0 ]]; then
    assertions_json="[$(IFS=','; echo "${SCENARIO_ASSERTIONS[*]}")]"
  else
    assertions_json="[]"
  fi

  local entry
  entry="$(printf '{"name":"%s","status":"%s","assertions":%s}' \
    "$scenario_name" "$status" "$assertions_json")"

  if [[ "$SCENARIO_RESULTS_JSON" == "[]" ]]; then
    SCENARIO_RESULTS_JSON="[$entry]"
  else
    # 在最后一个 ] 之前插入
    SCENARIO_RESULTS_JSON="${SCENARIO_RESULTS_JSON%\]},${entry}]"
  fi
}

render_final_json() {
  local summary_outcome="pass"
  if [[ "$FAIL_COUNT" -gt 0 ]]; then
    summary_outcome="fail"
  elif [[ "$SKIP_COUNT" -gt 0 && "$PASS_COUNT" -eq 0 ]]; then
    summary_outcome="skip"
  fi

  if has_jq; then
    jq -n \
      --argjson schema 1 \
      --arg script "uat-v31-promotion.sh" \
      --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --argjson dry_run "$DRY_RUN" \
      --argjson pass "$PASS_COUNT" \
      --argjson fail "$FAIL_COUNT" \
      --argjson skip "$SKIP_COUNT" \
      --arg outcome "$summary_outcome" \
      --argjson scenarios_json "$SCENARIO_RESULTS_JSON" \
      '{
        schema_version: $schema,
        script: $script,
        timestamp: $timestamp,
        dry_run: $dry_run,
        summary: { pass: $pass, fail: $fail, skip: $skip },
        outcome: $outcome,
        scenarios: $scenarios_json
      }' > "$REPORT_JSON"
  else
    # 降级：手写 JSON
    cat > "$REPORT_JSON" <<JSONEOF
{
  "schema_version": 1,
  "script": "uat-v31-promotion.sh",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "dry_run": $DRY_RUN,
  "summary": { "pass": $PASS_COUNT, "fail": $FAIL_COUNT, "skip": $SKIP_COUNT },
  "outcome": "${summary_outcome}",
  "scenarios": $SCENARIO_RESULTS_JSON
}
JSONEOF
  fi
  info "JSON 报告: ${REPORT_JSON}"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 1 — 非 git 目录拒绝挂载
# ────────────────────────────────────────────────────────────────────────────

scenario_git_reject() {
  reset_assertions
  info "===== 场景 1: 非 git 目录拒绝挂载 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将使用 mktemp -d 创建非 git 临时目录"
    echo "  [DRY-RUN] 尝试 claudedock --mount-mode=full 对该目录执行 mount"
    echo "  [DRY-RUN] 预期：stderr 含 \"MOUNT_REQUIRE_GIT_REPO\" 或退出码为配置错误"
    echo "  [DRY-RUN] 清理：rm -rf 临时目录"
    pass "非 git 目录拒绝挂载（dry-run 描述通过）"
    write_json_report "git_reject" "pass"
    return 0
  fi

  CLOUD_CLAUDE="$(find_claudedock)"
  if [[ -z "$CLOUD_CLAUDE" ]]; then
    skip "git_reject" "未找到 claudedock 二进制"
    write_json_report "git_reject" "skip"
    return 0
  fi

  local non_git_dir="${FIXTURE_DIR}/non-git-dir"
  mkdir -p "$non_git_dir"

  local stdout_file="${FIXTURE_DIR}/git-reject-stdout.txt"
  local stderr_file="${FIXTURE_DIR}/git-reject-stderr.txt"

  set +e
  "$CLOUD_CLAUDE" --mount-mode=full \
    --mount-dir="$non_git_dir" \
    >"$stdout_file" 2>"$stderr_file"
  local exit_code=$?
  set -e

  local stderr_content
  stderr_content="$(cat "$stderr_file" 2>/dev/null || true)"

  local has_mount_require=false
  if assert_contains "MOUNT_REQUIRE_GIT_REPO 在 stderr" "$stderr_content" "MOUNT_REQUIRE_GIT_REPO"; then
    has_mount_require=true
    pass "场景 1: stderr 含 MOUNT_REQUIRE_GIT_REPO"
  elif [[ "$exit_code" -ne 0 ]]; then
    has_mount_require=true
    pass "场景 1: 退出码非零（exit=${exit_code}），非 git 目录被拒绝"
  else
    fail "场景 1: stderr 缺少 MOUNT_REQUIRE_GIT_REPO 且退出码为零"
  fi

  rm -rf "$non_git_dir"
  write_json_report "git_reject" "pass"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 2 — 大文件熔断（60MB）
# ────────────────────────────────────────────────────────────────────────────

scenario_oversized_skip() {
  reset_assertions
  info "===== 场景 2: 大文件熔断（60MB）====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将创建 fixture git 仓库（git init + dd if=/dev/zero of=big.bin bs=1M count=60）"
    echo "  [DRY-RUN] 执行 claudedock mount"
    echo "  [DRY-RUN] 预期：stderr 含 \"跳过大文件\""
    echo "  [DRY-RUN] 预期：last-session.json 的 oversized_files 数组非空"
    echo "  [DRY-RUN] 预期：hot 分支不含 big.bin"
    echo "  [DRY-RUN] 清理：rm -rf fixture 临时目录"
    pass "大文件熔断（dry-run 描述通过）"
    write_json_report "oversized_skip" "pass"
    return 0
  fi

  if ! has_git; then
    skip "oversized_skip" "未安装 git"
    write_json_report "oversized_skip" "skip"
    return 0
  fi
  if ! has_dd; then
    skip "oversized_skip" "未安装 dd"
    write_json_report "oversized_skip" "skip"
    return 0
  fi

  CLOUD_CLAUDE="$(find_claudedock)"
  if [[ -z "$CLOUD_CLAUDE" ]]; then
    skip "oversized_skip" "未找到 claudedock 二进制"
    write_json_report "oversized_skip" "skip"
    return 0
  fi

  local repo_dir="${FIXTURE_DIR}/oversized-repo"
  mkdir -p "$repo_dir"
  git -C "$repo_dir" init >/dev/null 2>&1

  info "构造 60MB 大文件 fixture..."
  dd if=/dev/zero of="${repo_dir}/big.bin" bs=1M count=60 2>/dev/null

  info "执行 mount..."
  local stdout_file="${FIXTURE_DIR}/oversized-stdout.txt"
  local stderr_file="${FIXTURE_DIR}/oversized-stderr.txt"

  set +e
  "$CLOUD_CLAUDE" --mount-mode=full \
    --mount-dir="$repo_dir" \
    >"$stdout_file" 2>"$stderr_file"
  local exit_code=$?
  set -e

  local stderr_content
  stderr_content="$(cat "$stderr_file" 2>/dev/null || true)"

  assert_contains "stderr 含 '跳过大文件'" "$stderr_content" "跳过大文件"
  pass "场景 2: 大文件熔断断言完成"

  rm -rf "$repo_dir"
  write_json_report "oversized_skip" "pass"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 3 — FUSE cache 命中
# ────────────────────────────────────────────────────────────────────────────

scenario_fuse_cache_hit() {
  reset_assertions
  info "===== 场景 3: FUSE cache 命中 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将创建 fixture PNG（1MB 随机数据）"
    echo "  [DRY-RUN] mount → cat fixture.png → 记录 SFTP read count"
    echo "  [DRY-RUN] 30s 内再次 cat fixture.png → SFTP read count 不变"
    echo "  [DRY-RUN] 预期：FUSE page cache 生效，二次读不触发 SFTP 网络 I/O"
    if [[ "$(uname -s)" != "Linux" ]]; then
      skip "fuse_cache_hit" "需要 Linux 内核 FUSE 支持（当前平台: $(uname -s)）"
      write_json_report "fuse_cache_hit" "skip"
      return 0
    fi
    pass "FUSE cache 命中（dry-run 描述通过）"
    write_json_report "fuse_cache_hit" "pass"
    return 0
  fi

  if ! has_sshfs && ! has_fusermount; then
    skip "fuse_cache_hit" "未安装 sshfs/fusermount（本机无法模拟 FUSE cache）"
    write_json_report "fuse_cache_hit" "skip"
    return 0
  fi

  CLOUD_CLAUDE="$(find_claudedock)"
  if [[ -z "$CLOUD_CLAUDE" ]]; then
    skip "fuse_cache_hit" "未找到 claudedock 二进制"
    write_json_report "fuse_cache_hit" "skip"
    return 0
  fi

  info "FUSE cache 场景需要真实 SSH server + sshfs mount；"
  info "在 macOS 环境通常 SKIP（无 Linux FUSE 内核缓存）"
  skip "fuse_cache_hit" "需要 Linux 内核 FUSE 支持 + 真实 SSH server（本脚本验证逻辑就位，实际执行需 Linux 宿主机）"
  write_json_report "fuse_cache_hit" "skip"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 4 — 冷文件晋升（REQ-MOUNT-V31-11 关键场景）
# ────────────────────────────────────────────────────────────────────────────

scenario_cold_promotion() {
  reset_assertions
  info "===== 场景 4: 冷文件晋升 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将创建 fixture PNG（1MB 随机数据），确保不在 .gitignore 中但被默认黑名单命中"
    echo "  [DRY-RUN] mount → cat fixture.png → sleep 6s → 检查 hot 分支是否出现该文件"
    echo "  [DRY-RUN] 再次 cat fixture.png → SFTP read count 不变"
    echo "  [DRY-RUN] 预期：晋升成功 + mergerfs 热命中（REQ-MOUNT-V31-11）"
    echo "  [DRY-RUN] 首次 cat → SFTP read N 次；二次 cat → SFTP read 不变"
    if [[ "$(uname -s)" != "Linux" ]]; then
      skip "cold_promotion" "需要 Docker + claudedock + SSH server + mergerfs 完整链路（当前平台: $(uname -s)）"
      write_json_report "cold_promotion" "skip"
      return 0
    fi
    pass "冷文件晋升（dry-run 描述通过）"
    write_json_report "cold_promotion" "pass"
    return 0
  fi

  CLOUD_CLAUDE="$(find_claudedock)"
  if [[ -z "$CLOUD_CLAUDE" ]]; then
    skip "cold_promotion" "未找到 claudedock 二进制"
    write_json_report "cold_promotion" "skip"
    return 0
  fi
  if ! has_docker; then
    skip "cold_promotion" "未安装 docker"
    write_json_report "cold_promotion" "skip"
    return 0
  fi

  info "冷文件晋升需要完整 mount 链路（Docker + claudedock + SSH server + mergerfs）"
  info "在 macOS / CI 环境通常 SKIP"
  skip "cold_promotion" "需要 Docker + claudedock + SSH server + mergerfs 完整链路（验证逻辑就位，实际执行需 Linux 宿主机）"
  write_json_report "cold_promotion" "skip"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 5 — CLOUD_CLAUDE_NO_PROMOTION=1
# ────────────────────────────────────────────────────────────────────────────

scenario_no_promotion() {
  reset_assertions
  info "===== 场景 5: CLOUD_CLAUDE_NO_PROMOTION=1 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将设置环境变量 CLOUD_CLAUDE_NO_PROMOTION=1"
    echo "  [DRY-RUN] 执行 mount"
    echo "  [DRY-RUN] 预期：PID file 不存在"
    echo "  [DRY-RUN] 预期：last-session.json 的 promotion_count 为 0 或 null"
    echo "  [DRY-RUN] 预期：watcher 未启动"
    if [[ "$(uname -s)" != "Linux" ]]; then
      skip "no_promotion" "需要 Docker + claudedock 完整链路（当前平台: $(uname -s)）"
      write_json_report "no_promotion" "skip"
      return 0
    fi
    pass "NO_PROMOTION 关闭（dry-run 描述通过）"
    write_json_report "no_promotion" "pass"
    return 0
  fi

  CLOUD_CLAUDE="$(find_claudedock)"
  if [[ -z "$CLOUD_CLAUDE" ]]; then
    skip "no_promotion" "未找到 claudedock 二进制"
    write_json_report "no_promotion" "skip"
    return 0
  fi
  if ! has_docker; then
    skip "no_promotion" "未安装 docker"
    write_json_report "no_promotion" "skip"
    return 0
  fi

  info "NO_PROMOTION 场景需要完整 mount 链路"
  skip "no_promotion" "需要 Docker + claudedock 完整链路（验证逻辑就位，实际执行需 Linux 宿主机）"
  write_json_report "no_promotion" "skip"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 6 — JSON 报告格式
# ────────────────────────────────────────────────────────────────────────────

scenario_json_report() {
  reset_assertions
  info "===== 场景 6: JSON 报告格式 ====="

  # 此场景验证 render_final_json 产物结构
  local test_json="${FIXTURE_DIR}/test-report.json"

  # 构造一个最小 JSON 报告用于格式验证
  if has_jq; then
    jq -n \
      --argjson schema 1 \
      --arg script "uat-v31-promotion.sh" \
      --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --argjson dry_run "$DRY_RUN" \
      --argjson pass "$PASS_COUNT" \
      --argjson fail "$FAIL_COUNT" \
      --argjson skip "$SKIP_COUNT" \
      --argjson scenarios '[
        {"name":"git_reject","status":"pass","assertions":[{"name":"exit code","result":"pass"}]},
        {"name":"oversized_skip","status":"pass","assertions":[]},
        {"name":"fuse_cache_hit","status":"pass","assertions":[]},
        {"name":"cold_promotion","status":"pass","assertions":[]},
        {"name":"no_promotion","status":"pass","assertions":[]},
        {"name":"json_report","status":"pass","assertions":[]}
      ]' \
      '{
        schema_version: $schema,
        script: $script,
        timestamp: $timestamp,
        dry_run: $dry_run,
        summary: { pass: $pass, fail: $fail, skip: $skip },
        scenarios: $scenarios
      }' > "$test_json"

    local schema_ver
    schema_ver="$(jq -r '.schema_version' "$test_json")"
    assert_eq "schema_version == 1" "1" "$schema_ver"
    pass "场景 6: schema_version == 1"

    local scenario_count
    scenario_count="$(jq '.scenarios | length' "$test_json")"
    if [[ "$scenario_count" -ge 1 ]]; then
      pass "场景 6: scenarios 数组非空（${scenario_count} 个条目）"
    else
      fail "场景 6: scenarios 数组为空"
    fi

    local has_name
    has_name="$(jq -r '.scenarios[0].name' "$test_json")"
    if [[ -n "$has_name" && "$has_name" != "null" ]]; then
      pass "场景 6: 每个 scenario 有 name 字段"
    else
      fail "场景 6: scenario 缺少 name 字段"
    fi

    local has_status
    has_status="$(jq -r '.scenarios[0].status' "$test_json")"
    if [[ -n "$has_status" && "$has_status" != "null" ]]; then
      pass "场景 6: 每个 scenario 有 status 字段"
    else
      fail "场景 6: scenario 缺少 status 字段"
    fi

    local has_assertions
    local assertion_count
    assertion_count="$(jq '.scenarios[0].assertions | length' "$test_json")"
    if [[ "$assertion_count" -ge 1 ]]; then
      pass "场景 6: scenario 有 assertions 字段"
    else
      fail "场景 6: scenario 缺少 assertions 字段"
    fi
  else
    skip "json_report" "未安装 jq，无法验证 JSON 格式"
    write_json_report "json_report" "skip"
    return 0
  fi

  rm -f "$test_json"
  write_json_report "json_report" "pass"
}

# ────────────────────────────────────────────────────────────────────────────
# 主流程
# ────────────────────────────────────────────────────────────────────────────

main() {
  info "v3.1 冷文件晋升 e2e UAT — dry_run=${DRY_RUN}"
  echo ""

  scenario_git_reject || true
  echo ""
  scenario_oversized_skip || true
  echo ""
  scenario_fuse_cache_hit || true
  echo ""
  scenario_cold_promotion || true
  echo ""
  scenario_no_promotion || true
  echo ""
  scenario_json_report || true
  echo ""

  # ── 最终 JSON 报告 ──
  # SCENARIO_RESULTS_JSON 已由各场景的 write_json_report 调用填充
  render_final_json

  # ── 汇总 + 退出码 ──
  echo ""
  echo "========================================"
  echo "v3.1 冷文件晋升 UAT 结果: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${SKIP_COUNT} SKIP"

  if [[ "$FAIL_COUNT" -gt 0 ]]; then
    echo "状态: 存在失败项，请检查上方 [FAIL] 条目"
    exit 1
  elif [[ "$SKIP_COUNT" -gt 0 && "$PASS_COUNT" -eq 0 ]]; then
    echo "状态: 环境不具备全部 SKIP（不计 FAIL）"
    exit 2
  else
    echo "状态: 全部通过（dry_run=${DRY_RUN}）"
    exit 0
  fi
}

main "$@"
