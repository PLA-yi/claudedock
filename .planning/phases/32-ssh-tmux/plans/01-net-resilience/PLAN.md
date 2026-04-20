---
phase: 32
plan: 01-net-resilience
type: execute
wave: 1
depends_on: []
autonomous: true
requirements:
  - REQ-F3-A
  - REQ-F3-B
  - REQ-F3-C
  - REQ-F3-D
files_modified:
  - internal/cloudclaude/keepalive.go
  - internal/cloudclaude/keepalive_linux.go
  - internal/cloudclaude/keepalive_darwin.go
  - internal/cloudclaude/keepalive_other.go
  - internal/cloudclaude/keepalive_test.go
  - internal/cloudclaude/reconnect.go
  - internal/cloudclaude/reconnect_test.go
  - internal/cloudclaude/input_buffer.go
  - internal/cloudclaude/input_buffer_test.go
  - internal/cloudclaude/errcodes/codes.go
  - internal/cloudclaude/errcodes/session.go
  - internal/cloudclaude/errcodes/net.go
  - internal/cloudclaude/colors.go
  - internal/cloudclaude/last_session.go
  - internal/cloudclaude/last_session_test.go
  - internal/cloudclaude/ssh.go
must_haves:
  truths:
    - "客户端 keepalive_interval < 15s 时 cloud-claude 启动期 stderr 输出 [SESSION_KEEPALIVE_TOO_AGGRESSIVE] + 退出 ExitConfigError(=4)（REQ-F3-A / PITFALLS M11）"
    - "sshConnect 拨号成功后立即对 *net.TCPConn 启用 SO_KEEPALIVE + SetKeepAlivePeriod(15s)；Linux 追加 TCP_USER_TIMEOUT=30000、macOS 追加 TCP_KEEPALIVE=15；其它平台 stderr 输出 [NET_TCP_KEEPALIVE_UNSUPPORTED] 警告但不阻塞（D-04）"
    - "keepalive.RunKeepAlive(ctx, conn, interval, countMax) goroutine 每 interval 发一次 SendRequest(\"keepalive@openssh.com\", true, nil)；单次调用必须 goroutine + select <-time.After(interval) 包 timeout（dead network 防永久阻塞，RESEARCH §1.2）；连续 countMax 次失败后 conn.Close() + return（让 reconnect 感知 io.EOF）"
    - "reconnect.Reconnector.Run(ctx) 退避序列硬编码 [1s,2s,4s,8s,30s]，30s 后维持 30s 周期；每次重连调 sshConnect 复用启动期已缓存 SSHConfig.Password，过程不弹密码（REQ-F3-D）"
    - "reconnect.Trigger() channel size=1 + drop 多余 trigger；fastRetry 60s 滑动窗口 5 次仍失败 → 返回 errReconnectGaveUp（NET_RECONNECT_GAVE_UP）"
    - "reconnect.renderStatus 100ms ticker：disconnectDuration < 1.5s 不渲染 / >=1.5s 灰色 … N.Ns / >=8s 黄色 ⚠ 网络抖动中（N 秒未响应）/ >=30s 红色 ✗ 网络已断 N 秒，正在自动重试…；NO_COLOR=1 时去掉 ANSI escape（REQ-F3-C / D-22 / D-23）"
    - "input_buffer.BufferedStdin 把 os.Stdin 用 io.Pipe 中转：state==Connected 直写 pipeW；state==Reconnecting 进入 4KB（默认；planner 可调到 8KB）ringBuf + 灰色 ANSI echo 到 localEcho；ringBuf 满 → 丢最早 1KB + [SESSION_BUFFER_OVERFLOW] warning；非 TTY（!term.IsTerminal）→ 直接 os.Stdin → ssh.Session.Stdin 不启用 buffer（D-06 第 5 条）"
    - "input_buffer 检测到 \\r 或 \\n → 调用 onEnter() 触发 reconnect.Trigger()（D-05 第 4 条 + D-06）"
    - "errcodes 包追加 10 条新 Code 常量（7 SESSION_* + 3 NET_*）+ session.go 与 net.go 第二个 init() 注册；codes_test.go 现有遍历测试（NextAction ≤ 80 runes / 名称正则 / 无重复）必须全 pass"
    - "colors.go 追加 ansiGray=\\033[90m 常量；reconnect / input_buffer 通过 colorize(text, ansiGray, colorEnabled(noColor, w)) 输出，禁止裸写 ANSI"
    - "last_session.go LastSessionSnapshot 追加 TmuxSession / ClientRole / ReconnectCount 三字段全部 omitempty，schema_version 保持 1（D-27 / SP-07）"
    - "ssh.go::sshConnect 在 net.DialTimeout 后、ssh.NewClientConn 前插入 ConfigureTCPKeepAlive(tc, 15*time.Second) 调用（best-effort，错误仅打 NET_TCP_KEEPALIVE_UNSUPPORTED warning 不 return error）"
  artifacts:
    - path: "internal/cloudclaude/keepalive.go"
      provides: "RunKeepAlive(ctx, conn, interval, countMax) + sendKeepaliveWithTimeout + ConfigureTCPKeepAlive(*net.TCPConn, time.Duration) + 公共 configurePlatformSpecific 接口声明"
      contains: "func RunKeepAlive"
    - path: "internal/cloudclaude/keepalive_linux.go"
      provides: "//go:build linux configurePlatformSpecific 实现：setsockopt(IPPROTO_TCP, 18 /*TCP_USER_TIMEOUT*/, 30000)"
      contains: "//go:build linux"
    - path: "internal/cloudclaude/keepalive_darwin.go"
      provides: "//go:build darwin configurePlatformSpecific 实现：setsockopt(IPPROTO_TCP, 0x10 /*TCP_KEEPALIVE*/, 15)"
      contains: "//go:build darwin"
    - path: "internal/cloudclaude/keepalive_other.go"
      provides: "//go:build !linux && !darwin configurePlatformSpecific noop + NET_TCP_KEEPALIVE_UNSUPPORTED warning"
      contains: "//go:build !linux && !darwin"
    - path: "internal/cloudclaude/reconnect.go"
      provides: "Reconnector struct + Run(ctx) 退避循环 + Trigger() + renderStatus + ConnState 枚举 + backoffSeq[]time.Duration"
      contains: "type Reconnector struct"
    - path: "internal/cloudclaude/input_buffer.go"
      provides: "BufferedStdin{src, pipeW, ringBuf, state, localEcho, noColor, onEnter} + NewBufferedStdin(src, localEcho, noColor, onEnter) (*BufferedStdin, io.Reader) + Run(ctx) + Flush()"
      contains: "type BufferedStdin struct"
    - path: "internal/cloudclaude/errcodes/session.go"
      provides: "init() MustRegister 7 条 SESSION_* 错误码（Message / NextAction 与 RESEARCH §8 行 1056-1092 逐字符对齐）"
      contains: "SESSION_KEEPALIVE_TOO_AGGRESSIVE"
    - path: "internal/cloudclaude/errcodes/codes.go"
      provides: "追加 10 条 Code 常量（7 SESSION_* + 3 NET_*），与字面值完全相同（grep 友好）"
      contains: "SESSION_BUFFER_OVERFLOW"
    - path: "internal/cloudclaude/errcodes/net.go"
      provides: "第二个 init() 追加 3 条 NET_RECONNECT_* / NET_TCP_KEEPALIVE_UNSUPPORTED（不动现有 NET_OAUTH_* 注册块）"
      contains: "NET_RECONNECT_BACKOFF"
    - path: "internal/cloudclaude/colors.go"
      provides: "追加 ansiGray = \\033[90m 常量"
      contains: "ansiGray"
    - path: "internal/cloudclaude/last_session.go"
      provides: "LastSessionSnapshot 追加 TmuxSession / ClientRole / ReconnectCount 三字段，全部 omitempty"
      contains: "TmuxSession"
    - path: "internal/cloudclaude/ssh.go"
      provides: "sshConnect 在 DialTimeout 后插入 ConfigureTCPKeepAlive 调用"
      contains: "ConfigureTCPKeepAlive"
  key_links:
    - from: "ssh.go::sshConnect"
      to: "keepalive.go::ConfigureTCPKeepAlive"
      via: "TCP 拨号成功后立即调用，best-effort"
      pattern: "ConfigureTCPKeepAlive"
    - from: "reconnect.go::Reconnector"
      to: "ssh.go::sshConnect"
      via: "复用 SSHConfig 重新 dial（不弹密码）"
      pattern: "sshConnect"
    - from: "input_buffer.go::BufferedStdin"
      to: "reconnect.go::Reconnector.Trigger"
      via: "检测到 \\r/\\n → onEnter() → Trigger()"
      pattern: "onEnter"
    - from: "errcodes/session.go init"
      to: "errcodes/codes.go MustRegister"
      via: "包级 init 自动注册到全局 Registry"
      pattern: "MustRegister"
---

<plan_dependencies>
- 本 plan 是 Wave 1，**无 plan 依赖**。
- 本 plan 必须**先完成并 commit**：Plan 02 / Plan 03 都依赖：
  - errcodes 包已注册的 SESSION_* / NET_RECONNECT_* / NET_TCP_KEEPALIVE_UNSUPPORTED 常量
  - colors.go 的 ansiGray 常量
  - last_session.go 的 TmuxSession / ClientRole / ReconnectCount 字段
  - reconnect.Reconnector / input_buffer.BufferedStdin（Plan 02 runClaudeWithSession 包装时调用）
  - keepalive.RunKeepAlive（Plan 02 ConnectAndRunClaudeV3 在 conn 建立后启动该 goroutine）
- 本 plan 不改 ssh.go::ConnectAndRunClaudeV3 / runClaude（仅在 sshConnect 内部插一行）— 避免与 Plan 02 抢 ssh.go 的同函数。
</plan_dependencies>

<objective>
落地 Phase 32 网络层韧性基线：客户端 SSH KeepAlive（应用层 SendRequest + TCP 层平台特化 setsockopt）+ 自实现重连状态机（退避 1/2/4/8/30s + 不弹密码 + 三态 UX 渲染）+ 本地输入缓冲（断网期间灰色未确认 ANSI echo + 重连后按序提交）+ 全部 10 条新错误码注册。

Purpose: 兑现 ROADMAP §Phase 32 Success Criteria 第 1 / 4 / 5 / 6 条（KeepAlive 启动期校验 / 退避序列与不弹密码 / 灰色未确认渲染 / 重连失败 prompt 两要素），并为 Plan 02 提供 reconnect.Reconnector + input_buffer.BufferedStdin + keepalive.RunKeepAlive 三个可复用 service。
Output: 7 个新文件 + 6 个改造文件；零新增依赖；构建产物在 Linux / macOS / 其它平台均可编译。
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/execute-plan.md
@.cursor/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/32-ssh-tmux/32-CONTEXT.md
@.planning/phases/32-ssh-tmux/32-RESEARCH.md
@.planning/phases/32-ssh-tmux/32-PATTERNS.md
@.planning/phases/29-v3-worker/29-CONTEXT.md
@.planning/phases/30-entry-api/30-CONTEXT.md
@.planning/phases/31-cli/31-CONTEXT.md
@internal/cloudclaude/ssh.go
@internal/cloudclaude/sshfs_watcher.go
@internal/cloudclaude/colors.go
@internal/cloudclaude/last_session.go
@internal/cloudclaude/exitcodes.go
@internal/cloudclaude/errcodes/codes.go
@internal/cloudclaude/errcodes/mount.go
@internal/cloudclaude/errcodes/net.go
@internal/cloudclaude/errcodes/codes_test.go
@internal/cloudclaude/mount_strategy.go
@internal/cloudclaude/mount.go

<interfaces>
<!-- 本 plan 创建的对外 API（Plan 02 / Plan 03 直接消费）。 -->

internal/cloudclaude/keepalive.go 导出：

```go
package cloudclaude

import (
    "context"
    "net"
    "time"
    "golang.org/x/crypto/ssh"
)

// RunKeepAlive 在 conn 上每 interval 发一次 SSH 全局 keepalive 请求；
// 连续 countMax 次失败（含单次 timeout）后 conn.Close() 让上层 reconnect 感知。
// interval 必须 >= 15s（启动期已校验；本函数仍做 defensive check）。
// 单次 SendRequest 必须包 timeout（goroutine + select <-time.After(interval)），
// 否则 dead network 上 SendRequest 永久阻塞、失败计数永远不增长（RESEARCH §1.2 [ASSUMED]）。
func RunKeepAlive(ctx context.Context, conn ssh.Conn, interval time.Duration, countMax int) error

// ConfigureTCPKeepAlive 在 sshConnect 拨号成功后立即调用：
//  1. tcpConn.SetKeepAlive(true)
//  2. tcpConn.SetKeepAlivePeriod(period)（period=15s）
//  3. configurePlatformSpecific(tcpConn) — Linux 设 TCP_USER_TIMEOUT=30000ms / macOS 设 TCP_KEEPALIVE=15s / 其它 noop+warning
// 任一步骤失败仅返回 error，调用方 stderr 打 NET_TCP_KEEPALIVE_UNSUPPORTED 警告，**不阻塞**连接建立（D-04 第 4 条）。
func ConfigureTCPKeepAlive(tcpConn *net.TCPConn, period time.Duration) error
```

internal/cloudclaude/reconnect.go 导出：

```go
package cloudclaude

import (
    "context"
    "errors"
    "io"
    "sync/atomic"
    "time"
    "golang.org/x/crypto/ssh"
)

type ConnState int32
const (
    StateConnected ConnState = iota
    StateReconnecting
    StateGaveUp
)

type Reconnector struct {
    cfg            SSHConfig
    onConnLost     func()
    onReconnected  func(*ssh.Client) error
    triggerCh      chan struct{}
    state          atomic.Int32
    disconnectStart atomic.Int64
    fastRetryCount  int
    fastRetryWindow time.Time
    noColor         bool
    statusWriter    io.Writer
}

// NewReconnector 用启动期 SSHConfig（含已缓存 password）+ 三个回调构造。
// statusWriter 用于三态 UX 渲染（通常是 os.Stderr）；noColor 控制 ANSI escape 是否输出。
func NewReconnector(cfg SSHConfig, onConnLost func(), onReconnected func(*ssh.Client) error,
    statusWriter io.Writer, noColor bool) *Reconnector

// Run 阻塞执行重连循环；正常重连成功 return nil；fastRetry 兜底失败 return ErrReconnectGaveUp。
// 内部分两个 goroutine：
//  1. 主 goroutine：select{ ctx.Done | timer.C | triggerCh } + sshConnect retry
//  2. renderStatus goroutine：100ms ticker，按 disconnectStart 评估三态 UX 文本写 statusWriter
func (r *Reconnector) Run(ctx context.Context) error

// Trigger 由 input_buffer 在用户按 \r/\n 时调用，立即唤醒 select；channel 满则丢弃（防 spam）。
func (r *Reconnector) Trigger()

// State 返回当前连接状态（input_buffer 用其判定走 Connected 直传还是 Reconnecting 缓冲分支）。
func (r *Reconnector) State() ConnState

var ErrReconnectGaveUp = errors.New("reconnect gave up after fast-retry budget exceeded")
```

internal/cloudclaude/input_buffer.go 导出：

```go
package cloudclaude

import (
    "context"
    "io"
    "sync"
    "sync/atomic"
)

type BufferedStdin struct {
    src       io.Reader
    pipeW     io.WriteCloser
    state     *atomic.Int32 // 共享 reconnect.Reconnector 的 state（指针）
    ringBuf   []byte
    ringMu    sync.Mutex
    localEcho io.Writer
    noColor   bool
    onEnter   func()
    grayOpen  bool // 是否已 echo 过开头 \x1b[90m
}

// NewBufferedStdin 用 io.Pipe 拿到 (pipeR, pipeW)；返回的 io.Reader 直接喂给 ssh.Session.Stdin。
// state 指针必须由调用方共享给 reconnect.Reconnector（同一 atomic.Int32）。
// localEcho 是 os.Stdout（断网期间灰色未确认渲染目标）。
// onEnter 在 state==Reconnecting 且检测到 \r/\n 时调用（通常 = reconnect.Trigger）。
func NewBufferedStdin(src io.Reader, state *atomic.Int32, localEcho io.Writer,
    noColor bool, onEnter func()) (*BufferedStdin, io.Reader)

// Run 阻塞读 src 字节；按 state 分发：
//   StateConnected     → 直写 pipeW
//   StateReconnecting  → ringBuf append + 灰色 echo localEcho + Enter 触发 onEnter
//   StateGaveUp        → 丢弃
func (b *BufferedStdin) Run(ctx context.Context) error

// Flush 在 reconnect 成功的 onReconnected 回调中调用：把 ringBuf 内容按序写 pipeW + 清空 + echo \x1b[0m 关闭灰色。
func (b *BufferedStdin) Flush() error

// 默认环形缓冲容量；planner 已选 4096（可按 RESEARCH §4.5 调到 8192）。
const RingBufCapacity = 4096
```

internal/cloudclaude/last_session.go LastSessionSnapshot 字段（追加，omitempty）：

```go
TmuxSession    string `json:"tmux_session,omitempty"`     // Plan 02 写入
ClientRole     string `json:"client_role,omitempty"`      // "primary" | "secondary"，Plan 03 写入
ReconnectCount int    `json:"reconnect_count,omitempty"`  // Reconnector 每次成功重连 +1
```

internal/cloudclaude/colors.go 追加常量：

```go
ansiGray = "\033[90m" // [Phase 32 D-22 / D-23] reconnect 灰色 / input_buffer 未确认字符
```

internal/cloudclaude/errcodes/codes.go 追加 10 条常量：

```go
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
```

注：本 plan 注册的 7 条 SESSION_* 中只有 KEEPALIVE/BUFFER 两条由本 plan 真正调用；其余 5 条（TMUX_UNAVAILABLE / NOT_FOUND / TAKEOVER_NOTIFIED / TAKEOVER_FAILED / SYNC_LOCKED）由 Plan 02 / Plan 03 调用，但**全部在本 plan 一次性注册到 errcodes/session.go**，避免分散。
</interfaces>

<errcode_registry>
<!-- session.go 7 条 + net.go 追加 3 条 — Message/NextAction 直接复制 RESEARCH §8 表，禁止改动文案。 -->

errcodes/session.go：

```go
package errcodes

// SESSION_* 错误码注册（Phase 32）。文案与 32-RESEARCH.md §8 行 1056-1092 逐字符对齐。
//nolint:lll

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
```

errcodes/net.go 末尾追加第二个 init()（不动现有 NET_OAUTH_* 注册块）：

```go
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
</errcode_registry>
</context>

<tasks>

<task type="auto">
  <name>Task 1.1: errcodes 常量 + session.go 注册 + net.go 追加（无依赖，先行）</name>
  <files>
    internal/cloudclaude/errcodes/codes.go
    internal/cloudclaude/errcodes/session.go
    internal/cloudclaude/errcodes/net.go
  </files>
  <read_first>
    - internal/cloudclaude/errcodes/codes.go（看 const 块行号 119-136 + 命名正则 line 56 + Format helper line 99-117）
    - internal/cloudclaude/errcodes/mount.go（mirror 模板：init() + MustRegister 写法）
    - internal/cloudclaude/errcodes/net.go（看 NET_OAUTH_* init 块；本 task 在末尾追加第二个 init()，不破坏既有块）
    - internal/cloudclaude/errcodes/codes_test.go（line 20 命名正则、line 43 NextAction ≤ 80 runes、整表唯一性 — 本 task 提交后必须 pass）
    - .planning/phases/32-ssh-tmux/32-RESEARCH.md §8（行 1056-1130 的 10 条完整文案；planner 禁止自创 Message/NextAction）
    - .planning/phases/32-ssh-tmux/32-PATTERNS.md SP-06（init+MustRegister 严格 mirror；Code 必须匹配正则 ^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$；NextAction ≤ 80 runes）
    - .planning/phases/32-ssh-tmux/32-CONTEXT.md D-20 / D-21（10 条码表 + 不引入新 Format helper）
  </read_first>
  <action>
    1. internal/cloudclaude/errcodes/codes.go：在 const 块的 NET_OAUTH_NOT_FOUND 之后**追加 10 行**（顺序与上方 <interfaces> 块完全一致）；保持列对齐（Code 字面值与变量名完全相同，便于 grep）。

    2. internal/cloudclaude/errcodes/session.go（新文件）：完全 mirror errcodes/mount.go 写法 — package errcodes / 顶部注释 `// SESSION_* 错误码注册（Phase 32）。文案与 32-RESEARCH.md §8 行 1056-1092 逐字符对齐。` + `//nolint:lll` 行 + 单个 init() 内 7 个 MustRegister。Message / NextAction 必须**逐字符**复制上方 <errcode_registry> 块（不要"优化"措辞）。

    3. internal/cloudclaude/errcodes/net.go：**不修改**现有 init()（含 NET_OAUTH_* 三条）；在文件末尾**追加第二个 func init()** 注册 NET_RECONNECT_BACKOFF / NET_RECONNECT_GAVE_UP / NET_TCP_KEEPALIVE_UNSUPPORTED（Go 允许同包同文件多 init；与 mount.go 单 init 模式不同是为了不污染 OAuth 注册块）。

    4. **不**改 errcodes/codes.go 的命名正则 / 不**新增** Severity 等级（Info/Warn/Error/Fatal 已足够覆盖）。

    5. NextAction ≤ 80 runes 自检：`SESSION_KEEPALIVE_TOO_AGGRESSIVE` 的 NextAction = "调整 keepalive_interval 至 >= 15s，或移除该配置使用默认值"（≈ 28 runes，OK）；其余 9 条均已在 RESEARCH 段验证 ≤ 80。
  </action>
  <acceptance_criteria>
    - `go build ./internal/cloudclaude/errcodes/...` 成功
    - `go test ./internal/cloudclaude/errcodes/... -run TestRegistry -count=1` PASS（codes_test.go 现有遍历测试覆盖唯一性 + NextAction ≤ 80 runes + 命名正则）
    - `rg -n "SESSION_KEEPALIVE_TOO_AGGRESSIVE" internal/cloudclaude/errcodes/` 至少 2 次命中（codes.go 常量 + session.go 注册）
    - `rg -n "SESSION_BUFFER_OVERFLOW" internal/cloudclaude/errcodes/` ≥ 2 次命中
    - `rg -n "NET_RECONNECT_GAVE_UP" internal/cloudclaude/errcodes/` ≥ 2 次命中
    - `rg -n "NET_TCP_KEEPALIVE_UNSUPPORTED" internal/cloudclaude/errcodes/` ≥ 2 次命中
    - `rg -c "func init" internal/cloudclaude/errcodes/net.go` 输出 = 2（追加第二个 init 不破坏第一个）
    - `go vet ./internal/cloudclaude/errcodes/...` 通过
    - `errcodes.Format(errcodes.SESSION_TMUX_UNAVAILABLE, "tmux: command not found")` 在 REPL/test 中返回字符串以 `[SESSION_TMUX_UNAVAILABLE]` 开头并含中文 `建议:` 段（结构由 Format helper 保证；本 task 不需新增 Format 测试）
  </acceptance_criteria>
  <verify>
    <automated>go test ./internal/cloudclaude/errcodes/... -count=1 &amp;&amp; go vet ./internal/cloudclaude/errcodes/...</automated>
  </verify>
  <done>10 条 Code 常量 + 10 条 init MustRegister 全部就位；errcodes 包测试 PASS；下游 plan 可直接 import 这些常量。</done>
</task>

<task type="auto">
  <name>Task 1.2: keepalive.go + 三平台 build-tag 文件 + ssh.go::sshConnect 接入 + 单测</name>
  <files>
    internal/cloudclaude/keepalive.go
    internal/cloudclaude/keepalive_linux.go
    internal/cloudclaude/keepalive_darwin.go
    internal/cloudclaude/keepalive_other.go
    internal/cloudclaude/keepalive_test.go
    internal/cloudclaude/ssh.go
    internal/cloudclaude/colors.go
  </files>
  <read_first>
    - internal/cloudclaude/ssh.go（sshConnect line 144-166 改造点；不要改 runClaude / ConnectAndRunClaudeV3）
    - internal/cloudclaude/sshfs_watcher.go（Run 主循环 line 51-76 — keepalive Run 同构）
    - internal/cloudclaude/colors.go（行 9-16 const 块；本 task 仅追加 ansiGray 一行）
    - internal/cloudclaude/mount_strategy.go（line 346-351 — go watcher.Run(ctx) 启动模式参考；本 plan 不在 mount_strategy 启动 keepalive，由 Plan 02 在 ConnectAndRunClaudeV3 启动）
    - internal/cloudclaude/errcodes/net.go（NET_TCP_KEEPALIVE_UNSUPPORTED 注册位置）
    - internal/cloudclaude/mutagen_bin.go line 40-44（runtime.GOOS switch + 平台不支持 fallthrough 范式）
    - .planning/phases/32-ssh-tmux/32-RESEARCH.md §1.1-1.3（SendRequest 阻塞语义 + sendKeepaliveWithTimeout 模板）+ §2.1-2.5（TCP setsockopt 平台模板 + sshConnect 接入）
    - .planning/phases/32-ssh-tmux/32-PATTERNS.md keepalive.go / keepalive_*.go 段（exact 模板复用）
    - .planning/phases/32-ssh-tmux/32-CONTEXT.md D-03 / D-04 / D-23
  </read_first>
  <action>
    1. **internal/cloudclaude/keepalive.go**（公共部分）：

       ```go
       package cloudclaude

       import (
           "context"
           "errors"
           "net"
           "time"
           "golang.org/x/crypto/ssh"
       )

       // RunKeepAlive — 见 <interfaces> 块文档注释。
       func RunKeepAlive(ctx context.Context, conn ssh.Conn, interval time.Duration, countMax int) error {
           if interval < 15*time.Second {
               return errors.New("keepalive interval 必须 >= 15s")
           }
           if countMax <= 0 {
               countMax = 4
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
                       _ = conn.Close()
                       return err
                   }
               }
           }
       }

       func sendKeepaliveWithTimeout(conn ssh.Conn, timeout time.Duration) (bool, error) {
           type result struct{ ok bool; err error }
           ch := make(chan result, 1)
           go func() {
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

       // ConfigureTCPKeepAlive — 见 <interfaces> 块文档注释。
       func ConfigureTCPKeepAlive(tcpConn *net.TCPConn, period time.Duration) error {
           if err := tcpConn.SetKeepAlive(true); err != nil {
               return err
           }
           if err := tcpConn.SetKeepAlivePeriod(period); err != nil {
               return err
           }
           return configurePlatformSpecific(tcpConn)
       }
       ```

    2. **internal/cloudclaude/keepalive_linux.go**：

       ```go
       //go:build linux

       package cloudclaude

       import (
           "net"
           "syscall"
       )

       // tcpUserTimeout = TCP_USER_TIMEOUT；syscall pkg 不导出此常量。RFC 793 + Linux 2.6.37+ tcp(7) 定义为 18。
       const tcpUserTimeout = 18

       func configurePlatformSpecific(tcpConn *net.TCPConn) error {
           rawConn, err := tcpConn.SyscallConn()
           if err != nil {
               return err
           }
           var sockErr error
           if cErr := rawConn.Control(func(fd uintptr) {
               // 30000ms：与 ROADMAP §Phase 32 SC2 30s 抖动验收对齐；客户端先于服务端 (8×15s=120s) 宣告"断"
               sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpUserTimeout, 30000)
           }); cErr != nil {
               return cErr
           }
           return sockErr
       }
       ```

    3. **internal/cloudclaude/keepalive_darwin.go**：

       ```go
       //go:build darwin

       package cloudclaude

       import (
           "net"
           "syscall"
       )

       // tcpKeepalive = Darwin <netinet/tcp.h> TCP_KEEPALIVE 0x10。
       // Go stdlib SetKeepAlivePeriod 在 darwin 已经走该 sockopt（行为重复但保留 hook 方便日后加 TCP_KEEPCNT/INTVL）。
       const tcpKeepalive = 0x10

       func configurePlatformSpecific(tcpConn *net.TCPConn) error {
           rawConn, err := tcpConn.SyscallConn()
           if err != nil {
               return err
           }
           var sockErr error
           if cErr := rawConn.Control(func(fd uintptr) {
               sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpKeepalive, 15)
           }); cErr != nil {
               return cErr
           }
           return sockErr
       }
       ```

    4. **internal/cloudclaude/keepalive_other.go**：

       ```go
       //go:build !linux && !darwin

       package cloudclaude

       import (
           "fmt"
           "net"
           "os"
           "runtime"

           "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
       )

       func configurePlatformSpecific(tcpConn *net.TCPConn) error {
           // SetKeepAlive + SetKeepAlivePeriod 已经被公共 ConfigureTCPKeepAlive 调用过（stdlib 跨平台生效）。
           // 平台特化优化跳过；输出 warning 但不阻塞（CONTEXT D-04 第 4 条 best-effort）。
           fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.NET_TCP_KEEPALIVE_UNSUPPORTED, runtime.GOOS))
           _ = tcpConn // suppress unused
           return nil
       }
       ```

       **import path 注意**：用 `github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes` — 与 PATTERNS §keepalive_other.go 引用一致（与 mount_mutagen.go / errcodes/mount.go 现有 import 完全相同）。

    5. **internal/cloudclaude/colors.go**：在 const 块末尾追加一行：

       ```go
       ansiGray   = "\033[90m"  // [Phase 32 D-23] reconnect "..." / input_buffer 未确认字符
       ```

    6. **internal/cloudclaude/ssh.go**（**仅** sshConnect 内部插入；不动 ConnectAndRunClaudeV3 / runClaude）：

       在 line 155-160 区间，把：

       ```go
       tcpConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
       if err != nil {
           return nil, fmt.Errorf("SSH 连接失败（无法连接 %s）: %w", addr, err)
       }

       sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientCfg)
       ```

       改为：

       ```go
       tcpConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
       if err != nil {
           return nil, fmt.Errorf("SSH 连接失败（无法连接 %s）: %w", addr, err)
       }

       // [Phase 32 D-04] TCP keepalive — best-effort，失败仅 warning 不阻塞
       if tc, ok := tcpConn.(*net.TCPConn); ok {
           if e := ConfigureTCPKeepAlive(tc, 15*time.Second); e != nil {
               fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.NET_TCP_KEEPALIVE_UNSUPPORTED, e.Error()))
           }
       }

       sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientCfg)
       ```

       注意：errcodes 包应该已在 ssh.go import（Phase 31 已用）；如未 import 则补上 `"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"` 与 `"os"`（os 通常已 import — 检查）。

    7. **internal/cloudclaude/keepalive_test.go**（单测，至少 4 个用例）：

       ```go
       package cloudclaude

       import (
           "context"
           "errors"
           "net"
           "testing"
           "time"
           "golang.org/x/crypto/ssh"
       )

       // fakeConn 实现 ssh.Conn 接口，按测试需要决定 SendRequest 的返回。
       type fakeConn struct {
           ssh.Conn
           sendDelay  time.Duration
           sendErr    error
           closeCalled bool
       }
       func (f *fakeConn) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
           if f.sendDelay > 0 { time.Sleep(f.sendDelay) }
           return f.sendErr == nil, nil, f.sendErr
       }
       func (f *fakeConn) Close() error { f.closeCalled = true; return nil }

       func TestRunKeepAlive_RejectsTooShortInterval(t *testing.T) {
           err := RunKeepAlive(context.Background(), &fakeConn{}, 5*time.Second, 4)
           if err == nil { t.Fatal("期望 interval<15s 返回错误") }
       }

       func TestRunKeepAlive_TimeoutCounts(t *testing.T) {
           // sendDelay 远大于 interval timeout — 每次都超时，countMax 后关 conn
           f := &fakeConn{sendDelay: 30 * time.Second}
           ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
           defer cancel()
           // 用 15s interval / countMax=2 → 应在 ~30s 内返回（2 次超时）
           done := make(chan error, 1)
           go func() { done <- RunKeepAlive(ctx, f, 15*time.Second, 2) }()
           select {
           case err := <-done:
               if !f.closeCalled { t.Error("期望 countMax 触发后关闭 conn") }
               _ = err
           case <-time.After(60*time.Second):
               t.Fatal("RunKeepAlive 未在预期时间内返回")
           }
       }

       func TestRunKeepAlive_SuccessResetsFails(t *testing.T) {
           // 用 nil sendErr → SendRequest 立即成功；ctx cancel 后退出
           f := &fakeConn{sendErr: nil}
           ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
           defer cancel()
           // interval 15s + ctx 200ms → 没有 ticker 触发就 ctx.Done，无 close 调用
           if err := RunKeepAlive(ctx, f, 15*time.Second, 4); !errors.Is(err, context.DeadlineExceeded) {
               t.Fatalf("期望 ctx.DeadlineExceeded，得 %v", err)
           }
           if f.closeCalled { t.Error("ctx 取消不应关闭 conn") }
       }

       func TestConfigureTCPKeepAlive_NoPanicOnTCPConn(t *testing.T) {
           // 起一个临时 listener，accept 后拿到 *net.TCPConn 验证 ConfigureTCPKeepAlive 不 panic
           ln, err := net.Listen("tcp", "127.0.0.1:0")
           if err != nil { t.Fatal(err) }
           defer ln.Close()
           go func() { c, _ := ln.Accept(); if c != nil { c.Close() } }()
           c, err := net.Dial("tcp", ln.Addr().String())
           if err != nil { t.Fatal(err) }
           defer c.Close()
           tc := c.(*net.TCPConn)
           if err := ConfigureTCPKeepAlive(tc, 15*time.Second); err != nil {
               t.Logf("ConfigureTCPKeepAlive 平台返回错误（接受 — best-effort）: %v", err)
           }
       }
       ```

    8. **不**修改 ssh.go 的其它函数；ConnectAndRunClaudeV3 内启动 RunKeepAlive 的 goroutine 由 Plan 02 完成（避免 wave 1 与 wave 2 抢同函数）。
  </action>
  <acceptance_criteria>
    - `go build ./...` 在 darwin / linux 均成功（CI 即可验证）
    - `GOOS=linux GOARCH=amd64 go vet ./internal/cloudclaude/...` 通过（验证 keepalive_linux.go build tag 正确）
    - `GOOS=darwin GOARCH=arm64 go vet ./internal/cloudclaude/...` 通过
    - `GOOS=windows GOARCH=amd64 go build ./internal/cloudclaude/...` 通过（验证 keepalive_other.go fallback；Windows cloud-claude 可能未测试但不应破坏编译）
    - `go test ./internal/cloudclaude/... -run TestRunKeepAlive -count=1` 4 个用例 PASS
    - `go test ./internal/cloudclaude/... -run TestConfigureTCPKeepAlive -count=1` PASS
    - `rg -n "func RunKeepAlive" internal/cloudclaude/keepalive.go` 命中 1 行
    - `rg -n "func sendKeepaliveWithTimeout" internal/cloudclaude/keepalive.go` 命中 1 行（必须有 timeout 包装）
    - `rg -n "func ConfigureTCPKeepAlive" internal/cloudclaude/keepalive.go` 命中 1 行
    - `rg -n "//go:build linux" internal/cloudclaude/keepalive_linux.go` 命中 1 行
    - `rg -n "//go:build darwin" internal/cloudclaude/keepalive_darwin.go` 命中 1 行
    - `rg -n "//go:build !linux && !darwin" internal/cloudclaude/keepalive_other.go` 命中 1 行
    - `rg -n "tcpUserTimeout" internal/cloudclaude/keepalive_linux.go` 命中（值 = 18）
    - `rg -n "ConfigureTCPKeepAlive" internal/cloudclaude/ssh.go` 命中 1 行（在 sshConnect 内）
    - `rg -n "ansiGray" internal/cloudclaude/colors.go` 命中 1 行
    - `rg -n "TCP_USER_TIMEOUT" internal/cloudclaude/keepalive_linux.go` 命中（注释中说明常量含义，可选）
  </acceptance_criteria>
  <verify>
    <automated>go test ./internal/cloudclaude/... -run "TestRunKeepAlive|TestConfigureTCPKeepAlive" -count=1 &amp;&amp; GOOS=linux go vet ./internal/cloudclaude/... &amp;&amp; GOOS=darwin go vet ./internal/cloudclaude/... &amp;&amp; GOOS=windows go build ./internal/cloudclaude/...</automated>
  </verify>
  <done>SSH KeepAlive 应用层 + TCP 平台特化 + sshConnect 接入完成；三平台 build tag 正确；ansiGray 常量就绪。Plan 02 可在 ConnectAndRunClaudeV3 内 `go RunKeepAlive(ctx, connA, mountCfg.KeepAliveInterval, mountCfg.KeepAliveCountMax)`。</done>
</task>

<task type="auto">
  <name>Task 1.3: reconnect.go + input_buffer.go + last_session.go 字段 + 单测</name>
  <files>
    internal/cloudclaude/reconnect.go
    internal/cloudclaude/reconnect_test.go
    internal/cloudclaude/input_buffer.go
    internal/cloudclaude/input_buffer_test.go
    internal/cloudclaude/last_session.go
    internal/cloudclaude/last_session_test.go
  </files>
  <read_first>
    - internal/cloudclaude/sshfs_watcher.go（line 51-76 — Run select{ctx,ticker} + 失败计数模板，reconnect.Run 同构）
    - internal/cloudclaude/colors.go（colorEnabled / colorize；Task 1.2 已加 ansiGray）
    - internal/cloudclaude/last_session.go（line 15-25 现有 struct；本 task 在 APFSCaseInsensitive 之后追加 3 字段）
    - internal/cloudclaude/last_session_test.go（看现有 round-trip JSON 测试模式 — 追加新字段的覆盖用例）
    - internal/cloudclaude/mount_strategy.go（line 382-393 printBanner — colorize + colorEnabled 用法参考）
    - internal/cloudclaude/mount.go（line 105-112 sshRun helper；本 task 不调远程命令，仅参考 ctx 模式）
    - internal/cloudclaude/exitcodes.go（ExitNetworkError=2 — Plan 02 在 ErrReconnectGaveUp 时退出此码；本 task 仅需暴露错误）
    - .planning/phases/32-ssh-tmux/32-RESEARCH.md §3 / §4（reconnect 状态机 + input_buffer 实现细节）
    - .planning/phases/32-ssh-tmux/32-PATTERNS.md reconnect.go / input_buffer.go 段
    - .planning/phases/32-ssh-tmux/32-CONTEXT.md D-05 / D-06 / D-22 / D-23 / D-27
  </read_first>
  <action>
    1. **internal/cloudclaude/reconnect.go**（关键骨架，详细行为见 RESEARCH §3）：

       ```go
       package cloudclaude

       import (
           "context"
           "errors"
           "fmt"
           "io"
           "sync/atomic"
           "time"
           "golang.org/x/crypto/ssh"

           "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
       )

       type ConnState int32

       const (
           StateConnected ConnState = iota
           StateReconnecting
           StateGaveUp
       )

       var ErrReconnectGaveUp = errors.New("reconnect gave up after fast-retry budget exceeded")

       // backoffSeq 是固定退避序列（CONTEXT D-05 第 1 条）；30s 后维持 30s 周期。
       var backoffSeq = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 30 * time.Second}

       type Reconnector struct {
           cfg             SSHConfig
           onConnLost      func()
           onReconnected   func(*ssh.Client) error
           triggerCh       chan struct{}
           state           atomic.Int32
           disconnectStart atomic.Int64
           fastRetryCount  int
           fastRetryWindow time.Time
           noColor         bool
           statusWriter    io.Writer
           reconnectCount  atomic.Int64
       }

       func NewReconnector(cfg SSHConfig, onConnLost func(), onReconnected func(*ssh.Client) error,
           statusWriter io.Writer, noColor bool) *Reconnector {
           r := &Reconnector{
               cfg:           cfg,
               onConnLost:    onConnLost,
               onReconnected: onReconnected,
               triggerCh:     make(chan struct{}, 1),
               noColor:       noColor,
               statusWriter:  statusWriter,
           }
           r.state.Store(int32(StateConnected))
           return r
       }

       func (r *Reconnector) State() ConnState { return ConnState(r.state.Load()) }

       func (r *Reconnector) Trigger() {
           select {
           case r.triggerCh <- struct{}{}:
           default: // drop（防 Enter spam）
           }
       }

       // ReconnectCount 暴露累计成功重连次数，用于 last-session.json ReconnectCount 字段写入。
       func (r *Reconnector) ReconnectCount() int { return int(r.reconnectCount.Load()) }

       // Run 阻塞执行重连循环；正常 return nil（重连成功），fastRetry 兜底返回 ErrReconnectGaveUp。
       // 调用者需要在原 conn 关闭被感知（io.EOF）后启动 Run；renderStatus goroutine 由 Run 内部 spawn。
       func (r *Reconnector) Run(ctx context.Context) error {
           r.disconnectStart.Store(time.Now().UnixNano())
           r.state.Store(int32(StateReconnecting))
           if r.onConnLost != nil { r.onConnLost() }

           // renderStatus 在独立 goroutine 中跑（每 100ms 评估）
           statusCtx, cancelStatus := context.WithCancel(ctx)
           defer cancelStatus()
           go r.renderStatus(statusCtx)

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
                   r.recordFastRetry()
                   if r.exceededFastRetryBudget() {
                       r.state.Store(int32(StateGaveUp))
                       return ErrReconnectGaveUp
                   }
               case <-timer.C:
                   // 自然退避到点
               }

               newConn, err := sshConnect(r.cfg)
               if err == nil {
                   if cbErr := r.onReconnected(newConn); cbErr == nil {
                       r.disconnectStart.Store(0)
                       r.reconnectCount.Add(1)
                       r.state.Store(int32(StateConnected))
                       return nil
                   }
                   _ = newConn.Close()
               }

               if backoffIdx < len(backoffSeq)-1 {
                   backoffIdx++
               }
           }
       }

       func (r *Reconnector) recordFastRetry() {
           now := time.Now()
           if r.fastRetryWindow.IsZero() || now.Sub(r.fastRetryWindow) > 60*time.Second {
               r.fastRetryWindow = now
               r.fastRetryCount = 1
               return
           }
           r.fastRetryCount++
       }

       func (r *Reconnector) exceededFastRetryBudget() bool {
           return r.fastRetryCount > 5
       }

       // renderStatus 每 100ms 评估 disconnectDuration → 输出三态文本（行内覆盖 \r\x1b[K）。
       func (r *Reconnector) renderStatus(ctx context.Context) {
           ticker := time.NewTicker(100 * time.Millisecond)
           defer ticker.Stop()
           lastRendered := ""
           for {
               select {
               case <-ctx.Done():
                   if r.statusWriter != nil && lastRendered != "" {
                       fmt.Fprint(r.statusWriter, "\r\x1b[K")
                   }
                   return
               case <-ticker.C:
                   startNs := r.disconnectStart.Load()
                   if startNs == 0 {
                       if lastRendered != "" {
                           fmt.Fprint(r.statusWriter, "\r\x1b[K")
                           lastRendered = ""
                       }
                       continue
                   }
                   elapsed := time.Duration(time.Now().UnixNano() - startNs)
                   text := renderDisconnectStatus(elapsed, r.noColor)
                   if text != lastRendered {
                       fmt.Fprint(r.statusWriter, "\r\x1b[K"+text)
                       lastRendered = text
                   }
               }
           }
       }

       // renderDisconnectStatus 是纯函数，方便单测直接断言（CONTEXT D-22 三态阈值）。
       // noColor=true → 不输出 ANSI escape，仅纯文本（D-23）。
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

       // FormatGiveUpMessage 由 Plan 02 在 Run 返回 ErrReconnectGaveUp 后调用，输出 NET_RECONNECT_GAVE_UP 错误码。
       func FormatGiveUpMessage(retries int, totalDuration time.Duration) string {
           return errcodes.Format(errcodes.NET_RECONNECT_GAVE_UP, retries, totalDuration.Round(time.Second))
       }
       ```

    2. **internal/cloudclaude/input_buffer.go**：

       ```go
       package cloudclaude

       import (
           "context"
           "errors"
           "fmt"
           "io"
           "sync"
           "sync/atomic"

           "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
       )

       const RingBufCapacity = 4096

       type BufferedStdin struct {
           src       io.Reader
           pipeW     io.WriteCloser
           state     *atomic.Int32 // 共享 Reconnector.state
           ringBuf   []byte
           ringMu    sync.Mutex
           localEcho io.Writer
           noColor   bool
           onEnter   func()
           grayOpen  bool
       }

       func NewBufferedStdin(src io.Reader, state *atomic.Int32, localEcho io.Writer,
           noColor bool, onEnter func()) (*BufferedStdin, io.Reader) {
           pr, pw := io.Pipe()
           b := &BufferedStdin{
               src:       src,
               pipeW:     pw,
               state:     state,
               ringBuf:   make([]byte, 0, RingBufCapacity),
               localEcho: localEcho,
               noColor:   noColor,
               onEnter:   onEnter,
           }
           return b, pr
       }

       func (b *BufferedStdin) Run(ctx context.Context) error {
           buf := make([]byte, 1)
           for {
               select {
               case <-ctx.Done():
                   return ctx.Err()
               default:
               }
               n, err := b.src.Read(buf)
               if n > 0 {
                   c := buf[0]
                   switch ConnState(b.state.Load()) {
                   case StateConnected:
                       b.closeGrayIfOpen()
                       if _, werr := b.pipeW.Write(buf[:n]); werr != nil {
                           return werr
                       }
                   case StateReconnecting:
                       b.handleReconnecting(c)
                   case StateGaveUp:
                       // 丢弃
                   }
               }
               if err != nil {
                   if errors.Is(err, io.EOF) {
                       return nil
                   }
                   return err
               }
           }
       }

       func (b *BufferedStdin) handleReconnecting(c byte) {
           b.ringMu.Lock()
           if len(b.ringBuf) >= RingBufCapacity {
               // 丢最早 1024 字节（CONTEXT D-06 第 2 条）
               drop := 1024
               if drop > len(b.ringBuf) { drop = len(b.ringBuf) }
               b.ringBuf = b.ringBuf[drop:]
               if b.localEcho != nil {
                   fmt.Fprintln(b.localEcho, errcodes.Format(errcodes.SESSION_BUFFER_OVERFLOW))
               }
           }
           b.ringBuf = append(b.ringBuf, c)
           b.ringMu.Unlock()

           if b.localEcho != nil {
               // 进入 Reconnecting 时一次性 echo \x1b[90m，退出时 \x1b[0m（不逐字节包；RESEARCH §4.3）
               if !b.grayOpen && !b.noColor {
                   fmt.Fprint(b.localEcho, ansiGray)
                   b.grayOpen = true
               }
               fmt.Fprintf(b.localEcho, "%c", c)
           }
           if c == '\r' || c == '\n' {
               if b.onEnter != nil { b.onEnter() }
           }
       }

       func (b *BufferedStdin) closeGrayIfOpen() {
           if b.grayOpen && b.localEcho != nil && !b.noColor {
               fmt.Fprint(b.localEcho, ansiReset)
               b.grayOpen = false
           }
       }

       // Flush 把 ringBuf 内容按序写 pipeW；reconnect 成功的 onReconnected 回调中调用。
       func (b *BufferedStdin) Flush() error {
           b.closeGrayIfOpen()
           b.ringMu.Lock()
           defer b.ringMu.Unlock()
           if len(b.ringBuf) == 0 {
               return nil
           }
           if _, err := b.pipeW.Write(b.ringBuf); err != nil {
               return err
           }
           b.ringBuf = b.ringBuf[:0]
           return nil
       }

       // Close 关闭底层 pipeW（cleanup 用）。
       func (b *BufferedStdin) Close() error {
           if pc, ok := b.pipeW.(io.Closer); ok {
               return pc.Close()
           }
           return nil
       }
       ```

    3. **internal/cloudclaude/last_session.go**：在 LastSessionSnapshot struct 末尾追加 3 个字段（保持 schema_version 常量不变）：

       ```go
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

           // [Phase 32 D-27 新增] 全部 omitempty + schema_version 保持 1（向后兼容）
           TmuxSession    string `json:"tmux_session,omitempty"`     // 实际 attach 的 tmux session 名（Plan 02 写）
           ClientRole     string `json:"client_role,omitempty"`      // "primary" | "secondary"（Plan 03 写）
           ReconnectCount int    `json:"reconnect_count,omitempty"`  // Reconnector.ReconnectCount() 累计值
       }
       ```

    4. **单测覆盖**（reconnect_test.go）：

       ```go
       func TestRenderDisconnectStatus_Thresholds(t *testing.T) {
           cases := []struct {
               name string; d time.Duration; noColor bool; wantContains string
           }{
               {"under_1.5s_empty", 500*time.Millisecond, false, ""},
               {"1.5s_to_8s_gray", 3*time.Second, false, "\x1b[90m"},
               {"8s_to_30s_yellow", 10*time.Second, false, "\x1b[33m"},
               {"over_30s_red", 45*time.Second, false, "\x1b[31m"},
               {"no_color_30s_no_escape", 45*time.Second, true, "网络已断"},
               {"no_color_30s_no_ansi", 45*time.Second, true, "✗"},
           }
           for _, tc := range cases {
               t.Run(tc.name, func(t *testing.T) {
                   got := renderDisconnectStatus(tc.d, tc.noColor)
                   if tc.wantContains == "" {
                       if got != "" { t.Errorf("期望空字符串，得 %q", got) }
                       return
                   }
                   if !strings.Contains(got, tc.wantContains) {
                       t.Errorf("renderDisconnectStatus(%v, %v) = %q, want contain %q", tc.d, tc.noColor, got, tc.wantContains)
                   }
                   if tc.noColor && strings.Contains(got, "\x1b[") {
                       t.Errorf("noColor=true 不应含 ANSI escape，得 %q", got)
                   }
               })
           }
       }

       func TestReconnector_TriggerDropsExtras(t *testing.T) {
           r := NewReconnector(SSHConfig{}, nil, nil, nil, true)
           // 连续 trigger 100 次；第二次开始全部应被 drop（channel size=1）
           for i := 0; i < 100; i++ { r.Trigger() }
           // 现在 channel 里应只有 1 个（drain 一次后再 select 应阻塞）
           if len(r.triggerCh) != 1 {
               t.Errorf("期望 triggerCh 长度 = 1，得 %d", len(r.triggerCh))
           }
       }

       func TestReconnector_ExceededFastRetryBudget_60sWindow(t *testing.T) {
           r := NewReconnector(SSHConfig{}, nil, nil, nil, true)
           // 模拟 60s 窗口内 5 次 fastRetry → 第 6 次应触发兜底
           for i := 0; i < 5; i++ {
               r.recordFastRetry()
               if r.exceededFastRetryBudget() {
                   t.Fatalf("第 %d 次不应触发兜底", i+1)
               }
           }
           r.recordFastRetry()
           if !r.exceededFastRetryBudget() {
               t.Fatal("第 6 次应触发 fastRetry 兜底")
           }
       }

       func TestReconnector_FastRetryWindowResetsAfter60s(t *testing.T) {
           r := NewReconnector(SSHConfig{}, nil, nil, nil, true)
           r.recordFastRetry()
           r.fastRetryWindow = time.Now().Add(-65 * time.Second)
           r.recordFastRetry()
           if r.fastRetryCount != 1 {
               t.Errorf("窗口重置后期望 fastRetryCount=1，得 %d", r.fastRetryCount)
           }
       }

       func TestBackoffSeq(t *testing.T) {
           expected := []time.Duration{1*time.Second, 2*time.Second, 4*time.Second, 8*time.Second, 30*time.Second}
           if len(backoffSeq) != len(expected) {
               t.Fatalf("期望 backoffSeq 长度 %d，得 %d", len(expected), len(backoffSeq))
           }
           for i := range expected {
               if backoffSeq[i] != expected[i] {
                   t.Errorf("backoffSeq[%d] = %v, want %v", i, backoffSeq[i], expected[i])
               }
           }
       }
       ```

    5. **单测覆盖**（input_buffer_test.go）：

       ```go
       func TestBufferedStdin_ConnectedDirectWrite(t *testing.T) {
           src := strings.NewReader("hello")
           var state atomic.Int32
           state.Store(int32(StateConnected))
           var echo bytes.Buffer
           b, pipeR := NewBufferedStdin(src, &state, &echo, true, nil)
           done := make(chan error, 1)
           go func() { done <- b.Run(context.Background()) }()
           got, _ := io.ReadAll(pipeR)
           if !bytes.HasPrefix(got, []byte("hello")) {
               t.Errorf("期望 pipeR 读到 hello，得 %q", got)
           }
       }

       func TestBufferedStdin_ReconnectingBuffersAndGrayEchoes(t *testing.T) {
           src := strings.NewReader("ab")
           var state atomic.Int32
           state.Store(int32(StateReconnecting))
           var echo bytes.Buffer
           b, _ := NewBufferedStdin(src, &state, &echo, false, nil)
           ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
           defer cancel()
           _ = b.Run(ctx)
           // 灰色 escape 应至少出现 1 次（开头）
           if !bytes.Contains(echo.Bytes(), []byte(ansiGray)) {
               t.Error("Reconnecting 状态应灰色 echo")
           }
           // ringBuf 应含 "ab"
           if string(b.ringBuf) != "ab" {
               t.Errorf("ringBuf = %q, want \"ab\"", string(b.ringBuf))
           }
       }

       func TestBufferedStdin_RingBufOverflowDropsAndWarns(t *testing.T) {
           // 灌 4097 字节 → 应触发 SESSION_BUFFER_OVERFLOW + 丢前 1KB
           data := bytes.Repeat([]byte("x"), 4097)
           src := bytes.NewReader(data)
           var state atomic.Int32
           state.Store(int32(StateReconnecting))
           var echo bytes.Buffer
           b, _ := NewBufferedStdin(src, &state, &echo, true, nil)
           ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
           defer cancel()
           _ = b.Run(ctx)
           if !bytes.Contains(echo.Bytes(), []byte("SESSION_BUFFER_OVERFLOW")) {
               t.Error("应输出 SESSION_BUFFER_OVERFLOW warning")
           }
       }

       func TestBufferedStdin_EnterTriggersOnEnter(t *testing.T) {
           src := strings.NewReader("hi\r")
           var state atomic.Int32
           state.Store(int32(StateReconnecting))
           triggered := atomic.Int32{}
           b, _ := NewBufferedStdin(src, &state, io.Discard, true, func() { triggered.Add(1) })
           ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
           defer cancel()
           _ = b.Run(ctx)
           if triggered.Load() < 1 {
               t.Error("\\r 应触发 onEnter")
           }
       }

       func TestBufferedStdin_FlushClearsBuffer(t *testing.T) {
           src := strings.NewReader("xyz")
           var state atomic.Int32
           state.Store(int32(StateReconnecting))
           b, pipeR := NewBufferedStdin(src, &state, io.Discard, true, nil)
           ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
           defer cancel()
           _ = b.Run(ctx)
           done := make(chan []byte, 1)
           go func() { buf, _ := io.ReadAll(pipeR); done <- buf }()
           if err := b.Flush(); err != nil {
               t.Fatal(err)
           }
           _ = b.Close()
           select {
           case got := <-done:
               if !bytes.Contains(got, []byte("xyz")) {
                   t.Errorf("Flush 后 pipeR 应有 xyz，得 %q", got)
               }
           case <-time.After(time.Second):
               t.Fatal("flush 后 pipeR 未读到数据")
           }
           if len(b.ringBuf) != 0 {
               t.Errorf("Flush 后 ringBuf 应清空，得长度 %d", len(b.ringBuf))
           }
       }
       ```

    6. **单测覆盖**（last_session_test.go 追加）：在现有 round-trip 测试后追加：

       ```go
       func TestLastSessionSnapshot_NewFieldsRoundTrip(t *testing.T) {
           snap := LastSessionSnapshot{
               SchemaVersion:  1,
               Timestamp:      time.Now().UTC(),
               IntendedMode:   "auto",
               ActualMode:     "full",
               TmuxSession:    "claude-abc12345",
               ClientRole:     "primary",
               ReconnectCount: 3,
           }
           data, err := json.Marshal(snap)
           if err != nil { t.Fatal(err) }
           if !bytes.Contains(data, []byte(`"tmux_session":"claude-abc12345"`)) {
               t.Errorf("序列化丢失 tmux_session: %s", data)
           }
           if !bytes.Contains(data, []byte(`"client_role":"primary"`)) {
               t.Errorf("序列化丢失 client_role: %s", data)
           }
           if !bytes.Contains(data, []byte(`"reconnect_count":3`)) {
               t.Errorf("序列化丢失 reconnect_count: %s", data)
           }
           var back LastSessionSnapshot
           if err := json.Unmarshal(data, &back); err != nil { t.Fatal(err) }
           if back.TmuxSession != "claude-abc12345" || back.ClientRole != "primary" || back.ReconnectCount != 3 {
               t.Errorf("反序列化字段丢失: %+v", back)
           }
       }

       func TestLastSessionSnapshot_OmitemptyForEmpty(t *testing.T) {
           snap := LastSessionSnapshot{SchemaVersion: 1}
           data, _ := json.Marshal(snap)
           // 三个新字段空时不应出现在 JSON 中
           for _, key := range []string{"tmux_session", "client_role", "reconnect_count"} {
               if bytes.Contains(data, []byte(`"`+key+`"`)) {
                   t.Errorf("空字段 %s 应被 omitempty 隐藏: %s", key, data)
               }
           }
       }
       ```

    7. **不**在 reconnect.go 中调用 Plan 02 才有的 SessionConfig / SyncSessionLock — 本 plan 仅暴露接口；启动 Reconnector 的位置由 Plan 02 在 ConnectAndRunClaudeV3 内决定。
  </action>
  <acceptance_criteria>
    - `go build ./internal/cloudclaude/...` 成功
    - `go vet ./internal/cloudclaude/...` 通过
    - `go test ./internal/cloudclaude/... -run "TestRenderDisconnectStatus|TestReconnector|TestBackoffSeq|TestBufferedStdin|TestLastSessionSnapshot_NewFields|TestLastSessionSnapshot_Omitempty" -count=1` 全部 PASS
    - `rg -n "var backoffSeq" internal/cloudclaude/reconnect.go` 命中；序列严格 = `[1s,2s,4s,8s,30s]`（test 已断言）
    - `rg -n "ErrReconnectGaveUp" internal/cloudclaude/reconnect.go` 命中
    - `rg -n "func \(r \*Reconnector\) Trigger" internal/cloudclaude/reconnect.go` 命中
    - `rg -n "func renderDisconnectStatus" internal/cloudclaude/reconnect.go` 命中（且为非导出函数）
    - `rg -n "RingBufCapacity" internal/cloudclaude/input_buffer.go` 命中
    - `rg -n "TmuxSession\s+string" internal/cloudclaude/last_session.go` 命中
    - `rg -n "ClientRole\s+string" internal/cloudclaude/last_session.go` 命中
    - `rg -n "ReconnectCount\s+int" internal/cloudclaude/last_session.go` 命中
    - `rg -n "schema_version" internal/cloudclaude/last_session.go` 验证 SchemaVersion 仍 = 1（不应被改）
    - `rg -n "json:\"tmux_session,omitempty\"" internal/cloudclaude/last_session.go` 命中（omitempty 必须）
    - `rg -n "json:\"client_role,omitempty\"" internal/cloudclaude/last_session.go` 命中
    - `rg -n "json:\"reconnect_count,omitempty\"" internal/cloudclaude/last_session.go` 命中
  </acceptance_criteria>
  <verify>
    <automated>go test ./internal/cloudclaude/... -run "TestRenderDisconnectStatus|TestReconnector|TestBackoffSeq|TestBufferedStdin|TestLastSessionSnapshot" -count=1 &amp;&amp; go vet ./internal/cloudclaude/...</automated>
  </verify>
  <done>reconnect.Reconnector + input_buffer.BufferedStdin + last_session 三新字段全部就位，单测 PASS。Plan 02 可在 runClaudeWithSession 内：(a) NewReconnector(...)；(b) NewBufferedStdin(os.Stdin, &reconnector.state(待暴露), os.Stdout, noColor, reconnector.Trigger)；(c) reconnector.Run(ctx) 在 io.EOF 后启动；(d) onReconnected 回调中 bufferedStdin.Flush() + 重新挂 RunKeepAlive。</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| 客户端 stdin → SSH session | 用户键入字符经 input_buffer 中转写入 SSH session.Stdin（PTY raw 模式下逐字节透传） |
| 客户端 → SSH 服务端 | KeepAlive SendRequest 全局请求穿越网络；TCP setsockopt 修改本地 socket 选项 |
| reconnect → SSH 重新拨号 | sshConnect 复用启动期 SSHConfig.Password 重新认证（不弹密码即不写新数据到 stdin） |
| 错误码 stderr 输出 | errcodes.Format 输出含用户提供的字符串参数（如 keepalive 间隔、TCP 错误信息） |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-32-01 | Tampering | input_buffer.go ringBuf | mitigate | ringBuf 是进程内 byte slice，无序列化/反序列化路径；4KB 上限防 DoS（无限缓冲耗内存）；溢出时丢最早 1KB 而不是丢最新（用户最近的输入更重要） |
| T-32-02 | Information Disclosure | reconnect.go SSHConfig.Password 复用 | accept | 启动期密码已在内存（v2.0 行为）；reconnect 不写日志、不输出到 stderr，仅传给 sshConnect → ssh.ClientConfig；进程退出后自然回收。M11 / 真机 swap 风险 v2.0 已接受 |
| T-32-03 | DoS | keepalive.go SendRequest spam | mitigate | RunKeepAlive 启动期校验 interval ≥ 15s（REQ-F3-A 强约束 + SESSION_KEEPALIVE_TOO_AGGRESSIVE）；防御 PITFALLS M11（客户端配置过激引发 sshd MaxStartups 雪崩） |
| T-32-04 | DoS | reconnect.go Trigger spam（用户狂按 Enter） | mitigate | triggerCh size=1 + drop；fastRetry 60s 窗口 5 次封顶（NET_RECONNECT_GAVE_UP）；不会无限 sshConnect 拨号 |
| T-32-05 | Tampering | errcodes.Format(%s) 注入 | accept | 用户提供的字符串（keepalive interval / TCP 错误）会进入 stderr；ANSI escape 注入风险存在但对端是用户自己的终端（不通过任何信任边界外发），且 PITFALLS C8 已通过 errcodes 注册表保证 Code 命名空间正交 |
| T-32-06 | DoS | input_buffer.go 大量粘贴时灰色 echo 占用 stdout | accept | RingBuf 满后 SESSION_BUFFER_OVERFLOW 警告 + 丢最早 1KB；最坏情况 4KB stdout 写入，对终端显示无害 |
| T-32-07 | Spoofing | 重连时 HostKeyCallback = InsecureIgnoreHostKey | accept | 沿用 v2.0 既有行为（CONTEXT D-05 第 2 条 sshConnect 复用）；本 plan 不引入新的 host key 验证回退；等待全局加固（v3.1 候选） |

**Severity 分布（block_on=high）**：
- High: 0 项（本 plan 网络层韧性不引入新的高危信任边界）
- Medium: T-32-01 / T-32-03 / T-32-04（已 mitigate；防御 M11）
- Low: T-32-02 / T-32-05 / T-32-06 / T-32-07（accept；与 v2.0 风险等级持平）

无 High 项 → 不阻断 plan 执行（满足 block_on=high 设定）。
</threat_model>

<verification>
本 plan 完成时，可断言下列 ROADMAP §Phase 32 Success Criteria 子集（其余 Plan 02 / Plan 03 验收）：

1. **SC1（REQ-F3-A）**：执行 `KEEPALIVE_INTERVAL_OVERRIDE=10s ./cloud-claude`（或等价 mock 调用 `RunKeepAlive(ctx, conn, 10*time.Second, 4)`）必须立即返回 `errors.New("keepalive interval 必须 >= 15s")`；同时 `errcodes.Format(SESSION_KEEPALIVE_TOO_AGGRESSIVE, "10s")` 必须包含「[SESSION_KEEPALIVE_TOO_AGGRESSIVE]」与中文「建议:」段
2. **SC4（REQ-F3-D 退避序列与不弹密码）**：单测 `TestBackoffSeq` PASS（断言 `backoffSeq == [1s,2s,4s,8s,30s]`）；reconnect.Run 内部对 sshConnect 的调用复用 SSHConfig.Password（grep `r.cfg` / `sshConnect(r.cfg)` 命中），过程不读 stdin / 不调 askpass
3. **SC5（REQ-F3-B 灰色未确认 echo + 重连后按序提交）**：单测 `TestBufferedStdin_ReconnectingBuffersAndGrayEchoes` PASS（echo 含 ansiGray）+ `TestBufferedStdin_FlushClearsBuffer` PASS（Flush 后 pipeR 读到 ringBuf 内容）
4. **SC6（REQ-F3-C 重连失败 prompt 两要素）**：`errcodes.Format(NET_RECONNECT_GAVE_UP, 5, 60*time.Second)` 必须同时包含中文原因（"重连失败"）与中文下一步（"请检查网络后重新运行 cloud-claude，或运行 cloud-claude doctor 诊断"）
5. **三态 UX 阈值（D-22）**：单测 `TestRenderDisconnectStatus_Thresholds` 6 个用例 PASS（< 1.5s 空 / 1.5-8s 灰 / 8-30s 黄 / > 30s 红 + NO_COLOR 去 ANSI）

**全 plan 综合 verify 命令**：

```bash
go build ./... \
  && GOOS=linux go vet ./internal/cloudclaude/... \
  && GOOS=darwin go vet ./internal/cloudclaude/... \
  && GOOS=windows go build ./internal/cloudclaude/... \
  && go test ./internal/cloudclaude/... -run "TestRunKeepAlive|TestConfigureTCPKeepAlive|TestRenderDisconnectStatus|TestReconnector|TestBackoffSeq|TestBufferedStdin|TestLastSessionSnapshot" -count=1 \
  && go test ./internal/cloudclaude/errcodes/... -count=1
```
</verification>

<success_criteria>
- 7 个新文件 + 6 个改造文件全部就位且 git status clean（提交后）
- 所有新增单测 PASS（≥ 15 个用例）
- 全平台编译通过：linux/amd64 / darwin/arm64 / windows/amd64 三组 `go build ./internal/cloudclaude/...`
- errcodes 包测试 PASS（10 条新码全部注册 + 唯一 + 命名合法 + NextAction ≤ 80 runes）
- ssh.go 改动仅在 sshConnect 内插入一段 TCP keepalive 配置 + import errcodes/os；其它函数 zero diff（用 `git diff internal/cloudclaude/ssh.go` 复核）
- last_session.go 改动仅追加 3 个 omitempty 字段；SchemaVersion 仍 = 1
- colors.go 改动仅追加 1 行 ansiGray 常量；既有 5 个常量 zero diff
- 不引入任何新 go.mod 依赖（`go mod tidy` 后 diff 为空）
</success_criteria>

<output>
After completion, create `.planning/phases/32-ssh-tmux/32-01-SUMMARY.md` 描述：
- RunKeepAlive / ConfigureTCPKeepAlive / Reconnector / BufferedStdin 四个 service 的实际签名（与 plan 中 <interfaces> 块对照）
- 10 条 errcodes 注册位置（codes.go 行号 + session.go / net.go 行号）
- 3 平台 build-tag 文件的 setsockopt 常量值（实际编码与 RESEARCH §2 对照）
- 单测覆盖率（go test -cover 输出）
- Plan 02 / Plan 03 的接入点提示
</output>
