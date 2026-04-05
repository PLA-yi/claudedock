# Quick Task 260405-jji: 镜像版本管理 — Summary

**Completed:** 2026-04-05

## 变更概要

### 问题
重建/启动主机时，`worker.go` 中 `docker create` 不会自动拉取最新镜像，导致使用过期的本地镜像。用户无法在管理后台看到当前镜像版本，也没有方便的升级入口。

### 解决方案

#### 后端
1. **`internal/runtime/tasks/worker.go`** — `createHost()` 中在 `docker create` 之前添加 `docker pull`，确保每次创建/重建都使用最新镜像。pull 失败时 warn 日志但不阻塞，回退到本地已有镜像。
2. **`internal/controlplane/http/admin_hosts.go`** — 新增 `GetImageInfo()` handler，通过 `docker inspect` 对比容器当前镜像 ID 与 `image.lock` 指定镜像的最新本地 ID，返回是否有可用更新。
3. **`internal/controlplane/http/router.go`** — 注册 `GET /v1/admin/hosts/{hostID}/image-info` 路由。

#### 前端
4. **`web/admin/src/hooks/use-hosts.ts`** — 新增 `useHostImageInfo` hook，60 秒轮询。
5. **`web/admin/src/routes/_dashboard/hosts/$hostId.tsx`** — 主机详情页"配置"区域展示当前镜像 ID 和最新镜像 ID，带"有更新/已最新"标签。
6. **`web/admin/src/components/hosts/host-lifecycle-actions.tsx`** — 当检测到有更新时，显示绿色"升级镜像"按钮。点击弹出确认弹窗，确认后以 `preserve` 模式触发重建。

## 修改的文件
- `internal/runtime/tasks/worker.go` — 添加 `pullImage()` 方法，在 `createHost` 开头调用
- `internal/controlplane/http/admin_hosts.go` — 添加 `GetImageInfo()` handler 和 `shortImageID()` 工具函数
- `internal/controlplane/http/router.go` — 注册 image-info 路由
- `web/admin/src/hooks/use-hosts.ts` — 添加 `HostImageInfo` 类型和 `useHostImageInfo` hook
- `web/admin/src/routes/_dashboard/hosts/$hostId.tsx` — 镜像版本信息展示
- `web/admin/src/components/hosts/host-lifecycle-actions.tsx` — 升级按钮和确认弹窗
