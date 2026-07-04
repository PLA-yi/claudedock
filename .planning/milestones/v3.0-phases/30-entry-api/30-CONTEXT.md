# Phase 30: 控制面数据模型 + Entry API 扩展 - Context

**Gathered:** 2026-04-18
**Status:** Ready for planning

<domain>
## Phase Boundary

在控制面与客户端握手路径上，为 v3.0 能力探测与后续 volume / OAuth 工作打开数据通道，**不**交付任何面向用户的 REQ-F* 行为。具体包括：

1. 数据库：`claude_accounts` 增加 `persistent_volume_name`（与 REQ-F7-A 单 volume 命名一致）。
2. API 契约：`HostActionRequest` 增加 `ClaudeAccountID`（`Volumes` 已在 Phase 29 落地，本阶段与之对齐完成账号维度）。
3. Entry API：`POST /v1/entry/{shortId}/auth` 在保持 v2.0 字段不变的前提下，追加 `image_version`、`supports_mutagen`、`supports_mergerfs`、`claude_account_id`（全部 `omitempty`，旧客户端忽略未知 JSON 字段）。
4. `internal/claudedock.AuthResponse` 同步扩展字段与向后兼容语义。
5. 单元测试：旧结构反序列化新 JSON；新字段缺失时客户端降级默认值。

</domain>

<decisions>
## Implementation Decisions

### Q4 · 持久化 volume 命名（ROADMAP Open question）

- **D-01**：锁定 **单 named volume**：`claude-state-{claude_account_id}`（UUID 无连字符或与 DB 一致，由实现选定但全仓库一致）；Docker label `com.claudedock.account_id={claude_account_id}` 由 Phase 33 在 `volume create` 时写入。本阶段 migration 仅提供列，不强制非空。
- **D-02**：`persistent_volume_name` 列语义：**`NULL` = 尚未由控制面/任务分配名称**；一旦分配则与 D-01 规范一致，便于 Phase 33 `ensureDockerVolume` 幂等查找。禁止用空字符串表示「未分配」，减少三态。

### Q5 · 能力字段暴露面

- **D-03**：采用 ROADMAP 倾向 **(a)**——在现有 **`/v1/entry/{shortId}/auth`** 成功响应体中追加字段；**不**新增 `/capabilities` 或额外 round-trip endpoint。

### Q6 · host-agent 与镜像元数据

- **D-04**：维持 Phase 29 结论——**不**扩展 host-agent 返回 Docker image labels；`image_version` 与 `supports_*` 全部由**控制面**根据 `hosts.template_image_ref`（及可选的受管镜像 tag 约定）推导。

### Entry API 查询与字段推导

- **D-05**：`claude_account_id` 选择规则（同一主机多账号前的确定性）：
  1. `SELECT id FROM claude_accounts WHERE host_id = <当前已解析主机的 UUID> ORDER BY created_at ASC LIMIT 1`；
  2. 若无行，则 `WHERE user_id = <user.id> AND host_id IS NULL ORDER BY created_at ASC LIMIT 1`；
  3. 若仍无，**省略** `claude_account_id` 字段（`omitempty`），`supports_mutagen` / `supports_mergerfs` 仍按 D-06 仅依据镜像推导（可能为 `false`）。
- **D-06**：`image_version` 从 `hosts.template_image_ref` 解析——取最后一个 `:` 后的 tag（若无 `:` 则整串）；仅做字符串规范化（trim），**不**在 Phase 30 调用 Docker registry API。
- **D-07**：`supports_mutagen` 与 `supports_mergerfs`：当解析出的 `image_version` 与受管 v3 基线 tag **`v3.0.0`** 相等（字符串相等）时为 `true`，否则 `false`。后续若有多 tag，由后续 phase 扩展对照表；本阶段保持与 ROADMAP Success Criteria 字面一致。
- **D-08**：`status` 为 `not_ready` 或非 `ready` 的响应体 **不强制**带 v3 扩展字段（可为省略）；`ready` 路径必须带齐 ROADMAP 验收所列字段（在账号存在前提下 `claude_account_id` 非空）。

### HostActionRequest

- **D-09**：新增 `ClaudeAccountID string \`json:"claude_account_id,omitempty"\``（字段名与 JSON 与 ROADMAP 一致）。与 Phase 29 **D-21** 对齐：Phase 29 已交付 `Volumes`，本阶段交付账号 ID 供 Phase 33 组装 volume 与 worker 使用。

### 迁移与兼容

- **D-10**：新 migration 序号为 **`0014_claude_account_persistent_volume.sql`**（当前仓库最新为 `0013`，与 ROADMAP 一致）；在干净库与自 v2.0 升级库上 `up`/`down` 可重复执行（幂等 `IF NOT EXISTS` / 安全 `DROP`）。

### Claude's Discretion

- **D-11**：`EntryStore` 具体方法签名与是否合并查询（单次 JOIN vs 多次查询）由实现者按性能与可测性选择，只要不引入 N+1 明显回退。
- **D-12**：admin / GraphQL 等未在 ROADMAP 本 phase 列出的 surface **不在此 phase 扩展**。

### Folded Todos

（`todo match-phase` 无匹配，本节省略。）

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 规划与需求

- `.planning/ROADMAP.md` — Phase 30 Goal / Scope / Success Criteria / Open questions Q4–Q6
- `.planning/REQUIREMENTS.md` — REQ-F7-A 与 Q4 决策表
- `.planning/PROJECT.md` — v3.0 总目标与 F7 持久化方向

### 前置阶段上下文

- `.planning/phases/29-v3-worker/29-CONTEXT.md` — `VolumeMount` / `HostActionRequest.Volumes`、ClaudeAccountID 推迟到 Phase 30 的明确记录（D-18..D-21）

### 实现锚点（代码）

- `internal/controlplane/http/entry.go` — Entry `Auth` 处理器与当前 JSON 响应形状
- `internal/claudedock/entry.go` — `AuthResponse` 与 `Authenticate` 反序列化路径
- `internal/agentapi/contracts.go` — `HostActionRequest` / `VolumeMount`
- `internal/runtime/tasks/worker.go` — `createHost` 消费 `Volumes`（Phase 29）；本阶段规划需预留 `ClaudeAccountID` 传参落点
- `internal/store/migrations/0007_auth_unification.sql` — `claude_accounts` 表基线定义
- `internal/store/repository/models.go` — `ClaudeAccount`、`Host` 模型

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `HostActionRequest` 已含 `Volumes []VolumeMount`（Phase 29）；本阶段在其上追加 `ClaudeAccountID` 即可与 worker 路径自然衔接。
- `entry.go`（控制面）当前返回固定的 `ssh_port: 2222` 与五字段；扩展为同一 `writeJSON` map 追加键即可保持风格一致。

### Established Patterns

- JSON API 使用 `map[string]any` 或强类型 struct + `omitempty`；Entry 现用 `map[string]any`，规划时可评估是否改为局部 struct 以减少拼写错误。
- 仓库层通过 `repository` 包与 pgx 访问；新增查询应遵循现有错误包装与 `pgx.ErrNoRows` 处理。

### Integration Points

- `EntryHandler` 依赖 `EntryStore` 接口；扩展读路径时需同步 mock（如 `admin_hosts_test` 中的 stub）与实现类型。
- `claudedock.EntryClient.Authenticate` 在 `ready` 时校验 SSH 四元组；扩展后 v3 客户端可读取新字段，**不得**破坏现有四元组校验。

</code_context>

<specifics>
## Specific Ideas

- ROADMAP Success Criteria 要求示例值：`image_version="v3.0.0"`、`supports_mutagen=true`、`supports_mergerfs=true`、非空 `claude_account_id`（在测试数据满足 D-05 时）。

</specifics>

<deferred>
## Deferred Ideas

- **双 volume**（creds/cache 分离）与 **`ccp_` 前缀命名**：已在 Q4 明确推迟；若运维提出再开 phase 或 backlog。
- **独立 `/capabilities` endpoint**：Q5 已关闭；若未来需无凭证探测能力再评估。
- **从 registry / image inspect 读取 labels 推导能力**：Q6 已关闭；与 host-agent 扩展一并留给更远期。

### Reviewed Todos (not folded)

无。

</deferred>

---

*Phase: 30-entry-api*
*Context gathered: 2026-04-18*
