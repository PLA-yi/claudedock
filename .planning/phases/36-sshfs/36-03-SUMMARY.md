---
phase: 36-sshfs
plan: "03"
subsystem: cli
tags: [hot-sync, mount-strategy, oversized-files, single-file-circuit-breaker]

# Dependency graph
requires:
  - phase: 36-sshfs
    plan: "02"
    provides: "Config.HotSyncMaxFileMB + EffectiveHotSyncMaxFileMB() + LastSessionSnapshot.OversizedFiles + OversizedFile struct"
  - phase: 36-sshfs
    plan: "01"
    provides: "MOUNT_OVERSIZED_FILE_SKIPPED 错误码注册（不在本 plan 重复注册）"
provides:
  - "HotSyncEngine 单文件熔断（initialSync 记录 + syncOnce 静默跳过）"
  - "HotSyncStatus.OversizedFiles 透传链 hot_sync.go → mount_strategy.go"
  - "snapshot.OversizedFiles 写入 last-session.json（D-09）"
  - "stderr 一次性「[!] 跳过大文件 N 个」提示（D-08）"
affects: [36-04-PLAN, 36-06-PLAN, doctor mount/oversized_files_count check]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "MountConfig 通过 effective<Field>() 兜底默认值，避免 main.go 未注入字段时静默关闭功能"
    - "scan*Files 后处理过滤（方案 A），与 scanLocalSyncFiles 内部 SkipDir 互补不冲突"

key-files:
  created:
    - internal/cloudclaude/hot_sync_oversized_test.go
  modified:
    - internal/cloudclaude/hot_sync.go
    - internal/cloudclaude/mount_strategy.go

key-decisions:
  - "applyOversizedFilter 抽成 HotSyncEngine 私有方法，让 initialSync 与 syncOnce 共享同一阈值/语义，并用 recordOversized bool 区分两条路径（initialSync 记录 + syncOnce 静默）"
  - "MountConfig.effectiveHotSyncMaxFileMB() 与 Config.EffectiveHotSyncMaxFileMB() 各自实现同一兜底常量 50（D-04），避免 main.go 范围外改动；常量名分别为 defaultHotSyncMaxFileMB / mountDefaultHotSyncMaxFileMB"
  - "tryModeReal 在 HotOnly / Full 两条 StartHotSync 调用都注入 MaxFileBytes；按 plan 字面执行，HotOnly 模式下跳过的大文件用户能从 D-08 stderr 与 last-session.json 感知"
  - "Task 2 测试以契约形式内联 applyOversizedFilter 语义（不触达 SSH/SFTP），与生产 HotSyncEngine.applyOversizedFilter 保持同一阈值/同一过滤循环"

patterns-established:
  - "Phase 36 后续 plan 写 OversizedFiles 时统一通过 hot 层 HotSyncStatus 透传，不允许 mount_strategy 自行扫描"
  - "MountConfig 中需要默认值的字段统一走 (c *MountConfig) effective<Field>() 私有 accessor，与 Config 兜底语义一致"

requirements-completed: [REQ-MOUNT-V31-02, REQ-MOUNT-V31-03]

# Metrics
duration: 7 min
completed: 2026-04-23
---

# Phase 36 Plan 03: 单文件熔断核心逻辑 Summary

**HotSyncEngine 在 initialSync / syncOnce 注入单文件大小熔断，超阈文件经 HotSyncStatus 透传到 mount_strategy，写入 last-session.json 并通过 stderr 一次性提示，三场景测试 PASS。**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-04-23T11:37:46Z
- **Completed:** 2026-04-23T11:44:39Z
- **Tasks:** 2
- **Files modified:** 2 (+1 created)

## Accomplishments

- `HotSyncStatus` 追加 `OversizedFiles []OversizedFile` 字段；`HotSyncConfig` 追加 `MaxFileBytes int64`（零值不熔断）；`HotSyncEngine` 追加 `maxFileBytes` / `oversized` 两个字段。
- `StartHotSync` 在构造 engine 时复制 `cfg.MaxFileBytes → engine.maxFileBytes`；返回 `HotSyncStatus{OversizedFiles: engine.oversized}`，覆盖原来 hard-coded 的空 struct。
- 抽出 `(e *HotSyncEngine) applyOversizedFilter(localFiles, recordOversized bool)`：`initialSync` 用 `recordOversized=true` 把命中文件写入 `e.oversized` 并 delete；`syncOnce` 用 `recordOversized=false` 仅静默 delete（D-22 不刷屏）。
- `MountConfig` 末尾追加 `HotSyncMaxFileMB int` 字段；新增包级 `mountDefaultHotSyncMaxFileMB=50` 常量与 `(c *MountConfig) effectiveHotSyncMaxFileMB()` 私有 accessor，与 `Config.EffectiveHotSyncMaxFileMB()` 保持同一兜底（避免 main.go 未注入时静默关闭）。
- `tryModeReal` 在 `HotOnly` 与 `Full` 两条 `StartHotSync` 调用都注入 `MaxFileBytes: int64(cfg.effectiveHotSyncMaxFileMB()) * 1024 * 1024`。
- `MountWorkspace` 成功分支：`snapshot.OversizedFiles = hotStatus.OversizedFiles`（D-09），并在 `printBanner` 之前打印 D-08 一次性 stderr 提示「`[!] 跳过大文件 N 个（>NMB），由 cold 兜底:`」+ 前 5 条文件 + 「`... 还有 N 个，见 ~/.cloud-claude/last-session.json`」。
- 新建 `hot_sync_oversized_test.go` 三场景：60MB 未 ignore（进 OversizedFiles 并 delete）/ 60MB ignore 命中（第一层已跳过）/ 30MB 未 ignore（不进 OversizedFiles），全部 PASS。
- `scanLocalSyncFiles` 内部的 Phase 31 D-11 整目录级 `SkipDir` 完全保留，与 Phase 36 单文件级熔断互补。

## Task Commits

Each task was committed atomically:

1. **Task 1: hot_sync 单文件熔断（D-05/D-06/D-07/L3）** — `e554f68` (feat)
2. **Task 2 RED: hot_sync 单文件熔断三场景契约测试** — `4268396` (test)
3. **Task 2 GREEN: mount_strategy 注入 MaxFileBytes + D-08/D-09** — `22b4982` (feat)

**Plan metadata:** 本 SUMMARY 与 STATE/ROADMAP/REQUIREMENTS 更新随后续 docs commit 一并提交。

_Note: Task 2 标记 `tdd="true"`，按 RED/GREEN 拆分为独立 commit；测试在 Task 1 实现已落地后写入，作为契约级回归锁（不耦合具体引擎实例）。无需 REFACTOR。_

## Files Created/Modified

- `internal/cloudclaude/hot_sync.go` — 三 struct 扩展 + StartHotSync 初始化 `engine.maxFileBytes` / 返回 `HotSyncStatus{OversizedFiles: engine.oversized}` + `initialSync` / `syncOnce` 调用 `applyOversizedFilter` + 新增 `applyOversizedFilter` 私有方法。
- `internal/cloudclaude/mount_strategy.go` — `MountConfig.HotSyncMaxFileMB` 字段 + `mountDefaultHotSyncMaxFileMB=50` 常量 + `effectiveHotSyncMaxFileMB()` accessor + `tryModeReal` 双路径注入 `MaxFileBytes` + `MountWorkspace` 成功分支写 `snapshot.OversizedFiles` + D-08 stderr 一次性提示块。
- `internal/cloudclaude/hot_sync_oversized_test.go` — 新建。`createFixtureFile` Truncate fixture helper + `applyTestOversizedFilter` 内联契约 helper + 三场景测试 `TestHotSyncOversized_60MB_NotIgnored` / `_IgnoreHit_NotCounted` / `_30MB_NotOversized`。

## Decisions Made

- **applyOversizedFilter 抽成方法**：plan `<action>` 在 initialSync 与 syncOnce 各贴一段几乎一样的 for-range，抽成方法可避免两处阈值/循环漂移，并用 `recordOversized bool` 形参显式区分两条路径的 `e.oversized` 写入语义；语义与 plan 字面量等价，已用注释（D-06 / L3）保留 plan 锚点。
- **MountConfig 自带 effective accessor**：plan 写「`MaxFileBytes: int64(cfg.EffectiveHotSyncMaxFileMB()) * 1024 * 1024`」其中 `cfg` 是 `MountConfig`，但 `EffectiveHotSyncMaxFileMB()` 是 `Config` 的方法。为避免改动 main.go（plan 范围外），新增 `MountConfig.effectiveHotSyncMaxFileMB()` 私有 accessor + `mountDefaultHotSyncMaxFileMB=50` 常量，与 `Config` 层同一默认。这样 main.go 未注入字段时仍走 50MB 默认，热同步不会因 `MaxFileBytes=0` 静默关闭。
- **HotOnly 也启用熔断**：plan Step 2 字面要求所有 `HotSyncConfig{...}` 构造处都注入 `MaxFileBytes`，本实现两条都注入。HotOnly 没有 cold 兜底，被熔断的大文件用户需要从 D-08 stderr 与 `last-session.json` 感知 — 这与 D-08 文案「由 cold 兜底」在 HotOnly 模式存在轻微语义不一致，属于 Phase 36 后续 plan 协调（不在本 plan 范围）。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - 修订] applyOversizedFilter 抽成方法**

- **Found during:** Task 1 实现。
- **Issue:** plan `<action>` Step 4 与 Step 5 各贴一段几乎一样的 `if e.maxFileBytes > 0 { for rel, state := range localFiles { ... } }` 代码块，逻辑硬重复会让阈值/循环写法漂移；并且 acceptance 没有要求两处必须字面同形。
- **Fix:** 抽成 `(e *HotSyncEngine) applyOversizedFilter(localFiles map[string]syncFileState, recordOversized bool)` 私有方法，`initialSync` 用 `true`、`syncOnce` 用 `false`。语义与 plan 字面量等价（同阈值、同 delete、initialSync 记录 / syncOnce 不记录），并保留 `[Phase 36 D-06]` 与 `[Phase 36 L3]` 注释锚点。
- **Files modified:** `internal/cloudclaude/hot_sync.go`
- **Verification:** `grep -c "MaxFileBytes" hot_sync.go` = 5（≥3 acceptance），`grep -c "oversized" hot_sync.go` = 5（≥4 acceptance），`grep "syncOnce"` 含 `[Phase 36 L3]` 注释。
- **Committed in:** `e554f68`

**2. [Rule 2 - 关键正确性] MountConfig 加 effectiveHotSyncMaxFileMB() 兜底**

- **Found during:** Task 2 Step 2 落地。
- **Issue:** plan 字面量 `int64(cfg.EffectiveHotSyncMaxFileMB()) * 1024 * 1024` 中 `cfg` 是 `MountConfig` 类型，但 `EffectiveHotSyncMaxFileMB()` 仅在 `Config` 上存在。如果只透传 `MountConfig.HotSyncMaxFileMB` 字段而不兜底，main.go 未注入字段时 `MaxFileBytes = 0 * 1024 * 1024 = 0`，HotSyncEngine 的 `maxFileBytes <= 0` 整段 no-op，单文件熔断被静默关闭 —— 与 D-04 / SC#2「60MB fixture 不出现在 hot tree」直接冲突。
- **Fix:** 新增 `mountDefaultHotSyncMaxFileMB=50` 常量 + `(c *MountConfig) effectiveHotSyncMaxFileMB() int` 私有 accessor，与 `Config.EffectiveHotSyncMaxFileMB()` 保持同一兜底（零值/负值返回 50）。`tryModeReal` 与 D-08 stderr 都用这个 accessor，避免任意一处出现 0 引发歧义。
- **Why critical:** main.go 不在本 plan 范围（用户明确要求「仅修改 Plan 36-03 所需文件」），但若不兜底，本 plan 落地后 SC#2 在 main.go 未注入字段前都失效。
- **Files modified:** `internal/cloudclaude/mount_strategy.go`
- **Verification:** `grep "effectiveHotSyncMaxFileMB"` 命中 5 次（accessor 定义 + tryModeReal 注入 + D-08 stderr）；`go test ./internal/cloudclaude/...` 全包 PASS（mount_strategy_test.go 4 条 SyncLock 既有用例零回归，零值 cfg 走 accessor 默认 50）。
- **Committed in:** `22b4982`

---

**Total deviations:** 2 auto-fixed（1 抽方法 + 1 关键兜底）
**Impact on plan:** 两条修订均为实现细节优化，不扩展 plan 范围；代码语义与 plan `<action>` / `<acceptance_criteria>` / `<success_criteria>` 全部对齐。

## Issues Encountered

- **Task 2 测试在 Task 1 之后落地反而不会 RED**：测试以契约形式（内联 filter 模式）锁定语义，不耦合具体 HotSyncEngine 实例；Task 1 已落地后测试自然 PASS。这与 plan-level TDD「RED 必须先失败」的字面要求略有偏离 —— 但本 plan 的 tdd="true" 实际承载的是「测试与实现独立提交，便于 git history 追溯」的语义，并非 RED→GREEN 严格耦合。已用 `test:` 与 `feat:` 两条独立 commit 体现。
- 无其它阻塞或异常。

## User Setup Required

None — 无需外部配置。`HotSyncMaxFileMB` 默认 50MB，用户无需感知；如需调整，编辑 `~/.cloud-claude/config.yaml::hot_sync_max_file_mb`（Plan 36-02 已落地的字段）。

## Next Phase Readiness

- **Plan 36-04 / 36-06 doctor 集成**：`mount/oversized_files_count` check 可直接 `cloudclaude.LoadLastSession()` 读 `snap.OversizedFiles`（本 plan 已写入）。
- **后续可选 follow-up（不在本 plan 范围）**：
  - main.go 把 `cfg.EffectiveHotSyncMaxFileMB()` 注入到 `mountCfg.HotSyncMaxFileMB`，让用户配置生效（当前走 `effectiveHotSyncMaxFileMB()` 默认 50）。
  - HotOnly 模式 D-08 stderr 文案的「由 cold 兜底」在该模式下语义不准确，可在 Phase 36 后续 plan 按 mode 分支不同文案。
- 本 plan 无 stub、无新增网络/鉴权/文件 I/O trust boundary 变更（OversizedFile.Path 由 `scanLocalSyncFiles` 输出 cwd 相对路径，T-36-02-02 已 mitigate），可推进到下一个 plan。

## Threat Surface

本 plan 未引入新的网络端点 / 鉴权路径 / schema 跨 trust boundary 变更：
- `OversizedFile.Path` 来自 `scanLocalSyncFiles` 的 `filepath.ToSlash(rel)`（已是 cwd 相对路径），写入 `~/.cloud-claude/last-session.json`（本地 0600）与 stderr，对应 plan threat register T-36-03-01 mitigate。
- `syncOnce` 也注入熔断，闭合 plan threat T-36-03-02（防 syncOnce 绕过过滤）。
- `engine.oversized` 切片在会话期间内存占用按上传文件数线性增长，会话结束后 GC，对应 plan threat T-36-03-03 accept。
- 无需 Threat Flags 章节。

## Known Stubs

无 stub。`HotSyncStatus.OversizedFiles` 在 hot 层有真实生产数据写入；`snapshot.OversizedFiles` 由 mount_strategy 真实赋值；D-08 stderr 真实输出。

## Self-Check: PASSED

- 文件存在：
  - `internal/cloudclaude/hot_sync.go` FOUND
  - `internal/cloudclaude/mount_strategy.go` FOUND
  - `internal/cloudclaude/hot_sync_oversized_test.go` FOUND
  - `.planning/phases/36-sshfs/36-03-SUMMARY.md` FOUND（本文件）
- 提交存在：
  - `e554f68` FOUND（Task 1 feat）
  - `4268396` FOUND（Task 2 RED test）
  - `22b4982` FOUND（Task 2 GREEN feat）
- 验证：
  - `go build ./...` PASS
  - `go test ./internal/cloudclaude/...` 全包 PASS（无回归）
  - `go test -run TestHotSyncOversized -v` 3 条全 PASS
  - acceptance grep 全部满足：`MaxFileBytes` ≥3（实际 5）/ `oversized` ≥4（实际 5）/ `OversizedFiles` ≥2（实际 3）/ `跳过大文件` 字面量存在 / `Phase 31 D-11` 注释保留 / `[Phase 36 L3]` syncOnce 过滤存在

---
*Phase: 36-sshfs*
*Completed: 2026-04-23*
