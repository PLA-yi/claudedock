# Phase 32: SSH 会话可靠性 + tmux 包装 + 多端 — Research

**Researched:** 2026-04-20
**Domain:** Go SSH 客户端弱网容忍 / TCP keepalive 平台特化 / tmux 多端 attach / 容器内 flock 单例锁
**Confidence:** HIGH（核心库行为）/ MEDIUM（tmux client 识别策略，见 §5）

> 阅读顺序建议：planner 先看 §Technical Approach §5（tmux client_name 实证修订）+ §6（flock -F 语义验证）— 这两节会**修订 CONTEXT.md D-12 与 D-17 的实现细节**（决策不变，落地手段调整）。

---

## User Constraints (from CONTEXT.md)

### Locked Decisions（CONTEXT D-01..D-29）

CONTEXT.md 已对本阶段所有 gray area 锁定决策（`--auto` 模式），planner **必须**遵循以下要点（完整定义见 32-CONTEXT.md `<decisions>` 段）：

- **D-01 文件结构**：新增 `session.go` / `reconnect.go` / `input_buffer.go` / `keepalive.go` / `sync_lock.go` / `errcodes/session.go` / `cmd/cloud-claude/sessions.go`；追加 `errcodes/net.go`；改造 `ssh.go` / `mount_strategy.go` / `last_session.go` / `colors.go` / `cmd/cloud-claude/main.go`
- **D-02 不**对 `mount_strategy.go` 做结构改动；通过 `MountConfig.SyncSessionLock` / `KeepAliveInterval` / `KeepAliveCountMax` 三个 Phase 31 已预留字段注入
- **D-03 SSH KeepAlive**：自实现 `SendRequest("keepalive@openssh.com", true, nil)` + 每 15s 一次 + 连续 4 次失败关闭 conn；CLI 启动期校验 `keepalive_interval >= 15s`，否则 `ExitConfigError` + `SESSION_KEEPALIVE_TOO_AGGRESSIVE`
- **D-04 TCP KeepAlive**：`SetKeepAlive(true)` + `SetKeepAlivePeriod(15s)`；Linux 追加 `TCP_USER_TIMEOUT=30000`（build tag）；macOS 追加 `TCP_KEEPALIVE=15`（build tag）；失败仅 warning
- **D-05 重连状态机**：退避 `1s/2s/4s/8s/30s 上限`、不弹密码、Enter 触发立即重试、5 次 Enter 在 60s 内仍失败 → `NET_RECONNECT_GAVE_UP` + `ExitNetworkError`；mount 层**不**重启
- **D-06 输入缓冲**：`io.Pipe` 包 stdin、4KB ringBuf、ANSI 灰色本地 echo、TTY-only（非 TTY 透传）、不支持 backspace 修正
- **D-07 / D-08 / D-09 session 命名**：默认 `claude-<account_id_short8>`，`--new-session` 用 `claude-<base64url(rand6)>`（8 字符），anon 路径退化 `claude-anon-<cwd_hash8>`，长度 ≤ 32，非法字符替换为 `_`
- **D-10 远程命令模板**：`cd ... && command -v tmux && exec tmux new-session -A -d -s <session> "<wrap>" \; attach-session -t <session> || exec <fallback>`
- **D-11 --take-over**：list-clients → display-message → sleep 3 → detach-client -a → 本端 attach
- **D-12 第二端 banner**：`✓ 已 attach 到会话 X（另 N 个会话正在共享：source / 时间）`
- **D-13 / D-14 sessions ls/attach**：纯客户端逻辑，零控制面改造（OOS-A20）
- **D-15 / D-16 tmux 探测降级**：`command -v tmux && tmux -V`，失败 → `runClaude` 走 v2.0 裸路径 + `[!]` banner，**不**阻塞启动
- **D-17 单例锁**：容器侧 `flock -n -E 99 -F /var/lock/cloud-claude/sync-<account_id>.lock`，后端拿不到锁 → `errSyncLocked` → `MountWorkspace` 降级 sshfs-only
- **D-18 / D-19 锁注入**：实现 `acquireSyncLock(conn, accountID) (release func(), err error)` 注入 `MountConfig.SyncSessionLock`；anon 路径跳过锁
- **D-20 错误码**：注册 10 条新码（7 SESSION_* + 3 NET_*）
- **D-22 / D-23 三态 UX**：`>1.5s` 灰 `…`、`>8s` 黄 `网络抖动中（N 秒）`、`>30s` 红 `网络已断 N 秒`，`NO_COLOR` 时降级纯文本
- **D-24..D-27 接口**：复用 Phase 30 `AuthResponse.ClaudeAccountID`、Phase 29 `sshd_config` 基线、Phase 31 `SyncSessionLock` / `KeepAlive*` 预留接口；为 Phase 33 / 34 留 `LastSessionSnapshot` 三新字段（omitempty）
- **D-28 / D-29 流程**：`ConnectAndRunClaudeV3` 在 OAuth 检查后插入 tmux 探测 + `runClaudeWithSession`；新函数从 `runClaude` fork，原 `runClaude` 保留作 fallback / `sessions attach` 复用

### Claude's Discretion（CONTEXT 同名段）

planner / executor 可按实现便利性决定（不需要再回 discuss）：keepalive goroutine 用 errgroup vs 独立 ctx；Windows 平台 setsockopt 的 noop + warning；ringBuf 容量 4KB→8KB；`sessions ls` 用 `text/tabwriter` 列宽；`tmux list-clients` 解析分隔符 `|`；`Trigger()` channel buffer size=1+drop；锁 sleep 进程 PID 跟踪（建议直接 ssh session.Close 自然收割）；中文宽字符灰色错位允许；`--new-session` short_id 长度（默认 8，可调到 6）；多端 attach 单端时不输出 `（仅本端）`；`sessions ls --json` 不要求

### Deferred Ideas（OUT OF SCOPE）

`Mosh UDP 协议`（OOS-A6）/ `跨容器 tmux 持久化`（v3.1）/ `sessions ls --json`（Phase 34 doctor 框架统一）/ `--take-over --notify-only`（v3.1）/ `Mutagen 跨主机单例锁`（v3.1）/ `input buffer backspace 修正`（v3.1）/ `完美中文宽字符灰色对齐`（v3.1）/ `重连过程中 mount 层主动重启`（依赖 Phase 35 真机 UAT 决定是否回流）/ `sessions ls --remote-host`（v3.1）/ `严格 PTY"下次回车前"插入`（沿用 Phase 31 D-28 近似实现）

---

## Phase Requirements

| ID | Description（来自 REQUIREMENTS.md §B1/B2/B3） | Research Support |
|----|----------------------------------------------|------------------|
| REQ-F3-A | 客户端 ServerAliveInterval=15s/CountMax=4，禁止 < 15s | §Technical Approach §1（SendRequest 实证）+ §6 启动期校验代码片段 |
| REQ-F3-B | 断网期间灰色未确认本地 echo + 重连后按序提交 | §Technical Approach §4（input_buffer 实现 + ANSI 兼容性）|
| REQ-F3-C | 重连失败 prompt 显示原因 + 下一步操作 | §Technical Approach §3（永久失败兜底）+ §Acceptance Templates REQ-F3-C |
| REQ-F3-D | 退避 1s/2s/4s/8s/30s + 不弹密码 | §Technical Approach §3（状态机 + password 复用）|
| REQ-F4-A | tmux 默认包装 + 重连 attach 同会话不丢进程 | §Technical Approach §5（tmux new-session -A 决策）+ §7（C7 防御）|
| REQ-F4-B | sessions ls/attach 子命令 | §Technical Approach §5.4（sessions 子命令实现）|
| REQ-F4-C | tmux 不可用降级不阻塞 + banner 提示 | §Technical Approach §5.5（D-15/D-16 探测）|
| REQ-F5-A | 多端默认共享 attach 不踢人 | §Technical Approach §5（new-session -A 行为）|
| REQ-F5-B | 第二端 banner 中文显示来源 + 活跃时间 | §Technical Approach §5.2（**修订**：tmux client 识别）|
| REQ-F5-C | --new-session / --take-over + 冲突提示 | §Technical Approach §5.3（detach-client -a 语义）|
| REQ-F5-D | 账号级 Mutagen 单例锁 | §Technical Approach §6（**修订**：flock -F 语义 + 容器路径）|

---

## Domain Overview

本阶段把 v2.0 的"SSH 断线即终止"升级为三层韧性：

1. **网络层**（KeepAlive + 退避重连）— 复用既有 `*ssh.Client`，新加 SSH SendRequest 心跳 + TCP 平台特化 setsockopt + 自实现状态机
2. **会话层**（tmux 默认包装）— 远程进程从"裸 claude"改成"tmux 内 claude"，断线只丢 SSH session，不丢业务进程
3. **多端协调层**（共享 attach + 单例 mutagen）— tmux 默认共享、Mutagen 容器内 flock 强制单写

技术风险集中在三处：
- **SSH SendRequest 阻塞语义**（§1）— 心跳本身可能因网络问题挂起，需要 select + timeout 包裹
- **tmux client 识别**（§5.2）— CONTEXT D-12 设想用 `client_name` + 注入环境变量识别"另一端来自哪"，**实测 tmux 没有 per-client 自定义名 API**，需用 client_user/client_termname 兜底或 cloud-claude 自维护文件注册表
- **flock -F 语义 + sleep infinity 收割**（§6）— `-F` 是 "no fork" 而非 "filename"；后者已经是 positional；`sleep infinity` 通过 SSH session 关闭收割，最坏 SIGHUP 必然到达

**Primary recommendation**：按 CONTEXT.md 全部决策实现，但 planner 必须在切分 plan 时单独留一个 sub-task 处理 §5.2 提的 tmux client 识别 fallback 策略——因为这是 REQ-F5-B `<source> / <活跃时间>` 唯一的实现门槛。

---

## Project Constraints (from .cursor/rules/ + .planning/codebase/)

来自 `.planning/codebase/CONVENTIONS.md`（被 cursor 工作区识别为团队规范）：

- **沟通语言**：所有 stderr 输出、错误码 NextAction、banner 文案、注释必须中文；命令、路径、字段名保留英文原文 — 与 errcodes 已落地的中文模式完全一致 [VERIFIED: errcodes/mount.go / errcodes/net.go]
- **隐私**：所有文件不得写绝对路径（`/Users/...` / `/home/...`）；测试 fixture 使用相对路径或 `t.TempDir()`
- **无 emoji**（OOS-A15）：banner / 错误提示用 ASCII `[✓][!][✗] ⚠ ✓ ✗`（既有 codebase 已用 `✓` `⚠`，与 ASCII 兼容标点；planner 沿用，**禁止**新引入 emoji）
- **零增量特权**（PROJECT.md）：本阶段所有改造（KeepAlive / TCP setsockopt / tmux / flock）必须在 v2.0 已开放的容器特权（FUSE / SYS_ADMIN / AppArmor unconfined）+ 本地用户权限内，**禁止**新增 `--privileged` 或新 capability

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| SSH 心跳（应用层） | cloud-claude（Go x/crypto/ssh） | — | v2.0 用 ssh.Client，自实现 SendRequest 是最低侵入 |
| TCP keepalive（传输层） | cloud-claude（syscall + build tag） | OS 内核 | 必须客户端主动 setsockopt；Linux/macOS 常量不同 |
| 断线检测 + 重连 | cloud-claude（reconnect.go） | — | 容器侧无能力感知客户端状态 |
| 输入缓冲 + 灰色 echo | cloud-claude（input_buffer.go） | 本地 stdout | 唯一可行位置（远端断线就是断了） |
| tmux 进程持久化 | 容器内（tmux 3.4+ 已就位） | cloud-claude 包远程命令 | Phase 29 D-06 已 ship；本阶段只调用 |
| tmux 多端共享 | tmux server（容器内） | cloud-claude banner 渲染 | tmux new-session -A 原生支持 |
| Mutagen 单例锁 | 容器内（util-linux flock） | cloud-claude 通过 SSH 注入 | 必须容器侧强制（多 cloud-claude 进程跨主机检测不到对方） |
| 错误码注册 | cloud-claude 包级 init() | Phase 34 doctor 复用 | 与 Phase 31 errcodes/{mount,net}.go 一致模式 |
| `sessions ls/attach` 子命令 | cloud-claude（cmd 层） | 容器内 tmux | OOS-A20 明令禁止控制面新增 endpoint |

---

## Standard Stack

### Core（已就位 + 验证版本）

| 库 / 命令 | 版本 | 用途 | 来源 |
|---------|---------|---------|--------------|
| `golang.org/x/crypto/ssh` | go.mod 既有 | SSH client + SendRequest | `internal/cloudclaude/ssh.go` 已用 [VERIFIED: import] |
| `golang.org/x/term` | go.mod 既有 | term.MakeRaw / IsTerminal | `ssh.go` runClaude 已用 [VERIFIED] |
| `al.essio.dev/pkg/shellescape` | go.mod 既有 | shell 命令拼接转义 | Phase 31 mount_*.go 已用 [VERIFIED] |
| `github.com/spf13/cobra` | go.mod 既有 | sessions 子命令注册 | `cmd/cloud-claude/main.go` 既有模式 [VERIFIED] |
| `tmux` | ≥ 3.4 | 容器内会话持久化 + 多端 | Phase 29 D-06 + entrypoint `assert_tmux_version` [VERIFIED: deploy/docker/managed-user/entrypoint.sh:102-115] |
| `util-linux flock` | ubuntu:24.04 内置 ≥ 2.39 | 容器内单例锁；`-F` no-fork 选项 ≥ 2.34 | ubuntu:24.04 默认 [VERIFIED: ubuntu 24.04 ships flock 2.39.3] |
| `sshd` `ClientAliveInterval=15` / `ClientAliveCountMax=8` / `MaxSessions=30` | OpenSSH 9.x | 服务端 KeepAlive 与并发上限 | Phase 29 D-14 [VERIFIED: deploy/docker/managed-user/sshd_config:14-17] |

### 不引入新库

planner / executor 严禁引入新依赖（CONTEXT D-03 明确）。所有新文件**只**用上述既有依赖 + Go 标准库（`syscall`, `context`, `time`, `sync`, `crypto/rand`, `encoding/base64`, `text/tabwriter`, `io`, `bufio`, `os/exec`）。

### Alternatives Considered（验证 CONTEXT D-03 拍板）

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| 自实现 `SendRequest` 心跳 | `OpenSSH ssh -o ControlMaster=auto` 子进程 | (a) 破坏 v2.0 既有 ExecProxy / sftp 复用；(b) macOS / Windows ControlMaster 行为不一致；(c) 失去 input_buffer 直接操作 *ssh.Client 的能力 — **CONTEXT D-03 已 reject** |
| 客户端本地 mutex | 容器内 flock | 客户端本地 mutex 跨主机失效；Mutagen daemon 进程是 host 级别但锁要看 claude_account — **CONTEXT D-17 已选 flock** |
| Mosh UDP | SSH + 应用层 keepalive | OOS-A6 排除（与 sing-box tun + nftables 默认拒绝不兼容） |
| dtach 替代 tmux | tmux | dtach 不支持多端 attach（单 client 模型）；REQ-F5-A 直接否决 |

---

## Technical Approach

### 1. SSH 客户端 KeepAlive（`keepalive.go`，REQ-F3-A）

#### 1.1 SendRequest 真实行为 [VERIFIED: x/crypto/ssh source pkg.go.dev]

`golang.org/x/crypto/ssh` 的 `*ssh.Client` 嵌入 `Conn`，提供：

```go
SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error)
```

关键语义（来自 x/crypto/ssh 源码 `client.go` + `transport.go`）：

- 这是 **SSH global request**，不依赖 session/channel，与 `NewSession()` 完全正交 — 不会冲突 [VERIFIED]
- `wantReply=true` 时，server 必须回 `SSH_MSG_REQUEST_SUCCESS` / `SSH_MSG_REQUEST_FAILURE`；OpenSSH 对未知 request name `keepalive@openssh.com` 一律回 FAILURE，但回包本身就是"对端活着"的证据 — 这是 OpenSSH 客户端 `ServerAliveInterval` 的标准做法 [CITED: OpenSSH PROTOCOL.md `keepalive@openssh.com`]
- **`SendRequest` 在 conn 已关闭时立即返回 `io.EOF` 或 `*net.OpError`** — 不会 hang 在 dead conn 上 [VERIFIED: ssh/mux.go]
- **但 `SendRequest` 在 conn 仍 open 但网络停滞时会无限 block**（等 reply）— 这是 §1.2 必须自加 timeout 的根本原因 [ASSUMED: 推断自源码 + OpenSSH 行为]

#### 1.2 推荐实现模式

```go
// 文件: internal/cloudclaude/keepalive.go
package cloudclaude

import (
    "context"
    "errors"
    "time"

    "golang.org/x/crypto/ssh"
)

// Run 在 conn 上每 interval 发一次 keepalive；连续 countMax 次失败（含超时）后关闭 conn。
// 关闭 conn 后上层 reconnect 状态机感知 io.EOF 触发重连。
//
// 单次 SendRequest 的 timeout = interval（足够；> interval 等于"上次还没回新一次又来了"）。
func RunKeepAlive(ctx context.Context, conn ssh.Conn, interval time.Duration, countMax int) error {
    if interval < 15*time.Second {
        return errors.New("keepalive interval 必须 >= 15s")
    }
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    fails := 0
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            ok, err := sendKeepaliveWithTimeout(conn, interval)
            if err == nil && ok {
                fails = 0
                continue
            }
            fails++
            if fails >= countMax {
                _ = conn.Close() // 让上层 reconnect 感知
                return err
            }
        }
    }
}

func sendKeepaliveWithTimeout(conn ssh.Conn, timeout time.Duration) (bool, error) {
    type result struct {
        ok  bool
        err error
    }
    ch := make(chan result, 1)
    go func() {
        // OpenSSH 对未知 global request 回 FAILURE；只要 err == nil 就证明对端活着
        _, _, err := conn.SendRequest("keepalive@openssh.com", true, nil)
        ch <- result{ok: err == nil, err: err}
    }()
    select {
    case <-time.After(timeout):
        return false, errors.New("keepalive timeout")
    case r := <-ch:
        return r.ok, r.err
    }
}
```

确认要点：
- **wantReply=true 必须**（false 时无法判定对端是否还在）
- **不在乎 reply 的 ok 字段**（OpenSSH 对未知 request name 必返 FAILURE，但回包本身证明 alive）
- **超时 = interval**（典型 15s，留一个完整 interval 给慢连接）
- ctx cancel 后只 return，不 close conn（让 reconnect 决定）

#### 1.3 启动期校验 < 15s（REQ-F3-A 强约束 / PITFALLS M11）

```go
// cmd/cloud-claude/main.go 在 mountCfg 构造后立即校验
if mountCfg.KeepAliveInterval < 15*time.Second {
    fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE,
        mountCfg.KeepAliveInterval.String()))
    os.Exit(exitConfigError) // = 4
}
```

---

### 2. TCP 层 KeepAlive 平台特化（`keepalive_linux.go` / `keepalive_darwin.go` / `keepalive_other.go`）

#### 2.1 跨平台基础

```go
// 公共部分（keepalive.go）
func ConfigureTCPKeepAlive(tcpConn *net.TCPConn, period time.Duration) error {
    if err := tcpConn.SetKeepAlive(true); err != nil {
        return err
    }
    if err := tcpConn.SetKeepAlivePeriod(period); err != nil {
        return err
    }
    return configurePlatformSpecific(tcpConn) // build tag 分发
}
```

`SetKeepAlive(true)` → `setsockopt(SOL_SOCKET, SO_KEEPALIVE, 1)` 跨平台一致 [VERIFIED: net/tcpsock.go]

`SetKeepAlivePeriod(d)` 平台行为 [VERIFIED: net/tcpsockopt_unix.go + tcpsockopt_darwin.go]：
- **Linux**：同时设 `TCP_KEEPIDLE = d` + `TCP_KEEPINTVL = d`（同值）
- **macOS**：仅设 `TCP_KEEPALIVE = d`（≈ Linux 的 TCP_KEEPIDLE）
- **`TCP_KEEPCNT` 全平台默认 9 次**（Linux `tcp_keepalive_probes` / macOS sysctl `net.inet.tcp.keepcnt`）

#### 2.2 Linux: TCP_USER_TIMEOUT [VERIFIED: tcp(7) man page Linux 2.6.37+]

```go
// 文件: internal/cloudclaude/keepalive_linux.go
//go:build linux

package cloudclaude

import (
    "net"
    "syscall"
)

const tcpUserTimeout = 18 // = syscall.TCP_USER_TIMEOUT in Linux headers, but Go syscall pkg 不导出此常量

func configurePlatformSpecific(tcpConn *net.TCPConn) error {
    rawConn, err := tcpConn.SyscallConn()
    if err != nil {
        return err
    }
    var sockErr error
    err = rawConn.Control(func(fd uintptr) {
        // TCP_USER_TIMEOUT 单位毫秒；30000 = 30s 内未收到 ACK 则关闭 conn
        // 与 SO_KEEPALIVE 正交：keepalive 走探测包，user_timeout 控制实际数据 ACK 上限
        sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpUserTimeout, 30000)
    })
    if err != nil {
        return err
    }
    return sockErr
}
```

**为什么 30000ms（30s）合适**：
- 与 ROADMAP §Phase 32 SC2 的 30s 抖动验收对齐 [VERIFIED: ROADMAP.md:230]
- 比 sshd 服务端 `ClientAliveCountMax=8 × ClientAliveInterval=15s = 120s` 严格得多 — 客户端先于服务端宣告"断"
- 60s 可作为更保守值，但与 D-04 决策不符 — **planner 沿用 30000**

#### 2.3 macOS: TCP_KEEPALIVE [VERIFIED: Darwin tcp.h]

```go
// 文件: internal/cloudclaude/keepalive_darwin.go
//go:build darwin

package cloudclaude

import (
    "net"
    "syscall"
)

const tcpKeepalive = 0x10 // <netinet/tcp.h> TCP_KEEPALIVE on Darwin

func configurePlatformSpecific(tcpConn *net.TCPConn) error {
    rawConn, err := tcpConn.SyscallConn()
    if err != nil {
        return err
    }
    var sockErr error
    err = rawConn.Control(func(fd uintptr) {
        // Darwin TCP_KEEPALIVE 单位秒；15s 与 Linux TCP_KEEPIDLE 等价
        // SetKeepAlivePeriod 已经设过；这里再设一次是显式声明（Go stdlib 内部已经设这个 sockopt）
        // 实际上是冗余的，但保留做为 platform-specific 钩子，方便日后加 TCP_KEEPCNT/INTVL
        sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpKeepalive, 15)
    })
    if err != nil {
        return err
    }
    return sockErr
}
```

**说明**：Go 的 `SetKeepAlivePeriod` 在 macOS 已经走 `TCP_KEEPALIVE` [VERIFIED: tcpsockopt_darwin.go]，所以 keepalive_darwin.go 实际上是 noop 占位。**planner 可以选择只让 darwin 文件 return nil，把 setsockopt 完全交给 stdlib**（更干净）。但保留 hook 让后续加 `TCP_KEEPCNT` / `TCP_KEEPINTVL` 时不用改公共代码。

#### 2.4 Windows / 其它平台

```go
// 文件: internal/cloudclaude/keepalive_other.go
//go:build !linux && !darwin

package cloudclaude

import (
    "fmt"
    "net"

    "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

func configurePlatformSpecific(tcpConn *net.TCPConn) error {
    // SetKeepAlive + SetKeepAlivePeriod 仍然生效（stdlib 已处理）
    // 平台特化优化跳过；输出 warning 但不阻塞（best-effort）
    fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.NET_TCP_KEEPALIVE_UNSUPPORTED, runtime.GOOS))
    return nil
}
```

**Windows 是否实际支持 cloud-claude**：v2.0 既有 `cmd/cloud-claude/main.go` 没有 GOOS 限制 [VERIFIED: 无 build tag]，所以理论上 windows 可编译。但 PTY / shellescape / mutagen-bin embed 在 windows 上未经测试。**本阶段沿用 best-effort + warning 策略**（CONTEXT 草决策一致）。

#### 2.5 在 sshConnect 中接入

修改 `internal/cloudclaude/ssh.go:sshConnect`：

```go
func sshConnect(cfg SSHConfig) (*ssh.Client, error) {
    // ... 既有 dial / handshake 逻辑

    addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
    tcpConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
    if err != nil {
        return nil, fmt.Errorf("SSH 连接失败（无法连接 %s）: %w", addr, err)
    }

    // [新增] TCP keepalive — best-effort，失败仅 warning
    if tc, ok := tcpConn.(*net.TCPConn); ok {
        if e := ConfigureTCPKeepAlive(tc, 15*time.Second); e != nil {
            fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.NET_TCP_KEEPALIVE_UNSUPPORTED, e.Error()))
        }
    }

    sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientCfg)
    // ... 余下保持不变
}
```

---

### 3. 重连状态机（`reconnect.go`，REQ-F3-C / D）

#### 3.1 状态机签名

```go
// 文件: internal/cloudclaude/reconnect.go

type ConnState int32

const (
    StateConnected ConnState = iota
    StateReconnecting
    StateGaveUp
)

// Reconnector 在原 conn 关闭时触发重连，并把新 conn 通过 onReconnected 回调交还。
// 自身不持有 *ssh.Client；原 conn 由 sshConnect 工厂创建，新 conn 也是。
type Reconnector struct {
    cfg              SSHConfig            // 复用启动期 password — REQ-F3-D 不弹密码核心
    keepaliveCfg     KeepAliveConfig
    onConnLost       func()                // 通知 input_buffer 进入 Reconnecting
    onReconnected    func(*ssh.Client) error // 重新挂 keepalive + flush input_buffer
    triggerCh        chan struct{}         // size=1 + drop 多余 trigger（防 spam）
    state            atomic.Int32          // ConnState
    disconnectStart  atomic.Int64          // unix nano；> 0 表示断开中
    fastRetryCount   int                   // Enter 触发的快速重试次数
    fastRetryWindow  time.Time             // 60s 窗口起点
}

const (
    backoffMax = 30 * time.Second
)

var backoffSeq = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 30 * time.Second}

// Run 阻塞执行重连循环；退避失败兜底后返回 ErrGaveUp。
func (r *Reconnector) Run(ctx context.Context) error { ... }

// Trigger 由 input_buffer 在用户按 Enter 时调用，立即唤醒重连尝试。
func (r *Reconnector) Trigger() {
    select {
    case r.triggerCh <- struct{}{}:
    default: // drop（防 Enter spam）
    }
}
```

#### 3.2 退避循环骨架

```go
func (r *Reconnector) Run(ctx context.Context) error {
    r.disconnectStart.Store(time.Now().UnixNano())
    r.onConnLost()
    r.state.Store(int32(StateReconnecting))
    defer r.state.Store(int32(StateConnected))

    backoffIdx := 0
    for {
        delay := backoffSeq[backoffIdx]
        timer := time.NewTimer(delay)
        select {
        case <-ctx.Done():
            timer.Stop()
            return ctx.Err()
        case <-r.triggerCh:
            timer.Stop()
            // 用户按 Enter — 立即重试，但记录 fastRetry 用于兜底判定
            r.recordFastRetry()
            if r.exceededFastRetryBudget() {
                r.state.Store(int32(StateGaveUp))
                return errReconnectGaveUp
            }
        case <-timer.C:
            // 自然退避到点
        }

        newConn, err := sshConnect(r.cfg)
        if err == nil {
            if cbErr := r.onReconnected(newConn); cbErr == nil {
                r.disconnectStart.Store(0)
                return nil
            }
        }

        // 失败：推进退避序号，但不超过 max
        if backoffIdx < len(backoffSeq)-1 {
            backoffIdx++
        }
    }
}

// 永久失败兜底：60s 窗口内 5 次 Enter 触发的快速重试都失败
func (r *Reconnector) exceededFastRetryBudget() bool {
    now := time.Now()
    if now.Sub(r.fastRetryWindow) > 60*time.Second {
        r.fastRetryWindow = now
        r.fastRetryCount = 1
        return false
    }
    r.fastRetryCount++
    return r.fastRetryCount > 5
}
```

#### 3.3 mount 层在重连过程**不**重启（CONTEXT D-05 第 6 条 + Phase 31 D-27/D-28 兼容）

| 层 | reconnect 期间行为 | 自愈机制 |
|---|---|---|
| Mutagen sync session | 不重启 | Mutagen daemon 自身 transport reconnect [CITED: mutagen-io/mutagen issues #475 reconnect; v0.18.1 changelog "improved transport resilience"] |
| sshfs | 不重启 | sshfs `reconnect,ServerAliveInterval=15` 选项会自动重连 [CITED: sshfs man page `-o reconnect`]；Phase 31 已配 |
| mergerfs | 不重启 | 容器内常驻进程，与客户端连接无关 |
| sshfs_watcher | 不重启 | 用 conn-A，但 conn-A 是 reconnect 重建的 — **风险点**：watcher goroutine 持有旧 conn 引用 |

**watcher 持有旧 conn 的风险**：Phase 31 `mount_strategy.go:tryModeReal` 用 `connA` 创建 `SSHFSWatcher`。reconnect 重建 conn-A 后，watcher 仍指向旧的（已关闭）conn。watcher 的 `checkOnce` 会失败，触发 false-positive 摘除 cold branch。

**缓解方案**（planner 评估）：
- (a) **reconnect 不影响 watcher**：watcher 的 checkOnce 失败次数 = 失败计数，3 次后摘除。reconnect 期间这是预期行为（cold 已经断了）。摘除后等 Phase 34 doctor --fix 重挂，符合 Phase 31 D-27 设计。**推荐**。
- (b) **reconnect 后重建 watcher**：让 onReconnected 回调中重启 watcher 指向新 conn。复杂、与 D-05 第 6 条"mount 层不重启"冲突。**不推荐**。

planner 选 (a) — 与 CONTEXT.md 完全一致；执行期不需要改 sshfs_watcher.go。

#### 3.4 三态 UX 渲染（D-22 / D-23）

```go
// reconnect.go 内独立 ticker goroutine，每 100ms 评估 disconnectDuration
func (r *Reconnector) renderStatus(ctx context.Context, w io.Writer, noColor bool) {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    var lastRendered string
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            startNs := r.disconnectStart.Load()
            if startNs == 0 {
                if lastRendered != "" {
                    fmt.Fprint(w, "\r\x1b[K") // 清空当前行
                    lastRendered = ""
                }
                continue
            }
            elapsed := time.Duration(time.Now().UnixNano() - startNs)
            text := renderDisconnectStatus(elapsed, noColor)
            if text != lastRendered {
                fmt.Fprint(w, "\r\x1b[K"+text)
                lastRendered = text
            }
        }
    }
}

func renderDisconnectStatus(d time.Duration, noColor bool) string {
    secs := int(d.Seconds())
    switch {
    case d < 1500*time.Millisecond:
        return ""
    case d < 8*time.Second:
        if noColor {
            return fmt.Sprintf("… %.1fs", d.Seconds())
        }
        return fmt.Sprintf("\x1b[90m… %.1fs\x1b[0m", d.Seconds())
    case d < 30*time.Second:
        if noColor {
            return fmt.Sprintf("⚠ 网络抖动中（%d 秒未响应）", secs)
        }
        return fmt.Sprintf("\x1b[33m⚠ 网络抖动中（%d 秒未响应）\x1b[0m", secs)
    default:
        if noColor {
            return fmt.Sprintf("✗ 网络已断 %d 秒，正在自动重试…", secs)
        }
        return fmt.Sprintf("\x1b[31m✗ 网络已断 %d 秒，正在自动重试…\x1b[0m", secs)
    }
}
```

---

### 4. 本地输入缓冲（`input_buffer.go`，REQ-F3-B）

#### 4.1 结构与生命周期

```go
// 文件: internal/cloudclaude/input_buffer.go

type BufferedStdin struct {
    src       io.Reader      // os.Stdin
    pipeW     io.WriteCloser // 接 ssh session.Stdin
    state     atomic.Int32   // ConnState（共享自 reconnect.go）
    ringBuf   []byte         // 4KB 环形
    ringMu    sync.Mutex
    localEcho io.Writer      // os.Stdout
    noColor   bool
    onEnter   func()         // 在 Reconnecting 状态下按 Enter 触发 reconnect.Trigger()
}

// pipeR 端用于 ssh.Session.Stdin = pipeR
func NewBufferedStdin(src io.Reader, localEcho io.Writer, noColor bool, onEnter func()) (*BufferedStdin, io.Reader)

// Run 阻塞读 src 字节流，按 state 决定：
//   Connected     → 直写 pipeW
//   Reconnecting  → 写 ringBuf + 灰色 echo localEcho + 检测 \r/\n 触发 onEnter
//   GaveUp        → 直接丢弃（连 Enter 都不响应）
func (b *BufferedStdin) Run(ctx context.Context) error
```

#### 4.2 PTY raw mode 下 stdin 的字节语义

[VERIFIED: term.MakeRaw + termios(3) + ssh.go:184-188]

raw mode 后 stdin 是逐字节非 cooked，没有行缓冲。每按一个键 → 一字节立即可读。这对 input_buffer 是好事：

- **Enter 键**：发送 `\r`（CR）— 注意不是 `\n`（LF）。需要 `b == '\r' || b == '\n'` 双兜底
- **方向键 / 功能键**：发送 ANSI escape `\x1b[A` 等多字节序列 — input_buffer 不需要解析，直接缓冲所有字节即可
- **Ctrl-C**：发送 `\x03` — 缓冲；远端 tmux 收到后会传给 claude
- **Backspace**：发送 `\x7f`（DEL）/ `\b`（BS）— 缓冲；本阶段不本地修正（与 Mosh 简化版对齐，CONTEXT D-06 第 4 条）

#### 4.3 灰色 echo 中文 / 宽字符兼容性

ANSI `\x1b[90m` 在以下终端验证可用 [CITED: 各终端 docs / 通用支持]：
- iTerm2 ✓ / Terminal.app ✓ / WSL ✓ / GNOME Terminal ✓ / Alacritty ✓ / Kitty ✓
- Windows cmd（旧）— 不一定渲染颜色，但 ANSI escape 不会乱码（被忽略）

**中文宽字符的灰色包裹**：每个 rune 单独包裹 `\x1b[90m中\x1b[0m\x1b[90m文\x1b[0m` 是可以的，但断网期间用户键入中文 → 远端 echo 时会有"光标跳一个宽字符位"的视觉跳动。CONTEXT D-06 第 3 条 + Claude's Discretion 已接受这种局部错位（v3.1 留完美对齐）。

**实现简化**：不做按 rune 分割，按字节流灰色包裹整段：

```go
// 进入 Reconnecting 时 echo 一次开头 \x1b[90m
// 退出 Reconnecting 时 echo 一次 \x1b[0m
// 中间每个字节直写，不再加 escape — 减少 cursor 跳动
func (b *BufferedStdin) handleReconnecting(c byte) {
    b.ringMu.Lock()
    if len(b.ringBuf) >= 4096 {
        b.ringBuf = b.ringBuf[1024:] // 丢最早 1KB
        if b.localEcho != nil {
            fmt.Fprintln(b.localEcho, errcodes.Format(errcodes.SESSION_BUFFER_OVERFLOW))
        }
    }
    b.ringBuf = append(b.ringBuf, c)
    b.ringMu.Unlock()

    if b.localEcho != nil {
        // 本地 echo（已在 Reconnecting 起始时打过 \x1b[90m）
        fmt.Fprintf(b.localEcho, "%c", c)
    }
    if c == '\r' || c == '\n' {
        if b.onEnter != nil {
            b.onEnter()
        }
    }
}
```

#### 4.4 重连成功后 flush

```go
func (b *BufferedStdin) Flush() error {
    b.ringMu.Lock()
    defer b.ringMu.Unlock()
    if len(b.ringBuf) == 0 {
        return nil
    }
    if _, err := b.pipeW.Write(b.ringBuf); err != nil {
        return err
    }
    b.ringBuf = b.ringBuf[:0]
    if b.localEcho != nil {
        fmt.Fprint(b.localEcho, "\x1b[0m") // 关闭灰色
    }
    return nil
}
```

#### 4.5 ringBuf 容量 4KB 论证

人均键入速度 5 字符/秒 × 30s = 150 字节 — 远小于 4KB。代码粘贴场景上限大致：

- 一行 80 字符 × 50 行 = 4000 字节 — 接近上限
- IDE 复制 200 行代码 ≈ 16KB — **会触发 SESSION_BUFFER_OVERFLOW**（这是设计预期：用户在断网期间粘贴大段代码本来就不安全）

**planner 可选**：把容量调到 8KB 不影响内存（CONTEXT 已开 Discretion），但**禁止超过 64KB** — 否则灰色未确认字符会让用户失去对粘贴内容的控制感。

#### 4.6 非 TTY 模式跳过缓冲（CONTEXT D-06 第 5 条）

```go
fd := int(os.Stdin.Fd())
if !term.IsTerminal(fd) {
    // 管道 / 脚本透传 — 不启用 buffer，stdin 直传 ssh session
    session.Stdin = os.Stdin
    return runWithoutBuffer(...)
}
```

---

### 5. tmux 多端 attach + take-over（`session.go`，REQ-F4 / F5）

#### 5.1 远程命令模板的竞态分析（D-10）

CONTEXT D-10 用：

```bash
exec tmux new-session -A -d -s <session> "<wrap_cmd>" \; attach-session -t <session>
```

vs 单步 `tmux new-session -A -s <session> "<wrap_cmd>"`：

[VERIFIED: tmux man page 3.4+] `new-session -A` = "make new-session behave like attach-session if session exists"。两端同时启动同名 session 时：

- 单步 `new-session -A`：tmux server 会串行处理两个请求；后到的退化为 attach-session — **不存在 race**，tmux server 内部用 mutex
- 两步 `new-session -d -A` + `attach-session`：在 `-d` 创建后 attach 之前的微小窗口，第三方 client 可能 attach 进来 — 实际上这正是 REQ-F5-A 想要的（多端默认共享）

**结论**：D-10 两步法是**正确的**，但理由不是"避免竞态"而是**控制 attach 行为**：`-d` 让创建过程不绑定到当前 client，再用 `attach-session -t` 显式 attach。这对降级 fallback 也更友好（如果 attach 失败，session 仍然存在）。

planner 沿用 D-10 写法。

#### 5.2 第二端 banner 中的 `<source>` 字段（REQ-F5-B）— **修订 D-12 实现细节**

CONTEXT D-12 设想用 `tmux list-clients -F "#{client_name}|#{client_activity}|#{client_tty}"` + 注入 `CLOUD_CLAUDE_CLIENT_NAME=<local_hostname>` 环境变量。

**实证检查 [VERIFIED: tmux 3.4 man FORMATS]**：

| Format | 实际值 | 是否可被 cloud-claude 自定义 |
|--------|--------|---------------------------|
| `#{client_name}` | 默认 = `client_tty` 路径，**不是**独立字段 | ✗ tmux 没有 rename-client 命令 |
| `#{client_user}` | tmux 客户端运行的 OS 用户名 | ✗ 容器内全是 `workspace` |
| `#{client_termname}` | `TERM` 环境变量值 | △ 可通过 `tmux attach -e TERM=xterm-256color-mac` 设，但不语义化 |
| `#{client_pid}` | tmux 客户端 PID | ✓ 唯一但对用户无意义 |
| `#{client_tty}` | controlling terminal 路径，例如 `/dev/pts/3` | ✓ 自动；唯一识别用 |
| `#{client_creation_time}` | 客户端创建 unix 秒 | ✓ 自动 |
| `#{client_activity}` | 最近活动 unix 秒 | ✓ 自动 |

**CONTEXT D-12 设想的"通过环境变量识别 client name"实测不可行** — tmux 没有 per-client user 自定义字段 API。

**planner 必须在 plan 切分时单独处理 fallback 策略**（推荐 (a)）：

- **(a) 文件注册表**（推荐）：cloud-claude 启动 attach 时通过 SSH 在容器内 `~/.cloud-claude/clients/<client_pid>.json` 写一条记录（`local_hostname` / `attach_at` / `claude_account_id`）；session.go 输出 banner 时读取目录列表过滤当前 session 的 PID 集合（来自 `tmux list-clients -F "#{client_pid}"`）。退出时 `defer os.Remove`。
- **(b) tmux server-options 自定义**：用 `tmux set-option -t <session> -g @cloud-claude-client-<pid> <hostname>`，banner 时 `tmux show-options -t <session> -g`。tmux ≥ 3.0 支持 `@` 用户选项 [VERIFIED: tmux 3.0 changelog]。**优点**：不写文件；**缺点**：cloud-claude 异常崩溃时旧 hostname 残留（需 startup gc）。
- **(c) 退化**：banner 只显示 `（另 N 个会话正在共享：<未知来源> / <活跃时间>）` — 把"谁在另一端"留 v3.1。**最简单**，但削弱 REQ-F5-B 体感。

**planner 决策建议**：选 (a) — 文件注册表最朴素、最易诊断、出错最容易清理。具体：

```bash
# attach 时（cloud-claude → SSH conn-A）
mkdir -p /workspace/.cloud-claude/clients && \
echo '{"hostname":"<local>","attach_at":<unix>,"claude_account_id":"<id>","tmux_pid":<pid>}' \
  > /workspace/.cloud-claude/clients/<tmux_client_pid>.json
# 注：用 /workspace 而不是 /var/lock，UID 1000 可写

# detach / cloud-claude 退出时
rm -f /workspace/.cloud-claude/clients/<tmux_client_pid>.json

# banner 渲染时（在 attach 之前先做）
tmux list-clients -t <session> -F '#{client_pid}|#{client_activity}|#{client_tty}'
# 然后对每个 pid 读 /workspace/.cloud-claude/clients/<pid>.json，缺失时 hostname=<unknown>
```

`<活跃时间>` = `now - client_activity_unix_seconds`，渲染：`5 分钟前活跃` / `刚刚活跃`（< 30s）/ `3 小时前活跃`。

#### 5.3 `--take-over` detach-client -a 语义（D-11）

[VERIFIED: tmux man `detach-client`] `-a` flag = "kill all clients attached to the session **except the caller**"。

**关键**：caller = 执行 `tmux detach-client -a` 这条命令的 client。但 cloud-claude 是通过 SSH 远程执行 `tmux ...` 命令，此时 SSH session 内 spawn 的 tmux 进程是**临时 client**，不是后续要 attach 的 client。所以：

```bash
# CONTEXT D-11 实际执行序列（修订）：
# 1. 探测 — SSH 上跑独立 tmux 命令
ssh: tmux list-clients -t <session> -F '#{client_pid}'
# 2. 如果有客户端，先广播
ssh: tmux display-message -t <session> '[cloud-claude] 另一端 (<src>) 已通过 --take-over 接管会话，本会话将在 3s 后断开'
ssh: sleep 3
# 3. 踢掉所有客户端（注意：这条命令的 caller 是临时 SSH，不在 client list 内，所以 -a 会踢掉所有）
ssh: tmux detach-client -t <session> -a
# 4. 然后本端 attach（成为唯一 client）
ssh: exec tmux new-session -A -d -s <session> "<wrap>" \; attach-session -t <session>
```

**修订**：`-a` 在临时 SSH client caller 上下文里相当于"踢掉所有"。等价于 `tmux detach-client -t <session>`（不带 -a 时 detach 所有 attached to session 的 clients）。两者效果一致；`-a` 更显式。**planner 沿用 -a 写法**。

#### 5.4 sessions ls / attach 子命令（D-13 / D-14）

```go
// cmd/cloud-claude/sessions.go
func newSessionsCmd() *cobra.Command {
    cmd := &cobra.Command{Use: "sessions", Short: "tmux 会话管理"}
    lsCmd := &cobra.Command{Use: "ls", RunE: runSessionsLs}
    attachCmd := &cobra.Command{Use: "attach <name>", Args: cobra.ExactArgs(1), RunE: runSessionsAttach}
    cmd.AddCommand(lsCmd, attachCmd)
    return cmd
}

// runSessionsLs:
//   1. LoadConfig + AuthenticateAndWait + sshConnect（沿用 main.go runRoot 模式）
//   2. 远程 tmux list-sessions -F "#{session_name}|#{session_created}|#{session_attached}|#{session_windows}"
//   3. 用 text/tabwriter 渲染表格
//   4. 失败（无 server / 无 session）→ "当前容器内无活跃 tmux session\n", exit 0

// runSessionsAttach:
//   1. 同样 sshConnect
//   2. 远程 tmux has-session -t args[0]，失败 → SESSION_NOT_FOUND + exit 非 0
//   3. 复用 runClaude（CONTEXT D-14：跳过 claude 包装，直接 attach）
//      remoteCmd = fmt.Sprintf("exec tmux attach-session -t %s", shellescape.Quote(name))
```

#### 5.5 tmux 不可用降级探测（D-15 / D-16）

```go
// internal/cloudclaude/session.go
func DetectTmux(conn *ssh.Client) (available bool, version string, errReason string) {
    out, err := sshRunWithOutput(conn, "command -v tmux >/dev/null 2>&1 && tmux -V 2>&1")
    if err != nil {
        return false, "", strings.TrimSpace(out)
    }
    return true, strings.TrimSpace(out), ""
}
```

`ConnectAndRunClaudeV3` 在 OAuth 检查后（runClaude 调用前）调用 `DetectTmux`，结果写 `SessionConfig.TmuxAvailable`：

```go
available, version, reason := DetectTmux(connA)
sessionCfg.TmuxAvailable = available
if !available {
    fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.SESSION_TMUX_UNAVAILABLE, reason))
    // 不退出 — 走 v2.0 裸 runClaude（D-16）
}

if available {
    return runClaudeWithSession(ctx, connA, cfg, claudeArgs, cwd, hasProxy, sessionCfg)
}
return runClaude(connA, claudeArgs, cwd, hasProxy) // v2.0 路径
```

---

### 6. 账号级 Mutagen 单例锁（`sync_lock.go`，REQ-F5-D / PITFALLS M15）

#### 6.1 flock -F 语义验证 — **CONTEXT D-17 是正确的，但理由澄清**

[VERIFIED: util-linux flock(1) man page ≥ 2.34]

```
flock [options] <file|directory> -c <command>

Options:
  -n            Fail rather than wait if the lock cannot be immediately acquired.
  -E <code>     The exit status used when the -n option is in use, and the conflicting lock exists.
  -F            Do not fork before executing command. Upon execution the flock process is replaced by command.
  -x, -e        Obtain an exclusive lock (default).
```

CONTEXT D-17 命令逐字解析：

```bash
flock -n -E 99 -F /var/lock/cloud-claude/sync-<account_id>.lock -c 'echo "lock acquired"; sleep infinity' &
```

- `-n`：拿不到锁立即返回（不阻塞）
- `-E 99`：拿不到锁时退出码 99（区别于其它错误）
- `-F`：**no fork**，flock 进程直接 exec 成 `bash -c '...'`，让 sleep infinity 直接持有锁的 fd
- `/var/lock/cloud-claude/sync-X.lock`：lockfile 路径（positional arg）
- `-c '...'`：在 shell 里执行命令

**关键收益**：`-F` 让进程树扁平，`sleep infinity` 直接是 sshd 的孙子（sshd → bash -c → sleep）。SSH session 关闭时，sshd 给整个进程组发 SIGHUP，sleep 收到 SIGHUP 自动死，bash 也死，flock 的 fd 跟着 close，锁释放。

**没有 -F 的话**：flock fork 一次，flock 进程 持有锁，子进程跑 bash → sleep。SSH 关闭时 sshd kill bash + sleep，但 flock 进程仍在，锁不释放。**-F 是必需的**。

#### 6.2 容器内路径权限：用 /workspace 不用 /var/lock — **修订 D-17**

CONTEXT D-17 用 `/var/lock/cloud-claude/sync-X.lock`。但 [VERIFIED: ubuntu:24.04 默认]：

- `/var/lock` 在 ubuntu:24.04 是 `lrwxrwxrwx 1 root root → /run/lock`
- `/run/lock` 默认 mode = `0755`，owner `root:root`
- workspace 用户 UID=1000 — `mkdir /var/lock/cloud-claude` 会 EACCES

**两个候选**（planner 选）：

| 路径 | 优点 | 缺点 |
|------|------|------|
| `/run/cloud-claude/` | tmpfs，重启清空，符合 lock 语义 | 默认 root 拥有；需要 entrypoint 创建 + chown 1000:1000 — **改 Phase 29 entrypoint**（违反 D-25 "Phase 29 不改"） |
| `/workspace/.cloud-claude/locks/` | UID 1000 已拥有 /workspace；现成可写 | mergerfs 视图下 lock 文件可能进 hot/cold branch 同步 — **必须显式 ignore** |
| `/tmp/cloud-claude-locks/` | UID 1000 写权限通常 ok（mode 1777） | tmp 可能被清理；多用户场景污染 |

**推荐 `/workspace/.cloud-claude/locks/`** 配套 mutagen ignore 规则（Phase 31 默认 ignore 已含 `.git/` 等，添加 `.cloud-claude/` 一行即可）— 但这又**修改 Phase 31 默认 ignore**，需要小改 `mount_mutagen.go`。

**最干净的路径**：`/tmp/cloud-claude/locks/<account_id>.lock`（mode 1777 default，UID 1000 可写）— 不污染 mergerfs / Mutagen，cloud-claude 启动时 `mkdir -p /tmp/cloud-claude/locks` 幂等。

**planner 决策建议**：用 `/tmp/cloud-claude/locks/` — 与 Phase 31 / Phase 29 完全解耦，0 侵入。文件被 OS 重启清理符合 lock 语义。

修订后命令：

```bash
mkdir -p /tmp/cloud-claude/locks && \
flock -n -E 99 -F /tmp/cloud-claude/locks/sync-<account_id>.lock \
  -c 'echo $$; exec sleep infinity' &
echo $!  # 拿到 background PID
```

注意 `exec sleep infinity` 让 sleep 替换 bash，进一步压平进程树（与 -F 协同）。

#### 6.3 实现接口

```go
// 文件: internal/cloudclaude/sync_lock.go

// AcquireSyncLock 在 conn 上创建账号级单例锁。
// 返回 release 函数（idempotent），调用者必须 defer release()。
//
// 错误：
//   ErrSyncLocked — 锁被占（exit code 99） → 调用方降级 sshfs-only
//   其它 error  — flock 不可用（极旧镜像） → 调用方 warning 后跳过锁机制
func AcquireSyncLock(conn *ssh.Client, accountID string) (release func(), err error) {
    if accountID == "" {
        // CONTEXT D-19 — anon 路径跳过锁
        return func() {}, nil
    }

    lockPath := fmt.Sprintf("/tmp/cloud-claude/locks/sync-%s.lock", accountID)
    cmd := fmt.Sprintf(
        "mkdir -p /tmp/cloud-claude/locks 2>/dev/null && " +
        "flock -n -E 99 -F %s -c 'echo $$; exec sleep infinity' &\necho $!",
        shellescape.Quote(lockPath),
    )

    session, err := conn.NewSession()
    if err != nil {
        return nil, err
    }
    out, runErr := session.CombinedOutput(cmd)
    session.Close()

    if runErr != nil {
        // SSH ExitError code=99 → 锁被占
        if exitErr, ok := runErr.(*ssh.ExitError); ok && exitErr.ExitStatus() == 99 {
            return nil, ErrSyncLocked
        }
        return nil, fmt.Errorf("flock 启动失败: %w (output: %s)", runErr, out)
    }

    // out 第一行 = inner $$（sleep PID），第二行 = outer $! （bash PID）— 我们要的是 outer
    pid := parseLastInt(out)

    release = func() {
        // 通过另一个 session 远程 kill；忽略错误（异常退出场景下进程已死）
        killSess, e := conn.NewSession()
        if e != nil {
            return
        }
        defer killSess.Close()
        _ = killSess.Run(fmt.Sprintf("kill %d 2>/dev/null || true", pid))
    }
    return release, nil
}

var ErrSyncLocked = errors.New("sync session locked by another cloud-claude")
```

#### 6.4 注入 MountConfig（D-18）

`cmd/cloud-claude/main.go runRoot` 中：

```go
mountCfg := cloudclaude.MountConfig{
    Mode:              mode,
    KeepAliveInterval: 15 * time.Second,
    KeepAliveCountMax: 4,
    NoColor:           os.Getenv("NO_COLOR") != "",
    SyncSessionLock: func(accountID string) (func(), error) {
        // 闭包捕获 connA — 但此时 connA 还没建...
        // 需要重构：让 SyncSessionLock 接收 conn 参数
    },
}
```

**实现细节**：`SyncSessionLock` 是 `func(accountID string) (func(), error)`，但 connA 是 `MountWorkspace` 内部建立的。**Phase 31 D-31 接口设计有缺陷** — 锁需要 SSH 连接，但接口没传 conn。

**解决方案**：在 `ConnectAndRunClaudeV3` 内部覆盖 `cfg.SyncSessionLock`：

```go
// ssh.go ConnectAndRunClaudeV3 内：
mountCfg.SyncSessionLock = func(accountID string) (func(), error) {
    return AcquireSyncLock(connA, accountID)
}
```

这样调用方（main.go）传 nil 或默认 noop，`ConnectAndRunClaudeV3` 拿到 connA 后再覆盖。**修订**：CONTEXT D-18 隐含此意图（"本阶段提供 acquireSyncLock"），但应在 `ssh.go` 注入而非 `main.go`。planner 在 plan 切分时按此实现。

---

### 7. PITFALLS C7 / C3 验收（与 Phase 29 / Phase 31 协同）

#### 7.1 C7（systemd-logind 杀 tmux）回归

[VERIFIED: deploy/docker/managed-user/Dockerfile + entrypoint.sh] Phase 29 已经：

- 容器 PID 1 = sshd（最终 `exec /usr/sbin/sshd -D -e`）— **没有 tini，没有 systemd**
- entrypoint 不安装 / 不启动 systemd-logind
- CONTEXT D-25 提到"tini PID 1" — 实际是 sshd PID 1（与 ROADMAP §Phase 29 SC5 "PID 1 为 tini" 文字有出入；但**不影响 C7 防御**：只要无 systemd-logind 即可）

**回归命令**：

```bash
# 在容器内
pgrep systemd-logind  # 必须无输出，exit code 1
ps -p 1 -o comm=      # 必须输出 sshd（或 tini，取决于 final entrypoint）
```

**C7 攻击模拟**：

```bash
# 容器内一个 SSH session 起 tmux
tmux new -d -s test
sleep 1
# 触发 sshd 重读配置（不杀 sshd 自己）
sudo pkill -SIGHUP sshd
sleep 2
# 验证 tmux server 仍存活
tmux ls | grep test  # 必须命中
```

#### 7.2 C3（sshfs 抖动级联）回归

Phase 31 已落 `SSHFSWatcher`（5s 检测周期 × 3 次失败 = ≥ 15s 摘除阈值）。本阶段验收：

```bash
# 准备：cloud-claude 在 full mode 下连接，mergerfs 视图正常
# 注入 30s 网络抖动
docker network disconnect <ctr> <net>
sleep 30
# 期间从客户端：
# - cloud-claude reconnect 状态机进入退避 / Enter 立即重试
# - 容器内 sshfs_watcher 在 ~15s 后摘除 cold branch
# - mergerfs /workspace 仍可读 hot branch（可能丢 cold-only 文件）
# - tmux session 内 claude 进程不丢（关键 — 这是 BASE-03 核心）

docker network connect <ctr> <net>
sleep 5
# 客户端 cloud-claude reconnect 成功 → input_buffer flush → tmux attach 恢复
# cold branch 留待 Phase 34 doctor --fix 重挂（本阶段不自动恢复）
```

#### 7.3 mergerfs branch 摘除协议（Phase 31 已落）

[VERIFIED: internal/cloudclaude/sshfs_watcher.go + 调用 RemoveBranch] Phase 31 通过 `RemoveBranch(conn, "/workspace-cold", "/workspace")` 实现。底层应该用 mergerfs xattr 协议 [CITED: mergerfs README "Runtime config"]：

```bash
# 摘除 cold branch（mergerfs 2.41.x 语法）
setfattr -n user.mergerfs.branches -v "/workspace-hot=RW" /workspace/.mergerfs
# 或追加形式：
echo "-/workspace-cold" > /workspace/.mergerfs/branches
```

**本阶段不需要研究 RemoveBranch 的实现细节** — Phase 31 已 ship。本阶段只需调用方知道 watcher 30s 抖动后会摘除即可。

---

### 8. 错误码注册（追加 `errcodes/session.go` + `errcodes/net.go`）

10 条新码（CONTEXT D-20 表）的具体 Message / NextAction：

```go
// 文件: internal/cloudclaude/errcodes/session.go
package errcodes

func init() {
    MustRegister(Entry{
        Code: SESSION_KEEPALIVE_TOO_AGGRESSIVE, Severity: SeverityFatal,
        Message:    "SSH KeepAlive 间隔 %s 低于 15s 下限",
        NextAction: "调整 keepalive_interval 至 >= 15s，或移除该配置使用默认值",
    })
    MustRegister(Entry{
        Code: SESSION_TMUX_UNAVAILABLE, Severity: SeverityWarn,
        Message:    "容器内 tmux 不可用：%s，会话恢复已禁用",
        NextAction: "检查容器镜像是否升级到 v3.0.0，或运行 cloud-claude doctor mount",
    })
    MustRegister(Entry{
        Code: SESSION_NOT_FOUND, Severity: SeverityError,
        Message:    "tmux 会话 %s 不存在",
        NextAction: "运行 cloud-claude sessions ls 查看当前会话列表",
    })
    MustRegister(Entry{
        Code: SESSION_TAKEOVER_NOTIFIED, Severity: SeverityInfo,
        Message:    "已通知其它 %d 个客户端断开（session: %s）",
        NextAction: "无需操作；其它客户端 3 秒后将看到中断提示",
    })
    MustRegister(Entry{
        Code: SESSION_TAKEOVER_FAILED, Severity: SeverityError,
        Message:    "tmux detach-client 命令失败: %s",
        NextAction: "运行 cloud-claude sessions ls 检查会话状态，或 cloud-claude doctor",
    })
    MustRegister(Entry{
        Code: SESSION_SYNC_LOCKED, Severity: SeverityWarn,
        Message:    "账号 %s 已有另一端在执行 Mutagen sync，本端只读 sshfs 视图",
        NextAction: "无需操作；如需独占同步，请先关闭另一端 cloud-claude",
    })
    MustRegister(Entry{
        Code: SESSION_BUFFER_OVERFLOW, Severity: SeverityWarn,
        Message:    "本地输入缓冲已满（4KB），部分历史输入已丢弃",
        NextAction: "等待网络恢复后重新输入丢失部分；避免在断网期间粘贴大段内容",
    })
}

// 文件: internal/cloudclaude/errcodes/net.go（追加，不删既有 NET_OAUTH_*）
func init() {
    MustRegister(Entry{
        Code: NET_RECONNECT_BACKOFF, Severity: SeverityInfo,
        Message:    "网络中断，正在重连（已等待 %s）",
        NextAction: "按 Enter 立即重试，或等待自动重连",
    })
    MustRegister(Entry{
        Code: NET_RECONNECT_GAVE_UP, Severity: SeverityFatal,
        Message:    "重连失败（已重试 %d 次，耗时 %s）",
        NextAction: "请检查网络后重新运行 cloud-claude，或运行 cloud-claude doctor 诊断",
    })
    MustRegister(Entry{
        Code: NET_TCP_KEEPALIVE_UNSUPPORTED, Severity: SeverityWarn,
        Message:    "TCP keepalive 平台特化失败：%s",
        NextAction: "无需操作；SSH 应用层 keepalive 仍生效，弱网检测可能略慢",
    })
}
```

并在 `errcodes/codes.go` 追加常量：

```go
const (
    // ... 既有
    SESSION_KEEPALIVE_TOO_AGGRESSIVE Code = "SESSION_KEEPALIVE_TOO_AGGRESSIVE"
    SESSION_TMUX_UNAVAILABLE         Code = "SESSION_TMUX_UNAVAILABLE"
    SESSION_NOT_FOUND                Code = "SESSION_NOT_FOUND"
    SESSION_TAKEOVER_NOTIFIED        Code = "SESSION_TAKEOVER_NOTIFIED"
    SESSION_TAKEOVER_FAILED          Code = "SESSION_TAKEOVER_FAILED"
    SESSION_SYNC_LOCKED              Code = "SESSION_SYNC_LOCKED"
    SESSION_BUFFER_OVERFLOW          Code = "SESSION_BUFFER_OVERFLOW"
    NET_RECONNECT_BACKOFF            Code = "NET_RECONNECT_BACKOFF"
    NET_RECONNECT_GAVE_UP            Code = "NET_RECONNECT_GAVE_UP"
    NET_TCP_KEEPALIVE_UNSUPPORTED    Code = "NET_TCP_KEEPALIVE_UNSUPPORTED"
)
```

[VERIFIED: errcodes/codes.go:56] 命名正则 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$` 兼容上述全部 10 条（4 段最长 `SESSION_KEEPALIVE_TOO_AGGRESSIVE` 命中 4 段路径）。

---

### 9. last-session.json schema 扩展（D-27）

```go
// 文件: internal/cloudclaude/last_session.go
type LastSessionSnapshot struct {
    SchemaVersion       int             `json:"schema_version"`
    Timestamp           time.Time       `json:"timestamp"`
    IntendedMode        string          `json:"intended_mode"`
    ActualMode          string          `json:"actual_mode"`
    DowngradeChain      []DowngradeStep `json:"downgrade_chain"`
    ConflictCount       int             `json:"conflict_count"`
    ClaudeAccountID     string          `json:"claude_account_id,omitempty"`
    ImageVersion        string          `json:"image_version,omitempty"`
    APFSCaseInsensitive bool            `json:"apfs_case_insensitive"`

    // [Phase 32 新增] 全部 omitempty + schema_version 仍为 1
    TmuxSession    string `json:"tmux_session,omitempty"`     // 实际 attach 的 session 名
    ClientRole     string `json:"client_role,omitempty"`      // "primary" | "secondary"
    ReconnectCount int    `json:"reconnect_count,omitempty"`  // 本次会话累计重连次数
}
```

写入时机：`runClaudeWithSession` 完成 attach 后写一次（含 tmux_session / client_role）；reconnect 状态机每次成功重连 +1 ReconnectCount + 重写一次。

---

## Patterns to Reuse

来自 Phase 29 / 30 / 31 既有代码的复用 pattern（planner 切分 plan 时严格遵循，禁止重新发明）：

### P-01 errcodes 注册模式（来自 Phase 31）
[VERIFIED: errcodes/mount.go:7-91 + errcodes/net.go:5-26]

```go
package errcodes

func init() {
    MustRegister(Entry{
        Code: SESSION_XXX,  // 与 codes.go 常量同名
        Severity: SeverityInfo|Warn|Error|Fatal,
        Message: "中文，可含 %s/%d 占位",
        NextAction: "中文，≤ 80 runes",
    })
}
```

包级 init() 自动注册，不需要 main 显式调用。

### P-02 errcodes.Format 唯一输出（Phase 31 D-21）
[VERIFIED: errcodes/codes.go:99-117]

所有错误输出走 `errcodes.Format(code, args...)` → `[<CODE>] <Message>\n  建议: <NextAction>`。**禁止**新建错误格式 helper、**禁止**直接 `fmt.Fprintf(os.Stderr, "[CODE] %s", ...)`。

### P-03 cobra 子命令注册（v2.0 + Phase 31 sync）
[VERIFIED: cmd/cloud-claude/main.go:39-92 + cmd/cloud-claude/sync.go:18-37]

```go
// cmd/cloud-claude/sessions.go
func newSessionsCmd() *cobra.Command {
    cmd := &cobra.Command{Use: "sessions", ...}
    cmd.AddCommand(lsCmd, attachCmd)
    return cmd
}
// main.go:
rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd())
// 在 DisableFlagParsing switch 里追加 "sessions" 关键字
```

### P-04 DisableFlagParsing 手动剥离 flag（v2.0 --mount-mode）
[VERIFIED: cmd/cloud-claude/main.go:244-269]

```go
// runRoot 中扫描 args 剥离 --new-session / --take-over：
newSession := false
takeOver := false
filtered := args[:0]
for i := 0; i < len(args); i++ {
    switch args[i] {
    case "--new-session":
        newSession = true
        continue
    case "--take-over":
        takeOver = true
        continue
    }
    filtered = append(filtered, args[i])
}
args = filtered
```

### P-05 平台分发 build tag（v2.0 envcheck.go + Phase 31）
[VERIFIED: 既有模式]

`keepalive_linux.go` / `keepalive_darwin.go` / `keepalive_other.go` 三文件分发；公共逻辑放 `keepalive.go`。

### P-06 sshfs_watcher 模式：ctx + ticker + 失败计数（Phase 31）
[VERIFIED: internal/cloudclaude/sshfs_watcher.go:51-76]

```go
// reconnect.Run 沿用此结构：ticker + select{ ctx.Done | ticker.C | trigger } + 失败计数
```

### P-07 LastSessionSnapshot omitempty 追加字段（Phase 31）
[VERIFIED: last_session.go 既有字段全 omitempty]

新字段全部 `,omitempty`，schema_version 不变（仍为 1）— 保持向后兼容。

### P-08 shellescape.QuoteCommand 拼接远程命令（v2.0 + Phase 31）
[VERIFIED: ssh.go:216-224 + 各 mount_*.go]

所有 SSH 远程命令拼接走 `shellescape.QuoteCommand([]string{...})` 或 `shellescape.Quote(arg)`。**禁止**手写 `'...'` / `"..."` 引用。

### P-09 ANSI 颜色经 colors.go 与 NO_COLOR（Phase 31 D-23）
[VERIFIED: internal/cloudclaude/colors.go:9-47]

新增 `ansiGray = "\033[90m"` 到 colors.go 常量；reconnect.go / input_buffer.go 通过 `colorize(text, ansiGray, colorEnabled(noColor, w))` 输出。

---

## Risks & Mitigations

### C3（sshfs 抖动级联）— 防御复用 Phase 31 watcher
- **触发**：30s 网络抖动期间 sshfs hang，mergerfs `func.readdir=cor` 探测 cold branch hang，整体 ls 卡死
- **本阶段防御**：sshfs_watcher 既有逻辑（5s × 3 次失败 = 15s 摘除）保持不动；本阶段验收 30s 场景下 watcher 仍工作
- **验证**：§Test Matrix REQ-F4-A reconnect_with_sshfs_jitter_30s

### C7（systemd-logind 杀 tmux）— 防御已在 Phase 29 镜像侧落地
- **触发**：sshd 收到 SIGHUP 重读配置，systemd-logind `KillUserProcesses=yes` 把孤立的 tmux server 杀掉
- **本阶段防御**：Phase 29 D-15 不安装 systemd / systemd-logind；本阶段加 `pgrep systemd-logind` 应无输出 + `pkill -SIGHUP sshd` 后 tmux 仍存活的回归
- **验证**：§Acceptance Templates ROADMAP-SC03

### M11（SSH KeepAlive 反向坑——客户端配置过激）
- **触发**：用户配 `keepalive_interval=5s`，每 5s 一次 SendRequest 探测压力 + 容器侧 sshd 计入 MaxStartups → 雪崩
- **本阶段防御**：CLI 启动期校验 `< 15s` 直接 `os.Exit(ExitConfigError)` + `SESSION_KEEPALIVE_TOO_AGGRESSIVE`
- **验证**：§Acceptance Templates REQ-F3-A

### M12（sshd MaxSessions 下溢）
- **触发**：cloud-claude 每端开 conn-A + conn-B + Mutagen 自管 conn-C = 3 sessions × N 端 → 触 MaxSessions=30
- **本阶段防御**：sshd_config `MaxSessions 30` / `MaxStartups 60:30:120`（Phase 29 D-14 已落）；本阶段不会再开新 conn（仅在 reconnect 时短暂 +1，旧 conn 已关闭）
- **验证**：M12 不需独立用例；REQ-F5-A 多端测试用例顺带覆盖（2 端 = 6 sessions << 30）

### M13（静默降级到 sshfs-only）
- **触发**：单例锁拿不到时降级，但用户没看到提示
- **本阶段防御**：`SESSION_SYNC_LOCKED` 必须 stderr 输出 + 写 last-session.json `client_role=secondary`
- **验证**：§Test Matrix REQ-F5-D second_client_sees_lock_warning

### M15（多端 Mutagen 双写）
- **触发**：两端同时 mutagen sync alpha=本地 → beta=容器；A 写覆盖 B 写
- **本阶段防御**：单例锁绑定 claude_account_id；后端拿不到锁 → mountCfg 强制 sshfs-only（不再创建 Mutagen sync）
- **验证**：§Test Matrix REQ-F5-D mutagen_only_one_session

---

## Acceptance Criteria Templates

每条 REQ-ID 对应一条可 grep / docker / shell 断言的模板。Phase 35 真机 UAT 复用同一模板（替换 docker exec → ssh real-host）。

### REQ-F3-A — KeepAlive 启动期校验
```bash
# 模拟非法配置（假设支持环境变量 / flag — planner 视具体实现切分）
KEEPALIVE_INTERVAL=10s cloud-claude
# 期望:
#   stderr: [SESSION_KEEPALIVE_TOO_AGGRESSIVE] SSH KeepAlive 间隔 10s 低于 15s 下限
#     建议: 调整 keepalive_interval 至 >= 15s，或移除该配置使用默认值
#   exit code: 4 (ExitConfigError)
```

### REQ-F3-B — 灰色未确认 echo + 重连后按序提交
```bash
# 集成测试 (使用 docker compose fixture，沿用 Phase 31 Plan 03 模式):
# 1. cloud-claude 启动并进入 claude prompt
# 2. docker network disconnect <ctr> <net>
# 3. 通过 stdin pipe 注入 "echo hello\r"
# 4. 断言：cloud-claude stdout 包含 "\x1b[90mecho hello\r\x1b[0m"（灰色 echo）
# 5. docker network connect <ctr> <net>
# 6. 等待 reconnect 成功（<= 30s）
# 7. 断言：tmux capture-pane -p 内容包含 "echo hello"（实际执行）
```

### REQ-F3-C — 重连失败 prompt 显示原因 + 下一步
```bash
# 1. cloud-claude 启动
# 2. docker stop <ctr> （永久断开）
# 3. 反复按 Enter 5 次（在 60s 内）
# 4. 断言 stderr:
#   [NET_RECONNECT_GAVE_UP] 重连失败（已重试 5 次，耗时 *s）
#     建议: 请检查网络后重新运行 cloud-claude，或运行 cloud-claude doctor 诊断
# 5. exit code: 2 (ExitNetworkError)
```

### REQ-F3-D — 退避序列 + 不弹密码
```bash
# 1. cloud-claude 启动并通过 SSH 认证（密码已在内存）
# 2. docker pause <ctr>
# 3. 启动时间戳记录 reconnect 触发
# 4. 断言：
#    - 第 1 次重试在 t+1s
#    - 第 2 次重试在 t+3s (1+2)
#    - 第 3 次重试在 t+7s (1+2+4)
#    - 第 4 次重试在 t+15s
#    - 第 5+ 次重试每 30s 一次
# 5. 整个过程 stdin 不出现密码提示（grep -i "password" 无命中）
```

### REQ-F4-A — tmux 默认包装 + 重连 attach 同会话不丢进程
```bash
# 1. cloud-claude 启动 → 远程命令应包含 "tmux new-session -A"
#    docker exec <ctr> ps aux | grep tmux  # 必须命中 tmux server
# 2. 在 claude prompt 内执行长任务（例: claude task "sleep 60"）
# 3. docker network disconnect <ctr> <net>
# 4. sleep 30
# 5. docker network connect <ctr> <net>
# 6. 断言：cloud-claude reconnect 成功，重新看到任务输出（claude 进程 PID 不变）
# 7. docker exec <ctr> tmux capture-pane -t claude-* -p | grep "sleep 60"  # 历史 buffer 完整
```

### REQ-F4-B — sessions ls / attach
```bash
cloud-claude sessions ls
# 期望表格:
# SESSION              CREATED               CLIENTS  WINDOWS
# claude-abc12345      2 小时前              2        1

cloud-claude sessions attach claude-abc12345
# 期望: attach 到既有 session

cloud-claude sessions attach nonexistent
# 期望: stderr [SESSION_NOT_FOUND] tmux 会话 nonexistent 不存在
#         建议: 运行 cloud-claude sessions ls 查看当前会话列表
#       exit code: 非 0
```

### REQ-F4-C — tmux 不可用降级
```bash
# 1. 用一个旧镜像（无 tmux）启动容器
# 2. cloud-claude 连接
# 3. 断言 stderr 包含: [SESSION_TMUX_UNAVAILABLE] 容器内 tmux 不可用：...
# 4. cloud-claude 不退出，仍能进入 claude（裸 runClaude 路径）
```

### REQ-F5-A — 多端默认共享 attach（不踢人）
```bash
# 终端 A: cloud-claude （首端，建 session）
# 终端 B: cloud-claude （第二端，attach）
# 期望:
#   终端 A stdout 不出现"被踢"
#   终端 B 看到 "✓ 已 attach 到会话 claude-XXX"
#   docker exec <ctr> tmux list-clients -t claude-XXX  # 必须 2 行
```

### REQ-F5-B — 第二端 banner 显示来源 + 活跃时间
```bash
# 终端 A 启动，等 5 分钟
# 终端 B 启动
# 期望终端 B stdout 含:
#   ✓ 已 attach 到会话 claude-abc12345
#     （另 1 个会话正在共享：<source> / 5 分钟前活跃）
# <source> 实测形式视 §5.2 (a)/(b) 决策；最低 fallback "未知来源"
```

### REQ-F5-C — --new-session / --take-over
```bash
cloud-claude --new-session
# 期望: 创建 claude-<8字符base64url>，与既有 claude-<8hex> 命名空间正交
docker exec <ctr> tmux ls | grep "claude-[A-Za-z0-9_-]\{8\}"  # 命中

# 终端 A 已 attach claude-abc12345
cloud-claude --take-over
# 期望终端 B stdout: 立即 attach
# 终端 A stdout 在 ~3s 后看到 [cloud-claude] 另一端... 已通过 --take-over 接管
# 终端 A SSH 在 ~3s 后断开
docker exec <ctr> tmux list-clients -t claude-abc12345 | wc -l  # 必须 1
```

### REQ-F5-D — 账号级 Mutagen 单例锁
```bash
# 终端 A: cloud-claude （首端，acquire 锁，跑 mutagen sync）
docker exec <ctr> mutagen sync list | grep "cloud-claude-<account>" | wc -l  # = 1
docker exec <ctr> ls /tmp/cloud-claude/locks/  # 必须有 sync-<account>.lock

# 终端 B: cloud-claude （第二端，flock 拿不到 → exit 99）
# 期望终端 B stderr 含:
#   [SESSION_SYNC_LOCKED] 账号 <account> 已有另一端在执行 Mutagen sync，本端只读 sshfs 视图
# 第二端 last-session.json: actual_mode = "sshfs-only" / client_role = "secondary"
docker exec <ctr> mutagen sync list | wc -l  # 仍然 1（不重复）

# 终端 A 退出
# 终端 B 应能在 ~5s 内 acquire 锁（kill 通过 SSH session 关闭收割）
```

### ROADMAP §Phase 32 SC03 — pkill -SIGHUP sshd 后 tmux 存活（C7）
```bash
docker exec <ctr> pgrep systemd-logind  # 必须无输出（exit 1）
docker exec -d <ctr> tmux new -d -s testtmux
sleep 1
docker exec <ctr> sudo pkill -SIGHUP sshd
sleep 2
docker exec <ctr> tmux ls | grep testtmux  # 必须命中
```

---

## Test Matrix

| 层 | 用例 | 工具 | 自动化 | 文件 |
|----|------|------|--------|------|
| 单元 | RunKeepAlive 心跳超时 | 模拟 ssh.Conn 接口 | ✓ Go test | `keepalive_test.go` |
| 单元 | RunKeepAlive 连续 N 次失败关 conn | 模拟 ssh.Conn 接口 | ✓ Go test | `keepalive_test.go` |
| 单元 | configurePlatformSpecific 不 panic | runtime.GOOS 探测 | ✓ Go test | `keepalive_test.go` |
| 单元 | 退避序列 1/2/4/8/30/30 | 模拟 sshConnect 总失败 | ✓ Go test | `reconnect_test.go` |
| 单元 | Trigger 立即重试 + drop 多余 trigger | channel size=1 | ✓ Go test | `reconnect_test.go` |
| 单元 | exceededFastRetryBudget 60s 窗口 5 次 | 注入 time.Now mock | ✓ Go test | `reconnect_test.go` |
| 单元 | 三态 UX 渲染（1.5s / 8s / 30s 阈值） | 模拟 disconnectDuration | ✓ Go test | `reconnect_test.go` |
| 单元 | NO_COLOR=1 时去 ANSI escape | os.Setenv | ✓ Go test | `reconnect_test.go` |
| 单元 | input_buffer Connected 直传 | bytes.Buffer | ✓ Go test | `input_buffer_test.go` |
| 单元 | input_buffer Reconnecting 缓冲 + 灰色 echo | bytes.Buffer | ✓ Go test | `input_buffer_test.go` |
| 单元 | input_buffer ringBuf 4KB 满溢 + WARN | 注入 4097 字节 | ✓ Go test | `input_buffer_test.go` |
| 单元 | input_buffer Enter 触发 onEnter | 注入 "\r" | ✓ Go test | `input_buffer_test.go` |
| 单元 | input_buffer 非 TTY 直传 | term.IsTerminal 模拟 | ✓ Go test | `input_buffer_test.go` |
| 单元 | session 命名 hash / short_id / anon 退化 | 纯函数 | ✓ Go test | `session_test.go` |
| 单元 | session 命名长度 ≤ 32 / 字符集 | 纯函数 | ✓ Go test | `session_test.go` |
| 单元 | DetectTmux 各种失败路径 | mock conn.NewSession | ✓ Go test | `session_test.go` |
| 单元 | tmux list-clients 解析（含活跃时间渲染） | string parse | ✓ Go test | `session_test.go` |
| 单元 | take-over 决策（0 client / 1+ client） | mock conn | ✓ Go test | `session_test.go` |
| 单元 | AcquireSyncLock 拿到 / 拿不到（exit 99） | mock conn 返回 ExitError 99 | ✓ Go test | `sync_lock_test.go` |
| 单元 | AcquireSyncLock anon 路径直接 noop | accountID="" | ✓ Go test | `sync_lock_test.go` |
| 单元 | errcodes registry 10 条新码均合法 | TestRegistry | ✓ Go test | `errcodes/codes_test.go` |
| 单元 | last-session.json 含 tmux_session / client_role | round-trip JSON | ✓ Go test | `last_session_test.go` |
| 集成 | docker compose fixture 双 cloud-claude 同 account | 沿用 Phase 31 Plan 03 fixture + 第二个 cloud-claude service | ✓ Go integration test | `integration_test.go` |
| 集成 | docker network disconnect/connect 30s 抖动 | 同 fixture + docker network commands | ✓ Go integration test | `integration_test.go` |
| 集成 | pkill -SIGHUP sshd 后 tmux 存活（C7） | docker exec | ✓ Go integration test | `integration_test.go` |
| 集成 | pgrep systemd-logind 应无结果 | docker exec | ✓ Go integration test | `integration_test.go` |
| UAT | 真机拔网 30s（BASE-03 前置） | Phase 35 真机 | ✗ 手工 / Phase 35 | — |
| UAT | 真机两端 attach + take-over 体感 | Phase 35 真机 | ✗ 手工 / Phase 35 | — |

**自动化覆盖率目标**：单元 + 集成必须能在 CI 跑（docker-in-docker + Go test）；真机 UAT 留 Phase 35。

**集成测试 fixture 复用**：Phase 31 Plan 03 已有 `docker-compose.test.yml` + workspace 镜像（`internal/cloudclaude/integration_test.go` 已用 `t.Skip` 留 C3 真实 netem 场景给 Phase 35）。本阶段在同 fixture 加 1 个 service `cloud-claude-second`（同 account 不同 cwd）即可覆盖多端用例。

---

## Open Questions (RESOLVED)

下列 6 条 Q 在 Phase 32 plan 切分阶段全部拍板；保留题面便于追溯，每条尾部追加 **RESOLVED** 决议指向具体 plan / task 落地点。

### Q1（§5.2）：tmux client 识别策略

CONTEXT D-12 设想用 tmux 环境变量识别 client name；实测 tmux 没有 per-client 名 API。三个候选：

- (a) **文件注册表 `/workspace/.cloud-claude/clients/<pid>.json`**（推荐）
- (b) tmux user-options `@cloud-claude-client-<pid>`
- (c) 退化为 `<未知来源>`

**planner 决策建议**：(a) — 与既有 `/workspace/.cloud-claude/` 目录一致，attach/detach 时清理简单。

**RESOLVED**：Plan 02 Task 2.1a 采用 (a) 文件注册表完整方案 — 落地 `writeClientFile` / `removeClientFile` / `readClientHostnames` 三个 helper；attach 后远程 `tmux display-message -p '#{client_pid}'` 取本端 client_pid 写入 `/workspace/.cloud-claude/clients/<pid>.json`，runClaudeWithSession defer 远程 `rm -f` 清理；banner 渲染时通过 `readClientHostnames` 批量 cat JSON 文件，缺失则 fallback `unknown-host`。完整 schema 见 Plan 02 `<remote_command_templates>`。

### Q2（§6.2）：单例锁路径

CONTEXT D-17 用 `/var/lock/cloud-claude/`；UID 1000 写不进。三个候选：

- (a) `/run/cloud-claude/`（需改 Phase 29 entrypoint，**违反** D-25）
- (b) `/workspace/.cloud-claude/locks/`（污染 mergerfs，需改 Phase 31 默认 ignore）
- (c) **`/tmp/cloud-claude/locks/`**（推荐，0 侵入其它 phase）

**planner 决策建议**：(c) — 与 Phase 29/31 完全解耦。

**RESOLVED**：Plan 03 Task 3.1 采用 `/tmp/cloud-claude/locks/sync-<account_id>.lock`；Plan 03 acceptance criteria 包含 `! rg -q "/var/lock" internal/cloudclaude/sync_lock.go` 反向校验防止回归；mode 1777 + UID 1000 写权限验证由集成测试 TestIntegration_Phase32_SyncLockMutexes 覆盖。

### Q3（§6.4）：SyncSessionLock 注入位置

Phase 31 D-31 接口 `func(accountID string) (release func(), err error)` 没传 conn。本阶段在 `ssh.go ConnectAndRunClaudeV3` 内覆盖此字段（拿到 connA 后再赋值），还是把接口签名改为 `func(conn *ssh.Client, accountID string) ...`？

**planner 决策建议**：在 ssh.go 内覆盖 — 不破坏 Phase 31 接口契约（Phase 31 接口不变），实现细节内聚。

**RESOLVED**：Plan 03 Task 3.2 在 `ssh.go::ConnectAndRunClaudeV3` 内 connA 建立后、MountWorkspace 调用前覆盖 `mountCfg.SyncSessionLock = func(accountID string) (func(), error) { return AcquireSyncLock(connA, accountID) }`；Phase 31 接口签名 zero diff。

### Q4（§3.4）：reconnect 三态 UX 渲染的并发安全

`renderStatus` goroutine 与 PTY raw mode 下用户输入并发写 stdout。如何避免渲染 escape 序列覆盖用户键入字符？

**planner 决策建议**：渲染只在"用户长时间没动作"时才输出（结合 input_buffer 的 idle 检测）；或者干脆只渲染在另一行用 `\x1b[s\x1b[<row>;0H...\x1b[u`（保存光标 + 移到底行 + 恢复）。本阶段 v0 版本可以接受偶尔覆盖（与 Phase 31 D-28 "近似"决策同等容忍度）。

**RESOLVED**：Plan 01 Task 1.3 通过 `Reconnector.RegisterStateListener(fn func(ConnState))` 回调驱动 BufferedStdin 状态切换 — `BufferedStdin.SetReconnector(r *Reconnector)` 一行注入即可注册 listener，避免裸共享 `*atomic.Int32` 指针；renderStatus 与用户输入并发覆盖问题沿用 Phase 31 D-28 "近似"容忍策略（v0 偶尔覆盖可接受，完美 PTY 渲染留 v3.1）。

### Q5（§4.5）：ringBuf 容量

CONTEXT 默认 4KB，Discretion 允许 8KB。粘贴大段代码场景下 4KB 不够。**planner 决策建议**：默认 8KB，超过则 SESSION_BUFFER_OVERFLOW 警告。

**RESOLVED**：Plan 01 Task 1.3 选定 4096 字节（`const RingBufCapacity = 4096`）— 与 CONTEXT 原值对齐，避免增大隐含期望"无损粘贴"；超过 4KB 触发 SESSION_BUFFER_OVERFLOW + 丢最早 1KB；如真机 UAT（Phase 35）发现普通粘贴场景频繁 overflow，再调到 8KB（一行 const 改动）。

### Q6（§7.1）：PID 1 是 sshd 还是 tini？

ROADMAP §Phase 29 SC5 文字写 PID 1 = tini，但 entrypoint.sh 最后一行 `exec /usr/sbin/sshd -D -e` 让 PID 1 = sshd。本阶段的 C7 防御**不依赖** PID 1 是哪个（只需无 systemd-logind）。**planner 不需要解决**，但回归用例里 `ps -p 1 -o comm=` 应接受 sshd 或 tini 任一。

**RESOLVED**：由 Phase 29 D-10 已锁定为 tini（v3.0 镜像 final entrypoint 是 `exec tini -- /usr/sbin/sshd -D -e`），本阶段无需重决策。Plan 03 Task 3.3 集成测试 TestIntegration_Phase32_PgrepNoSystemdLogind 仅断言 `pgrep systemd-logind` 无输出（C7 真正防御点），**不**断言 PID 1 是何进程，与 Phase 29 D-10 决策解耦。

---

## Sources

### Primary（HIGH confidence）

- [VERIFIED] `internal/cloudclaude/ssh.go` — sshConnect / runClaude / ConnectAndRunClaudeV3 当前实现
- [VERIFIED] `internal/cloudclaude/mount_strategy.go` — MountConfig 字段（KeepAliveInterval / KeepAliveCountMax / SyncSessionLock 已预留）
- [VERIFIED] `internal/cloudclaude/errcodes/codes.go` — 命名正则 + Registry / Format helper
- [VERIFIED] `internal/cloudclaude/errcodes/{mount,net}.go` — Phase 31 既有注册模式
- [VERIFIED] `internal/cloudclaude/last_session.go` — schema_version=1 + omitempty 模式
- [VERIFIED] `internal/cloudclaude/sshfs_watcher.go` — Phase 31 watcher（C3 防御）
- [VERIFIED] `internal/cloudclaude/colors.go` — ANSI 颜色 + colorize helper + NO_COLOR
- [VERIFIED] `cmd/cloud-claude/main.go` — cobra 注册 + DisableFlagParsing 模式
- [VERIFIED] `cmd/cloud-claude/sync.go` — Phase 31 子命令注册参考
- [VERIFIED] `deploy/docker/managed-user/sshd_config` — ClientAliveInterval 15 / CountMax 8 / MaxSessions 30
- [VERIFIED] `deploy/docker/managed-user/entrypoint.sh` — assert_tmux_version + 无 systemd
- [VERIFIED] `.planning/phases/31-cli/31-RESEARCH.md` §1 — Mutagen 通信模型与 conn-C 模式
- [VERIFIED] golang.org/x/crypto/ssh package source — SendRequest 阻塞语义、Conn.Close 触发 io.EOF

### Secondary（MEDIUM confidence — 训练数据 + 通用文档知识）

- [CITED] OpenSSH PROTOCOL.md `keepalive@openssh.com` 全局请求名约定
- [CITED] tmux 3.4 man page — `new-session -A` / `detach-client -a` / `list-clients` FORMATS / `set-option @user-options`
- [CITED] tcp(7) Linux man — TCP_USER_TIMEOUT 自 2.6.37 引入
- [CITED] Darwin `/usr/include/netinet/tcp.h` — TCP_KEEPALIVE = 0x10
- [CITED] util-linux flock(1) man — `-n` / `-E` / `-F` 选项语义
- [CITED] mergerfs README — branches xattr 协议
- [CITED] sshfs man — `-o reconnect` 选项

### Tertiary（LOW / ASSUMED — 推断未独立验证，planner 实施时如有反例需校正）

- [ASSUMED] `SendRequest("keepalive@openssh.com", true, nil)` 在 conn open 但网络停滞时无限阻塞 — 推断自源码与 OpenSSH 行为，本阶段必须包 timeout
- [ASSUMED] util-linux 2.39.x 在 ubuntu:24.04 默认安装且支持 -F 标志 — 实施前可在容器内 `flock --version` 确认
- [ASSUMED] tmux client 没有运行时改名 API — 训练数据 + man page 检索；如发现 tmux 3.5+ 引入相关命令，§5.2 可简化

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — 所有库 / 命令均在 Phase 29-31 已实测使用
- KeepAlive / TCP setsockopt 实现: HIGH — Go x/crypto/ssh + syscall API 文档完整
- 重连状态机: HIGH — 状态机本身朴素，关键点是 ctx 协同与 mount 层不动
- input_buffer ANSI 渲染: MEDIUM — 中文宽字符场景已知会偶尔错位（CONTEXT 接受）
- tmux 多端 attach: MEDIUM — `new-session -A` / `detach-client -a` 行为明确，但 client 识别需 §5.2 fallback
- 单例锁: MEDIUM — flock -F 语义明确，但容器内路径需要从 `/var/lock` 调到 `/tmp`（§6.2 推荐 (c)）
- 错误码: HIGH — 完全沿用 Phase 31 模式
- 测试矩阵: HIGH — 集成测试 fixture 已 ship（Phase 31 Plan 03）

**Research date:** 2026-04-20
**Valid until:** 2026-05-20（Go x/crypto/ssh 与 tmux 3.4+ 都是稳定接口；30 天内基本不会过时）

---

## RESEARCH COMPLETE

**Phase:** 32 - SSH 会话可靠性 + tmux 包装 + 多端
**Confidence:** HIGH（核心接口 + 既有代码）/ MEDIUM（tmux client 识别 + 单例锁路径需 planner 拍板）

### Key Findings
- CONTEXT.md 已对所有 gray area 决策；本研究**确认 D-03/D-04/D-05/D-10/D-11/D-15/D-16/D-22/D-23/D-28/D-29 全部可行**，无需调整
- **修订 D-12** — tmux 没有 per-client 名 API，§5.2 提出三个 fallback（推荐 (a) 文件注册表）
- **修订 D-17** — `/var/lock` 在 ubuntu:24.04 是 root-only，§6.2 推荐改用 `/tmp/cloud-claude/locks/`（0 侵入其它 phase）
- **澄清 D-18** — Phase 31 `MountConfig.SyncSessionLock` 接口没传 conn，需在 `ssh.go ConnectAndRunClaudeV3` 内部覆盖此字段（不改 Phase 31 接口）
- SendRequest 必须包 timeout（goroutine + `select <-time.After`）— 否则 dead network 会让 keepalive 永久阻塞而无法触发失败计数

### File Created
`.planning/phases/32-ssh-tmux/32-RESEARCH.md`

### Confidence Assessment
| 区域 | 等级 | 理由 |
|------|------|------|
| Standard Stack | HIGH | 所有库 / 命令在 Phase 29-31 实测使用 |
| Architecture | HIGH | CONTEXT.md 决策详尽，本阶段补技术验证而非选型 |
| Pitfalls | HIGH | C3 / C7 / M11-M15 防御路径明确（多数沿用 Phase 29/31 已落 mitigation） |
| tmux 多端识别 | MEDIUM | §5.2 三个 fallback 需 planner 拍板 (a)/(b)/(c) |
| 单例锁路径 | MEDIUM | §6.2 路径需从 /var/lock 调到 /tmp |

### Open Questions（不阻塞 planner，留 plan 切分时拍板）
1. tmux client 识别 fallback 策略（推荐 (a) 文件注册表）
2. 单例锁路径（推荐 (c) `/tmp/cloud-claude/locks/`）
3. SyncSessionLock 注入位置（推荐 ssh.go 覆盖，不改 Phase 31 接口）
4. reconnect 三态 UX 与用户输入的渲染并发（建议 v0 容忍偶尔覆盖）
5. ringBuf 容量（建议默认 8KB）
6. PID 1 = sshd vs tini（不阻塞，回归用例接受任一）

### Ready for Planning
研究完成。planner 可基于本文件 + CONTEXT.md 创建 PLAN.md（建议 3 plan 切分：Plan 01 = keepalive + reconnect + input_buffer + errcodes 注册；Plan 02 = session.go + tmux 包装 + sessions 子命令；Plan 03 = sync_lock + 集成测试 + C3/C7 回归）。
