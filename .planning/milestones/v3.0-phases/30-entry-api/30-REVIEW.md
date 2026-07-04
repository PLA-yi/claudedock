---
phase: 30-entry-api
review_date: 2026-04-18
depth: standard
status: issues_found
files_reviewed: 11
findings:
  blocker: 0
  major: 0
  minor: 1
  nit: 3
---

# Phase 30 Code Review

## Summary
Phase 30（数据模型 + Entry API 扩展）整体实现紧贴 30-CONTEXT 的 D-01..D-12 决策，两波（Wave 1 数据层 / Wave 2 协议层）边界清晰，未互相侵占；全部 STRIDE 威胁（T-30-01..T-30-07）都有可执行的测试或可接受理由，`go test` 在四个相关 package 全部通过。发现一条 `bool + omitempty` 语义上的小风险与三条可选改进建议，均不阻塞合并。

## Findings

### Blockers
无。

### Major Issues
无。

### Minor Issues

- **M1 · `SupportsMutagen`/`SupportsMergerfs` 使用 `omitempty` 会模糊「显式 false」与「服务器未返回」**
  - 位置：`internal/claudedock/entry.go:32`、`internal/claudedock/entry.go:33`
  - 现象：控制面在 ready 路径走 `map[string]any`，总是显式写入 `supports_mutagen: false / supports_mergerfs: false`（见 `internal/controlplane/http/entry.go:212`-`213`）；但客户端 struct 带 `omitempty`，若客户端再次 marshal 这份 `AuthResponse`，`false` 会被省略，导致下游无法区分「服务器说 false」与「服务器未声明」。
  - 影响：Phase 30 内零影响（客户端只读、不重发）；但若 Phase 31/33 想基于该结构再转发或持久化，D-08「不强制带」和「显式 false」的语义会被压平。
  - 建议：保持现状可接受（Phase 30 D-08 允许非 ready 省略；ready 的 false 本就只是「不启用能力」），但请在 Phase 31/33 引入转发逻辑前复核；或将 `bool` 改为 `*bool` + `omitempty` 以准确保留三态。

### Nits

- **N1 · D-05 step 1 与 step 2 之间的 fallthrough 语义需要更显式的注释**
  - 位置：`internal/store/repository/queries.go:1209`-`1217`
  - 现象：当 `hostID != ""` 且「以 host 绑定」查询返回 `pgx.ErrNoRows` 时，代码直接 fall through 到 user fallback；`// fall through to user fallback` 注释已经存在，但没有点明这正是 D-05 第三步之前允许的退化路径。
  - 建议：将注释补成 `// step 1 miss: fall through to D-05 step 2 (user-scoped fallback)` 之类字样，方便后续读者直接对齐决策。

- **N2 · migration 0014 缺少一个「file-level 注释」说明 down path 由运维手工执行**
  - 位置：`internal/store/migrations/0014_claude_account_persistent_volume.sql:11`-`12`
  - 现象：SQL 注释已经写了回滚语句，但未点明当前项目 `migrator` 只跑 up、down 需要运维手工执行。
  - 建议：在注释里补一句「本仓库 migrator 只 apply up；回滚需运维手工 `psql -c 'ALTER TABLE ... DROP COLUMN IF EXISTS ...'`」以消除歧义。

- **N3 · Script handler 仍使用 `grep -o` 解析 JSON，对未来新增字段不够健壮**
  - 位置：`internal/controlplane/http/entry.go:81`-`85`
  - 现象：Phase 30 未改动 `Script()`，但它基于字符串模式抽取 `ssh_user/ssh_pass/ssh_host/ssh_port/status`。当前新增字段命名（`image_version`、`supports_*`、`claude_account_id`）与既有模式没有冲突，所以功能未退化。
  - 建议：属于遗留问题，不在 Phase 30 范围内；后续若再给 ready 响应追加以 `ssh_` 或 `status` 为前缀的字段需同步检查脚本。留作 backlog。

## Strengths

- Wave 1 严格限定在 `internal/store/**`，并用 `TestWave1_DataLayerBoundary` 测试把「参数化查询」「文件归属」「migration 存在」三条强约束写进回归，对后续重构极具防御性。
- `ResolveClaudeAccountIDForEntry` 把 D-05 三步规则落成「ok=false ≠ 错误」的清爽契约，并通过 `TestEntryAuth_ResolverError_Returns500` 锁定「DB 故障必须 fail-fast」，同时避免了存在探针（T-30-03/T-30-05）。
- Entry 响应扩展遵循 D-08：`not_ready`、`401`、`no_host` 等非 ready 路径一律不泄露 v3 能力字段，有显式的测试覆盖（`TestEntryAuth_NotReady_DoesNotForceExtensionFields` / `TestEntryAuth_InvalidCredentials_NoExtensions`）。
- `deriveEntryCapabilities` 的实现显式处理 `registry.internal:5000/...`（带端口）、空格、空串、`v3.0.0-rc1` 等边界 case，测试样本覆盖完整；能力判断只做字符串等值比较，避免提前引入「多 tag 对照表」（D-07 要求）。
- `AuthResponse` 新字段全部 `omitempty` 且附决策注释；`entry_compat_test.go` 同时覆盖了「v3 gateway → 新字段透传」「v2 gateway → 零值 + 既有 SSH 四元组仍被校验」两条路径，`TestAuthResponse_MarshalOmitempty` 还守护了序列化侧。
- 提交粒度细（RED → GREEN → doc 成对出现），易于回放与回滚。

## Threat Model Verification

| Threat ID | Mitigation Verified | Evidence |
|-----------|---------------------|----------|
| T-30-01 (SQLi on repository queries) | ✓ | `resolveClaudeAccountByHostSQL` / `resolveClaudeAccountByUserFallbackSQL` / `getHostByShortIDSQL` 全部使用 `$1` 参数化；`TestWave1_DataLayerBoundary` 断言「无 `fmt.Sprintf` / `||` 拼接，至少含 `$1`」 |
| T-30-02 (migration idempotency) | ✓ | `0014_claude_account_persistent_volume.sql` 使用 `ADD COLUMN IF NOT EXISTS`，并禁用 `NOT NULL` / 空串默认值；`TestMigration0014_FileContent` 明文断言必含 / 禁含 token |
| T-30-03 (claude_account 存在探针泄露) | ✓ (accept) | 解析器返回 `(id, ok=false, nil)` 而非错误；`TestEntryAuth_Ready_NoClaudeAccount_OmitsField` 验证未命中时响应不含 `claude_account_id`，不产生可观察的差分 |
| T-30-04 (handler map key 拼写漂移) | ✓ | `entry_auth_test.go` 对全部 5 个 ready 字段逐键断言；`entry_caps_test.go` 锁死推导规则；JSON 键名与客户端 `AuthResponse` tag 一一对齐 |
| T-30-05 (capability / account_id 过度暴露) | ✓ | handler 只在 ready 且密码校验通过后才拼装扩展字段；`not_ready` 路径单独 `writeJSON`、`401/403/404/500` 路径同样不携带（`TestEntryAuth_NotReady_…`、`TestEntryAuth_InvalidCredentials_…`） |
| T-30-06 (supports 推导误判) | ✓ (accept) | 误判永远朝 `false` 降级；`deriveEntryCapabilities` 仅等值比较 `v3.0.0`，`TestDeriveEntryCapabilities` 覆盖 rc 版本、空串、无冒号、带端口等边界 |
| T-30-07 (v2/v3 兼容回归) | ✓ | `TestHostActionRequest_V2Compat` / `…_ForwardCompat`、`TestAuthResponse_MissingV3Fields_DefaultZero`、`TestAuthenticate_V2Gateway_NoExtensionsRequired` 覆盖双向兼容；所有新字段统一 `omitempty` |

## Wave Boundary Verification

- **Wave 1 commits 仅触达 `internal/store/**`** ：✓
  - `97b07b2`、`59e982a`、`cba3e14`、`5c5ca66`、`7a09965` 的 `git show --stat` 仅包含 `internal/store/migrations/0014_claude_account_persistent_volume.sql`、`internal/store/repository/models.go`、`internal/store/repository/queries.go`、`internal/store/repository/migration_0014_test.go`，未越界进入 `internal/controlplane` / `internal/claudedock` / `internal/agentapi`。
- **Wave 2 消费 Wave 1 的产物、未重新实现 SQL**：✓
  - `internal/controlplane/http/entry.go` 通过 `EntryStore.ResolveClaudeAccountIDForEntry` 接口引用 Wave 1 的仓储实现，并读取 `HostSSHAuth.TemplateImageRef` / `Host.TemplateImageRef`（Wave 1 扩展的 `GetHostByShortID` / 既有 `GetPrimaryHostByUserID` 输出），未在 HTTP 层重新写 `SELECT` / 参数化查询。
  - `deriveEntryCapabilities` 仅做字符串解析，没有任何 DB 调用或 host-agent 回路，符合 D-04。
  - `internal/agentapi/contracts.go` 只新增一个 `ClaudeAccountID string` 字段，Wave 2 的 `worker_volume_test.go` 仅测 JSON 契约，没有触达数据层。
- **artifacts[].contains 与实际代码一致**：✓
  - Wave 1：`persistent_volume_name`（migration、models JSON tag、queries 注释均可查）、`PersistentVolumeName`（`models.go:217`）、`ResolveClaudeAccountIDForEntry`（`queries.go:1200`）。
  - Wave 2：`claude_account_id`（`contracts.go` 新字段 JSON tag、handler 响应键、`AuthResponse` tag）、`supports_mutagen`（`entry.go:212`）、`SupportsMergerfs`（`claudedock/entry.go:33`）。
