---
phase: 30-entry-api
plan: 02-entry-http-cli-contract
sub_scope: B
type: execute
wave: 2
depends_on:
  - 01-migration-repository
files_modified:
  - internal/agentapi/contracts.go
  - internal/controlplane/http/entry.go
  - internal/cloudclaude/entry.go
  - internal/controlplane/http/*_test.go
  - internal/cloudclaude/*_test.go
autonomous: true
requirements:
  - ROADMAP Phase 30 Success Criteria 1–4
must_haves:
  truths:
    - "HostActionRequest 含 ClaudeAccountID omitempty JSON tag claude_account_id"
    - "Entry Auth ready 响应含 image_version / supports_mutagen / supports_mergerfs / claude_account_id（omitempty），旧字段不变"
    - "cloudclaude.AuthResponse 扩展字段；旧二进制忽略未知 JSON 字段的集成语义有测试覆盖"
    - "supports_* 推导严格等于 image tag v3.0.0（CONTEXT D-07）"
---

## Goal

扩展控制面 Entry `Auth` 与 `cloud-claude` 客户端 `AuthResponse`，并在 agentapi 契约中加入 `ClaudeAccountID`，满足 ROADMAP Phase 30 成功标准与 30-CONTEXT 全部锁定项。

## Scope

### In

1. `internal/agentapi/contracts.go`：在 `HostActionRequest` 增加 `ClaudeAccountID`（不与 Phase 29 字段顺序冲突；置于 `Volumes` 旁或文档约定位置）。
2. `internal/controlplane/http/entry.go`：
   - 扩展 `EntryStore`：能取 `TemplateImageRef` + 执行 Plan 01 的 claude_account 查询（可合并到现有 host 解析路径）。
   - 解析 tag 的小函数（D-06）；填充 D-07 bool；D-05 id；`not_ready` 分支可不带来源字段（D-08）。
3. `internal/cloudclaude/entry.go`：`AuthResponse` 新字段指针或 `omitempty`；不破坏 `ready` 时 SSH 四元组校验逻辑。
4. 测试：HTTP handler 级或 store mock：`ready` + 受管镜像 tag → 四新字段断言；旧 JSON → 旧 struct；缺失新字段的响应 → 新 client 零值行为。

### Out

- worker 消费 `ClaudeAccountID` 拼 volume（Phase 33）
- admin UI

## Dependencies

- Plan 01（repository 查询与列存在）

## Tasks

### Task 2.1 — agentapi `ClaudeAccountID`

按 CONTEXT D-09 添加字段；`go test` 覆盖 JSON round-trip 与 v2.0 无该字段反序列化。

### Task 2.2 — EntryStore + Auth handler

实现 join/查询；所有 `EntryStore` stub（如 admin 测试）补全新方法默认行为。

### Task 2.3 — cloudclaude `AuthResponse`

字段 + 如需要时对 v3 字段的读取辅助（不设新 REQ）。

### Task 2.4 — 成功标准对齐测试

覆盖 ROADMAP 四条 Success Criteria 的可自动化子集（migration 在 Plan 01）。

---

## Verification

- [ ] `go test ./...` 相关包通过
- [ ] 手动或用集成测试：`POST /v1/entry/.../auth` ready 体含预期键

---

<threat_model>

| 威胁 | 严重性 | 缓解 |
|------|--------|------|
| 破坏 v2.0 客户端 JSON 解析 | 高 | 仅追加字段 + omitempty；旧 client 集成测试 |
| EntryStore 接口变更导致编译遗漏 | 中 | 全仓库 `EntryStore` 实现 grep |
| 错误暴露内部 ID | 低 | `claude_account_id` 仅在认证成功后返回，与现 SSH 凭据同级 |

</threat_model>
