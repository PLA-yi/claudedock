---
phase: 30-entry-api
plan: 01-migration-entry-store
subsystem: database
tags: [postgres, migration, pgx, repository, claude-accounts, entry-api]

requires:
  - phase: 29-v3-worker
    provides: "HostActionRequest.Volumes 与 worker volume mount 基线（本计划在数据层与其对齐，账号维度交给 ResolveClaudeAccountIDForEntry）"

provides:
  - "migration 0014：claude_accounts.persistent_volume_name 列（TEXT, NULL 表示未分配，D-01/D-02/D-10）"
  - "ClaudeAccount.PersistentVolumeName *string 字段映射（NULL/omitempty 语义）"
  - "HostSSHAuth.TemplateImageRef 字段 + GetHostByShortID SELECT 扩展"
  - "Repository.ResolveClaudeAccountIDForEntry(ctx, userID, hostID) 实现 D-05 两阶段解析"
  - "数据层 SQL 包级常量（resolveClaudeAccountByHost/UserFallback/getHostByShortID）供回归断言"

affects:
  - 30-entry-api Plan 02（Entry API host contract，消费 TemplateImageRef 推导 image_version / supports_*）
  - 30-entry-api Plan 03+（Entry HTTP/CLI 响应，消费 ResolveClaudeAccountIDForEntry 填充 claude_account_id）
  - Phase 33（Docker named volume 创建，读取 persistent_volume_name，写回 claude-state-{id}）

tech-stack:
  added: []
  patterns:
    - "SQL 文本提升为包级常量（`const xxxSQL = \`...\``），便于 go test 做文本断言，而非运行真实 Postgres"
    - "reflect + 包级 SQL 常量做数据层契约测试，避免引入 pgxmock/testcontainers 等新依赖"
    - "三态收敛：*string + omitempty 承载 NULL 语义，禁止使用空字符串默认值"

key-files:
  created:
    - internal/store/migrations/0014_claude_account_persistent_volume.sql
    - internal/store/repository/migration_0014_test.go
  modified:
    - internal/store/repository/models.go
    - internal/store/repository/queries.go

key-decisions:
  - "D-02 落地方式：通过 `*string` + `json:...,omitempty` 承载 NULL 语义；migration 不写 DEFAULT ''，也不加 NOT NULL"
  - "D-05 两阶段解析以两条独立 SQL 常量实现而非单次 JOIN：便于在无 DB 的单元测试中做 SQL 文本断言，且保持可读性"
  - "GetHostByShortID 只扩展 SELECT 列（新增 TemplateImageRef），不改变 Scan 目标以外的字段，保持对 sshproxy / EntryHandler 的 100% 向后兼容"
  - "hostID 入参允许为空字符串：空串时跳过 host 绑定查询直接走 user fallback，方便 Entry API 在只有 user 上下文时复用"
  - "0014 migration 只做 up；down 写在 SQL 注释中，与现存 0007~0013 的单向 migrator 一致（不引入 down 框架改动）"

patterns-established:
  - "Phase 30 数据层契约测试范式：reflect 验证结构体字段 + 包级 SQL 常量做 pattern 断言 + 迁移文件内容断言"
  - "Wave 边界守护测试（TestWave1_DataLayerBoundary）：以测试形式锁死当前计划的职责边界，防止后续 wave 悄悄回流"

requirements-completed: [REQ-F7-A]

duration: ~25min
completed: 2026-04-18
---

# Phase 30 Plan 01: Migration & Entry Store Summary

**`claude_accounts.persistent_volume_name` 列落地 + `ResolveClaudeAccountIDForEntry` 两阶段解析 + `HostSSHAuth.TemplateImageRef`，为 Phase 30 Wave 2 Entry API 提供零依赖的数据层契约。**

## Performance

- **Duration:** ~25 min
- **Started:** 2026-04-18（Wave 1 执行）
- **Completed:** 2026-04-18
- **Tasks:** 3（Task 1/2 走 TDD RED→GREEN，Task 3 单步补回归）
- **Commits:** 5（2 test + 2 feat + 1 test 回归守护）

## Accomplishments

- 落地 migration `0014_claude_account_persistent_volume.sql`（`ADD COLUMN IF NOT EXISTS persistent_volume_name TEXT`，无 `DEFAULT ''`，无 `NOT NULL`，DROP 注释就位），同时满足空库与 v2.0 升级库的幂等 up。
- `ClaudeAccount` 结构体新增 `PersistentVolumeName *string` + `json:"persistent_volume_name,omitempty"`，并在单测中以 JSON 回路确认未分配时字段被省略。
- 以两条包级 SQL 常量实现 D-05 两阶段解析 `ResolveClaudeAccountIDForEntry(ctx, userID, hostID) (string, bool, error)`；未命中返回 `ok=false` 而非 `error`。
- `GetHostByShortID` 的 SELECT 通过 `COALESCE(h.template_image_ref, '')` 纳入 `TemplateImageRef`，为 Wave 2 推导 `image_version` / `supports_*` 提供唯一数据入口；上游 `sshproxy` / `controlplane/http` 测试套件保持通过。
- 新增 Wave 1 边界守护测试，锁死「数据层 only」的职责边界，避免后续计划悄悄把 HTTP / cloudclaude 改动回流到本计划。

## Task Commits

每个任务单独原子提交（TDD：test → feat）：

1. **Task 1 RED: add failing tests for migration 0014 and nullable PersistentVolumeName** — `97b07b2` (test)
2. **Task 1 GREEN: land migration 0014 and PersistentVolumeName field** — `59e982a` (feat)
3. **Task 2 RED: add failing tests for D-05 resolver and template_image_ref exposure** — `cba3e14` (test)
4. **Task 2 GREEN: implement D-05 account resolver and expose template_image_ref** — `5c5ca66` (feat)
5. **Task 3: guard Wave 1 data-layer boundary** — `7a09965` (test)

## Files Created/Modified

- `internal/store/migrations/0014_claude_account_persistent_volume.sql` — 新增 `persistent_volume_name` 列（可空 TEXT），并在 SQL 注释中写明 DROP 回滚路径。
- `internal/store/repository/models.go` — `ClaudeAccount` 新增 `PersistentVolumeName *string`；`HostSSHAuth` 新增 `TemplateImageRef string`。
- `internal/store/repository/queries.go` — `GetHostByShortID` 升级为使用 `getHostByShortIDSQL` 常量并选择 `template_image_ref`；新增 `resolveClaudeAccountByHostSQL` / `resolveClaudeAccountByUserFallbackSQL` 包级常量与 `ResolveClaudeAccountIDForEntry` 方法（D-05）。
- `internal/store/repository/migration_0014_test.go` — 新增单元测试：迁移文件内容断言、结构体反射、SQL pattern 断言、方法签名守护、Wave 1 边界守护。

## Decisions Made

- **零外部依赖的数据层测试策略**：仓库既没有现成的 Postgres 测试夹具，也未引入 pgxmock / testcontainers；因此将 SQL 文本提升为包级常量，用 `strings.Contains` + `reflect` 完成 D-05 解析顺序与结构体形状的契约断言，既避免引入新依赖，又保证“解析顺序悄悄改动”会立即被 CI 打断。
- **`*string` + `omitempty` 承载 NULL**：显式拒绝空字符串默认值（D-02），代价是在 JSON 解码侧需要 `nil` 检查；通过测试中的 `json.Marshal` 回路验证该语义。
- **`GetHostByShortID` 扩展而非新建方法**：所有现有调用方（`sshproxy.RepoResolver`、`controlplane/http.EntryHandler`）依旧走同一入口，`HostSSHAuth` 新增字段但 Scan 目标只增不减。
- **`hostID` 允许空串**：保持 Entry API 只有 user 上下文的调用路径也可直接使用 `ResolveClaudeAccountIDForEntry`，避免在 Wave 2 再写一份 user-only 回退。

## Deviations from Plan

None — plan executed exactly as written。所有任务按 TDD RED→GREEN 推进，未触发 Rule 1/2/3 自动修复，未触及 HTTP / cloudclaude 等非数据层文件。

## Issues Encountered

- 仓库历史上没有 `repository` 包的单元测试（`internal/store/repository/*_test.go` 不存在），且未引入任何 DB 测试依赖。为保证 `go test -count=1 -short` 可执行并产生有意义的断言，采用「SQL 常量 + reflect + 文件内容」方案；若后续某个计划引入了真实 DB 测试夹具，可在此基础上追加行为级测试而不必改现有契约断言。

## Threat Model Compliance

- **T-30-01（参数化查询）**：`resolveClaudeAccountBy*SQL` 与 `getHostByShortIDSQL` 全部使用 `$1` 占位符，`TestWave1_DataLayerBoundary` 中显式断言不含 `fmt.Sprintf` 字符串拼接。
- **T-30-02（migration 幂等）**：`ADD COLUMN IF NOT EXISTS` + 包含 `DROP COLUMN IF EXISTS` 注释的回滚路径，`TestMigration0014_FileContent` 显式断言这两个 token。
- **T-30-03（信息披露）**：`ResolveClaudeAccountIDForEntry` 的错误语义是 `(accountID, ok, err)`；未命中走 `ok=false` 而非 error，避免把「账号存在性」抬升为外部可观察错误。

## User Setup Required

None — 本计划只涉及控制面 Postgres schema 与仓储层代码，无外部服务配置。

## Next Phase Readiness

- Wave 2 Plan 02（`02-entry-api-host-contract`）可直接读取 `HostSSHAuth.TemplateImageRef` 并调用 `ResolveClaudeAccountIDForEntry` 完成 `image_version` / `supports_*` / `claude_account_id` 的拼装，不再需要额外的 migration 或 SQL 改动。
- Phase 33（Docker worker）可读取 `ClaudeAccount.PersistentVolumeName`：`nil` → 生成 `claude-state-{account_id}` 并回写，非 `nil` → 视为已分配的 volume 名直接挂载。

---
*Phase: 30-entry-api*
*Plan: 01-migration-entry-store*
*Completed: 2026-04-18*

## Self-Check

Performed the following verifications before completion:

- `[ -f internal/store/migrations/0014_claude_account_persistent_volume.sql ]` → FOUND
- `[ -f internal/store/repository/migration_0014_test.go ]` → FOUND
- `git log --oneline | grep 97b07b2` → FOUND (Task 1 test commit)
- `git log --oneline | grep 59e982a` → FOUND (Task 1 feat commit)
- `git log --oneline | grep cba3e14` → FOUND (Task 2 test commit)
- `git log --oneline | grep 5c5ca66` → FOUND (Task 2 feat commit)
- `git log --oneline | grep 7a09965` → FOUND (Task 3 guard commit)
- `go test ./internal/store/repository/... -count=1` → PASS
- `go test ./... -count=1 -short` → PASS (cloudclaude, controlplane/http, runtime/tasks, network, repository all green)

## Self-Check: PASSED
