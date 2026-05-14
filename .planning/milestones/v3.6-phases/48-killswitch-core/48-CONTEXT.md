# Phase 48: Kill-switch 核心验证 - Context

**Gathered:** 2026-05-14
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous, auto_rest accept-all)

<domain>
## Phase Boundary

验证两条最关键的 kill-switch：

- MVS-09：sing-box 崩溃后用户容器立即断网（不降级直连）
- MVS-10：容器内手工篡改 `resolv.conf` 仍然走 tun 或被拒绝

**本 phase 交付 2 条 kill-switch 不变量的真实 e2e 用例**：

- 48-01：`docker kill` sing-box gateway → 3 秒内容器 `curl` 失败 + host eth0 抓包零非网关流量
- 48-02：容器内 `cp + mv` 改写 `/etc/resolv.conf` → DNS 仍只走 tun 或被防火墙拒绝 + host eth0 抓包无 UDP/53 → 8.8.8.8

**不在本 phase 范围**：

- 防泄漏 8 条不变量（属 Phase 49）
- Kill-switch 压力测试（属 Phase 50：SIGKILL / tun0 down / Pumba / docker network disconnect）
- verify.go 多目标参数化（属 Phase 51 QUAL-02）

**macOS 本地执行约束**：沿用 Phase 46/47 模式 —— `tests/e2e/` `//go:build e2e && linux` 隔离。

</domain>

<decisions>
## Implementation Decisions

### Area 1: sing-box 崩溃断网 (MVS-09 / 48-01)

- **崩溃触发**：`docker kill <gateway-container-id>`（SIGKILL，模拟运行时崩溃；不发 SIGTERM，避免被 sing-box 自身 graceful shutdown 走 cleanup 路径）。
- **断网断言**：容器内 `curl --max-time 3 https://ifconfig.io`（或 Phase 46 锁定的 3 源之一）必须在 3 秒内失败，exit code 非 0；**不允许 fallback 直连**。
- **host eth0 抓包 oracle**：`tcpdump -i eth0 -nn -c 5 src host <容器IP> and not dst <gateway IP>` 必须 0 包（独立于容器自身报告，避免容器谎报）。
- **timing 契约**：kill 后 ≤ 3 秒断网；超过 3 秒列 fail（与 Phase 50 KILL-01 一致）。

### Area 2: resolv.conf 篡改免疫 (MVS-10 / 48-02)

- **篡改方式**：容器内 `cp /etc/resolv.conf /tmp/r && echo "nameserver 8.8.8.8" > /tmp/r && cat /tmp/r > /etc/resolv.conf`（绕过 ro bind mount 的用户态手法）；如果 `cat >` 失败，改用 `bash -c 'echo nameserver 8.8.8.8 > /etc/resolv.conf'` 但允许失败（用户态绕过失败也是合法防御）。
- **DNS 行为断言**：篡改后立即 `dig +short +time=3 example.com`，预期两种语义之一：
  - 「走 tun 返回正常 A 记录」（说明 nft/tun 接管，用户态改写不生效）
  - 「被防火墙拒绝 / timeout」（说明被显式 drop）
  - 复用 Phase 46 `ClassifyDNSResult` 纯函数。
- **host eth0 抓包 oracle**：`tcpdump -i eth0 -nn -c 5 udp port 53 and dst 8.8.8.8 and src <容器IP>` 必须 0 包（独立 oracle）。
- **失败 artifact**：失败时 dump `cat /etc/resolv.conf` + `nft list ruleset` counter + tcpdump pcap（与 Phase 45 DumpHook 集成）。

### Area 3: GoldenPath 扩展

- **新增方法**（挂 `*GoldenPath`）：
  - `KillGateway(ctx) error` —— `docker kill` gateway container
  - `TamperResolvConf(ctx, target string) (applied bool, err error)` —— 容器内尝试 `cat >` 改写，返回是否成功应用
  - `TcpdumpOnHostEth0(ctx, bpfFilter string, count int, timeout time.Duration) (packets int, err error)` —— host 上 tcpdump，回到独立 oracle 路径
  - `ProbeOutboundFromUser(ctx, url string, timeout time.Duration) (exitCode int, err error)` —— 容器内 `curl --max-time` 探测
- **沿用 Phase 46 `Vote / ClassifyDNSResult / DNSProbeResult`**：DNS 断言直接复用。

### Area 4: 复用 + 验证策略

- **用例隔离**：每用例独立 `StartGoldenPath(t)`（kill gateway 会污染下一用例）。
- **VERIFICATION 策略**：darwin 编译 + 新增纯函数单测 PASS = `status: passed`；Linux 真机 + tcpdump 断言 deferred-to-CI（hosted ubuntu-24.04 runner 需 `sudo tcpdump` 权限，Phase 45 CI workflow 已就绪）。
- **Plan 粒度**：严格 2 plan / 2 用例（48-01, 48-02）。
- **与 Phase 50 边界**：本 phase 只测 `docker kill -SIGKILL` 和 `resolv.conf 篡改`；`tun0 down` / Pumba netem / `docker network disconnect` 是 Phase 50。

### Claude's Discretion

- `KillGateway` 内部是否带 grace 选项（建议固定 SIGKILL，无 grace）
- `TcpdumpOnHostEth0` 的实现路径（建议 `host.Exec` 复用 testcontainer host 或 dedicated privileged sidecar；在 darwin 上 Skip）
- 用例命名：`tests/e2e/killswitch_singbox_crash_test.go` / `killswitch_resolvconf_tamper_test.go`
- 新增纯函数（如 `ParseTcpdumpCountOutput / ClassifyKillswitchResult`）的拆分粒度

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- **Phase 46 已交付** `tests/e2e/helpers.go` + `helpers_linux.go`：复用 `GoldenPath`、`Vote`、`ClassifyDNSResult`、`DNSProbeResult`、`DefaultDenyMatrix`、`BuildDenyProbeCmd`、`SummarizeDenyResults`。
- **Phase 47 已扩** `helpers_linux.go`：复用 `SimulateExpiry / KillHostAgent / WaitHostHealthStatus / PostBindEgressIP` 等方法范式。
- **`harness/scenario.WithControlPlaneEnv`**：Phase 47 加，本 phase 不预期需要额外控制面 env，但 gateway 自身的环境变量入口在 Scenario 中需确认。
- **`tests/e2e/harness/dump.go`**：DumpHook 已挂，失败时自动 dump。
- **`internal/runtime/ContainerProxyProvider`**：gateway / worker 启动真实路径，e2e 跑的是生产路径。
- **`internal/network/`**（如存在）：nftables 规则 + tun 设备管理代码，绕不过这些规则。

### Established Patterns

- **build tag**：`//go:build e2e && linux` 用于 e2e；纯函数单测无 tag。
- **failure-only artifact**：DumpHook 失败时自动调用，本 phase 扩 hook 加 tcpdump / `cat /etc/resolv.conf` 输出。
- **中文沟通**：所有面向用户的 stderr / SUMMARY / commit message 默认中文。

### Integration Points

- **新增 e2e 用例文件**（`e2e && linux`）：
  - `tests/e2e/killswitch_singbox_crash_test.go`（MVS-09 / 48-01）
  - `tests/e2e/killswitch_resolvconf_tamper_test.go`（MVS-10 / 48-02）
- **扩 `tests/e2e/helpers_linux.go`**：4 个新方法（见 Area 3）。
- **扩 `tests/e2e/helpers.go`**（无 build tag）：tcpdump count 解析、kill-switch 结果分类纯函数。
- **扩 `tests/e2e/helpers_test.go`**（无 build tag）：上述纯函数单测。
- **可能扩展 `tests/e2e/harness/dump.go`**：失败时 dump tcpdump pcap / `cat /etc/resolv.conf` 输出到 artifact 目录。
- **不引入新 Go 依赖**。

</code_context>

<specifics>
## Specific Ideas

- **`KillGateway` 实现**：
  ```go
  // docker kill <gateway container id> 直接发 SIGKILL
  ```
- **`TamperResolvConf` 实现**：
  ```go
  // bash -c 'cat > /etc/resolv.conf <<EOF\nnameserver 8.8.8.8\nEOF'
  // 返回 (applied bool, err error)：cat > 成功 = applied=true
  ```
- **`TcpdumpOnHostEth0` 签名草案**：
  ```go
  func (p *GoldenPath) TcpdumpOnHostEth0(ctx context.Context, bpfFilter string, count int, timeout time.Duration) (packets int, err error)
  ```
- **`ParseTcpdumpCountOutput` 纯函数**：解析 `tcpdump -c N` 退出后 stderr 输出形如 `5 packets captured`。

</specifics>

<deferred>
## Deferred Ideas

- **`tun0 down` / Pumba netem / `docker network disconnect`**：属 Phase 50 范围，本 phase 不做。
- **多 worker 并发 kill-switch**：单 worker 验证即可，并发场景列后续。
- **tcpdump 抓包文件持久化策略**：本 phase 失败时 dump 到 artifact，CI 上保留 30 天（与 Phase 45 一致）；持久化 / 长保留属 Phase 52。
- **kill-switch 恢复路径**：本 phase 只测「断网立即生效」，不测「重启 gateway 后恢复」（属性能验证范畴）。
- **resolv.conf 篡改在 ro bind 失败的细化分类**：本 phase 允许「用户态改写失败」即视为合法防御，不强求改写必须成功。
- **Linux runner 真机签字**：deferred-to-CI（与 Phase 46/47 一致）。

</deferred>
