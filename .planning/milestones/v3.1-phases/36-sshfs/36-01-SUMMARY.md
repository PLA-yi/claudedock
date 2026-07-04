---
phase: 36-sshfs
plan: "01"
subsystem: cli
tags: [errcodes, explain, mount, testing]

# Dependency graph
requires:
  - phase: 31-cli
    provides: "mount 错误码域与三层文件映射语义基线"
  - phase: 34-claudedock-doctor-v3
    provides: "统一 Registry、ExtendedExplanations 与 explain 子命令框架"
provides:
  - "MOUNT_REQUIRE_GIT_REPO 与 MOUNT_OVERSIZED_FILE_SKIPPED 两条错误码注册"
  - "两条 mount 错误的中文长说明与 explain 子进程回归测试"
affects: [36-04-PLAN, 36-06-PLAN, errcodes, explain]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "错误码按 Code -> MustRegister -> registerExplanation -> explain 测试 的闭环扩展"
    - "explain 子进程测试在每个 go test 进程内编译独立临时二进制，避免陈旧缓存"

key-files:
  created: []
  modified:
    - internal/claudedock/errcodes/codes.go
    - internal/claudedock/errcodes/mount.go
    - internal/claudedock/errcodes/explanations.go
    - cmd/claudedock/explain_test.go

key-decisions:
  - "MOUNT_REQUIRE_GIT_REPO 与 MOUNT_OVERSIZED_FILE_SKIPPED 不加入 ExplainExempt，必须提供完整长说明"
  - "保留 explain 子进程测试，但把编译产物切到进程内临时路径，避免 /tmp 旧二进制污染验收"

patterns-established:
  - "新增 mount 错误码时，同步补齐 Registry、ExtendedExplanations 和 explain 子进程测试"
  - "子进程型 CLI 测试不要复用跨进程持久二进制缓存"

requirements-completed: [REQ-MOUNT-V31-06]

# Metrics
duration: 2 min
completed: 2026-04-23
---

# Phase 36 Plan 01: sshfs Summary

**两条 Phase 36 mount 错误码、对应中文长说明和 explain 子进程回归测试已接入统一 errcodes 注册表。**

## Performance

- **Duration:** 2 min
- **Started:** 2026-04-23T19:14:09+08:00
- **Completed:** 2026-04-23T19:16:53+08:00
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- 注册 `MOUNT_REQUIRE_GIT_REPO` 与 `MOUNT_OVERSIZED_FILE_SKIPPED`，补齐 Phase 36 后续计划可直接复用的错误码基础设施。
- 为两条新错误码写入中文长说明，并确认它们不属于 `ExplainExempt`，会被 `claudedock explain` 直接渲染。
- 扩展 `explain` 子进程测试，锁定两条新错误码的 exit 0 与长说明长度要求。

## Task Commits

Each task was committed atomically:

1. **Task 1: 注册 2 条 Code 常量 + 2 条 MustRegister Entry** - `088b95f` (feat)
2. **Task 2: 注册 2 条 ExtendedExplanations + 扩展 explain 子进程测试** - `d22a42e` (feat)

**Plan metadata:** 本文件将随当前 docs commit 一并提交。

## Files Created/Modified
- `internal/claudedock/errcodes/codes.go` - 新增两条 mount 错误码常量。
- `internal/claudedock/errcodes/mount.go` - 注册两条 mount 错误码的 severity、message 和 next_action。
- `internal/claudedock/errcodes/explanations.go` - 写入两条长说明，实际 rune 计数分别为 `754` 和 `830`。
- `cmd/claudedock/explain_test.go` - 增加两条 explain 子进程回归测试，并修正测试二进制缓存策略。

## Decisions Made
- 两条新错误码都面向用户可见的 mount 失败/告警语义，因此不进入 `ExplainExempt`，而是要求完整的长说明闭环。
- `explain` 测试继续保留真实二进制子进程路径，但编译产物改为当前 `go test` 进程内唯一的临时路径，避免旧 `/tmp` 产物让新错误码测试出现假失败。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] 修复 explain 子进程测试复用陈旧二进制**
- **Found during:** Task 2（注册 2 条 ExtendedExplanations + 扩展 explain 子进程测试）
- **Issue:** 首次运行新增 `TestExplain_Mount*` 时，测试命中了旧的 `/tmp/claudedock-explain-test` 二进制，导致新错误码被误判为 unknown code。
- **Fix:** 把 `buildOnceExplainBin()` 改成每个 `go test` 进程只编译一次新的临时二进制，不再复用跨测试进程残留文件。
- **Files modified:** `cmd/claudedock/explain_test.go`
- **Verification:** `go test ./cmd/claudedock/... -run TestExplain -v`
- **Committed in:** `d22a42e` (part of task commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** 该修复只清除了测试层假失败，不引入额外范围扩张；plan 目标完整达成。

## Issues Encountered
- Task 2 初次验收时，`/tmp` 下的旧 explain 测试二进制导致新错误码没有进入子进程结果；已在同一 task 内修复并复跑通过。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- `36-04-PLAN.md` 可以直接复用 `MOUNT_REQUIRE_GIT_REPO`。
- `36-06-PLAN.md` 可以直接复用 `MOUNT_REQUIRE_GIT_REPO` 与 `MOUNT_OVERSIZED_FILE_SKIPPED` 作为 doctor 输出错误码。
- 本 plan 无 stub、无新增安全面、无外部依赖阻塞，已准备进入 `36-02-PLAN.md`。

## Self-Check: PASSED

- 已确认 `36-01-SUMMARY.md` 文件存在。
- 已确认任务提交 `088b95f` 与 `d22a42e` 都存在于 git 历史中。

---
*Phase: 36-sshfs*
*Completed: 2026-04-23*
