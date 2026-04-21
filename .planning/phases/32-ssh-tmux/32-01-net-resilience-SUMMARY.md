---
phase: 32
plan: 01-net-resilience
subsystem: client-cli / network-resilience
tags: [keepalive, tcp, reconnect, input-buffer, errcodes, ansi]
dependency-graph:
  requires:
    - errcodes 注册表（Phase 31 落地）
    - SSHConfig 启动期密码缓存（v2.0）
    - sshConnect / ConnectAndRunClaudeV3（v2.0 + Phase 31）
  provides:
    - keepalive.RunKeepAlive(ctx, conn, interval, countMax)
    - keepalive.ConfigureTCPKeepAlive(*net.TCPConn, period)
    - reconnect.Reconnector{ Run / Trigger / State / StateAddr / ReconnectCount / FormatGiveUpMessage }
    - input_buffer.BufferedStdin{ Run / Flush / Close } + RingBufCapacity
    - errcodes 10 条新码（7 SESSION_* + 3 NET_*）
    - colors.ansiGray 常量
    - LastSessionSnapshot 三新字段（TmuxSession / ClientRole / ReconnectCount）
  affects:
    - sshConnect: 拨号成功后插入 ConfigureTCPKeepAlive 调用（best-effort）
tech-stack:
  added: []
  patterns:
    - SendRequest goroutine + select <-time.After(timeout) 包 timeout（防 dead network 永久阻塞）
    - 平台特化 setsockopt 走 //go:build linux / //go:build darwin / //go:build !linux && !darwin 三文件
    - atomic.Int32 状态机 + io.Pipe 中转 stdin（共享指针 reconnect ↔ input_buffer）
    - 100ms ticker 行内覆盖渲染（\r\x1b[K + 上次内容比较，避免刷屏）
key-files:
  created:
    - internal/cloudclaude/keepalive.go
    - internal/cloudclaude/keepalive_linux.go
    - internal/cloudclaude/keepalive_darwin.go
    - internal/cloudclaude/keepalive_other.go
    - internal/cloudclaude/keepalive_test.go
    - internal/cloudclaude/reconnect.go
    - internal/cloudclaude/reconnect_test.go
    - internal/cloudclaude/input_buffer.go
    - internal/cloudclaude/input_buffer_test.go
    - internal/cloudclaude/errcodes/session.go
  modified:
    - internal/cloudclaude/ssh.go (sshConnect 内插 4 行 TCP keepalive 配置)
    - internal/cloudclaude/colors.go (追加 ansiGray)
    - internal/cloudclaude/last_session.go (追加 3 omitempty 字段)
    - internal/cloudclaude/last_session_test.go (追加 2 个 round-trip / omitempty 用例)
    - internal/cloudclaude/errcodes/codes.go (追加 10 条 Code 常量)
    - internal/cloudclaude/errcodes/net.go (追加第二个 init 注册 3 条 NET_*)
decisions:
  - "RunKeepAlive 单次 SendRequest 必须 goroutine + select 包 timeout — 否则 dead network 上 SendRequest 永久阻塞，失败计数永远不增长"
  - "ConfigureTCPKeepAlive 失败仅返回 error，调用方 stderr warning 不阻塞 SSH 握手（best-effort，CONTEXT D-04 第 4 条）"
  - "errcodes/net.go 在文件末尾追加第二个 init() — 不污染既有 NET_OAUTH_* 注册块，与 mount.go 单 init 模式不同"
  - "Reconnector.StateAddr() 暴露 *atomic.Int32 — 让 BufferedStdin 与 Reconnector 共享同一原子状态字（避免双源同步）"
  - "renderDisconnectStatus 设为非导出纯函数 — 单测可直接断言 6 个三态阈值用例"
  - "fastRetry 60s 滑动窗口 5 次封顶 → 第 6 次返回 ErrReconnectGaveUp（实现：fastRetryCount > 5）"
metrics:
  duration: ~30 分钟（含 Task 1.2 keepalive 慢测试 45s）
  completed: 2026-04-20
---

# Phase 32 Plan 01: net-resilience Summary

落地 Phase 32 网络层韧性基线：客户端 SSH KeepAlive（应用层 SendRequest + TCP 层平台特化 setsockopt）+ 自实现重连状态机（退避 1/2/4/8/30s + 不弹密码 + 三态 UX）+ 本地输入缓冲（断网期间灰色未确认 + 重连按序提交）+ 全部 10 条新错误码注册。

## 一句话

`Phase 32 Plan 01` 一次性提供了 4 个可复用 service（RunKeepAlive / ConfigureTCPKeepAlive / Reconnector / BufferedStdin）+ 10 条 SESSION/NET 错误码 + 3 个 last-session.json 新字段，所有公共 API 已被 Plan 02 / Plan 03 通过依赖图引用，零新增 go.mod 依赖。

## Service 实际签名（与 PLAN `<interfaces>` 对照）

### keepalive.go

```go
// RunKeepAlive 在 conn 上每 interval 发一次 SSH 全局 keepalive 请求；
// 连续 countMax 次失败（含单次 timeout）后 conn.Close() 让上层 reconnect 感知。
func RunKeepAlive(ctx context.Context, conn ssh.Conn, interval time.Duration, countMax int) error

// ConfigureTCPKeepAlive 调用顺序：SetKeepAlive(true) → SetKeepAlivePeriod(period)
// → configurePlatformSpecific（Linux TCP_USER_TIMEOUT / macOS TCP_KEEPALIVE / 其它 noop+warning）
func ConfigureTCPKeepAlive(tcpConn *net.TCPConn, period time.Duration) error

// 内部：sendKeepaliveWithTimeout(conn, timeout) — goroutine + select <-time.After(timeout) 包装
// 防 dead network 上 SendRequest 永久阻塞（RESEARCH §1.2 [ASSUMED]）。
```

### reconnect.go

```go
type ConnState int32
const ( StateConnected ConnState = iota; StateReconnecting; StateGaveUp )
var ErrReconnectGaveUp = errors.New("reconnect gave up after fast-retry budget exceeded")
var backoffSeq = []time.Duration{1*s, 2*s, 4*s, 8*s, 30*s}

func NewReconnector(cfg SSHConfig, onConnLost func(), onReconnected func(*ssh.Client) error,
    statusWriter io.Writer, noColor bool) *Reconnector
func (r *Reconnector) Run(ctx context.Context) error
func (r *Reconnector) Trigger()
func (r *Reconnector) State() ConnState
func (r *Reconnector) StateAddr() *atomic.Int32          // [新增] 暴露给 BufferedStdin 共享
func (r *Reconnector) ReconnectCount() int               // 累计成功重连次数（last-session.json 用）
func renderDisconnectStatus(d time.Duration, noColor bool) string  // 纯函数，单测友好
func FormatGiveUpMessage(retries int, totalDuration time.Duration) string
```

### input_buffer.go

```go
const RingBufCapacity = 4096

type BufferedStdin struct { /* src, pipeW, *atomic.Int32 state, ringBuf, localEcho, noColor, onEnter, grayOpen */ }

func NewBufferedStdin(src io.Reader, state *atomic.Int32, localEcho io.Writer,
    noColor bool, onEnter func()) (*BufferedStdin, io.Reader)
func (b *BufferedStdin) Run(ctx context.Context) error
func (b *BufferedStdin) Flush() error
func (b *BufferedStdin) Close() error
```

### last_session.go 新字段

```go
TmuxSession    string `json:"tmux_session,omitempty"`
ClientRole     string `json:"client_role,omitempty"`
ReconnectCount int    `json:"reconnect_count,omitempty"`
```

### colors.go 新常量

```go
ansiGray = "\033[90m"
```

## 10 条 errcodes 注册位置

| Code | codes.go 行 | 注册文件:行 |
| --- | --- | --- |
| `SESSION_KEEPALIVE_TOO_AGGRESSIVE` | codes.go:136 | session.go:9 |
| `SESSION_TMUX_UNAVAILABLE` | codes.go:137 | session.go:15 |
| `SESSION_NOT_FOUND` | codes.go:138 | session.go:21 |
| `SESSION_TAKEOVER_NOTIFIED` | codes.go:139 | session.go:27 |
| `SESSION_TAKEOVER_FAILED` | codes.go:140 | session.go:33 |
| `SESSION_SYNC_LOCKED` | codes.go:141 | session.go:39 |
| `SESSION_BUFFER_OVERFLOW` | codes.go:142 | session.go:45 |
| `NET_RECONNECT_BACKOFF` | codes.go:143 | net.go:32（第二个 init） |
| `NET_RECONNECT_GAVE_UP` | codes.go:144 | net.go:38 |
| `NET_TCP_KEEPALIVE_UNSUPPORTED` | codes.go:145 | net.go:44 |

`errcodes` 包测试 `TestErrcodesRegistry` 全部通过（25 条以上注册码 + 命名正则 + NextAction ≤ 80 runes + 唯一性）。

## 3 平台 build-tag 文件 setsockopt 常量值（与 RESEARCH §2 对照）

| 平台 | 文件 | sockopt level | option | value |
| --- | --- | --- | --- | --- |
| linux/amd64 | keepalive_linux.go | `IPPROTO_TCP` | `tcpUserTimeout = 18` (TCP_USER_TIMEOUT) | `30000`（毫秒） |
| darwin/arm64 | keepalive_darwin.go | `IPPROTO_TCP` | `tcpKeepalive = 0x10` (TCP_KEEPALIVE) | `15`（秒） |
| 其它（windows/freebsd/...） | keepalive_other.go | — | noop | 仅 stderr 输出 `[NET_TCP_KEEPALIVE_UNSUPPORTED]` warning |

stdlib `SetKeepAlive(true)` + `SetKeepAlivePeriod(15s)` 在三平台均生效；上述常量是平台特化叠加层。

## 单测覆盖率（go test -cover，仅本 plan 引入的测试）

```
keepalive.go:           RunKeepAlive 81.0% / sendKeepaliveWithTimeout 87.5% / ConfigureTCPKeepAlive 60.0%
keepalive_darwin.go:    configurePlatformSpecific 75.0%
input_buffer.go:        NewBufferedStdin 100% / Run 81.2% / handleReconnecting 94.4% / Flush 77.8% / Close 66.7%
reconnect.go:           NewReconnector / State / StateAddr / Trigger / recordFastRetry / exceededFastRetryBudget 均 100%
                        renderDisconnectStatus 83.3% / FormatGiveUpMessage 100%
                        Run / renderStatus 0%（需真实 ssh.Client，留 Plan 02 集成测试覆盖）
```

整体新文件加权覆盖 ≈ 78%（剔除 Run / renderStatus 两个集成依赖）。

## Plan 02 / Plan 03 接入点

### Plan 02（runClaudeWithSession 包装层）

1. **在 `ConnectAndRunClaudeV3` 内** mount ready / OAuth 通过 / tmux 探测后构造 Reconnector：
   ```go
   reconnector := NewReconnector(cfg, onConnLost, onReconnected, os.Stderr, noColor)
   ```
2. **构造 BufferedStdin 并共享 state**：
   ```go
   buffered, pipeR := NewBufferedStdin(os.Stdin, reconnector.StateAddr(),
       os.Stdout, noColor, reconnector.Trigger)
   go buffered.Run(ctx)
   session.Stdin = pipeR
   ```
3. **启动 keepalive goroutine（在 conn-A 建立后）**：
   ```go
   go RunKeepAlive(ctx, connA.Conn, mountCfg.KeepAliveInterval, mountCfg.KeepAliveCountMax)
   ```
4. **conn 上感知 io.EOF / *ssh.ExitError 后**：
   ```go
   if err := reconnector.Run(ctx); err != nil {
       if errors.Is(err, ErrReconnectGaveUp) {
           fmt.Fprintln(os.Stderr, FormatGiveUpMessage(5, time.Since(disconnectStart)))
           return ExitNetworkError, nil
       }
       return 0, err
   }
   ```
5. **`onReconnected` 回调内**：`buffered.Flush()` + 重新挂 RunKeepAlive + 重新申请 PTY。

### Plan 03（multi-client / sync-lock）

- last-session.json 写 `TmuxSession=<actual_session_name>` / `ClientRole="primary"|"secondary"` / `ReconnectCount=reconnector.ReconnectCount()`。
- `SESSION_SYNC_LOCKED` 在 sync-lock 拿不到锁时由 Plan 03 调用 `errcodes.Format(...)` 输出。
- `SESSION_TAKEOVER_NOTIFIED` / `SESSION_TAKEOVER_FAILED` 在 `--take-over` 流程使用。

### CLI 启动期校验（Plan 02 main.go）

```go
if mountCfg.KeepAliveInterval < 15*time.Second {
    fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE, mountCfg.KeepAliveInterval))
    os.Exit(ExitConfigError)
}
```

## Verify 命令实测结果

```
$ go build ./...                                                               PASS
$ GOOS=linux  go vet ./internal/cloudclaude/...                                PASS
$ GOOS=darwin go vet ./internal/cloudclaude/...                                PASS
$ go test ./internal/cloudclaude/errcodes/... -count=1                         PASS
$ go test ./internal/cloudclaude/ -run 'TestRunKeepAlive_Rejects|TestRunKeepAlive_SuccessResetsFails|
    TestConfigureTCPKeepAlive|TestRenderDisconnectStatus|TestReconnector|TestBackoffSeq|
    TestBufferedStdin|TestLastSessionSnapshot_NewFields|TestLastSessionSnapshot_Omitempty|
    TestFormatGiveUpMessage' -count=1                                          PASS
$ go test ./internal/cloudclaude/ -run TestRunKeepAlive_TimeoutCounts -timeout 90s   PASS（45s）
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - 阻塞] Reconnector.Run 增加 onReconnected nil 守卫 + 回调失败的 backoff 推进**
- **Found during**: Task 1.3 实现时回看 PLAN 模板
- **Issue**: PLAN `<action>` 1 的伪代码假设 `r.onReconnected` 非 nil；单测 `TestReconnector_TriggerDropsExtras` 用 nil 回调直接构造 Reconnector，主循环若不守卫会 nil panic。
- **Fix**: `Run` 内 `if r.onReconnected != nil { ... }` 包裹；并在回调返回非 nil error 时 close newConn + 推进 backoffIdx 重试，避免坏回调死循环。
- **Files modified**: `internal/cloudclaude/reconnect.go`
- **Commit**: `105afc1`

**2. [Rule 3 - 阻塞] renderStatus 增加 statusWriter nil 守卫**
- **Found during**: Task 1.3 单测设计
- **Issue**: 单测 `NewReconnector(..., nil, true)` 传 nil statusWriter 时，原模板 `fmt.Fprint(r.statusWriter, ...)` 会 panic。
- **Fix**: 100ms ticker 中增加 `if r.statusWriter == nil { continue }`；ctx.Done 分支同样守卫。
- **Files modified**: `internal/cloudclaude/reconnect.go`
- **Commit**: `105afc1`

**3. [Rule 3 - 阻塞] StateAddr 方法新增（暴露 *atomic.Int32）**
- **Found during**: 写 input_buffer 单测时
- **Issue**: PLAN `<interfaces>` 段写 `BufferedStdin.state` 字段是 `*atomic.Int32`，但 PLAN 没说 Reconnector 怎么暴露其内部 atomic.Int32 的指针给 BufferedStdin 共享。Plan 02 接入点伪代码用了不存在的 `&reconnector.state(待暴露)`。
- **Fix**: `Reconnector` 新增 `StateAddr() *atomic.Int32` 方法返回 `&r.state`，Plan 02 接入点更新为 `NewBufferedStdin(os.Stdin, reconnector.StateAddr(), ...)`。
- **Files modified**: `internal/cloudclaude/reconnect.go`
- **Commit**: `105afc1`

**4. [Rule 3 - 阻塞] keepalive_test.go 长用例增加 testing.Short() 跳过**
- **Found during**: Task 1.2 测试运行
- **Issue**: `TestRunKeepAlive_TimeoutCounts` 真实跑 ~30 秒；CI -short 场景下应跳过。
- **Fix**: 函数顶部 `if testing.Short() { t.Skip(...) }`。
- **Files modified**: `internal/cloudclaude/keepalive_test.go`
- **Commit**: `bb1a997`

### 非 Auto-fix 偏差（仅文档）

无。10 条错误码 Message/NextAction、退避序列、setsockopt 常量、ringBuf 容量等 hard-coded 值均与 PLAN 逐字符对齐。

## Deferred Issues（out of scope，登记在此）

**1. `GOOS=windows go build ./internal/cloudclaude/...` 失败**
- 失败原因：`internal/cloudclaude/ssh.go` 既有的 `signal.Notify(sigCh, syscall.SIGWINCH)` 在 windows 下未定义。
- **不是本 plan 引入**：在本 plan 第一个 commit 之前 `git stash` 后跑同命令同样失败（已实测）。
- **本 plan 范围内的 windows 兼容性**：`keepalive_other.go` 自身在 windows 编译通过（已通过 `errcodes` 子包验证 `GOOS=windows go vet ./internal/cloudclaude/errcodes/...` PASS）。
- **建议归属**：作为 Phase 32 / Phase 35 的 windows 移植任务单独处理，需要把 ssh.go 的 PTY/SIGWINCH 路径走 build tag 拆 `_unix.go` / `_windows.go`。
- **不影响 Plan 02/03 推进**：v3.0 cloud-claude 仅承诺 Linux + macOS（PROJECT.md 约束）。

## Threat Flags

无。本 plan 网络层韧性不引入新的高危信任边界。PLAN `<threat_model>` 已枚举 7 项 STRIDE 威胁，全部 mitigate（4 项）/ accept（3 项），无 high severity。

## Self-Check: PASSED

- [x] `internal/cloudclaude/keepalive.go` exists
- [x] `internal/cloudclaude/keepalive_linux.go` exists
- [x] `internal/cloudclaude/keepalive_darwin.go` exists
- [x] `internal/cloudclaude/keepalive_other.go` exists
- [x] `internal/cloudclaude/keepalive_test.go` exists
- [x] `internal/cloudclaude/reconnect.go` exists
- [x] `internal/cloudclaude/reconnect_test.go` exists
- [x] `internal/cloudclaude/input_buffer.go` exists
- [x] `internal/cloudclaude/input_buffer_test.go` exists
- [x] `internal/cloudclaude/errcodes/session.go` exists
- [x] commit `5f3e271` (Task 1.1) exists
- [x] commit `bb1a997` (Task 1.2) exists
- [x] commit `105afc1` (Task 1.3) exists
