# Phase 47: MVS 治理与心跳验证 - Context

**Gathered:** 2026-05-14
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous, 4/4 grey areas accepted)

<domain>
## Phase Boundary

把"到期容器自动停止 + 出口 IP 双绑互斥 + host-agent 心跳与恢复"三条治理路径变成 e2e 自动用例，杜绝"上线后才发现治理逻辑没生效"。

**本 phase 交付 3 条治理不变量的真实 e2e 用例**（每条对应一个 plan）：

- MVS-06：到期容器自动停止 + `host.stopped` 事件落库
- MVS-07：同一出口 IP 第二次绑定被拒绝（4xx + 稳定 error code），原绑定不破坏
- MVS-08：host-agent 心跳与恢复（30s unhealthy / 60s 恢复 healthy，无人工干预）

**不在本 phase 范围**：

- Kill-switch（属 Phase 48 / 50）
- 防泄漏对抗（属 Phase 49）
- verify.go 代码层加固（属 Phase 51）
- 完整 artifact 采集（属 Phase 52）

**macOS 本地执行约束**：沿用 Phase 46 模式 —— `tests/e2e/` 用例 `//go:build e2e && linux` 隔离，darwin 跑纯函数单测，Linux runner 跑断言。

</domain>

<decisions>
## Implementation Decisions

### Area 1: 到期 + ExpiryScanner (MVS-06)

- **ExpiryScanner 加速方式**：环境变量 `EXPIRY_SCANNER_INTERVAL=1s` 缩短轮询周期；**不引入 clockwork/fake clock 库**（避免影响生产代码路径）。如果现有 ExpiryScanner 没有可配置间隔，先确认环境变量名再写测试；若名称不一致以源码为准。
- **到期 fixture**：DB 预置 `users.expires_at = now - 1s` 的过期用户 + 已运行容器；scanner 触发后断言容器停 + `events` 表入审计。
- **`host.stopped` 事件断言**：查 `events` 表 `kind='host.stopped' AND host_id=X` 30s 内出现；行内含 `reason` 字段标注「过期触发」。
- **用例总 timeout**：60s（30s 等 scanner + 30s 缓冲）。

### Area 2: 出口 IP 双绑互斥 (MVS-07)

- **互斥触发**：调真控制面 `POST /v1/admin/hosts/{B}/egress-ips`（或现有实际 API 路径，以源码为准）传已绑给 A 的 IP；**不 DB 直接插冲突行**（绕过 API 层会漏掉 SQL 约束之外的应用层校验）。
- **期望响应**：HTTP **409 Conflict** + 稳定 error code（先 grep `internal/controlplane/http/` 取已存在常量，例如 `egress_ip_already_bound` 或类似）；如果当前实现是 400 / 500 / 没有 error code，把这一项列为 Linux runner deferred + 在 SUMMARY 标记需修源码。
- **A 绑定不破坏**：用例最后查 DB `host_egress_ips` 表确认 (host_id=A, ip=X) 行仍存在；不允许 API 错误处理把原绑定也回滚。
- **error code 来源**：grep `cmd/control-plane/` `internal/controlplane/` `internal/agentapi/` 取当前 error code 常量；漂移以源码为准并在 SUMMARY 中记录差异。

### Area 3: host-agent 心跳与恢复 (MVS-08)

- **kill 方式**：`pkill -9 -f host-agent`（进程级杀），保持 docker container 存活、container 内 supervisor / systemd / dumb-init 拉起；**不用 `docker kill`** 整容器（会绕过本测要测的"进程级恢复"语义）。
- **unhealthy 断言**：轮询 `GET /v1/admin/hosts/{X}/health`（或现有 admin health API），30s 内 status 从 `healthy` → `unhealthy`；使用 Phase 45 `waitFor.WaitForHTTP` 变体，predicate 解析 JSON `status` 字段。
- **healthy 恢复**：60s 内 status 回到 `healthy`（业务契约写死）；如果 60s 内没回，列为 Linux runner deferred 而非 darwin fail。
- **force resync**：用例**不主动调** force resync API；契约是"自动恢复无人工干预"，主动调反而绕过了被测的恢复路径。

### Area 4: 跨用例复用 + 验证策略

- **用例隔离**：每用例独立 `StartGoldenPath(t)`，**不共享 suite fixture**（到期会停容器，污染下一用例；kill host-agent 同理）。
- **GoldenPath 扩展**：在 `tests/e2e/helpers_linux.go` 给 `*GoldenPath` 加方法：
  - `SimulateExpiry(ctx, userID) error` —— UPDATE `users.expires_at = now - 1s` + 等下一次 scanner tick
  - `KillHostAgent(ctx) error` —— `docker exec` + `pkill -9 -f host-agent`
  - `WaitHostHealthStatus(ctx, hostID, expected string, timeout) error` —— 复用 `waitFor.WaitForHTTP`
  - `PostBindEgressIP(ctx, hostID, ip string) (status int, errorCode string, err error)` —— 调控制面 admin API，返回 status + error code 二元组
- **VERIFICATION 策略**：darwin 编译 + 纯函数单测 PASS = `status: passed`；Linux 真机 e2e 列 `human_verification_needed`（deferred-to-CI，非阻塞 ship）。
- **Plan 粒度**：严格 3 plan 对应 3 用例（47-01..03），每 plan 独立 commit。

### Claude's Discretion

以下细节实现层由子代理自行决定：

- `SimulateExpiry` 内部要不要 sleep 一个 ExpiryScanner 周期才返回（建议加可选 `waitForTick bool` 参数）
- `WaitHostHealthStatus` 的轮询间隔（建议 500ms-1s）
- `PostBindEgressIP` 的请求体结构（按 admin API 当前 schema 写）
- 心跳测试中 supervisor 类型自动识别（dumb-init / systemd / supervisord，按 container 内现状）
- 用例命名：`tests/e2e/expiry_test.go` / `egress_ip_binding_test.go` / `host_agent_recovery_test.go`
- 纯函数单测的拆分粒度（建议挂在 `helpers_test.go` 已有的 table-driven 套件下）

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- **Phase 46 已交付** `tests/e2e/helpers.go`（纯函数 + 锁定表）和 `tests/e2e/helpers_linux.go`（`GoldenPath` + `StartGoldenPath` + `RunBootstrapScript`）—— 本 phase 直接 import 并扩方法。
- **`tests/e2e/harness/{scenario, waitfor, artifacts, dump}`** —— Phase 45 骨架，Phase 47 直接复用。
- **Phase 46 锁定的纯函数**：`Vote / ClassifyDNSResult / DefaultDenyMatrix / BootstrapExitCodeContract` —— Phase 47 不动这些，新增治理相关纯函数。
- **`internal/controlplane/`**：admin API 的 handler / error code 常量定义所在；grep 取真相。
- **`internal/eventlog`**（或 `internal/agentapi/events`，以源码为准）：events 表读写 SDK。
- **ExpiryScanner**：在 `internal/scheduler/` 或 `cmd/control-plane/scheduler/` 中（待 grep 确认）；环境变量名以源码为准。
- **`internal/agentapi`**：host-agent ↔ 控制面 SDK，health endpoint schema。

### Established Patterns

- **build tag**：`//go:build e2e && linux` 用于 e2e 用例；纯函数 helpers_test.go 无 tag。
- **错误包装**：`fmt.Errorf("...: %w", err)` 链式。
- **waitFor 用法**：Phase 45 4 个变体已就绪，本 phase 心跳轮询用 `WaitForHTTP`，事件断言用 `WaitFor`（自定义 predicate）。
- **failure-only artifact dump**：DumpHook 已挂，本 phase 用例失败时自动 dump 容器日志 + events 表 + nft ruleset。
- **中文沟通**：所有 SUMMARY / VERIFICATION / commit message 默认中文。

### Integration Points

- **新增 e2e 用例文件**（均 `//go:build e2e && linux`）：
  - `tests/e2e/expiry_test.go`（MVS-06）
  - `tests/e2e/egress_ip_binding_test.go`（MVS-07）
  - `tests/e2e/host_agent_recovery_test.go`（MVS-08）
- **扩展 `tests/e2e/helpers_linux.go`**：新增 4 个 `GoldenPath` 方法（SimulateExpiry / KillHostAgent / WaitHostHealthStatus / PostBindEgressIP）。
- **扩展 `tests/e2e/helpers_test.go`**：新增治理相关纯函数单测（如 PostBindEgressIPResponse 解析、HostHealthStatus 枚举映射）。
- **可能扩展 `tests/e2e/harness/scenario`**：如果当前 builder 不支持以特定 `EXPIRY_SCANNER_INTERVAL` 启动控制面，需在 scenario 加一个 option，例如 `WithControlPlaneEnv(map[string]string)`。
- **fixture**：如果需要 admin API 调用的 admin token / session，沿用 Phase 46 已建的 fixture 通路（或在 `tests/e2e/fixtures/` 加 admin-bootstrap.sql）。
- **不引入新 Go 依赖**（沿用 Phase 45/46 已落地的 testcontainers-go / testify / waitFor）。

</code_context>

<specifics>
## Specific Ideas

- **`SimulateExpiry` 签名草案**：
  ```go
  func (p *GoldenPath) SimulateExpiry(ctx context.Context, userID string, waitForTick bool) error
  ```
- **`KillHostAgent` 实现思路**：
  ```go
  // docker exec host-agent-container pkill -9 -f host-agent
  // 不杀容器，让容器内 supervisor 拉起
  ```
- **`WaitHostHealthStatus` 签名草案**：
  ```go
  func (p *GoldenPath) WaitHostHealthStatus(ctx context.Context, hostID string, expected HostHealthStatus, timeout time.Duration) error
  // 内部用 waitFor.WaitForHTTP，predicate 解析 JSON `status` 字段
  ```
- **`PostBindEgressIP` 签名草案**：
  ```go
  func (p *GoldenPath) PostBindEgressIP(ctx context.Context, hostID, ip string) (status int, errorCode string, body []byte, err error)
  // 返回三元组：status + error code + 原始 body
  ```
- **error code 常量锁定**：若 grep 拿到 `ErrCodeEgressIPAlreadyBound` 或类似，作为锁定值；漂移在 SUMMARY 记录。

</specifics>

<deferred>
## Deferred Ideas

- **Linux runner 真机签字**：本 phase 不在 darwin 上强证，列 deferred-to-CI。
- **多 host 同时争抢同一 IP**：本 phase 单 A/B 两 host 串行测试，并发争抢列后续 phase 或单独 phase。
- **expiry 触发的容器优雅停止 vs 强杀**：本 phase 只断言「停了」+ 「事件入库」，停止方式（SIGTERM grace / SIGKILL）属 deferred。
- **心跳恢复的最大时间窗实测分布**：本 phase 锁 60s 上限，不做分布统计。
- **error code 国际化**：当前 stderr / API error message 中英文混合断言 contains，多语言切换列 v3.7+。
- **完整 artifact 采集**：本 phase 沿用 Phase 45 占位 hook，Phase 52 (OBS-01..03) 补完整 5 子目录采集。

</deferred>
