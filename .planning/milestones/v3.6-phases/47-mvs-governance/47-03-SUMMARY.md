---
phase: 47-mvs-governance
plan: 03
title: host-agent 心跳与恢复 (MVS-08)
status: shipped
mvs: MVS-08
created: 2026-05-14
---

# Phase 47 Plan 03 — SUMMARY

## 实际落地

### 新增 / 修改文件

- `tests/e2e/helpers.go`（无 build tag）：
  - 新增 `HostHealthStatus` 枚举（`Unknown / Healthy / Unhealthy / Degraded`）+ `String()`。
  - 新增 `ParseControlPlaneHealth(body []byte) (overall HostHealthStatus, agent HostHealthStatus, err error)` 纯函数，解析 `/healthz` 响应。
  - 新增 `HostHealthRecoveryContract` 锁定表：`UnhealthyWithin=30s, HealthyWithin=60s`。
- `tests/e2e/helpers_test.go`（无 build tag）：
  - 新增 7 个纯函数单测：`TestHelpersHostHealthStatus_String`、`TestHelpersParseControlPlaneHealth_{OKAgentOK,WarningAgentUnreachable,DegradedDBError,MissingChecks,InvalidJSON}`、`TestHelpersHostHealthRecoveryContract_Locked`。
- `tests/e2e/helpers_linux.go`（`e2e && linux`）：
  - 新增 `(*GoldenPath).KillHostAgent(ctx) error`：`docker exec <container> sh -c 'pkill -9 -f host-agent'`，容器名从 `E2E_HOST_AGENT_CONTAINER` 环境变量取（默认 `host-agent`）。
  - 新增 `(*GoldenPath).WaitHostHealthStatus(ctx, expected, timeout) error`：用 `harness.WaitFor` 轮询 `/healthz`，predicate 解析 JSON body 的 `checks.agent` 字段。
  - 新增 `IsEmbeddedHostAgent() bool` 包级函数：通过 `HOST_AGENT_MODE` 环境变量推断 embedded 模式（embedded 下 kill host-agent = 杀控制面）。
- `tests/e2e/host_agent_recovery_test.go`（`e2e && linux`，新文件）：
  - `TestHostAgent_KillRecover_GoldenPath` 主用例：基线 healthy → kill → 30s 内 unhealthy → 60s 内自动恢复 healthy。
  - embedded 模式 → t.Skip。
  - 总 timeout 150s（30 + 30 + 60 + 30 缓冲）。

### 关键设计

- **/healthz 复用而非新建 per-host endpoint**：单宿主机 v1 下控制面只有一个 host-agent 邻居，全局 `checks.agent` 字段已经精确反映该 host-agent 的进程级健康。多宿主机扩展属未来 phase。
- **DisableKeepAlives**：高频轮询用 keep-alive 复用连接，会因 TCP 半开导致假阳（host-agent 已挂但 conn pool 拿到 stale connection）。`disableKeepAliveClient` 强制单次请求一次握手，避免该问题。
- **不主动调 force resync API**：CONTEXT §Area 3 明确决策——契约是「自动恢复无人工干预」，主动调反而绕过被测路径。
- **`pkill -9 -f host-agent`**：进程级精准杀，不杀容器；容器内 supervisor（dumb-init / systemd / supervisord，本测不挑剔具体实现）负责拉起。

## 与 PLAN 偏差

- PLAN 草案曾建议把 `WaitHostHealthStatus` 通过 `harness.WaitForHTTP` 变体实现，落地直接用 `harness.WaitFor` + 自定义 predicate（更灵活，不需要给 harness 加新 helper）。
- PLAN 草案的 `KillHostAgent` 「自动识别 supervisor 类型」未落地——测试不挑剔具体 supervisor 实现，只要 60s 内 host-agent 进程被拉起即视为满足；类型识别属 deferred 项（CONTEXT §Discretion）。
- 新增 `IsEmbeddedHostAgent()` 包级函数（PLAN 中作为 Skip 条件的描述），让 embedded 模式的跳过逻辑可在其它 plan 复用。

## ROADMAP / CONTEXT 偏差

| ROADMAP/CONTEXT 草案 | 源码真相 | 处置 |
|---------------------|----------|------|
| `GET /v1/admin/hosts/{X}/health` admin endpoint（ROADMAP §Phase 47 §Details 3、CONTEXT §Area 3） | **不存在**。grep `internal/controlplane/http/router.go` 与 `admin_hosts.go`，只有 `/v1/admin/hosts/{hostID}` 主资源 + bypass / claude / VNC 子资源；无 health 子路径。 | 以源码为准复用全局 `GET /healthz`（`router.go:103`，无 admin guard），通过 `checks.agent` 字段表达 host-agent 进程级健康。单宿主机 v1 语义足够。 |
| 「status 字段从 healthy → unhealthy」状态机（CONTEXT §Area 3） | `/healthz` 响应顶层 `status` 取值为 `ok / warning / degraded` 而非 `healthy / unhealthy`；agent 字段在 `checks.agent` 子对象里，取值 `ok / unreachable`。 | `ParseControlPlaneHealth` 把 `ok→Healthy / warning→Unhealthy / degraded→Degraded` 显式映射；`checks.agent` 映射同理。`HostHealthStatus` 枚举抽象掉源码字面量差异。 |
| 「30s unhealthy」上限（CONTEXT §Area 3） | 实际由 `agentapi.Client.Ping()` 的 HTTP 超时（30s 内 default Transport timeout）+ /healthz 内 3s timeout 决定；当 host-agent 进程死后，Ping 立即 ECONNREFUSED，几乎瞬时进入 unhealthy。 | 锁定 30s 上限作为「最坏情况」合同；实际通常 < 5s。 |
| 「60s 恢复 healthy」上限（CONTEXT §Area 3） | 取决于容器内 supervisor 配置（restart delay + host-agent 自身启动时间）。 | 锁定 60s 作为业务契约硬上限；如 Linux runner 跑出超时，触发 supervisor 配置审计而非放宽契约。 |
| 「multi-host 场景」 | v1 仅单宿主机部署，控制面与一个 host-agent 1:1 | 文档化为本 plan 范围决策；多宿主机 per-host health API 列 Phase 50+ deferred。 |

## Linux 真机验证项（deferred-to-CI）

- `TestHostAgent_KillRecover_GoldenPath` 在以下条件下跑通：
  - Scenario.Start Step 2..7 全部真实实现（Phase 46 Plan 01 续集）。
  - `HOST_AGENT_MODE != embedded`（否则 t.Skip）。
  - host-agent 运行在独立容器（默认名 `host-agent`，通过 `E2E_HOST_AGENT_CONTAINER` 覆写）。
  - 容器内 supervisor 配置为「host-agent 退出即重启」（dumb-init 不会自动重启子进程，需 supervisord / s6 / systemd unit）。
- 可能失败模式：
  - 60s 内未恢复 healthy → 多半是 supervisor 配置缺失而非测试代码问题。SUMMARY 标 deploy gap，要求 Phase 50/51 在 host-agent 镜像里落 supervisord / s6 / systemd unit。
  - `pkill -9 -f host-agent` 匹配到非 host-agent 进程（如 host-agent-helper） → 改用 `pkill -9 -x host-agent`（精确匹配）。

## darwin 本地验证

- `go build ./tests/e2e/...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./tests/e2e/ -run "Helpers" -count=1` PASS（17 个新增纯函数单测：5 个 MVS-06 + 5 个 MVS-07 + 7 个 MVS-08）。
- `bash scripts/lint-no-bare-sleep.sh` PASS。

## 给 Phase 48..52 的接口约定

- `HostHealthStatus` 枚举 + `String()` —— Phase 50（Kill-switch 核心）与 Phase 51（resilience hardening）必将复用。
- `ParseControlPlaneHealth(body)` —— 任何需要解析 `/healthz` 的 plan 直接 import；如未来 backend 引入 `checks.network / checks.image_cache` 等新字段，扩展返回值即可。
- `(*GoldenPath).WaitHostHealthStatus(ctx, expected, timeout)` —— Phase 48..50 kill-switch / 防泄漏对抗用例可复用。
- `(*GoldenPath).KillHostAgent(ctx)` —— Phase 48 kill-switch 用例可复用。
- `IsEmbeddedHostAgent()` —— 任何依赖独立 host-agent 进程的用例必须先调它判断 skip。
- `HostHealthRecoveryContract` —— Phase 51 调整 SLA 上限时改这里即可，所有依赖契约的用例自动跟随。
