# Phase 50: Kill-switch 压力测试 - Context

**Gathered:** 2026-05-14
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous, auto_rest accept-all)

<domain>
## Phase Boundary

通过 SIGKILL / `tun0 down` / Pumba netem 故障注入 / `docker network disconnect` 等手段对 kill-switch 做压力测试，确认任何故障姿势下 worker 都不会回落到 host 默认路由。

**4 plan 对应 4 条 KILL 不变量**：

- KILL-01 `docker kill -SIGKILL` sing-box gateway → 3 秒内容器 curl 失败
- KILL-02 `ip link set tun0 down` → 容器 curl 失败
- KILL-03 Pumba netem delay/loss 注入 → SSH 会话存活但出口 IP 校验可能超时（行为契约固定）
- KILL-04 网关容器 `docker network disconnect` → worker 不回落 host 默认路由（host eth0 抓包零非网关流量）

**关键设计**：每条 KILL 用例覆盖一种独立故障姿势，**全部断言 worker 永不 fallback 直连**；与 Phase 48 不同，本 phase 专注「压力 / 极端故障」场景。

**不在本 phase 范围**：

- 防泄漏（属 Phase 49）
- Kill-switch 核心（属 Phase 48 —— 本 phase KILL-01 与 Phase 48 MVS-09 重叠，但 KILL-01 增加「host eth0 抓包零非网关流量 ≤3s」更严格的 timing 契约）
- verify.go 加固 / namespace 重试参数化（Phase 51）

**macOS 本地约束**：`tests/e2e/` `//go:build e2e && linux` 隔离；Pumba 是 Linux-only。

</domain>

<decisions>
## Implementation Decisions

### Area 1: KILL-01 SIGKILL & KILL-02 tun0 down

- **KILL-01 与 Phase 48 MVS-09 关系**：Phase 48 已落 `KillGateway`；KILL-01 复用同方法 + 增加 **timing 严格化**（3 秒内 curl 失败 + host eth0 抓包零非网关流量）。Phase 50 写**专用** `TestKillSwitch_01_SigkillTiming` 用例文件 `tests/e2e/killswitch_stress/` 子目录，与 Phase 48 用例并存（重叠但语义不同）。
- **KILL-02 tun0 down**：新增 `GoldenPath.SetTunDevDown(ctx)` 方法，内部 `docker exec <gateway> ip link set tun0 down`；用例断言容器内 curl 必须失败。
- **timing 锁定**：KILL-01 / KILL-02 都锁 **≤3s** 断网窗口；超过 3s 列 fail。
- **artifact dump**：失败时自动 dump tcpdump + nft ruleset + `ip link / ip addr show`。

### Area 2: KILL-03 Pumba netem 注入

- **故障类型**：`pumba netem --duration 30s delay --time 1000` 给 gateway container 注入 1000ms delay；可选叠加 `pumba netem loss --percent 50`。
- **Pumba 集成**：通过 `docker run --rm gaiaadm/pumba netem ...` 跑 sidecar 进程；e2e 用例内调 `docker run`。
- **行为契约**：
  - **SSH 控制流必须存活**（用 `ProbeSSHInContainer(ctx, timeout)` 试 SSH banner pump，应在 timeout 内拿到 banner）
  - **出口 IP 校验允许超时但不允许给错误的 IP**（Phase 46 `Vote` 多数派语义：如果 ≥2 源返回非预期 IP，fail；如果所有源全 timeout，**inconclusive 不 fail**）
- **netem 清理**：用例结束 `defer pumba cleanup` 或等 `--duration` 自动结束。

### Area 3: KILL-04 docker network disconnect

- **故障触发**：`docker network disconnect <bridge-net> <gateway-container>`，把 gateway 从 docker bridge 摘走。
- **断言**：worker 内 curl 必须失败 + host eth0 `tcpdump src <workerIP>` 零非网关流量（worker **不允许** fallback 到 host bridge 默认路由直连）。
- **恢复路径**：用例不要求恢复（本 phase 只测「不 fallback」契约），但 cleanup 时 `docker network connect` 把 gateway 接回（避免污染后续用例）。
- **新方法**：`GoldenPath.DisconnectGatewayFromBridge(ctx) error` + `ReconnectGatewayToBridge(ctx) error`。

### Area 4: 用例组织 + 验证策略

- **新增目录**：`tests/e2e/killswitch_stress/`，4 个用例文件 + suite_test.go。
- **VERIFICATION 策略**：darwin 编译 + 纯函数单测 PASS = `status: passed`；Linux 真机 + Pumba sidecar + tcpdump 列 `human_verification`（deferred-to-CI；KILL-03 需 Linux runner pull `gaiaadm/pumba` image，已在 CI 网络白名单内）。
- **Plan 粒度**：严格 4 plan / 4 用例（50-01..04）。
- **与 Phase 48 用例文件区分**：Phase 48 文件名 `killswitch_singbox_crash_test.go` / `killswitch_resolvconf_tamper_test.go` 保留；Phase 50 新增 `killswitch_stress/` 目录，避免命名冲突。

### Claude's Discretion

- Pumba image tag 选择（建议 `gaiaadm/pumba:0.10.0` 固定 tag，避免 latest 漂移）
- KILL-03 SSH 存活探测的实现（建议 `docker exec <worker> nc -z localhost 22` 或 `ssh -o ConnectTimeout=3 ...`）
- KILL-04 cleanup 失败容错（建议 t.Cleanup 内 best-effort，log warn 不 fail）
- 4 用例并行 vs 串行（建议串行：故障注入互相影响）
- 纯函数（`ParsePumbaOutput / ClassifyStressVerdict`）的拆分粒度

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- **Phase 48 已交付** `KillGateway / TcpdumpOnHostEth0 / ProbeOutboundFromUser / InspectContainerIPv4`：本 phase 直接复用。
- **Phase 49 已交付** `LeakProbeResult / ClassifyLeakProbe`：可参考但本 phase 用专用 `StressVerdict`（防泄漏 vs 压力测试语义不同）。
- **Phase 46 `Vote`**：KILL-03 出口 IP 多数派投票直接复用。
- **`netshoot` sidecar + tcpdump`**：Phase 48 路径就绪。
- **`tests/e2e/harness/dump.go`**：DumpHook 失败时 dump 容器日志 + nft + ip link 状态。

### Established Patterns

- **build tag**：`//go:build e2e && linux`；纯函数无 tag。
- **failure-only artifact dump**。
- **中文沟通**。

### Integration Points

- **新增目录 `tests/e2e/killswitch_stress/`**：4 用例文件 + suite_test.go（共享 fixture 可选，但故障会污染，建议每用例独立 GoldenPath）。
- **扩 `tests/e2e/helpers_linux.go`**：
  - `SetTunDevDown(ctx) error` —— `docker exec gateway ip link set tun0 down`
  - `SetTunDevUp(ctx) error` —— cleanup 用
  - `DisconnectGatewayFromBridge(ctx) error` —— `docker network disconnect`
  - `ReconnectGatewayToBridge(ctx) error` —— cleanup 用
  - `InjectPumbaNetem(ctx, target string, params PumbaNetemParams) (cleanup func(), err error)` —— Pumba sidecar 运行
  - `ProbeSSHBanner(ctx, timeout) error` —— 容器内 nc 试 22 端口或拿 banner
- **扩 `tests/e2e/helpers.go`**（无 tag）：
  - `StressVerdict` 三值枚举（Pass / Fail / Inconclusive）
  - `KillswitchStressContract` 锁定（KILL-01..04 各自 timing / behavior 契约表）
  - `ParsePumbaOutput` 纯函数（解析 pumba stdout）
  - `ClassifyStressResult(verdict StressVerdict, evidence ...) StressFinding`
- **扩 `tests/e2e/helpers_test.go`**：上述纯函数 fixture-driven 单测，含 testdata 里的 pumba 输出样本。
- **不引入新 Go 依赖**。

</code_context>

<specifics>
## Specific Ideas

- **`PumbaNetemParams` 结构**：
  ```go
  type PumbaNetemParams struct {
      Mode     string        // "delay" / "loss" / "duplicate"
      DelayMs  int           // delay 模式
      LossPct  int           // loss 模式
      Duration time.Duration
  }
  ```
- **`KillswitchStressContract` 锁定表**：
  ```go
  var KillswitchStressContract = map[string]struct {
      MaxDisconnectMs int    // KILL-01/02/04: ≤3000
      SSHAlive        bool   // KILL-03: true
      AllowInconclusive bool // KILL-03: true（出口 IP 投票全超时不 fail）
  }{
      "KILL-01": {MaxDisconnectMs: 3000, SSHAlive: false, AllowInconclusive: false},
      "KILL-02": {MaxDisconnectMs: 3000, SSHAlive: false, AllowInconclusive: false},
      "KILL-03": {MaxDisconnectMs: 0, SSHAlive: true, AllowInconclusive: true},
      "KILL-04": {MaxDisconnectMs: 3000, SSHAlive: false, AllowInconclusive: false},
  }
  ```

</specifics>

<deferred>
## Deferred Ideas

- **更多故障姿势**（CPU 抖动 / OOMKilled / fork bomb 等）：本 phase 锁 4 条，扩展属后续 phase 或里程碑。
- **KILL-04 多 bridge 网络场景**：本 phase 单 bridge，多 bridge 列后续。
- **Pumba 多 target 同时注入**：本 phase 单 target，并行注入列后续。
- **KILL 故障恢复后 worker 自动重连验证**：本 phase 只测故障期间「不 fallback」，恢复路径属性能测试。
- **Linux runner 真机签字**：deferred-to-CI。
- **完整 artifact 采集**：Phase 52 OBS-01..03 范围。

</deferred>
