---
phase: quick
plan: "260418"
subsystem: runtime / agent
wave: 1
dependency_graph:
  requires: []
  provides: [QUICK-260418]
  affects:
    - internal/runtime/tasks/worker.go
    - internal/runtime/tasks/embedded_dispatcher.go
    - internal/agent/server.go
tech-stack:
  added: []
  patterns:
    - "defer recover() 三层防御"
    - "net/http testHook 模式（TestPanicTrigger 包级变量）"
    - "named return value 让 defer 可修改返回值"
key-files:
  created:
    - internal/runtime/tasks/worker_panic_test.go
    - internal/runtime/tasks/embedded_dispatcher_test.go
    - internal/agent/server_test.go
  modified:
    - internal/runtime/tasks/worker.go
    - internal/runtime/tasks/embedded_dispatcher.go
    - internal/agent/server.go
decisions:
  - "TestPanicTrigger 导出为跨包测试钩子（internal/agent 测试需注入 panic）"
  - "Dispatch 层 recovery 使用 named return (resp, err) 让 defer 可修改返回值"
  - "handler recovery 中 request.Body 已读取后无法重读，使用已解码的 request 构造 fallback"
  - "三层 recovery 均为防御性冗余：worker 捕获 action handler panic，dispatch 捕获 UpdateTaskStatus panic，handler 捕获 HTTP 层 panic"
metrics:
  duration_seconds: 825
  completed_date: "2026-04-24T06:46:45Z"
  tasks: 3
  files: 6
---

# Quick Task 260418: 三层 panic recovery 修复启动排队卡住问题

**One-liner:** 为 Worker.Execute、EmbeddedDispatcher.Dispatch 和 agent HTTP handler 三层添加 panic recovery，确保单个任务 panic 不会导致进程崩溃，且任务状态被正确标记为 failed。

## 执行摘要

| 任务 | 描述 | 提交 |
|------|------|------|
| 1 | Worker.Execute panic recovery + 单测 | e2bdc38 |
| 2 | EmbeddedDispatcher.Dispatch panic recovery + 单测 | 19b098f |
| 3 | agent/server.go HTTP handler panic recovery + 单测 | 15b5f1b |

## 变更详情

### Task 1: Worker.Execute panic recovery

**文件:** `internal/runtime/tasks/worker.go`

- 添加 `TestPanicTrigger` 包级测试钩子（net/http testHook 模式）
- `Execute` 方法改为命名返回值 `(update agentapi.TaskStatusUpdate)`
- `defer recover()` 捕获 panic 后：
  - `slog.Error` 记录 `"worker panic recovered"` 含 task_id/host_id/action/panic
  - 调用 `w.repo.UpdateHostStatus(ctx, request.HostID, "failed")`
  - 构造 `TaskStatusUpdate{Status: failed, ErrorCode: "panic_recovered"}` 返回

**单测:** `internal/runtime/tasks/worker_panic_test.go`

- `TestWorkerExecute_PanicRecovered`：验证 panic 时返回 failed + panic_recovered
- `TestWorkerExecute_PanicRecovered_UpdatesHostStatus`：验证 recovery 后 host status 更新
- `TestWorkerExecute_NoPanic_BehaviorUnchanged`：回归测试，无 panic 时行为不变

### Task 2: EmbeddedDispatcher.Dispatch panic recovery

**文件:** `internal/runtime/tasks/embedded_dispatcher.go`

- `Dispatch` 方法改为命名返回值 `(resp agentapi.HostActionResponse, err error)`
- `defer recover()` 捕获 panic 后：
  - `slog.Error` 记录 `"dispatcher panic recovered"`
  - 构造 fallback update（TaskID 来自 request，避免 worker.Execute 未返回）
  - 尝试 `d.worker.UpdateTaskStatus(ctx, fallback)`
  - 返回 `resp = HostActionResponse{Update: fallback}, err = nil`
- `RunHostAction` 继承 Dispatch 的 recovery（直接调用 Dispatch）

**单测:** `internal/runtime/tasks/embedded_dispatcher_test.go`

- `TestEmbeddedDispatcher_PanicRecovered`：worker panic 时返回 failed update
- `TestEmbeddedDispatcher_NormalPath_Unchanged`：正常路径行为不变
- `TestEmbeddedDispatcher_RunHostAction_PanicRecovered`：适配器路径也受保护

### Task 3: agent HTTP handler panic recovery

**文件:** `internal/agent/server.go`

- `POST /v1/host-actions` handler 顶部添加 `defer recover()`
- 捕获 panic 后：
  - `s.logger.Error` 记录 `"host-agent handler panic recovered"`
  - 构造 fallback update（使用已解码的 request.TaskID）
  - 尝试 `s.worker.UpdateTaskStatus(r.Context(), fallback)`
  - 返回 HTTP 500 + JSON 响应 `HostActionResponse{Update: fallback}`

**单测:** `internal/agent/server_test.go`

- `TestServer_POSTHandler_PanicRecovered`：panic 时返回 HTTP 500 + failed update
- `TestServer_POSTHandler_NormalPath_Unchanged`：正常路径行为不变
- `TestServer_POSTHandler_ServerSurvivesPanic`：连续请求验证 server 存活
- `TestServer_POSTHandler_PanicRecovery_Returns500`：完整 server 集成测试（SKIP，unix socket 需特殊 transport）

## 验证结果

```
go test ./internal/runtime/tasks/... -race -count=1   # PASS
 go test ./internal/agent/... -race -count=1           # PASS
 go build ./...                                         # PASS
```

## Deviations from Plan

**无偏差** — 计划按预期执行，无 Rule 1-4 触发。

### 小调整（非偏差）

1. `testPanicTrigger` 导出为 `TestPanicTrigger`：因为 agent 包测试需要跨包注入 panic 触发器。这是实现层面的必要调整，不改变设计意图。

2. `Dispatch` 和 `Execute` 使用命名返回值：让 `defer recover()` 能在恢复后修改返回值。这是 Go 语言层面的技术选择，计划中已暗示（"返回上述 update"）。

## Self-Check: PASSED

- [x] `internal/runtime/tasks/worker.go` 存在且包含 `defer recover()`
- [x] `internal/runtime/tasks/embedded_dispatcher.go` 存在且包含 `defer recover()`
- [x] `internal/agent/server.go` 存在且 handler 包含 `defer recover()`
- [x] `internal/runtime/tasks/worker_panic_test.go` 存在且 `go test` PASS
- [x] `internal/runtime/tasks/embedded_dispatcher_test.go` 存在且 `go test` PASS
- [x] `internal/agent/server_test.go` 存在且 `go test` PASS
- [x] 提交 e2bdc38 存在于 git 历史
- [x] 提交 19b098f 存在于 git 历史
- [x] 提交 15b5f1b 存在于 git 历史
- [x] `go build ./...` PASS
- [x] `-race` 模式全 PASS
