# Phase 33: Claude Code 状态持久化（CLI + 镜像 + admin GC） - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-21
**Phase:** 33-claude-code-cli-admin-gc
**Mode:** `--auto` — Claude 自动选择推荐默认值（依据 ROADMAP scope 字面 + Phase 29/30 已锁定边界 + PITFALLS / Out of Scope 约束）
**Areas discussed:** Volume 命名最终格式、Volume 创建触发链路、entrypoint 持久化拓扑、host-agent 删除通道、admin DELETE 事务一致性、容器仍持有 volume 的 rm 行为、admin host 详情页字段、Plan 拆分

---

## Volume 命名最终格式与 label 规范

| Option | Description | Selected |
|--------|-------------|----------|
| `claude-state-{uuid_with_hyphens}` + `account_id` + `managed=true` 双 label | 与 DB `claude_accounts.id` 字面一致；`managed=true` 二级保险给 prune 审计 | ✓ |
| `claude-state-{uuid_no_hyphens}` 单 label | 节省 4 字节，但失去与 DB 的字面 grep 对照 | |
| `ccp_claude_<id>_home` + 独立 cache volume（双 volume） | 拆分粒度更细 | （Phase 30 D-01 已 deferred） |

**User's choice (auto):** `claude-state-{uuid_with_hyphens}` + 双 label
**Notes:** Phase 30 D-01 已锁定 "单 volume" 但留了 "格式由实现选定但全仓库一致" 的开口。本阶段补齐 hyphen 保留 + 第二条 `managed=true` label 用于运维 `docker volume ls --filter label=com.cloud-cli-proxy.managed` 一键审计。**不**写 `created_at` label（与 Docker 自带元数据重复 + label-filter 不能精确匹配字符串）。

---

## Volume 创建触发链路

| Option | Description | Selected |
|--------|-------------|----------|
| Worker `createHost` 内部按需 + 幂等 | 与 ROADMAP scope 字面 "`worker.go:createHost ensureDockerVolume 幂等`" 一致；单职责 | ✓ |
| 控制面在 claude_account 创建时主动预创建 | 容器创建路径解耦，但需新加 host-agent 调用方 + 双写风险 | |
| Dispatcher 上游显式补 VolumeMount，Worker 不知情 | 透明，但每个 dispatch 调用方都要重复拼，遗漏一处就静默丢失 | |

**User's choice (auto):** Worker `createHost` 内部按需 + 幂等（D-04 / D-05）
**Notes:** 选项 1 与 ROADMAP 字面一致。worker 在 `request.ClaudeAccountID != ""` 时**自动补一条** VolumeMount（D-05），同时调用 `repo.UpsertClaudeAccountPersistentVolumeName` 把 volume 名落到 DB（D-06，仅 NULL → 已分配，与 Phase 30 D-02 三态消除一致）。`ClaudeAccountID == ""` 路径（v2.0 旧 host）走 fallback：跳过 volume 补写 + entrypoint symlink 仍生效但无持久化数据，**不**报错阻塞启动（D-07）。

---

## entrypoint 持久化拓扑

| Option | Description | Selected |
|--------|-------------|----------|
| 单 volume 挂 `/var/lib/claude-persist` + entrypoint symlink + 首次 seed | 兼容现状；Docker named volume 不原生支持 subpath 挂载 | ✓ |
| 单 volume 直挂 `/home/claude/.claude` + 另一 volume 挂 `.cache/claude` | 解耦但回到双 volume，违反 Phase 30 D-01 单 volume 决议 | |
| Docker subpath 挂载（engine 25+ 实验性） | 服务端 docker 版本不可控 | |
| symlink 但不做 seed（依赖 Claude Code 自创建目录） | 简化但 first-run 可能因目录不存在报错 | |

**User's choice (auto):** Option 1 — 单 volume + symlink + `cp -an` seed（D-08 / D-09）
**Notes:** 入口函数 `prepare_persistent_state` 插入位置在 `prepare_v3_dirs` 之后、`prepare_mutagen_agent` 之前（先把 home dir 物理位置摆好，再让 mutagen agent 看到稳定路径）。首次为空时 `cp -an /home/claude/.claude/. /var/lib/claude-persist/.claude/` 把镜像预建内容 seed 进 volume；`-a` 保留权限/时间戳，`-n` 不覆盖已有文件，`|| true` 兜底极端 storage driver 警告。三层防御 PITFALLS M17：(1) Dockerfile 预建 chown；(2) Phase 29 `prepare_v3_dirs` 二次 chown；(3) 本阶段 `prepare_persistent_state` 第三次 chown + symlink 链接本身 chown。

---

## host-agent 删除通道形态

| Option | Description | Selected |
|--------|-------------|----------|
| 沿用 `POST /v1/host-actions` + 新增 `Action=volume_remove`，复用 `Volumes` 字段 | 与 Phase 29 D-22 / Phase 30 D-04 边界一致；零 endpoint 表面增量 | ✓ |
| 新增独立 `DELETE /v1/volumes/{name}` endpoint | 语义更直接，但破坏 host-agent "单一 endpoint" 现状 | |
| 新增独立 `POST /v1/volumes/remove` endpoint | 同上 | |

**User's choice (auto):** Option 1 — 复用 `host-actions` + 新增 Action（D-13 / D-14 / D-15 / D-16）
**Notes:** `Action=volume_remove` 时只读 `Volumes []VolumeMount.Name`，其余字段 ignore；force 标志通过 `request.Labels["force"] == "true"` 携带（避免再扩 `HostActionRequest` 字段）。host-agent `mux` 三个 route 不变。`removeDockerVolume` 实现：force=false → 直接 `docker volume rm`，in-use 时返回错误传播；force=true → `docker volume rm -f`；volume 不存在视为成功（幂等）。

---

## admin DELETE 事务一致性模型

| Option | Description | Selected |
|--------|-------------|----------|
| 强一致：事务内同步 host-agent rm，失败回滚 | ROADMAP scope 字面 "事务内调用 host-agent volume rm 失败回滚"；语义最强 | ✓（默认） |
| 最终一致：DB 删完 best-effort + 必失败时记 audit 留 GC | 不会卡死管理员，但 orphan 风险 | ✓（`?force=true` 出口） |
| 双写 / saga：先 mark deleted → host-agent rm 成功后硬删 | 引入 status 字段 + 异步消费器 | |

**User's choice (auto):** 默认强一致 + `?force=true` 出口走最终一致（D-17 / D-18 / D-19 / D-20）
**Notes:** 强一致路径事务内同步调用，ctx 超时 10s。host-agent rm 失败 → 事务 rollback + 写 audit event `claude_account.delete_volume_rm_failed` + HTTP 409 中文提示 "请先停止使用该账号的所有 host 后重试，或追加 ?force=true 强删 volume"。force 路径：DB 先 commit，再调 host-agent + `Labels["force"]="true"` → `docker volume rm -f`，ctx 超时 30s；rm 失败返回 200 + body `{"deleted": true, "volume_rm": "failed", "next_action": "运维需手工 docker volume rm -f <name>"}`，把 orphan 风险显式上报不静默丢失。理由：force 出口避免容器还在跑时管理员永远删不掉账号，符合 PROJECT.md "运维清晰" 价值观。

---

## 容器仍持有 volume 时的 rm 失败行为

| Option | Description | Selected |
|--------|-------------|----------|
| 默认拒绝（不传 `--force`）+ 中文事件 + force 出口手工触发 | 安全优先；用户必须显式承担风险 | ✓ |
| 默认 `docker volume rm -f` 强删 | 简单，但容器内进程的 OAuth 操作会瞬间失败 | |
| 等待容器 stop 后重试（带 backoff） | 不可控等待时长 | |

**User's choice (auto):** 默认拒绝 + force 出口（D-15 / D-19）
**Notes:** 与 SC #6 "事务回滚不留半成品" 字面一致。`docker volume rm` 在 in-use 时 docker daemon 直接返回 `Error response from daemon: remove <name>: volume is in use`，host-agent 把此错误传播到 admin handler → 事务 rollback。force 仅在 admin 显式 `?force=true` 时启用 `docker volume rm -f`。

---

## admin host 详情页字段

| Option | Description | Selected |
|--------|-------------|----------|
| `GET /v1/admin/hosts/{id}` 响应顶层追加 `persistent_volume_name`（来自 LEFT JOIN） | OOS-A19 边界 "最多加一行"；零新页面 | ✓ |
| 新增 `GET /v1/admin/claude-accounts/{id}/volume` endpoint | 解耦但违反 OOS-A19 + OOS-A20 | |
| 在 list endpoint `/v1/admin/hosts` 也追加该字段 | list 性能/列宽考虑，OOS-A19 解读为 detail-only | |
| 同时在用户面 `/v1/user/hosts/{id}` 暴露 | 用户视角无 volume 名需求 | |

**User's choice (auto):** Option 1（D-22 / D-23 / D-24）
**Notes:** 仓储新增 `GetHostWithClaudeAccount(ctx, hostID) (HostDetail, error)` 单次 LEFT JOIN，避免 N+1。现有 `GetHost` / `ListHostsWithUsername` 保持不变（Phase 29.1 已锁定的 6 个 SELECT 不动）。新字段以 `omitempty` 形式追加，旧前端二进制忽略未知字段（与 Phase 30 D-03 兼容策略一致）。用户面 `/v1/user/hosts/{id}` 显式拒绝（D-24）。

---

## Plan 拆分

| Option | Description | Selected |
|--------|-------------|----------|
| 2 plans：(01) 镜像 + Worker + agentapi；(02) admin DELETE + host detail + UAT | 与 ROADMAP `0/2 plans` 字面一致；Plan 02 自然依赖 Plan 01 | ✓ |
| 3 plans：(01) 镜像；(02) Worker + agentapi；(03) admin handler + UAT | 粒度更细，但 (01)(02) 强耦合（symlink 与 volume 创建同源） | |
| 1 plan：全部一起 | 单 wave 风险大，回滚粒度差 | |

**User's choice (auto):** 2 plans（D-28 / D-29）
**Notes:** Plan 01 涵盖 entrypoint `prepare_persistent_state`、`agentapi.ActionVolumeRemove`、worker `ensureDockerVolume` / `removeDockerVolume` / `BuildClaudeStateVolumeName` / `Execute()` switch / `createHost` 自动补 volume / `repository.UpsertClaudeAccountPersistentVolumeName` + 单元测试 D-25 第 1-4 + 第 7 项。Plan 02 涵盖 `admin_claude_accounts.go` DELETE handler（含 force flag）、`admin_hosts.go` GET detail 追加 `persistent_volume_name`、`Repository.GetHostWithClaudeAccount` LEFT JOIN、`router.go` route 注册、单测 D-25 第 5-6 项、UAT 清单 D-26、运维手册章节。Plan 02 的 admin handler 严守 Phase 29.1 Plan 02 已建立的 "runtime/worker fail-fast + audit event" 模式，**不**新引入 service / use case 层抽象。

---

## Claude's Discretion

以下细节交给 planner / executor 自决（不影响 plan 通过 verifier）：

- `cp -an` vs `rsync -a --ignore-existing`（推荐 cp -an，零依赖）
- `BuildClaudeStateVolumeName` 是否长度截断（49 字符 << Docker 上限 255，无需）
- audit event metadata 字段顺序（保持 account_id / volume_name / error_code / error_message）
- `removeDockerVolume` 是否先 inspect fast-fail（推荐直接 rm）
- handler query 参数 `force=true` / `1` / `yes` 是否同接受（planner 决策）
- 仓储新方法是否拆 `claude_account_repository.go` 还是复用 `repository` 包（推荐复用，与 Phase 30 风格一致）

## Deferred Ideas

- **独立 GC 定时任务**：v3.1 backlog（admin DELETE 联动是主路径，GC 是兜底）
- **持久化 volume 备份脚本**：v3.1 / 运维手册增量
- **CREATE/UPDATE claude_account admin handler**：本阶段 scope 仅 DELETE 联动，超出范围
- **用户面 host detail 暴露 `persistent_volume_name`**：D-24 显式拒绝
- **多 volume / 子目录直挂 / subpath 挂载**：Phase 30 D-01 锁定单 volume；subpath docker 版本不可控
- **OAuth credentials at-rest 加密**：与 v1 "单宿主机 + 网络强约束" 模型一致，永久 deferred
- **runtime_service.go / bootstrap_auth.go ClaudeAccountID 注入覆盖率审计**：Phase 30 D-09 已落字段，生产路径覆盖率待 Plan 01 单测验证；如发现缺口回流 Phase 30 follow-up
- **前端 admin SPA TS interface 同步**：Plan 02 自查；若强类型化则同步追加，否则推迟
