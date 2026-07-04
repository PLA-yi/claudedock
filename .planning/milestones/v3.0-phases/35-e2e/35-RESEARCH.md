# Phase 35: E2E 稳定化 + 性能验收 - Research

**Researched:** 2026-04-22
**Domain:** Performance benchmarking, network resilience UAT, multi-OS acceptance testing, runbook documentation
**Confidence:** HIGH

## Summary

Phase 35 is the final v3.0 acceptance phase — no new feature code, only verification artifacts. The work splits into four tracks: (1) automated performance benchmarks with synthetic 10k file trees, (2) scripted network resilience UAT using `tc qdisc`, (3) multi-OS real-machine validation (macOS APFS + Ubuntu 25.04 AppArmor), and (4) operations runbooks + acceptance checklist.

All functional code should already be shipped in Phases 29-34. This phase's risk is NOT technical implementation — it's ensuring the verification scripts and runbooks are comprehensive enough to catch regressions, and that the acceptance checklist is executable enough to be run by a human on real hardware.

**Primary recommendation:** Build all verification as executable bash scripts with JSON + markdown dual output, reuse existing script patterns from Phases 29-34, and treat the acceptance checklist as the single source of truth for v3.0 sign-off.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **10k 文件树**：使用 synthetic 脚本生成（`scripts/gen-bench-tree.sh`），保证可重复性。文件结构模拟 mono-repo：80% 小文件（< 4KB 源码）、15% 中等文件（< 1MB 配置/文档）、5% 大文件（< 10MB 二进制），总大小控制在 ~200MB。
- **对比基线包含 3 档**：
  1. 本地文件系统（宿主机 ext4 或 APFS）——绝对基线
  2. mergerfs full 模式（Mutagen + sshfs + mergerfs 三层全开）
  3. sshfs-only 降级模式（验证降级后的性能下限）
- **统计方式**：每种配置 warm-up 1 次 + 测量 10 次，取 P50 和 P99。报告输出 JSON + 人类可读表格。
- **CI 自动化**：在 `.github/workflows/ci.yml` 新增 `perf-benchmark` job，跑 ubuntu-latest 上的 synthetic 基准。macOS APFS 真机基准不在 CI 中跑，作为本地/真机验收项。
- **基准命令**：`rg .`（全量文本搜索）和 `ls -R /workspace`（元数据遍历），两者分别对应 CPU 密集和 metadata 密集场景。
- **拔网手段**：脚本化 `tc qdisc add dev <iface> root netem loss 100%`（精确可控），恢复时 `tc qdisc del`。备选 `iptables -I OUTPUT -d <host_ip> -j DROP`。
- **判定标准（量化）**：
  - **10s 拔网**：claudedock 进程不退出；tmux 内 claude 进程 `ps` 仍在；本地 input_buffer 键入内容不丢
  - **30s 拔网**：同上 + 恢复网络后 60s 内自动重连成功；`tmux capture-pane` 与拔网前 buffer 一致
  - **2min 拔网**：claudedock 最终进入"重连失败提示"状态（REQ-F3-C）；tmux 内进程仍存活；恢复网络后手动按 Enter 可重新连接
- **"无感知"量化指标**：
  - 进程存活：`docker exec <ctr> pgrep -f claude` 在拔网全程返回 0
  - Buffer 完整性：拔网前 `tmux capture-pane` 与恢复后对比，字符级一致
  - 输入回放：本地脚本向 stdin 注入固定字符串，重连后远端 `cat` 输出与注入一致
- **执行方式**：脚本驱动（`scripts/uat-network-resilience.sh`），关键场景（30s/2min）需人工在报告中签字确认观察结果。
- **macOS APFS**：使用开发者本地 M 系列 Mac 执行。GitHub Actions macos runner 为虚拟化环境，FUSE 性能数据不具参考性，故不作为 CI 基准平台。
  - 必测场景：case-insensitive 双向同步（创建 `Foo.txt` + `foo.txt` 冲突文件，断言 Mutagen `--mode=two-way-resolved` 无数据丢失）
- **Ubuntu 25.04**：CI 中使用 `ubuntu-latest`（目前 24.04）跑 AppArmor 模拟检测 + docker 三路 FUSE 挂载验证。若需严格 25.04 内核行为验证，在真机或云主机上补跑。
  - 必测场景：AppArmor `local override` 部署后 `verify-fuse-compat.sh` 全通过；sshfs + mutagen-agent + mergerfs 三路并发 mountpoint 全部就绪
- **自动化程度**：脚本化 80% + 人工签字 20%。脚本自动生成测试报告（JSON + markdown），人工在关键场景（APFS 冲突、2min 拔网、AppArmor 真机）报告中确认并签字。
- **手册位置**：`docs/runbooks/` 目录新增 5 章，与已有 `v3-claude-state-volumes.md` 保持一致风格。文件名前缀 `v3-`：
  - `v3-upgrade-guide.md` — 升级指南
  - `v3-apparmor-deployment.md` — AppArmor override 部署
  - `v3-doctor-troubleshoot.md` — doctor 排障手册
  - `v3-persistent-volume-lifecycle.md` — 持久卷生命周期与 GC（与已有 `v3-claude-state-volumes.md` 整合，不重复）
  - `v3-error-code-index.md` — 错误码索引
- **验收清单**：`scripts/v3-acceptance-checklist.sh` — 可执行 bash 脚本，遍历 30 条 REQ + 4 条 BASE，每项输出 `[PASS]/[FAIL]/[SKIP]` + 证据路径。脚本末尾生成 markdown 报告 `v3-acceptance-report.md`。
- **签字流程**：
  1. 脚本在目标环境执行生成报告
  2. 报告附于 Phase 35 PR 中
  3. PR 合并视为"签字通过"
  4. 真机环境需在报告中显式标注机器信息（OS 版本、硬件型号、执行时间）
- **版本标记**：手册头部标注 `适用版本: v3.0.x`，验收报告文件名含日期戳（`v3-acceptance-report-20260422.md`）。

### Claude's Discretion

- 10k 文件 synthetic 生成的具体目录深度和文件分布比例
- 性能基准报告的精确输出格式（JSON schema 细节）
- 弱网 UAT 脚本中 `tc` vs `iptables` 的最终选型（优先 `tc`，如环境不支持回退 `iptables`）
- 验收清单脚本中 SKIP 项的判定逻辑（环境不具备时优雅跳过）
- 运维手册的章节内具体排版和示例命令格式

### Deferred Ideas (OUT OF SCOPE)

- 持续性能监控（perf regression dashboard）—— v3.1+ 可考虑，不在本验收 phase 内
- 自动化真机农场（multi-OS CI runner）—— 资源投入较大，v3.1 评估
- 性能基准的历史趋势图（自动生成折线图对比各版本）—— 需要额外基础设施，v3.1 评估
- 弱网 UAT 的 packet-level 抓包分析 —— 如验收发现问题时可深入，非本 phase 交付物
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| BASE-01 | 元数据响应：10k 文件 `rg .` / `ls -R` ≤ 本地 1.5× | Synthetic tree + `hyperfine` or shell `time` loop for P50/P99 |
| BASE-02 | 首连 ≤ 8s：claudedock 冷启动到 prompt 可输入 | Shell `time` wrapper + tmux capture-pane 检测 prompt 就绪 |
| BASE-03 | 弱网容忍：30s 抖动无感知，2min 后自动重连 | `tc qdisc` netem + `pgrep` / `tmux capture-pane` 量化判定 |
| BASE-04 | 镜像体积 ≤ 700MB CI gate（二次回归） | Reuse `scripts/verify-managed-image.sh` from Phase 29 |
| REQ-F1-B | 三段式中文进度端到端测量 | BASE-02 脚本同时断言 stderr 输出包含三段式关键字 |
| REQ-F1-C | 10k 文件性能 1.5× | BASE-01 直接覆盖 |
| REQ-F3-B | 断网本地缓冲 + 灰色样式 | UAT 脚本注入 stdin + 远端 `cat` 验证回放 |
| REQ-F3-C | 重连失败提示明确 | 2min 拔网场景断言 stderr 包含中文提示 + 下一步操作 |
| REQ-F3-D | 退避策略 1/2/4/8/30s | UAT 脚本每 10s 打印状态日志，人工核对退避序列 |
| REQ-F4-A | tmux 默认包装 + 重连不丢进程 | `pgrep -f claude` 全程存活断言 |
| REQ-F5-D | 账号级 Mutagen 单例锁 | 多端场景在验收清单中覆盖 |
| M5 | APFS case-insensitive 冲突 | `Foo.txt`/`foo.txt` 冲突文件创建 + 断言无数据丢失 |
| M13 | 禁止静默降级 | 三层分别破坏 + 断言 stderr 含中文降级说明 + 错误码 |
| C6 | Ubuntu 25.04 AppArmor 嵌套 FUSE | `host-preflight.sh` + `verify-fuse-compat.sh` 复用 |
</phase_requirements>

## Standard Stack

### Core
| Library/Tool | Version | Purpose | Why Standard |
|--------------|---------|---------|--------------|
| `hyperfine` | 1.18+ | Cross-platform command benchmarking | Community standard for CLI perf; outputs JSON; handles warm-up |
| `tc` (iproute2) | 宿主机内核自带 | Network disruption simulation | Kernel-native, precise, reversible |
| `rg` (ripgrep) | 14.x+ | Full-text search benchmark command | Fast, respects `.gitignore`, standard in dev workflows |
| `tmux` | 3.6a+ | Session survival + buffer capture | Already in v3.0 image; `capture-pane` for buffer integrity checks |
| `flock` | util-linux 2.39+ | Sync lock mechanism | Already used in Phase 32 sync-lock |
| `jq` | 1.7+ | JSON parsing in bash scripts | De facto standard for shell JSON processing |

### Supporting
| Library/Tool | Version | Purpose | When to Use |
|--------------|---------|---------|-------------|
| `iptables` | 宿主机自带 | Fallback network disruption | When `tc` unavailable (e.g. macOS or restricted containers) |
| `getfattr` | attr 2.5+ | mergerfs branch verification | Already used in Phase 29/34 doctor checks |
| `docker image inspect` | Docker 28.x | Image size measurement | BASE-04 CI gate |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `hyperfine` | Shell `for i in {1..10}; do time ...; done` | Hyperfine handles warm-up, outlier detection, JSON export; raw shell is error-prone |
| `tc qdisc` | `iptables -I OUTPUT -j DROP` | `tc` is more precise (per-interface, per-delay); `iptables` is broader and harder to scope |
| Synthetic tree generation | Clone real mono-repo | Synthetic guarantees reproducibility across CI and local runs; real repo varies over time |

## Architecture Patterns

### Pattern 1: Dual-Output Verification Script
**What:** Every verification script produces both machine-readable JSON and human-readable markdown.
**When to use:** All benchmark and UAT scripts in this phase.
**Example:**
```bash
# From Phase 34 ci-doctor-grep.sh pattern
output_json() { jq -n --arg status "$1" --arg evidence "$2" '{status:$status,evidence:$evidence}'; }
output_md()  { echo "| $1 | $2 | $3 |"; }
```

### Pattern 2: pass/fail/warn/info Unified Functions
**What:** All scripts use the same 4-function output style established in Phase 29-34.
**When to use:** Every bash script in this phase.
**Example:**
```bash
# Established pattern from test/bootstrap/e2e_bootstrap_ssh.sh
pass()  { echo "[PASS] $*"; ((PASS++)); }
fail()  { echo "[FAIL] $*" >&2; ((FAIL++)); }
warn()  { echo "[WARN] $*"; }
info()  { echo "[INFO] $*"; }
```

### Pattern 3: Environment-Aware Skip Logic
**What:** Scripts detect their runtime environment and SKIP tests that require unavailable infrastructure.
**When to use:** Acceptance checklist running in CI vs. local Mac vs. Ubuntu 25.04 bare metal.
**Example:**
```bash
is_ci()       { [[ "${CI:-}" == "true" ]]; }
is_macos()    { [[ "$(uname -s)" == "Darwin" ]]; }
is_ubuntu25() { grep -q "25.04" /etc/os-release 2>/dev/null; }

if is_ci && [[ "$test_id" == "BASE-01-APFS" ]]; then
  skip "APFS test not available in CI"
fi
```

### Anti-Patterns to Avoid
- **Hard-coding absolute paths:** All paths must be relative to project root or use `$PWD` resolution (matches CLAUDE.md security constraint).
- **Relying on human observation without scriptable assertion:** Every success criterion must have at least one automated check; human sign-off is for "I observed the screen and it matches" not "I think it probably worked."
- **Writing benchmark results only to stdout:** Results must be persisted to `.planning/phases/35-e2e/benchmarks/` for version-to-version comparison.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Command benchmarking with timing | Shell `time` loops | `hyperfine` | Warm-up handling, statistical analysis (mean/P50/P99), JSON export, outlier detection |
| Network disruption simulation | Custom proxy / firewall rules | `tc qdisc netem` | Kernel-native, millisecond-precision, fully reversible with `tc qdisc del` |
| JSON generation from bash | `echo '{"key": "'"$var"'"}'` | `jq -n` | Proper escaping, schema validation, readable |
| Markdown table generation | Manual `printf` | Templated heredoc with `column -t` | Maintainable, handles variable-width content |

**Key insight:** This phase is 100% glue scripts and documentation. The quality of the phase depends on using mature, well-tested tools for benchmarking and simulation rather than custom implementations that introduce their own bugs.

## Common Pitfalls

### Pitfall 1: Benchmark Noise from Background Processes
**What goes wrong:** CI runners or local laptops have background activity (indexing, Docker healthchecks) that skews P99 by 10× or more.
**Why it happens:** `rg` and `ls -R` are metadata-heavy; a single filesystem cache eviction ruins the tail latency.
**How to avoid:** Warm-up run mandatory before measurement; run 10 iterations and report P50/P99 separately; document hardware/CI runner specs in report.
**Warning signs:** P99 >> 2× local baseline while P50 is fine — indicates noise, not real performance issue.

### Pitfall 2: `tc qdisc` Requires Root / CAP_NET_ADMIN
**What goes wrong:** UAT script fails on macOS or in Docker-in-Docker CI because `tc` needs privileges.
**Why it happens:** macOS uses `pfctl` not `tc`; GitHub Actions `ubuntu-latest` containers don't have `NET_ADMIN` by default.
**How to avoid:** Script detects `tc` availability and falls back to `iptables` or `pfctl`; CI job runs with `--privileged` or documents the limitation.
**Warning signs:** `RTNETLINK answers: Operation not permitted` on script startup.

### Pitfall 3: tmux Buffer Capture Race Condition
**What goes wrong:** `tmux capture-pane` right after reconnection shows an empty or partial buffer because tmux hasn't finished redrawing.
**Why it happens:** Reconnection involves SSH handshake + tmux attach + pane redraw; capturing too early misses content.
**How to avoid:** Sleep 2s after `tmux attach` before `capture-pane`; or use `tmux capture-pane -e -p` with retry loop until expected content appears.
**Warning signs:** Buffer integrity check fails intermittently.

### Pitfall 4: Synthetic Tree Doesn't Represent Real Workloads
**What goes wrong:** 10k files of random content benchmark well but real mono-repos have deep `.git` trees, `node_modules`, symlinks.
**Why it happens:** `rg` skips binary files and respects `.gitignore`; synthetic content must mimic these patterns for realistic numbers.
**How to avoid:** Include `.git/` directory with realistic object count; include `node_modules/` with nested empty dirs; use realistic source file extensions.
**Warning signs:** Benchmark `rg .` is faster than expected because there are no binary files to skip.

### Pitfall 5: Acceptance Checklist Becomes Unmaintainable
**What goes wrong:** 34 items × 3 environments = 100+ checks in one giant script that nobody wants to run.
**Why it happens:** Without modular structure, a single failing early check blocks the entire run.
**How to avoid:** Group by track (BASE / REQ-F1 / REQ-F3 / ...); each track is a function; `--track=base` flag for partial runs; continue-on-failure with summary at end.
**Warning signs:** Script takes > 30 minutes to complete; developers run it once and never again.

## Code Examples

### Benchmark Script Skeleton
```bash
#!/usr/bin/env bash
# Source: Phase 35 research — established project pattern
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BENCH_DIR="${PROJECT_ROOT}/.planning/phases/35-e2e/benchmarks"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
REPORT_JSON="${BENCH_DIR}/bench-${TIMESTAMP}.json"
REPORT_MD="${BENCH_DIR}/bench-${TIMESTAMP}.md"

pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*" >&2; }

# Generate synthetic tree
"${PROJECT_ROOT}/scripts/gen-bench-tree.sh" --count=10000 --output="/tmp/bench-tree"

# Run hyperfine for each configuration
hyperfine --warmup 1 --runs 10 \
  --export-json "${REPORT_JSON}" \
  -n "local" 'ls -R /tmp/bench-tree' \
  -n "mergerfs" 'ls -R /workspace' \
  -n "sshfs-only" 'ls -R /mnt/cold'

# Generate markdown report from JSON
jq -r '.results[] | "| \(.command) | \(.mean) | \(.max) |"' "${REPORT_JSON}" > "${REPORT_MD}"
```

### Network Disruption Script Skeleton
```bash
#!/usr/bin/env bash
# Source: Phase 35 research — tc-based approach with fallback

IFACE="${TEST_IFACE:-eth0}"
HOST_IP="${TARGET_HOST_IP:-}"

disrupt_network() {
  local duration="$1"
  if command -v tc &>/dev/null && sudo tc qdisc show dev "$IFACE" &>/dev/null; then
    sudo tc qdisc add dev "$IFACE" root netem loss 100%
    sleep "$duration"
    sudo tc qdisc del dev "$IFACE" root netem loss 100%
  elif command -v iptables &>/dev/null; then
    sudo iptables -I OUTPUT -d "$HOST_IP" -j DROP
    sleep "$duration"
    sudo iptables -D OUTPUT -d "$HOST_IP" -j DROP
  else
    echo "[SKIP] No network disruption tool available"
    return 1
  fi
}

# Usage: disrupt_network 30  # 30s blackout
```

### Acceptance Checklist Item Pattern
```bash
# From Phase 34 ci-doctor-grep.sh style
check_item() {
  local id="$1" desc="$2" cmd="$3" expected="$4"
  local output
  output=$(eval "$cmd" 2>&1) || true
  if echo "$output" | grep -q "$expected"; then
    echo "[PASS] ${id}: ${desc}"
    echo "| ${id} | PASS | ${desc} |" >> "$REPORT_MD"
    ((PASS++))
  else
    echo "[FAIL] ${id}: ${desc}"
    echo "Expected: ${expected}"
    echo "Got: ${output}"
    echo "| ${id} | FAIL | ${desc} |" >> "$REPORT_MD"
    ((FAIL++))
  fi
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Manual UAT with `ctrl+c` ssh | Scripted `tc qdisc` disruption | Phase 35 | Reproducible, quantifiable, CI-friendly |
| Single-run `time` measurement | `hyperfine` with warm-up + P50/P99 | Phase 35 | Statistical rigor for performance claims |
| Plain text test reports | JSON + markdown dual output | Phase 34 | Machine-consumable for regression detection |
| Per-phase scattered docs | `docs/runbooks/v3-*.md` unified | Phase 35 | Consistent ops documentation |

**Deprecated/outdated:**
- `mutagen` daemon lifecycle management by claudedock: Removed in recent commits (Phase 34 refactor), replaced with in-process hot sync.

## Open Questions (RESOLVED)

1. **Does GitHub Actions `ubuntu-latest` support `tc qdisc`?**
   - What we know: GA runners are VMs with full kernel access; `tc` should work with `sudo`.
   - What's unclear: Whether `NET_ADMIN` capability is available without `--privileged`.
   - Recommendation: Test in CI with `sudo tc qdisc show dev eth0` as a pre-check; fall back to documented SKIP if unavailable.
   > RESOLVED: CI 中以 `sudo tc qdisc show dev eth0` 作预检；不可用时脚本走 `iptables` fallback，再不行则 SKIP 并在报告中文档化（已在 Plan 02 三层闸门落地）。

2. **What is the exact `hyperfine` JSON schema for P50/P99 extraction?**
   - What we know: `hyperfine` `--export-json` includes `results[].times[]` array of all runs.
   - What's unclear: Whether P50/P99 are pre-computed or need manual calculation from `times` array.
   - Recommendation: Use `jq` to compute percentiles from `times` array; verify schema with `hyperfine --version`.
   > RESOLVED: 不依赖 hyperfine 内置百分位；用 `jq` 从 `.results[].times[]` 排序后按索引抽取 P50 / P99（已在 Plan 01 T2 `<interfaces>` 与 PATTERNS Pattern A 末段固化）。

3. **How to programmatically detect "prompt 可输入" for BASE-02?**
   - What we know: Claude Code prompt shows `>` or similar indicator.
   - What's unclear: Exact string to grep for in `tmux capture-pane` output.
   - Recommendation: Use `tmux capture-pane -p | grep -E '(>|claude|➜)'` with timeout loop; document the exact pattern after first manual observation.
   > RESOLVED: 使用 `tmux capture-pane -p | grep -E '(^>|claude> |➜ )'` 带超时循环（200ms × ≤ 75 次 = 15s 上限）；已在 Plan 01 T3 action 锁定。

## Validation Architecture

> Skip this section entirely if workflow.nyquist_validation is false in .planning/config.json

**Note:** `.planning/config.json` has `workflow.nyquist_validation: false`. This section is included for completeness but will not drive plan structure.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go test + bash integration scripts |
| Config file | none — see Wave 0 |
| Quick run command | `go test ./internal/claudedock/... -count=1` |
| Full suite command | `make ci-gate` (includes doctor grep + image size + unit tests) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| BASE-01 | 10k file rg/ls-R P50 ≤ 1.5× | integration | `scripts/perf-benchmark.sh` | ❌ Wave 0 |
| BASE-02 | Cold start ≤ 8s + 三段式进度 | integration | `scripts/cold-start-benchmark.sh` | ❌ Wave 0 |
| BASE-03 | 30s blackout no perception | manual+script | `scripts/uat-network-resilience.sh` | ❌ Wave 0 |
| BASE-04 | Image ≤ 700MB | CI gate | `scripts/verify-managed-image.sh` | ✅ Phase 29 |
| M13 | Silent downgrade detection | integration | `scripts/degradation-regression.sh` | ❌ Wave 0 |

### Wave 0 Gaps
- [ ] `scripts/gen-bench-tree.sh` — synthetic tree generator
- [ ] `scripts/perf-benchmark.sh` — BASE-01 benchmark runner
- [ ] `scripts/cold-start-benchmark.sh` — BASE-02 timing measurement
- [ ] `scripts/uat-network-resilience.sh` — BASE-03 network UAT
- [ ] `scripts/degradation-regression.sh` — M13 silent downgrade test
- [ ] `scripts/v3-acceptance-checklist.sh` — Master checklist runner
- [ ] `docs/runbooks/v3-*.md` — 5 new runbook chapters
- [ ] `.github/workflows/ci.yml` — `perf-benchmark` + `image-size-regression` jobs

## Sources

### Primary (HIGH confidence)
- `hyperfine` official docs (sharkdp/hyperfine) — JSON export schema, warmup behavior, statistical methods
- `iproute2` / `tc` man pages — `tc qdisc add/del` syntax, `netem` options
- Project Phase 29-34 scripts — `scripts/ci-doctor-grep.sh`, `scripts/verify-fuse-compat.sh`, `scripts/verify-managed-image.sh` as established patterns
- Project `internal/claudedock/errcodes/` — error code registry for M13 verification
- Project `internal/claudedock/doctor/` — 5-dimension check framework for runbook reference

### Secondary (MEDIUM confidence)
- GitHub Actions documentation — runner capabilities, `sudo` availability, Docker privilege model
- tmux man page — `capture-pane` options, `-e` (escape sequences), `-p` (print to stdout)

### Tertiary (LOW confidence)
- macOS `pfctl` as `tc` alternative — not verified; marked for validation if needed

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all tools are well-established, versions known
- Architecture: HIGH — patterns established in Phases 29-34, direct reuse
- Pitfalls: MEDIUM-HIGH — some UAT edge conditions (tmux race, prompt detection) need empirical validation

**Research date:** 2026-04-22
**Valid until:** 2026-05-22 (stable tools, long validity)
