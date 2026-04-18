---
phase: 30-entry-api
plan: 02-entry-api-host-contract
type: execute
wave: 2
depends_on:
  - 01-migration-entry-store
files_modified:
  - internal/agentapi/contracts.go
  - internal/runtime/tasks/worker_volume_test.go
  - internal/controlplane/http/entry.go
  - internal/controlplane/http/entry_caps_test.go
  - internal/controlplane/http/entry_auth_test.go
  - internal/cloudclaude/entry.go
  - internal/cloudclaude/entry_compat_test.go
autonomous: true
requirements:
  - REQ-F7-A
must_haves:
  truths:
    - "`HostActionRequest` 含 `ClaudeAccountID`，JSON `claude_account_id,omitempty`（D-09）；旧 JSON 反序列化仍兼容"
    - "`POST .../auth` 在 `status=ready` 时输出 `image_version`、`supports_mutagen`、`supports_mergerfs`；`claude_account_id` 在 `ok=true` 时非空（D-08）；`not_ready` 可不带来扩展字段（D-08）"
    - "`AuthResponse` 扩展字段对旧客户端安全（未知 JSON 忽略）；新字段缺失时零值语义可测（D-03 兼容）"
  artifacts:
    - path: internal/agentapi/contracts.go
      provides: "ClaudeAccountID 字段"
      contains: claude_account_id
    - path: internal/controlplane/http/entry.go
      provides: "EntryStore 扩展 + Auth 响应 map 追加键"
      contains: image_version
    - path: internal/cloudclaude/entry.go
      provides: "AuthResponse 新字段"
      contains: image_version
  key_links:
    - from: entry Auth ready 分支
      to: Repository.ResolveClaudeAccountIDForEntry
      via: "EntryStore 新方法"
      pattern: ResolveClaudeAccountIDForEntry
    - from: template_image_ref
      to: image_version / supports_*
      via: "纯函数推导（D-06/D-07）"
      pattern: ParseImageVersionFromTemplateRef
---

## 目标

在 **不新增 HTTP 路由**（D-03）前提下，完成：`HostActionRequest` 的 `ClaudeAccountID` 契约（D-09）、Entry `/v1/entry/{shortId}/auth` **ready** 响应扩展（D-03、D-06、D-07、D-08）、`cloudclaude.AuthResponse` 对齐（D-03），并用自动化测试锁住 v2/v3 兼容语义（ROADMAP Success Criteria 2–3）。

## 执行上下文

- 依赖 plan `01-migration-entry-store` 已提供的仓储方法与 `HostSSHAuth.TemplateImageRef`。
- 锁定决策：`.planning/phases/30-entry-api/30-CONTEXT.md`

## 范围说明

### 纳入

- **`internal/agentapi/contracts.go`**：`HostActionRequest` 在 `Volumes` 字段后（或语义相近位置）增加 `ClaudeAccountID string \`json:"claude_account_id,omitempty"\``（D-09）。**不得**改动已有字段名/tag 顺序的兼容红线（与 Phase 29 plan 04 一致）。
- **`internal/runtime/tasks/worker_volume_test.go`**（或并列测试文件）：补充 `HostActionRequest` JSON round-trip：仅发旧字段 → `ClaudeAccountID==""`；带 `claude_account_id` → 正确解析。
- **`internal/controlplane/http/entry.go`**：
  - `EntryStore` 接口增加 `ResolveClaudeAccountIDForEntry(ctx, hostID, userID string) (id string, ok bool, err error)`（签名与 plan 01 实现一致）。
  - `Auth` 成功且 `hostStatus==running`（保持现有判定）时，在现有 `map[string]any` 上追加：`image_version`、`supports_mutagen`、`supports_mergerfs`、`claude_account_id`（均为 `omitempty` 语义：零值不写入或省略）。`not_ready` 分支保持现状，**不强制**追加扩展字段（D-08）。
  - `image_version`：从 `TemplateImageRef` 取最后一个 `:` 后子串，无 `:` 则用整串 trim（D-06）。
  - `supports_mutagen` / `supports_mergerfs`：当且仅当 `image_version == "v3.0.0"` 时为 `true`，否则 `false`（D-07）。
  - `claude_account_id`：调用 `Resolve...`；`ok==false` 则省略该键（D-05）；`ok==true` 必须写入非空 UUID 字符串（D-08 在账号存在前提下非空——此处 `ok` 即存在）。
  - **用户 short_id 回退路径**：使用 `GetPrimaryHostByUserID` 返回的 `Host.TemplateImageRef`；`host_id` 用 `primaryHost.ID` 调 Resolve。
  - **主机 short_id 路径**：使用 `GetHostByShortID` 返回的 `TemplateImageRef` 与 `HostID`。
- **`internal/cloudclaude/entry.go`**：`AuthResponse` 增加 `ImageVersion`、`SupportsMutagen`、`SupportsMergerfs`、`ClaudeAccountID`（推荐指针或 `omitempty` 与 JSON 省略一致）；**不得**收紧现有 `ready` 四元组校验逻辑使旧响应失败（仅当 `status==ready` 时仍校验 SSH 四字段）。
- **测试**：`entry_caps_test.go` 表驱动测 tag 解析与 supports 推导；`entry_auth_test.go` 用 `httptest` + **stub EntryStore** 断言 ready JSON 同时包含四个新键（在 stub 返回 `template_image_ref` 含 `:v3.0.0` 且 Resolve 返回 `ok=true` 时）；`entry_compat_test.go` 旧 `AuthResponse` 反序列化带未知字段的 JSON 不报错；新结构反序列化缺字段时布尔为 false / 指针 nil。

### 排除

- host-agent 镜像 label 拉取（D-04）；admin UI；真实 E2E 二进制矩阵（以 `go test` 为主）。

<tasks>

<task type="auto" tdd="true">
  <name>任务 1：HostActionRequest.ClaudeAccountID + JSON 测试</name>
  <files>internal/agentapi/contracts.go,internal/runtime/tasks/worker_volume_test.go</files>
  <behavior>
    - 测试：旧 JSON（无 `claude_account_id`）反序列化后字段为空。
    - 测试：含 `"claude_account_id":"550e8400-e29b-41d4-a716-446655440000"` 时 round-trip 保留。
  </behavior>
  <action>按 D-09 添加字段；测试放 worker 包以复用 Phase 29 JSON 风格或改 `agentapi` 包内 `contracts_test.go`（若新建文件则更新 `files_modified` 列表）。**禁止**修改 `VolumeMount` / `Volumes` 语义。</action>
  <verify>
    <automated>go test ./internal/runtime/tasks/... -count=1 -short</automated>
  </verify>
  <done>D-09 落地；`go test ./internal/runtime/tasks/...` 绿灯。</done>
</task>

<task type="auto" tdd="true">
  <name>任务 2：纯函数 image_version / supports + EntryStore 与 Auth 响应</name>
  <files>internal/controlplane/http/entry.go,internal/controlplane/http/entry_caps_test.go,internal/controlplane/http/entry_auth_test.go</files>
  <behavior>
    - `entry_caps_test.go`：`registry.example.com/ccp:v3.0.0` → `v3.0.0`，supports 均为 true；`foo:1.2` → `1.2`，supports false；无冒号整串；空白 trim。
    - `entry_auth_test.go`：stub 返回 running + `template_image_ref` + Resolve ok → 响应 JSON 含四个新键且值符合 D-07；Resolve `ok=false` → 无 `claude_account_id` 键但有 `image_version` 与 supports（可能 false）。
  </behavior>
  <action>实现推导函数（同包内未导出小写即可）；扩展 `EntryStore` 与 `Auth`（D-03、D-05–D-08）。所有实现注释与用户可见文案用中文（CONVENTIONS）。**Repository 具体实现已在 plan 01**——本任务仅接线与 handler。</action>
  <verify>
    <automated>go test ./internal/controlplane/http/... -count=1 -short</automated>
  </verify>
  <done>`go test ./internal/controlplane/http/...` 绿灯；ready 路径行为符合 D-08。</done>
</task>

<task type="auto" tdd="true">
  <name>任务 3：cloudclaude.AuthResponse 扩展与兼容单测</name>
  <files>internal/cloudclaude/entry.go,internal/cloudclaude/entry_compat_test.go</files>
  <behavior>
    - 旧五字段 + 新扩展字段 JSON → 旧字段 `Authenticate` 仍过校验。
    - 仅五字段 JSON → 新能力字段零值不阻断连接。
  </behavior>
  <action>扩展结构体与文档注释（中文说明 v3 字段含义）；**不得**在 `Authenticate` 对 `SupportsMutagen` 等新增强制校验以免破坏旧网关（D-03）。对应 D-03。</action>
  <verify>
    <automated>go test ./internal/cloudclaude/... -count=1 -short</automated>
  </verify>
  <done>`go test ./internal/cloudclaude/...` 绿灯。</done>
</task>

</tasks>

<threat_model>
## 信任边界

| 边界 | 说明 |
|------|------|
| 互联网客户端 → Entry `/auth` | 密码与入口 shortId 属未信任输入；响应体影响 CLI 后续行为 |

## STRIDE 登记

| 威胁 ID | 类别 | 组件 | 处置 | 缓解措施 |
|---------|------|------|------|----------|
| T-30-04 | I | Entry `Auth` 响应 | mitigate | 仅追加非机密能力字段；`claude_account_id` 为 UUID，不扩大口令面；保持 HTTPS 部署指引在运维文档（既有假设） |
| T-30-05 | T | `map[string]any` 拼键 | mitigate | 单元测试锁定键名拼写；`ready` 仍仅于认证成功后返回 |
| T-30-06 | E | 错误推断 `image_version` | accept | 错误 tag 仅导致能力为 false；不授予额外特权 |
| T-30-07 | I | `HostActionRequest` JSON | mitigate | `omitempty` + 测试覆盖旧 payload；与 Phase 29 worker 行为一致 |
</threat_model>

<verification>
- `go test ./internal/controlplane/http/... ./internal/cloudclaude/... ./internal/runtime/tasks/... -count=1 -short` 全部通过。
- `go vet ./...` 若 CI 已启用则执行器应本地跑通（可选写入 SUMMARY）。
</verification>

<success_criteria>
- ROADMAP 本 phase Success Criteria 2–3 由自动化测试等价覆盖（旧 client JSON、新字段读取）。
- D-04、D-12 未触碰。
</success_criteria>

<output>
完成后撰写 `.planning/phases/30-entry-api/plans/02-entry-api-host-contract/SUMMARY.md`。
</output>

## 来源审计（规划闭环）

| 来源 | 条目 | Plan | 状态 |
|------|------|------|------|
| GOAL | Entry API 扩展 + HostActionRequest 账号维度 | 02 | COVERED |
| REQ | REQ-F7-A（握手侧账号标识与后续 volume 名一致） | 02 | COVERED |
| RESEARCH | entry.go / cloudclaude / contracts / 测试策略 | 02 | COVERED |
| CONTEXT | D-03..D-09,D-12（排除） | 02 | COVERED |
