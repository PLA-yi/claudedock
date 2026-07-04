---
phase: 37-e2e-uat
plan: "02"
subsystem: mount
tags: [cold-promoter, inotify, last-session, mergerfs, sshfs, promotion]

# Dependency graph
requires:
  - phase: 37-e2e-uat
    plan: "01"
    provides: ColdPromoter 核心引擎（inotify watcher + PromotionEngine + Stats/Wait API）
  - phase: 36
    provides: OversizedFiles 字段模式（omitempty + schema_version=1），单文件熔断
provides:
  - LastSessionSnapshot 新增 PromotionCount/PromotionBytes/PromotionFailedCount 三个字段
  - tryModeReal Full 路径集成 ColdPromoter（mergerfs ready 后启动 + cleanup LIFO + stats flush）
  - CLOUD_CLAUDE_NO_PROMOTION 环境变量控制（=1 跳过 watcher 启动）
  - 残留 PID 清理（每次 mount 入口清理上次会话的 cold-promoter 进程）
affects: [37-e2e-uat 后续 plans, doctor mount 维度]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "omitempty + schema_version=1 不变：promotion 统计字段为零值时不出现在 last-session.json 中"
    - "stats flush before writeLastSession：promoter.Stats() 在 tryModeReal 返回前刷入 snapshot"
    - "cleanup LIFO: promoterCancel → promoter.Wait → cancel watcher → merge → sshfs → hot_sync"

key-files:
  created: []
  modified:
    - internal/claudedock/last_session.go - LastSessionSnapshot 新增 3 个 promotion 字段
    - internal/claudedock/mount_strategy.go - tryModeReal Full 路径 ColdPromoter 集成
    - internal/claudedock/last_session_test.go - promotion 字段 omitempty 测试

key-decisions:
  - "promotion stats 在 tryModeReal 返回前刷入 snapshot（writeLastSessionWarn 之前），此时 promoter 刚启动统计为 0——plan 明确接受此为 mount 就绪时的快照语义"
  - "PID 残留清理无条件执行（不受 CLOUD_CLAUDE_NO_PROMOTION 影响），确保上次 Full 模式会话的 rogue promoter 被终止"
  - "CLOUD_CLAUDE_NO_PROMOTION=1 时 promoter 变量保持 nil，cleanup 闭包通过 if promoter != nil 守卫跳过 Wait()"

patterns-established:
  - "Phase 37 promotion 统计字段模式：与 Phase 36 OversizedFiles 字段一致——omitempty + schema_version=1 不变"
  - "tryModeReal snapshot 注入模式：通过 *LastSessionSnapshot 参数传递，非 nil 时填充 promotion 统计"

requirements-completed: [REQ-MOUNT-V31-08, REQ-MOUNT-V31-12, REQ-MOUNT-V31-13]

# Metrics
duration: 15min
completed: 2026-04-24
---

# Phase 37 Plan 02: ColdPromoter 集成到 mount 生命周期 + 统计持久化

**ColdPromoter 挂入 tryModeReal Full 路径：mergerfs 就绪后启动，cleanup LIFO 回收，LastSessionSnapshot 新增 3 个 promotion 字段，CLOUD_CLAUDE_NO_PROMOTION=1 完全跳过**

## Performance

- **Duration:** 15min
- **Started:** 2026-04-24T04:50:00Z
- **Completed:** 2026-04-24T04:55:42Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- LastSessionSnapshot 新增 PromotionCount / PromotionBytes / PromotionFailedCount 三个 omitempty 字段
- tryModeReal Full 路径集成 ColdPromoter：mergerfs ready → NewColdPromoter → go promoter.Run(ctx)，cleanup LIFO 顺序为 promoterCancel → promoter.Wait → cancel watcher → merge → sshfs → hot_sync
- CLOUD_CLAUDE_NO_PROMOTION=1 时 promoter 变量保持 nil，cleanup 闭包通过 nil guard 跳过 Wait()，snapshot promotion 字段保持零值不写入 JSON
- 每次 mount 入口无条件清理上次残留的 cold-promoter PID 文件和进程

## Task Commits

Each task was committed atomically:

1. **Task 1: 扩展 last-session.json schema** - `f905cf2` (feat)
2. **Task 2: 集成 ColdPromoter 到 tryModeReal Full 路径** - `1c2a0f9` (feat)
3. **Task 3: 验证 CLOUD_CLAUDE_NO_PROMOTION 行为** - 纯验证，无代码变更

**Plan metadata:** TBD (docs commit after SUMMARY)

## Files Created/Modified
- `internal/claudedock/last_session.go` - LastSessionSnapshot struct 末尾追加 3 个 promotion 字段（omitempty + schema_version=1 不变）
- `internal/claudedock/last_session_test.go` - 新增 TestLastSession_PromotionFields_Omitempty 验证空值不在 JSON 中出现
- `internal/claudedock/mount_strategy.go` - tryModeReal Full 路径集成 ColdPromoter（PID cleanup + NO_PROMOTION gate + promoter start + cleanup LIFO + stats flush）；新增 import strconv/strings/path/filepath；tryMode/tryModeReal 签名新增 *LastSessionSnapshot 参数

## Decisions Made
1. **promotion stats 刷新时机**：在 tryModeReal 返回前刷入 snapshot（writeLastSessionWarn 之前）。此时 promoter 刚启动，统计为 (0, 0, 0)——plan 明确接受此为 mount 就绪时的快照语义，与 Phase 36 OversizedFiles 在 writeLastSessionWarn 之前赋值的模式一致
2. **PID 残留清理**：无条件执行（不检查 CLOUD_CLAUDE_NO_PROMOTION），确保上次 Full 模式会话的 rogue promoter 被终止
3. **snapshot 参数注入**：通过 tryMode/tryModeReal 签名新增 `*LastSessionSnapshot` 参数传递，tryModeWithHooks（测试路径）不接收此参数，保持向后兼容

## Deviations from Plan

None - plan executed exactly as written. 验收标准中 `grep -c "snapshot.PromotionCount"` 输出为 2（1 处代码 + 1 处文档注释），而非 plan 预期的 1——代码逻辑完全正确，文档注释为函数签名说明的一部分。

## Issues Encountered

None.

## CLOUD_CLAUDE_NO_PROMOTION 手动验证指令

在 Linux 容器内执行以下验证：

```bash
# 验证 watcher 不启动
CLOUD_CLAUDE_NO_PROMOTION=1 claudedock --mount-mode full 2>&1 | grep -c "cold-promoter"
# 期望输出 0

# 验证 promotion_count 不出现在 last-session.json
cat ~/.claudedock/last-session.json | jq '.promotion_count'
# 期望 null

# 验证 PID 文件不存在（NO_PROMOTION 时不写入）
ls ~/.claudedock/cold-promoter.pid
# 期望 No such file or directory
```

## Next Phase Readiness
- ColdPromoter 已完整集成到 mount 生命周期，Plan 03 可直接使用 Stats() API 进行 doctor 自检
- promotion 统计字段已就位，doctor mount 维度可读取 last-session.json 的 promotion_count/promotion_bytes/promotion_failed_count
- 剩余计划：37-03 / 37-05

---
*Phase: 37-e2e-uat*
*Plan completed: 2026-04-24*

## Self-Check: PASSED

All files and commits verified:
- `internal/claudedock/last_session.go` — FOUND
- `internal/claudedock/mount_strategy.go` — FOUND
- `internal/claudedock/last_session_test.go` — FOUND
- `.planning/phases/37-e2e-uat/37-02-SUMMARY.md` — FOUND
- Commit `f905cf2` — FOUND
- Commit `1c2a0f9` — FOUND
