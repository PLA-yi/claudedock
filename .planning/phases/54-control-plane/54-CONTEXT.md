---
phase: 54
phase_name: 控制面单容器化
milestone: v4.0
created: 2026-05-16
status: discussing
mode: autonomous
---

# Phase 54 CONTEXT: 控制面单容器化

## Phase Goal (from ROADMAP)

control-plane / host-agent 不再创建 gw 容器；`container_proxy_provider` 简化为单容器路径；`cloudproxy-net-*` 自定义 bridge 退役；双绑互斥契约不变。

## Success Criteria (from ROADMAP)

1. 创建一台 host 后只生成一个 docker 容器（`cloudproxy-<id>`），`docker network ls` 不再出现 `cloudproxy-net-<HostID>`。
2. `internal/network/container_proxy_provider.go` 净行数减少 ≥ 300 行；`teardownGateway` 路径整体删除。
3. sing-box config 由 host-agent 在 `PrepareHost`（实际下沉到 `PrepareGateway` 调用窗口，见 D-54-3）时注入 user 容器，文件权限 `root:singbox 0640`；容器启动后该文件被 rm（`docker exec ls /etc/sing-box/` 不见 `config.json`）。
4. v3.6 51-09 双绑互斥 pre-check（`ErrCodeEgressIPAlreadyBound` + 409 + 双语 message）API 行为契约保持不变。
5. `deploy/docker/sing-box-gateway/` 目录退役；Makefile `gateway-image` target 删除；镜像构建产物不再产出 gw 镜像。

## Inherited Decisions (from v4.0 milestone)

| ID | Decision | 在 Phase 54 的体现 |
|---|---|---|
| D-V4-1 | sing-box 进程降权 | Phase 53 已落地（file cap + setpriv uid=9000）；Phase 54 不动 |
| D-V4-2 | config 凭据保护 | Phase 53 entrypoint 已落地 `shred -u`；Phase 54 负责"注入"侧的对称权限（`root:singbox 0640`，见 D-54-2）|
| D-V4-3 | DNS 强制 stub | Phase 53 已落地（resolv.conf 锁 127.0.0.1 + nft drop 53/853）；Phase 54 不再注入 v3.5 双源 `resolv.conf`/`nsswitch.conf` bind mount（见 D-54-5）|
| D-V4-4 | sing-box 死 → 容器死 | Phase 53 已落地 PID 1 fail-closed；Phase 54 在 docker create args 上加 `--restart=on-failure` 兜底（见 D-54-4）|
| D-V4-5 | 不迁移、breaking | Phase 54 直接删 gw 路径，不保留 sidecar 双模式 |
| D-V4-6 | e2e 改造 | Phase 55 接手 |

## Phase 53 关联（Phase 54 必须遵守的下游契约）

参考 `.planning/phases/53-entrypoint/53-VERIFICATION.md` + `53-CR-01` fix（commit `fix(53-CR-01)` 已落地）：

- **config 权限契约**：entrypoint `start_singbox_or_die` (`deploy/docker/managed-user/entrypoint.sh` L178-189) hard-asserts `perm == 640 && owner == root && group == singbox`，任一不符直接 fail-closed（`exit 1`）。Phase 54 host-agent 注入侧必须严格按 `root:9000 0640` 写文件。
- **config 路径契约**：entrypoint 写死消费 `/etc/sing-box/config.json`（同名常量 `SING_BOX_CONFIG`）。Phase 54 bind-mount 目标路径不可变。
- **sing-box 容器内监听**：entrypoint 启动后 sing-box 在容器 netns 内监听 `tun0`（v3.5 gateway config `address=172.19.0.1/30`）和 DNS stub（`127.0.0.1:53`）。这两个端点是容器内 nft 规则的放行/拒绝边界。
- **nft 已由 entrypoint 应用**：`apply_nft_or_die` (entrypoint L243-262) 在容器内自身 netns 应用 `/etc/cloud-claude/default-deny.nft`。Phase 54 host-agent **不再**进入 worker netns 跑 `worker_firewall_linux.go::ApplyWorkerFirewallRules`（见 D-54-7）。
- **`shred -u` 是 entrypoint 责任**：Phase 54 host-agent 写完文件之后什么都不做，容器内 entrypoint 自己负责擦。

## Scouting Findings — 现状盘点

### `internal/network/container_proxy_provider.go`（520 行，待精简）

| 段落 | 行号 | v4.0 处置 |
|---|---|---|
| `PrepareGateway` | L53-142 | 重写：删 gw 容器创建 + bridge 创建 + DNS 源文件 + ruleset placeholder；改为"sing-box config 注入"单一职责 |
| `PrepareHost` | L157-238 | 精简：删 `dockerNetworkConnect`/`docker network disconnect bridge`/`configureWorkerEgress`/`applyWorkerFirewall`/控制面 join 隔离网络；只保留 `verifyWorkerNetwork` + log |
| `CleanupHost` / `teardownGateway` | L240-282 | `teardownGateway` 整段删除；`CleanupHost` 简化为"清理 host 端 config 目录"（无 gw 容器可停，无 bridge 可拆）|
| `GatewayImage` | L284-289 | 删除（无 gw 镜像）|
| `GatewayConfigDir` | L291-301 | 改名为 `SingBoxConfigDir`（语义匹配，保留导出符号给外部 worker.go 用）|
| `WriteContainerDNSConfig` + `resolvConfContent` / `nsswitchConfContent` | L18-38 / L303-321 | 整段删除（v4.0 entrypoint 自己锁 resolv.conf 走 127.0.0.1）|
| `gatewayContainerName` / `networkName` | L323-345 | 删除（无 gw 容器、无 cloudproxy-net 网络）|
| `subnetThirdOctet` | L351-355 | 删除（无自定义 bridge subnet 需要）|
| `dockerNetworkCreate` / `dockerRunGateway` / `dockerNetworkConnect` / `waitGatewayHealthy` / `configureWorkerEgress` / `tryConfigureWorkerEgress` | L357-519 | 整段删除（共 ~163 行）|
| `proxyServerIP` | L327-341 | 保留（仍用于 `verifyWorkerNetwork` 的预期 IP 校验）|

**预估净删行数**：>= 350 行（满足 SC2 ≥ 300 行）。

### `internal/network/container_proxy_provider_linux.go`（49 行）

| 函数 | v4.0 处置 |
|---|---|
| `applyWorkerFirewall` | 删除（容器内 entrypoint 应用 nft）|
| `verifyWorkerNetwork` | 保留（仍是 host-side 出口 IP / DNS / leak 校验入口）|
| `cleanupWorkerFirewall` | 删除（容器死了 netns 也没了，无需 host 侧 cleanup）|

### `internal/runtime/tasks/worker.go::buildCreateArgs`（L209-320）

| 段落 | v4.0 处置 |
|---|---|
| `--network bridge` | 保留（默认 docker bridge，给 SSH 端口转发用）|
| `--cap-add NET_ADMIN` | 保留（sing-box 容器内建 tun 需要）|
| `--cap-drop NET_RAW` | 保留（Phase 51 QUAL-06 收紧，单容器仍有效）|
| `--device /dev/fuse` + apparmor | 保留（mergerfs / sshfs 仍需要）|
| `-p 0:22`（非 linux）| 保留（macOS / docker desktop SSH 端口暴露）|
| **新增** `--device /dev/net/tun` | 加上（sing-box tun 在容器内创建）|
| **新增** `--cap-drop ALL` 后再 `--cap-add NET_ADMIN` 的语义 | 评估：Phase 51 已经走"docker 默认 cap 集合 - NET_RAW + NET_ADMIN"路径，进一步 `--cap-drop ALL` 会破坏 sshd（绑 22）、KasmVNC、mergerfs 等子系统所需的 SETUID/SETGID/CHOWN/DAC_*/SYS_CHROOT 等 cap。**Claude's discretion 决策**：保持 Phase 51 现状（`--cap-add NET_ADMIN --cap-drop NET_RAW`），不再强求 `--cap-drop ALL`，见 D-54-8。|
| **新增** `--restart=on-failure` | 加上（兜底 D-V4-4：sing-box 死 → 容器死 → docker 拉起）|
| **新增** `-v <host-config>:/etc/sing-box/config.json:ro` bind mount | 加上（host-agent 注入 sing-box config）|
| `-v <gwDir>/resolv.conf:/etc/resolv.conf:ro` + nsswitch.conf | **删除**（D-V4-3 entrypoint 自己锁 resolv.conf）|

### `internal/network/call_order_test.go` 调用顺序契约

当前断言 `PrepareGateway → buildCreateArgs → docker create → docker start → PrepareHost` 的硬约束（用于 v3.5+ 的 gateway 时序）。在 v4.0 下，`PrepareGateway` 改名为 sing-box config 注入步骤，**调用顺序契约不变**（注入文件必须在 docker create 之前完成，否则 bind-mount source path 不存在）。测试断言保留，only needle 名字保持不变。

### `internal/controlplane/http/admin_bindings.go`（双绑互斥）

v3.6 51-09 落地的 `Bind()` handler 拦截逻辑（L75-98）：
- 409 + `error_code = "egress_ip_already_bound"` + 中文 message + 英文括号兼容子串 + `host_id` / `egress_ip_id` 字段回显
- 数据源：`store.GetBindingHostIDByEgressIP(ctx, req.EgressIPID)`

Phase 54 **不动这段代码**，但要加一条 unit test 锁死契约（防止后续重构无意打破），与 v3.6 51-09 测试集对齐。`AdminBindingStore` 接口已经定义齐全，无需扩展。

### `deploy/docker/sing-box-gateway/`（待删）

- `Dockerfile` 22 行（基于 debian:bookworm-slim，sing-box 1.13.3）
- `entrypoint.sh`（gateway 容器入口脚本）

Phase 54 整段 `git rm -r`。

### `Makefile`（待清理）

| 段落 | 处置 |
|---|---|
| `.PHONY: gateway-image` 声明 | 删 `gateway-image` |
| `GATEWAY_IMAGE := cloud-cli-proxy-sing-gateway:local` | 删 |
| `gateway-image:` target（5 行）| 删 |
| `dev` target 中非 Linux fallback "build gateway-image" 段（~6 行）| 删 |
| 其他引用 | grep 兜底搜 `GATEWAY_IMAGE` / `gateway-image` 二次清零 |

### `tests/e2e/harness/scenario.go` 等 e2e 资产

含 `WithSingBoxGateway(...)` builder API + gateway 启动同步辅助代码。**Phase 55 接手迁移**，Phase 54 不动；为避免 Phase 55 之前 e2e 编译破坏，Phase 54 保留 `network.GatewayConfigDir` 旧符号 alias（或干脆改名为 `SingBoxConfigDir` 同时 `func GatewayConfigDir() = SingBoxConfigDir` 兼容 1 个 milestone，见 D-54-9）。

## 锁定决策 D-54-* （Phase 54 内部，autonomous mode — Claude's discretion）

> 本 phase 用 autonomous 模式生成，所有灰色区域由 Claude 基于以下三条事实链自动决策，**不向用户提问**：
> 1. Phase 53 已落地的同容器架构（entrypoint 5 函数 + nft + setpriv 降权 + config 权限契约）
> 2. v3.6 控制面已有抽象（Provider 接口 / `ContainerProxyProvider` / host-agent worker.go / admin_bindings.go）
> 3. 51-09 双绑互斥 API 契约（必须原样保留）

### D-54-1 Provider 接口形状

**选择**：保持 `Provider` interface 三个方法 `PrepareGateway` / `PrepareHost` / `CleanupHost` 不变。

**Rationale**：
- `internal/network/provider.go` 接口被 `RoutingProvider` / `ContainerProxyProvider` / `SingBoxProvider` 三处实现 + `internal/runtime/tasks/worker.go` 与 `internal/network/call_order_test.go` 多处消费。改名/裁剪接口会引发全工程跨包破坏，超出本 phase 边界。
- v4.0 下 `PrepareGateway` 的语义从"启动 sidecar gateway 容器 + 写 DNS 源文件"演化为"写 sing-box config 文件（host-agent 注入侧）"。语义平移但接口符号不变。
- `CleanupHost` 仍要清 host 端 `SingBoxConfigDir`（删 config 文件 + 目录），有真实工作量，不是 no-op。

**Alternative**：合并为 `PrepareHost(ctx, spec) error` 单方法。拒绝 — 会引发 `worker.go` / `RoutingProvider` / `SingBoxProvider` / `call_order_test.go` 同步级联修改，超出本 phase 范围。

### D-54-2 sing-box config 权限模型

**选择**：`root:singbox 0640`，与 Phase 53 entrypoint `start_singbox_or_die` hard-assert 严格对齐。

**Rationale**：
- Phase 53 实施过程中 53-CR-01 review finding 已经把"原方案 root:root 0600"修正为"root:singbox 0640"，原因是 sing-box 进程以 uid=9000 (`singbox` 用户) 跑、0600 owner-only read 会导致 sing-box 读不到 config 启动失败。
- entrypoint L178-189 已经 hard-assert：`perm != 640 || owner != root || group != singbox` 三态之一为真则 `exit 1`。Phase 54 host-agent 写入侧必须严格匹配，否则容器永远起不来。
- 写入实现：`os.WriteFile(path, data, 0o640)` + `os.Chown(path, 0, 9000)`。注意 Linux 下 `singbox` 组 GID 也固定为 9000（Dockerfile `groupadd --gid 9000 singbox`），host-agent 直接用 numeric gid=9000 即可，不依赖 host 上 `getent group singbox`。

**Alternative**：tmpfs / docker secret。拒绝 — `F-V4-TMPFS` 已被 v4.0 milestone 显式 defer 到 v4.1（REQUIREMENTS.md "Future Requirements"）。

### D-54-3 注入时机 — `PrepareGateway` 阶段（不是 `PrepareHost`）

**选择**：sing-box config 写盘放在 `PrepareGateway` 阶段（即 `worker.go::createHost` 中 `docker create` **之前**），而非 ROADMAP success criteria 字面写的 `PrepareHost`。

**Rationale**：
- `call_order_test.go::TestWorker_CreateHost_CallOrder` 锁死的顺序是 `PrepareGateway → buildCreateArgs → docker create → docker start → PrepareHost`。bind-mount 引用的源文件**必须**在 `docker create` 时存在，否则 docker 立即报错 "no such file or directory"。
- 故注入逻辑必须在 `PrepareGateway` 完成，`PrepareHost` 只负责"容器起来后做出口 IP / DNS 校验"。
- ROADMAP 描述里说"PrepareHost 时注入"是语义化简称，按"控制面 host-agent 在 host 创建链路中"理解；实现层归 `PrepareGateway`。这是 autonomous 决策、不向用户回滚校正，但在 plan 与 SUMMARY 显式标记，便于 verify 阶段对账。
- success criteria SC3 行为可观测层（启动后 fs 不见 config.json）由 entrypoint 的 `shred -u`/`rm -f` 保证（Phase 53 已落地），与注入时机无关；Phase 54 只对"写入端"负责。

### D-54-4 容器 `--restart=on-failure` 策略

**选择**：在 `buildCreateArgs` 中追加 `--restart=on-failure` （不带 max-retry 上限）。

**Rationale**：
- v3.6 worker 容器是 `--restart=no`（依赖控制面调度恢复），原因是 v3.5 / v3.6 gateway 容器与 worker 容器是两个进程，gw 死 worker 也得控制面感知；现在合并为单容器，sing-box 死 → entrypoint kill PID 1 → 容器死，docker engine 自动拉起更直接、不引入控制面 race。
- 不带 max-retry：sing-box 持续 fail 会触发指数退避（docker 默认 100ms → 200ms → 400ms ... 上限 1m）；管理员能从 `docker ps -a` / `RestartCount` 看见，不需要 docker engine 自己放弃。
- 与 Phase 53 SC5 / NET-03 链路对齐："容器 ≤3s 退出，docker `restart=on-failure` ≤5s 拉起新容器"。

**Alternative**：`--restart=unless-stopped`。拒绝 — 控制面 stopHost 时希望"explicit stop 即停"，`unless-stopped` 会让 docker engine 不主动重启但 host reboot 后又拉起来，与控制面状态机产生竞争。

### D-54-5 删除 v3.5 双源 DNS bind mount

**选择**：删除 `worker.go::buildCreateArgs` 中的 `-v <gwDir>/resolv.conf:/etc/resolv.conf:ro` + `nsswitch.conf` 注入段。

**Rationale**：
- Phase 53 `lock_resolv_conf` (entrypoint L228-241) 自己在容器内 `cat > /etc/resolv.conf` 写死 `nameserver 127.0.0.1`，并 `chattr +i` 兜底；ro bind-mount 反而会阻止 entrypoint 改写。
- `nsswitch.conf` 在 v3.5 用于关 mDNS/myhostname/wins 路径；v4.0 同容器 + nft 全 drop 外部 53/853，理论上即使 nsswitch 走 mdns 也会被 nft 拦截。但为了与 Phase 53 nft `default-deny.nft` 完整对齐、避免遗留干扰，Phase 54 删除该 bind-mount，进一步在 D-54-6 评估是否补 Dockerfile 内置静态 `nsswitch.conf` 兜底。

### D-54-6 `/etc/nsswitch.conf` 兜底（Dockerfile 静态）

**选择**：Phase 54 **不动** Dockerfile，让 ubuntu:24.04 默认 nsswitch（含 `hosts: files dns mdns4_minimal [NOTFOUND=return] mdns4`）维持原样；依赖 nft 兜底拦截 mDNS。

**Rationale**：
- 改 Dockerfile 会强制重 build managed-user 镜像，Phase 53 Plan 53-01 build verify 已经被 D-53-PRE-1（claude.ai/install.sh HTTP 403）阻塞过；本 phase 不想再触发镜像重 build。
- mDNS (UDP 5353) 默认被 `default-deny.nft` output policy drop 拦截（chain 默认 drop + 没有 5353 放行规则）；功能上"安全"，仅仅是 `/etc/nsswitch.conf` 仍有 mDNS lookup 字段不优雅。
- 若 Phase 55 e2e LEAK-mdns 类用例发现绕过，再补静态 nsswitch.conf 到 Dockerfile（Phase 55 / Phase 56 tech debt）。

### D-54-7 `worker_firewall_linux.go::ApplyWorkerFirewallRules` 不再被调用

**选择**：保留 `worker_firewall_linux.go` 文件（不 git rm），但删除 `container_proxy_provider_linux.go::applyWorkerFirewall` 包装函数 + `container_proxy_provider.go::PrepareHost` 中的调用点。

**Rationale**：
- 容器内 nft 由 entrypoint `apply_nft_or_die` 自己跑 (`nft -f /etc/cloud-claude/default-deny.nft`)，不再需要 host 侧通过 `nftables.WithNetNSFd` 进入 worker netns 写规则。
- 但 `ApplyWorkerFirewallRules` 包含大量 v3.5 bypass 路径相关逻辑（`whitelist_v4` set / uid 锁 / linklocal drop）和单测，**整体删除风险大**；保留文件但断开调用，让 dead code 显式留作 Phase 55 / Phase 56 收尾（与 `gateway_singbox_config.go` 同处理）。
- Phase 55 e2e 迁移会重新评估是否需要 host 侧 nft（answer: 不需要），届时一并清理。本 phase 留一条 tech debt 登记。

### D-54-8 容器 cap 集合维持 Phase 51 现状

**选择**：保持 `--cap-add NET_ADMIN --cap-drop NET_RAW`（Phase 51 QUAL-06 落地状态），**不**额外 `--cap-drop ALL`。

**Rationale**：
- `--cap-drop ALL` 会去掉 docker 默认 14 个 cap（含 SETUID / SETGID / CHOWN / DAC_OVERRIDE / DAC_READ_SEARCH / FOWNER / FSETID / KILL / NET_BIND_SERVICE / SETFCAP / SETPCAP / SYS_CHROOT / NET_ADMIN / NET_RAW / SYS_PTRACE / MKNOD / AUDIT_WRITE），entrypoint 几乎所有 `chmod`/`chown`/`shred`/`useradd`/`chpasswd` 都会报 Permission denied。
- 用户给的"建议 `--cap-add NET_ADMIN --cap-drop ALL`"是一种理想态描述；Claude 评估后认为实际工程上要先把所有 setuid 链路全部审计完毕才能落地，**远超 Phase 54 范围**。
- 用户 shell uid=1000 在容器内仍然是非 root（Phase 53 Dockerfile + entrypoint 保证），`getpcaps $$` 输出空集合（NET-04 静态已 satisfy）。"用户没特权"这个 SEC-03 语义并不依赖 `--cap-drop ALL`，依赖 user 进程本身就没在 cap inheritable set 里。
- 后续可在 v4.1 做 `--cap-drop ALL` 化的专项 phase（审计完所有 entrypoint cap 依赖）。

### D-54-9 `GatewayConfigDir` → `SingBoxConfigDir` 改名 + 兼容 alias

**选择**：新增 `SingBoxConfigDir(hostID string) string` 作为主导出符号；保留 `GatewayConfigDir(hostID string) string = SingBoxConfigDir(hostID)` 兼容 alias 一个 milestone（v4.0），Phase 55 / Phase 56 / v4.1 任一阶段把 alias 删掉。

**Rationale**：
- `tests/e2e/` 与可能未列出的归档 PLAN / RESEARCH 文档引用 `network.GatewayConfigDir`；本 phase 不想跨包修改 e2e 资产（Phase 55 接手）。
- 主路径用语义清晰的新名 `SingBoxConfigDir`；alias 单行 + 注释明确"v4.0 兼容，下一个里程碑删除"。

### D-54-10 host 端 `SingBoxConfigDir` 路径布局

**选择**：保持 `<DATA_DIR>/gateway/<host_id>/` 路径不动；config 文件名从 `config.json` → 仍是 `config.json`（与 Phase 53 entrypoint 消费路径 `/etc/sing-box/config.json` 配对，名字一致最少惊讶）。

**Rationale**：
- 路径已经在 `internal/runtime/tasks/worker.go` + `tests/e2e/` 多点引用。改名引发额外 diff，不增加任何安全/可读性收益。
- 路径里含 "gateway" 字样是历史包袱，但 Phase 54 不为净化命名做 grep 替换工作；可在 v4.1 做一次性 rename phase。

### D-54-11 双绑互斥 unit test 加固

**选择**：Plan 54-03 新增 1 个 _table-driven_ Go test，断言 `AdminBindingsHandler.Bind()` 在双绑场景下返回：

1. HTTP status = 409
2. JSON body `error_code` 字段 = `"egress_ip_already_bound"`
3. JSON body `error` 字段同时包含 "出口 IP 已绑定" 中文 + "egress IP already bound" 英文子串（覆盖 v3.6 51-09 双语契约）
4. JSON body `host_id` 字段 = 既有 host id（不是请求 host id）
5. JSON body `egress_ip_id` 字段 = 请求 egress ip id

**Rationale**：v3.6 已有的 `admin_bindings_test.go` 已覆盖此契约；Phase 54 加一个**显式以 `Test_Phase54_DoubleBindingContract_PreservedAfterSingleContainerRefactor` 命名**的"契约保护测试"，作为单容器化重构的不变式锁，避免 reviewer 误删 41 行 pre-check。

### D-54-12 `host-agent` 进程的实际执行边界

**选择**：Phase 54 中"host-agent 注入 sing-box config"实质等同于"`internal/network/ContainerProxyProvider.PrepareGateway` 在 control-plane 进程内完成 `os.WriteFile` + `os.Chown`"。**不引入新的跨进程通道**。

**Rationale**：
- 当前 `cmd/host-agent/` 是单文件 main.go，控制面与 host-agent 默认在同进程跑（v3.6 Makefile dev target 注释 "Agent → embedded (in-process)"）。生产部署也可以拆出独立 binary，但 `network.Provider` 接口在两种模式下是同一个对象。
- 写文件本身是宿主机文件系统操作，不需要 docker exec / docker cp，开销最低、最容易做幂等。
- 安全性：`os.Chown(path, 0, 9000)` 要求控制面进程以 root 跑（v3.6 systemd unit 已经如此），与现状一致。

## 不在本 phase 做的事（明确 out of scope）

- ❌ Phase 53 entrypoint / Dockerfile 任何改动（D-53-* 已落地）
- ❌ Phase 55 e2e 重构（含 `harness.Scenario.WithSingBoxGateway` 合并到 `.WithUser(...)`）
- ❌ Phase 56 CI paths 扩面 + Makefile `e2e` target
- ❌ `worker_firewall_linux.go` / `gateway_singbox_config.go` / `singbox_provider_linux.go` 等 dead code 文件物理删除（保留供 Phase 55 / Phase 56 / v4.1 一次性清理）
- ❌ `RoutingProvider` / `SingBoxProvider` 路径裁剪（接口对齐即可）
- ❌ `--cap-drop ALL` 容器特权进一步收紧（D-54-8 说明）
- ❌ Dockerfile 静态 `/etc/nsswitch.conf`（D-54-6 说明）
- ❌ tmpfs 注入凭据（F-V4-TMPFS 已 defer 到 v4.1）

## Test Strategy（Phase 54 范围）

Phase 54 不跑 e2e（Phase 55 才覆盖单容器 e2e 全套）。Plan 验证按以下三层：

| 层 | 内容 | 覆盖 SC |
|---|---|---|
| Static | grep / line count / 文件存在性断言 | SC2（净行数 ≥ 300 行下降）、SC5（gateway 目录消失 + Makefile target 消失）|
| Unit test (Go) | `ContainerProxyProvider` 单测改造 + 双绑契约 unit test + `worker.go` create args 断言 | SC3（注入文件 root:9000 0640）、SC4（双绑契约保留）|
| Linux 真机 manual smoke | `make dev` 起控制面 + 在 Linux 宿主创建一个 host + `docker network ls` + `docker ps` 双断言 | SC1（只生成一个容器 + 无 `cloudproxy-net-*`）|

真机 smoke 在 darwin 开发机上不可跑（docker desktop VM 限制），列入 `human_verification` deferred-to-Phase-55-CI（与 Phase 53 同款分类策略）。

## Wave Structure 预告

```
Wave 1（独立、可并行）
├── 54-01 container_proxy_provider 单容器路径重构（删 gw + bridge + teardown）
└── 54-03 双绑互斥契约保持 + 单元测试加固（独立文件，跟 54-01 不冲突）

Wave 2（依赖 54-01 完成）
├── 54-02 sing-box 容器内 config 生成 + host-agent 注入 root:singbox 0640
└── 54-04 sing-box-gateway 镜像退役 + Makefile gateway-image target 删除
```

依赖说明：
- 54-02 需要 54-01 先把 `PrepareGateway` 简化为单一职责的注入入口
- 54-04 需要 54-01 先把 `GatewayImage()` 调用点全部删除，否则 Makefile 删了 target 控制面跑不起来

## Discussion Log

| Date | Topic | Decision | Mode |
|---|---|---|---|
| 2026-05-16 | Provider 接口形状 | D-54-1 保持三方法不变 | autonomous |
| 2026-05-16 | config 权限模型 | D-54-2 `root:singbox 0640`（对齐 Phase 53 53-CR-01）| autonomous |
| 2026-05-16 | 注入时机 | D-54-3 实际归 PrepareGateway（call-order 契约要求）| autonomous |
| 2026-05-16 | 容器 restart 策略 | D-54-4 `--restart=on-failure` 兜底 | autonomous |
| 2026-05-16 | DNS bind mount 处置 | D-54-5 删除 v3.5 双源注入 | autonomous |
| 2026-05-16 | nsswitch.conf 静态化 | D-54-6 暂不动 Dockerfile，nft 兜底 | autonomous |
| 2026-05-16 | host 侧 nft 调用 | D-54-7 断开调用点，文件保留 | autonomous |
| 2026-05-16 | 容器 cap 集合 | D-54-8 维持 Phase 51 现状，不 `--cap-drop ALL` | autonomous |
| 2026-05-16 | 符号改名 | D-54-9 `SingBoxConfigDir` + alias `GatewayConfigDir` 兼容一个里程碑 | autonomous |
| 2026-05-16 | host 端路径布局 | D-54-10 沿用 `<DATA_DIR>/gateway/<host_id>/` 不动 | autonomous |
| 2026-05-16 | 双绑契约测试加固 | D-54-11 显式契约保护 test | autonomous |
| 2026-05-16 | host-agent 注入边界 | D-54-12 控制面进程内 `os.WriteFile` + `os.Chown`，不引入跨进程通道 | autonomous |

## Ready for Planning

- 12 phase-internal decisions (D-54-1..12) 全部 autonomous 落地
- 0 open question 留给 plan 阶段（autonomous 模式不留问号）
- Scouting findings 完整对齐 Phase 53 落地状态（entrypoint 权限契约 / nft 路径 / sing-box 容器内监听）
- 4 plan / 2 wave 结构已经规划

**Next:** `/gsd-execute-phase 54` 执行 4 个 plan。
