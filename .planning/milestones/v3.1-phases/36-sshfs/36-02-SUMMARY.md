---
phase: 36-sshfs
plan: "02"
subsystem: cli
tags: [config, last-session, hot-sync, oversized-files, schema]

# Dependency graph
requires:
  - phase: 32-ssh-tmux
    provides: "LastSessionSnapshot schema_version=1 + omitempty 追加模式（D-27）"
provides:
  - "Config.HotSyncMaxFileMB int 字段 + EffectiveHotSyncMaxFileMB() accessor（默认 50MB）"
  - "OversizedFile struct（Path string + SizeBytes int64）"
  - "LastSessionSnapshot.OversizedFiles []OversizedFile omitempty 字段"
affects: [36-03-PLAN, 36-06-PLAN, hot_sync, mount_strategy, doctor]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Config 通过零值 + Effective*() accessor 实现"默认值"与"用户覆盖"双模"
    - "LastSessionSnapshot 通过 omitempty + schema_version=1 不变实现纯加法演进"

key-files:
  created: []
  modified:
    - internal/claudedock/config.go
    - internal/claudedock/last_session.go
    - internal/claudedock/last_session_test.go

key-decisions:
  - "HotSyncMaxFileMB 零值/负值统一走 defaultHotSyncMaxFileMB=50，不在 Validate() 强校验上限（D-04）"
  - "OversizedFile.Path 使用 cwd 相对路径以避免泄露家目录结构（T-36-02-02 mitigate）"
  - "OversizedFiles 使用 omitempty + schema_version=1 不变，旧 last-session.json 反序列化零破坏"

patterns-established:
  - "Hot-sync 类配置统一走 'Effective<Field>' accessor，禁止调用方裸读 Config 字段"
  - "Phase 36 后续 plan（03+）写 OversizedFiles 时统一使用相对路径，避免与威胁矩阵 T-36-02-02 冲突"

requirements-completed: [REQ-MOUNT-V31-02, REQ-MOUNT-V31-03]

# Metrics
duration: 4 min
completed: 2026-04-23
---

# Phase 36 Plan 02: 单文件熔断数据契约 Summary

**Config 新增 HotSyncMaxFileMB(默认 50) accessor，LastSessionSnapshot 新增 OversizedFiles 数组与 OversizedFile struct，schema_version=1 不变，3 条序列化测试 PASS。**

## Performance

- **Duration:** ~4 min
- **Started:** 2026-04-23T11:22:43Z
- **Completed:** 2026-04-23T11:26:08Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- `Config` 暴露 `HotSyncMaxFileMB int` yaml 字段（`hot_sync_max_file_mb,omitempty`）+ `EffectiveHotSyncMaxFileMB()` accessor，零值/负值返回常量 `defaultHotSyncMaxFileMB=50`，正值返回自身。
- `LastSessionSnapshot` 末尾追加 `OversizedFiles []OversizedFile` 字段（json `oversized_files,omitempty`），并新增 `OversizedFile` struct（`path`/`size_bytes`），`schema_version=1` 不变。
- `WriteLastSession`/`LoadLastSession` 无需改动——`encoding/json` 自动处理新字段，向后兼容旧 last-session.json 文件。
- 新增 3 条序列化测试（`Roundtrip` / `OmitemptyEmpty` / `OmitemptyNil`）全部 PASS，既有 Phase 32 D-27 序列化测试零回归。

## Task Commits

Each task was committed atomically:

1. **Task 1: Config.HotSyncMaxFileMB 字段 + EffectiveHotSyncMaxFileMB() accessor** — `a8c3cb5` (feat)
2. **Task 2 RED: 序列化失败测试（编译失败为预期 RED）** — `cdeebb5` (test)
3. **Task 2 GREEN: OversizedFiles 字段 + OversizedFile struct** — `b1bdbdd` (feat)

**Plan metadata:** 本 SUMMARY + STATE/ROADMAP 更新随后续 docs commit 一并提交。

_Note: Task 2 标记 `tdd="true"`，按 RED/GREEN 拆分为独立 commit；无需 REFACTOR（实现简洁、无清理空间）。_

## Files Created/Modified
- `internal/claudedock/config.go` — `Config` 末尾追加 `HotSyncMaxFileMB`，`EffectiveProxyCommands` 之后追加 `defaultHotSyncMaxFileMB=50` 常量与 `EffectiveHotSyncMaxFileMB()` accessor。
- `internal/claudedock/last_session.go` — `LastSessionSnapshot` 末尾追加 `OversizedFiles []OversizedFile` 字段，`DowngradeStep` 之后新增 `OversizedFile` struct。
- `internal/claudedock/last_session_test.go` — 末尾追加 `TestLastSession_OversizedFiles_Roundtrip` / `_OmitemptyEmpty` / `_OmitemptyNil` 三条测试。

## Decisions Made
- HotSyncMaxFileMB 零值/负值统一返回 50：避免 yaml 缺省值场景需要用户感知；超大值（如 1000）由 doctor disk 维度间接发现，不在 Validate() 强校验（与 D-04 一致）。
- OversizedFile 字段命名采用 `path` + `size_bytes`（snake_case JSON 与既有 `schema_version`/`reason_code` 保持一致），Go 字段使用 `Path` + `SizeBytes` 与 doctor 输出友好。
- 新字段位置：`OversizedFiles` 紧跟 Phase 32 D-27 三字段之后（保留 omitempty 同区段），`OversizedFile` struct 紧跟 `DowngradeStep`（保持"snapshot 字段 + 子结构"块状布局）。

## Deviations from Plan

None — plan 完全按 `<action>` 字面量执行：Config 字段名/yaml tag/accessor 实现，OversizedFile 字段名/json tag/struct 位置，3 条测试函数名/断言全部一字不差。

唯一可记的形式偏差是 Task 1 acceptance 中 `grep -c "HotSyncMaxFileMB"` 的预期值 `3`：实际 grep 命中 7 次（含 `defaultHotSyncMaxFileMB`、accessor 注释、签名、if 分支、两条 return）。代码与 plan `<action>` 块一致，acceptance 数字仅为估算偏差，不构成实现层偏差，无需 Rule 1 修订。

## Issues Encountered
- 无。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- **Plan 03（hot_sync 熔断逻辑）** 可直接读 `cfg.EffectiveHotSyncMaxFileMB() * 1024 * 1024` 注入 `HotSyncConfig.MaxFileBytes`，并把扫描得到的超阈文件以 `[]OversizedFile` 形式返回（路径必须为 cwd 相对路径，威胁 T-36-02-02 要求）。
- **Plan 03（mount_strategy）** 可在写 last-session.json 时把 hot status 中的 `OversizedFiles` 直接赋给 `snapshot.OversizedFiles`，无需再做 schema 升级。
- 本 plan 无 stub、无新增安全面、无外部依赖阻塞，可推进到 Plan 03 / Plan 04。

## Threat Surface
本 plan 未引入任何新网络端点 / 鉴权路径 / 文件 I/O / schema 跨 trust boundary 变更。`OversizedFile.Path` 由后续 plan 写入相对路径（threat T-36-02-02 mitigate 落实在 Plan 03 写端而非本 plan 数据契约层）。无需 `Threat Flags` 章节。

## Known Stubs
无 stub。`OversizedFiles` 字段由后续 Plan 03 实际写入；本 plan 仅落地数据契约 + 序列化测试，符合 Phase 36 wave 1 并行划分。

## Self-Check: PASSED

- 文件存在：
  - `internal/claudedock/config.go` FOUND
  - `internal/claudedock/last_session.go` FOUND
  - `internal/claudedock/last_session_test.go` FOUND
  - `.planning/phases/36-sshfs/36-02-SUMMARY.md` FOUND（本文件）
- 提交存在：
  - `a8c3cb5` FOUND（Task 1 feat）
  - `cdeebb5` FOUND（Task 2 RED test）
  - `b1bdbdd` FOUND（Task 2 GREEN feat）
- 验证：
  - `go build ./...` PASS
  - `go test ./internal/claudedock/...` 全包 PASS（无回归）
  - `go test -run TestLastSession_OversizedFiles -v` 3 条全 PASS

---
*Phase: 36-sshfs*
*Completed: 2026-04-23*
