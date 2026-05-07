---
status: diagnosed
trigger: "主机重启后(管理后台 停止→启动)出现状态不一致和 IP 泄漏两个关联问题，怀疑根因是 stop/start 流程在网络命名空间/路由重建上存在缺陷"
created: 2026-05-06T00:00:00Z
updated: 2026-05-06T00:00:00Z
---

## Current Focus

hypothesis: 已纠正之前的误判。IP 泄漏的唯一根因是「user 容器 default 路由仍指向 bridge gw 192.168.215.1，流量根本没进 gw 容器，所以与 sing-box 是否生效完全无关」。状态不一致是独立的状态机问题。
test: 已通过证据复核（用户纠正 + 重读 startHost / PrepareHost 调用链）。
expecting: -
next_action: 输出修正后的 ROOT CAUSE FOUND；建议另开 plan 任务做 user 容器路由修复 + 状态机对齐。

## Symptoms

expected:
- 列表页和详情页状态显示一致
- 主机重启后用户容器仍然走网关容器(gw)的隧道出网，curl ip.me 应返回受控的出口 IP

actual:
- 管理后台执行 停止 然后 启动 后:
  1) 列表页主机运行状态显示"运行中"
  2) 详情页主机名称右侧显示"失败"，下方还出现"启动"按钮
  3) 进入用户容器执行 curl ip.me 返回宿主机真实 IP 222.128.27.131（IP 泄漏）
- 实际上两个 docker 容器都已启动:
  - cloudproxy-gw-90ac81fb-15bc-47d1-b110-08d79ddadc86 (gateway 容器，跑 sing-box)
  - cloudproxy-90ac81fb-15bc-47d1-b110-08d79ddadc86 (用户容器)

errors: 无显式错误消息，但状态显示不一致 + 网络路由不正确

reproduction:
1. 在管理后台对一台已运行的主机执行"停止"
2. 然后执行"启动"
3. 观察列表页和详情页状态差异
4. SSH 进入用户容器，执行 curl ip.me 验证出口 IP

started: 用户刚刚遇到，疑似最近的某次重构引入

## Eliminated

- hypothesis: sing-box gateway 镜像缺 iptables/nft 导致 auto_route 失效，转发流量从 gw 的 bridge 接口直接出宿主真实 IP（旧根因 1）
  evidence: 用户纠正——99.173.22.83 是上游代理给的伪装 IP（**不是**宿主真实 IP）。宿主真实 IP 是 222.128.27.131。在 gw 容器内 `curl ip.me` 返回 99.173.22.83 → sing-box 实际工作正常，auto_route 通过 ip rule policy routing 已经把 gw 自身的出网导进了 tun。`ip route show` 默认不显示 policy rules，正确观察方式是 `ip rule show` + `ip route show table all`。iptables 不是必须，auto_route 通过 ip rule 也能工作。
  timestamp: 2026-05-06 (用户纠正后)

- hypothesis: stop 流程没有同时清理 user 容器和 gw 容器
  evidence: 重读 stopHost (worker.go:437-454) + teardownGateway (container_proxy_provider.go:141-153) — stop 路径会先 `docker stop user`，再 disconnect cp/worker 出 cloudproxy-net、`docker rm -f gw`、`docker network rm cloudproxy-net`、清 config dir。清理路径完整。
  timestamp: 2026-05-06

- hypothesis: start 流程只 docker start 旧 user 容器，但没有重建网络配置
  evidence: 重读 startHost (worker.go:388-435)。顺序为 (1) validateAndPrepare (2) docker start userContainer (3) buildEgressConfig (4) provider.PrepareHost (5) waitForSSH。**第 (4) 步会调用 PrepareHost**，PrepareHost 内部会调用 `dockerNetworkConnect cloudproxy-net workerName workerIP` (line 97) → sleep 1s (line 110) → `configureWorkerEgress` (line 112)。**重启路径上 configureWorkerEgress 确实被调用**。
  timestamp: 2026-05-06

## Evidence

- timestamp: 2026-05-06
  checked: web/admin/src/routes/_dashboard/hosts/index.tsx + admin_hosts.go List handler (`internal/controlplane/http/admin_hosts.go:57-78`)
  found: 列表页使用 `getDockerStatuses()` 通过 `docker ps -a --filter label=cloud-cli-proxy.managed=true` 取得 DockerStatus，UI 在 index.tsx:92 优先用 `docker===running` 显示「运行中」。详情页 GetHostDetail 从 DB 读 host.status，UI（$hostId.tsx:61）把 `failed` 渲染成「失败」+「启动」按钮。两路数据源不同。
  implication: list/detail 显示不一致是「DB 状态 != 实时 docker 状态」；只要 worker.Execute 返回 error，repo.UpdateHostStatus(hostID, "failed") 把 DB 设 failed，docker 容器其实仍在运行 → list=运行中、detail=失败。

- timestamp: 2026-05-06
  checked: internal/runtime/tasks/worker.go:104-138（Execute 错误路径）
  found: Worker.Execute 在任何 error 路径都 `UpdateHostStatus(hostID, "failed")`，但**没有 `docker stop` 已经启动的容器**。这是状态机分裂的源头：容器仍 running，DB 已 failed。
  implication: 即便 list/detail 数据源对齐了，「容器在跑、状态 failed」这种半成品状态对运维和用户都不友好；失败路径应做容器回滚，让 docker ps 与 DB 状态同步。

- timestamp: 2026-05-06 (用户现场证据，已纠正出口 IP 含义)
  checked: docker inspect / ip route / sing-box 日志 / 容器内 curl
  found:
    1. 重启后 user 容器同时挂了 bridge (192.168.215.2) 和 cloudproxy-net (10.99.114.3)，**default route = `192.168.215.1 dev eth1`**（bridge gw）。
    2. gw 容器同时挂了 bridge (192.168.215.3) + cloudproxy-net (10.99.114.2) + tun0 (172.19.0.1/30)；`ip route show` 看到 default = `192.168.215.1 dev eth1`，但这是 main 路由表的视图，sing-box 的策略路由放在 `ip rule` + 独立 table 里（用户纠正：`ip route show` 默认不显示）。
    3. sing-box 进程在跑，日志「started at tun0」「sing-box started」无错。
    4. **gw 自己 `curl https://ip.me` 返回 99.173.22.83，这是上游代理伪装 IP，不是宿主真实 IP**（宿主真实 IP = 222.128.27.131）。**sing-box auto_route 正常工作**——gw 自身出网通过策略路由进了 tun。
    5. **user 容器内 `curl https://ip.me` 返回 222.128.27.131（宿主真实 IP），这是泄漏点**。
  implication:
    - sing-box 完全无辜：gw 内部 curl 返回伪装 IP 证明 auto_route + tun 链路 OK。
    - user 容器流量根本没进 gw —— `default via 192.168.215.1 dev eth1` 直接从 bridge 出宿主，绕过了整个 gw + sing-box 体系。
    - 因此 IP 泄漏的唯一直接原因 = **user 容器 default 路由不指向 gw**。

- timestamp: 2026-05-06
  checked: internal/network/container_proxy_provider.go PrepareHost 全流程 + configureWorkerEgress 脚本
  found:
    1. PrepareHost 重建顺序: writeFile config -> network create -> docker run gw (`--restart unless-stopped`) -> network connect bridge gw -> waitGatewayHealthy(只看进程 Running + 日志没 FATAL) -> network connect cloudproxy-net worker workerIP -> Linux: `disconnect -f bridge worker`（错误被 `_=` 吞掉）-> sleep 1s -> configureWorkerEgress -> 最后 connect cp 进 cloudproxy-net。
    2. configureWorkerEgress 脚本（line 261-276）:
       ```sh
       set -e
       for i in 1..5; do DEV=$(ip -o addr show | grep '<workerIP>' | awk '{print $2}' | head -1); [ -n "$DEV" ] && break; sleep 1; done
       [ -z "$DEV" ] && exit 1
       ip route del default 2>/dev/null || true
       ip route add default via <gwIP> dev "$DEV"
       echo 'nameserver 8.8.8.8' > /etc/resolv.conf
       ```
    3. 脚本只做「删一条 default + 加一条 default + 写 resolv.conf」，没有 verify、没有重试、没有等 docker 网络栈稳定。
    4. configureWorkerEgress 之后是 `dockerNetworkConnect netName cpID`（line 117-122）—这一步在 macOS 上 hostname 不是 docker 容器名，实际会失败被 Warn 吞掉，**不影响 user 容器路由**。
  implication:
    - 重启路径 PrepareHost 整体 return nil（gw 还在跑证明 line 81/88/93/98/113 各 teardown 都没触发），所以 configureWorkerEgress **必然执行了且退出 0**。
    - 但现场 user 容器 default 是 192.168.215.1（bridge gw），不是 gwIP（10.99.114.2）。**configureWorkerEgress 跑了之后被覆盖了**。

- timestamp: 2026-05-06
  checked: deploy/docker/managed-user/Dockerfile + entrypoint.sh
  found: user 容器 entrypoint 没有任何修改 default route 的逻辑。`grep -n "route\|ip\b\|default" entrypoint.sh` 仅匹配到 sysctl ipv6 disable 和无关字符串。
  implication: user 容器内部不主动改路由——覆盖来自外部（docker daemon / Docker Desktop / `--restart=unless-stopped` 自动重启）。

- timestamp: 2026-05-06 (平台路径与 stop→start 时序复盘)
  checked: 现场 bridge 网段 192.168.215.0/24（典型 macOS Docker Desktop 网段，非 Linux 默认 172.17.0.0/16） + container_proxy_provider.go:105 `runtime.GOOS == "linux"` 分支
  found:
    - 该机是 macOS Docker Desktop（设计上 v1 只支持 Linux 单宿主机，但开发者在 macOS 上做开发/复测）。
    - macOS 路径下 PrepareHost 跳过 `docker network disconnect bridge worker`（设计为保留 bridge 给 SSH 端口映射）。
    - stop→start 时序:
      - stopHost: `docker stop user`；`disconnect -f cloudproxy-net worker`（force，对 stopped 容器有效）；`rm -f gw`；`network rm cloudproxy-net`。结果：user 容器持久网络配置 = 仅 bridge。
      - startHost: `docker start user` → docker daemon 按持久配置重建 netns，**重新写 default route 指向 bridge gw 192.168.215.1**；buildEgressConfig；PrepareHost: 新建 cloudproxy-net + gw → `network connect cloudproxy-net user`（worker 加 eth0，docker daemon 不动 default）→ macOS 跳过 disconnect bridge → sleep 1s → configureWorkerEgress 改 default 到 gwIP；最后 connect cp（macOS 上 fail-warn 不影响 worker）。
    - 矛盾：configureWorkerEgress 跑过且退 0（gw 没被 teardown 证明 PrepareHost 整体成功），但现场 default 不是 gwIP。
  implication:
    - 唯一合理的覆盖来源 = `--restart=unless-stopped` 自动重启 user 容器。worker 容器有 `--restart=unless-stopped`（worker.go:190）。如果 user 容器在 PrepareHost 之后某时刻发生过崩溃 / 自动重启（macOS Docker Desktop 在某些情况下会触发），docker daemon 会重建 netns 并写 default = bridge gw（按容器持久配置 NetworkMode=bridge）。configureWorkerEgress **不会**被再次调用——worker 的 startHost 流程已经结束，整个系统不知道 docker daemon 内部又重启了一次。
    - 另一种可能（无现场日志难以排除）：macOS Docker Desktop 在 `docker network connect` 完成后异步刷新路由表，把 default 改回 bridge gw。configureWorkerEgress 在 sleep 1s 后跑，可能跑得太早，路由刷新发生在它之后。
    - 不论哪种机制，根本问题都是：**user 容器的 default 路由可能在 configureWorkerEgress 跑完之后被外部因素覆盖**，且系统没有任何检测/补救——既不 verify、也不持久化、也不让 user 容器 entrypoint 自我修复。

## Resolution

root_cause: |
  **唯一直接根因（IP 泄漏）+ 一个独立的状态机根因：**

  根因 A（IP 泄漏，唯一直接原因）：user 容器在重启路径下 default route = bridge gw（192.168.215.1）而不是 gwIP（10.99.114.2），流量直接从 bridge 接口出宿主，根本没进 gw 容器，所以与 sing-box 是否生效完全无关。

  机制：`configureWorkerEgress`（`internal/network/container_proxy_provider.go:259-284`）在 PrepareHost 中确实被调，且在「stop→start」路径上脚本退出 0（gw 容器仍在跑可证 PrepareHost 整体成功），但 default 路由在 configureWorkerEgress 之后被覆盖。最可能的覆盖来源：
  - user 容器有 `--restart=unless-stopped`（`worker.go:190`），如果在 PrepareHost 之后某时刻自动重启，docker daemon 按持久配置（NetworkMode=bridge）重建 netns 并写 default = bridge gw，**configureWorkerEgress 不会被再调**。
  - 或：macOS Docker Desktop 在 `docker network connect` 之后异步刷新路由表，把 default 改回 bridge gw（configureWorkerEgress 的 sleep 1s 不够长 / 顺序不对）。
  - 配套薄弱点：user 容器在 macOS 路径下保留 bridge（`container_proxy_provider.go:105` 跳过 disconnect bridge）；`configureWorkerEgress` 没 verify、不持久化、不防御外部覆盖；user 容器 entrypoint 不自我修复路由。

  根因 B（状态机分裂，独立问题）：
  - list（`admin_hosts.go:57-78`）用 `docker ps -a` 实时容器状态，UI 优先以容器 running 显示「运行中」。
  - detail（同文件 `:110-155`）只读 DB host.status，UI 把 `failed` 渲染成「失败」+「启动」按钮。
  - `worker.Execute`（`worker.go:104-138`）在任何 error 路径都 `UpdateHostStatus(hostID, "failed")`，但**不停已经启动的容器**。
  - 当 PrepareHost / waitForSSH 之后或之中失败（IP 泄漏导致 SSH 探测失败是常见触发），docker ps=running、DB=failed → list=运行中、detail=失败。

  **A 和 B 的因果关系**：A 让 user 容器路由错乱，可能让 waitForSSH 的 `docker exec workerName bash -c '</dev/tcp/127.0.0.1/22'` 通过（这条用 localhost，不依赖路由），但下游 SSH 探测或控制面别的连接可能因路由问题失败 → DB 置 failed → 触发 B 的状态分裂。但 A 和 B 是各自独立的，可分别修复。

  **撤回的错误判断**：之前把 sing-box gateway 缺 iptables 当作根因 1。错误来源：把 gw 容器内 `curl ip.me` 返回的 99.173.22.83 误判为「宿主真实 IP」，没意识到那是上游代理给的伪装 IP（宿主真实 IP 是 222.128.27.131）。事实是 sing-box auto_route 通过 ip rule policy routing 已经把 gw 自身出网导进了 tun，**`ip route show` 默认看不到 policy rules**，要 `ip rule show` + `ip route show table all` 才看得到。该判断已撤销，sing-box 不需要任何改动。

fix: |
  **修复方案（只需修两件事，本会话只做 ROOT CAUSE FOUND，不直接动代码）：**

  Fix-1（根因 A，必做，决定泄漏存亡）— 让 user 容器的 default 路由不可能回到 bridge：

  推荐方案 A.1（最彻底，强烈建议）：让 user 容器**不再保留 bridge 接口**。
  - `internal/network/container_proxy_provider.go:105`：把 macOS/Windows 的早返回去掉，让所有平台都跑 `docker network disconnect -f bridge worker`。
  - macOS SSH 端口映射的替代方案：在 user 容器创建时（`worker.go:204-206` 的 `-p 0:22`）继续保留宿主端口映射——**docker port mapping 跟 NetworkMode 无关**，只要容器有任何 docker network 即可。bridge 不存在了，cloudproxy-net 上的端口映射照样能从宿主访问到（Docker Desktop 用 vpnkit 透出端口，跟具体 docker network 无关）。
  - 或者：保留 bridge 但让 user 容器在 cloudproxy-net 上的接口拥有 `metric=0` 的 default route + bridge 接口的 default route 的 metric 设为很大（永远不会被选）。
  - 验收口径：`docker exec userContainer ip -o link show` 只看到 lo + cloudproxy-net 接口，没有 bridge eth1。

  推荐方案 A.2（如果 A.1 不可行，作为短期最小修复）：把 user 容器 default 路由的写入做成**幂等 + 自校验 + 持久化**。
  - 重写 `configureWorkerEgress`（`container_proxy_provider.go:259-284`）脚本：
    1. 用 `ip -o route show default` 列出**所有** default 行，逐行 `ip route del default via <gw> dev <iface>` 全部删干净，而不是只删一条。
    2. `ip route add default via <gwIP> dev eth0 metric 0`，给低 metric 防 docker 异步刷新覆盖。
    3. **add 后立即 verify**：`ip route show default | head -1 | grep -q "via <gwIP>"`，不通过就 retry 3 次，3 次都不行就抛错。
    4. 把 `nameserver 8.8.8.8` 改成 sing-box gateway 的 DNS（gwIP）— 这条独立小问题，避免 user 容器走宿主 DNS 解析（8.8.8.8 经过 user 容器路由表，理论上还是从 gw 出去，但更明确点）。
  - 把 user 容器的路由修复**搬到 user 容器自己的 entrypoint** 里，做成「容器启动时第一件事」：
    - `deploy/docker/managed-user/entrypoint.sh` 启动早期：等 `eth0`（cloudproxy-net 接口）出现、然后 `ip route replace default via <gwIP>`。
    - gwIP 通过环境变量传入（`worker.go` buildCreateArgs 加 `-e CCP_GATEWAY_IP=<gwIP>`，或者用约定网段算出来）。
    - 这样无论 docker daemon 何时重建 netns（包括 `--restart=unless-stopped` 自动重启），user 容器一启动就会自我修复路由，不依赖 worker 流程。
  - 防御加固：把 user 容器的 `--restart=unless-stopped` 暂时改回 `--restart=no`，避免 docker daemon 在 worker 流程之外自动重启容器、绕过 PrepareHost。或者保留 unless-stopped 但确保 entrypoint 自我修复路由（推荐）。

  Fix-2（根因 B，独立修，纯状态机）— list / detail 数据源对齐 + 失败路径回滚：
  - list 改成只读 DB host.status，docker 实时状态降为 detail 辅助字段（`admin_hosts.go:57-78` 去掉 DockerStatus 注入；`web/admin/src/routes/_dashboard/hosts/index.tsx:92` 不再用 docker===running 判断）。
  - 或方案 B：list 增加「DB 状态 != docker 实时状态」的 ⚠️ 标记（保留实时性同时不掩盖异常）。
  - 同步在 worker.Execute 失败路径里**补一次 docker stop**（`worker.go:131` 之前先 `runDocker(ctx, "stop", containerName)`），让失败容器回到 stopped 而不是 running。这是根除状态分裂的源头。

  防御加固（强烈建议同步加，避免回归）：
  - PrepareHost 完成后做一次「冒烟探测」：`docker exec worker curl --max-time 5 https://api.ipify.org`，把返回 IP 与 `EgressConfig.ExpectedIP` 比对，不符就 teardown + 报 `ErrEgressIPMismatch`（错误码已存在于 `internal/network/errors.go`）。这是**根因 A 的最后兜底**——即便修复后某个新场景再次让 default 走 bridge，冒烟探测会立刻发现并把状态置 failed，而不是静默 IP 泄漏。
  - waitGatewayHealthy（`container_proxy_provider.go:239`）保持现状即可——sing-box 工作正常，原先认为它要改是误判。

verification: 待修复后由用户做端到端复测：管理后台停止→启动同一台主机；user 容器内 `curl https://ip.me` 必须返回受控 egress IP（比如 99.173.22.83 这种伪装 IP，**不能**返回宿主真实 IP 222.128.27.131）；list 和 detail 状态一致；`docker exec userContainer ip route show default` 必须只剩一条 `via <gwIP> dev eth0`。

files_changed: []
