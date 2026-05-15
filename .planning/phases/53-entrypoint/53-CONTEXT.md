---
phase: 53
phase_name: 镜像与 entrypoint 基线
milestone: v4.0
created: 2026-05-16
status: discussing
---

# Phase 53 CONTEXT: 镜像与 entrypoint 基线

## Phase Goal (from ROADMAP)

managed-user 镜像内置 sing-box + entrypoint 串接启动序列，本地手工 `docker run` 起来后 `curl ip.me` 走绑定的出口 IP；用户 SSH 进来非 root、无 NET_ADMIN、读不到 sing-box config。

## Inherited Decisions (from v4.0 milestone, see PROJECT.md)

| ID | Decision | Choice | Applies to Phase 53 |
|---|---|---|---|
| D-V4-1 | sing-box 进程身份 | setuid 降权到非 root 账号 | ✅ entrypoint 实现 |
| D-V4-2 | config 凭据保护 | root:root 0600 + 启动后 rm | ✅ entrypoint 实现 |
| D-V4-3 | DNS 策略 | nft drop 外部 53/853 + sing-box stub | ✅ entrypoint 实现 |
| D-V4-4 | sing-box 死 | PID 1 fail-closed | ✅ entrypoint 实现 |

## Scouting Findings — 现有资产盘点

### 当前 managed-user 镜像（v3.3.0）

| 项 | 现状 | 对 v4.0 的含义 |
|---|---|---|
| 基础镜像 | `ubuntu:24.04` | 沿用，与 v3.6 worker 维度对齐 |
| PID 1 | `/usr/bin/tini -- /usr/local/bin/entrypoint.sh` | **复用**。tini 的 signal handling 是 sing-box fail-closed 的天然基础 |
| 默认用户 | `workspace` (uid=1000) | 保留；用户名可由 `CONTAINER_USER` 环境变量覆盖 |
| 用户特权 | ⚠️ **`workspace ALL=(ALL) NOPASSWD:ALL`**（Dockerfile L108 + entrypoint L181 重写） | 🚨 **v4.0 必须删除**。这是潜在 SEC-01..03 漏洞——用户能 sudo 后 kill sing-box / 读 config / 改 ruleset |
| sing-box binary | ❌ 未安装在镜像中 | 需要 IMG-01 加进去（参考 sing-box-gateway Dockerfile ARG=1.13.3） |
| sing-box 启动代码 | ✅ `entrypoint.sh` L316-384 已有 `MODE=local` 分支 | **复用 + 改造**。失败语义从"WARNING fallback proxy"改为"FATAL exit 1" |
| `singbox` 系统账号 | ❌ 不存在 | IMG-02 新增 `useradd singbox uid=9000` |
| nftables 工具 | ❌ `nft` binary 不在镜像 | IMG-03 加 `nftables` 包 |
| IPv6 关闭 | ✅ L210-211 `sysctl -w net.ipv6.conf.*.disable_ipv6=1` | 复用 |
| IPv4 forwarding / rp_filter | 未显式配置 | v4.0 entrypoint 加 sysctl |
| `/dev/fuse` chmod 666 | L213-215 | 复用 |

### 当前 sing-box gateway 镜像

| 项 | 现状 |
|---|---|
| 基础镜像 | `debian:bookworm-slim`（glibc，必须） |
| sing-box 版本 | **1.13.3** (`ARG SINGBOX_VERSION=1.13.3` in `deploy/docker/sing-box-gateway/Dockerfile`) |
| binary 来源 | `https://github.com/SagerNet/sing-box/releases/download/v1.13.3/sing-box-1.13.3-linux-${ARCH}.tar.gz` |
| entrypoint | 独立 `deploy/docker/sing-box-gateway/entrypoint.sh` |

**v4.0 决策**：sing-box binary 安装方式直接复制 sing-box-gateway Dockerfile 的 L11-18 段到 managed-user Dockerfile，**版本 ARG 提取为 build arg 共享**（避免两处漂移）。Phase 54 退役 sing-box-gateway 镜像。

### 当前 nft 规则集应用路径

`internal/network/worker_firewall_linux.go::ApplyWorkerFirewallRules` 当前签名：

```go
func ApplyWorkerFirewallRules(containerNS netns.NsHandle, gwIP, bridgeGW, proxyIP net.IP, sshPort uint16) error
```

**调用位置**：`container_proxy_provider_linux.go::applyWorkerFirewall` 从 host 端进入 worker netns 应用规则（L11-30）。

**v4.0 改造方向**（在 Phase 54 落地，Phase 53 暂只需镜像内置 `nft` binary）：

- 拆出"规则集生成"纯函数（无 netns 依赖），导出为 entrypoint 可调用的形式
- 选项 A：entrypoint 内调用 `nft -f /etc/cloud-claude/default-deny.nft` 应用规则
- 选项 B：保留 host-agent 进入容器 netns 注入（架构更复杂但减少容器内 `nft` 依赖）
- **推荐 A**：与"sing-box 同容器"语义一致，容器自带完整网络栈

### 现有 sing-box 配置 schema

Phase 53 暂不深入，由 Phase 54 host-agent 注入路径定义。Phase 53 只需消费"已注入到 `/etc/sing-box/config.json` 的合法 config"。

## 锁定决策 D-53-*（Phase 53 内部）

### D-53-1 sing-box 安装方式

**选择**：直接在 managed-user Dockerfile 中加 `ARG SINGBOX_VERSION=1.13.3` + 同款 curl + install 步骤，从 GitHub release 拉 binary。

**Rationale**：与 sing-box-gateway Dockerfile 一致；不引入 apt 仓库依赖；版本 ARG 提取为 build arg，构建脚本（Phase 54 退役 gateway 时统一）可共享。

**Alternative considered**：apt 包 `sing-box`（debian/ubuntu 没有官方 apt 仓库）—— 拒绝。

### D-53-2 sing-box 运行身份

**选择**：sing-box 通过其 `process.user` 字段在 config 中声明 setuid 到 `singbox` 账号（uid=9000）。

**Verification required**：sing-box 1.13.x 官方 docs 是否支持 `process.user`？需要 Phase 53 plan 阶段先做 spike：

```bash
# Phase 53 spike: 验证 process.user 行为
docker run --rm --cap-add NET_ADMIN -v config:/etc/sing-box ghcr.io/sagernet/sing-box:v1.13.3 \
  run -c /etc/sing-box/config.json
# 配置内 "process": { "user": "singbox" }
# 期待：sing-box 建 tun 后切换到 uid=9000
```

**Fallback if `process.user` 不支持**：使用 `capsh` + `setpriv` 在 entrypoint 内手动建好 tun 后 setcap → exec sing-box。复杂但可行。

### D-53-3 sing-box 启动失败语义

**选择**：A —— PID 1 fail-closed。

**实现**：

```bash
# entrypoint.sh 简化骨架
sing-box run -c /etc/sing-box/config.json &
SINGBOX_PID=$!

# WaitFor tun0 ready (复用 v3.6 WaitFor 语义，但容器内是 bash)
for i in $(seq 1 30); do
  ip link show tun0 >/dev/null 2>&1 && break
  sleep 0.5
done
ip link show tun0 >/dev/null 2>&1 || { echo "[FATAL] sing-box tun0 未就绪" >&2; kill $SINGBOX_PID; exit 1; }

# 应用 nft default-deny
nft -f /etc/cloud-claude/default-deny.nft || { echo "[FATAL] nft 规则应用失败" >&2; kill $SINGBOX_PID; exit 1; }

# 删除 config 文件（D-V4-2）
shred -u /etc/sing-box/config.json 2>/dev/null || rm -f /etc/sing-box/config.json

# sing-box 死亡监控（fail-closed）
monitor_singbox() {
  wait $SINGBOX_PID
  echo "[FATAL] sing-box 退出 (exit=$?)，容器退出" >&2
  kill -TERM 1  # 让 tini 优雅退出整个容器
}
monitor_singbox &

# 降权 + sshd
exec /usr/sbin/sshd -D -e
```

**关键点**：

- 监控子进程 `wait $SINGBOX_PID` → 退出后 `kill -TERM 1` 让 tini 关停整个容器
- docker run 时 `--restart=on-failure` 触发重建（Phase 54 控制面落地）
- sshd 仍是 foreground 进程，但 sing-box 死 → kill PID 1 → 整个容器死

### D-53-4 `workspace` 用户 sudo 权限处理

**选择**：**删除** sudo NOPASSWD。Dockerfile L108-109 直接删除该 RUN 段；entrypoint L180-182 改为：

```bash
# v4.0: 显式删除 sudoers 配置（兜底，防止历史镜像残留）
rm -f /etc/sudoers.d/workspace /etc/sudoers.d/${RUN_USER}
```

**Breaking change 告知**：v4.0 release notes 必须显式说明用户不再有 sudo。任何依赖 `sudo apt install` 的用户工作流需迁移到 v4.0 image 重建时预装。

**Mitigation**：如果用户确实需要在容器内装包，可走"管理员从面板下发预装清单 → 镜像层添加"路径（v4.1 候选）。

### D-53-5 DNS 强制走 sing-box

**选择**：A 强化 —— sing-box stub resolver + nft drop 外部 DNS。

**实现**：

```bash
# entrypoint.sh 在 nft 规则应用前
cat > /etc/resolv.conf <<EOF
# v4.0: DNS 强制走 sing-box stub resolver
nameserver 127.0.0.1
options edns0 trust-ad
EOF
chmod 0644 /etc/resolv.conf
chattr +i /etc/resolv.conf 2>/dev/null || true  # immutable，防用户改
```

**nft 规则集追加段**（Phase 54 落 sing-box config schema 时定稿）：

```
# 外部 UDP/53、TCP/53、TCP/853 (DoT) 全 drop
ip protocol udp udp dport 53 ip daddr != 127.0.0.1 drop
ip protocol tcp tcp dport { 53, 853 } ip daddr != 127.0.0.1 drop
# DoH 常见端口路径（仅在 sing-box 解析后通过 tun 出网允许）
# 不在 nft 拦截 443（业务流量必须走），由 sing-box DNS over tun 接管
```

### D-53-6 `/dev/net/tun` 暴露

**选择**：通过 `--device /dev/net/tun` 注入。Phase 53 验证 docker run 命令行可行性；Phase 54 落地到 container_proxy_provider。

**Cap 配置**：`--cap-add NET_ADMIN`（必须，sing-box 建 tun 需要）。

**Cap drop**：`--cap-drop ALL` 之后 `--cap-add NET_ADMIN`（仅授予 sing-box 需要的最小集合）。sing-box setuid 后 NET_ADMIN 在 inheritable set 中不传递给子进程（验证项，与 D-53-2 一同 spike）。

## Open Questions（提交给 plan 阶段研究）

1. **sing-box 1.13.x `process.user` 字段确切名称与行为**：官方文档 / source code 需 plan 阶段先扫一遍。备选关键字：`users`、`user`、`process.user`、`process.uid`。
2. **NET_ADMIN cap inheritance**：sing-box setuid 到 singbox 用户后，子进程（如果有）能否继承 NET_ADMIN？理论上 ambient cap 应清空，但需实证。
3. **tun device cgroup 白名单**：除了 `--device /dev/net/tun`，是否需要 `--device-cgroup-rule "c 10:200 rwm"`？取决于 docker engine 版本与 cgroup v1/v2。
4. **`chattr +i /etc/resolv.conf`** 在 ubuntu:24.04 + overlayfs 是否生效？需要 spike。
5. **现有 MODE=local 分支兼容性**：v4.0 移除 MODE 区分（所有 managed-user 都是 sing-box-in-container），还是保留 MODE=local 作为本地 CLI 路径？倾向 **移除**，本地 CLI 走同一 entrypoint。

## Non-Goals（Phase 53 不做）

- ❌ host-agent 控制面变更（Phase 54）
- ❌ sing-box config schema 设计（Phase 54）
- ❌ e2e 用例重构（Phase 55）
- ❌ KasmVNC / desktop / VNC 部分变更（保持现有 MODE=remote 行为）
- ❌ sing-box DNS rule 表 / route 表细化（Phase 54）
- ❌ entrypoint 在 cgroup v1 vs v2 的细分（按当前 ubuntu:24.04 默认 cgroup v2 路径走）

## Test Strategy（Phase 53 内部验证，Phase 55 完整 e2e）

Phase 53 不要求跑 e2e（Phase 55 才覆盖）。但要求每个 Plan 内部至少有一条本地 docker 手测脚本：

| 测试名 | 命令 | 期望 |
|---|---|---|
| T-53-1 | `docker run --rm --device /dev/net/tun --cap-add NET_ADMIN -v $(pwd)/test-config.json:/etc/sing-box/config.json:ro managed-user:v4-dev sleep 60`，另一终端 `docker exec` 看 tun0 | tun0 接口存在 + sing-box 进程跑在 uid=9000 |
| T-53-2 | 上面容器内 `cat /etc/sing-box/config.json` | 失败（文件已 rm） |
| T-53-3 | 上面容器内 `curl https://ip.me` | 返回 config 中的上游出口 IP，不是宿主真实 IP |
| T-53-4 | 上面容器内 `getpcaps $$`（user shell） | 空 cap |
| T-53-5 | 上面容器内 `docker exec <c> kill -9 $(pidof sing-box)` | 容器在 ≤3s 内退出 |
| T-53-6 | 同 T-53-5 后 `docker ps -a` | 容器 status=exited，docker `--restart=on-failure` 触发重建 |

## Discussion Log

| Date | Topic | Decision |
|---|---|---|
| 2026-05-16 | sing-box 进程身份 | D-V4-1 锁定 setuid 降权（user 拍板）|
| 2026-05-16 | config 凭据保护 | D-V4-2 锁定 0600+rm（user 拍板）|
| 2026-05-16 | DNS 策略 | D-V4-3 锁定 nft+stub（user 拍板）|
| 2026-05-16 | sing-box 死语义 | D-V4-4 锁定 fail-closed（user 拍板）|
| 2026-05-16 | 用户迁移 | D-V4-5 不迁移，v4.0 breaking |
| 2026-05-16 | e2e 改造 | D-V4-6 按草案 |
| 2026-05-16 | 现 workspace sudo | D-53-4 删除（v4.0 breaking 告知） |
| 2026-05-16 | sing-box 安装方式 | D-53-1 同 gateway 镜像（GitHub release） |
| 2026-05-16 | resolv.conf 锁定 | D-53-5 nameserver 127.0.0.1 + chattr +i |

## Ready for Planning

✅ All 6 v4.0 milestone decisions inherited
✅ 6 phase-internal decisions (D-53-1..6) locked
✅ 5 open questions tagged for plan-phase research
✅ Scouting findings documented, no codebase surprises remaining
✅ Test strategy defined (Phase 53 self-test, no e2e coupling)

**Next:** `/gsd-plan-phase 53` 进入 plan 阶段，把 5 个 open questions 通过 research 闭环 + 产出 plan 文件。
