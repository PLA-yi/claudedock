---
phase: quick
plan: 260328-trs
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/store/migrations/0006_host_resource_limits.sql
  - internal/store/repository/models.go
  - internal/store/repository/queries.go
  - internal/agentapi/contracts.go
  - internal/runtime/tasks/worker.go
  - internal/controlplane/http/admin_hosts.go
  - internal/controlplane/http/admin_hosts_test.go
autonomous: true
requirements: []
must_haves:
  truths:
    - "hosts 表新增 memory_limit_mb、cpu_limit、disk_limit_gb 三个字段，且有合理默认值"
    - "docker create 时会携带 --memory 和 --cpus 参数"
    - "管理员创建/更新 host 时可通过 API 指定资源限制参数"
  artifacts:
    - path: "internal/store/migrations/0006_host_resource_limits.sql"
      provides: "数据库迁移：hosts 表加资源限制列"
    - path: "internal/store/repository/models.go"
      provides: "Host 结构体新增 MemoryLimitMB、CPULimit、DiskLimitGB 字段"
    - path: "internal/runtime/tasks/worker.go"
      provides: "createHost 函数在 docker create 参数中注入 --memory 和 --cpus"
  key_links:
    - from: "internal/controlplane/http/admin_hosts.go"
      to: "internal/store/repository/queries.go"
      via: "UpsertHostParams 传递资源限制参数"
      pattern: "MemoryLimitMB|CPULimit|DiskLimitGB"
    - from: "internal/runtime/runtime_service.go"
      to: "internal/agentapi/contracts.go"
      via: "HostActionRequest 携带资源限制字段到 worker"
      pattern: "MemoryLimitMB|CPULimit"
    - from: "internal/runtime/tasks/worker.go"
      to: "docker create"
      via: "将资源限制字段转为 --memory 和 --cpus 参数"
      pattern: "--memory|--cpus"
---

<objective>
为用户容器添加资源限制功能（内存、CPU、磁盘），贯穿数据模型、容器创建逻辑和管理 API 三层。

Purpose: 防止单个容器占用过多宿主机资源，保障多用户场景下的稳定性。
Output: hosts 表新增资源限制字段，docker create 自动携带限制参数，管理 API 支持配置。
</objective>

<execution_context>
@/workspace/Desktop/claudedock/.claude/get-shit-done/workflows/execute-plan.md
@/workspace/Desktop/claudedock/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@internal/store/repository/models.go
@internal/store/repository/queries.go
@internal/store/migrations/0005_host_env_and_user_entry.sql
@internal/agentapi/contracts.go
@internal/runtime/runtime_service.go
@internal/runtime/tasks/worker.go
@internal/controlplane/http/admin_hosts.go
@internal/controlplane/http/admin_hosts_test.go
</context>

<tasks>

<task type="auto">
  <name>Task 1: 数据模型层 — 迁移脚本 + Host 结构体 + UpsertHost 查询</name>
  <files>
    internal/store/migrations/0006_host_resource_limits.sql
    internal/store/repository/models.go
    internal/store/repository/queries.go
  </files>
  <action>
1. 创建迁移文件 `0006_host_resource_limits.sql`，为 hosts 表新增三列：
   - `memory_limit_mb INTEGER NOT NULL DEFAULT 4096` — 内存上限，单位 MB，默认 4GB
   - `cpu_limit NUMERIC(4,1) NOT NULL DEFAULT 2.0` — CPU 核数上限，用 NUMERIC 允许小数（如 0.5、1.5），默认 2 核
   - `disk_limit_gb INTEGER NOT NULL DEFAULT 20` — 磁盘上限，单位 GB，默认 20GB
   使用 `ALTER TABLE hosts ADD COLUMN IF NOT EXISTS` 语法，与已有迁移风格保持一致。

2. 在 `models.go` 的 `Host` 结构体中新增三个字段：
   - `MemoryLimitMB int` (json tag: `memory_limit_mb`)
   - `CPULimit float64` (json tag: `cpu_limit`)
   - `DiskLimitGB int` (json tag: `disk_limit_gb`)
   放在 `Hostname` 和 `CreatedAt` 之间。

3. 在 `models.go` 的 `UpsertHostParams` 结构体中新增对应的三个字段：
   - `MemoryLimitMB int`
   - `CPULimit float64`
   - `DiskLimitGB int`

4. 修改 `queries.go` 中的 `UpsertHost` 方法：
   - INSERT 列列表加入 `memory_limit_mb, cpu_limit, disk_limit_gb`
   - VALUES 占位符追加 `$8, $9, $10`
   - ON CONFLICT DO UPDATE SET 中追加这三列
   - RETURNING 子句追加这三列
   - 传参 Scan 追加读取这三列
   注意：在传入参数前，如果值为零值则使用默认值（MemoryLimitMB=0 → 4096，CPULimit=0 → 2.0，DiskLimitGB=0 → 20）。这样旧调用方不传这些字段也能得到合理默认。

5. 检查 `queries.go` 中所有 Scan Host 的地方（如 GetHost、ListHostsWithUsername、GetHostDetail 等），确保 RETURNING / SELECT 子句也包含新字段，Scan 也读取新字段。用 grep 搜索所有 `&item.Hostname` 或 `&h.Hostname` 来定位。
  </action>
  <verify>
    <automated>cd /workspace/Desktop/claudedock && go build ./internal/store/...</automated>
  </verify>
  <done>
    - 迁移文件存在且语法正确
    - Host 和 UpsertHostParams 包含三个新字段
    - UpsertHost 查询写入和返回新字段
    - 所有 Host Scan 点都已更新
    - `go build ./internal/store/...` 通过
  </done>
</task>

<task type="auto">
  <name>Task 2: 容器创建逻辑 — 资源限制参数注入到 docker create</name>
  <files>
    internal/agentapi/contracts.go
    internal/runtime/runtime_service.go
    internal/runtime/tasks/worker.go
  </files>
  <action>
1. 在 `contracts.go` 的 `HostActionRequest` 结构体中新增：
   - `MemoryLimitMB int` (json tag: `memory_limit_mb,omitempty`)
   - `CPULimit float64` (json tag: `cpu_limit,omitempty`)
   这两个字段由 runtime_service 从 Host 模型读取后填入。磁盘限制不在此处传递，因为 Docker `--storage-opt` 需要特定存储驱动支持，v1 阶段暂不实现磁盘限额的运行时约束（仅在数据模型中记录配额值）。

2. 在 `runtime_service.go` 的 `QueueHostAction` 方法中：
   - 在构造 `agentapi.HostActionRequest` 时，从已读取的 `host` 对象中取 `MemoryLimitMB` 和 `CPULimit` 赋值到 request。
   - 如果值为 0，使用默认值（4096 / 2.0）。

3. 在 `worker.go` 的 `createHost` 方法中，在构建 docker create 参数 `args` 切片时：
   - 在 `--sysctl` 行之后、`-e` 行之前，插入资源限制参数：
     ```go
     if request.MemoryLimitMB > 0 {
         args = append(args, "--memory", fmt.Sprintf("%dm", request.MemoryLimitMB))
     }
     if request.CPULimit > 0 {
         args = append(args, "--cpus", fmt.Sprintf("%.1f", request.CPULimit))
     }
     ```
   - 注意 `--memory` 的值格式是 `Nm`（如 `4096m`），`--cpus` 的值是浮点数字符串（如 `2.0`）。
  </action>
  <verify>
    <automated>cd /workspace/Desktop/claudedock && go build ./internal/...</automated>
  </verify>
  <done>
    - HostActionRequest 包含 MemoryLimitMB 和 CPULimit 字段
    - runtime_service 将 Host 的资源限制传递到 request
    - createHost 在 docker create 参数中注入 --memory 和 --cpus
    - `go build ./internal/...` 通过
  </done>
</task>

<task type="auto">
  <name>Task 3: API 层 — 管理接口支持资源限制参数 + 测试更新</name>
  <files>
    internal/controlplane/http/admin_hosts.go
    internal/controlplane/http/admin_hosts_test.go
  </files>
  <action>
1. 在 `admin_hosts.go` 的 `Create()` handler 中：
   - 在请求体结构体 `body` 中新增三个可选字段：
     ```go
     MemoryLimitMB int     `json:"memory_limit_mb"`
     CPULimit      float64 `json:"cpu_limit"`
     DiskLimitGB   int     `json:"disk_limit_gb"`
     ```
   - 在调用 `h.store.UpsertHost` 时将这三个字段传入 `UpsertHostParams`。
   - 零值表示"使用默认值"，不做额外校验（由数据层兜底默认值）。

2. 在 `admin_hosts.go` 中新增一个 `UpdateResources()` handler 方法（可选但推荐），接受 PATCH `/v1/admin/hosts/{hostID}/resources`：
   - 请求体：`{ "memory_limit_mb": 8192, "cpu_limit": 4.0, "disk_limit_gb": 40 }`
   - 从 store 读取现有 host，将非零请求值覆盖到 UpsertHostParams 中，调用 UpsertHost 更新。
   - 返回更新后的 host 对象。
   - 如果不需要 UpdateResources 作为独立端点，也可以直接通过现有的 Create（upsert 语义）覆盖。根据代码现状，UpsertHost 已经是 upsert 语义，所以也可以跳过此步，仅在 Create 中支持即可。选择更简单的方案。

3. 如果创建了新路由，在 `router.go` 中注册。

4. 更新 `admin_hosts_test.go`：
   - 如果 `stubHostStore` 缺少新接口方法签名，补齐（确保编译通过）。
   - 不需要新增测试用例（除非编译要求）。
  </action>
  <verify>
    <automated>cd /workspace/Desktop/claudedock && go test ./internal/controlplane/http/... -count=1 -timeout 30s</automated>
  </verify>
  <done>
    - 管理员创建 host 时可传 memory_limit_mb、cpu_limit、disk_limit_gb
    - 零值自动使用默认值（4096MB / 2核 / 20GB）
    - 现有测试编译通过且不回归
    - `go test ./internal/controlplane/http/...` 全部通过
  </done>
</task>

</tasks>

<verification>
1. `go build ./...` 整个项目编译通过
2. `go test ./internal/... -count=1` 所有测试通过
3. 迁移文件 0006 存在且 SQL 语法正确
4. 搜索 `--memory` 和 `--cpus` 能在 worker.go 中找到
</verification>

<success_criteria>
- hosts 表有 memory_limit_mb（默认 4096）、cpu_limit（默认 2.0）、disk_limit_gb（默认 20）三列
- docker create 命令自动携带 --memory 和 --cpus 参数
- 管理 API 创建 host 时支持传入资源限制参数
- 所有现有测试不回归
</success_criteria>

<output>
完成后创建 `.planning/quick/260328-trs-cpu/260328-trs-SUMMARY.md`
</output>
