---
phase: 30-entry-api
plan: 01-migration-entry-store
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/store/migrations/0014_claude_account_persistent_volume.sql
  - internal/store/repository/models.go
  - internal/store/repository/queries.go
  - internal/store/repository/migration_0014_test.go
autonomous: true
requirements:
  - REQ-F7-A
must_haves:
  truths:
    - "存在 `0014_claude_account_persistent_volume.sql`，对 `claude_accounts` 增加可空 `persistent_volume_name`（语义见 D-02）"
    - "`ClaudeAccount` 模型可表达该列的 NULL/非 NULL；`GetHostByShortID` 返回 `hosts.template_image_ref` 供 Entry 推导镜像 tag（D-06 前置）"
    - "`Repository.ResolveClaudeAccountIDForEntry`（或等价命名）严格按 D-05 两条 SQL 顺序解析；无匹配时 `ok=false` 且非错误"
  artifacts:
    - path: internal/store/migrations/0014_claude_account_persistent_volume.sql
      provides: "ADD COLUMN IF NOT EXISTS persistent_volume_name；含可手工执行的 down 片段注释（DROP IF EXISTS）"
      contains: persistent_volume_name
    - path: internal/store/repository/models.go
      provides: "ClaudeAccount.PersistentVolumeName 可空字段"
      contains: PersistentVolumeName
    - path: internal/store/repository/queries.go
      provides: "HostSSHAuth 扩展 TemplateImageRef；Resolve 方法"
      contains: ResolveClaudeAccountIDForEntry
  key_links:
    - from: GetHostByShortID
      to: hosts.template_image_ref
      via: "SELECT 列扩展"
      pattern: template_image_ref
    - from: ResolveClaudeAccountIDForEntry
      to: claude_accounts
      via: "host_id 优先、再 user_id+host_id IS NULL（D-05）"
---

## 目标

落地 Phase 30 数据面：`claude_accounts.persistent_volume_name` 列（REQ-F7-A / Q4 → D-01、D-02、D-10），并为 Entry 握手提供 **`template_image_ref` 与确定性 `claude_account_id` 解析** 的仓储能力（D-05、D-11），**不**在本 plan 修改 HTTP handler 或 `HostActionRequest`（属 plan 02）。

## 执行上下文

- 讨论锁定：`.planning/phases/30-entry-api/30-CONTEXT.md`
- 研究备忘：`.planning/phases/30-entry-api/30-RESEARCH.md`

## 范围说明

### 纳入

- 新迁移文件名必须为 **`0014_claude_account_persistent_volume.sql`**（D-10）。`ALTER TABLE ... ADD COLUMN IF NOT EXISTS persistent_volume_name TEXT`；**禁止**用空字符串表示未分配（D-02）。列可为 NULL；不设 `NOT NULL DEFAULT ''`。
- 文件末尾用 **SQL 注释** 给出可手工执行的 down 片段：`ALTER TABLE claude_accounts DROP COLUMN IF EXISTS persistent_volume_name;`，以满足 ROADMAP「可回滚」叙述且不与当前仅正向的 `migrator` 冲突。
- `ClaudeAccount` 增加 `PersistentVolumeName *string`（或 `sql.NullString`，与仓库现有可空风格一致）。
- `HostSSHAuth` 增加 `TemplateImageRef string`（或从 DB COALESCE 为空串），`GetHostByShortID` 的 `SELECT` 增加 `h.template_image_ref`（及 `COALESCE` 若需）。
- 新增 `func (r *Repository) ResolveClaudeAccountIDForEntry(ctx context.Context, hostID, userID string) (id string, ok bool, err error)`：
  1. 先查 `host_id = $hostID ORDER BY created_at ASC LIMIT 1`；
  2. 若无行，再查 `user_id = $userID AND host_id IS NULL ORDER BY created_at ASC LIMIT 1`；
  3. 仍无行则 `ok=false`、`id==""`、`err==nil`（D-05 步骤 3）。
- 若未来已有 `ListClaudeAccounts` 等扫描 `claude_accounts` 的查询，本 plan 一并补扫 `persistent_volume_name`；当前仓库若无此类 SQL，仅保证模型与迁移一致即可。

### 排除（D-12 / 其它阶段）

- admin / GraphQL 新 surface；`docker volume create`；Entry JSON 响应体（plan 02）。

<tasks>

<task type="auto" tdd="true">
  <name>任务 1：迁移 0014 + ClaudeAccount 模型字段</name>
  <files>internal/store/migrations/0014_claude_account_persistent_volume.sql,internal/store/repository/models.go,internal/store/repository/migration_0014_test.go</files>
  <behavior>
    - 测试：迁移文件存在且包含 `ADD COLUMN IF NOT EXISTS persistent_volume_name`；注释块中含 `DROP COLUMN IF EXISTS persistent_volume_name`。
    - 测试：`ClaudeAccount` 结构体含 `PersistentVolumeName` 字段且 `json` tag 使用 `omitempty`（若序列化用得到）。
  </behavior>
  <action>按 D-01/D-02/D-10 编写迁移与模型字段；迁移内不写绝对路径；索引仅在为后续 phase 确有查询模式时再加，本 plan 默认不加（RESEARCH 可选索引）。对应 D-01（列就绪）、D-02、D-10。</action>
  <verify>
    <automated>go test ./internal/store/repository/... -count=1 -short</automated>
  </verify>
  <done>迁移与模型字段合并；`migration_0014_test.go` 绿灯。</done>
</task>

<task type="auto" tdd="true">
  <name>任务 2：GetHostByShortID 扩展 + ResolveClaudeAccountIDForEntry</name>
  <files>internal/store/repository/queries.go,internal/store/repository/migration_0014_test.go</files>
  <behavior>
    - 测试（表驱动）：对 `ResolveClaudeAccountIDForEntry` 使用 **fake DB** 或 **仅测 SQL 常量化片段** 不可行时，改为在 `migration_0014_test.go` 或新建 `resolve_claude_account_test.go` 中：用 `pgxmock` **或** 最小接口 `type rowQuerier interface{ QueryRow(...)}` + fake stub，断言两次查询的先后顺序与 `ok` 语义（无第二命中时 `ok=false`）。若引入 `pgxmock` 需改 `go.mod`，优先用 **手写 stub 实现 `Querier` 子集**（仅 `QueryRow`）避免新依赖（D-11）。
    - 至少覆盖：仅命中 host 绑定；仅命中 host_id IS NULL；全无。
  </behavior>
  <action>扩展 `HostSSHAuth` 与 `GetHostByShortID`（D-06 数据输入）；实现 D-05 解析逻辑；错误仅来自 DB 故障，无行不算错误。sshproxy 等调用方忽略新字段不受影响。</action>
  <verify>
    <automated>go test ./internal/store/repository/... -count=1 -short</automated>
  </verify>
  <done>仓库编译通过；`ResolveClaudeAccountIDForEntry` 行为与 D-05 一致且有自动化断言。</done>
</task>

</tasks>

<threat_model>
## 信任边界

| 边界 | 说明 |
|------|------|
| SQL 迁移 | 以文件形式进入部署管线；错误 DDL 可导致启动失败或数据不可用 |
| 仓储查询 | 从控制面进程访问 DB；参数化查询边界防止注入 |

## STRIDE 登记

| 威胁 ID | 类别 | 组件 | 处置 | 缓解措施 |
|---------|------|------|------|----------|
| T-30-01 | T | `queries.go` / Resolve | mitigate | 仅使用参数化查询（`$1`/`$2`），禁止字符串拼接用户输入进 SQL |
| T-30-02 | I | `0014_*.sql` | mitigate | 迁移仅 DDL；部署前在 CI 跑 `go test` + 代码审阅核对 `IF NOT EXISTS` / `DROP IF EXISTS` |
| T-30-03 | E | `claude_accounts` 行泄露 | accept | 与既有 Entry 认证同属控制面信任域；本方法不扩大 HTTP 暴露面 |
</threat_model>

<verification>
- `go test ./internal/store/repository/... -count=1 -short` 通过。
- `go build ./...` 通过（可由 CI 覆盖）。
</verification>

<success_criteria>
- 迁移文件命名与 D-10 一致；列语义满足 D-02。
- D-05 在仓储层可测且行为固定。
</success_criteria>

<output>
完成后撰写 `.planning/phases/30-entry-api/plans/01-migration-entry-store/SUMMARY.md`（由执行器生成）。
</output>

## 来源审计（规划闭环）

| 来源 | 条目 | Plan | 状态 |
|------|------|------|------|
| GOAL | 控制面通道 + 数据模型 | 01 | COVERED |
| REQ | REQ-F7-A（命名粒度数据模型） | 01 | COVERED |
| RESEARCH | migration / repository / D-05 SQL | 01 | COVERED |
| CONTEXT | D-01,D-02,D-05,D-10,D-11,D-12（排除项） | 01 | COVERED |
