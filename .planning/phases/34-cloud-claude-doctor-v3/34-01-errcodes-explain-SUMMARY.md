---
phase: 34-cloud-claude-doctor-v3
plan: 01
subsystem: cli
tags: [errcodes, registry, explain, cobra, rustc-style, doctor-foundation]

# Dependency graph
requires:
  - phase: 31-cli
    provides: errcodes Registry/MustRegister/Lookup/Format 基座 + 13 条 MOUNT/NET 错误码
  - phase: 32-ssh-tmux
    provides: SESSION_*/NET_RECONNECT_*/NET_TCP_KEEPALIVE_* 共 9 条错误码注册
  - phase: 33-claude-code-cli-admin-gc
    provides: STATE_VOLUME_IN_USE_001 字面量（admin handler 硬编码，本 plan 补录到 Registry）
provides:
  - 8 域闭合错误码 Registry：{MOUNT,SESSION,NET,STATE,SYSTEM,SSH,AUTH,DISK}（42 条注册项）
  - ExtendedExplanations map（38 条 ≥ 200 中文字符长说明，五段模板）
  - ExplainExempt set（4 条 Info 类豁免）
  - registerExplanation helper（防御重复 + 空串 panic）
  - cloud-claude explain <code> CLI 子命令（对标 rustc --explain，未注册 exit 4）
  - 4 条 explanations 断言测试（覆盖 / lower-case / 8 域闭合 / Info-only）
affects:
  - 34-02-doctor-framework（doctor 检查项命中错误码时 errcodes.Format + errcodes.Lookup 输出）
  - 34-03-doctor-fix-integration（--fix 5 类修复对应 SYSTEM_FUSE_RESIDUAL_MOUNT / SYSTEM_DNS_RESOLVE_FAILED / SSH_KNOWN_HOSTS_CONFLICT / AUTH_TOKEN_EXPIRED 字面量）

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "errcodes 域文件模板：每域单独 .go 文件，init() 调 MustRegister，文件名与 DOMAIN 一致"
    - "ExtendedExplanations 五段模板：触发场景 / 根本原因 / 复现方式 / 修复路径 / 关联文档"
    - "explain 子命令 三段输出：Format(code) + 空行 + 详细说明（rustc --explain 风格）"
    - "子进程级 cobra 测试：buildOnce + exec.CommandContext + ExitError.ExitCode() 断言"

key-files:
  created:
    - internal/cloudclaude/errcodes/state.go
    - internal/cloudclaude/errcodes/system.go
    - internal/cloudclaude/errcodes/ssh.go
    - internal/cloudclaude/errcodes/auth.go
    - internal/cloudclaude/errcodes/disk.go
    - internal/cloudclaude/errcodes/explanations.go
    - internal/cloudclaude/errcodes/explanations_test.go
    - cmd/cloud-claude/explain.go
    - cmd/cloud-claude/explain_test.go
  modified:
    - internal/cloudclaude/errcodes/codes.go
    - internal/cloudclaude/errcodes/codes_test.go
    - cmd/cloud-claude/main.go

key-decisions:
  - "MOUNT_AUTO_DOWNGRADED 从 ExplainExempt 移到 ExtendedExplanations（Severity=Warn 与 TestExplainExemptOnlyInformational 断言矛盾，Rule 1）"
  - "NET_EGRESS_IP_DRIFT 登记到 errcodes/auth.go 的 init（避免新建独立 network.go，与 auth/gateway 同语义组）"
  - "AUTH_OAUTH_REFRESH_FAILED.NextAction 与 net.go NET_OAUTH_EXPIRED 字面量保持一致（在容器内运行 cloud-claude exec claude login 重新登录）"
  - "STATE_VOLUME_IN_USE_001 字面量与 Phase 33 admin_claude_accounts.go 硬编码逐字符一致（D-27 兼容已部署 frontend）"
  - "ExtendedExplanations 实际注册 38 条（11 MOUNT + 11 SESSION/NET + 16 Phase 34 新），超 plan 下限 30"

patterns-established:
  - "Pattern: 错误码域文件 — 每个 DOMAIN_ 前缀独立 .go 文件 + init() 注册，文案 verbatim 守恒"
  - "Pattern: explain CLI — cobra ExactArgs(1) + 三段输出 + os.Exit(exitConfigError) 未找到"
  - "Pattern: explanation 测试 — 遍历 Registry + ExplainExempt 双查 + utf8.RuneCountInString ≥ 200 强约束"
  - "Pattern: registerExplanation 防御 — 空串 panic + 重复 panic（与 MustRegister 同语义）"

requirements-completed: [REQ-F8-A, REQ-F8-B, REQ-F8-C]

# Metrics
duration: 10min
completed: 2026-04-21
---

# Phase 34 Plan 01: errcodes-explain Summary

**8 域闭合错误码 Registry（42 条）+ rustc-style cloud-claude explain CLI（38 条 ≥ 200 中文字符长说明）+ 4 条覆盖率断言**

## Performance

- **Duration:** ≈ 10 min（580s）
- **Started:** 2026-04-21T10:05:21Z
- **Completed:** 2026-04-21T10:15:01Z
- **Tasks:** 8 / 8
- **Files modified:** 12（9 新建 + 3 修改）

## Accomplishments

- 17 条新 Code 字面量补全 Phase 31/32/33 散落落码后遗症（STATE×3 / SYSTEM×4 / SSH×2 / AUTH×4 / NET 扩展×1 / DISK×3）
- 5 个域注册文件（state/system/ssh/auth/disk.go）按 mount.go 模板逐字符登记
- ExtendedExplanations 38 条长说明 + ExplainExempt 4 条豁免，最终 Registry 42 条全覆盖
- `cloud-claude explain <code>` CLI 子命令上线，对标 `rustc --explain`
- 9 条 errcodes 测试 + 3 条 explain 子进程测试全 PASS（含 8 域闭合断言 / v2.0 lower-case 禁入断言）

## Task Commits

Each task was committed atomically:

1. **Task 1.1: codes.go 追加 17 个 Phase 34 新错误码常量字面量** - `01d9f12` (feat)
2. **Task 1.2: 注册 STATE_* 域 3 条错误码** - `b2fcd24` (feat)
3. **Task 1.3: 注册 SYSTEM_* 域 4 条错误码** - `b421445` (feat)
4. **Task 1.4: 注册 SSH/AUTH/DISK 域 + NET_EGRESS_IP_DRIFT 共 10 条错误码** - `2a03abe` (feat)
5. **Task 1.5: 新建 ExtendedExplanations + ExplainExempt + registerExplanation** - `6ae00c6` (feat)
6. **Task 1.6: 新增 4 条 explanations 断言 + 放宽 Registry 下限到 30** - `6dc4037` (test)
7. **Task 1.7: cloud-claude explain <code> 子命令** - `75ae2d7` (feat)
8. **Task 1.8: explain 子进程级测试 3 条** - `ccd9317` (test)

## Files Created/Modified

### 新建（9 个）

- `internal/cloudclaude/errcodes/state.go` - STATE_* 3 条注册（含 D-27 兼容 STATE_VOLUME_IN_USE_001）
- `internal/cloudclaude/errcodes/system.go` - SYSTEM_* 4 条注册（FUSE / DNS / AppArmor / timeout）
- `internal/cloudclaude/errcodes/ssh.go` - SSH_* 2 条注册（known_hosts 冲突 + sshd 基线漂移）
- `internal/cloudclaude/errcodes/auth.go` - AUTH_* 4 条 + NET_EGRESS_IP_DRIFT
- `internal/cloudclaude/errcodes/disk.go` - DISK_* 3 条（local / container / mutagen bloat）
- `internal/cloudclaude/errcodes/explanations.go` - ExtendedExplanations 38 条 + ExplainExempt 4 条
- `internal/cloudclaude/errcodes/explanations_test.go` - 4 条断言（覆盖 / lower-case / 8 域 / Info-only）
- `cmd/cloud-claude/explain.go` - cloud-claude explain <code> 子命令
- `cmd/cloud-claude/explain_test.go` - 3 条子进程级测试

### 修改（3 个）

- `internal/cloudclaude/errcodes/codes.go` - 常量块追加 17 行 Phase 34 新 Code 字面量
- `internal/cloudclaude/errcodes/codes_test.go` - Registry 下限 15 → 30
- `cmd/cloud-claude/main.go` - AddCommand 追加 newExplainCmd() + DisableFlagParsing switch case "explain"

## Verification

| 检查 | 结果 |
| ---- | ---- |
| `go build ./...` | PASS |
| `go test ./internal/cloudclaude/errcodes/ -count=1` | PASS（9 测试，含 4 新增） |
| `go test ./cmd/cloud-claude/ -run TestExplain_ -count=1` | PASS（3 子进程测试） |
| `len(errcodes.Registry())` | **42** ≥ 42 |
| `len(errcodes.ExtendedExplanations)` | **38** ≥ 30 |
| `len(errcodes.ExplainExempt)` | **4** |
| 8 域闭合断言 `TestAllDomainsClosed` | PASS（MOUNT/SESSION/NET/STATE/SYSTEM/SSH/AUTH/DISK） |
| v2.0 lower-case grep（codes.go + 5 域文件） | empty ✅（PITFALLS C8 守住） |
| 每条非豁免 explanation ≥ 200 中文字符 | PASS（utf8.RuneCountInString 断言） |
| `STATE_VOLUME_IN_USE_001` 字面量在 admin_claude_accounts.go 不变 | PASS（D-27 兼容） |

## Decisions Made

详见 frontmatter `key-decisions`。核心：

1. **MOUNT_AUTO_DOWNGRADED 移出 ExplainExempt**：plan 内部不一致（plan §1.5 列入豁免，但 plan §1.6 的 TestExplainExemptOnlyInformational 拒绝非 Info 码）。按 Rule 1 修复：写长说明加入 ExtendedExplanations，使两份测试一致 PASS。
2. **NET_EGRESS_IP_DRIFT 登记到 auth.go**：plan 明确允许 executor 二选一，选 auth.go 避免跨文件 init 顺序依赖（与 gateway/egress 语义同组）。
3. **AUTH_OAUTH_REFRESH_FAILED.NextAction 复用 NET_OAUTH_EXPIRED 字面量**：满足 RESEARCH §9 第 7 条要求，让用户在两类 OAuth 故障下看到一致的修复指令。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] MOUNT_AUTO_DOWNGRADED 从 ExplainExempt 移到 ExtendedExplanations**

- **Found during:** Task 1.5（写 explanations.go）+ Task 1.6（写 TestExplainExemptOnlyInformational）
- **Issue:** Plan §1.5 把 `MOUNT_AUTO_DOWNGRADED` 列入 ExplainExempt，但其 Severity=SeverityWarn；plan §1.6 的 `TestExplainExemptOnlyInformational` 又断言 ExplainExempt 仅允许 SeverityInfo。两份规范矛盾，必产生测试失败。
- **Fix:** 把 `MOUNT_AUTO_DOWNGRADED` 从 ExplainExempt 移除，并在 ExtendedExplanations 中为它写一条 ≥ 200 字符长说明（"自动 mount 模式下某一层启动失败……"）。最终 ExplainExempt 4 条全部为 SeverityInfo。
- **Files modified:** `internal/cloudclaude/errcodes/explanations.go`
- **Verification:** `TestExplainExemptOnlyInformational` + `TestAllCodesHaveExplanations` 同时 PASS。
- **Committed in:** `6ae00c6`（Task 1.5 commit）

---

**Total deviations:** 1 auto-fixed（1 plan 内部不一致，纯 bug 修复）
**Impact on plan:** 修复使 plan-level 测试套件全部通过；ExplainExempt 数从 5 → 4，ExtendedExplanations 数从 ≥ 30 → 38（更高覆盖率）；Acceptance criteria 仍全部满足（grep MOUNT_APFS_CASE_INSENSITIVE / STATE_LAST_SESSION_MISSING 都命中；registerExplanation 调用数 39 ≥ 31）。

## Issues Encountered

无。所有任务一次性通过 build + test。

## Known Stubs

无。本 plan 是基础设施 + 数据注册，不引入 UI / 数据流 stub。

## Threat Flags

无新增 trust boundary。本 plan 纯 CLI + 注册表，T-34-01-01..05 mitigation 全部 verbatim 落地（Lookup map-only / explanations 无 token 字面量 / D-27 STATE_VOLUME_IN_USE_001 字面量守恒）。

## TDD Gate Compliance

本 plan frontmatter `type: execute`（非 tdd），无 RED/GREEN gate 强制要求；但 Task 1.6 / 1.8 的测试文件与 Task 1.5 / 1.7 的实现文件分别独立 commit，事实上呈现"实现 → 测试"的等价分离形态。

## Carry-over for Plan 02 / Plan 03

- **可用错误码字面量清单**：Plan 02 doctor framework 在每个 check 命中时直接调 `errcodes.Format(errcodes.<CODE>, args...)` 输出统一两段（已通过 TestFormat_Render 锁定模板）。
- **--fix 5 类修复字面量**（Plan 03）：
  - mount fix → `SYSTEM_FUSE_RESIDUAL_MOUNT` + `cloud-claude doctor mount --fix`
  - network fix → `SYSTEM_DNS_RESOLVE_FAILED` + `cloud-claude doctor network --fix`
  - ssh fix → `SSH_KNOWN_HOSTS_CONFLICT` + `cloud-claude doctor ssh --fix`
  - auth fix → `AUTH_TOKEN_EXPIRED` + `cloud-claude doctor auth --fix`
  - disk fix → `DISK_MUTAGEN_DATA_BLOAT` + `mutagen daemon stop && rm -rf ~/.cloud-claude/mutagen/sessions/`
- **未覆盖到 ExtendedExplanations 的既有 Code**：0（除 4 条 ExplainExempt 外全部覆盖；Plan 02 / 03 新增 Code 时务必同步注册说明，TestAllCodesHaveExplanations 会强约束）。
- **explain --verbose RelatedChecks 扩展点**：保留在 explain.go runExplain 末尾，由 Plan 02 doctor 维度落地后回填。

## Next Phase Readiness

- ✅ Wave 1 基础设施完成；Wave 2 doctor framework 可立即开工，所有错误码字面量 + 长说明 + CLI 命令就绪
- ✅ REQ-F8-A / REQ-F8-B / REQ-F8-C 三项需求闭合
- ✅ ROADMAP §Phase 34 Success Criteria SC#1（Registry 唯一/中文/next_action/无前缀冲突）+ SC#8（cloud-claude explain CLI）全部交付
- ⚠️ Plan 02 / 03 新增 Code 时务必同步注册 ExtendedExplanations，否则 CI TestAllCodesHaveExplanations 会立即失败（设计如此）

## Self-Check

- [x] `internal/cloudclaude/errcodes/state.go` exists ✅
- [x] `internal/cloudclaude/errcodes/system.go` exists ✅
- [x] `internal/cloudclaude/errcodes/ssh.go` exists ✅
- [x] `internal/cloudclaude/errcodes/auth.go` exists ✅
- [x] `internal/cloudclaude/errcodes/disk.go` exists ✅
- [x] `internal/cloudclaude/errcodes/explanations.go` exists ✅
- [x] `internal/cloudclaude/errcodes/explanations_test.go` exists ✅
- [x] `cmd/cloud-claude/explain.go` exists ✅
- [x] `cmd/cloud-claude/explain_test.go` exists ✅
- [x] commit `01d9f12` exists ✅
- [x] commit `b2fcd24` exists ✅
- [x] commit `b421445` exists ✅
- [x] commit `2a03abe` exists ✅
- [x] commit `6ae00c6` exists ✅
- [x] commit `6dc4037` exists ✅
- [x] commit `75ae2d7` exists ✅
- [x] commit `ccd9317` exists ✅

## Self-Check: PASSED

---

*Phase: 34-cloud-claude-doctor-v3*
*Completed: 2026-04-21*
