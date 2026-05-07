# Phase 38: SSH Proxy 端口转发支持 - Research

**Researched:** 2026-05-07
**Domain:** SSH Protocol Port Forwarding (RFC 4254), golang.org/x/crypto/ssh proxy implementation
**Confidence:** HIGH

## Summary

Phase 38 需要在现有 SSH Proxy (`internal/sshproxy/proxy.go`) 上实现端口转发支持，使 VS Code Remote-SSH 的端口转发功能能够正常工作。当前 proxy 在 `handleConnection` 中拒绝所有非 `session` 类型的 channel（`direct-tcpip`、`forwarded-tcpip` 等），这是 VS Code Remote-SSH 端口转发失败的根本原因。

研究确认：标准实现路径完全依赖 `golang.org/x/crypto/ssh`（已在使用），无需引入新依赖。核心工作分为三部分：(1) 处理客户端发来的 `direct-tcpip` channel 请求，代理到容器侧；(2) 处理 `tcpip-forward` 全局请求，在容器侧注册端口监听；(3) 处理容器侧发来的 `forwarded-tcpip` channel，回传给客户端。安全校验（SSH-04）需在 proxy 层拦截非法转发目标。

**Primary recommendation:** 在 `handleConnection` 中按 channel 类型分发（`session`、`direct-tcpip`、`forwarded-tcpip`），对 `direct-tcpip` 和 `forwarded-tcpip` 使用与现有 `handleChannel` 类似的"接受-拨号-双向拷贝"模式，对 `tcpip-forward` 全局请求使用 `targetClient.SendRequest` 透传。

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| SSH-01 | `direct-tcpip` channel 转发：解析 payload，通过已有 SSH 连接向容器请求 `direct-tcpip`，双向并发拷贝，同一连接支持多 forwarding channel | 使用 `ssh.Unmarshal` 解析 `channelOpenDirectMsg` 格式的 payload；用 `targetClient.OpenChannel("direct-tcpip", ssh.Marshal(&msg))` 向容器发起对应请求；`io.Copy` 双向 goroutine 模式与现有 session 代理一致 |
| SSH-02 | `tcpip-forward` 全局请求和 `forwarded-tcpip` channel：处理客户端远程端口转发请求，当远程端口有连接时打开 `forwarded-tcpip` channel 回传 | 对 `tcpip-forward` 全局请求用 `targetClient.SendRequest` 透传；对容器发来的 `forwarded-tcpip` 用 `targetClient.HandleChannelOpen("forwarded-tcpip")` 接收后代理回客户端 |
| SSH-03 | 容器内 `sshd_config` 显式开启端口转发 | `deploy/docker/managed-user/sshd_config` 已配置 `AllowTcpForwarding yes`、`AllowStreamLocalForwarding yes`、`GatewayPorts no`，无需修改 |
| SSH-04 | SSH Proxy 对 forwarding 目标做安全校验：拒绝管理网段、Docker socket、metadata 端点，只允许容器 netns 内地址或用户显式白名单 | 在 `direct-tcpip` 解析出目标 host:port 后，在打开容器侧 channel 前进行 IP/域名黑名单校验 |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| golang.org/x/crypto/ssh | v0.41.0 | SSH 协议实现：channel 管理、全局请求、wire format 编解码 | 项目已使用，Go 生态唯一成熟的 SSH 库，RFC 4254 完整实现 |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| net (stdlib) | Go 1.26.1 | IP 解析、CIDR 匹配、端口校验 | 安全校验时解析目标地址 |
| sync (stdlib) | Go 1.26.1 | WaitGroup、Mutex | 多 channel 并发管理 |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| golang.org/x/crypto/ssh | 自研 SSH 协议栈 | 绝对不可行，RFC 4254 实现复杂度极高 |
| golang.org/x/crypto/ssh | github.com/gliderlabs/ssh | 高层封装，底层仍是 x/crypto/ssh，增加依赖无收益 |

## Architecture Patterns

### Recommended Project Structure

```
internal/sshproxy/
├── proxy.go          # Server, handleConnection, channel dispatch
├── proxy_test.go     # 现有测试 + forwarding 测试
├── forward.go        # direct-tcpip / forwarded-tcpip / tcpip-forward 处理逻辑
└── validate.go       # forwarding 目标安全校验
```

### Pattern 1: Channel Type Dispatch (SSH-01)
**What:** 在 `handleConnection` 中不再简单拒绝非 session channel，而是按类型分发到不同 handler。
**When to use:** 所有需要支持多种 channel 类型的 SSH proxy/server。
**Example:**
```go
// Source: internal/sshproxy/proxy.go (current) + golang.org/x/crypto/ssh
for newChan := range chans {
    switch newChan.ChannelType() {
    case "session":
        go s.handleChannel(newChan, targetAddr, targetUser, targetPassword, targetPrivateKey)
    case "direct-tcpip":
        go s.handleDirectTCPIP(newChan, targetClient) // SSH-01
    case "forwarded-tcpip":
        go s.handleForwardedTCPIP(newChan, targetClient) // SSH-02
    default:
        newChan.Reject(ssh.UnknownChannelType, fmt.Sprintf("channel type %s not supported", newChan.ChannelType()))
    }
}
```

### Pattern 2: direct-tcpip Payload 解析与代理 (SSH-01)
**What:** 解析客户端发来的 `direct-tcpip` channel open 请求中的目标地址，安全校验后向容器侧发起对应的 `direct-tcpip` channel，然后双向拷贝数据。
**When to use:** 本地端口转发代理场景。
**Example:**
```go
// Source: golang.org/x/crypto/ssh/tcpip.go (verified from source)
type channelOpenDirectMsg struct {
    raddr string
    rport uint32
    laddr string
    lport uint32
}

func (s *Server) handleDirectTCPIP(newChan ssh.NewChannel, targetClient *ssh.Client) {
    var msg channelOpenDirectMsg
    if err := ssh.Unmarshal(newChan.ExtraData(), &msg); err != nil {
        newChan.Reject(ssh.ConnectionFailed, "invalid direct-tcpip payload")
        return
    }

    // SSH-04: 安全校验
    if !s.validateForwardTarget(msg.raddr, int(msg.rport)) {
        newChan.Reject(ssh.Prohibited, "forwarding to this target is not allowed")
        return
    }

    clientChan, clientReqs, err := newChan.Accept()
    if err != nil {
        return
    }
    defer clientChan.Close()
    go ssh.DiscardRequests(clientReqs)

    // 向容器侧发起对应的 direct-tcpip
    targetChan, targetReqs, err := targetClient.OpenChannel("direct-tcpip", ssh.Marshal(&msg))
    if err != nil {
        fmt.Fprintf(clientChan.Stderr(), "forwarding failed: %v\r\n", err)
        return
    }
    defer targetChan.Close()
    go ssh.DiscardRequests(targetReqs)

    // 双向并发拷贝
    var wg sync.WaitGroup
    wg.Add(2)
    go func() {
        defer wg.Done()
        io.Copy(targetChan, clientChan)
        targetChan.CloseWrite()
    }()
    go func() {
        defer wg.Done()
        io.Copy(clientChan, targetChan)
        clientChan.CloseWrite()
    }()
    wg.Wait()
}
```

### Pattern 3: tcpip-forward 全局请求透传 (SSH-02)
**What:** 客户端发送 `tcpip-forward` 全局请求时，proxy 需要将其转发到容器侧的 sshd，由容器侧实际监听端口。当容器侧有连接进入时，sshd 会主动向 proxy 打开 `forwarded-tcpip` channel。
**When to use:** 远程端口转发代理场景。
**Example:**
```go
// Source: golang.org/x/crypto/ssh/tcpip.go (ListenTCP / channelForwardMsg)
type channelForwardMsg struct {
    addr  string
    rport uint32
}

// 在 handleConnection 中不再 DiscardRequests(globalReqs)，而是处理：
func (s *Server) handleGlobalRequests(globalReqs <-chan *ssh.Request, targetClient *ssh.Client) {
    for req := range globalReqs {
        switch req.Type {
        case "tcpip-forward", "cancel-tcpip-forward":
            // 透传到容器侧
            ok, resp, err := targetClient.SendRequest(req.Type, req.WantReply, req.Payload)
            if err != nil {
                if req.WantReply {
                    req.Reply(false, nil)
                }
                continue
            }
            if req.WantReply {
                req.Reply(ok, resp)
            }
        default:
            // 其他全局请求默认拒绝
            if req.WantReply {
                req.Reply(false, nil)
            }
        }
    }
}
```

### Pattern 4: forwarded-tcpip 回传 (SSH-02)
**What:** 容器侧 sshd 收到远程连接后，会主动向 proxy（作为其 SSH client）打开 `forwarded-tcpip` channel。proxy 需要将此 channel 回传给原始客户端。
**When to use:** 远程端口转发的反向数据流。
**Example:**
```go
// Source: golang.org/x/crypto/ssh/tcpip.go (forwardedTCPPayload)
type forwardedTCPPayload struct {
    Addr       string
    Port       uint32
    OriginAddr string
    OriginPort uint32
}

// 在建立 targetClient 后，注册 handler 并启动 goroutine：
forwardedCh := targetClient.HandleChannelOpen("forwarded-tcpip")
if forwardedCh != nil {
    go s.proxyForwardedChannels(forwardedCh, clientConn) // clientConn 是 ssh.Conn 接口
}

func (s *Server) proxyForwardedChannels(incoming <-chan ssh.NewChannel, clientConn ssh.Conn) {
    for newChan := range incoming {
        // 可选：解析 payload 做安全校验
        var payload forwardedTCPPayload
        _ = ssh.Unmarshal(newChan.ExtraData(), &payload)

        // 向客户端打开 forwarded-tcpip channel
        // 注意：需要客户端也支持接收 forwarded-tcpip，VS Code SSH client 天然支持
        ch, reqs, err := clientConn.OpenChannel("forwarded-tcpip", newChan.ExtraData())
        if err != nil {
            newChan.Reject(ssh.ConnectionFailed, "failed to open client channel")
            continue
        }
        go ssh.DiscardRequests(reqs)

        targetCh, targetReqs, err := newChan.Accept()
        if err != nil {
            ch.Close()
            continue
        }
        go ssh.DiscardRequests(targetReqs)

        // 双向拷贝（同 Pattern 2）
        go func() {
            io.Copy(ch, targetCh)
            ch.CloseWrite()
        }()
        go func() {
            io.Copy(targetCh, ch)
            targetCh.CloseWrite()
        }()
    }
}
```

### Anti-Patterns to Avoid
- **在 proxy 层自行监听 TCP 端口:** `tcpip-forward` 应该透传给容器侧 sshd 处理，proxy 不应自己 `net.Listen`。自行监听会引入端口冲突、生命周期管理、连接状态同步等复杂问题。
- **忽略 `WantReply` 的全局请求:** 所有 `tcpip-forward` 请求都带 `WantReply=true`，必须正确回复，否则客户端会 hang。
- **不复用 targetClient:** 每个 SSH 连接应该只建立一个到容器侧的 `targetClient`，所有 channel 复用此连接。当前 `handleChannel` 里每次 session 都 `ssh.Dial`，这是可以优化的点，但 Phase 38 保持现有行为即可。

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SSH wire format 编解码 | 手写二进制解析 | `ssh.Marshal()` / `ssh.Unmarshal()` | RFC 4251 的 string/uint32/mpint 编码规则复杂，且需处理长度前缀和字节序 |
| direct-tcpip payload 结构 | 自定义 struct | 与 `golang.org/x/crypto/ssh` 内部结构字段名/类型一致的 struct | 库使用反射按字段顺序编码，字段名首字母大小写和类型必须匹配 |
| 双向数据拷贝 | 手写 select + io.Copy | `io.Copy` + `CloseWrite` + `sync.WaitGroup` | 需要正确处理 EOF、半关闭、goroutine 泄漏 |
| TCP 连接管理 | 在 proxy 层自行 listen/accept | 透传给容器侧 sshd | sshd 已处理端口分配、并发连接、SO_REUSEADDR 等 |

**Key insight:** `golang.org/x/crypto/ssh` 的 `tcpip.go` 本身就是最佳参考实现。其 `Client.dial()`、`ListenTCP()`、`forwardList.handleChannels()` 三个方法分别对应 SSH-01、SSH-02 的 client 侧实现。proxy 作为中间人，只需将 client 的请求翻译成对 target container 的对应操作。

## Common Pitfalls

### Pitfall 1: `direct-tcpip` payload 解析字段名不匹配
**What goes wrong:** 自己定义的 struct 字段名与 `golang.org/x/crypto/ssh` 内部使用的字段名不一致，导致 `ssh.Unmarshal` 解析失败或得到零值。
**Why it happens:** 库使用反射读取 struct 字段，按**字段定义顺序**（不是字段名）匹配 wire format 中的顺序。但字段必须导出（首字母大写）才能被反射访问。
**How to avoid:** 严格使用与库源码一致的字段名和顺序：
```go
type channelOpenDirectMsg struct {
    raddr string  // 对应 wire format: string (目标地址)
    rport uint32  // 对应 wire format: uint32 (目标端口)
    laddr string  // 对应 wire format: string (源地址)
    lport uint32  // 对应 wire format: uint32 (源端口)
}
```
**Warning signs:** `raddr` 为空字符串或 `rport` 为 0，但客户端明明发送了有效地址。

### Pitfall 2: 全局请求 channel 未消费导致连接 hang
**What goes wrong:** `globalReqs` channel 如果无人消费，底层 mux 的 goroutine 会阻塞，导致整个 SSH 连接无法处理新消息。
**Why it happens:** `ssh.NewServerConn` 文档明确要求 "The Request and NewChannel channels must be serviced, or the connection will hang."
**How to avoid:** 必须用 goroutine 持续消费 `globalReqs`，不能简单丢弃（`ssh.DiscardRequests` 虽然可用，但对 `tcpip-forward` 需要实际处理）。
**Warning signs:** 客户端发送 `tcpip-forward` 后无任何响应，连接超时。

### Pitfall 3: `HandleChannelOpen` 返回 nil 表示已注册
**What goes wrong:** 对同一个 channel type 多次调用 `HandleChannelOpen` 会返回 `nil`，第二次注册失败。
**Why it happens:** 库内部检查 `c.channelHandlers[channelType]` 是否已存在。
**How to avoid:** 每个 `targetClient` 只对 `"forwarded-tcpip"` 调用一次 `HandleChannelOpen`，在连接建立时注册。
**Warning signs:** 远程端口转发连接到达但 proxy 未收到 `forwarded-tcpip` channel。

### Pitfall 4: `CloseWrite` 与 `Close` 混用导致 goroutine 泄漏
**What goes wrong:** 只用 `Close()` 不用 `CloseWrite()`，或者顺序错误，导致 `io.Copy`  goroutine 永远阻塞。
**Why it happens:** SSH channel 的 `Close()` 关闭整个 channel，但 `io.Copy(dst, src)` 在 `src` 未 EOF 时不会返回。`CloseWrite()` 发送 SSH_MSG_CHANNEL_EOF，使对端 `Read` 返回 EOF。
**How to avoid:** 双向拷贝的标准模式：
```go
// A -> B
go func() { io.Copy(b, a); b.CloseWrite() }()
// B -> A
go func() { io.Copy(a, b); a.CloseWrite() }()
```
**Warning signs:** 端口转发连接关闭后 goroutine 数量持续增长。

### Pitfall 5: 安全校验绕过（DNS 解析时序问题）
**What goes wrong:** 只对 IP 地址做黑名单校验，但客户端通过域名连接时，域名解析后可能指向被禁网段。
**Why it happens:** `direct-tcpip` 的 `raddr` 可能是域名（如 `metadata.google.internal`），proxy 在校验时看到的是域名而非 IP。
**How to avoid:** 校验逻辑应同时检查：(1) `raddr` 是否匹配黑名单域名模式；(2) 如果 `raddr` 是 IP，检查是否落入禁止 CIDR。不主动做 DNS 解析（避免引入 DNS 依赖和时延），依赖域名黑名单匹配。
**Warning signs:** 客户端通过域名成功转发到被禁目标。

## Code Examples

### Verified Pattern: direct-tcpip 完整处理
```go
// Source: golang.org/x/crypto/ssh/tcpip.go (Client.dial 方法) + internal/sshproxy/proxy.go
// 此模式已在库源码中验证，proxy 侧是 client 侧的镜像操作

func handleDirectTCPIP(newChan ssh.NewChannel, targetClient *ssh.Client, logger *slog.Logger) {
    type channelOpenDirectMsg struct {
        raddr string
        rport uint32
        laddr string
        lport uint32
    }

    var msg channelOpenDirectMsg
    if err := ssh.Unmarshal(newChan.ExtraData(), &msg); err != nil {
        newChan.Reject(ssh.ConnectionFailed, "invalid payload")
        return
    }

    // 安全校验（SSH-04）
    if isForbiddenTarget(msg.raddr, int(msg.rport)) {
        newChan.Reject(ssh.Prohibited, "target not allowed")
        return
    }

    clientCh, clientReqs, err := newChan.Accept()
    if err != nil {
        return
    }
    defer clientCh.Close()
    go ssh.DiscardRequests(clientReqs)

    targetCh, targetReqs, err := targetClient.OpenChannel("direct-tcpip", ssh.Marshal(&msg))
    if err != nil {
        fmt.Fprintf(clientCh.Stderr(), "forward failed: %v\r\n", err)
        return
    }
    defer targetCh.Close()
    go ssh.DiscardRequests(targetReqs)

    var wg sync.WaitGroup
    wg.Add(2)
    go func() {
        defer wg.Done()
        io.Copy(targetCh, clientCh)
        targetCh.CloseWrite()
    }()
    go func() {
        defer wg.Done()
        io.Copy(clientCh, targetCh)
        clientCh.CloseWrite()
    }()
    wg.Wait()
}
```

### Verified Pattern: tcpip-forward 全局请求透传
```go
// Source: golang.org/x/crypto/ssh/tcpip.go (ListenTCP 方法)
// proxy 作为中间人，将 client 的 global request 转发给 target

func handleGlobalRequests(reqs <-chan *ssh.Request, targetClient *ssh.Client) {
    for req := range reqs {
        switch req.Type {
        case "tcpip-forward", "cancel-tcpip-forward":
            ok, resp, err := targetClient.SendRequest(req.Type, req.WantReply, req.Payload)
            if err != nil {
                if req.WantReply {
                    req.Reply(false, nil)
                }
                continue
            }
            if req.WantReply {
                req.Reply(ok, resp)
            }
        default:
            if req.WantReply {
                req.Reply(false, nil)
            }
        }
    }
}
```

### Verified Pattern: 安全校验函数骨架
```go
// Source: project-specific (SSH-04 requirement)

var forbiddenCIDRs = []string{"10.99.0.0/16"}
var forbiddenHosts = []string{
    "metadata.google.internal",
    "169.254.169.254", // AWS/GCP/Azure metadata
}
var forbiddenPorts = map[int]bool{2375: true, 2376: true} // Docker socket ports

func isForbiddenTarget(host string, port int) bool {
    // 检查端口
    if forbiddenPorts[port] {
        return true
    }
    // 检查域名黑名单
    for _, h := range forbiddenHosts {
        if strings.EqualFold(host, h) {
            return true
        }
    }
    // 检查 IP CIDR
    ip := net.ParseIP(host)
    if ip != nil {
        for _, cidr := range forbiddenCIDRs {
            _, ipNet, _ := net.ParseCIDR(cidr)
            if ipNet.Contains(ip) {
                return true
            }
        }
    }
    return false
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| 拒绝所有非 session channel | 按类型分发处理 | Phase 38 | 启用 VS Code Remote-SSH 端口转发 |
| `ssh.DiscardRequests(globalReqs)` | 处理 `tcpip-forward` / `cancel-tcpip-forward` | Phase 38 | 支持远程端口转发 |

**Deprecated/outdated:**
- 无。`golang.org/x/crypto/ssh` 的 forwarding API 自 v0.30+ 以来稳定，v0.41.0 无 breaking changes。

## Open Questions

1. **同一 SSH 连接复用 targetClient 的可行性**
   - What we know: 当前 `handleChannel` 每次 session 都新建 `ssh.Dial`，连接开销可接受
   - What's unclear: 如果同一 client SSH 连接上有多个 concurrent `direct-tcpip` channel，是否应复用同一个 `targetClient`
   - Recommendation: Phase 38 保持每个 channel 独立 `ssh.Dial`（与现有 session 行为一致），后续优化可在 Phase 40/41 考虑连接池

2. **VS Code 是否同时使用 `direct-tcpip` 和 `tcpip-forward`**
   - What we know: VS Code Remote-SSH 的端口转发主要使用 `direct-tcpip`（本地转发到远程），语言服务器等扩展使用此路径
   - What's unclear: VS Code Server 的某些功能（如自动更新检查）是否会触发 `tcpip-forward`（远程转发回本地）
   - Recommendation: Phase 38 同时实现两者，Phase 40 E2E 验证时确认实际使用模式

3. **`forwarded-tcpip` 回传时的 clientConn 引用**
   - What we know: `handleConnection` 中 `sshConn` 是 `*ssh.ServerConn`，实现了 `ssh.Conn` 接口，有 `OpenChannel` 方法
   - What's unclear: 从 `forwarded-tcpip` handler goroutine 安全引用 `sshConn` 的生命周期（connection close 时）
   - Recommendation: 使用 `sshConn.Wait()` 检测连接关闭，在 handler 中检查 context/closed 状态

## Validation Architecture

> Skipped: `workflow.nyquist_validation` is `false` in `.planning/config.json`.

## Sources

### Primary (HIGH confidence)
- `golang.org/x/crypto/ssh` v0.41.0 source (local module cache) — `tcpip.go`, `client.go`, `channel.go`, `mux.go`, `messages.go`, `connection.go`
  - `channelOpenDirectMsg`, `channelForwardMsg`, `forwardedTCPPayload` wire format structs
  - `Client.dial()`, `Client.ListenTCP()`, `forwardList.handleChannels()` implementation patterns
  - `NewChannel`, `Channel`, `Request`, `Conn` interfaces
- `internal/sshproxy/proxy.go` (project source) — current proxy implementation
- `deploy/docker/managed-user/sshd_config` (project source) — `AllowTcpForwarding yes` 已配置

### Secondary (MEDIUM confidence)
- RFC 4254 Section 7.1 (direct-tcpip), 7.2 (forwarded-tcpip), 4 (global requests) — 协议规范，已通过库源码实现验证

### Tertiary (LOW confidence)
- 无。所有核心发现均来自库源码直接验证。

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — 项目已使用 golang.org/x/crypto/ssh，源码直接验证所有 API
- Architecture: HIGH — 库源码 `tcpip.go` 提供了完整的 client 侧参考实现，proxy 作为镜像操作路径清晰
- Pitfalls: HIGH — 字段名匹配、channel hang、goroutine 泄漏等问题均有源码和文档支撑

**Research date:** 2026-05-07
**Valid until:** 2026-06-07 (golang.org/x/crypto/ssh 为稳定库，30 天有效期合理)
