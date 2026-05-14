---
phase: 47-mvs-governance
status: passed
verified_at: 2026-05-14
verified_by: claude-opus-4-7
mvs_covered: [MVS-06, MVS-07, MVS-08]
---

# Phase 47 — VERIFICATION

## 验证结论

**status: passed** —— 满足 CONTEXT.md §Area 4 决策：「darwin 编译 + 纯函数单测 PASS = `passed`；Linux 真机 e2e 列 `human_verification_needed`（deferred-to-CI，非阻塞 ship）。」

darwin 本地 4 道闸全部通过：

| 闸 | 命令 | 结果 |
|----|------|------|
| 1 | `go build ./tests/e2e/...` | PASS |
| 2 | `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` | PASS |
| 3 | `go test ./tests/e2e/ -run "Helpers" -count=1` | PASS（17 个新增纯函数单测全部 ok） |
| 4 | `bash scripts/lint-no-bare-sleep.sh` | `[ok] tests/e2e 内无裸 time.Sleep` |

附加闸（Phase 47 新引入的生产改动）：

| 闸 | 命令 | 结果 |
|----|------|------|
| 5 | `go build ./cmd/control-plane/...` | PASS |

## MVS 覆盖证据矩阵

| MVS | 主用例（Linux runner） | 纯函数单测（darwin） | 锁定常量 / 表 | 落地文件 |
|-----|-----------------------|---------------------|---------------|----------|
| **MVS-06** 到期容器自动停止 + 审计事件 | `TestExpiry_AutoStop_GoldenPath` @ `tests/e2e/expiry_test.go` | `TestHelpersExpiryEventType_Locked`、`TestHelpersParseEventTypes_{Empty,EmptyArray,SingleEvent,MultiPreservesOrder,InvalidJSON}` @ `tests/e2e/helpers_test.go` | `ExpiryEventType="host.stop.expired"`、`UserExpiredEventType="user.expired"` @ `tests/e2e/helpers.go` | `tests/e2e/expiry_test.go`、`tests/e2e/helpers_linux.go::(*GoldenPath).SimulateExpiry`、`cmd/control-plane/main.go` (env var) |
| **MVS-07** 出口 IP 双绑互斥 | `TestEgressIPBinding_DoubleBindExcluded` @ `tests/e2e/egress_ip_binding_test.go` | `TestHelpersEgressIPDoubleBindContract_Locked`、`TestHelpersParseBindEgressIPResponse_{Success2xx,ConflictWithError,EmptyBody,NonJSONBody}` @ `tests/e2e/helpers_test.go` | `EgressIPDoubleBindContract{WantStatus:409, WantErrSubstring:"already bound"}` @ `tests/e2e/helpers.go` | `tests/e2e/egress_ip_binding_test.go`、`tests/e2e/helpers_linux.go::(*GoldenPath).{AdminLogin,PostBindEgressIP,QueryBindingExists}` |
| **MVS-08** host-agent 心跳与恢复 | `TestHostAgent_KillRecover_GoldenPath` @ `tests/e2e/host_agent_recovery_test.go` | `TestHelpersHostHealthStatus_String`、`TestHelpersParseControlPlaneHealth_{OKAgentOK,WarningAgentUnreachable,DegradedDBError,MissingChecks,InvalidJSON}`、`TestHelpersHostHealthRecoveryContract_Locked` @ `tests/e2e/helpers_test.go` | `HostHealthStatus` 枚举、`HostHealthRecoveryContract{UnhealthyWithin:30s, HealthyWithin:60s}` @ `tests/e2e/helpers.go` | `tests/e2e/host_agent_recovery_test.go`、`tests/e2e/helpers_linux.go::(*GoldenPath).{KillHostAgent,WaitHostHealthStatus}`、`IsEmbeddedHostAgent()` |

## ROADMAP / CONTEXT 偏差汇总

| ID | ROADMAP / CONTEXT 草案 | 源码真相 | 处置位置 |
|----|-----------------------|---------|----------|
| D-47-1 | 审计事件 `host.stopped`（ROADMAP §Phase 47 §Details 1、CONTEXT §Area 1） | 实际事件 `host.stop.expired`，metadata 含 `reason="user expired"`（`internal/controlplane/scheduler/expiry.go::expireUser`） | 常量 `ExpiryEventType` 以源码为准；darwin 单测 `TestHelpersExpiryEventType_Locked` 守护；详见 `47-01-SUMMARY.md` |
| D-47-2 | 环境变量 `EXPIRY_SCANNER_INTERVAL`（CONTEXT §Area 1） | `app.Config.ExpiryScanInterval` 字段；`cmd/control-plane/main.go` 原本不解析任何 env | 本 phase 在 main.go 新增 `EXPIRY_SCAN_INTERVAL`（去掉 `_SCANNER_` 对齐字段名），是 Phase 47 唯一允许的生产改动；详见 `47-01-SUMMARY.md` |
| D-47-3 | 出口 IP 双绑互斥靠 4xx + 稳定 error code（CONTEXT §Area 2） | `host_egress_bindings` 表仅 `UNIQUE (host_id, egress_ip_id)` 复合键（无 `egress_ip_id` 单列 UNIQUE）；`admin_bindings.go::Bind` 无 pre-check；所有错误用 `{"error":"自由文本"}`，无枚举码 | 测试写「期望行为」，断言策略分级（PASS / PARTIAL / BACKEND GAP）；Linux runner 预期跑红，作为 backend gap 的真实证据；建议 Phase 51 QUAL-04 修源码；详见 `47-02-SUMMARY.md` |
| D-47-4 | `GET /v1/admin/hosts/{X}/health` per-host admin endpoint（CONTEXT §Area 3） | 不存在该端点；只有全局 `GET /healthz`（无 admin guard），通过 `checks.agent` 字段表达 host-agent 进程级健康 | 复用 `/healthz` checks.agent；`ParseControlPlaneHealth` 把 `ok/warning/degraded` × `ok/unreachable` 映射到 `HostHealthStatus` 枚举；多宿主机 per-host endpoint 列 Phase 50+ deferred；详见 `47-03-SUMMARY.md` |
| D-47-5 | 健康状态枚举 `healthy / unhealthy`（CONTEXT §Area 3） | `/healthz` 顶层 `status` 取 `ok / warning / degraded`，`checks.agent` 取 `ok / unreachable` | `HostHealthStatus` 抽象掉字面量差异；用例代码不直接依赖任何字符串，全部走枚举 |

## human_verification_needed（deferred-to-CI）

以下 3 项必须在 Linux runner（含 docker + 真实拓扑 + Step 2..7 完整实现）跑通：

1. **MVS-06 `TestExpiry_AutoStop_GoldenPath`**
   - 前置：Phase 46 Plan 01 Step 2 实现并消费 `controlPlaneSpec.ExtraEnv`，把 `EXPIRY_SCAN_INTERVAL=1s` 写入控制面子进程 env。
   - 验证：30s 内容器 `cloudproxy-<host>` status=stopped + events 表出现 `host.stop.expired` 行。

2. **MVS-07 `TestEgressIPBinding_DoubleBindExcluded`**
   - 前置：admin login fixture（admin user + JWT secret 生效）。
   - **预期结果**：基于当前 backend 状态，第二次绑定**不会**返回 409 + `already bound`；测试断言会跑出「BACKEND GAP」分支。这是预期路径（参考 CONTEXT §Area 2「列为 Linux runner deferred + 在 SUMMARY 标记需修源码」），不阻塞 Phase 47 验证通过。
   - 修源码后预期 status=409 + ErrorMessage 含 `already bound` 子串 + A 原绑定不破坏 + B 不被意外写入。

3. **MVS-08 `TestHostAgent_KillRecover_GoldenPath`**
   - 前置：`HOST_AGENT_MODE != embedded`；host-agent 运行在独立容器（默认 `host-agent`，通过 `E2E_HOST_AGENT_CONTAINER` 覆写）；容器内 supervisor 配置「host-agent 退出即重启」。
   - 验证：30s 内 `/healthz` `checks.agent="unreachable"`；60s 内自动回到 `"ok"`，全程不调 force resync。

## 给 Phase 48..52 的接口约定（防止下游漂移）

新增 GoldenPath 方法（`tests/e2e/helpers_linux.go`，`e2e && linux`）：

- `(*GoldenPath).SimulateExpiry(ctx, userID string, waitForTick bool) error`
- `(*GoldenPath).AdminLogin(ctx) (token string, err error)`
- `(*GoldenPath).PostBindEgressIP(ctx, hostID, egressIPID string) (BindEgressIPResponse, error)`
- `(*GoldenPath).QueryBindingExists(ctx, hostID, egressIPID string) (bool, error)`
- `(*GoldenPath).KillHostAgent(ctx) error`
- `(*GoldenPath).WaitHostHealthStatus(ctx, expected HostHealthStatus, timeout time.Duration) error`

新增包级函数 / 类型 / 常量（`tests/e2e/helpers.go`，无 build tag）：

- `IsEmbeddedHostAgent() bool`（实际在 helpers_linux.go，但语义跨平台）
- `ExpiryEventType / UserExpiredEventType` 常量
- `ParseEventTypes(body []byte) ([]string, error)`
- `BindEgressIPResponse` + `ParseBindEgressIPResponse(status, body)`
- `EgressIPDoubleBindContract` 锁定表
- `HostHealthStatus` 枚举 + `String()`
- `ParseControlPlaneHealth(body []byte) (overall, agent HostHealthStatus, err error)`
- `HostHealthRecoveryContract{UnhealthyWithin:30s, HealthyWithin:60s}`

新增 Scenario builder（`tests/e2e/harness/scenario.go`，`e2e`）：

- `(*Scenario).WithControlPlaneEnv(envs map[string]string) *Scenario`
- `controlPlaneSpec.ExtraEnv map[string]string` 字段

生产代码改动（`cmd/control-plane/main.go`）：

- 解析 `EXPIRY_SCAN_INTERVAL` 环境变量，写入 `app.Config.ExpiryScanInterval`；非法值 warn 后落回默认 60s。

## 提交记录

```
6eefe9a docs(47): 拆出 Phase 47 三个 PLAN.md
2782167 feat(47-01): 到期容器自动停止 + host.stop.expired 审计事件 (MVS-06)
077515d feat(47-02): 出口 IP 双绑互斥 e2e 用例 (MVS-07)
8e2cc8f feat(47-03): host-agent 心跳与恢复 e2e 用例 (MVS-08)
```

（VERIFICATION 自身提交在本文件落定后追加。）
