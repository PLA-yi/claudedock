# Phase 33: Claude Code 状态持久化（CLI + 镜像 + admin GC） - Context

**Gathered:** 2026-04-21
**Status:** Ready for planning
**Mode:** `--auto`（所有 gray areas 取推荐默认值，决策依据 ROADMAP scope / Success Criteria 字面 + Phase 29/30 已锁定边界）

<domain>
## Phase Boundary

让 OAuth credentials 与 Claude Code 缓存（`~/.claude` + `~/.cache/claude`）跨容器重建持久化，并保证账号删除时 volume 被同步清理。本阶段交付 F7 完整闭环（除已在 Phase 31 落地的 F7-C 连接前 OAuth 中文提示外）。

具体范围：

1. **镜像 entrypoint** — 在现有 v3 stage 串行编排里补一个 `prepare_persistent_state` 阶段：把 `/var/lib/claude-persist`（Phase 33 新挂载的 named volume）下的 `.claude` / `.cache/claude` 子目录初始化（首次为空时 `cp -an` seed 镜像预建内容）+ `chown -R 1000:1000` + `ln -sfn` 把 `/home/claude/.claude` 与 `/home/claude/.cache/claude` 指向 volume；保留 Phase 29 D-09 已写的二次 `chown` 兜底。
2. **Worker `createHost`** — 在 `buildCreateArgs` 之前插入 `ensureDockerVolume(name, labels)` 幂等创建；当 `request.ClaudeAccountID != ""` 时由 worker 自动补一条 `VolumeMount{Name: "claude-state-{id}", Target: "/var/lib/claude-persist", Labels: {...}}`，并把 `account_id` 落到 `claude_accounts.persistent_volume_name`（如尚未分配）。
3. **agentapi 协议** — 新增 `HostAction` 常量 `ActionVolumeRemove`，请求体复用 `HostActionRequest.Volumes`（仅 `Name` 必填，`Target/ReadOnly/Labels` 忽略）；host-agent 路由仍是 `POST /v1/host-actions`（**不**新增 endpoint，与 Phase 29 D-22 / Phase 30 D-04 边界一致）。
4. **admin DELETE claude_account handler** — 新增 `DELETE /v1/admin/claude-accounts/{id}`（若已存在则增强）：在 SQL 事务内同步调用 host-agent `ActionVolumeRemove`，rm 失败 → 事务回滚 + 写 audit event；可选 `?force=true` 走最终一致路径（DB 删完 best-effort + 必传 `docker volume rm -f`）。
5. **admin host 详情页** — 在现有 `GET /v1/admin/hosts/{id}` 响应里追加 `persistent_volume_name` 字段（来源：`hosts → claude_accounts join`），不新增页面、不改 list endpoint（OOS-A19 边界 "最多加一行"）。
6. **测试 + UAT** — Worker `ensureDockerVolume` 幂等单测、agentapi `ActionVolumeRemove` round-trip、admin handler 回滚单测、entrypoint symlink seed 行为脚本测试；人工 UAT 删 account 后 `docker volume ls --filter label=...` 必须空。

本阶段**不**交付：
- `cloud-claude` 客户端任何变更（持久化对客户端透明）
- 独立 GC / orphan volume 清扫定时任务（推迟到 v3.1 backlog）
- 双 volume / 子目录直挂等替代拓扑（Q4 已在 Phase 30 D-01 锁定单 volume）

</domain>

<decisions>
## Implementation Decisions

### Volume 命名与 label 规范

- **D-01**：volume 名 `claude-state-{claude_account_id}`，其中 `{claude_account_id}` **保留 PostgreSQL UUID 原格式（含连字符）** 与 `claude_accounts.id` 字面一致。理由：(1) Docker volume name 允许 `-`；(2) DB 与 volume 名一一可视对照便于运维 `grep`；(3) Phase 30 D-01 措辞 "由实现选定但全仓库一致" 在此具体化。
- **D-02**：volume 创建时强制写入两条 label：
  - `com.cloud-cli-proxy.account_id={claude_account_id}` —— 唯一性键，filter 入口
  - `com.cloud-cli-proxy.managed=true` —— 二级保险，配合 `docker volume ls --filter label=com.cloud-cli-proxy.managed` 做存量审计
  本阶段**不**写 `com.cloud-cli-proxy.created_at`（避免与 Docker 自带 `CreatedAt` 重复 + 不便于 label-filter 精确匹配）。
- **D-03**：`hosts.template_image_ref` 与 `image_version` 不影响 volume 名。volume 完全按 `claude_account_id` 切分，与镜像版本解耦。

### Volume 创建触发链路（Worker 端）

- **D-04**：worker `createHost` 在 `buildCreateArgs` 之前调用 `ensureDockerVolume(ctx, name, labels)`（包级函数 + `var ensureDockerVolume = realEnsureDockerVolume` 模式便于注入 mock，沿用 Phase 29.1 Plan 02 的 `execInContainer` 提升模式）。函数语义：
  1. `docker volume inspect <name>`：存在则比对 label，缺失或不一致写 audit event 但**不**重建（幂等优先）；
  2. 不存在则 `docker volume create --label k=v --label k=v <name>`，失败返回 `volume_create_failed` 错误码；
  3. 总是返回 `(name, nil)`，后续 `--mount` 拼装才能继续。
- **D-04a**：D-04 第 1 条的 "label 比对" 推迟到 v3.1 backlog。理由：当前 v3.0 单 worker 实现路径下不会出现 label 漂移（worker 自动写 label 是包级常量），引入 inspect JSON 解析增加复杂度但风险面极低；M16 的覆盖由 admin DELETE 联动 + 运维手册 orphan 审计脚本兜底。Plan 01 Task 1.3 `realEnsureDockerVolume` `inspect` 成功直接 `return nil` 与本决策一致；D-25.2 测试覆盖只保留 "NotExists_RunsCreate" + "AlreadyExists_SkipsCreate"。
- **D-05**：worker 在 `request.ClaudeAccountID != ""` 时**自动补一条** `VolumeMount{Name: BuildClaudeStateVolumeName(id), Target: "/var/lib/claude-persist", Labels: map[string]string{"com.cloud-cli-proxy.account_id": id, "com.cloud-cli-proxy.managed": "true"}}`，避免上游 dispatcher 每个调用方都重复拼。若 `request.Volumes` 已显式包含同 `Name`，跳过补写（显式优先）。
- **D-06**：worker `createHost` 成功后调用 `repo.UpsertClaudeAccountPersistentVolumeName(ctx, accountID, name)`（仓储侧新增方法，仅在当前列为 NULL 时写入；非空一致跳过、非空冲突写 audit event）。`persistent_volume_name` 列从 `NULL` 变为已分配后**永不回写 NULL**，与 Phase 30 D-02 三态消除一致。
- **D-07**：本阶段**不**改 dispatcher / scheduler 链路上传入 `ClaudeAccountID` 的来源——已在 Phase 30 D-09 由控制面 `EntryHandler` / `runtime_service.go` 注入；Phase 33 仅消费现有字段。如果某一调用路径仍未填 `ClaudeAccountID`（例如 v2.0 旧 host 重建），worker 跳过 volume 补写 + entrypoint symlink 走 fallback（`/var/lib/claude-persist` 内空 + symlink 仍生效但无持久化数据），**不**报错阻塞 host 启动（M16 风险但不破坏现有路径）。

### Entrypoint 持久化拓扑（镜像侧）

- **D-08**：单 volume 挂载点固定为 `/var/lib/claude-persist`（与 Phase 29 D-16 预建目录一致）。**不**采用 "为每个子目录单独挂 volume" 或 "Docker subpath 挂载"（subpath 在 docker engine 25.0+ 才稳定，且服务端版本不可控）。
- **D-09**：entrypoint 新增函数 `prepare_persistent_state`，插入位置：在 `prepare_v3_dirs` 之后、`prepare_mutagen_agent` 之前（先把 home dir 物理位置摆好，再让 mutagen agent 看到稳定路径）。函数体伪码：
  ```sh
  prepare_persistent_state() {
    local root=/var/lib/claude-persist
    mkdir -p "$root/.claude" "$root/.cache/claude"
    # 首次挂载（volume 为空）时 seed 镜像预建内容；-a 保留权限/时间戳，-n 不覆盖已有文件
    if [ -d /home/claude/.claude ] && [ -z "$(ls -A "$root/.claude" 2>/dev/null)" ]; then
      cp -an /home/claude/.claude/. "$root/.claude/" 2>/dev/null || true
    fi
    if [ -d /home/claude/.cache/claude ] && [ -z "$(ls -A "$root/.cache/claude" 2>/dev/null)" ]; then
      cp -an /home/claude/.cache/claude/. "$root/.cache/claude/" 2>/dev/null || true
    fi
    chown -R 1000:1000 "$root"
    # 替换为 symlink；ln -sfn 处理已存在的目录或 link
    rm -rf /home/claude/.claude /home/claude/.cache/claude
    ln -sfn "$root/.claude" /home/claude/.claude
    mkdir -p /home/claude/.cache
    ln -sfn "$root/.cache/claude" /home/claude/.cache/claude
    chown -h 1000:1000 /home/claude/.claude /home/claude/.cache/claude
  }
  ```
  关键不变量：
  1. **幂等**：volume 已有内容时不覆盖（`cp -an`），symlink 重复创建不报错（`ln -sfn`）；
  2. **权限**：volume 内容物 + symlink 链接本身都是 `1000:1000`（防御 PITFALLS M17）；
  3. **不阻塞**：`cp -an` 失败 `|| true`（极端场景下 volume 上有 immutable 文件不应让容器起不来）。
- **D-10**：保留 Phase 29 D-09 现有 `prepare_v3_dirs` 的二次 `chown -R 1000:1000 /var/lib/claude-persist`（兜底）。新加的 `prepare_persistent_state` 在其后执行，叠加防御。
- **D-11**：**不**对 `~/.claude/.credentials.json` 做加密/读写分离/备份等额外处理。OAuth credentials 文件由 Claude Code 自身格式负责；本阶段只保证 volume 内容不丢、权限正确。
- **D-12**：`/home/claude/.cache` 父目录在镜像里预建 `chown 1000:1000`（Phase 29 D-16 已含），entrypoint `mkdir -p /home/claude/.cache` 仅作兜底（容器在某些极端场景下 Dockerfile layer 可能丢）。

### host-agent 协议扩展

- **D-13**：`internal/agentapi/contracts.go` 新增 Action 常量：
  ```go
  ActionVolumeRemove HostAction = "volume_remove"
  ```
  请求体复用 `HostActionRequest`：`Action=volume_remove` 时只读 `Volumes []VolumeMount.Name`（Target/ReadOnly/Labels 忽略），其余字段（HostID/ContainerName/EntryPassword 等）允许为空。
- **D-14**：worker `Execute()` switch 新增 case `agentapi.ActionVolumeRemove`：`for vm := range request.Volumes` 调用 `removeDockerVolume(ctx, vm.Name, force)` —— 默认 `force=false`，通过 `request.Labels["force"] == "true"` 携带（避免再扩字段）。
- **D-15**：`removeDockerVolume` 实现：
  - `force=false`：执行 `docker volume rm <name>`，volume 仍被容器持有时 docker 会返回 "volume is in use"，**直接传播错误**给上游事务回滚（D-19）；
  - `force=true`：执行 `docker volume rm -f <name>`，仅在管理员显式 `?force=true` 时启用；
  - volume 不存在（`No such volume`）视为成功（幂等）。
- **D-16**：本阶段**不**新增 `/v1/volumes/*` REST endpoint。host-agent `mux` 仅 `GET /healthz` / `GET /v1/containers/{name}/status` / `POST /v1/host-actions` 三个 route 不变。理由：减少协议表面、与 Phase 29/30 决策一致。

### admin DELETE claude_account 事务模型

- **D-17**：新增 handler `DELETE /v1/admin/claude-accounts/{id}`（若现有 admin 路由已经有占位则增强；当前 grep 显示 `claude_accounts` 只在仓储/migration 出现，控制面尚无 CRUD，本阶段一并补齐 DELETE，**不**做 CREATE/UPDATE handler——超出 scope）。Query 参数：
  - `force=true`（可选）：走 D-19 最终一致路径
- **D-18**：默认强一致路径（无 `force` 或 `force=false`）：
  1. `BEGIN TRANSACTION`
  2. `SELECT id, persistent_volume_name FROM claude_accounts WHERE id = $1 FOR UPDATE`
  3. 若 `persistent_volume_name IS NOT NULL`：通过 `agentapi.Client.RunHostAction(ctx, HostActionRequest{Action: ActionVolumeRemove, Volumes: [{Name: name}]})` 同步调用 host-agent，`ctx` 带 10s 超时；
  4. host-agent 返回非成功 → `ROLLBACK` + 写 audit event `claude_account.delete_volume_rm_failed`（metadata: account_id, volume_name, error_code, error_message）+ 返回 HTTP 409 `volume_in_use`（中文提示 "请先停止使用该账号的所有 host 后重试，或追加 ?force=true 强删 volume"）；
  5. host-agent 成功 → `DELETE FROM claude_accounts WHERE id = $1` + `COMMIT` + 写 audit event `claude_account.deleted`（metadata: account_id, volume_name）。
- **D-19**：`force=true` 最终一致路径：
  1. `BEGIN TRANSACTION` → `DELETE FROM claude_accounts WHERE id = $1` → `COMMIT`（DB 状态先收口）；
  2. 异步（同请求内但事务外）调用 host-agent `ActionVolumeRemove`，`Labels: {"force": "true"}` → host-agent 走 `docker volume rm -f`；
  3. rm 失败：写 audit event `claude_account.force_volume_rm_failed`（metadata 记 volume name + 错误码）+ 返回 HTTP 200 with body `{"deleted": true, "volume_rm": "failed", "next_action": "运维需手工 docker volume rm -f <name>"}`；rm 成功返回 `{"deleted": true, "volume_rm": "succeeded"}`。
  4. **理由**：force 出口避免容器还在跑时管理员永远删不掉账号；同时把 orphan 风险显式上报（不静默丢失）。
- **D-20**：HTTP 调用 host-agent 的超时：
  - 强一致路径 ctx 超时 10s（事务长度上限）；
  - force 路径 ctx 超时 30s（rm -f 仍受 docker daemon 调度时间影响）。
- **D-21**：审计事件命名：使用 `claude_account.*` 前缀（与现有 `runtime.*` / `host.*` 同形式），所有事件 metadata 严守 Phase 29.1 Rule "不写明文密码 / 不写凭据"（claude_account 本身有 `email`，可记 account_id + email 但不记 OAuth token）。

### admin host 详情页扩展

- **D-22**：在现有 `GET /v1/admin/hosts/{id}` handler 响应 JSON 顶层追加 `persistent_volume_name`（值取自 `claude_accounts join hosts on host_id`，可能为 `null`）；其余字段顺序 / 类型不变。**不**改 list endpoint `/v1/admin/hosts`（OOS-A19 "最多加一行" 解释为 "host 详情页加一字段"，list 保持精简）。
- **D-23**：实现路径：
  - 仓储新增方法 `GetHostWithClaudeAccount(ctx, hostID) (HostDetail, error)` 单次 LEFT JOIN，避免 N+1；
  - 现有 `GetHost` / `ListHostsWithUsername` 保持不变（Phase 29.1 已锁定的 6 个 SELECT 语句不动）；
  - handler 把新字段以 `omitempty` 形式追加，旧前端二进制忽略未知字段（与 Phase 30 D-03 兼容策略一致）。
- **D-24**：用户面 `GET /v1/user/hosts/{id}` **不**追加 `persistent_volume_name`（用户视角不需要 volume 名；如有需求开 v3.1 phase）。

### 单元测试与 UAT 范围

- **D-25**：单元测试覆盖：
  1. `BuildClaudeStateVolumeName(id)` 字符串拼接 + 边界（空 id 返回错误）；
  2. `ensureDockerVolume` 幂等（mock docker：第一次创建 / 第二次 inspect 命中 / label 不一致写 audit）；
  3. `removeDockerVolume` 幂等（不存在 → 成功）+ in-use 失败传播 + force=true 路径；
  4. `agentapi.HostActionRequest` round-trip（Action=volume_remove）；
  5. admin DELETE handler：mock host-agent 成功 → DB 删除；mock 失败 → 事务回滚 + DB 行仍在；force=true → rm 失败但 DB 已删；
  6. `Repository.GetHostWithClaudeAccount` LEFT JOIN 返回 `nil` 与命中两条路径；
  7. `Repository.UpsertClaudeAccountPersistentVolumeName` 只在 NULL 时写入 + 一致跳过 + 冲突写 audit（用 mock recorder）。
- **D-26**：人工 UAT 清单（Plan 02 收尾）：
  1. 删除一个未运行 host 的 account → `docker volume ls --filter label=com.cloud-cli-proxy.account_id=<id>` 必须空；
  2. 删除一个有运行 host 的 account（默认 force=false）→ HTTP 409 + 中文提示 + DB 行仍在；
  3. 加 `?force=true` 重试 → HTTP 200 + DB 行删 + volume 删；
  4. 同一 account 容器 stop → start 后 `~/.claude/.credentials.json` 内容不变；
  5. 重建容器 (rebuild) 后 OAuth credentials 仍可用（不需 `claude login`）。
- **D-27**：**不**写 docker compose fixture 集成测试（沿用 Phase 31 / 32 的 t.Skip 风格留 Phase 35 真机），但 `removeDockerVolume` mock 必须覆盖 docker 实际错误字符串 `"volume is in use"` 与 `"No such volume"` 两种返回。

### Plan 拆分

- **D-28**：本阶段拆分为 **2 plans**（与 ROADMAP 字面 `0/2 plans` 一致）：
  - **Plan 01 — 镜像 + Worker + agentapi**（Wave 1）：
    - `deploy/docker/managed-user/entrypoint.sh` 新增 `prepare_persistent_state`
    - `internal/agentapi/contracts.go` 新增 `ActionVolumeRemove`
    - `internal/runtime/tasks/worker.go` 新增 `ensureDockerVolume` / `removeDockerVolume` / `BuildClaudeStateVolumeName` / `Execute()` switch 扩展 / `createHost` 自动补 volume 逻辑
    - `internal/store/repository/queries.go` 新增 `UpsertClaudeAccountPersistentVolumeName`
    - 单元测试 D-25 第 1-4 + 第 7 项
  - **Plan 02 — admin DELETE + host detail + UAT**（Wave 2，依赖 Plan 01）：
    - `internal/controlplane/http/admin_claude_accounts.go` 新建（DELETE handler + force flag）
    - `internal/controlplane/http/admin_hosts.go` GET detail 追加 `persistent_volume_name`
    - `internal/store/repository/queries.go` 新增 `GetHostWithClaudeAccount` LEFT JOIN
    - `internal/controlplane/http/router.go` 注册新 route（adminGuard 链路）
    - 单元测试 D-25 第 5-6 项
    - UAT 清单 D-26 + 运维手册章节（孤儿 volume 审计脚本）
- **D-29**：Plan 02 的 admin handler 严守 Phase 29.1 Plan 02 已建立的"runtime/worker fail-fast + audit event"模式，**不**新引入 service 层 / use case 层抽象。

### Claude's Discretion

以下细节由 planner / executor 按实现便利性决定，不阻塞 plan-phase：

- `cp -an` vs `rsync -a --ignore-existing` 做 seed（推荐 `cp -an`，体积零依赖）
- `BuildClaudeStateVolumeName` 是否做长度截断（UUID 36 字符 + `claude-state-` 13 字符 = 49，远低于 Docker 255 上限，无需截断）
- audit event 的 `metadata` 具体字段顺序（保持 `account_id` / `volume_name` / `error_code` / `error_message` 即可）
- `removeDockerVolume` 是否在 `force=false` 时先 inspect 一次 fast-fail（推荐：直接 rm 让 docker 自己判断，减少调用）
- handler 的 query 参数解析（`force=true` / `1` / `yes` 三种均接受 vs 仅 `true`，由 planner 决定）
- 仓储新增方法是否复用 `Repository` 还是新增 `claude_account_repository.go`（推荐复用现有 `repository` 包，与 Phase 30 风格一致）

### Folded Todos

无（`todo match-phase 33` 返回 0 条匹配）。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 项目规划与需求

- `.planning/PROJECT.md` — v3.0 总目标 / F7 持久化方向 / "沟通语言中文" 约束
- `.planning/REQUIREMENTS.md` §F7 — REQ-F7-A / REQ-F7-B / REQ-F7-D（REQ-F7-C 已在 Phase 31 落地）
- `.planning/REQUIREMENTS.md` §Critical Pitfalls — **C5**（M17 同源 chown 防御）/ **M16**（orphan volume）/ **M17**（named volume 权限初始化）— 本阶段必须显式防御
- `.planning/REQUIREMENTS.md` §Open Questions — Q4 已在 Phase 30 D-01 锁定，本阶段沿用
- `.planning/REQUIREMENTS.md` §Out of Scope — **OOS-A19**（admin 只加一行字段，不新增管理页）
- `.planning/ROADMAP.md` §Phase 33 — Goal / Scope / 6 条 Success Criteria / Open questions
- `.planning/STATE.md` — v3.0 milestone 当前进度 + Phase 32 进入 verifier 后 Phase 33 启动序列
- `.planning/codebase/CONVENTIONS.md` — 中文沟通强制 + 禁绝对路径 / 凭据规范

### 前置阶段上下文

- `.planning/phases/29-v3-worker/29-CONTEXT.md` — D-09 entrypoint 串行编排 + D-16 `/var/lib/claude-persist` 预建 + D-18..D-22 `VolumeMount` / `HostActionRequest.Volumes` 契约 + D-22 向后兼容 omitempty + D-21 `ClaudeAccountID` 留 Phase 30
- `.planning/phases/30-entry-api/30-CONTEXT.md` — D-01/D-02 单 volume 命名 + label `com.cloud-cli-proxy.account_id` 锁定 + D-09 `HostActionRequest.ClaudeAccountID` 字段 + D-04 不扩展 host-agent endpoint + D-10 migration 0014
- `.planning/phases/29.1-gethost-entry-password-workspace/29.1-CONTEXT.md` — runtime/worker fail-fast + audit event 模式（Plan 02），本阶段 admin handler 沿用
- `.planning/phases/31-cli/31-CONTEXT.md` — REQ-F7-C 在 Phase 31 落地的具体 hook 点（与本阶段 REQ-F7-A/B/D 在客户端侧无交集）
- `.planning/phases/32-ssh-tmux/32-CONTEXT.md` — 账号级 Mutagen 单例锁实现（与 Phase 33 volume 命名共享 `claude_account_id` 维度）

### 研究基线

- `.planning/research/SUMMARY.md` §3 REQ-F7-A/B/D + §5 TOP 10（M16 / M17）+ §P5 行 "Worker docker volume create 幂等 / admin DELETE 联动 volume rm"
- `.planning/research/STACK.md` §持久化 — Docker named volume 配置 / 备份策略 / 首次创建语义
- `.planning/research/PITFALLS.md` **M16**（named volume 不被 prune 默认清理 + label-filter 审计脚本）/ **M17**（named volume 首次挂载继承镜像目录权限 / chown 兜底）
- `.planning/research/ARCHITECTURE.md` §host-agent 边界 — 只扩 Action 不加 endpoint
- `.planning/research/FEATURES.md` §F7 持久化字段细节

### 既有代码（直接改造 / 必读对象）

- `deploy/docker/managed-user/entrypoint.sh` — Phase 29 D-09 v3 stages 已就位（`prepare_v3_dirs` + `prepare_mutagen_agent` + `prepare_mergerfs_check` + `assert_tmux_version`）；本阶段在 `prepare_v3_dirs` 之后插入 `prepare_persistent_state`
- `internal/agentapi/contracts.go` — `HostAction` 常量 + `VolumeMount` 类型 + `HostActionRequest.{Volumes,ClaudeAccountID}` 已就位（Phase 29 D-18/D-22 + Phase 30 D-09）；本阶段新增 `ActionVolumeRemove`
- `internal/runtime/tasks/worker.go` — `Execute()` switch（line 48-63）+ `buildCreateArgs`（line 135-192，已支持 `--mount type=volume,...`）+ `WorkerRepo` 接口（line 32-37，含 `RecordEvent`）；本阶段在 switch 加 case + 在 `createHost` 调用 `ensureDockerVolume` + `WorkerRepo` 接口扩展 `UpsertClaudeAccountPersistentVolumeName`
- `internal/store/repository/queries.go` — `resolveClaudeAccountByHostSQL` / `ResolveClaudeAccountIDForEntry`（line 1203-1258）展示 `claude_accounts` 查询风格；本阶段新增 `UpsertClaudeAccountPersistentVolumeName` + `GetHostWithClaudeAccount`
- `internal/store/repository/models.go` — `ClaudeAccount.PersistentVolumeName *string`（line 209-222，Phase 30 D-02）已就位
- `internal/store/migrations/0014_claude_account_persistent_volume.sql` — 列已添加，本阶段不再新增 migration
- `internal/agent/server.go` — host-agent mux 路由仅 3 个 route（healthz / containers status / host-actions），本阶段不动
- `internal/controlplane/http/admin_hosts.go` — `GET /v1/admin/hosts/{id}` handler 现状 + `getDockerStatuses` 函数（Phase 29.1 已识别为 docker daemon 依赖测试基础设施问题，本阶段 GetWithClaudeAccount 走纯 DB JOIN 不依赖 docker）
- `internal/controlplane/http/router.go` — admin route 注册风格（adminGuard 链 + middleware）

### 既有决策参考（模式延续）

- Phase 29.1 Plan 02 — runtime fail-fast + RecordEvent + 不写凭据 metadata 模式
- Phase 11 / 12 — admin handler / repository 接口 / 测试 stub 风格
- Phase 32 Plan 03 sync_lock — `flock` 路径 / shellescape / 包级 var mock 模式（本阶段 `var ensureDockerVolume = real...` 沿用）

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `internal/runtime/tasks/worker.go:buildCreateArgs` 已实现 `--mount type=volume,src=...,dst=...[,readonly]` 拼装（line 175-184，Phase 29.1 测试覆盖完整），本阶段无需重写
- `agentapi.HostActionRequest.{Volumes, ClaudeAccountID}` 字段已就位 + 已有完整 omitempty / round-trip 测试（`worker_volume_test.go`），本阶段仅扩 `Action` 常量
- `entrypoint.sh` 现有 `prepare_v3_dirs` 已包含 `chown -R 1000:1000 /var/lib/claude-persist`（line 62-69），本阶段叠加 `prepare_persistent_state` 二次保险
- `repository.ClaudeAccount.PersistentVolumeName *string` 三态语义（NULL = 未分配 / 非空 = 已分配）由 Phase 30 锁定，本阶段直接消费
- `repository.ResolveClaudeAccountIDForEntry` 双阶段查询模式（host_id 优先 + user fallback）展示 `claude_accounts` 查询风格，本阶段 `GetHostWithClaudeAccount` 沿用 LEFT JOIN 风格
- `agentapi.Client.RunHostAction(ctx, HostActionRequest)` 已是 control-plane → host-agent 的统一调用入口；本阶段 admin handler 直接复用，不新增 client 方法
- Phase 29.1 Plan 04 已有 admin handler `POST /v1/admin/hosts/resync-passwords` 注入 `var syncContainerPassword` 的可测试模式，本阶段 admin DELETE handler 沿用 `var runHostAction = client.RunHostAction` 模式
- `WorkerRepo.RecordEvent` 接口已存在（Phase 29.1 Plan 02 扩展），本阶段直接使用，**不**重新扩展接口

### Established Patterns

- HTTP handler：`router.go` 的 `adminGuard` 中间件链 + handler 函数 `func(w http.ResponseWriter, r *http.Request)` 直接注册到 `http.ServeMux` 模式
- SQL：所有 SELECT 语句提升为包级 `const xxxSQL`（Phase 29.1 Plan 01 强制约定），新增查询遵循同样模式
- agentapi.Client：所有 host-agent 调用走 `RunHostAction` + 由 worker 内部 switch 分派 `Execute(ctx, request)`；`Execute` 返回 `TaskStatusUpdate`，error_code 走 `host_action_failed` / `*_failed` 字符串常量
- 单元测试：worker 单测用 `minimalCreateHostRequest()` 工厂；agentapi round-trip 用 `json.Marshal` + `strings.Contains`；mock docker 走包级 `var execInContainer = realExecInContainer` 模式（Phase 29.1 Plan 02 提升）
- audit event：`repository.RecordEventParams{Type: "<domain>.<event>", Subject: "...", Metadata: map}`，metadata 严守不写凭据/不写绝对路径
- 中文错误消息：HTTP error body 用 `{"error": {"code": "<DOMAIN>_<KIND>_<NUM>", "message": "中文原因", "next_action": "中文下一步"}}`（Phase 31/32/34 错误码体系，本阶段沿用 `STATE_*` 前缀）

### Integration Points

- worker 自动补 volume 依赖 `request.ClaudeAccountID`（Phase 30 D-09 由 EntryHandler 在 control-plane → host-agent 路径填充；查 `internal/runtime/runtime_service.go` 与 `internal/controlplane/http/bootstrap_auth.go` 的 dispatch 调用点确认覆盖率）
- entrypoint `prepare_persistent_state` 依赖 worker 在 `--mount type=volume,src=claude-state-<id>,dst=/var/lib/claude-persist` 提供挂载点（Phase 33 自身闭环，无外部依赖）
- admin DELETE handler 依赖 `agentapi.Client.RunHostAction` 与 `Repository.GetClaudeAccount`（如不存在需 Plan 02 一并新增最小化的 `GetClaudeAccountForUpdate(ctx, id)`，注意 `FOR UPDATE` 行锁与事务上下文）
- host detail handler 字段追加对前端：现有 React admin SPA `webapp/src/admin/hosts/...` 若强类型化，需同步追加 TS interface（Plan 02 自查；若 admin 列表页不展示则可推迟到 v3.1）

### 既有反模式 / 已知坑

- `internal/controlplane/http/admin_hosts.go:getDockerStatuses` 直接 `exec.Command("docker", "ps", ...)` 是 Phase 29.1 已识别的测试基础设施债务（list 测试 hang）；本阶段**不**修该问题，但 `GetHostWithClaudeAccount` 必须**纯 DB JOIN**，不引入新的 docker exec 依赖到 detail handler
- entrypoint `cp -an` seed 在某些 docker storage driver（overlay2）下可能命中 cross-device link 警告（无害），用 `|| true` 兜底
- `docker volume rm` 在 daemon 不可用时会 hang 而不是 fast-fail：admin handler ctx 必须带超时（D-20）

</code_context>

<specifics>
## Specific Ideas

- **用户已锁定**：单 volume + 标签命名（Phase 30 D-01）；本阶段在此基础上补 `managed=true` 二级 label（D-02）
- **用户已锁定**：不扩展 host-agent endpoint（Phase 29 D-22 / Phase 30 D-04）；本阶段通过新增 `Action=volume_remove` 复用单一 endpoint
- **用户已锁定**：不写 OAuth token / 凭据到 audit metadata（Phase 29.1 Plan 02 mitigation）；本阶段 audit event 仅记 account_id / volume_name / 错误码
- **用户已锁定**：admin 不新增管理页（OOS-A19）；本阶段在 host detail 加一字段 + 不动 list endpoint
- **用户偏好**（PROJECT.md "优雅、好用、运维清晰"）：force 出口必须显式（query 参数 + 中文提示），不做隐式行为变更
- **本阶段倾向**：`docker volume rm` 失败默认拒绝 + 中文提示 "请先停止 host 后重试"，避免 force 静默丢数据；force 出口仅做最后手段

</specifics>

<deferred>
## Deferred Ideas

### 阶段内识别但不交付的 follow-up

- **独立 GC 定时任务**（cron 扫 orphan volume，按 label 过滤 + DB 反查无 account 时清理）—— v3.1 backlog（M16 已在 admin DELETE 联动覆盖主路径，GC 是兜底）
- **持久化 volume 备份脚本**（STACK.md §持久化 提到 `scripts/backup.sh` 集成 `tar /var/lib/docker/volumes/claude-state-*`）—— v3.1 / 运维手册增量
- **CREATE/UPDATE claude_account 控制面 handler**（当前仓储侧 SQL 已就位但无 admin handler）—— 本阶段 scope 仅 DELETE 联动；CREATE/UPDATE 由独立 phase 处理
- **用户面 host detail 暴露 `persistent_volume_name`**（D-24 显式拒绝）—— 用户视角无此需求；如有 v3.1 反馈再开
- **多 volume 拓扑**（creds / cache 拆分 / `ccp_` 前缀命名）—— Phase 30 已 deferred，沿用
- **subpath 挂载**（Docker engine 25+ 实验性）—— 服务端 docker 版本不可控，永久 deferred
- **OAuth credentials 加密 at rest**（volume 内容物加密）—— 与 v1 "网络强约束 + 单宿主机" 模型一致，deferred
- **runtime_service.go / bootstrap_auth.go 链路 ClaudeAccountID 注入覆盖率审计**（Phase 30 D-09 已落字段，但实际生产路径覆盖率未验证）—— 若 Plan 01 单测发现 worker 收不到 `ClaudeAccountID`，回流到 Phase 30 follow-up
- **前端 admin SPA TS interface 同步**（host detail 新字段）—— Plan 02 自查；若 React 端无强类型化可推迟
- **`docker volume inspect` 失败时的 audit event 详细程度**（区分 daemon 不可用 / volume 不存在 / 权限拒绝）—— Plan 01 实现时按 docker 错误字符串细分

### Reviewed Todos (not folded)

无（`todo match-phase 33` 零匹配）。

</deferred>

---

*Phase: 33-claude-code-cli-admin-gc*
*Context gathered: 2026-04-21（auto mode）*
