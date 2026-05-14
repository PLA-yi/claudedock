# network/ —— 网络状态（Phase 52 OBS-01..03 收集）

## 里面是什么

宿主机 root namespace 的网络快照。文件清单：

- `nft-ruleset.txt` — `nft list ruleset` 全量输出（含 Phase 51 加的 counter）
- `ip-link.txt` — `ip -o link`（所有 NIC 状态）
- `ip-addr.txt` — `ip -o addr`（所有 NIC IP 地址）
- `ip-route.txt` — `ip route`（默认路由 / tun0 优先级）
- `ip-netns.txt` — `ip netns list`（每个 worker 一个 netns）
- `listen-tcp.txt` — `ss -tln` 或 `netstat -tln`（监听端口列表）

worker netns 内的状态当前**不**采集（CONTEXT §Specifics 决策：Phase 52 v1 不递归 netns，留 v2 扩展）。需要时手动 `ip netns exec <ns> nft list ruleset` 排查。

## 采集命令

```bash
nft list ruleset > network/nft-ruleset.txt 2>&1
ip -o link    > network/ip-link.txt
ip -o addr    > network/ip-addr.txt
ip route      > network/ip-route.txt
ip netns list > network/ip-netns.txt
ss -tln       > network/listen-tcp.txt   # 或 netstat -tln 兜底
```

## 典型排障场景

1. **出口 IP 不对（Phase 46 MVS-02 / Phase 49 LEAK-* 失败）** → 看 `ip-route.txt` 是否走 tun0 作为默认路由；看 `nft-ruleset.txt` 的 `oifname != tun0 drop` 规则是否生效。
2. **nft drop 规则没生效** → grep `nft-ruleset.txt` 找 `169.254.0.0/16 drop`（Phase 51 QUAL-05 加的）、`tcp dport 53 drop` 等；同时检查 counter 字段（Phase 51 给每条规则加了 counter）确认是否被命中。
3. **worker netns 漏建** → 看 `ip-netns.txt` 是否有 `cloudproxy-<host_id>` 字面（命名由 `internal/network/container_proxy_provider.go` 决定）。
4. **kill-switch 失效（Phase 48 MVS-09）** → 看 `ip-link.txt` 中 tun0 是否 UP；`nft-ruleset.txt` 的 `output` chain 是否还有 `accept`。
5. **监听端口冲突** → 看 `listen-tcp.txt`，确认 control-plane / postgres testcontainer 端口未被占用。

## darwin 上为空怎么办

darwin 没有 `nft` 和 `ip`，这两个文件会写「nft not available」/「ip not available」占位。`ss` 在 darwin 也通常没有，会回落到 `netstat`。

这是预期，darwin 上跑只是验证脚本骨架能跑通。真实采集要去 Linux runner。

## 与既有 phase 的关联

- Phase 51 QUAL-05 给所有 nft 规则加了 `counter`，本目录的 `nft-ruleset.txt` 现在能直接看到「每条规则被命中了多少包 / 字节」，比 v3.5 之前更有用。
- Phase 49 LEAK-07 的 link-local drop 规则就在本文件中体现。
- Phase 48 / 50 的 host eth0 tcpdump 走 netshoot sidecar，结果**不**落盘到本目录（只看 stderr 解析包数），所以本目录无 pcap 文件，不与 tcpdump sidecar 流程冲突。
