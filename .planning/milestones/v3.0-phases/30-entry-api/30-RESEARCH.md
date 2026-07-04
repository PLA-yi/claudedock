# Phase 30 — Research Notes

> 供 `gsd-planner` 消费。子代理不可用时的精简版；事实以仓库内代码与 CONTEXT 为准。

## 锁定前提（来自 30-CONTEXT.md）

- 单 volume 名 `claude-state-{claude_account_id}`；列 `persistent_volume_name`：`NULL` = 未分配。
- 仅扩展 `POST /v1/entry/{shortId}/auth` 响应；不新 endpoint。
- 不从 host-agent 拉 image labels；`image_version` / `supports_*` 由控制面从 `hosts.template_image_ref` 推导；`supports_*` 仅当 tag 等于 `v3.0.0` 为 true。
- `ClaudeAccountID` 加入 `HostActionRequest`；`Volumes` 已在 Phase 29 存在。

## 代码锚点 [VERIFIED: workspace grep/read]

- `internal/controlplane/http/entry.go`：`Auth` 用 `EntryStore`；成功响应 `writeJSON` + `map[string]any`（`ssh_user`/`ssh_pass`/…）。扩展字段应保留 `not_ready` 分支行为（CONTEXT：可不强制新字段）。
- `internal/claudedock/entry.go`：`AuthResponse` 仅五 SSH 字段 + status；`json.Unmarshal` 忽略未知字段已满足旧 client；新字段用指针或 `omitempty` 以便「缺省语义」单测。
- `internal/agentapi/contracts.go`：`VolumeMount`、`HostActionRequest` 已存在；追加 `ClaudeAccountID` 与 `json` tag `claude_account_id,omitempty`。
- `internal/store/migrations/`：下一文件 `0014_*.sql`；`claude_accounts` 定义于 `0007_auth_unification.sql`。
- `repository.ClaudeAccount` 尚无 `PersistentVolumeName` — models + queries 需同步。

## 规划需覆盖的任务面

1. **Migration**：`ALTER TABLE claude_accounts ADD COLUMN persistent_volume_name TEXT;` + 可选索引（按 CONTEXT 非必须）；`down` 安全 drop column。
2. **Repository**：按 D-05 选择 `claude_account_id` 的 SQL；可选与 `hosts` 单次 JOIN 减少往返。
3. **EntryStore 接口**：新增方法或扩展现有 `GetHostByShortID` 返回体（注意破坏面）；所有 stub 实现同步（如 `admin_hosts_test` stub）。
4. **Entry Auth**：`ready` 时填充四新字段；解析 `TemplateImageRef` 的 tag 辅助函数 + 单元测试表驱动。
5. **claudedock**：`AuthResponse` 字段；若 v3 需读 supports，文档化零值 = 旧网关。
6. **测试**：`encoding/json` 旧 struct 对新 JSON；新 client 对缺字段响应；migration 集成测试若项目已有模式则跟随。

## 风险与陷阱 [ASSUMED]

- **多 `claude_accounts` 同行**：D-05 `ORDER BY created_at` 必须在产品上与 Phase 33 创建顺序一致，否则需在后续 phase 显式「主账号」标记。
- **`template_image_ref` 无 tag**：`image_version` 空、`supports_*` false — 与 CLI 降级预期一致 [ASSUMED]。

## Project Constraints

- 遵守 CLAUDE.md / CONVENTIONS：回复中文；git 跟踪文件无绝对路径；无真实密钥。
- API 向后兼容：旧 claudedock 必须仍能 `Authenticate` [VERIFIED: Go json.Unmarshal 忽略未知字段行为]。

## Validation Architecture

（`nyquist_validation_enabled` 为 false — 本仓库本次 init 未要求 VALIDATION.md；可省略或一句话指向 ROADMAP Success Criteria。）

## RESEARCH COMPLETE
