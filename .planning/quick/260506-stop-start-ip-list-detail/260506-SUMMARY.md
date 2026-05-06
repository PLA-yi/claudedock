---
phase: quick
plan: "260506"
subsystem: "network + runtime + admin-api + web-admin"
tags: ["ip-leak", "status-sync", "docker-stop", "smoke-verify", "egress-ip"]
dependency_graph:
  requires: []
  provides: ["FIX-A-IP-LEAK", "FIX-B-STATUS-SYNC", "FIX-C-SMOKE-VERIFY"]
  affects: ["internal/network/container_proxy_provider.go", "internal/runtime/tasks/worker.go", "internal/controlplane/http/admin_hosts.go", "web/admin/src/routes/_dashboard/hosts/index.tsx", "web/admin/src/routes/_dashboard/hosts/$hostId.tsx"]
tech-stack:
  added: []
  patterns: ["disconnect-bridge-universal", "metric-0-default-route", "verify-retry", "smoke-curl", "db-status-single-source"]
key-files:
  created: []
  modified:
    - internal/network/container_proxy_provider.go
    - internal/runtime/tasks/worker.go
    - internal/controlplane/http/admin_hosts.go
    - web/admin/src/routes/_dashboard/hosts/index.tsx
    - web/admin/src/routes/_dashboard/hosts/$hostId.tsx
decisions:
  - "所有平台统一 disconnect bridge（删除 runtime.GOOS == linux 限制），macOS Docker Desktop 的 SSH 端口映射由 vpnkit 在 cloudproxy-net 上也能工作"
  - "configureWorkerEgress 改为反竞态脚本：删除所有现有 default 路由 → 添加 metric 0 default → 立即 grep verify → 最多 3 次 retry"
  - "列表页与详情页均以 DB host.status 为唯一状态源，docker_status 仅在详情页作为降级辅助"
  - "worker.Execute 失败路径在 UpdateHostStatus('failed') 之前执行 docker stop，消除状态分裂"
  - "PrepareHost 完成后用 curl api.ipify.org 做出口 IP 冒烟探测，失败 teardown 并返回 ErrEgressIPMismatch"
metrics:
  duration: "~8 分钟"
  completed_date: "2026-05-06"
---

# Phase quick Plan 260506: 修复 stop→start 后 IP 泄漏 + list/detail 状态不一致 Summary

**一句话总结：** 修复管理后台 stop→start 后 user 容器 default route 被 bridge 覆盖导致的 IP 泄漏，统一列表/详情页以 DB status 为唯一数据源，并在 PrepareHost 完成后加入出口 IP 冒烟探测兜底。

## 执行结果

| 任务 | 名称 | 状态 | Commit |
|------|------|------|--------|
| 1 | 修复 IP 泄漏 — 统一 disconnect bridge + configureWorkerEgress 反竞态 | 完成 | d43b2c8 |
| 2 | 修复状态机分裂 — list 去 DockerStatus + worker 失败路径补 stop + 前端统一 DB status | 完成 | 5ac4163 |
| 3 | 防御兜底 — PrepareHost 完成后出口 IP 冒烟探测 | 完成（与 Task 1 同文件，合入 d43b2c8） | d43b2c8 |

## 变更详情

### Fix A：IP 泄漏修复（container_proxy_provider.go）

- **删除 `runtime.GOOS == "linux"` 条件**：所有平台统一执行 `docker network disconnect -f bridge workerName`
- **configureWorkerEgress 重构**：
  - 拆分为 `tryConfigureWorkerEgress`（单次执行）+ `configureWorkerEgress`（最多 3 次 retry，退避 500ms/attempt）
  - 脚本现在：
    1. 等待接口出现（保留 5 次循环）
    2. **删除所有现有 default 路由**（`ip route show default | while read ... ip route del default via ... dev ...`）
    3. **添加 metric 0 default**：`ip route add default via <gwIP> dev "$DEV" metric 0`
    4. **立即 verify**：`ip route show default | head -1 | grep -q "via <gwIP>"`
- **新增 `verifyWorkerEgress`**：curl `api.ipify.org`（纯文本，无 JSON 解析风险），`--max-time 5` 防 hang
- **PrepareHost 插入探测点**：configureWorkerEgress 成功后、cp 网络连接之前，若 `spec.Egress.ExpectedIP != ""` 则执行 verify；失败自动 `teardownGateway` + 返回 `ErrEgressIPMismatch`

### Fix B：状态机分裂修复（4 个文件）

- **admin_hosts.go**：List handler 删除 `getDockerStatuses()` 调用及注入循环，列表页响应不再包含 `docker_status` 覆盖
- **worker.go**：Execute 错误路径在构造 errorCode / TaskStatusUpdate 之前插入 `docker stop containerName`，消除 "DB=failed 但 docker=running" 分裂
- **web/admin index.tsx**：
  - `getHostStatus` 删除所有 `docker_status` 优先判断分支，改为以 `host.status` 为唯一数据源
  - `isRunning` 改为 `host.status === "running"`
  - `isStopped` 改为 `host.status === "stopped" || "failed" || "not found"`
- **web/admin $hostId.tsx**：
  - `getHostStatus` 改为优先查 `statusConfig[host.status]`，仅当 DB status 未命中时才降级到 `dockerStatusConfig[docker_status]`

## 验证结果

- `go build ./internal/network/...` PASS
- `go build ./internal/controlplane/http/...` PASS
- `go build ./internal/runtime/tasks/...` PASS
- `go test ./internal/network/... ./internal/runtime/tasks/... ./internal/controlplane/http/...` PASS（无回归）
- `npm run build`（web/admin）PASS

## Deviations from Plan

无 — 计划执行完全按文档执行，无偏差。

## Known Stubs

无 — 所有变更均为实装，无占位符或硬编码空值。

## Self-Check: PASSED

- [x] 修改的 5 个文件均存在
- [x] Commit d43b2c8 存在（`fix(quick-260506): 修复 IP 泄漏`）
- [x] Commit 5ac4163 存在（`fix(quick-260506): 修复状态机分裂`）
- [x] 编译与测试全部通过
