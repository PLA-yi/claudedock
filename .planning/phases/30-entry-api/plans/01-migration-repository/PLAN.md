---
phase: 30-entry-api
plan: 01-migration-repository
sub_scope: A
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/store/migrations/0014_claude_account_persistent_volume.sql
  - internal/store/repository/models.go
  - internal/store/repository/queries.go
autonomous: true
requirements:
  - REQ-F7-A（数据模型列落地）
must_haves:
  truths:
    - "migration 0014 为 claude_accounts 增加 persistent_volume_name TEXT NULL（默认 NULL），up/down 幂等"
    - "ClaudeAccount Go 模型含 PersistentVolumeName *string 或 sql.NullString（与仓库风格一致）"
    - "提供按 D-05 选择 claude_account 行的查询（host_id 优先，再 user_id+host_id IS NULL）供 Entry 使用"
  artifacts:
    - path: "internal/store/migrations/0014_claude_account_persistent_volume.sql"
      provides: "schema 变更"
      contains: "persistent_volume_name"
---

## Goal

为 `claude_accounts` 增加 `persistent_volume_name` 列（与 30-CONTEXT D-01/D-02 一致），并在 repository 层暴露读取与 D-05 账号选择查询，为 Plan 02 Entry API 提供数据面支撑。

## Scope

### In

1. 新增 `internal/store/migrations/0014_claude_account_persistent_volume.sql`：`ADD COLUMN IF NOT EXISTS` + 注释说明语义；`down` 中 `DROP COLUMN IF EXISTS`。
2. `internal/store/repository/models.go`：`ClaudeAccount` 增加 `PersistentVolumeName` 字段（nullable）。
3. `internal/store/repository/queries.go`（及生成代码若项目使用 sqlc）：列表/单条扫描同步；新增 `SelectClaudeAccountForEntry(ctx, hostID, userID)` 或等价命名（返回 optional account row）。

### Out

- Entry HTTP 处理与 JSON 响应 → Plan 02
- `HostActionRequest` / `cloudclaude` → Plan 02
- `docker volume create` → Phase 33

## Dependencies

无（Wave 1）。

## Tasks

### Task 1.1 — 编写 migration 0014

实现 30-CONTEXT D-10；本地对空库与已有数据 `up`/`down` 各跑两次验证幂等。

### Task 1.2 — 模型与查询

更新 `ClaudeAccount` 与所有 `Scan` 路径；实现 D-05 SQL；单元测试可用 sqlite 内存或项目既有 store 测试 harness。

### Task 1.3 — 验证

`go test ./internal/store/...`（或仓库等价路径）通过。

---

## Verification

- [ ] migration 在 CI / 本地 migrate 工具链执行成功
- [ ] 新列在 ORM/手写查询中无遗漏 Scan

---

<threat_model>

| 威胁 | 严重性 | 缓解 |
|------|--------|------|
| migration 在生产重复执行失败 | 中 | 仅 `IF NOT EXISTS`；review down 脚本 |
| 账号选择 SQL 在多行场景非确定 | 低 | `ORDER BY created_at` + CONTEXT 已记录产品假设 |

</threat_model>
