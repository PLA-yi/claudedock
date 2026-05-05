---
phase: quick
plan: 260505-gjs
subsystem: controlplane + admin-frontend
tags: [sse, egress-ip, probe, streaming, eventsource]
dependency_graph:
  requires: []
  provides: [SSE-01, SSE-02, SSE-03]
  affects: [admin_egress_ip_probe.go, router.go, use-egress-ips.ts, test-result-dialog.tsx, egress-ips/index.tsx]
tech_stack:
  added: []
  patterns: [SSE over HTTP, fetch + ReadableStream, Go channel + goroutine]
key_files:
  created: []
  modified:
    - internal/controlplane/http/admin_egress_ip_probe.go
    - internal/controlplane/http/router.go
    - web/admin/src/hooks/use-egress-ips.ts
    - web/admin/src/components/egress-ips/test-result-dialog.tsx
    - web/admin/src/routes/_dashboard/egress-ips/index.tsx
decisions:
  - endpoint 使用 GET 而非 POST，因为原生 EventSource 只支持 GET；在 handler 注释中明确说明此 GET 会触发非幂等的探测操作
  - 前端使用 fetch + ReadableStream 模拟 SSE（而非原生 EventSource），因为需要自定义 Authorization header
  - runProbeStream 中 5 个阶段的时序采用最简方案：在 getProxyDialer 调用前后插入 stage push，避免大面积重构现有探测函数
metrics:
  duration_seconds: 356
  completed_date: "2026-05-05"
---

# Phase quick Plan 260505-gjs: 出口 IP 探测 SSE 流式推送 Summary

将出口 IP 探测从同步 POST 模式改造为 SSE 实时流式推送模式。后端新增 SSE endpoint，探测过程中分阶段推送状态；前端用 fetch + ReadableStream 接收并实时展示进度，最终展示完整测试结果。

## 执行结果

| 任务 | 名称 | 状态 | Commit |
|------|------|------|--------|
| 1 | 后端 SSE 流式探测 endpoint | 完成 | `4ae4948` |
| 2 | 前端 SSE hook 与阶段性弹窗 | 完成 | `a1b8654` |

## 变更详情

### 后端 (Task 1)

**`internal/controlplane/http/admin_egress_ip_probe.go`**
- 新增 `ProbeStage` 类型与 6 个常量：`StagePulling`、`StageStarting`、`StageConnecting`、`StageTesting`、`StageDone`、`StageError`
- 新增 `ProbeStreamEvent` 结构体，包含 `stage`、`message`、`result` 字段
- 新增 `runProbeStream` goroutine 函数：
  - 查询 EgressIP 并校验 proxy_config
  - 按序推送 5 个阶段：pulling -> starting -> connecting -> testing -> done
  - 任何错误推送 `StageError` 后关闭 channel
  - 复用现有 `getProxyDialer`、`testConnectivity`、`testEgressIP` 函数
  - 通过 `defer proxyCleanup()` 确保 sing-box 探针容器和临时文件清理
- 新增 `TestProxyStream()` handler：
  - 设置 SSE headers：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`、`X-Accel-Buffering: no`
  - 创建带缓冲 channel，启动 goroutine 执行 `runProbeStream`
  - 主循环 select 监听 channel、`r.Context().Done()`、`ctx.Done()`（超时）
  - 每条 SSE 消息格式为 `data: {...}\n\n`，写入后立即 `flusher.Flush()`
  - 客户端断开或超时自动退出并触发 goroutine 清理

**`internal/controlplane/http/router.go`**
- 新增路由：`GET /v1/admin/egress-ips/{ipID}/test/stream`
- 原有 `POST /v1/admin/egress-ips/{ipID}/test` 保留不变

### 前端 (Task 2)

**`web/admin/src/hooks/use-egress-ips.ts`**
- 新增 `ProbeStage` 类型与 `ProbeStreamEvent` 接口
- 新增 `useTestEgressIPSSE` hook，返回 `{ stage, message, result, error, isRunning, start, stop }`
  - `start(ipId)`：重置状态为 pulling，通过 fetch + ReadableStream 读取 SSE
  - 解析 SSE 消息（按 `\n\n` 分割，提取 `data:` 行），JSON 解析后更新状态
  - 收到 `done` 或 `error` 阶段时自动停止并关闭 reader
  - `stop()`：调用 AbortController.abort() 中止 fetch
  - 保留原有 `useTestEgressIP`（同步版本）不变

**`web/admin/src/components/egress-ips/test-result-dialog.tsx`**
- 扩展 props 接口，新增 `stage`、`message` 字段
- 新增 `StageProgress` 组件：
  - 4 个步骤（拉取镜像、初始化容器、建立连接、执行检测）
  - 当前步骤高亮（蓝色 + 旋转 Loader），已完成步骤显示勾选图标（绿色），未开始步骤显示灰色序号
- 探测中（`stage` 存在且不是 done/error）展示阶段性进度条 + 当前消息
- 探测出错展示红色错误提示框
- 探测完成展示完整测试结果（连通性、出口 IP、DNS 泄漏），保持原有布局

**`web/admin/src/routes/_dashboard/egress-ips/index.tsx`**
- 引入 `useTestEgressIPSSE` 和 `ProbeStage`
- 替换同步 `testingIds` + `handleTest` 逻辑为 SSE 流式探测
- `handleTest` 调用 `sseTest.start(ip.id)` 并记录 `testDialogIpId`
- `useEffect` 监听 `sseTest.result` 和 `sseTest.stage === "done"`，自动保存结果到 localStorage
- 弹窗状态：`sseTest.isRunning` 或 `sseTest.stage === "error"` 或已有结果时打开
- 表格中检测按钮：如果正在检测当前 IP，显示当前 stage 的简短描述（如"拉取镜像"）
- 下拉菜单测试项同样适配

## 验证结果

- [x] 后端编译通过：`go build ./internal/controlplane/http/...`
- [x] 前端类型检查通过：`cd web/admin && npx tsc --noEmit`

## 偏差记录

无偏差。计划按预期执行，未触发 Rule 1-4。

## 已知 Stub

无。所有功能已完整实现，无占位符或硬编码空值。

## Self-Check: PASSED

- [x] `internal/controlplane/http/admin_egress_ip_probe.go` 存在且包含 `TestProxyStream`、`runProbeStream`、`ProbeStage`、`ProbeStreamEvent`
- [x] `internal/controlplane/http/router.go` 包含 `GET /v1/admin/egress-ips/{ipID}/test/stream` 路由
- [x] `web/admin/src/hooks/use-egress-ips.ts` 包含 `useTestEgressIPSSE`、`ProbeStage`、`ProbeStreamEvent`
- [x] `web/admin/src/components/egress-ips/test-result-dialog.tsx` 支持阶段性进度展示
- [x] `web/admin/src/routes/_dashboard/egress-ips/index.tsx` 调用 SSE hook
- [x] Commit `4ae4948` 存在
- [x] Commit `a1b8654` 存在
