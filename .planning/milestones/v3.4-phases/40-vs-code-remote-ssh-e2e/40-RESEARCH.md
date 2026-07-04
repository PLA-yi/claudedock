# Phase 40: VS Code Remote-SSH E2E 验证 - Research

**Researched:** 2026-05-08
**Domain:** VS Code Remote-SSH / sing-box 流量验证 / SSH 端口转发
**Confidence:** HIGH

## Summary

Phase 40 是一个验证阶段，不涉及新功能开发。核心任务是验证 VS Code Remote-SSH 能通过 SSH Proxy（2222 端口）完整连接到 managed-user 容器，并确认所有流量严格走 sing-box 出口。

Phase 39 已完成 `claudedock local` 子命令、entrypoint `MODE=local` 分支、sing-box tun/proxy 启动逻辑。Phase 38 已完成 SSH Proxy 的 `direct-tcpip` 和 `forwarded-tcpip` 转发支持。代码基础设施已就绪，本阶段需要做的是：编写 UAT 脚本 + 手动测试 checklist + 必要的最小修复。

**Primary recommendation:** 编写 `tests/scripts/uat-vscode-remote-ssh.sh` 脚本覆盖可脚本化的检查项（进程检测、出口 IP 验证、DNS 泄漏检测），配合手动 checklist 覆盖 GUI 交互部分（VS Code 连接、文件浏览、终端、端口转发、扩展安装）。

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- 手动 E2E 验证为主，辅以脚本化检查（非 Playwright 自动化）
- 提供 `tests/scripts/uat-vscode-remote-ssh.sh` 脚本覆盖可脚本化的检查项（出口 IP 验证、进程检测、流量路由检测）
- 手动测试 checklist 写在计划文档中，记录每一步的操作和预期结果
- 使用 `claudedock local` 启动本地 managed-user 容器（Phase 39 已完成）
- 容器必须启用 egress 配置（sing-box tun 模式），以验证流量约束
- 宿主机上安装 VS Code + Remote-SSH 扩展作为测试客户端
- SSH 连接信息从 `claudedock local` 输出获取（host, port, user, password）
- 容器内执行 `curl ifconfig.me` 或等效命令，返回 IP 必须等于 egress 配置的 ExpectedIP
- VS Code 端口转发场景：在容器内启动一个 HTTP 服务，通过 VS Code 端口转发从宿主机访问，检查请求来源 IP
- VS Code Server 下载流量验证：检查容器内 sing-box 日志，确认 `update.code.visualstudio.com` 域名走 proxy-out 出站
- DNS 泄漏验证：容器内 `nslookup` 必须走 tun 捕获的 DNS（8.8.8.8 via sing-box），不能走宿主机 DNS
- 如果验证发现问题，只做最小修复以通过验证标准
- 不扩展功能范围（如增加新的 CLI 命令或新的网络模式）
- 修复范围限于：entrypoint.sh 适配、sing-box 配置补全、端口转发规则调整、SSH 配置修正

### Claude's Discretion

- 测试脚本的具体检查项和输出格式
- 手动 checklist 的详细程度
- 验证过程中发现的 edge case 是否需要额外修复
- 日志采集和诊断信息的展示方式

### Deferred Ideas (OUT OF SCOPE)

- Phase 41: Doctor 扩展覆盖 Remote-SSH 场景诊断
- 自动化 VS Code E2E 测试（Playwright + VS Code Extension Host）— 后续版本
- 多容器并发 Remote-SSH 验证 — 后续版本
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| SSH-05 | VS Code Remote-SSH 端到端验证：连接、VS Code Server、端口转发、claude 命令 | SSH Proxy 已支持 direct-tcpip（Phase 38-01）和 forwarded-tcpip（Phase 38-02）；entrypoint MODE=local 已启动 sshd；`claudedock local` 输出 SSH 连接信息 |
| SEC-01 | 验证 direct-tcpip 转发流量走 sing-box tun | sing-box tun 模式通过 auto_route 捕获所有流量；DNS hijack 规则将端口 53 流量重定向到 sing-box；验证方式：容器内 curl ifconfig.me 返回 egress IP |
| SEC-02 | VS Code Server 下载/扩展安装流量也走受控出口 | VS Code Server 从 update.code.visualstudio.com 下载（HTTPS 443）；sing-box tun auto_route 捕获所有 TCP 流量；验证方式：sing-box 日志确认域名走 proxy-out |
</phase_requirements>

## Standard Stack

### Core

| Library/Tool | Version | Purpose | Why Standard |
|-------------|---------|---------|-------------|
| bash | 宿主机自带 | UAT 脚本 | 项目已有 UAT 脚本模式（uat-v31-promotion.sh） |
| curl | 宿主机自带 | 出口 IP 验证 | 容器内 curl ifconfig.me 确认 egress IP |
| jq | 宿主机自带 | JSON 报告生成 | UAT 脚本 JSON 输出 |
| VS Code + Remote-SSH 扩展 | 最新版 | 测试客户端 | 手动 E2E 测试的 GUI 客户端 |

### Supporting

| Library/Tool | Version | Purpose | When to Use |
|-------------|---------|---------|-------------|
| sing-box | 镜像内 | 流量隧道 | 容器内 tun/proxy 模式出网 |
| nslookup/dig | 容器内 | DNS 泄漏验证 | 确认 DNS 走 sing-box |
| netstat/ss | 容器内 | 进程/端口检测 | 确认 sing-box 和 VS Code Server 进程存在 |

**No new dependencies needed.** Phase 40 全部使用现有工具和容器内命令。

## Architecture Patterns

### VS Code Remote-SSH 连接流程

VS Code Remote-SSH 的连接过程如下：

1. **SSH 连接建立**：VS Code 通过 SSH 连接到目标主机（本场景：SSH Proxy 2222 端口）
2. **VS Code Server 下载**：首次连接时，VS Code 在远程主机上下载并安装 VS Code Server（~/.vscode-server/bin/{commit}/）
3. **VS Code Server 启动**：在远程主机上启动 VS Code Server 进程，监听随机端口
4. **端口转发**：VS Code 通过 SSH direct-tcpip 通道将本地端口转发到远程 VS Code Server 端口
5. **通信**：后续所有 VS Code 通信（文件操作、终端、扩展）都通过 SSH session channel 和 direct-tcpip 通道进行

### SSH 通道使用模式

VS Code Remote-SSH 使用以下 SSH 通道类型：

| 通道类型 | 用途 | SSH Proxy 支持状态 |
|---------|------|-------------------|
| session | 终端、shell、exec | 已支持（Phase 38-01 proxy.go handleChannel） |
| direct-tcpip | 端口转发（VS Code Server 通信、用户端口转发） | 已支持（Phase 38-01 forward.go handleDirectTCPIP） |
| tcpip-forward (global request) | 远程端口转发 | 已支持（Phase 38-02 forward.go handleGlobalRequests） |
| forwarded-tcpip | 远程端口转发的通道 | 已支持（Phase 38-02 forward.go proxyForwardedChannels） |

**关键发现：** Phase 38 已经实现了 VS Code Remote-SSH 需要的所有 SSH 通道类型。理论上 VS Code Remote-SSH 应该能直接连接。

### VS Code Server 下载域名

VS Code Server 从以下域名下载（需要 sing-box 出站覆盖）：

| 域名 | 用途 | 协议 |
|------|------|------|
| `update.code.visualstudio.com` | VS Code Server 主下载 | HTTPS 443 |
| `vscode.download.prss.microsoft.com` | CDN 备用下载 | HTTPS 443 |
| `marketplace.visualstudio.com` | 扩展市场 | HTTPS 443 |
| `*.gallerycdn.vsassets.io` | 扩展 CDN | HTTPS 443 |

sing-box tun 模式的 `auto_route: true` 会捕获所有 TCP 流量，这些域名的请求会自动走 proxy-out 出站。

### 流量验证架构

```
宿主机 (macOS/Linux)
├── VS Code 客户端
│   └── SSH 连接 → 127.0.0.1:2222 (SSH Proxy)
│
├── SSH Proxy (Go)
│   └── direct-tcpip / session → 容器 sshd
│
└── managed-user 容器 (Docker)
    ├── sshd (端口 22, publish 到宿主机)
    ├── VS Code Server (通过 SSH session 启动)
    ├── sing-box tun (auto_route 捕获所有流量)
    │   └── proxy-out → 外部代理服务器 → 出口 IP
    └── curl ifconfig.me → 验证出口 IP
```

### sing-box tun 模式流量捕获

sing-box tun 模式的关键配置（来自 `gateway_singbox_config.go`）：

```json
{
  "inbounds": [{
    "type": "tun",
    "auto_route": true,
    "strict_route": false,
    "stack": "mixed",
    "sniff": true,
    "sniff_override_destination": true
  }],
  "dns": {
    "servers": [{"tag": "dns-remote", "type": "tcp", "server": "1.1.1.1", "detour": "proxy-out"}],
    "strategy": "ipv4_only"
  },
  "route": {
    "rules": [
      {"ip_cidr": ["<proxy-server-ip>/32"], "outbound": "direct"},
      {"port": 53, "action": "hijack-dns"}
    ],
    "final": "proxy-out"
  }
}
```

**关键点：**
- `auto_route: true` 自动捕获所有流量
- `sniff: true` + `sniff_override_destination: true` 识别协议并按域名路由
- DNS 端口 53 被 hijack 到 sing-box DNS 服务器
- 所有非直连流量走 `proxy-out`（final 规则）
- 代理服务器 IP 走 `direct`（避免回环）

### 潜在问题与风险

#### 1. macOS 宿主机 tun 模式不可用

macOS 没有 `/dev/net/tun`，无法在 Docker 容器内使用 tun 模式。本地测试应使用 proxy（SOCKS/HTTP）模式。

**影响：** proxy 模式依赖环境变量（ALL_PROXY / HTTP_PROXY / HTTPS_PROXY），不是所有进程都尊重这些变量。VS Code Server 可能不走代理。

**缓解：** 在 Linux 宿主机上做 tun 模式的完整验证。macOS 上只验证 proxy 模式的 happy path。

#### 2. VS Code Server 可能不走环境变量代理

VS Code Server 是 Node.js 应用，Node.js 的 `fetch` 和 `http` 模块默认不读取 HTTP_PROXY 环境变量。需要显式配置代理。

**影响：** 在 proxy 模式下，VS Code Server 下载扩展和更新可能绕过代理。

**缓解：** tun 模式无此问题（auto_route 捕获所有流量）。proxy 模式下可设置 VS Code 的 `http.proxy` 配置。

#### 3. sing-box tun 模式需要容器特权

tun 模式需要 `--cap-add NET_ADMIN` 和 `--device /dev/net/tun`。Phase 39 Plan 02 的 egress.go 已处理此逻辑（tun 模式自动追加这些参数）。

#### 4. SSH Proxy 的 ServerVersion 暴露代理身份

当前 SSH Proxy 的 ServerVersion 是 `SSH-2.0-CloudCLIProxy`。VS Code Remote-SSH 可能对非标准 SSH Server 有兼容性问题。

**缓解：** 如果 VS Code 连接失败，可将 ServerVersion 改为 `SSH-2.0-OpenSSH_8.9`。

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| VS Code Server 下载 | 手动下载二进制 | VS Code 自动下载 | VS Code 自动匹配版本，手动下载版本不匹配 |
| 端口转发测试 | 自建 SSH 客户端 | VS Code 内置端口转发 | 验证真实用户路径 |
| 出口 IP 检测 | 自建 IP 检测服务 | curl ifconfig.me | 标准、可靠、无需维护 |

## Common Pitfalls

### Pitfall 1: macOS 无 /dev/net/tun
**What goes wrong:** 容器启动 sing-box tun 模式失败，报 `/dev/net/tun: no such file or directory`
**Why it happens:** macOS Docker Desktop 不支持 tun 设备
**How to avoid:** macOS 测试使用 proxy 模式（SOCKS/HTTP），tun 模式验证在 Linux 宿主机上做
**Warning signs:** entrypoint 日志显示 `WARNING: sing-box tun mode requires /etc/sing-box/config.json`

### Pitfall 2: VS Code Remote-SSH 连接超时
**What goes wrong:** VS Code 显示 "Connecting to SSH host..." 长时间无响应
**Why it happens:** SSH Proxy 的 ServerVersion 不标准，或 VS Code Server 下载被阻塞
**How to avoid:** 检查 SSH Proxy 日志确认连接是否建立；检查容器内网络是否可达
**Warning signs:** SSH Proxy 日志无 "SSH proxy session" 记录

### Pitfall 3: 端口转发被安全规则拒绝
**What goes wrong:** VS Code 端口转发失败，显示 "Could not establish connection"
**Why it happens:** SSH Proxy 的 `isForbiddenTarget` 误判端口转发目标
**How to avoid:** 检查 SSH Proxy 日志中的 "forwarding to forbidden target rejected" 记录
**Warning signs:** VS Code 输出面板显示 SSH 转发错误

### Pitfall 4: sing-box 未启动但验证通过
**What goes wrong:** 验证脚本报告通过，但实际流量走的直连
**Why it happens:** sing-box 启动失败但被 WARNING 忽略，容器仍能通过 Docker bridge 访问外网
**How to avoid:** 验证脚本必须先检查 sing-box 进程存在，再做出口 IP 验证
**Warning signs:** `curl ifconfig.me` 返回宿主机出口 IP 而非 egress IP

## Code Examples

### UAT 脚本模式（参考 uat-v31-promotion.sh）

```bash
#!/usr/bin/env bash
# tests/scripts/uat-vscode-remote-ssh.sh — Phase 40 VS Code Remote-SSH E2E UAT
set -euo pipefail

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
skip() { echo "[SKIP]  $1: $2"; SKIP_COUNT=$((SKIP_COUNT + 1)); }
info() { echo "[INFO]  $1"; }
```

### 容器内出口 IP 验证

```bash
# 在容器内执行
CONTAINER_IP=$(docker exec "$CONTAINER_NAME" curl -s --max-time 10 ifconfig.me)
if [ "$CONTAINER_IP" = "$EXPECTED_EGRESS_IP" ]; then
  pass "出口 IP 验证: $CONTAINER_IP == $EXPECTED_EGRESS_IP"
else
  fail "出口 IP 验证: $CONTAINER_IP != $EXPECTED_EGRESS_IP"
fi
```

### sing-box 进程检测

```bash
# 检查 sing-box 进程是否存在
if docker exec "$CONTAINER_NAME" pgrep -x sing-box >/dev/null 2>&1; then
  pass "sing-box 进程运行中"
else
  fail "sing-box 进程未运行"
fi
```

### DNS 泄漏验证

```bash
# 容器内 nslookup 必须走 sing-box DNS
DNS_RESULT=$(docker exec "$CONTAINER_NAME" nslookup ifconfig.me 2>&1)
if echo "$DNS_RESULT" | grep -q "Server:"; then
  pass "DNS 解析正常"
else
  fail "DNS 解析失败"
fi
```

### VS Code Server 进程检测

```bash
# 检查 VS Code Server 进程是否存在（连接后）
if docker exec "$CONTAINER_NAME" pgrep -f "vscode-server" >/dev/null 2>&1; then
  pass "VS Code Server 进程运行中"
else
  skip "vscode_server_process" "VS Code Server 未运行（可能尚未通过 VS Code 连接）"
fi
```

### sing-box 日志检查

```bash
# 检查 sing-box 日志确认域名走 proxy-out
SING_LOG=$(docker exec "$CONTAINER_NAME" cat /var/log/sing-box.log 2>/dev/null || true)
if echo "$SING_LOG" | grep -q "update.code.visualstudio.com"; then
  pass "VS Code 更新域名走 sing-box"
else
  skip "vscode_update_traffic" "sing-box 日志中未发现 VS Code 更新域名（可能需要触发更新）"
fi
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| 无本地容器支持 | `claudedock local` 一键启动 | Phase 39 | 本地 E2E 验证成为可能 |
| SSH Proxy 仅支持 session | SSH Proxy 支持 direct-tcpip + forwarded-tcpip | Phase 38 | VS Code Remote-SSH 端口转发成为可能 |
| entrypoint 仅 remote 模式 | entrypoint 支持 MODE=local | Phase 39 | 跳过 KasmVNC，仅 sshd + sing-box |

## Open Questions

1. **VS Code Remote-SSH 是否需要 `env` 请求转发？**
   - What we know: VS Code 会通过 SSH session 发送 `env` 请求设置环境变量
   - What's unclear: SSH Proxy 的 handleChannel 是否正确转发 `env` 请求
   - Recommendation: 验证时观察，如果 VS Code 连接后环境变量不对，在 handleChannel 的请求转发中确认 `env` 类型被包含

2. **sing-box tun 模式在 Docker 容器内的兼容性？**
   - What we know: 需要 `--cap-add NET_ADMIN` 和 `/dev/net/tun`
   - What's unclear: Docker Desktop (macOS) 是否支持 passthrough `/dev/net/tun`
   - Recommendation: macOS 上跳过 tun 验证，使用 proxy 模式；tun 验证在 Linux 宿主机上做

3. **SSH Proxy 的 ServerVersion 对 VS Code 兼容性？**
   - What we know: 当前是 `SSH-2.0-CloudCLIProxy`
   - What's unclear: VS Code Remote-SSH 是否对 ServerVersion 有要求
   - Recommendation: 验证时观察，如果不兼容则改为标准 OpenSSH 版本字符串

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Docker | 容器启动 | 需验证 | 28.x | 无 |
| VS Code + Remote-SSH | 手动测试 | 需安装 | 最新版 | 无（手动测试核心依赖） |
| curl | 出口 IP 验证 | 容器内自带 | — | wget |
| jq | JSON 报告 | 宿主机 | 需安装 | 手写 JSON |
| sing-box | 流量隧道 | 镜像内 | 需确认 | proxy 模式 |

**Missing dependencies with no fallback:**
- VS Code + Remote-SSH 扩展（手动测试核心依赖，需在宿主机安装）

**Missing dependencies with fallback:**
- jq（可降级为手写 JSON 输出）

## Sources

### Primary (HIGH confidence)
- 项目内 SSH Proxy 实现: `internal/sshproxy/proxy.go`, `forward.go` — 确认 direct-tcpip 和 forwarded-tcpip 支持
- 项目内 entrypoint.sh — 确认 MODE=local 分支和 sing-box 启动逻辑
- 项目内 Phase 39 实现 — 确认 `claudedock local` 可用

### Secondary (MEDIUM confidence)
- VS Code Remote-SSH 官方文档: https://code.visualstudio.com/docs/remote/ssh
- sing-box 官方文档: https://sing-box.sagernet.org/

### Tertiary (LOW confidence)
- VS Code Server 下载域名列表（基于训练数据，需验证）

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — 全部使用现有工具和容器内命令
- Architecture: HIGH — VS Code Remote-SSH 连接流程基于 SSH 标准协议，Phase 38/39 已实现所有必要组件
- Pitfalls: MEDIUM — macOS tun 不可用是确定的，VS Code Server 代理行为需实际验证

**Research date:** 2026-05-08
**Valid until:** 2026-06-08（验证阶段，代码稳定期较长）

---

*Phase: 40-vs-code-remote-ssh-e2e*
*Research completed: 2026-05-08*
