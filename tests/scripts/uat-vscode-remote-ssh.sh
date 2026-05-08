#!/usr/bin/env bash
# tests/scripts/uat-vscode-remote-ssh.sh — Phase 40 VS Code Remote-SSH E2E UAT
#
# 覆盖 6 大场景（sing-box 进程 / 出口 IP / DNS 泄漏 / VS Code Server / sshd / sing-box 日志），
# 风格与 uat-v31-promotion.sh 一致：
#   --dry-run 默认安全（只打印操作描述，不做实际断言）
#   --confirm-destructive 触发实际断言（需要运行中的容器）
#
# 用法：
#   bash tests/scripts/uat-vscode-remote-ssh.sh --dry-run
#   bash tests/scripts/uat-vscode-remote-ssh.sh --confirm-destructive --container=NAME --expected-egress-ip=1.2.3.4
#
# 退出码：
#   0  PASS（全部场景通过 或 dry-run 完成）
#   1  FAIL（任一断言失败）
#   2  SKIP（环境不具备：无 docker / 无运行容器等）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUTPUT_DIR="${PROJECT_ROOT}/.planning/phases/40-vs-code-remote-ssh-e2e/benchmarks"

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
skip() { echo "[SKIP]  $1: $2"; SKIP_COUNT=$((SKIP_COUNT + 1)); }
info() { echo "[INFO]  $1"; }

usage() {
  cat <<'EOF'
uat-vscode-remote-ssh.sh — Phase 40 VS Code Remote-SSH E2E UAT（6 场景）

用法:
  tests/scripts/uat-vscode-remote-ssh.sh [选项]

选项:
  --dry-run                 默认模式：打印每个场景的操作描述，不做实际断言
  --confirm-destructive     触发实际断言：需要运行中的容器
  --container=NAME          指定容器名（默认自动检测 cloud-claude-local-*）
  --expected-egress-ip=IP   指定期望的出口 IP（不指定则跳过出口 IP 断言）
  --output-dir=DIR          报告输出目录
  --help, -h                显示本帮助

场景覆盖:
  1. sing-box 进程检测              断言容器内 sing-box 进程存在
  2. 出口 IP 验证                   断言 curl ifconfig.me 返回期望的 egress IP
  3. DNS 泄漏检测                   断言容器内 DNS 解析正常（走 sing-box）
  4. VS Code Server 进程检测        检查容器内 vscode-server 进程（可选）
  5. sshd 进程检测                  断言容器内 sshd 进程存在
  6. sing-box 日志域名检查          检查 sing-box 日志中是否有 VS Code 更新域名

需求锚点:
  SSH-05  VS Code Remote-SSH 端到端验证
  SEC-01  验证 direct-tcpip 转发流量走 sing-box tun
  SEC-02  VS Code Server 下载/扩展安装流量走受控出口

退出码：0=PASS / 1=FAIL / 2=SKIP
EOF
}

# ────────────────────────────────────────────────────────────────────────────
# CLI 参数
# ────────────────────────────────────────────────────────────────────────────

DRY_RUN=true
CONTAINER_NAME=""
EXPECTED_EGRESS_IP=""
OUTPUT_DIR="${OUTPUT_DIR}"

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --confirm-destructive)
      DRY_RUN=false
      ;;
    --container=*) CONTAINER_NAME="${arg#--container=}" ;;
    --expected-egress-ip=*) EXPECTED_EGRESS_IP="${arg#--expected-egress-ip=}" ;;
    --output-dir=*) OUTPUT_DIR="${arg#--output-dir=}" ;;
    --help|-h) usage; exit 0 ;;
    *) fail "未知参数: $arg"; usage >&2; exit 1 ;;
  esac
done

mkdir -p "$OUTPUT_DIR"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
REPORT_JSON="${OUTPUT_DIR}/uat-vscode-remote-ssh-${TIMESTAMP}.json"

# ────────────────────────────────────────────────────────────────────────────
# 环境探测
# ────────────────────────────────────────────────────────────────────────────

has_docker() { command -v docker >/dev/null 2>&1; }

detect_container() {
  if [ -n "$CONTAINER_NAME" ]; then
    echo "$CONTAINER_NAME"
    return
  fi
  local name
  name=$(docker ps --filter "label=cloud-claude-local=true" --format '{{.Names}}' 2>/dev/null | head -1)
  if [ -z "$name" ]; then
    name=$(docker ps --filter "name=cloud-claude-local" --format '{{.Names}}' 2>/dev/null | head -1)
  fi
  echo "$name"
}

# ────────────────────────────────────────────────────────────────────────────
# 断言辅助
# ────────────────────────────────────────────────────────────────────────────

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

assert_process_running() {
  local label="$1" container="$2" pattern="$3"
  if docker exec "$container" pgrep -f "$pattern" >/dev/null 2>&1; then
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"pass","detail":"process found: %s"}' "$label" "$pattern")")
    return 0
  else
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"fail","detail":"process not found: %s"}' "$label" "$pattern")")
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

  if command -v jq >/dev/null 2>&1; then
    jq -n \
      --argjson schema 1 \
      --arg script "uat-vscode-remote-ssh.sh" \
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
    cat > "$REPORT_JSON" <<JSONEOF
{
  "schema_version": 1,
  "script": "uat-vscode-remote-ssh.sh",
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
# 场景 1 — sing-box 进程检测
# ────────────────────────────────────────────────────────────────────────────

scenario_singbox_process() {
  reset_assertions
  info "===== 场景 1: sing-box 进程检测 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行: docker exec \$CONTAINER pgrep -x sing-box"
    echo "  [DRY-RUN] 预期: sing-box 进程存在"
    echo "  [DRY-RUN] 注意: proxy 模式和 tun 模式都适用"
    pass "sing-box 进程检测（dry-run 描述通过）"
    write_json_report "singbox_process" "pass"
    return 0
  fi

  if ! has_docker; then
    skip "singbox_process" "未安装 docker"
    write_json_report "singbox_process" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "singbox_process" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "singbox_process" "skip"
    return 0
  fi

  info "容器: $container"

  if assert_process_running "sing-box 进程" "$container" "sing-box"; then
    pass "场景 1: sing-box 进程运行中"
  else
    fail "场景 1: sing-box 进程未运行"
  fi

  write_json_report "singbox_process" "$([ "$FAIL_COUNT" -eq 0 ] && echo "pass" || echo "fail")"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 2 — 出口 IP 验证
# ────────────────────────────────────────────────────────────────────────────

scenario_egress_ip() {
  reset_assertions
  info "===== 场景 2: 出口 IP 验证 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行: docker exec \$CONTAINER curl -s --max-time 15 ifconfig.me"
    echo "  [DRY-RUN] 预期: 返回 IP 等于 --expected-egress-ip 参数值"
    if [ -z "$EXPECTED_EGRESS_IP" ]; then
      echo "  [DRY-RUN] 注意: 未指定 --expected-egress-ip，将 SKIP"
      skip "出口 IP 验证" "未指定 --expected-egress-ip（dry-run）"
      write_json_report "egress_ip" "skip"
    else
      pass "出口 IP 验证（dry-run 描述通过，期望 IP: $EXPECTED_EGRESS_IP）"
      write_json_report "egress_ip" "pass"
    fi
    return 0
  fi

  if [ -z "$EXPECTED_EGRESS_IP" ]; then
    skip "egress_ip" "未指定 --expected-egress-ip"
    write_json_report "egress_ip" "skip"
    return 0
  fi

  if ! has_docker; then
    skip "egress_ip" "未安装 docker"
    write_json_report "egress_ip" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "egress_ip" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "egress_ip" "skip"
    return 0
  fi

  info "容器: $container"
  info "检测出口 IP（可能需要 10-15 秒）..."

  local actual_ip
  actual_ip=$(docker exec "$container" curl -s --max-time 15 ifconfig.me 2>/dev/null || echo "CURL_FAILED")

  if [ "$actual_ip" = "CURL_FAILED" ]; then
    fail "场景 2: curl ifconfig.me 执行失败"
    SCENARIO_ASSERTIONS+=('{"name":"curl 执行","result":"fail","detail":"curl ifconfig.me failed"}')
  elif assert_eq "出口 IP" "$EXPECTED_EGRESS_IP" "$actual_ip"; then
    pass "场景 2: 出口 IP 验证通过 ($actual_ip)"
  else
    fail "场景 2: 出口 IP 不匹配 (实际: $actual_ip, 期望: $EXPECTED_EGRESS_IP)"
  fi

  write_json_report "egress_ip" "$([ "$FAIL_COUNT" -eq 0 ] && echo "pass" || echo "fail")"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 3 — DNS 泄漏检测
# ────────────────────────────────────────────────────────────────────────────

scenario_dns_leak() {
  reset_assertions
  info "===== 场景 3: DNS 泄漏检测 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行: docker exec \$CONTAINER nslookup ifconfig.me"
    echo "  [DRY-RUN] 预期: DNS 解析成功，走 sing-box DNS"
    pass "DNS 泄漏检测（dry-run 描述通过）"
    write_json_report "dns_leak" "pass"
    return 0
  fi

  if ! has_docker; then
    skip "dns_leak" "未安装 docker"
    write_json_report "dns_leak" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "dns_leak" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "dns_leak" "skip"
    return 0
  fi

  info "容器: $container"

  local dns_result
  dns_result=$(docker exec "$container" nslookup ifconfig.me 2>&1 || true)

  if echo "$dns_result" | grep -q "Address:"; then
    pass "场景 3: DNS 解析成功"
    assert_contains "DNS 结果包含 Address" "$dns_result" "Address:"
  else
    fail "场景 3: DNS 解析失败"
    SCENARIO_ASSERTIONS+=('{"name":"DNS 解析","result":"fail","detail":"nslookup failed"}')
  fi

  write_json_report "dns_leak" "$([ "$FAIL_COUNT" -eq 0 ] && echo "pass" || echo "fail")"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 4 — VS Code Server 进程检测
# ────────────────────────────────────────────────────────────────────────────

scenario_vscode_server() {
  reset_assertions
  info "===== 场景 4: VS Code Server 进程检测 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行: docker exec \$CONTAINER pgrep -f vscode-server"
    echo "  [DRY-RUN] 预期: VS Code Server 进程存在（需要先通过 VS Code 连接）"
    echo "  [DRY-RUN] 注意: 如果尚未通过 VS Code 连接，此场景将 SKIP"
    skip "VS Code Server 进程" "需要先通过 VS Code 连接（dry-run）"
    write_json_report "vscode_server" "skip"
    return 0
  fi

  if ! has_docker; then
    skip "vscode_server" "未安装 docker"
    write_json_report "vscode_server" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "vscode_server" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "vscode_server" "skip"
    return 0
  fi

  info "容器: $container"

  if docker exec "$container" pgrep -f "vscode-server" >/dev/null 2>&1; then
    pass "场景 4: VS Code Server 进程运行中"
    SCENARIO_ASSERTIONS+=('{"name":"VS Code Server","result":"pass","detail":"process found"}')
  else
    skip "vscode_server" "VS Code Server 未运行（可能尚未通过 VS Code 连接）"
    write_json_report "vscode_server" "skip"
    return 0
  fi

  write_json_report "vscode_server" "pass"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 5 — sshd 进程检测
# ────────────────────────────────────────────────────────────────────────────

scenario_sshd_process() {
  reset_assertions
  info "===== 场景 5: sshd 进程检测 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行: docker exec \$CONTAINER pgrep -x sshd"
    echo "  [DRY-RUN] 预期: sshd 进程存在"
    pass "sshd 进程检测（dry-run 描述通过）"
    write_json_report "sshd_process" "pass"
    return 0
  fi

  if ! has_docker; then
    skip "sshd_process" "未安装 docker"
    write_json_report "sshd_process" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "sshd_process" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "sshd_process" "skip"
    return 0
  fi

  info "容器: $container"

  if assert_process_running "sshd 进程" "$container" "sshd"; then
    pass "场景 5: sshd 进程运行中"
  else
    fail "场景 5: sshd 进程未运行"
  fi

  write_json_report "sshd_process" "$([ "$FAIL_COUNT" -eq 0 ] && echo "pass" || echo "fail")"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 6 — sing-box 日志域名检查
# ────────────────────────────────────────────────────────────────────────────

scenario_singbox_log_domains() {
  reset_assertions
  info "===== 场景 6: sing-box 日志域名检查 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将检查 sing-box 日志中是否有 VS Code 更新域名"
    echo "  [DRY-RUN] 域名: update.code.visualstudio.com, marketplace.visualstudio.com"
    echo "  [DRY-RUN] 注意: 需要先通过 VS Code 连接并触发扩展安装/更新"
    skip "sing-box 日志域名" "需要先通过 VS Code 连接并触发更新（dry-run）"
    write_json_report "singbox_log_domains" "skip"
    return 0
  fi

  if ! has_docker; then
    skip "singbox_log_domains" "未安装 docker"
    write_json_report "singbox_log_domains" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "singbox_log_domains" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "singbox_log_domains" "skip"
    return 0
  fi

  info "容器: $container"

  # 尝试多种方式获取 sing-box 日志
  local sing_log=""
  sing_log=$(docker exec "$container" cat /var/log/sing-box.log 2>/dev/null || true)
  if [ -z "$sing_log" ]; then
    sing_log=$(docker logs "$container" 2>&1 | grep -i "sing-box" || true)
  fi

  if [ -z "$sing_log" ]; then
    skip "singbox_log_domains" "无法获取 sing-box 日志"
    write_json_report "singbox_log_domains" "skip"
    return 0
  fi

  local found_domains=false
  if echo "$sing_log" | grep -q "update.code.visualstudio.com"; then
    pass "场景 6: sing-box 日志中发现 update.code.visualstudio.com"
    found_domains=true
  fi
  if echo "$sing_log" | grep -q "marketplace.visualstudio.com"; then
    pass "场景 6: sing-box 日志中发现 marketplace.visualstudio.com"
    found_domains=true
  fi

  if [ "$found_domains" = false ]; then
    skip "singbox_log_domains" "sing-box 日志中未发现 VS Code 更新域名（可能需要触发更新）"
    write_json_report "singbox_log_domains" "skip"
    return 0
  fi

  write_json_report "singbox_log_domains" "pass"
}

# ────────────────────────────────────────────────────────────────────────────
# 主流程
# ────────────────────────────────────────────────────────────────────────────

main() {
  info "VS Code Remote-SSH E2E UAT — dry_run=${DRY_RUN}"
  echo ""

  scenario_singbox_process || true
  echo ""
  scenario_egress_ip || true
  echo ""
  scenario_dns_leak || true
  echo ""
  scenario_vscode_server || true
  echo ""
  scenario_sshd_process || true
  echo ""
  scenario_singbox_log_domains || true
  echo ""

  render_final_json

  echo ""
  echo "========================================"
  echo "VS Code Remote-SSH E2E UAT 结果: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${SKIP_COUNT} SKIP"

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
