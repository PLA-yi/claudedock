# Quick Task 260405-jji: 镜像版本管理

**Created:** 2026-04-05
**Status:** Executing

## Goal

每次重建/创建主机时自动拉取最新 Docker 镜像，在管理后台展示主机当前镜像版本与最新版本对比，支持一键升级（重建）。

## Tasks

### Task 1: 后端 — 自动拉取最新镜像

**Files:** `internal/runtime/tasks/worker.go`
**Action:** 在 `createHost()` 中 `docker create` 之前添加 `docker pull`
**Verify:** Go 编译通过，`pullImage` 方法存在
**Done:** `docker pull` 在每次 create/rebuild 前执行，失败时回退使用本地镜像

### Task 2: 后端 — 镜像版本 API

**Files:** `internal/controlplane/http/admin_hosts.go`, `internal/controlplane/http/router.go`
**Action:** 新增 `GET /v1/admin/hosts/{hostID}/image-info` 端点，返回容器当前镜像 ID、最新镜像 ID、是否有更新
**Verify:** Go 编译通过，路由已注册
**Done:** API 返回 `container_image_id`, `latest_image_id`, `update_available` 等字段

### Task 3: 前端 — 镜像版本展示与一键升级

**Files:** `web/admin/src/hooks/use-hosts.ts`, `web/admin/src/routes/_dashboard/hosts/$hostId.tsx`, `web/admin/src/components/hosts/host-lifecycle-actions.tsx`
**Action:** 添加 `useHostImageInfo` hook，主机详情页展示镜像版本对比，生命周期操作区增加"升级镜像"按钮（有更新时显示绿色高亮），点击弹出确认弹窗后触发 rebuild
**Verify:** 无 lint 错误，UI 元素存在
**Done:** 镜像版本信息展示、升级按钮和确认弹窗均已实现
