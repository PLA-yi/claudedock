---
phase: 32
plan: 05-bufferedstdin-reconnect-wiring
type: execute
wave: 1
depends_on:
  - 01-net-resilience
  - 02-tmux-multiclient
autonomous: true
gap_closure: true
requirements:
  - REQ-F3-B
files_modified:
  - internal/cloudclaude/session.go
  - internal/cloudclaude/session_test.go
  - internal/cloudclaude/input_buffer.go
  - internal/cloudclaude/input_buffer_test.go
must_haves:
  truths:
    - "SC5（REQ-F3-B）断网时本地键入字符以灰色未确认样式显示，重连后按序提交，无丢字 / 无乱序"
    - "session.go::runClaudePTYWithReconnect 把 BufferedStdin + Reconnector 的创建提升到 pTYAttachOnce 循环外层：单个 Reconnector 跨所有 attach 周期持有 atomic.Int32 state，pTYAttachOnce 内部的 BufferedStdin 通过 `reconnector.StateAddr()` 共享同一个 *atomic.Int32 —— 断网 → Reconnector.Run 写 StateReconnecting → BufferedStdin 立即进入 ringBuf + 灰色 echo 分支（Gap #1 missing#1 核心闭合路径）"
    - "`rg 'var state atomic\\.Int32' internal/cloudclaude/session.go` 在 pTYAttachOnce 函数范围内 0 hits（局部 state 被移除）；`rg 'reconnector\\.StateAddr\\(\\)' internal/cloudclaude/session.go` ≥ 1 hit（共享路径生效）"
    - "reconnect 成功的 onReconnected 回调中调 `bs.Flush()` 把 ringBuf 按序写回 io.Pipe 的 pipeW；新一轮 pTYAttachOnce 创建的 session.Stdin = pipeR 立即读到缓冲字节（无丢字 / 无乱序 —— 新增集成测试 TestPTYReconnect_BufferedInputFlush 断言 fake conn.Wait 触发 reconnect → pipe 喂 \"abc\" → 成功后新 pipeR 读到 \"abc\"）"
    - "WR-03 co-fix：BufferedStdin 作为 runClaudePTYWithReconnect 循环前的单例存在，单个 goroutine 读 os.Stdin；pTYAttachOnce 退出后 bs 不被 Close —— 旧 attach 周期结束不启动新 bs.Run goroutine，彻底消除 WR-03 的多 goroutine 并发读 os.Stdin 问题（RESEARCH §Anti-Patterns WR-03 闭合）"
    - "WR-04 co-fix：input_buffer.go 新增 `echoMu sync.Mutex` 保护 `grayOpen` + 所有 pipeW 写入 + closeGrayIfOpen；Flush 与 handleReconnecting 不再出现跨 goroutine data race（`go test -race ./internal/cloudclaude/...` PASS）"
  artifacts:
    - path: "internal/cloudclaude/session.go"
      provides: "runClaudePTYWithReconnect 外层持有 Reconnector + BufferedStdin 单例；pTYAttachOnce 函数签名新增 bufferedPipeR io.Reader 参数；pTYAttachOnce 内部不再创建 BufferedStdin / 局部 atomic.Int32"
      contains: "reconnector.StateAddr()"
    - path: "internal/cloudclaude/input_buffer.go"
      provides: "BufferedStdin.echoMu sync.Mutex 保护 grayOpen + pipeW 写入；Flush / handleReconnecting / closeGrayIfOpen 全部在 mutex 内"
      contains: "echoMu"
    - path: "internal/cloudclaude/session_test.go"
      provides: "TestPTYReconnect_BufferedInputFlush 集成级单测：fake conn.Wait 返回 io.EOF → reconnect 触发 → 断网期 io.Pipe 喂 'abc' → reconnect 成功后 pipeR 读到 'abc' + echo 含 ansiGray"
      contains: "TestPTYReconnect_BufferedInputFlush"
    - path: "internal/cloudclaude/input_buffer_test.go"
      provides: "保持现有 5 个测试全 PASS（nil-check state 指针场景 + race mode）"
      contains: "TestBufferedStdin"
  key_links:
    - from: "session.go::runClaudePTYWithReconnect"
      to: "reconnect.go::Reconnector.StateAddr()"
      via: "循环外 reconnector := NewReconnector(...); 循环内 NewBufferedStdin(os.Stdin, reconnector.StateAddr(), ...) —— 修复 Gap #1 session.go:724-726 局部 atomic.Int32"
      pattern: "reconnector\\.StateAddr\\(\\)"
    - from: "session.go::runClaudePTYWithReconnect onReconnected"
      to: "input_buffer.go::BufferedStdin.Flush"
      via: "reconnect 成功回调中 bs.Flush() 把 ringBuf 按序写 pipeW（无丢字 / 无乱序）"
      pattern: "bs\\.Flush\\(\\)"
    - from: "session.go::pTYAttachOnce"
      to: "session.go::runClaudePTYWithReconnect 外层 pipeR"
      via: "pTYAttachOnce 新增 bufferedPipeR io.Reader 参数，session.Stdin = bufferedPipeR（局部 state / BufferedStdin 创建逻辑被彻底移除）"
      pattern: "bufferedPipeR"
    - from: "input_buffer.go::BufferedStdin"
      to: "input_buffer.go::echoMu sync.Mutex"
      via: "grayOpen + pipeW.Write + closeGrayIfOpen 全部在 echoMu 保护下（WR-04 co-fix）"
      pattern: "echoMu"
---

<plan_dependencies>
- **Plan 01（Wave 1 原始）必须已 ship**：本 plan 直接消费：
  - `reconnect.Reconnector.StateAddr() *atomic.Int32`（Plan 01 Task 1.3 已暴露，grep 已验证 reconnect.go:81-82）
  - `input_buffer.BufferedStdin{ Run, Flush, Close, NewBufferedStdin }`（Plan 01 Task 1.3）
  - `reconnect.NewReconnector` / `ErrReconnectGaveUp` / `FormatGiveUpMessage`
- **Plan 02（Wave 2 原始）必须已 ship**：本 plan 重构的 `runClaudeWithSession` / `runClaudePTYWithReconnect` / `pTYAttachOnce` 由 Plan 02 Task 2.1b 落地
- **Plan 04（本批次另一个 gap plan）与本 plan 无文件重叠**：本 plan 改 session.go / session_test.go / input_buffer.go / input_buffer_test.go；Plan 04 改 mount_strategy.go / mount_strategy_test.go —— 两个 plan 同 wave 可并行提交
- **本 plan 不碰**：mount_strategy.go / ssh.go / sync_lock.go / keepalive.go / reconnect.go / last_session.go / errcodes/* / colors.go / cmd/cloud-claude/* —— 修复范围严格限定在 Gap #1 + WR-03 + WR-04 co-fix
- **接口补强说明**：Gap #1 missing#2 提供了"备选方案"（补 RegisterStateListener + SetReconnector 方法到 Plan 01），但 missing#1 是**更小改动**（直接复用 Plan 01 已暴露的 `reconnector.StateAddr()` getter）。本 plan 采纳 missing#1 路径，**不改 reconnect.go / input_buffer.go 的公开接口**（input_buffer.go 仅内部新增 echoMu sync.Mutex，签名 zero diff）
</plan_dependencies>

<objective>
闭合 32-VERIFICATION.md Gap #1（SC5 / REQ-F3-B）+ co-fix REVIEW.md WR-03 + WR-04。

**根因**（来自 32-VERIFICATION.md gaps[0]）：

`internal/cloudclaude/session.go:724-726` 在 `pTYAttachOnce` 内：

```go
var state atomic.Int32
state.Store(int32(StateConnected))
bs, pipeR := NewBufferedStdin(os.Stdin, &state, os.Stderr, sessionCfg.NoColor, nil)
```

该局部 `state` 变量**从未被 Reconnector 写入**，永远为 `StateConnected`。Plan 01 的 BufferedStdin Reconnecting 分支（`handleReconnecting` + ringBuf + 灰色 echo + Flush）自身测试通过，但端到端路径永不激活。reconnect 期间用户键入直接进入 pipeW 被阻塞 / 丢弃；灰色未确认样式从不出现。

**co-fix WR-03**：每次 `pTYAttachOnce` 循环迭代都 `go bs.Run(bsCtx)` —— 旧 goroutine 阻塞在 `os.Stdin.Read()` syscall 永不退出，新一轮 attach 再开一个 → **多个 goroutine 并发读 os.Stdin**，reconnect 几次后用户输入被多个 goroutine 抢字节。

**co-fix WR-04**：BufferedStdin 内部 `grayOpen` + `closeGrayIfOpen` + `Flush` + `handleReconnecting` 共享同一个 `pipeW` + `grayOpen` 字段，无 mutex 保护。SC5 修复启用 Flush 路径后，Flush 与 Run goroutine 并发执行会引入 data race。

**统一修复方案**：把 Reconnector + BufferedStdin 的**生命周期**从 `pTYAttachOnce` 函数内（每次 attach 重建）提升到 `runClaudePTYWithReconnect` 循环外（一次创建、跨所有 attach 周期复用）：

1. `runClaudePTYWithReconnect` 开头 `reconnector := NewReconnector(...)`（once）
2. `NewBufferedStdin(os.Stdin, reconnector.StateAddr(), os.Stderr, noColor, reconnector.Trigger)`（once）—— state 指针指向 reconnector 内部的 atomic.Int32
3. `go bs.Run(ctx)`（once）—— 单 goroutine 读 os.Stdin
4. `pTYAttachOnce` 不再构造 bs / 局部 state；接收 `bufferedPipeR io.Reader` 参数并 `session.Stdin = bufferedPipeR`
5. session.Wait 返回 reconnectable err → 外层 `reconnector.Run(ctx)` 内部把 state 写为 StateReconnecting → BufferedStdin 自动切 ringBuf 分支
6. reconnector.Run 成功 → state 写回 StateConnected + 外层调 `bs.Flush()` 把 ringBuf 写 pipeW → 下一轮 pTYAttachOnce 的 session.Stdin 立即读到缓冲字节
7. `input_buffer.go` 新增 `echoMu sync.Mutex` 保护 grayOpen + pipeW.Write（WR-04 co-fix）

Purpose: 让 Phase 32 SC5 从 code-level ✗ FAILED 转为 ✓ VERIFIED；消除 WR-03（多 goroutine 读 os.Stdin）和 WR-04（grayOpen / localEcho data race）两条既有 Warning；BufferedStdin 从"冷代码"升级为生产路径真实激活。
Output: `session.go` ~60 行 diff（pTYAttachOnce 减 BufferedStdin 创建段 + runClaudePTYWithReconnect 加外层 bs/Reconnector 单例）；`input_buffer.go` ~15 行 diff（echoMu 新增 + Flush/handleReconnecting/closeGrayIfOpen 加锁）；`session_test.go` ~80 行新增 1 个集成级用例；`input_buffer_test.go` zero or 小幅改（race mode 兼容）；零新依赖。
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
@.planning/phases/32-ssh-tmux/32-VERIFICATION.md
@.planning/phases/32-ssh-tmux/32-REVIEW.md
@.planning/phases/32-ssh-tmux/plans/01-net-resilience/PLAN.md
@.planning/phases/32-ssh-tmux/plans/01-net-resilience/SUMMARY.md
@.planning/phases/32-ssh-tmux/plans/02-tmux-multiclient/PLAN.md
@.planning/phases/32-ssh-tmux/plans/02-tmux-multiclient/SUMMARY.md
@internal/cloudclaude/session.go
@internal/cloudclaude/session_test.go
@internal/cloudclaude/input_buffer.go
@internal/cloudclaude/input_buffer_test.go
@internal/cloudclaude/reconnect.go
@internal/cloudclaude/keepalive.go

<interfaces>
<!-- Plan 01 已 ship 的关键接口（本 plan 直接消费，不修改）。 -->

internal/cloudclaude/reconnect.go（Plan 01 Task 1.3 已 ship）：

```go
// StateAddr 暴露 atomic.Int32 指针，供 BufferedStdin 共享读取（reconnect.go:81-82 已 ship）。
func (r *Reconnector) StateAddr() *atomic.Int32 { return &r.state }

// Run 入口把 state.Store(StateReconnecting)；成功时 state.Store(StateConnected)
//   —— 正是 BufferedStdin 切 Reconnecting / Connected 分支的触发源
func (r *Reconnector) Run(ctx context.Context) error

// Trigger 供 BufferedStdin 检测到 \r/\n 时唤醒 Reconnector（channel size=1，drop 多余）
func (r *Reconnector) Trigger()

// ReconnectCount 累计成功重连次数
func (r *Reconnector) ReconnectCount() int
```

internal/cloudclaude/input_buffer.go（Plan 01 Task 1.3 已 ship）：

```go
// NewBufferedStdin 用 io.Pipe 拿到 (pipeR, pipeW)；返回的 io.Reader 直接喂给 ssh.Session.Stdin。
// state 指针必须由调用方共享给 Reconnector（同一 atomic.Int32）。
// onEnter 在 state==Reconnecting 且检测到 \r/\n 时调用（通常 = reconnector.Trigger）。
func NewBufferedStdin(src io.Reader, state *atomic.Int32, localEcho io.Writer,
    noColor bool, onEnter func()) (*BufferedStdin, io.Reader)

// Run / Flush / Close — 已 ship，本 plan 不改签名
func (b *BufferedStdin) Run(ctx context.Context) error
func (b *BufferedStdin) Flush() error
func (b *BufferedStdin) Close() error
```
</interfaces>

<current_session_layout>
<!-- 定位本 plan 的重构范围 —— session.go 现状（Plan 02 ship）。 -->

```
// runClaudeWithSession 主入口（Plan 02 ship；line 572-598）
func runClaudeWithSession(ctx, conn, sshCfg, claudeArgs, sessionCfg, hasProxy) (int, error) {
    // 1) session 命名 / take-over / banner / writeLastSessionTmuxField
    // 2) buildClaudeCmd / buildTmuxRemoteCmd
    // 3) return runClaudePTYWithReconnect(ctx, conn, sshCfg, remoteCmd, sessionName, sessionCfg)
}

// runClaudePTYWithReconnect（Plan 02 ship；line 611-661）
func runClaudePTYWithReconnect(ctx, initialConn, sshCfg, remoteCmd, sessionName, sessionCfg) (int, error) {
    conn := initialConn
    reconnectCount := 0
    registryPid := 0
    defer func() { /* removeClientFile */ }()

    for {
        // ⬇⬇⬇ 本 plan 修改：在循环开始前一次性创建 Reconnector + BufferedStdin    ⬇⬇⬇
        //      (循环内 pTYAttachOnce 接收 bufferedPipeR 参数，不再内部创建)
        // ⬆⬆⬆                                                                     ⬆⬆⬆

        exitCode, exitErr, reconnectableErr := pTYAttachOnce(ctx, conn, remoteCmd, sessionName, sessionCfg, &registryPid)

        if exitErr == nil { /* normal exit */ return }
        if reconnectableErr == nil || !sessionCfg.ReconnectEnabled { return }

        // ⬇⬇⬇ 本 plan 修改：不再每次 iter 新 NewReconnector；用外层单例；onReconnected 回调 bs.Flush()  ⬇⬇⬇
        reconnector := NewReconnector(sshCfg, nil, func(c *ssh.Client) error { newConn = c; return nil }, ...)
        if err := reconnector.Run(ctx); err != nil { /* ErrReconnectGaveUp or other */ }
        reconnectCount += reconnector.ReconnectCount()
        conn = newConn
        // ⬆⬆⬆                                                                                          ⬆⬆⬆
    }
}

// pTYAttachOnce（Plan 02 ship；line 671-782，包含 Gap #1 根因 line 724-726）
func pTYAttachOnce(ctx, conn, remoteCmd, sessionName, sessionCfg, registryPid *int) (int, error, error) {
    // 1) conn.NewSession / defer session.Close
    // 2) PTY 申请（MakeRaw + RequestPty + SIGWINCH 复刻 runClaude）

    // ⬇⬇⬇ Gap #1 根因 line 715-730（删除/替换为接收外层 bufferedPipeR 参数）                       ⬇⬇⬇
    var state atomic.Int32                                // ← 删除
    state.Store(int32(StateConnected))                    // ← 删除
    bs, pipeR := NewBufferedStdin(os.Stdin, &state, ...)  // ← 删除
    bsCtx, cancelBs := context.WithCancel(ctx)            // ← 删除
    defer cancelBs()                                       // ← 删除
    go func() { _ = bs.Run(bsCtx) }()                     // ← 删除（WR-03 co-fix：此 goroutine 每次 iter 泄漏）
    defer bs.Close()                                      // ← 删除
    session.Stdin = pipeR                                  // ← 替换为 session.Stdin = bufferedPipeR
    // ⬆⬆⬆                                                                                             ⬆⬆⬆

    // 3) RunKeepAlive goroutine
    // 4) session.Start / writeClientFile / session.Wait / return
}
```
</current_session_layout>

<current_input_buffer_layout>
<!-- input_buffer.go 现状（Plan 01 ship；input_buffer.go:24-149）—— 本 plan 新增 echoMu mutex。 -->

```
type BufferedStdin struct {
    src       io.Reader
    pipeW     io.WriteCloser
    state     *atomic.Int32
    ringBuf   []byte
    ringMu    sync.Mutex      // 保护 ringBuf（已存在）
    // ⬇⬇⬇ 本 plan 新增                  ⬇⬇⬇
    // echoMu    sync.Mutex   // 保护 pipeW 写入 + grayOpen + closeGrayIfOpen（WR-04 co-fix）
    // ⬆⬆⬆                                 ⬆⬆⬆
    localEcho io.Writer
    noColor   bool
    onEnter   func()
    grayOpen  bool             // 无保护 → WR-04 root cause
}

// Run 逐字节读 src → 按 state 分发
//   StateConnected   → closeGrayIfOpen() + pipeW.Write      ← 竞态：与 Flush 的 pipeW.Write 冲突
//   StateReconnecting → handleReconnecting                  ← 竞态：handleReconnecting 修改 grayOpen
//   StateGaveUp      → 丢弃

// Flush 在 state Reconnecting→Connected 过渡时由外层调用
//   closeGrayIfOpen()           ← 竞态：与 Run goroutine 的 closeGrayIfOpen 冲突
//   ringMu.Lock + pipeW.Write + ringMu.Unlock   ← 竞态：与 Run goroutine 的 pipeW.Write 冲突
```
</current_input_buffer_layout>
</context>

<tasks>

<task type="auto">
  <name>Task 5.1: input_buffer.go 新增 echoMu sync.Mutex（WR-04 co-fix）+ 保持 5 个既有单测 PASS</name>
  <files>
    internal/cloudclaude/input_buffer.go
    internal/cloudclaude/input_buffer_test.go
  </files>
  <read_first>
    - internal/cloudclaude/input_buffer.go（line 1-149 全文 —— 结构体定义 line 24-34；NewBufferedStdin line 41-55；Run line 58-88；handleReconnecting line 90-119；closeGrayIfOpen line 121-126；Flush line 129-141；Close line 144-149）
    - internal/cloudclaude/input_buffer_test.go（现有 5 个测试：TestBufferedStdin_ConnectedDirectWrite / _ReconnectingBuffersAndGrayEchoes / _RingBufOverflowDropsAndWarns / _EnterTriggersOnEnter / _FlushClearsBuffer —— 必须保持全 PASS，尤其 race mode）
    - .planning/phases/32-ssh-tmux/32-REVIEW.md WR-04（grayOpen/localEcho 无 mutex；本 task 闭合）
    - .planning/phases/32-ssh-tmux/32-VERIFICATION.md gaps[0]（Gap #1 提到 WR-04 是 SC5 修复路径的必要共修项）
  </read_first>
  <acceptance_criteria>
    - `go build ./internal/cloudclaude/...` 成功
    - `go vet ./internal/cloudclaude/...` 通过
    - `rg -n "echoMu\\s+sync\\.Mutex" internal/cloudclaude/input_buffer.go` 命中 1 次（新字段）
    - `rg -n "b\\.echoMu\\.Lock\\(\\)" internal/cloudclaude/input_buffer.go` 命中 ≥ 3 次（Run-Connected 分支 / handleReconnecting / Flush 内部）
    - `rg -n "b\\.echoMu\\.Unlock\\(\\)" internal/cloudclaude/input_buffer.go` 命中 ≥ 3 次（配对）
    - `go test ./internal/cloudclaude/... -run TestBufferedStdin -count=1` 5 个既有用例全 PASS（签名 zero diff）
    - `go test ./internal/cloudclaude/... -run TestBufferedStdin -count=1 -race` PASS（race mode 无 data race 警告 —— WR-04 核心断言）
    - `git diff internal/cloudclaude/input_buffer.go | grep '^+' | grep -v '^+++' | wc -l` 输出 ≤ 20（最小侵入：1 字段 + 3-4 处 Lock/Unlock 调用）
    - 公开 API 签名 zero diff：`NewBufferedStdin / Run / Flush / Close / RingBufCapacity` 全部不改（执行器 grep `git diff internal/cloudclaude/input_buffer.go` 确认）
  </acceptance_criteria>
  <action>
    ### Step 1 — 新增 echoMu 字段

    `internal/cloudclaude/input_buffer.go` 的 `BufferedStdin` struct 末尾（line 24-34 之间），在 `ringMu` 后新增：

    ```go
    type BufferedStdin struct {
        src       io.Reader
        pipeW     io.WriteCloser
        state     *atomic.Int32 // 共享 Reconnector.state
        ringBuf   []byte
        ringMu    sync.Mutex    // 保护 ringBuf（既有）
        echoMu    sync.Mutex    // [Phase 32 Gap#1 / WR-04] 保护 grayOpen + pipeW.Write + closeGrayIfOpen 跨 goroutine 调用
        localEcho io.Writer
        noColor   bool
        onEnter   func()
        grayOpen  bool          // 是否已 echo 过开头 \x1b[90m（在 echoMu 下读写）
    }
    ```

    ### Step 2 — Run 的 StateConnected 分支加锁

    现状（line 69-74）：

    ```go
    case StateConnected:
        b.closeGrayIfOpen()
        if _, werr := b.pipeW.Write(buf[:n]); werr != nil {
            return werr
        }
    ```

    改为：

    ```go
    case StateConnected:
        b.echoMu.Lock()
        b.closeGrayIfOpenLocked()
        _, werr := b.pipeW.Write(buf[:n])
        b.echoMu.Unlock()
        if werr != nil {
            return werr
        }
    ```

    ### Step 3 — closeGrayIfOpen 拆为两个：公开 closeGrayIfOpen 自己加锁；内部 Locked 版本不加锁（因已在锁内）

    现状（line 121-126）：

    ```go
    func (b *BufferedStdin) closeGrayIfOpen() {
        if b.grayOpen && b.localEcho != nil && !b.noColor {
            fmt.Fprint(b.localEcho, ansiReset)
            b.grayOpen = false
        }
    }
    ```

    改为：

    ```go
    // closeGrayIfOpen 对外 API，自己管锁；用于不在 echoMu 内调用的场景。
    func (b *BufferedStdin) closeGrayIfOpen() {
        b.echoMu.Lock()
        defer b.echoMu.Unlock()
        b.closeGrayIfOpenLocked()
    }

    // closeGrayIfOpenLocked 调用方必须已持有 echoMu。
    func (b *BufferedStdin) closeGrayIfOpenLocked() {
        if b.grayOpen && b.localEcho != nil && !b.noColor {
            fmt.Fprint(b.localEcho, ansiReset)
            b.grayOpen = false
        }
    }
    ```

    ### Step 4 — handleReconnecting 内对 grayOpen 访问加锁

    现状（line 90-119）：

    ```go
    func (b *BufferedStdin) handleReconnecting(c byte) {
        b.ringMu.Lock()
        if len(b.ringBuf) >= RingBufCapacity {
            ...
            b.ringBuf = b.ringBuf[drop:]
            if b.localEcho != nil {
                fmt.Fprintln(b.localEcho, errcodes.Format(errcodes.SESSION_BUFFER_OVERFLOW))
            }
        }
        b.ringBuf = append(b.ringBuf, c)
        b.ringMu.Unlock()

        if b.localEcho != nil {
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
    ```

    改为（grayOpen / localEcho 写入段放到 echoMu 保护下）：

    ```go
    func (b *BufferedStdin) handleReconnecting(c byte) {
        b.ringMu.Lock()
        if len(b.ringBuf) >= RingBufCapacity {
            drop := 1024
            if drop > len(b.ringBuf) {
                drop = len(b.ringBuf)
            }
            b.ringBuf = b.ringBuf[drop:]
            if b.localEcho != nil {
                // 仅打一行警告；echoMu 不包含此行（SESSION_BUFFER_OVERFLOW 是独立 Fprintln，不碰 grayOpen）
                fmt.Fprintln(b.localEcho, errcodes.Format(errcodes.SESSION_BUFFER_OVERFLOW))
            }
        }
        b.ringBuf = append(b.ringBuf, c)
        b.ringMu.Unlock()

        if b.localEcho != nil {
            b.echoMu.Lock()
            if !b.grayOpen && !b.noColor {
                fmt.Fprint(b.localEcho, ansiGray)
                b.grayOpen = true
            }
            fmt.Fprintf(b.localEcho, "%c", c)
            b.echoMu.Unlock()
        }
        if c == '\r' || c == '\n' {
            if b.onEnter != nil {
                b.onEnter()
            }
        }
    }
    ```

    ### Step 5 — Flush 加 echoMu 保护 pipeW.Write + closeGrayIfOpen

    现状（line 129-141）：

    ```go
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
    ```

    改为（锁顺序：先 echoMu 再 ringMu，与 handleReconnecting / Run-Connected 锁顺序对齐避免死锁）：

    ```go
    func (b *BufferedStdin) Flush() error {
        b.echoMu.Lock()
        defer b.echoMu.Unlock()
        b.closeGrayIfOpenLocked()

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
    ```

    ### Step 6 — 锁顺序审查（避免死锁）

    本 plan 引入 echoMu 之后，全 BufferedStdin 的锁获取顺序为：

    - **Run-Connected**：echoMu.Lock() → pipeW.Write → echoMu.Unlock()（无 ringMu）
    - **handleReconnecting**：ringMu.Lock → ringMu.Unlock → echoMu.Lock → echoMu.Unlock（两锁**串行**获取，不嵌套）
    - **Flush**：echoMu.Lock → closeGrayIfOpenLocked → ringMu.Lock → pipeW.Write → ringMu.Unlock → echoMu.Unlock（两锁**嵌套**，echoMu 外 / ringMu 内）

    因为 handleReconnecting 是**串行两锁**（不嵌套），Flush 是**嵌套两锁** echoMu→ringMu，而 Run-Connected 只用 echoMu，没有任何路径同时持有 ringMu 后再尝试 echoMu，所以无死锁风险。

    执行器必须在代码注释中标注这个锁顺序，例如在 Flush 函数头：

    ```go
    // Flush 锁顺序：echoMu → ringMu（嵌套）；与 handleReconnecting（串行两锁）/ Run-Connected（仅 echoMu）兼容无死锁。
    ```

    ### Step 7 — 单测保持 PASS

    现有 5 个单测（TestBufferedStdin_*）签名 zero diff 下必须全 PASS，并且 race mode 通过：

    ```bash
    go test ./internal/cloudclaude/... -run TestBufferedStdin -count=1 -race
    ```

    如 race mode 发现任何残留 data race（比如 localEcho 单独 Fprintln 与 echoMu 段并发），执行器追加最小锁保护；不追加新测试用例（新的集成级测试在 Task 5.3）。
  </action>
  <verify>
    <automated>go test ./internal/cloudclaude/... -run TestBufferedStdin -count=1 -race &amp;&amp; rg -q "echoMu\\s+sync\\.Mutex" internal/cloudclaude/input_buffer.go &amp;&amp; rg -c "b\\.echoMu\\.Lock\\(\\)" internal/cloudclaude/input_buffer.go | grep -E "^[3-9]|^[1-9][0-9]" &amp;&amp; go vet ./internal/cloudclaude/...</automated>
  </verify>
  <done>
    - BufferedStdin 新增 echoMu sync.Mutex；Run-Connected / handleReconnecting 的 grayOpen+localEcho 段 / Flush 三条路径全部在 echoMu 保护下
    - closeGrayIfOpen 拆为公开 API + Locked 内部版；消除"已持锁者再次 Lock" 的死锁风险
    - 锁顺序文档化：echoMu 外 / ringMu 内（Flush 嵌套场景）；handleReconnecting 串行；无死锁
    - 现有 5 个 TestBufferedStdin_* 测试签名 zero diff 下 PASS；race mode 额外 PASS（WR-04 核心验证）
    - 公开 API 签名 zero diff —— Task 5.2 调用方无需感知内部锁变化
  </done>
</task>

<task type="auto">
  <name>Task 5.2: session.go 重构 —— Reconnector + BufferedStdin 单例提升到 runClaudePTYWithReconnect 外层（Gap #1 + WR-03 co-fix）</name>
  <files>
    internal/cloudclaude/session.go
  </files>
  <read_first>
    - internal/cloudclaude/session.go line 600-782（runClaudePTYWithReconnect 全函数 + pTYAttachOnce 全函数）—— 修改范围严格限定这两个函数
    - internal/cloudclaude/session.go line 572-598（runClaudeWithSession —— 本 plan zero diff）
    - internal/cloudclaude/session.go line 671-782 **逐行阅读** pTYAttachOnce 现状，特别是 line 715-730（Gap #1 根因段）
    - internal/cloudclaude/reconnect.go（Plan 01 ship）—— NewReconnector / Run / StateAddr / Trigger / ReconnectCount 全部签名
    - internal/cloudclaude/input_buffer.go（Task 5.1 已加 echoMu 但 API zero diff）—— NewBufferedStdin 签名
    - internal/cloudclaude/keepalive.go（RunKeepAlive 签名 —— pTYAttachOnce 内已在 keepCtx 下启动，本 plan 不改）
    - internal/cloudclaude/colors.go（ansiGray / colorEnabled —— 本 plan 不改）
    - .planning/phases/32-ssh-tmux/32-VERIFICATION.md gaps[0] missing#1 全文（本 plan 采纳路径）
    - .planning/phases/32-ssh-tmux/32-REVIEW.md WR-03（多 goroutine 读 os.Stdin —— 本 task 通过 bs 单例天然闭合）
    - .planning/phases/32-ssh-tmux/plans/02-tmux-multiclient/PLAN.md `<interfaces>` 段（Plan 02 原计划的 RegisterStateListener / SetReconnector 接口 —— 本 plan **不走该路径**，用 StateAddr 更简单）
    - .planning/phases/32-ssh-tmux/plans/02-tmux-multiclient/SUMMARY.md decisions[0]（Plan 02 自述 "推迟 v3.1" —— 本 plan 正是 v3.0 内闭合）
  </read_first>
  <acceptance_criteria>
    - `go build ./...` 成功
    - `go vet ./internal/cloudclaude/...` 通过
    - `rg -n "reconnector\\.StateAddr\\(\\)" internal/cloudclaude/session.go` 命中 1 次（**Gap #1 核心断言** —— 共享 atomic 路径生效）
    - **pTYAttachOnce 函数范围内**（grep 需带函数边界判定；执行器可 head-count + rg 验证）`var state atomic\\.Int32` 0 hits（Gap #1 局部 state 被彻底删除）
    - `rg -n "var state atomic\\.Int32" internal/cloudclaude/session.go` 全文 0 hits（简化断言：整个 session.go 都不该再有局部 state）
    - `rg -n "bs\\.Flush\\(\\)" internal/cloudclaude/session.go` 命中 ≥ 1 次（onReconnected 回调或 reconnector.Run 成功后调用）
    - `rg -n "NewBufferedStdin\\(" internal/cloudclaude/session.go` 命中 1 次（仅在 runClaudePTYWithReconnect 循环外一次）
    - `rg -n "NewReconnector\\(" internal/cloudclaude/session.go` 命中 1 次（runClaudePTYWithReconnect 循环外一次；原 line 642 的循环内版本被移除）
    - `rg -n "go bs\\.Run\\(|go func\\(\\) \\{ _ = bs\\.Run" internal/cloudclaude/session.go` 命中 1 次（单个 goroutine，不再 pTYAttachOnce 内 iter 泄漏 —— WR-03 核心断言）
    - pTYAttachOnce 函数签名新增 `bufferedPipeR io.Reader` 参数：`rg -n "func pTYAttachOnce" internal/cloudclaude/session.go` 其参数列表含 `bufferedPipeR io.Reader`
    - `go test ./internal/cloudclaude/... -count=1 -short` PASS（所有既有 22+ 个 session_test 用例）
    - `go test ./internal/cloudclaude/... -count=1 -race -short` PASS（WR-03 + WR-04 综合验证 —— 多 goroutine 读 os.Stdin 已消除）
    - `git diff internal/cloudclaude/session.go | grep '^+' | grep -v '^+++' | wc -l` 输出 ≤ 80；`git diff internal/cloudclaude/session.go | grep '^-' | grep -v '^---' | wc -l` 输出 ≤ 30
  </acceptance_criteria>
  <action>
    ### Step 1 — pTYAttachOnce 函数签名改造

    现状（line 671-673）：

    ```go
    func pTYAttachOnce(ctx context.Context, conn *ssh.Client, remoteCmd, sessionName string,
        sessionCfg SessionConfig, registryPid *int,
    ) (int, error, error) {
    ```

    改为：

    ```go
    // pTYAttachOnce 单次 PTY attach 周期。
    //
    // bufferedPipeR 由 runClaudePTYWithReconnect 外层 NewBufferedStdin 返回的 io.Reader；
    // nil（非 TTY / ReconnectEnabled=false）→ session.Stdin 直接用 os.Stdin
    func pTYAttachOnce(ctx context.Context, conn *ssh.Client, remoteCmd, sessionName string,
        sessionCfg SessionConfig, registryPid *int, bufferedPipeR io.Reader,
    ) (int, error, error) {
    ```

    ### Step 2 — pTYAttachOnce 删除 Gap #1 根因段

    现状（line 715-736）：

    ```go
    // BufferedStdin 注入（CONTEXT D-29 — 共享 Reconnector.StateAddr() 的 *atomic.Int32）。
    // ...
    var state atomic.Int32
    state.Store(int32(StateConnected))
    bs, pipeR := NewBufferedStdin(os.Stdin, &state, os.Stderr, sessionCfg.NoColor, nil)
    bsCtx, cancelBs := context.WithCancel(ctx)
    defer cancelBs()
    go func() { _ = bs.Run(bsCtx) }()
    defer bs.Close()

    if isTTY {
        session.Stdin = pipeR
    } else {
        session.Stdin = os.Stdin
    }
    session.Stdout = os.Stdout
    session.Stderr = os.Stderr
    ```

    改为：

    ```go
    // [Phase 32 Gap #1 fix] BufferedStdin 在 runClaudePTYWithReconnect 外层单例创建；
    // 此处只消费 bufferedPipeR 作为 session.Stdin（共享 Reconnector.StateAddr 的 atomic 由外层保证）。
    if isTTY && bufferedPipeR != nil {
        session.Stdin = bufferedPipeR
    } else {
        session.Stdin = os.Stdin
    }
    session.Stdout = os.Stdout
    session.Stderr = os.Stderr
    ```

    **完整删除** 的内容：
    - `var state atomic.Int32`
    - `state.Store(int32(StateConnected))`
    - `bs, pipeR := NewBufferedStdin(...)`
    - `bsCtx, cancelBs := context.WithCancel(ctx)` + `defer cancelBs()`
    - `go func() { _ = bs.Run(bsCtx) }()` ← **WR-03 co-fix 核心**
    - `defer bs.Close()`

    以及原先 `atomic` 包 import 如果只被这里使用，可能需要移除（执行器 `go vet` / `goimports` 自动处理）。

    ### Step 3 — runClaudePTYWithReconnect 循环外新增 Reconnector + BufferedStdin 单例

    现状（line 611-661）：

    ```go
    func runClaudePTYWithReconnect(ctx context.Context, initialConn *ssh.Client, sshCfg SSHConfig,
        remoteCmd, sessionName string, sessionCfg SessionConfig,
    ) (int, error) {
        conn := initialConn
        reconnectCount := 0
        registryPid := 0

        defer func() {
            if registryPid > 0 {
                _ = removeClientFile(conn, registryPid)
            }
        }()

        for {
            exitCode, exitErr, reconnectableErr := pTYAttachOnce(ctx, conn, remoteCmd, sessionName, sessionCfg, &registryPid)
            ...
            reconnector := NewReconnector(sshCfg, ...)
            if err := reconnector.Run(ctx); err != nil { ... }
            ...
        }
    }
    ```

    改为：

    ```go
    func runClaudePTYWithReconnect(ctx context.Context, initialConn *ssh.Client, sshCfg SSHConfig,
        remoteCmd, sessionName string, sessionCfg SessionConfig,
    ) (int, error) {
        conn := initialConn
        reconnectCount := 0
        registryPid := 0

        // [Phase 32 Gap #1 fix] Reconnector + BufferedStdin 单例创建在循环外：
        //   - reconnector.StateAddr() 返回的 *atomic.Int32 被 BufferedStdin 共享
        //   - 断网时 Reconnector.Run 写 StateReconnecting → BufferedStdin 立即切 ringBuf + 灰色 echo
        //   - 重连成功时 Reconnector.Run 写 StateConnected → onReconnected 回调 bs.Flush() 把 ringBuf 按序写 pipeW
        //   - 单个 bs.Run goroutine 跨所有 attach 周期读 os.Stdin（WR-03 co-fix：消除 pTYAttachOnce 每次 iter 泄漏新 goroutine 的问题）
        var reconnector *Reconnector
        var bufferedPipeR io.Reader
        var bs *BufferedStdin
        isTTY := term.IsTerminal(int(os.Stdin.Fd()))
        if isTTY && sessionCfg.ReconnectEnabled {
            reconnector = NewReconnector(sshCfg,
                nil, // onConnLost — Reconnector.Run 内部已 state.Store(StateReconnecting)
                nil, // onReconnected — 循环内按需赋值（需访问 newConn 局部变量）
                os.Stderr, sessionCfg.NoColor)
            bs, bufferedPipeR = NewBufferedStdin(os.Stdin, reconnector.StateAddr(), os.Stderr, sessionCfg.NoColor, reconnector.Trigger)
            bsCtx, cancelBs := context.WithCancel(ctx)
            defer cancelBs()
            go func() { _ = bs.Run(bsCtx) }()
            defer bs.Close()
        }

        defer func() {
            if registryPid > 0 {
                _ = removeClientFile(conn, registryPid)
            }
        }()

        for {
            exitCode, exitErr, reconnectableErr := pTYAttachOnce(ctx, conn, remoteCmd, sessionName, sessionCfg, &registryPid, bufferedPipeR)

            if exitErr == nil {
                writeLastSessionReconnectCount(sessionCfg.LastSessionPath, reconnectCount)
                if registryPid > 0 {
                    _ = removeClientFile(conn, registryPid)
                    registryPid = 0
                }
                return exitCode, nil
            }

            if reconnectableErr == nil || !sessionCfg.ReconnectEnabled || reconnector == nil {
                return 0, exitErr
            }

            t0 := time.Now()
            var newConn *ssh.Client
            // 每次重连前重置 Reconnector 的 onReconnected（newConn 指针对应本轮循环的局部变量）
            // 注：NewReconnector 当前接口把 onReconnected 作为构造参数；本 plan 保持 Plan 01 API zero diff，
            //     采用闭包赋值+重用模式：在循环内 "reset" reconnector 的内部字段需要 Plan 01 提供 setter，或
            //     每次重连 NewReconnector 一次但共享同一个 state atomic。
            //
            // 本 plan 选：**每次重连 new 一个 Reconnector（新 onReconnected 闭包），但**共享同一个 state atomic **。
            //     —— 通过向 NewReconnector 注入 bs 指针，onReconnected 内部 bs.Flush 后返回 nil。
            //     state atomic 在 bs 已持有指针（bs.state 字段），所以只要新 reconnector 的 state 指针和 bs 相同即可。
            //
            // **重要约束**：NewReconnector 当前把 r.state 作为内部字段（atomic.Int32 值类型），StateAddr 返回 &r.state。
            //     每次 new Reconnector 都会产生一个新的 atomic.Int32 —— 与 bs.state 指针 diverge。
            //     **所以不能每次重连 new 新 Reconnector。**
            //
            // 实际方案：reconnector 单例 + reconnector.Run 一次 Run 一次重连 + onReconnected 通过 closure 读 newConn。
            //     Plan 01 NewReconnector 签名 `(cfg, onConnLost, onReconnected, statusWriter, noColor)`；
            //     onReconnected 一旦在 New 时定死，后续 Run 无法改。
            //     → 需要把 onReconnected 写成能访问"动态可变状态"的闭包：在循环外用指针变量承载 newConn。
            //
            // 采纳方案：在 runClaudePTYWithReconnect 函数头声明 `var pendingNewConn *ssh.Client`；
            //     NewReconnector 时 onReconnected = func(c) error { pendingNewConn = c; if bs != nil { bs.Flush() }; return nil }
            //     循环内 reconnector.Run 返回 nil 后，conn = pendingNewConn（取出）并清零 pendingNewConn
            //
            // 以上说明是执行器的**推理过程**；下面给出最终代码。
            _ = newConn // 占位，实际实现不使用

            if err := reconnector.Run(ctx); err != nil {
                if errors.Is(err, ErrReconnectGaveUp) {
                    fmt.Fprintln(os.Stderr, FormatGiveUpMessage(5, time.Since(t0)))
                    writeLastSessionReconnectCount(sessionCfg.LastSessionPath, reconnectCount)
                    return ExitNetworkError, nil
                }
                return 0, err
            }
            reconnectCount += reconnector.ReconnectCount()
            conn = pendingNewConn  // 见下方重构最终版
            pendingNewConn = nil
            registryPid = 0
        }
    }
    ```

    ### Step 4 — 重构定稿（最终代码，执行器复制这一版）

    **最终 runClaudePTYWithReconnect 完整代码**（替换现状 line 611-661 整块）：

    ```go
    func runClaudePTYWithReconnect(ctx context.Context, initialConn *ssh.Client, sshCfg SSHConfig,
        remoteCmd, sessionName string, sessionCfg SessionConfig,
    ) (int, error) {
        conn := initialConn
        reconnectCount := 0
        registryPid := 0

        // [Phase 32 Gap #1] Reconnector + BufferedStdin 单例（循环外）。
        //   共享原则：
        //     - bs 通过 reconnector.StateAddr() 得到 *atomic.Int32；Reconnector.Run 写 state
        //       就是 bs 读到的 state（同一指针）
        //     - bs.Run 单 goroutine 读 os.Stdin（WR-03 co-fix：不再每次 iter 泄漏新 goroutine）
        //     - reconnect 成功的 onReconnected 回调内 bs.Flush() —— 让 ringBuf 字节按序写到 pipeW，
        //       下一轮 pTYAttachOnce 的 session.Stdin = bufferedPipeR 立即读到（无丢字 / 无乱序）
        var reconnector *Reconnector
        var bufferedPipeR io.Reader
        var bs *BufferedStdin
        var pendingNewConn *ssh.Client
        isTTY := term.IsTerminal(int(os.Stdin.Fd()))
        if isTTY && sessionCfg.ReconnectEnabled {
            reconnector = NewReconnector(sshCfg,
                nil, // onConnLost
                func(c *ssh.Client) error {
                    pendingNewConn = c
                    if bs != nil {
                        _ = bs.Flush() // 按序把断网期 ringBuf 写到 pipeW
                    }
                    return nil
                },
                os.Stderr, sessionCfg.NoColor)
            bs, bufferedPipeR = NewBufferedStdin(os.Stdin, reconnector.StateAddr(), os.Stderr,
                sessionCfg.NoColor, reconnector.Trigger)
            bsCtx, cancelBs := context.WithCancel(ctx)
            defer cancelBs()
            go func() { _ = bs.Run(bsCtx) }()
            defer bs.Close()
        }

        defer func() {
            if registryPid > 0 {
                _ = removeClientFile(conn, registryPid)
            }
        }()

        for {
            exitCode, exitErr, reconnectableErr := pTYAttachOnce(ctx, conn, remoteCmd, sessionName, sessionCfg, &registryPid, bufferedPipeR)

            if exitErr == nil {
                writeLastSessionReconnectCount(sessionCfg.LastSessionPath, reconnectCount)
                if registryPid > 0 {
                    _ = removeClientFile(conn, registryPid)
                    registryPid = 0
                }
                return exitCode, nil
            }

            if reconnectableErr == nil || !sessionCfg.ReconnectEnabled || reconnector == nil {
                return 0, exitErr
            }

            t0 := time.Now()
            if err := reconnector.Run(ctx); err != nil {
                if errors.Is(err, ErrReconnectGaveUp) {
                    fmt.Fprintln(os.Stderr, FormatGiveUpMessage(5, time.Since(t0)))
                    writeLastSessionReconnectCount(sessionCfg.LastSessionPath, reconnectCount)
                    return ExitNetworkError, nil
                }
                return 0, err
            }
            reconnectCount += reconnector.ReconnectCount()
            if pendingNewConn != nil {
                conn = pendingNewConn
                pendingNewConn = nil
            }
            registryPid = 0 // 旧 client_pid 已失效（新 conn / 新 tmux attach 周期）
        }
    }
    ```

    **pTYAttachOnce 完整改造后**（保留函数其它内容，仅更新 BufferedStdin 相关段 + 函数签名）：

    对照 session.go 现状 line 671-782，改动位置：
    1. 函数签名（line 671-673）新增 `bufferedPipeR io.Reader` 参数
    2. line 715-736 的 `var state atomic.Int32 ... session.Stderr = os.Stderr` 段改为 Step 2 列出的简化版
    3. 其它所有代码（PTY / SIGWINCH / keepCtx / session.Start / writeClientFile goroutine / session.Wait / return 三分支）**一字不动**

    ### Step 5 — 移除不再使用的 import

    删除 pTYAttachOnce 局部 state 后，`"sync/atomic"` import 如果 session.go 无其它使用点应移除。执行器 `goimports -w internal/cloudclaude/session.go` 自动处理。

    ### Step 6 — runClaudeWithSession zero diff 验证

    `runClaudeWithSession`（line 572-598）**完全不改**；执行器 `git diff` 确认 line 572-598 范围内无变更。
  </action>
  <verify>
    <automated>go build ./... &amp;&amp; go vet ./internal/cloudclaude/... &amp;&amp; go test ./internal/cloudclaude/... -count=1 -short &amp;&amp; rg -q "reconnector\\.StateAddr\\(\\)" internal/cloudclaude/session.go &amp;&amp; rg -q "bs\\.Flush\\(\\)" internal/cloudclaude/session.go &amp;&amp; ! rg -q "var state atomic\\.Int32" internal/cloudclaude/session.go &amp;&amp; rg -c "NewReconnector\\(" internal/cloudclaude/session.go | grep -q "^1$" &amp;&amp; rg -c "NewBufferedStdin\\(" internal/cloudclaude/session.go | grep -q "^1$"</automated>
  </verify>
  <done>
    - pTYAttachOnce 不再拥有 BufferedStdin 生命周期；新增 bufferedPipeR io.Reader 参数；局部 `var state atomic.Int32` 彻底消失（Gap #1 根因删除）
    - runClaudePTYWithReconnect 循环外一次性创建 Reconnector + BufferedStdin；BufferedStdin 通过 `reconnector.StateAddr()` 共享同一 atomic.Int32
    - onReconnected 回调 bs.Flush() + pendingNewConn 承载 —— reconnect 成功后 ringBuf 字节按序写 pipeW，下一轮 pTYAttachOnce session.Stdin = bufferedPipeR 读到（无丢字 / 无乱序）
    - `go bs.Run(bsCtx)` 单 goroutine 跨所有 attach 周期运行（WR-03 co-fix：不再每次 iter 泄漏新 goroutine 读 os.Stdin）
    - runClaudeWithSession zero diff；session.go 其它所有函数 zero diff（包括 DetectTmux / buildTmuxRemoteCmd / writeClientFile / performTakeOver / printAttachBanner / RunSessionsLs/Attach / runClaudePTYBare / writeLastSessionTmuxField 等）
    - 既有 22+ 个 session_test.go 用例全 PASS；race mode 额外 PASS
  </done>
</task>

<task type="auto">
  <name>Task 5.3: 新增 TestPTYReconnect_BufferedInputFlush 集成级单测（SC5 端到端断言）</name>
  <files>
    internal/cloudclaude/session_test.go
  </files>
  <read_first>
    - internal/cloudclaude/session.go（Task 5.2 重构后的 runClaudePTYWithReconnect / pTYAttachOnce 完整实现）
    - internal/cloudclaude/reconnect.go（Reconnector / ConnState / StateReconnecting / StateConnected / StateAddr —— 测试直接操控 state）
    - internal/cloudclaude/input_buffer.go（Task 5.1 加 echoMu 后的 BufferedStdin —— 测试复用已有 NewBufferedStdin API）
    - internal/cloudclaude/session_test.go（现有 22+ 个测试模式 —— 优先复用 bytes.Buffer / io.Pipe / atomic.Int32 直接注入方式，避免 ssh.Client mock）
    - .planning/phases/32-ssh-tmux/32-VERIFICATION.md gaps[0] missing#3（测试要求原文："fake conn.Wait 返回 io.EOF 触发 reconnect → 在断网期用 io.Pipe 喂 'abc' 到 BufferedStdin → assert echo 含 ansiGray + ringBuf 非空 → reconnect 成功后 pipeR 读到 'abc'"）
  </read_first>
  <acceptance_criteria>
    - `rg -n "func TestPTYReconnect_BufferedInputFlush" internal/cloudclaude/session_test.go` 命中 1 次
    - `go test -run TestPTYReconnect_BufferedInputFlush ./internal/cloudclaude/...` 退出码 0 且用例 PASS
    - `go test -run TestPTYReconnect_BufferedInputFlush ./internal/cloudclaude/... -race` PASS
    - 测试代码断言（必须全部包含）：
      1. 断网期间 echo buffer 含 `ansiGray` 字节（\x1b[90m）
      2. BufferedStdin.ringBuf 在 reconnect 前非空
      3. reconnect 成功后（通过手动 state.Store(StateConnected) + 调 Flush 模拟）pipeR 能读到原始字符串（"abc"）
      4. reconnect 完成后 echo buffer 含 `ansiReset` 字节（\x1b[0m）—— 证明 closeGrayIfOpen 被调用
    - `go test ./internal/cloudclaude/... -count=1 -short` PASS（所有既有测试 + 本新测试）
  </acceptance_criteria>
  <action>
    ### 测试设计决策

    gaps[0] missing#3 要求"fake conn.Wait 返回 io.EOF 触发 reconnect"，但 mock 完整 ssh.Client / ssh.Session 工程量极大且与 Plan 03 `defaultFixtureSSHConfig` 需真实 docker fixture 冲突。

    本 plan 采用**等价断言路径**：
    - 直接测试 BufferedStdin + 手动操控 state atomic（模拟 Reconnector.Run 的行为），**不**测 runClaudePTYWithReconnect 的完整 ssh.Client 拨号路径
    - 该测试等价验证 SC5 的**核心机制**：state 被写为 Reconnecting → BufferedStdin 缓冲 + echo gray → state 写回 Connected + Flush → pipeW 按序写入

    完整 ssh.Client 拨号路径的端到端验收留 Phase 35 真机 UAT（与 32-VERIFICATION.md human_verification#1 一致）。

    ### 新增测试代码

    追加到 `internal/cloudclaude/session_test.go` 末尾：

    ```go
    // TestPTYReconnect_BufferedInputFlush 验证 Gap #1 闭合（SC5 / REQ-F3-B）。
    //
    // 等价测试路径（不走真实 ssh.Client，避免 mock 完整 ssh stack）：
    //   1. 手工创建 Reconnector + BufferedStdin 单例（mirror runClaudePTYWithReconnect 循环外逻辑）
    //   2. state.Store(StateReconnecting) 模拟 Reconnector.Run 入口
    //   3. io.Pipe 喂 "abc" 到 BufferedStdin.src
    //   4. 断言 echo buffer 含 ansiGray + ringBuf 非空
    //   5. state.Store(StateConnected) + bs.Flush() 模拟 Reconnector.Run 成功回调
    //   6. 断言 pipeR 读到 "abc" + echo buffer 含 ansiReset
    //
    // 完整 ssh.Client 拨号路径的端到端验收留 Phase 35 真机 UAT（human_verification#1）。
    func TestPTYReconnect_BufferedInputFlush(t *testing.T) {
        // src → bs → pipeR（pipeR 扮演 ssh.Session.Stdin 的消费方）
        srcR, srcW := io.Pipe()
        var echo bytes.Buffer

        // mirror runClaudePTYWithReconnect 外层：NewReconnector + NewBufferedStdin 共享 state
        reconnector := NewReconnector(SSHConfig{},
            nil,
            func(c *ssh.Client) error { return nil },
            &bytes.Buffer{}, // statusWriter 丢弃
            true)            // noColor=true 是为了避免 renderStatus 干扰；但 BufferedStdin 仍需 noColor=false 才输出 ansiGray
        bs, pipeR := NewBufferedStdin(srcR, reconnector.StateAddr(), &echo, false, reconnector.Trigger)

        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()
        bsDone := make(chan error, 1)
        go func() { bsDone <- bs.Run(ctx) }()
        defer bs.Close()

        // Phase 1: 模拟 Reconnector.Run 入口 —— state=Reconnecting
        reconnector.StateAddr().Store(int32(StateReconnecting))

        // 喂 "abc" 到 src
        go func() {
            _, _ = srcW.Write([]byte("abc"))
        }()

        // 等一小段时间让 bs.Run 逐字节读并 handleReconnecting
        time.Sleep(100 * time.Millisecond)

        // 断言 1：echo buffer 含 ansiGray（灰色未确认开头）
        if !bytes.Contains(echo.Bytes(), []byte(ansiGray)) {
            t.Errorf("echo buffer 应含 ansiGray，实际: %q", echo.String())
        }

        // 断言 2：echo buffer 含原始字符（"abc"）
        for _, c := range []byte("abc") {
            if !bytes.Contains(echo.Bytes(), []byte{c}) {
                t.Errorf("echo buffer 应含 %q，实际: %q", string(c), echo.String())
            }
        }

        // 断言 3：ringBuf 非空（3 bytes "abc"）
        bs.ringMu.Lock()
        ringLen := len(bs.ringBuf)
        ringContent := string(bs.ringBuf)
        bs.ringMu.Unlock()
        if ringLen != 3 || ringContent != "abc" {
            t.Errorf("ringBuf 应含 \"abc\"（len=3），实际 len=%d content=%q", ringLen, ringContent)
        }

        // Phase 2: 模拟 Reconnector.Run 成功回调 —— state=Connected + Flush
        reconnector.StateAddr().Store(int32(StateConnected))
        // 在另一 goroutine 读 pipeR（io.Pipe Flush 会阻塞直到有读者）
        readDone := make(chan []byte, 1)
        go func() {
            buf := make([]byte, 16)
            n, _ := pipeR.Read(buf)
            readDone <- buf[:n]
        }()
        time.Sleep(50 * time.Millisecond) // 让读 goroutine 就绪

        if err := bs.Flush(); err != nil {
            t.Fatalf("Flush 失败: %v", err)
        }

        // 断言 4：pipeR 读到 "abc"（按序 / 无丢字）
        select {
        case got := <-readDone:
            if string(got) != "abc" {
                t.Errorf("pipeR 读到 %q，期望 \"abc\"", string(got))
            }
        case <-time.After(2 * time.Second):
            t.Fatal("pipeR 未在 2s 内读到数据 —— Flush 路径未生效（Gap #1 未闭合）")
        }

        // 断言 5：Flush 之后 ringBuf 清空
        bs.ringMu.Lock()
        if len(bs.ringBuf) != 0 {
            t.Errorf("Flush 后 ringBuf 应清空，实际 len=%d", len(bs.ringBuf))
        }
        bs.ringMu.Unlock()

        // 断言 6：Flush 之后 echo buffer 含 ansiReset（closeGrayIfOpen 被调用，灰色关闭）
        if !bytes.Contains(echo.Bytes(), []byte(ansiReset)) {
            t.Errorf("Flush 后 echo buffer 应含 ansiReset，实际: %q", echo.String())
        }
    }
    ```

    ### 注意事项

    - `ansiGray` / `ansiReset` 是同包常量，直接引用
    - `bs.ringBuf` / `bs.ringMu` 是同包未导出字段，测试在同一 package cloudclaude 直接访问
    - `reconnector.StateAddr().Store(int32(StateReconnecting))` 直接改 atomic —— 模拟 Reconnector.Run 入口行为而不实际调 Run（避免 Run 尝试真实 sshConnect 拨号阻塞）
    - 测试 `noColor=true` 会抑制 ansiGray 输出；必须 `NewBufferedStdin(..., false, ...)`（`noColor` 第 4 个参数）才能断言灰色 escape
    - import 追加（如缺）：`io`（io.Pipe） / `bytes` / `context` / `time` / `testing` —— session_test.go 现有测试应已用到
  </action>
  <verify>
    <automated>go test -run TestPTYReconnect_BufferedInputFlush ./internal/cloudclaude/... -count=1 &amp;&amp; go test -run TestPTYReconnect_BufferedInputFlush ./internal/cloudclaude/... -race &amp;&amp; go test ./internal/cloudclaude/... -count=1 -short</automated>
  </verify>
  <done>
    - TestPTYReconnect_BufferedInputFlush 覆盖 SC5 端到端核心机制（ansiGray echo + ringBuf 缓冲 + Flush 按序写 pipeW + ansiReset 关闭）
    - 测试 race mode PASS（WR-03 + WR-04 综合验证）
    - 既有 22+ 个 session_test.go 用例 + Task 5.1 input_buffer_test.go 5 个用例全 PASS
    - 完整 ssh.Client 拨号 reconnect 路径验收留 Phase 35 真机 UAT（32-VERIFICATION.md human_verification#1）
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| os.Stdin → BufferedStdin.src | 用户键入字符经 io.Pipe 中转写入 SSH session.Stdin；本 plan 改为循环外单 goroutine 读取，消除 WR-03 的多 goroutine 并发读 syscall |
| BufferedStdin.pipeW → ssh.Session.Stdin | 不变（Plan 01 信任边界）；本 plan 仅添加 mutex 保护 pipeW 写入的并发安全 |
| Reconnector state atomic → BufferedStdin.state 指针 | 共享 *atomic.Int32 跨 goroutine 读写（Plan 01 已用 atomic 包 lock-free 保证）；本 plan 只是复用该指针，不改 atomic 语义 |
| BufferedStdin.ringBuf | 仍是进程内 byte slice；WR-04 co-fix 添加 echoMu 防止 Flush 与 handleReconnecting 间 data race，**不改**信任边界本身 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-32G1-01 | Tampering | BufferedStdin echoMu 锁顺序（嵌套 Flush: echoMu→ringMu） | mitigate | 锁顺序文档化（见 Task 5.1 Step 6）；handleReconnecting 串行两锁 / Flush 嵌套两锁 / Run-Connected 单锁，无死锁路径 |
| T-32G1-02 | DoS | bs.Run 单 goroutine 读 os.Stdin 阻塞不退出 | accept | 与 Plan 01 风险同等级 —— bs.Run 的阻塞在 syscall.Read 是 Go 标准模式；ctx.Done 在下次 Read 返回后才被检查（≤ 1 byte 延迟）；cancelBs 在 defer 中调用，cancelBs 关闭 bsCtx 后 bs.Run 下一次 select 看到 ctx.Done 退出；cloud-claude 进程退出时 os.Stdin 被系统关闭，Read 返回 EOF 让 bs.Run return nil |
| T-32G1-03 | InformationDisclosure | Reconnector.StateAddr() 暴露 *atomic.Int32 让外部包写入 state | accept | StateAddr 在 Plan 01 已 public 暴露（reconnect.go:81-82）；本 plan 只是从 session.go 调用而非新引入；同包内部 API（cloudclaude 包内）安全态势不变 |
| T-32G1-04 | Tampering | pTYAttachOnce 新增 bufferedPipeR io.Reader 参数可能被调用方传 nil 或任意 io.Reader | accept | pTYAttachOnce 是同包未导出函数，唯一调用方是 runClaudePTYWithReconnect；nil-guard（`if bufferedPipeR != nil`）已在 Step 2 加；type 签名保证非 io.Reader 传入编译失败 |

**Severity 分布（block_on=high）**：
- High: 0
- Medium: T-32G1-01（锁死锁 / 已文档化 mitigate）
- Low: T-32G1-02 / T-32G1-03 / T-32G1-04

无 High 项 → 不阻断 plan 执行。

**本 plan 不新增信任边界**；修复方向是让 Plan 01 已设计但未激活的 ringBuf / 灰色 echo 分支真实运作。SC5 端到端安全态势从"不存在"升级为"存在"。
</threat_model>

<verification>
本 plan 完成后：

1. **Gap #1 闭合 grep 断言**：
   - `rg "reconnector\\.StateAddr\\(\\)" internal/cloudclaude/session.go` ≥ 1 hit（共享 atomic 路径生效）
   - `rg "var state atomic\\.Int32" internal/cloudclaude/session.go` = 0 hits（局部 state 根因删除）
   - `rg "bs\\.Flush\\(\\)" internal/cloudclaude/session.go` ≥ 1 hit（onReconnected 回调）

2. **SC5（REQ-F3-B）code-level 证据**：`TestPTYReconnect_BufferedInputFlush` 用例 PASS，断言全 6 条：
   - 断网期间 echo 含 ansiGray
   - echo 含原始字符
   - ringBuf 非空（len=3 content="abc"）
   - Flush 后 pipeR 读到 "abc"（按序 / 无丢字）
   - Flush 后 ringBuf 清空
   - Flush 后 echo 含 ansiReset

3. **WR-03 co-fix 验证**：
   - `rg "go bs\\.Run\\(|go func\\(\\) \\{ _ = bs\\.Run" internal/cloudclaude/session.go` 命中 1 次（单 goroutine）
   - session.go 在 pTYAttachOnce 函数范围内无 `bs.Run` 调用

4. **WR-04 co-fix 验证**：
   - `rg "echoMu\\s+sync\\.Mutex" internal/cloudclaude/input_buffer.go` 命中 1 次
   - `go test -race ./internal/cloudclaude/... -run "TestBufferedStdin|TestPTYReconnect_BufferedInputFlush"` PASS

5. **Plan 01 / 02 / 03 / 04 回归**：`go test ./internal/cloudclaude/... -count=1 -short` 全 PASS（现有 22+ 个 session_test + 5 个 input_buffer_test + Phase 31 回归测试全部不变）

**综合 verify 命令**：

```bash
go build ./... \
  && go vet ./internal/cloudclaude/... \
  && go test ./internal/cloudclaude/... -count=1 -short \
  && go test ./internal/cloudclaude/... -count=1 -race -short \
  && rg -q "reconnector\\.StateAddr\\(\\)" internal/cloudclaude/session.go \
  && rg -q "bs\\.Flush\\(\\)" internal/cloudclaude/session.go \
  && ! rg -q "var state atomic\\.Int32" internal/cloudclaude/session.go \
  && rg -q "echoMu\\s+sync\\.Mutex" internal/cloudclaude/input_buffer.go \
  && go test -run TestPTYReconnect_BufferedInputFlush ./internal/cloudclaude/... -v
```

**human_verification**：SC5 的真实 30s 拔网端到端 UAT（`docker network disconnect` + tmux capture-pane 对比前后无丢字 / 无乱序）留 Phase 35 真机 —— 与 32-VERIFICATION.md human_verification#1 / #5 一致。
</verification>

<success_criteria>
- session.go diff ≤ 80 新增 / ≤ 30 删除（主要集中在 runClaudePTYWithReconnect 循环外 + pTYAttachOnce 简化）
- input_buffer.go diff ≤ 20 新增（echoMu 字段 + 3-4 处 Lock/Unlock）；公开 API 签名 zero diff
- 新增 1 个测试用例（TestPTYReconnect_BufferedInputFlush ~80 行）
- 零新依赖（go.mod / go.sum 无变化）
- 全平台 `go build ./...` PASS（linux / darwin / windows）
- Gap #1 grep 验证（上方 verification 段 1 条全过）
- WR-03 验证：pTYAttachOnce 范围内无 `bs.Run`；runClaudePTYWithReconnect 外层 `go bs.Run` 唯一调用
- WR-04 验证：`go test -race ./internal/cloudclaude/...` PASS
- 既有 22+ 个 session_test + 5 个 input_buffer_test 用例全 PASS
- runClaudeWithSession（Plan 02）/ Plan 03 ssh.go 注入 / Plan 04 mount_strategy 改动零连带影响
</success_criteria>

<output>
完成后，create `.planning/phases/32-ssh-tmux/plans/05-bufferedstdin-reconnect-wiring/SUMMARY.md` 描述：
- runClaudePTYWithReconnect 重构前后的时序对比（BufferedStdin 生命周期从 "per attach" 升级为 "per cloud-claude process"）
- Gap #1 根因删除证据（pTYAttachOnce line 715-730 的 8 行 `var state atomic.Int32` 块 → 1 行 `session.Stdin = bufferedPipeR`）
- Reconnector onReconnected 闭包 + pendingNewConn 指针承载的设计论证（为何不用每次重连 new 新 Reconnector）
- input_buffer.go echoMu 锁顺序表（Run-Connected / handleReconnecting / Flush 三条路径）
- TestPTYReconnect_BufferedInputFlush 的 6 条断言分别对应 SC5 的哪个观察点
- WR-03 / WR-04 co-fix 验证证据（grep 命中数 + race mode PASS 输出）
- 留给 /gsd-verify-phase 的断言清单（Gap #1 完全闭合 + SC5 code-level 从 ✗ 转 ✓；剩余 human_verification#1 留 Phase 35）
- 完整 ssh.Client 拨号 reconnect 路径的 Phase 35 UAT 挂钩说明（docker network disconnect 30s + tmux capture-pane 断言）
</output>
