---
phase: 30-entry-api
phase_name: 控制面数据模型 + Entry API 扩展
verification_date: 2026-04-18
status: passed
score: 4/4
must_haves_verified: 4
must_haves_total: 4
requirement_ids:
  - REQ-F7-A
human_verification:
  - id: hv-01
    description: "在真实 Postgres 18.x 宿主上执行 migrator 升级路径（v2.0 → v3.0），确认 0014 在空库与已有数据的升级库上均可 up 且不报错"
    expected: "migrator 输出 0014 已应用，`\\d claude_accounts` 包含可空 `persistent_volume_name TEXT`"
  - id: hv-02
    description: "生产或预发布 gateway 接入 v2.0 cloud-claude 二进制时，验证旧客户端反序列化 v3 扩展字段不报错"
    expected: "v2.0 CLI 成功完成 ready 流并进入 SSH；v3 扩展字段被静默忽略"
---

# Phase 30: 控制面数据模型 + Entry API 扩展 — Verification Report

## Phase Goal Recap

> 为 v3.0 体验所需的"客户端动态能力探测"打开控制面通道——`claude_accounts.persistent_volume_name` 字段就绪、`HostActionRequest` 在 API 契约层接收 `ClaudeAccountID + Volumes`、`Entry API` 在现有 `/v1/entry/{id}/auth` 响应里追加 `image_version` / `supports_mutagen` / `supports_mergerfs` / `claude_account_id` 字段（向后兼容，旧 client 不破）。

本阶段不交付任何 user-facing REQ-F*，但 Phase 31/33 全部依赖它。

## Success Criteria Verification

| # | Criterion | Verdict | Evidence |
|---|-----------|---------|----------|
| 1 | migration `0014` 在干净库 + v2.0 库上均能 up 幂等执行 | ✓ PASS | `internal/store/migrations/0014_claude_account_persistent_volume.sql` 使用 `ADD COLUMN IF NOT EXISTS persistent_volume_name TEXT`，无 `NOT NULL`、无 `DEFAULT ''`；`TestMigration0014_FileContent` 锁定必含/禁含 token。Down 路径以 SQL 注释标注由运维手工执行（与 0007~0013 单向 migrator 对齐） |
| 2 | v2.0 客户端二进制调用新版 Entry API 不返回错误，未知字段被忽略 | ✓ PASS | `internal/cloudclaude/entry_compat_test.go::TestAuthResponse_MissingV3Fields_DefaultZero` 与 `TestAuthenticate_V2Gateway_NoExtensionsRequired` 分别覆盖「v2 gateway 响应缺字段→零值」与「v2 gateway 不强制带 v3 字段仍可完成握手」；`AuthResponse` 全部新字段使用 `omitempty` |
| 3 | v3.0 客户端能正确读到 `image_version="v3.0.0"` / `supports_mutagen=true` / `supports_mergerfs=true` / `claude_account_id="<uuid>"` | ✓ PASS | `internal/controlplane/http/entry_auth_test.go` 验证 ready 路径响应键值；`internal/controlplane/http/entry_caps_test.go::TestDeriveEntryCapabilities` 覆盖 `v3.0.0` / 带 registry 前缀 / 带端口 / `v3.0.0-rc1` / 空串 / 无冒号等边界；`entry_compat_test.go` 验证 v3 字段在客户端结构中完整回路 |
| 4 | `HostActionRequest` 在 host-agent 端解析 `Volumes` 字段且不引入新增 endpoint（沿用现有 `/agent/host/action`） | ✓ PASS | `Volumes []VolumeMount` 由 Phase 29 已交付；Phase 30 追加 `ClaudeAccountID string \`json:"claude_account_id,omitempty"\``（`internal/agentapi/contracts.go:54`）；`internal/runtime/tasks/worker_volume_test.go` 覆盖 v2 无字段/v3 含字段的 round-trip 兼容；未新增 endpoint，D-03 得到贯彻 |

**总分：4/4 通过。**

## Requirements Traceability

| Requirement | Scope in Phase 30 | Status |
|-------------|-------------------|--------|
| REQ-F7-A | 命名约定数据模型落地：`claude_accounts.persistent_volume_name` 字段、`HostActionRequest.ClaudeAccountID` 契约 | ✓ COVERED at data-contract layer（实际 volume 创建与 label 由 Phase 33 交付） |

Phase 30 无独占用户可感知 REQ；为 Phase 31（F1/F2）、Phase 33（F7）提供前置契约。

## Wave Boundary & Consumption Chain

- **Wave 1（数据层）** commits（`97b07b2`、`59e982a`、`cba3e14`、`5c5ca66`、`7a09965`）仅触及 `internal/store/migrations/*` 与 `internal/store/repository/*`，未越界；`TestWave1_DataLayerBoundary` 以测试形式守护此边界。
- **Wave 2（协议层）** 通过 `EntryStore.ResolveClaudeAccountIDForEntry` 与 `HostSSHAuth.TemplateImageRef` 消费 Wave 1 产物；HTTP 层未重写 SQL，`deriveEntryCapabilities` 仅做字符串等值比较，无 DB / host-agent 回路，满足 D-04。

## Automated Check Results

```
go test ./internal/store/repository/... -count=1 -short   → PASS
go test ./internal/store/repository/... -count=1          → PASS
go test ./internal/runtime/tasks/... -count=1 -short      → PASS
go test ./internal/controlplane/http/... -count=1 -short  → PASS
go test ./internal/cloudclaude/... -count=1 -short        → PASS
go test ./... -count=1 -short                             → PASS (cloudclaude, controlplane/http, controlplane/scheduler, network, runtime/tasks, store/repository)
```

## Threat Model Coverage (from PLAN.md threat tables)

| Threat | Status | Evidence |
|--------|--------|----------|
| T-30-01 SQL injection in repository | ✓ mitigated | 三条新增 SQL (`resolveClaudeAccountByHostSQL` / `resolveClaudeAccountByUserFallbackSQL` / `getHostByShortIDSQL`) 均使用 `$1` 占位；`TestWave1_DataLayerBoundary` 断言「无 `fmt.Sprintf` / `||` 拼接」 |
| T-30-02 migration idempotency | ✓ mitigated | `ADD COLUMN IF NOT EXISTS` + `TestMigration0014_FileContent` |
| T-30-03 claude_account 存在探针 | ✓ accepted | `ResolveClaudeAccountIDForEntry` 返回 `(id, ok=false, nil)`；`entry_auth_test.go::TestEntryAuth_Ready_NoClaudeAccount_OmitsField` 验证 miss 不外泄 |
| T-30-04 handler key 拼写漂移 | ✓ mitigated | `entry_auth_test.go` 对 ready 路径 5 个键逐项断言 |
| T-30-05 capability / account_id 过度暴露 | ✓ mitigated | 仅 ready + 密码校验通过后拼装；`not_ready` / `401` / `403` / `404` / `500` 路径单独 `writeJSON` |
| T-30-06 supports 推导误判 | ✓ accepted | 误判永远朝 false 降级；`deriveEntryCapabilities` 等值比较 `v3.0.0` |
| T-30-07 v2/v3 兼容回归 | ✓ mitigated | `entry_compat_test.go` + `worker_volume_test.go` 双向兼容测试；所有新字段 `omitempty` |

## Open Questions — Resolved in Discuss Phase

- **Q4** volume 命名：已定稿为单 volume `claude-state-{claude_account_id}` + label `com.cloud-cli-proxy.account_id`（30-CONTEXT D-01）
- **Q5** 扩展方式：已定稿在现有 `/v1/entry/{id}/auth` 加字段，不新增 `/capabilities`（D-03）
- **Q6** host-agent 是否扩展返回 image labels：定稿不扩展，控制面仅依据 `template_image_ref` 推导（D-04）

## Known Items Requiring Later Action

- **Code Review Minor M1**（见 `30-REVIEW.md`）：`SupportsMutagen`/`SupportsMergerfs` 使用 `bool + omitempty` 无法区分「显式 false」与「未返回」，Phase 30 内零影响；Phase 31/33 若引入转发逻辑需复核或改 `*bool`。
- **Code Review Nit N1/N2**：注释可读性微调，可与 Phase 31 开工时顺手修正。

## Human Verification Items

以下项目不在单元测试可覆盖范围内，需真实环境确认（详见 frontmatter `human_verification`）：

1. **hv-01**：在真实 Postgres 18.x 宿主上跑 migrator 升级路径（v2.0 → v3.0），确认 0014 在空库与已有数据的升级库上均可 up。（`ADD COLUMN IF NOT EXISTS` 理论幂等，但真实 DB 回归仅由运维执行。）
2. **hv-02**：生产或预发布 gateway 接入 v2.0 cloud-claude 二进制时，验证旧客户端可正常完成 ready 流，v3 扩展字段被静默忽略。（自动化仅验证 JSON 结构兼容，未验证端到端进程。）

## Verdict

**Phase 30 verification passed (4/4 success criteria).**

- 数据模型层（migration 0014、`PersistentVolumeName *string`、`ResolveClaudeAccountIDForEntry`、`HostSSHAuth.TemplateImageRef`）完整就位。
- API 契约层（`HostActionRequest.ClaudeAccountID`、Entry `/auth` 响应扩展、`cloudclaude.AuthResponse`）向后兼容并有测试锁定。
- Wave 1 / Wave 2 边界清晰，Phase 31（mount 重构）、Phase 33（volume 编排）可直接消费。
- 存在 2 条人工验证项（真实 DB 升级、v2 客户端端到端）需运维在上线窗口完成。

推荐继续推进到 Phase 31。
