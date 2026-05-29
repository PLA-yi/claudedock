# Phase 57: 资源限制可配置化 - Context

**Gathered:** 2026-05-29
**Status:** Ready for planning

<domain>
## Phase Boundary

允许管理员在创建主机和停止主机后手动设置内存、CPU 和磁盘限制。支持"无限制"选项（不传 Docker 资源参数，容器可使用宿主机全部资源）。前端管理界面提供直观的资源限制选择控件。

**In scope:**
- "无限制"语义在数据库、API、Docker 层面的完整实现
- 创建主机时设置资源限制（前端控件）
- 停止主机后修改资源限制（新增 PATCH API + 前端编辑）
- 内存、CPU、磁盘三种资源限制

**Out of scope:**
- 运行中容器热调资源（`docker update`）—— 可在未来 phase 扩展
- 用户自助面板（非管理员）的资源限制视图
- 资源使用量监控 / 图表
- 按用户角色限制可选范围（RBAC）

</domain>

<decisions>
## Implementation Decisions

### D-01: "无限制"的数据库语义
- 数据库列改为 **nullable**（`ALTER COLUMN DROP NOT NULL`）
- `NULL` = 无限制（Docker 层面不传 `--memory` / `--cpus` / `--storage-opt`）
- 正值 = 具体限制
- 迁移时保留现有值不变（已有主机的 4096MB / 2.0 CPU / 20GB 是显式限制）
- 默认值逻辑移到 **API 层**：当管理员创建主机时不传资源字段，API 层填入默认值（4096MB / 2.0 CPU / 20GB）

### D-02: "无限制"的 API 语义
- 资源字段使用 **指针类型**（`*int` / `*float64`）区分三种语义：
  - 字段省略或 `null` → 使用默认值（4096MB / 2.0 CPU / 20GB）
  - 字段设为 `0` → 无限制
  - 字段设为正数 → 该具体限制
- 这个三态模型在 Create 和 PATCH 两个端点保持一致

### D-03: 何时可调整资源限制
- **创建时**：通过现有 Create API 的资源字段设置
- **停止后**：新增 `PATCH /api/admin/hosts/:id/resources` 端点
  - 仅允许 `stopped` 状态的主机修改资源限制
  - 修改后下次 `docker create`（即 Start）时生效
  - 运行中的主机返回 409 Conflict
- 不支持运行中热调（`docker update`），保持简单

### D-04: 磁盘限制纳入本次实现
- `disk_limit_gb` 字段已存在于数据库，本次一并完善
- Worker `buildCreateArgs()` 新增 `--storage-opt size=Xg` 参数
- 注意：需要宿主机存储驱动支持（overlay2 + xfs + pquota），不支持时 Docker 忽略该参数，不阻塞创建

### D-05: 前端交互设计
- **创建表单**：新增"资源限制"区域，三个字段各一个选择器
  - 每个选择器：下拉菜单 + 预设值 + "自定义"选项展开数字输入
  - 内存预设：无限制 / 1 GB / 2 GB / 4 GB（默认）/ 8 GB / 16 GB / 自定义
  - CPU 预设：无限制 / 0.5 核 / 1 核 / 2 核（默认）/ 4 核 / 8 核 / 自定义
  - 磁盘预设：无限制 / 10 GB / 20 GB（默认）/ 50 GB / 100 GB / 自定义
- **主机详情页**：显示当前资源限制为可编辑区域
  - 停止状态：可点击"编辑"修改，保存后调 PATCH API
  - 运行中状态：显示当前值，编辑按钮禁用 + tooltip "请先停止主机"
  - 删除主机后重建时使用新设置的限制

### D-06: Worker 层实现
- `HostActionRequest` 新增 `DiskLimitGB int` 字段（当前只有 Memory 和 CPU）
- `buildCreateArgs()` 中：
  - 值 > 0：传对应 Docker 参数
  - 值 <= 0 或未设置：不传参数（Docker 默认无限制）
- `runtime_service.go` 中的 `defaultIntIfZero` / `defaultFloatIfZero` 兜底逻辑移除，让 NULL/0 语义直接透传

### Claude's Discretion
- 前端 select 预设值的具体列表可根据宿主机实际配置调整，planner 有灵活度
- PATCH API 的请求体验证（最小值、最大值）由 planner 决定合理范围
- 如果 `--storage-opt` 在目标宿主机不生效，可降级为仅记录不强制，planner 有判断权

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 数据库迁移
- `internal/store/migrations/0006_host_resource_limits.sql` — 现有资源限制列定义（NOT NULL DEFAULT）

### 数据模型
- `internal/store/repository/models.go` — Host struct 和 UpsertHostParams 中的资源限制字段
- `internal/store/repository/queries.go` L359-372 — UpsertHost 中的默认值兜底逻辑

### API 层
- `internal/controlplane/http/admin_hosts.go` L147-282 — Create 端点，接受资源限制参数
- `internal/controlplane/http/admin_hosts.go` L284-337 — Start/Stop/Rebuild 生命周期端点
- `internal/agentapi/contracts.go` L41-73 — HostActionRequest 结构体

### Worker 层
- `internal/runtime/tasks/worker.go` L210-264 — buildCreateArgs() 构建 docker create 命令
- `internal/runtime/runtime_service.go` L175-185 — QueueHostAction 构建 HostActionRequest

### Local 路径
- `internal/local/local.go` L15-16, L200-234 — local 模式的资源限制默认值和 buildCreateArgs

### 前端
- `web/admin/src/routes/_dashboard/hosts/` — 主机管理页面
- `web/admin/src/routes/_dashboard/hosts/$hostId.tsx` — 主机详情页

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `HostActionRequest` 结构体已有 `MemoryLimitMB` 和 `CPULimit` 字段，只需新增 `DiskLimitGB`
- `buildCreateArgs()` 已有内存和 CPU 的 Docker 参数生成逻辑，模式可复用于磁盘
- Admin API 的 Create 端点已接受资源参数，模式可复制到 PATCH 端点
- 前端 TanStack Router + shadcn/ui 组件库已就绪，select / input 组件可直接使用

### Established Patterns
- API 端点使用 `nethttp.HandlerFunc` + `json.NewDecoder` 模式
- 错误返回使用 `writeJSON(w, statusCode, map[string]string{"error": "..."})`
- 数据库操作通过 `Repository` 方法 + pgx 驱动
- 前端使用 TanStack Router 文件路由 + React Query 数据获取

### Integration Points
- 新增 PATCH 端点需要在路由注册处添加（检查 `internal/controlplane/http/router.go` 或类似文件）
- 前端需要在主机创建表单和详情页集成新的资源限制控件
- Migration 需要编号接续现有最新 migration

</code_context>

<specifics>
## Specific Ideas

- 用户核心诉求：**优雅、好用**。前端交互要直观，默认值合理，无限制选项要醒目。
- "无限制"应该是显式选择，不是"什么都不填"的隐式行为。
- 创建和停止后都能调整，形成完整的资源管理闭环。

</specifics>

<deferred>
## Deferred Ideas

- 运行中容器热调资源（`docker update`）—— 可作为独立 phase
- 用户自助面板查看自己的资源限制
- 资源使用量实时监控和图表
- 按用户套餐 / 角色限制可选资源范围

</deferred>

---

*Phase: 57-资源限制可配置化*
*Context gathered: 2026-05-29*
