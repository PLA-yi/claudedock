---
phase: 54-control-plane
plan: "01"
subsystem: infra
tags: [docker, sing-box, networking, refactor, single-container, v4.0]

# Dependency graph
requires:
  - phase: 53-entrypoint
    provides: 容器内 sing-box + entrypoint start_singbox_or_die + apply_nft_or_die（用户镜像自带 sing-box 1.11+ 与 fail-closed nft 规则）
provides:
  - SingBoxConfigDir 单点 + GatewayConfigDir 一里程碑兼容 alias（D-54-9）
  - PrepareGateway 退化为「mkdir + writeContainerSingBoxConfig stub」
  - PrepareHost 退化为「verifyWorkerNetwork 单步」
  - CleanupHost 退化为「os.RemoveAll(SingBoxConfigDir)」单一职责
  - worker.buildCreateArgs：--restart=on-failure + --device /dev/net/tun + sing-box config ro bind mount
  - 删除 sidecar gateway 容器、cloudproxy-net-* 自定义 bridge、teardownGateway 链路
affects: [54-02, 54-03, 54-04, 55, 56]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "单容器架构：user 容器即出口，sing-box 自起 tun0，host-agent 只做 config 注入 + verify"
    - "ro bind mount + entrypoint shred -u 的 PoLP 配置投递模式（D-V4-2）"

key-files:
  created: []
  modified:
    - internal/network/container_proxy_provider.go (519 → 155 行，-364 行)
    - internal/network/container_proxy_provider_linux.go (49 → 22 行)
    - internal/network/container_proxy_provider_other.go (17 → 14 行)
    - internal/network/container_proxy_provider_test.go (重写：删除已退役符号测试，新增 SingBoxConfigDir / CleanupHost 测试)
    - internal/network/provider.go (注释更新，签名 0 行变更)
    - internal/network/verify.go (resolvConfContent 常量从 container_proxy_provider.go 迁入)
    - internal/runtime/tasks/worker.go (buildCreateArgs + createHost/startHost 注释)
  deleted:
    - internal/network/container_proxy_provider_linux_test.go (仅测被删除的 firewall 函数)
    - internal/network/container_proxy_dns_test.go (测试 WriteContainerDNSConfig + worker.go DNS bind-mount needle，全部退役)

key-decisions:
  - "D-54-9：SingBoxConfigDir 新导出 + GatewayConfigDir 一里程碑 alias，避开跨包大改"
  - "D-54-10：物理路径名 \"gateway\" 保留为 v3.x 历史包袱，仅语义重定义；v4.1 再考虑迁移到 sing-box/<host_id>/"
  - "D-54-4：--restart no → on-failure（单容器出口必须可恢复，sing-box 崩溃由 docker 拉起）"
  - "D-54-5：删除 v3.5 容器 DNS 入口锁 bind mount（容器内 sing-box 自接管 DNS）"
  - "D-54-7：applyWorkerFirewall / cleanupWorkerFirewall 两入口删除（entrypoint apply_nft_or_die 接管）"
  - "resolvConfContent 常量迁移到 verify.go 而非整体删除：verifyDNS 在过渡期仍依赖，54-02+ 重设计 verify 路径时再调整"

patterns-established:
  - "writeContainerSingBoxConfig stub fail fast：54-01 留空 stub，docker create 因 bind mount 源 config.json 不存在立即失败，避免 v4.0 链路静默旁路；54-02 实现真正写盘"
  - "CleanupHost best-effort 幂等：失败仅 Warn，不阻断上层 stop / rebuild"

requirements-completed: [CTRL-01, CTRL-02]

# Metrics
duration: ~40min
completed: 2026-05-16
---

# Phase 54 Plan 01: container_proxy_provider 单容器路径重构 Summary

**container_proxy_provider.go 从 519 行精简到 155 行（-364 行），删除 sidecar gateway 容器 / cloudproxy-net-* 自定义 bridge / teardownGateway / worker netns 注入路径，PrepareGateway 退化为「mkdir + writeContainerSingBoxConfig stub」，PrepareHost 退化为「verifyWorkerNetwork」**

## Performance

- **Duration:** ~40 min
- **Started:** 2026-05-16T04:29:00Z
- **Completed:** 2026-05-16T05:10:00Z
- **Tasks:** 8（T1-T8 全部完成，按 PLAN Commit Plan 拆 4 个 atomic commits + 1 个 SUMMARY commit）
- **Files modified:** 7（+2 测试文件删除）

## Accomplishments

- `container_proxy_provider.go` 净行数 519 → 155，超过 PLAN ≥ 300 行下降目标（实际 -364）
- `PrepareGateway` 重写为单一职责：`mkdir SingBoxConfigDir` + `writeContainerSingBoxConfig` stub
- `PrepareHost` 重写为单一职责：`verifyWorkerNetwork` 出口 IP / DNS / leak 三检
- `CleanupHost` 重写为单一职责：`os.RemoveAll(SingBoxConfigDir)`
- `teardownGateway` 整段删除（不再有 sidecar gateway 容器 / 隔离网络需要清理）
- 12 个 dead helper 删除：networkName / gatewayContainerName / subnetThirdOctet / dockerNetworkCreate / dockerRunGateway / dockerNetworkConnect / waitGatewayHealthy / configureWorkerEgress / tryConfigureWorkerEgress / GatewayImage / WriteContainerDNSConfig / proxyServerIP
- 2 个 v3.5 常量删除：gatewayTPProxyPort / nsswitchConfContent；resolvConfContent 迁入 verify.go
- Linux firewall 两入口删除：applyWorkerFirewall / cleanupWorkerFirewall（D-54-7）
- worker.buildCreateArgs 改造：`--restart=on-failure`（D-54-4）+ `--device /dev/net/tun` + sing-box config ro bind mount；删除 v3.5 `/etc/resolv.conf` + `/etc/nsswitch.conf` bind mount（D-54-5）
- 新增 `SingBoxConfigDir` 导出 + `GatewayConfigDir` 一里程碑兼容 alias（D-54-9）
- `Provider` 接口签名 0 行变更（V5 通过），call_order 3 个调用顺序契约测试全 PASS（V7）
- 4 个新增正向测试（V8 SingBoxConfigDir / GatewayConfigDir alias / CleanupHost 删除目录 / CleanupHost 目录不存在幂等）

## Task Commits

按 PLAN Commit Plan 拆 4 个 atomic commits：

1. **T1 + T2: 改造 PrepareGateway + PrepareHost** — `46679b7` (refactor)
2. **T3 + T4 + T5 + T6: 删除 teardownGateway + dead helpers + 改造 CleanupHost + linux/other firewall 入口** — `7a4accd` (refactor)
3. **T7: worker.buildCreateArgs 改造** — `36e9a2f` (refactor)
4. **T8: 测试同步 + 新增 SingBoxConfigDir / CleanupHost 测试** — `3f87815` (test)

**Plan metadata:** _本 commit_ (docs: 54-01-SUMMARY)

## Files Created/Modified

- `internal/network/container_proxy_provider.go` — 主目标文件，519 → 155 行；PrepareGateway / PrepareHost / CleanupHost 全部重写；删除所有 v3.x sidecar gateway helpers；新增 SingBoxConfigDir + GatewayConfigDir alias + writeContainerSingBoxConfig stub
- `internal/network/container_proxy_provider_linux.go` — 删 applyWorkerFirewall + cleanupWorkerFirewall，仅保留 verifyWorkerNetwork
- `internal/network/container_proxy_provider_other.go` — 同步删 non-linux stub
- `internal/network/container_proxy_provider_test.go` — 删 TestNetworkName / TestGatewayContainerName / TestSubnetThirdOctet_* / TestGatewayImage_* / TestConfigureWorkerEgress_* / TestTryConfigureWorkerEgress_*；新增 TestSingBoxConfigDir_DefaultBase / TestSingBoxConfigDir_RespectsDATA_DIR / TestGatewayConfigDir_AliasMatchesSingBoxConfigDir / Test_CleanupHost_RemovesConfigDir / Test_CleanupHost_OnMissingDir
- `internal/network/container_proxy_provider_linux_test.go` — 整文件删除（仅测被删除的 applyWorkerFirewall / cleanupWorkerFirewall）
- `internal/network/container_proxy_dns_test.go` — 整文件删除（测试 WriteContainerDNSConfig + worker.go DNS bind mount needle，全部退役）
- `internal/network/provider.go` — 接口注释更新到单容器架构语义，签名 0 行变更
- `internal/network/verify.go` — resolvConfContent 常量迁入（过渡期 verifyDNS 仍需）
- `internal/runtime/tasks/worker.go` — buildCreateArgs 加 --restart=on-failure + --device /dev/net/tun + sing-box config bind mount，删 v3.5 DNS bind mount；createHost / startHost 注释更新

## Decisions Made

- **resolvConfContent 迁移而非删除**（Rule 3 处理 PLAN 隐含 blocker）：PLAN T1 列出 L18-38 的 resolvConfContent + nsswitchConfContent 常量直接删除，但 `internal/network/verify.go::verifyDNS` 仍以「整体逐字节相等」比对 resolvConfContent 作为 DNS 入口锁判定。直接删除会 break go build。处理：
  - `nsswitchConfContent` 与 `WriteContainerDNSConfig` 一同删除（确认无其他消费者）
  - `resolvConfContent` 迁入 verify.go（与唯一消费者 verifyDNS 同文件），加注释说明 54-02 重设计 verify 路径时再调整
  - 该决策不改变 51-09 双绑互斥 pre-check 行为契约，也不改 ErrCodeEgressIPAlreadyBound / 409 / 双语 message
- **extractProxyServer 保留**：T4 删除清单提到「如果只剩 proxyServerIP 一处调用，保留」。删除 applyWorkerFirewall 后 proxyServerIP 无调用方，故删 proxyServerIP；但 extractProxyServer 仍被 `singbox_provider_linux.go`（Out of Scope dead code）+ `outbound_parse_test.go` 引用，保留
- **过渡期注释清理**：worker.go createHost / startHost 与 provider.go 的注释中残留「teardownGateway / DNS 入口锁」描述，全部更新到单容器架构语义，避免后续 reader 误读。Provider 接口签名 0 行变更，V5 通过

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] resolvConfContent 常量迁移到 verify.go 而非直接删除**

- **Found during:** T4（删除 v3.x consts 阶段）
- **Issue:** PLAN T1 要求删除 L17-38 的 gatewayTPProxyPort / resolvConfContent / nsswitchConfContent 常量；但 `internal/network/verify.go::verifyDNS` 第 326 行用 `rawContent != resolvConfContent` 做 DNS 入口锁判定，直接删除会 break go build
- **Fix:** `nsswitchConfContent` 和 `gatewayTPProxyPort` 直接删除（无其他消费者）；`resolvConfContent` 迁入 verify.go（与唯一消费者 verifyDNS 同文件），加注释「v4.0 单容器架构下 host-agent 不再写盘，但 verifyDNS 在过渡期仍以整体逐字节相等比对来判定 DNS 入口锁是否被破坏，因此保留」
- **Files modified:** `internal/network/container_proxy_provider.go`（删除）、`internal/network/verify.go`（迁入）
- **Verification:** `go build ./...` + `go test -short ./internal/network/...` 全部通过；resolvConfContent 仍可被 verifyDNS 访问；commit 2 SUMMARY 已记录
- **Committed in:** `7a4accd`（Task T3+T4+T5+T6 commit）

**2. [Rule 3 - Housekeeping] 注释里残留 teardownGateway / DNS 入口锁字面引用**

- **Found during:** V2 grep（删完代码后扫剩余命中）
- **Issue:** `internal/network/provider.go` 接口注释、`internal/runtime/tasks/worker.go` createHost / startHost 注释里残留「teardownGateway 与 dockerNetworkCreate 等步骤交错」「PrepareGateway 内部含 teardownGateway → 幂等重起」等描述，新架构下完全错误。PLAN V2 grep 允许注释命中，但语义噪音会误导后续 reader
- **Fix:** 更新所有相关注释到单容器架构语义；Provider 接口注释签名 0 行变更（V5 通过）
- **Files modified:** `internal/network/provider.go`、`internal/runtime/tasks/worker.go`
- **Verification:** V5 git diff 仅显示注释行变化，签名稳定；V7 call-order 测试全 PASS（注释更新不影响 needle 字面量）
- **Committed in:** `3f87815`（Task T8 commit）

**3. [N/A - 非偏离] T8 测试新增 Test_CleanupHost_OnMissingDir 反向断言**

- **Found during:** T8 实现
- **Issue:** PLAN T8 列出 Test_CleanupHost_RemovesConfigDir 正向断言；新 CleanupHost 是 best-effort 幂等（失败仅 Warn 不返错），需补反向断言锁「目录不存在时返回 nil」
- **Fix:** 新增 `Test_CleanupHost_OnMissingDir` 反向断言，确保上层 stop / rebuild 链路在 host 端目录已不存在时也能正常通过 CleanupHost
- **Files modified:** `internal/network/container_proxy_provider_test.go`
- **Verification:** 测试 PASS，符合 D-54 best-effort 幂等约束
- **Committed in:** `3f87815`（Task T8 commit）

---

**Total deviations:** 2 auto-fixed（2 个 Rule 3 blocking + housekeeping）+ 1 test addition（非偏离，T8 加固）
**Impact on plan:** 0 行为契约变更，0 scope creep。51-09 双绑互斥 pre-check / ErrCodeEgressIPAlreadyBound / 409 / 双语 message 0 行 diff。

## Issues Encountered

- **shell `for` 循环 V2 grep 验证脚本 set -e 行为**：在主 shell 内跑 V2 grep 循环时 `rg` 命中数为 0 触发 exit code 1，配合 set -e 让整个循环提前退出。改用 `bash -c` 子 shell + `set +e` 显式覆盖后正常输出。不影响最终 verification 结果
- **commit 4 `git commit` 报「无文件要提交」但 commit 实际成功**：git status 显示工作区已干净是因为前一个 add + commit 链路在错误处理路径上输出了 exit code 1 但实际产出 commit hash（`3f87815`）。已通过 `git log` 二次确认

## Verification Results（V1-V8）

| 验证 | 期望 | 实际 | 状态 |
|---|---|---|---|
| V1 行数 | `container_proxy_provider.go` 净 -≥300 行（旧 520 → 新 ≤ 220） | 旧 519 → 新 155，-364 行 | ✅ |
| V2 grep | 已退役符号在 `internal/network/` + `internal/runtime/tasks/` 代码行 0 命中（注释不计） | 10 个符号代码行全部 0 命中（注释余项 6 处，PLAN 允许） | ✅ |
| V3 worker.go | `on-failure` + `/dev/net/tun` 在 buildCreateArgs 内出现 | 行 221-223（on-failure）+ 236-238（/dev/net/tun） | ✅ |
| V4 `--network bridge` | 紧接的 string literal 仍是 `"bridge"` | 行 216：`"--network", "bridge"` | ✅ |
| V5 Provider 接口 | 签名 0 行 diff（仅注释可加） | 仅注释行 diff，签名 stable | ✅ |
| V6 build + unit test | `go build ./...` 0 error，`go test -short ./...` 全 PASS | 全绿（含 controlplane / e2e harness） | ✅ |
| V7 call-order | TestWorker_{CreateHost,StartHost,RebuildHost}_CallOrder 3 个测试全 PASS | 全 PASS | ✅ |
| V8 SingBox 新测试 | 4 个新测试 PASS | TestSingBoxConfigDir_DefaultBase / TestSingBoxConfigDir_RespectsDATA_DIR / TestGatewayConfigDir_AliasMatchesSingBoxConfigDir / Test_CleanupHost_RemovesConfigDir 全 PASS | ✅ |

## Known Stubs

**`writeContainerSingBoxConfig`** in `internal/network/container_proxy_provider.go:88` — 设计意图 stub 返回 nil，让单容器链路骨架就位。`worker.buildCreateArgs` 的 sing-box config ro bind mount 在 `config.json` 不存在时会让 docker create 立即失败（fail fast），避免 v4.0 链路静默旁路。Plan 54-02 实现真正写盘逻辑后整链才打通；本 plan Risks 章节已显式声明该过渡态为预期行为。

## Threat Flags

无新增网络 / 认证 / 文件访问安全相关表面。本 plan 整体方向是收缩攻击面：

- 删除 sidecar gateway 容器消除一个长存 sing-box 进程的攻击面
- 删除 cloudproxy-net-* 自定义 bridge 消除一个跨容器的 docker network 表面
- 删除 host-agent 进入 worker netns 的能力（applyWorkerFirewall / configureWorkerEgress）将 nft / 路由配置职责完全移到容器内部
- worker.buildCreateArgs 保留 `--cap-drop NET_RAW` + apparmor=unconfined 兜底
- 新增 `--device /dev/net/tun` 表面，但 Phase 53 已验证容器内 sing-box 用法

## Next Phase Readiness

- ✅ Plan 54-02 (host-agent sing-box config 写盘 + 双源 SOCKS5 → VLESS/VMess/...): 单容器链路骨架已就位，`writeContainerSingBoxConfig` stub 等真正实现替换，bind mount 源 / 目的契约已锁
- ✅ Plan 54-03 (双绑互斥契约单测加固): 51-09 错误码 / 状态机 / 双语 message 0 行 diff（grep 确认），可继续加固单测而无契约冲突
- ✅ Plan 54-04 (清理 deploy/docker/sing-box-gateway/ + Makefile gateway-image target): 控制面侧已 0 引用 `cloud-cli-proxy-sing-gateway:local` 镜像，可安全清理
- ⚠️ Phase 55 (e2e harness 迁移 WithSingBoxGateway → 单容器): 本 plan Out of Scope，但通过保留 `GatewayConfigDir` alias 提供了一个里程碑的兼容窗口

## Self-Check

- [x] `internal/network/container_proxy_provider.go` 存在且 155 行（`wc -l`）
- [x] `internal/network/container_proxy_provider_linux_test.go` 已删除（`git ls-files` 不含）
- [x] `internal/network/container_proxy_dns_test.go` 已删除
- [x] commit `46679b7` 存在（T1+T2）
- [x] commit `7a4accd` 存在（T3+T4+T5+T6）
- [x] commit `36e9a2f` 存在（T7）
- [x] commit `3f87815` 存在（T8）
- [x] V1-V8 全部通过

## Self-Check: PASSED

---
*Phase: 54-control-plane*
*Plan: 01*
*Completed: 2026-05-16*
