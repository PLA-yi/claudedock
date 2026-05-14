---
phase: 47-mvs-governance
plan: 01
title: 到期容器自动停止 + host.stop.expired 审计事件 (MVS-06)
status: shipped
mvs: MVS-06
created: 2026-05-14
---

# Phase 47 Plan 01 — SUMMARY

## 实际落地

### 新增 / 修改文件

- `tests/e2e/helpers.go`（无 build tag）：
  - 新增常量 `ExpiryEventType = "host.stop.expired"`、`UserExpiredEventType = "user.expired"`。
  - 新增 `ParseEventTypes(body []byte) ([]string, error)` 纯函数，解析 admin events API 响应。
- `tests/e2e/helpers_test.go`（无 build tag）：
  - 新增 5 个纯函数单测：`TestHelpersExpiryEventType_Locked`、`TestHelpersParseEventTypes_{Empty,EmptyArray,SingleEvent,MultiPreservesOrder,InvalidJSON}`。
- `tests/e2e/helpers_linux.go`（`e2e && linux`）：
  - 新增 `(*GoldenPath).SimulateExpiry(ctx, userID, waitForTick)` 方法，直接连 Postgres `UPDATE users.expires_at = NOW() - 1s` + 可选等 `user.expired` 事件落库。
  - 新增内部 helper `disableKeepAliveClient` 与 `getEnvOrDefault`（其余两 plan 共享）。
  - 新增 `pgx/v5` 与 `os` import。
- `tests/e2e/expiry_test.go`（`e2e && linux`，新文件）：`TestExpiry_AutoStop_GoldenPath` 主用例 + 两个内部 helper（`waitHostStatus` 走 admin API、`waitExpiryEvent` 直查 events 表）。
- `tests/e2e/harness/scenario.go`（`e2e`）：
  - 给 `controlPlaneSpec` 加 `ExtraEnv map[string]string` 字段。
  - 新增 `(*Scenario).WithControlPlaneEnv(envs map[string]string) *Scenario` builder 方法；调用前未声明 control-plane → t.Fatalf。
  - **未动** Step 2..7 实现（仍为 sentinel），仅锁定契约，待 Phase 46 Plan 01 续集消费。
- `cmd/control-plane/main.go`（生产代码小幅改动）：
  - 解析 `EXPIRY_SCAN_INTERVAL` 环境变量，写入 `app.Config.ExpiryScanInterval`；非法值 warn 后落回默认。
  - 这是本 plan 唯一的生产改动，纯 config 解析，不动 ExpiryScanner 业务逻辑。

### 锁定契约

- `ExpiryEventType` / `UserExpiredEventType` 常量从源码 `internal/controlplane/scheduler/expiry.go::expireUser` 抽取；任一漂移 → darwin 单测层立即失败。
- `EXPIRY_SCAN_INTERVAL` 环境变量名首次落地，Phase 50+ 若引入 scheduler 重构需保持向后兼容。
- `Scenario.WithControlPlaneEnv` builder 签名首次落地，Phase 48..52 可直接消费。

## 与 PLAN 偏差

- **PLAN 草案曾建议把 `host.stopped` 写进契约**，实际源码事件类型为 `host.stop.expired`；按 CONTEXT §Area 1「以源码为准」决策，常量锁定为 `host.stop.expired`。详见下文 ROADMAP 偏差节。
- PLAN 中提到的 fixture SQL `tests/e2e/fixtures/expired-user-host.sql` 未落地；目前 SimulateExpiry 直接通过 `UPDATE users` 改 `expires_at`，配合 Phase 46 Plan 02..05 已建的 admin API 通路或 scenario Step 3 fixture 即可（无需独立 SQL 文件）。
- PLAN 中 admin API 路径 `GET /v1/admin/hosts/{X}` 的响应解析做了双重 schema 兜底（顶层 `host.status` + 扁平 `status`），增强对未来 schema 演进的韧性；这是落地时新增的防御性逻辑，不影响 PLAN 契约。

## ROADMAP / CONTEXT 偏差

| ROADMAP/CONTEXT 草案 | 源码真相 | 处置 |
|---------------------|----------|------|
| `host.stopped` 事件名（ROADMAP §Phase 47 §Details 1、CONTEXT §Area 1） | 源码 `expiry.go::expireUser` 写入 `host.stop.expired`，metadata 含 `reason="user expired"` | 常量锁 `ExpiryEventType="host.stop.expired"`，darwin 单测 `TestHelpersExpiryEventType_Locked` 守护 |
| `EXPIRY_SCANNER_INTERVAL` 环境变量（CONTEXT §Area 1） | 源码 `app.Config.ExpiryScanInterval` 字段，`cmd/control-plane/main.go` 原本不解析任何相关 env | 本 plan 在 main.go 新增 `EXPIRY_SCAN_INTERVAL`（去掉 `_SCANNER_`），命名靠拢字段名；Phase 47 这是允许的最小生产改动 |
| ExpiryScanner 是否「加速」走 fake clock | 源码无任何 clockwork/fake clock，单条 sleep 由 scheduler.Job 控制 | 不引入第三方依赖，靠 `EXPIRY_SCAN_INTERVAL=1s` 走真实生产路径 |

## Linux 真机验证项（deferred-to-CI）

- `TestExpiry_AutoStop_GoldenPath` 在 Scenario.Start Step 2..7 全部真实实现 + `HOST_AGENT_MODE!=embedded` + `EXPIRY_SCAN_INTERVAL=1s` 下跑通。
- 关键依赖：
  - Phase 46 Plan 01 Step 2 真实启动控制面子进程并消费 `controlPlaneSpec.ExtraEnv`（即把 `EXPIRY_SCAN_INTERVAL=1s` 写进 exec.Cmd.Env）。
  - Step 7 真实启动用户容器（status=running 才会被 ExpiryScanner 看到）。
- 失败模式（CI runner 落地后需观察）：
  - 容器 stop 慢于 30s（受 Docker stop grace period 影响） → 把 waitHostStatus timeout 调到 60s。
  - 事件落库慢于 30s（极少见，但 PG 同步提交慢时可能） → 调 waitExpiryEvent timeout。

## darwin 本地验证

- `go build ./tests/e2e/...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./tests/e2e/ -run "Helpers" -count=1` PASS（5 个新增纯函数单测）。
- `bash scripts/lint-no-bare-sleep.sh` PASS。
- `go build ./cmd/control-plane/...` PASS（main.go 改动后）。

## 给 Phase 48..52 的接口约定

- `GoldenPath.SimulateExpiry(ctx, userID, waitForTick)` —— Phase 50/51 治理回放可复用。
- `ParseEventTypes(body []byte) ([]string, error)` —— 任何需要扫 admin events API 列表的 phase 直接 import。
- `Scenario.WithControlPlaneEnv(envs map[string]string)` —— Phase 48..52 引入新 env 变量时直接挂；不要再为每个变量加新方法。
- `EXPIRY_SCAN_INTERVAL` env 变量 —— 生产部署侧亦可消费，Phase 50 Kill-switch 治理可参考这一命名风格（如 `HEALTH_PROBE_INTERVAL`）。
