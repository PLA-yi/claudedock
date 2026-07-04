---
phase: 01-foundation-control-plane-host-agent
plan: "03"
subsystem: infra
tags: [unix-socket, host-agent, docker, systemd, runtime]
requires:
  - phase: 01-01
    provides: control-plane API shell, repository methods, and task tables
  - phase: 01-02
    provides: pinned managed-user image contract in image.lock
provides:
  - host-agent unix socket API and docker-backed task worker
  - lifecycle endpoints that enqueue tasks and return `202 Accepted`
  - systemd units, preflight checks, and a phase-2 network provider seam
affects: [phase-2-networking, phase-3-ssh, operations]
tech-stack:
  added: [unix domain sockets, systemd units]
  patterns: [control-plane-to-agent-split, task-status-writeback, image-lock-runtime-loading]
key-files:
  created:
    - cmd/host-agent/main.go
    - internal/agent/server.go
    - internal/agentapi/client.go
    - internal/runtime/runtime_service.go
    - internal/controlplane/http/hosts.go
    - internal/controlplane/http/tasks.go
    - internal/network/provider.go
    - deploy/systemd/claudedock-host-agent.service
  modified:
    - internal/controlplane/app/app.go
    - internal/controlplane/http/router.go
    - internal/store/repository/queries.go
    - deploy/systemd/claudedock-control-plane.service
    - deploy/scripts/host-preflight.sh
key-decisions:
  - "控制面通过 Unix socket 私有 API 调用 host-agent，而不是在 HTTP 层直接操作 Docker。"
  - "运行时规格由 `image.lock` 解析得到，生命周期请求不再散落硬编码镜像参数。"
  - "Phase 2 网络实现仅以 `Provider` 接口预留，不在 Phase 1 提前落地。"
patterns-established:
  - "Pattern: HTTP handler 只创建任务并返回 `task_id`，特权动作由 host-agent 回写状态。"
  - "Pattern: host-agent 先写 `running`，再根据执行结果写 `succeeded|failed` 与错误摘要。"
requirements-completed: [RUNT-01]
duration: 1 min
completed: 2026-03-26
---

# Phase 01 Plan 03: 主机代理与特权边界 Summary

**基于 Unix socket 的 host-agent、真实 Docker 生命周期 worker 与 systemd 特权边界**

## Performance

- **Duration:** 1 min
- **Started:** 2026-03-26T16:58:52+08:00
- **Completed:** 2026-03-26T16:59:08+08:00
- **Tasks:** 3
- **Files modified:** 16

## Accomplishments
- 新增 host-agent 进程、私有 API 契约和 Docker-backed worker，把 `create/start/stop/rebuild` 特权动作移出控制面。
- 让控制面从 `image.lock` 读取 `image_name`、`default_user`、`home_mount` 和 `preserve-home` 默认模式，并通过生命周期端点返回 `task_id`。
- 补齐 systemd unit、宿主机预检脚本和 `NoopProvider`，为 Phase 2 网络强约束留下明确扩展位。

## Task Commits

Each task was committed atomically:

1. **Task 1: 定义宿主机代理私有 API 与任务执行骨架** - `ab6a873` (feat)
2. **Task 2: 连接控制面生命周期端点、任务调度、任务列表与镜像锁** - `667c291` (feat)
3. **Task 3: 交付宿主机预检与 systemd 特权边界** - `388ee9a` (chore)

**Plan metadata:** pending

## Files Created/Modified
- `cmd/host-agent/main.go` - host-agent 启动入口与数据库连接
- `internal/agent/server.go` - 监听 `/run/claudedock/host-agent.sock` 的私有 API 服务
- `internal/agentapi/contracts.go` - `HostActionRequest`、`HostActionResponse`、`TaskStatusUpdate`
- `internal/agentapi/client.go` - 控制面对 Unix socket 的 `RunHostAction` 客户端
- `internal/runtime/tasks/worker.go` - 真实 Docker lifecycle 执行与任务状态写回
- `internal/runtime/runtime_service.go` - `image.lock` 解析与生命周期任务排队逻辑
- `internal/controlplane/http/hosts.go` - `POST /v1/hosts/{hostID}/create|start|stop|rebuild`
- `internal/controlplane/http/tasks.go` - 真实 `GET /v1/tasks` 列表
- `internal/network/provider.go` - `Provider` / `NoopProvider` seam
- `deploy/systemd/claudedock-host-agent.service` - host-agent 权限边界

## Decisions Made
- 任务调度阶段就消费 `image.lock`，避免 host-agent 和 control-plane 分别猜测镜像/挂载参数。
- host-agent worker 在容器缺失时允许 `start_host` 先补 `create_host`，降低生命周期空状态下的操作失败率。
- 预检脚本强制要求 `docker`、`ip`、`systemctl` 以及 `nft|iptables` 之一，确保后续网络强约束落地前就暴露宿主机缺项。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] 扩展 `app.go` 以接入 runtime service**
- **Found during:** Task 2（连接控制面生命周期端点、任务调度、任务列表与镜像锁）
- **Issue:** 计划列出的主要变更文件未包含 `internal/controlplane/app/app.go`，但如果不修改应用装配层，新的 host-agent client、dispatcher 与任务 handler 无法真正被控制面使用。
- **Fix:** 在 `app.go` 中接入 `agentapi.Client`、`Dispatcher`、`runtime.Service` 与新的 tasks/hosts handlers。
- **Files modified:** `internal/controlplane/app/app.go`
- **Verification:** `go test ./...`
- **Committed in:** `667c291`

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** 这是必要的装配修正，没有引入额外产品范围，只让既定生命周期流真正可达。

## Issues Encountered
- 当前执行环境缺少 `systemd-analyze`，因此无法运行计划要求的 `systemd-analyze verify ...`；已通过内容校验和脚本语法校验确认 unit 文件与 preflight 逻辑。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- 控制面与 host-agent 的职责边界已经在代码、Unix socket 契约和 systemd 单元层面固定，可直接承接 Phase 2 的网络 provider 实现。
- 若要做真实 Linux 宿主机联调，需要在具备 Docker daemon 和 `systemd-analyze` 的环境中重跑生命周期与 unit 验证。

---
*Phase: 01-foundation-control-plane-host-agent*
*Completed: 2026-03-26*
