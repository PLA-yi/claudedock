# Phase 35: E2E 稳定化 + 性能验收 - Pattern Map

**Mapped:** 2026-04-22
**Files analyzed:** 13 new + 1 modified
**Analogs found:** 13 / 13（模板可直接复用，无完全空白文件）

---

## 文件分类（File Classification）

| 新增/修改文件 | 角色 | 数据流 | 最近分析对象 | 匹配度 |
|-----------------|------|--------|--------------|-----------|
| `scripts/gen-bench-tree.sh` | utility（fixture 生成器） | file-I/O（本地生成） | `scripts/test-fixture-up.sh` + `verify-fuse-compat.sh` 的 `pass()/fail()` skeleton | role-match（本项目无现成 synthetic tree generator） |
| `scripts/perf-benchmark.sh` | verification script（benchmark runner） | 子进程 time/hyperfine → JSON + markdown | `scripts/ci-doctor-grep.sh` | role-match（JSON+MD 双输出 pattern 完全复用；benchmark 本身是新 domain） |
| `scripts/cold-start-benchmark.sh` | verification script（timing） | docker run → stopwatch → JSON | `scripts/verify-managed-image.sh` + `scripts/ci-doctor-grep.sh` | role-match |
| `scripts/uat-network-resilience.sh` | UAT script（弱网） | `tc qdisc`（sudo）+ tmux capture-pane + stdin 注入 | `scripts/verify-fuse-compat.sh`（阶段式 + docker exec） | role-match |
| `scripts/degradation-regression.sh` | regression script（M13） | 破坏 mount 层 → 断言 stderr 含错误码 + 中文说明 | `scripts/ci-doctor-grep.sh`（错误码 regex 断言） + `test/bootstrap/e2e_bootstrap_ssh.sh`（grep-based 断言） | exact |
| `scripts/v3-acceptance-checklist.sh` | master checklist runner | 批量 check → `[PASS]/[FAIL]/[SKIP]` → markdown 报告 | `scripts/ci-doctor-grep.sh` + `scripts/verify-fuse-compat.sh` | exact（汇总大全融合两个模板） |
| `docs/runbooks/v3-upgrade-guide.md` | runbook markdown | human-read | `docs/runbooks/v3-claude-state-volumes.md` | exact |
| `docs/runbooks/v3-apparmor-deployment.md` | runbook markdown | human-read | `docs/runbooks/v3-claude-state-volumes.md` + `deploy/scripts/host-preflight.sh`（check_apparmor_fusermount3） | exact |
| `docs/runbooks/v3-doctor-troubleshoot.md` | runbook markdown | human-read → 引用 5 维度 | `docs/runbooks/v3-claude-state-volumes.md` + `internal/claudedock/doctor/doctor.go`（维度顺序） | exact |
| `docs/runbooks/v3-persistent-volume-lifecycle.md` | runbook markdown | human-read（整合） | `docs/runbooks/v3-claude-state-volumes.md`（同根同源） | exact — 禁止复制内容，改为 §0 链接跳转 |
| `docs/runbooks/v3-error-code-index.md` | runbook markdown | 从 `internal/claudedock/errcodes/` 生成 → human-read | `docs/runbooks/v3-claude-state-volumes.md`（样式） + `internal/claudedock/errcodes/codes.go`（遍历 Registry） | exact |
| `.github/workflows/ci.yml`（新增 `perf-benchmark` job） | CI workflow | GH Actions runner → script 调用 | `.github/workflows/ci.yml` 现有 `go-test` job + `build-images.yml`（`strategy.matrix.include`） | exact |
| `.github/workflows/ci.yml`（新增 `image-size-regression` job） | CI workflow | docker build → `verify-managed-image.sh` | `.github/workflows/build-images.yml`（docker build 流程） + `scripts/verify-managed-image.sh` | exact |

---

## 共用模板（Pattern Assignments）

### 所有 bash 脚本共享 skeleton（6 个新脚本必须照抄此骨架）

**分析对象：** `scripts/verify-fuse-compat.sh` 与 `scripts/ci-doctor-grep.sh` 混合

**Skeleton 1 — shebang + strict mode + PROJECT_ROOT 解析**（引自 `test/bootstrap/e2e_bootstrap_ssh.sh:1-9`）：

```bash
#!/usr/bin/env bash
# scripts/<name>.sh — Phase 35 <purpose>
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
```

**Skeleton 2 — 4-function output style + 计数器**（引自 `scripts/verify-fuse-compat.sh:3-14`）：

```bash
PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
warn() { echo "[WARN]  $1"; WARN_COUNT=$((WARN_COUNT + 1)); }
info() { echo "[INFO]  $1"; }
```

> ⚠️ `scripts/install.sh:11-13` 使用了 ANSI 色码版本（`printf '\033[1;34m==>\033[0m %s\n'`），Phase 35 所有脚本一律采用 `verify-fuse-compat.sh` 的**纯文本**变体，因为：
> 1. CI 日志不保留颜色
> 2. 便于 `| grep '^\[FAIL\]'` 提取失败项
> 3. JSON 模式下需要稳定 prefix

**Skeleton 3 — tmpdir + trap cleanup**（引自 `scripts/ci-doctor-grep.sh:17-19`）：

```bash
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
```

> `verify-fuse-compat.sh:16-19` 给出容器清理变体：

```bash
cleanup() { docker rm -f "$CONTAINER_NAME" 2>/dev/null || true; }
trap cleanup EXIT
```

**Skeleton 4 — 依赖工具断言**（引自 `scripts/ci-doctor-grep.sh:21-22` + `deploy/scripts/host-preflight.sh:4-9`）：

```bash
command -v jq >/dev/null || { echo "需要 jq (brew install jq / apt install jq)"; exit 1; }

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}
require_cmd hyperfine
```

**Skeleton 5 — 阶段分段 + 汇总输出**（引自 `scripts/verify-fuse-compat.sh:32-34, 228-238`）：

```bash
echo ""
echo "=== 阶段 1: <title> ==="

# ...checks...

echo ""
echo "========================================"
echo "验证结果: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${WARN_COUNT} WARN"
if [ "$FAIL_COUNT" -eq 0 ]; then
  echo "状态: 全部通过"
  exit 0
else
  echo "状态: 存在失败项，请检查上方 [FAIL] 条目"
  exit 1
fi
```

---

### Pattern A: JSON + Markdown 双输出（perf-benchmark / cold-start / v3-acceptance-checklist 必用）

**分析对象：** `scripts/ci-doctor-grep.sh:25-44`

**核心技巧**：`jq -e '...'` 做断言（非 0 退出）+ `jq -r '...'` 抽取字段；二进制先吐 JSON 到 tmp，再 grep 文本输出。

```bash
# 1) 调用生成 JSON（允许非 0 退出码，| true 托底）
hyperfine --warmup 1 --runs 10 \
  --export-json "$WORK/bench.json" \
  -n "local" 'ls -R /tmp/bench-tree' \
  -n "mergerfs" 'ls -R /workspace' || true

# 2) 合法性断言（ci-doctor-grep.sh:29-30 pattern）
jq empty "$WORK/bench.json" >/dev/null 2>&1 \
  || { echo "FAIL: hyperfine JSON 输出非合法" >&2; cat "$WORK/bench.json" >&2; exit 1; }

# 3) 字段抽取（ci-doctor-grep.sh:33 pattern）
jq -e '.results | length == 3' "$WORK/bench.json" >/dev/null \
  || { echo "FAIL: 预期 3 种配置结果，实际不符" >&2; exit 1; }

# 4) 生成 markdown 表（RESEARCH §Code Examples）
jq -r '.results[] | "| \(.command) | \(.mean * 1000 | floor) | \(.max * 1000 | floor) |"' \
  "$WORK/bench.json" > "$REPORT_MD"
```

**P50/P99 从 `.results[].times[]` 数组计算**（research open question #2 的答案）：

```bash
jq -r '
  .results[] |
  {
    name: .command,
    p50: ((.times | sort)[((. | length) * 0.5 | floor)]),
    p99: ((.times | sort)[((. | length) * 0.99 | floor)])
  } | "| \(.name) | \(.p50) | \(.p99) |"' "$WORK/bench.json"
```

---

### Pattern B: 环境感知 skip 逻辑（v3-acceptance-checklist.sh 必用）

**分析对象：** `deploy/scripts/host-preflight.sh:11-48`（OS 版本 + 工具 + profile 三层闸门）

**三层闸门结构**（OS → 工具 → 功能）：

```bash
# 语义：每层任一缺失即 skip（不 fail）

is_ci()        { [[ "${CI:-}" == "true" ]]; }
is_macos()     { [[ "$(uname -s)" == "Darwin" ]]; }
is_linux()     { [[ "$(uname -s)" == "Linux" ]]; }
is_ubuntu25()  {
  [ -f /etc/os-release ] || return 1
  . /etc/os-release
  [ "${ID:-}" = "ubuntu" ] && {
    ubuntu_major="${VERSION_ID%%.*}"
    ver_rest="${VERSION_ID#*.}"
    ubuntu_minor="${ver_rest%%.*}"
    [ "${ubuntu_major}" -gt 25 ] || { [ "${ubuntu_major}" -eq 25 ] && [ "${ubuntu_minor:-0}" -ge 4 ]; }
  }
}
has_tc()       { command -v tc >/dev/null 2>&1; }
has_root_net() { [[ $EUID -eq 0 ]] || sudo -n tc qdisc show dev lo &>/dev/null; }

# Skip 调用点
skip() {
  echo "[SKIP]  $1: $2"
  SKIP_COUNT=$((SKIP_COUNT + 1))
}

# 用法
if is_ci && [[ "$test_id" == "BASE-01-APFS" ]]; then
  skip "BASE-01-APFS" "CI 环境无 APFS，本项目真机执行"
  continue
fi

if ! has_tc || ! has_root_net; then
  skip "BASE-03" "tc qdisc 不可用或无 NET_ADMIN 权限，需本地 sudo 执行"
  continue
fi
```

**信源备注：** `host-preflight.sh:18-35` 是本项目唯一完整的 Ubuntu 版本解析样本；禁止用 `uname -r`（kernel 版本 ≠ OS 版本）。

---

### Pattern C: Docker 容器 helper（perf-benchmark / cold-start / degradation-regression 用）

**分析对象：** `scripts/verify-fuse-compat.sh:74-85`

```bash
IMAGE_NAME="$(awk -F': ' '$1 == "local_dev_image_name" { print $2 }' deploy/docker/managed-user/image.lock)"
[[ -z "${IMAGE_NAME}" ]] && { fail "镜像名读取失败"; exit 1; }

CONTAINER_NAME="bench-$$"
docker run -d \
  --name "$CONTAINER_NAME" \
  --cap-add SYS_ADMIN \
  --device /dev/fuse \
  --security-opt apparmor=unconfined \
  "$IMAGE_NAME" sleep 600 >/dev/null

trap 'docker rm -f "$CONTAINER_NAME" 2>/dev/null || true' EXIT
```

> **`image.lock` 约定：** 所有 Phase 35 脚本都应从 `deploy/docker/managed-user/image.lock` 读取镜像名（`local_dev_image_name` 字段），与 `verify-managed-image.sh:4` 保持一致。

---

### Pattern D: 错误码 + 中文建议 grep 断言（degradation-regression.sh 必用，M13 验证）

**分析对象：** `scripts/ci-doctor-grep.sh:51-72`（awk section 匹配 + grep 否定链）

```bash
# 抓取目标段后用 grep -v 反查缺失项
BAD=$(awk '
  /^── (network|auth|ssh|mount|disk) ──$/ { in_section=1; next }
  /^$/                                    { in_section=0 }
  in_section && /^\s*\[[!✗]\]/            { print $0 }
' "$WORK/report.txt" | grep -v "建议:" || true)
if [ -n "$BAD" ]; then
  echo "FAIL: 以下 [!]/[✗] 行缺 '建议:' 子串:" >&2
  echo "$BAD" >&2
  exit 1
fi

# 错误码格式（<DOMAIN>_<KIND>_<NUM>）严格断言
BAD_CODE=$(... | grep -vE '错误码:\s*[A-Z]+_[A-Z]+_[A-Z0-9]+' || true)
```

**M13 专用变体**（三路分别破坏 → 各自断言对应错误码）：

```bash
# 破坏 mergerfs 层 → 应触发 MOUNT_MERGERFS_FAILED
docker exec "$CONTAINER_NAME" pkill -9 mergerfs || true
OUTPUT=$(docker exec "$CONTAINER_NAME" claudedock doctor --json 2>&1 || true)
if ! echo "$OUTPUT" | jq -e '.checks[] | select(.code == "MOUNT_MERGERFS_FAILED")' >/dev/null; then
  fail "M13: mergerfs 破坏未触发 MOUNT_MERGERFS_FAILED 错误码"
fi
```

**错误码清单来源：** 遍历 `internal/claudedock/errcodes/` 下 8 个域文件的 `const` 块（`codes.go:120-178`）。

---

### Pattern E: 网络破坏 + 恢复（uat-network-resilience.sh 核心）

**分析对象：** RESEARCH §Code Examples + `ci-doctor-grep.sh` 的 `|| true` 安全退出

**tc → iptables 两级 fallback**（本项目无现成脚本，采用 RESEARCH skeleton + 项目风格融合）：

```bash
IFACE="${TEST_IFACE:-eth0}"
HOST_IP="${TARGET_HOST_IP:-}"
DISRUPT_MODE=""

disrupt_start() {
  if has_tc && has_root_net; then
    sudo tc qdisc add dev "$IFACE" root netem loss 100%
    DISRUPT_MODE="tc"
    info "网络破坏已启用：tc netem 100% loss on $IFACE"
  elif command -v iptables &>/dev/null && [[ -n "$HOST_IP" ]]; then
    sudo iptables -I OUTPUT -d "$HOST_IP" -j DROP
    DISRUPT_MODE="iptables"
    info "网络破坏已启用：iptables DROP to $HOST_IP"
  else
    skip "BASE-03" "tc/iptables 均不可用或缺目标 IP"
    return 1
  fi
}

disrupt_stop() {
  case "$DISRUPT_MODE" in
    tc)       sudo tc qdisc del dev "$IFACE" root netem loss 100% 2>/dev/null || true ;;
    iptables) sudo iptables -D OUTPUT -d "$HOST_IP" -j DROP 2>/dev/null || true ;;
  esac
  DISRUPT_MODE=""
}

trap disrupt_stop EXIT  # 脚本异常退出时务必恢复网络
```

---

### Pattern F: tmux capture-pane + pgrep 量化判定（BASE-03 核心断言）

**分析对象：** RESEARCH 的 Pitfall 3 + CONTEXT 的"无感知"量化指标（CONTEXT.md:42-46）

```bash
# 存活断言（CONTEXT D-F3-B）：拔网期间每 5s 取样 1 次
check_alive_loop() {
  local duration="$1" ctr="$2"
  local t=0 interval=5
  while [ "$t" -lt "$duration" ]; do
    if ! docker exec "$ctr" pgrep -f claude >/dev/null; then
      fail "claude 进程在拔网第 ${t}s 退出（应全程存活）"
      return 1
    fi
    sleep "$interval"
    t=$((t + interval))
  done
}

# Buffer 完整性（避开 Pitfall 3 race condition）：
capture_buffer_retry() {
  local session="$1" expected="$2"
  for i in 1 2 3 4 5; do
    sleep 2
    local out
    out=$(tmux capture-pane -t "$session" -p -e 2>/dev/null || echo "")
    if echo "$out" | grep -qF "$expected"; then
      echo "$out"
      return 0
    fi
  done
  return 1
}
```

---

### Pattern G: 运维手册 markdown 头部 + 章节骨架

**分析对象：** `docs/runbooks/v3-claude-state-volumes.md:1-7` 头部 + `§1-§8` 结构

**头部（5 个新手册必须照抄）：**

```markdown
# <Title>（v3.0+）

> 适用版本：v3.0 起；对应阶段 Phase <N>（<phase-id>）
> 关联需求：<REQ-ID>（<短述>）/ <REQ-ID>（<短述>）

---

## 1. 背景
## 2. <域内规范/流程表>
## 3. 生命周期 / 操作步骤
## 4. <Audit / 清单 / 事件>
## 5. 故障排查
## 6. Deferred 项（vX.Y backlog）
## 7. 参考
```

**CONTEXT 要求每章必含"快速诊断命令"小节**（引自 CONTEXT.md:88）：

```markdown
### 快速诊断命令

```bash
# 3-5 条 copy-paste 即跑的命令
claudedock doctor --json | jq '.summary'
docker ps --filter label=com.claudedock.managed=true
...
```

**故障排查节样式**（`v3-claude-state-volumes.md:162-197`）：
- 每个 case 三元：事件/现象 → 排查步骤（编号列表） → 修复命令
- 举例命令必须可直接 copy 执行，不能留 `<placeholder>` 占位

**参考节样式**（`v3-claude-state-volumes.md:233-240`）—引用代码路径时写到函数级：

```markdown
- `internal/claudedock/doctor/doctor.go`（RunDoctor / 5 维度顺序）
- `internal/claudedock/errcodes/codes.go`（Registry / MustRegister）
```

---

### Pattern H: v3-error-code-index.md 专用生成逻辑

**分析对象：** `internal/claudedock/errcodes/codes.go:87-97`（Registry 浅拷贝 API）+ `explanations.go:19-25`（ExplainExempt）

**手册 §2 表格来源：** 遍历 8 个域文件的 `init() MustRegister` 块。

| 域文件 | 对应错误码前缀 |
|--------|----------------|
| `internal/claudedock/errcodes/auth.go` | `AUTH_*` |
| `internal/claudedock/errcodes/disk.go` | `DISK_*` |
| `internal/claudedock/errcodes/mount.go` | `MOUNT_*` |
| `internal/claudedock/errcodes/net.go` | `NET_*` |
| `internal/claudedock/errcodes/session.go` | `SESSION_*` |
| `internal/claudedock/errcodes/ssh.go` | `SSH_*` |
| `internal/claudedock/errcodes/state.go` | `STATE_*` |
| `internal/claudedock/errcodes/system.go` | `SYSTEM_*` |

**建议做法：** 在手册 §2 开头嵌入自动化导出命令，保证手册与代码不漂移：

```markdown
> 本表由 `claudedock explain --all` 或 `go run ./cmd/errcodes-dump`（如有）生成，
> 直接映射 `internal/claudedock/errcodes/` 下 `init()` 中 `MustRegister` 调用。
> 若发现本表与实际行为不一致，以代码为准。
```

断言所有 Code 都 ≤ 1 处定义（通过 `MustRegister` 重复 panic 已保证，`codes.go:73-75`），手册只需列出：
- **Code**（来自 `Entry.Code`）
- **Severity**（INFO/WARN/ERROR/FATAL，来自 `codes.go:28-40`）
- **Message**（中文）
- **NextAction**（中文建议）
- **Domain**（由前缀推断）
- **是否登记 ExtendedExplanation**（来自 `ExplainExempt` 对照表）

---

### Pattern I: v3-doctor-troubleshoot.md 专用结构

**分析对象：** `internal/claudedock/doctor/doctor.go:83-84`（5 维度顺序 + 执行时序）

**手册骨架**（每个维度 1 个小节，顺序必须与代码一致）：

```markdown
## 3. 五维度检查逻辑

### 3.1 network（本地可独立跑）
- 检查项：dns_resolve / gateway_reachable / egress_ip_visible
- 常见错误码：SYSTEM_DNS_RESOLVE_FAILED / NET_EGRESS_IP_DRIFT
- 排障：...

### 3.2 auth（cfg + Entry API + 远端 OAuth）
### 3.3 ssh
### 3.4 mount
### 3.5 disk
```

**信源：** `doctor.go:153-172`（network 实现样板）展示 check 命名约定（`domain="network"`, `name="dns_resolve"` 等）。

---

### Pattern J: v3-persistent-volume-lifecycle.md 整合规则

**禁止重复 `v3-claude-state-volumes.md` 已有内容。** 建议方案：

1. 新文件定位为**顶层总览**，引导读者按问题类型跳转：
   ```markdown
   ## 本手册导航

   | 问题 | 跳转 |
   |------|------|
   | Claude OAuth 缓存 / `~/.claude` 持久化 | → [v3-claude-state-volumes.md](./v3-claude-state-volumes.md) |
   | 同步 Volume 寿命 / GC / Mutagen 数据卷 | § 本文件 §3 |
   | mergerfs union 上层 cold/hot 卷 | § 本文件 §4 |
   ```
2. 新增内容集中在 Phase 29-32 的 hot/cold 双卷机制（`v3-claude-state-volumes.md` 不覆盖此范围）。

---

### Pattern K: .github/workflows/ci.yml 新增 job 样板

**分析对象：** `.github/workflows/ci.yml:18-32` 的 `go-test` job + `build-images.yml:19-37` 的 `strategy.matrix.include`

**`perf-benchmark` job 模板：**

```yaml
  perf-benchmark:
    name: Performance Benchmark (synthetic 10k)
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Install hyperfine + ripgrep
        run: |
          sudo apt-get update
          sudo apt-get install -y hyperfine ripgrep jq

      - name: Generate synthetic tree
        run: bash scripts/gen-bench-tree.sh --count=10000 --output=/tmp/bench-tree

      - name: Run benchmark (CI baseline only)
        run: bash scripts/perf-benchmark.sh --ci-mode

      - name: Upload bench artifact
        uses: actions/upload-artifact@v4
        with:
          name: perf-bench-${{ github.sha }}
          path: .planning/phases/35-e2e/benchmarks/
```

**`image-size-regression` job 模板**（复用现有 `verify-managed-image.sh`）：

```yaml
  image-size-regression:
    name: Image Size Regression (BASE-04)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Build managed-user image
        run: docker build -t ghcr.io/test/managed-user:${{ github.sha }} -f deploy/docker/managed-user/Dockerfile .
      - name: Assert size ≤ 700MB
        run: |
          bash scripts/verify-managed-image.sh
          size=$(docker image inspect ghcr.io/test/managed-user:${{ github.sha }} --format='{{.Size}}')
          max=$((700 * 1024 * 1024))
          if [ "$size" -gt "$max" ]; then
            echo "FAIL: image size $size > $max (700MB)" >&2
            exit 1
          fi
```

**sudo 可用性：** GH Actions `ubuntu-latest` runner 默认允许 `sudo` 无密码；`tc qdisc` **可运行**但受限于容器化 runner kernel。RESEARCH §Open Questions #1 建议在脚本内用 `has_root_net()`（见 Pattern B）预检，不行就 SKIP 而非 FAIL。

---

## 横切 Pattern 汇总（Shared Patterns）

### Pattern L: 产物目录约定

**所有基准/UAT 报告写入：** `.planning/phases/35-e2e/benchmarks/`（CONTEXT 第 115 行明确要求）

```bash
BENCH_DIR="${PROJECT_ROOT}/.planning/phases/35-e2e/benchmarks"
mkdir -p "$BENCH_DIR"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
REPORT_JSON="${BENCH_DIR}/bench-${TIMESTAMP}.json"
REPORT_MD="${BENCH_DIR}/bench-${TIMESTAMP}.md"
```

**验收报告文件名含日期戳**（CONTEXT 第 70 行）：
```bash
ACCEPTANCE_REPORT="${PROJECT_ROOT}/docs/runbooks/v3-acceptance-report-$(date +%Y%m%d).md"
```

---

### Pattern M: 中文退出提示 + 建议子串

**项目已固化的文案约定**（`scripts/install.sh:12-13`、`ci-doctor-grep.sh:21-22`、`verify-fuse-compat.sh:27`）：
- 错误提示含 "**错误:**" / "**FAIL:**" 前缀
- **每条 `[FAIL]` 行后必须有"建议:"子串 + 错误码**（`ci-doctor-grep.sh:51-67` 硬断言）
- 中文说明长度 ≤ 80 runes（与 `errcodes.Entry.NextAction` 约束一致）

---

### Pattern N: CLAUDE.md 路径安全约束

**绝对路径 → `$PROJECT_ROOT` 相对路径**。所有 Phase 35 脚本禁止硬编码 `/Users/...` 或 `/home/...`。

信源：`CLAUDE.md` + RESEARCH 的 Anti-Patterns 条目。

---

## 无现成 analog 的文件（需结合 RESEARCH 模板自建）

| 文件 | 原因 | Fallback |
|------|------|----------|
| 纯 synthetic tree 生成器 `gen-bench-tree.sh` | 项目历史上无同等需求 | 使用 RESEARCH §Code Examples 中骨架 + 本 PATTERNS 的 skeleton 1-5 组装 |
| hyperfine 驱动的 `perf-benchmark.sh` | 项目首次引入 hyperfine | RESEARCH §Benchmark Script Skeleton 直接复制 + Pattern A JSON 处理 |
| `tc qdisc` 控制的网络破坏 | 项目首次在脚本中控制内核网络 | RESEARCH §Network Disruption Script Skeleton + Pattern E 的 fallback |

> 这三类脚本均可从 RESEARCH.md 中的示例（lines 208-283）起步，套用本 PATTERNS.md 的 skeleton 1-5 + Pattern A/B/E 即可落地。

---

## 运维手册与脚本引用关系（Planner 参考）

| 新增手册 | 必须枚举/引用的代码位置 |
|----------|--------------------------|
| `v3-upgrade-guide.md` | `deploy/docker/managed-user/image.lock`（镜像版本锁）+ `scripts/install.sh`（客户端升级流程） |
| `v3-apparmor-deployment.md` | `deploy/scripts/host-preflight.sh:11-73`（check_apparmor_fusermount3，已有部署建议文案） + `scripts/verify-fuse-compat.sh:42-58` |
| `v3-doctor-troubleshoot.md` | `internal/claudedock/doctor/doctor.go:83-84`（5 维度顺序）+ `doctor/{network,auth,ssh,mount,disk}.go`（每维度 check） + `doctor/fix.go`（--fix 幂等修复） |
| `v3-persistent-volume-lifecycle.md` | 现有 `v3-claude-state-volumes.md`（跳转链接） + `internal/runtime/tasks/worker.go`（hot/cold 卷生命周期）  |
| `v3-error-code-index.md` | `internal/claudedock/errcodes/codes.go:120-178`（Code 常量） + 8 个域 `init()` 文件 + `explanations.go`（长说明） |

---

## Metadata

**分析扫描范围：**
- `scripts/*.sh`（11 个文件）
- `deploy/scripts/*.sh`（3 个文件）
- `test/bootstrap/*.sh`
- `docs/runbooks/`（已有 1 份参照）
- `.github/workflows/*.yml`（4 个）
- `internal/claudedock/errcodes/`（12 个 Go 文件）
- `internal/claudedock/doctor/`（19 个 Go 文件，聚焦 `doctor.go` / `check.go`）

**关键模板来源文件（按重要性排序）：**
1. `scripts/verify-fuse-compat.sh` — 阶段式 + 4 函数 + 汇总 skeleton（Phase 35 所有脚本核心骨架）
2. `scripts/ci-doctor-grep.sh` — JSON + 文本双模式 + awk/grep 断言（v3-acceptance-checklist / degradation-regression 核心）
3. `docs/runbooks/v3-claude-state-volumes.md` — 运维手册头部 + 8 章节骨架（5 份新手册样板）
4. `deploy/scripts/host-preflight.sh` — OS 版本解析 + advisory skip（Pattern B 信源）
5. `.github/workflows/ci.yml` + `build-images.yml` — CI job 结构模板

**Pattern extraction date:** 2026-04-22
