---
phase: 32-ssh-tmux
reviewed: 2026-04-20T00:00:00Z
depth: standard
files_reviewed: 24
files_reviewed_list:
  - cmd/cloud-claude/main.go
  - cmd/cloud-claude/sessions.go
  - internal/cloudclaude/colors.go
  - internal/cloudclaude/errcodes/codes.go
  - internal/cloudclaude/errcodes/net.go
  - internal/cloudclaude/errcodes/session.go
  - internal/cloudclaude/input_buffer.go
  - internal/cloudclaude/input_buffer_test.go
  - internal/cloudclaude/integration_test.go
  - internal/cloudclaude/keepalive.go
  - internal/cloudclaude/keepalive_darwin.go
  - internal/cloudclaude/keepalive_linux.go
  - internal/cloudclaude/keepalive_other.go
  - internal/cloudclaude/keepalive_test.go
  - internal/cloudclaude/last_session.go
  - internal/cloudclaude/last_session_test.go
  - internal/cloudclaude/mount_strategy.go
  - internal/cloudclaude/reconnect.go
  - internal/cloudclaude/reconnect_test.go
  - internal/cloudclaude/session.go
  - internal/cloudclaude/session_test.go
  - internal/cloudclaude/ssh.go
  - internal/cloudclaude/sync_lock.go
  - internal/cloudclaude/sync_lock_test.go
findings:
  critical: 0
  warning: 5
  info: 7
  total: 12
status: minor
---

# Phase 32: Code Review Report

**Reviewed:** 2026-04-20
**Depth:** standard
**Files Reviewed:** 24
**Status:** minor (no critical / security issues; carry-over integration gaps tracked separately)

## Summary

Phase 32 落地的三块代码（KeepAlive + Reconnector + BufferedStdin / tmux 多端 + sessions cobra / 账号级 flock 单例锁）整体质量较高：错误码全部走 `errcodes.Format`、远程命令一律 `shellescape.Quote`、跨平台 keepalive 拆分干净、`sync_lock.go` 用 `flock -F` 解决了 SSH 收割的关键 pitfall。文案命名全部满足正则与 ≤80 runes 约束。

主要风险集中在 **PTY/Reconnect 并发协同的 goroutine 生命周期** 上：

- `pTYAttachOnce` 每轮重连泄漏 1 个 SIGWINCH goroutine（`for range sigCh`，channel 永不 close）；同问题在 `runClaudePTYBare` / `ssh.go::runClaude`。
- `runClaudePTYWithReconnect` 主 goroutine 与 `writeClientFile` 异步 goroutine 共享 `*registryPid`，无任何同步原语 → race。
- 多次 attach 时 `BufferedStdin.Run` 的旧 goroutine 仍在阻塞 `os.Stdin.Read`，新一轮 attach 启动新 `bs.Run`，两端并发 `os.Stdin.Read` → 输入字节会被任一 goroutine 抢走（focus #6 提的 stdin 竞争是真实的）。
- `BufferedStdin.localEcho` / `b.grayOpen` 在 `Run` 与 `Flush` 跨 goroutine 共享读写，无 mutex 覆盖（当前 plan 未实际激活 Flush，所以暂未爆雷，但属于代码内已存在的 race）。

Carry-over gaps（已说明，本报告不计入 findings）：
- BufferedStdin 未与 `Reconnector.StateAddr()` 共享 → REQ-F3-B 未端到端激活。
- `mount_strategy.MountWorkspace` 未调 `mountCfg.SyncSessionLock` → REQ-F5-D 未端到端激活。

`buildTmuxRemoteCmd` 的 fallback 分支（`|| exec <wrapCmd 字面值>`）会触发 `exec cd ...`，bash 拒绝对 builtin 做 exec，fallback 实际不可用；好在主 `command -v tmux` 短路通常使其不被命中，仍建议修复。

其余为 info 级建议：`sync_lock.go::lockPath` 缺少 path-traversal 防御、`WriteLastSession` 用固定 `.tmp` 后缀（未来并发写会冲突）、`keepalive.go` 用 `time.After` 不可取消、两个 error 返回签名等。

---

## Warnings

### WR-01: SIGWINCH goroutine 在每次 attach 后泄漏（channel 永不关闭）

**File:** `internal/cloudclaude/session.go:703-712` （`pTYAttachOnce`）

`signal.Notify(sigCh, syscall.SIGWINCH)` + `for range sigCh { ... }` + `defer signal.Stop(sigCh)`：`signal.Stop` 只是停止信号投递，**不会 close channel**。`for range sigCh` 永远阻塞在 receive，直到进程退出。`pTYAttachOnce` 在每轮 reconnect 都会再开一个新的 `sigCh` + 新 goroutine → 重连 N 次就泄漏 N 个 goroutine。

同样问题在 `runClaudePTYBare:815-823` 和 `ssh.go::runClaude:265-273`（前者每次 sessions attach 调用泄漏 1 个；后者 v2.0 路径每个进程仅 1 个，影响小）。

**额外副作用**：泄漏 goroutine 在收到 SIGWINCH 时会调 `session.WindowChange(...)`，但其 session 已 Close，调用静默失败但仍占 CPU/IO 路径。

**Fix:** 显式 close channel 并退出 goroutine。推荐用 ctx 关停，避免对 `os/signal` 内部依赖：

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGWINCH)
sigCtx, cancelSig := context.WithCancel(ctx)
defer cancelSig()
defer signal.Stop(sigCh)
go func() {
    for {
        select {
        case <-sigCtx.Done():
            return
        case <-sigCh:
            if w, h, gerr := term.GetSize(fd); gerr == nil {
                _ = session.WindowChange(h, w)
            }
        }
    }
}()
```

`pTYAttachOnce`、`runClaudePTYBare`、`ssh.go::runClaude` 三处都需改。

---

### WR-02: `*registryPid` 在主 goroutine 与 writeClientFile goroutine 之间存在 data race

**File:** `internal/cloudclaude/session.go:614-660, 671-766`

`runClaudePTYWithReconnect` 声明 `registryPid := 0` 并把 `&registryPid` 传给 `pTYAttachOnce`。后者在 `session.Start` 后启动 goroutine 调 `writeClientFile`，该 goroutine 写 `*registryPid = pid`（line 764），同时：

1. `runClaudePTYWithReconnect` 的 deferred `removeClientFile(conn, registryPid)`（line 619-621）在函数返回时读 `registryPid`；
2. 正常退出分支 line 629-632 读 `registryPid`；
3. 重连成功后 line 659 重置 `registryPid = 0`，下一轮 `pTYAttachOnce` 又会读 `*registryPid == 0` 决定是否启动新的 writeClientFile goroutine。

`writeClientFile` 远程跑 `tmux display-message` + 写 JSON，可能耗时几百 ms 到秒级；`pTYAttachOnce` 在 session.Wait 提前结束（远端立即崩）的极端情况下可能在 goroutine 写完前就返回，于是：

- 主 goroutine 可能读到旧的 0 → cleanup 漏调 `removeClientFile`，留下孤儿 `<pid>.json`；
- 或主 goroutine 已 reset 0、goroutine 才追写 pid → 下一轮 attach 看到 `*registryPid != 0` → 跳过 writeClientFile，新连接没有 banner hostname 数据；
- `go vet -race` 一定能查出。

**Fix:** 改用 `atomic.Int64` 或 `chan int`/`sync.Mutex` 同步。最小改动：

```go
type pTYAttachState struct {
    registryPid atomic.Int64
}
// runClaudePTYWithReconnect 用 atomic.Int64 存 pid，
// goroutine 用 CompareAndSwap(0, pid) 写入；
// 读侧统一 .Load()
```

或者把 writeClientFile 同步执行（相对 PTY 启动它本来就只是 1-2 个远程命令的延迟）。

---

### WR-03: 多次 attach 时 `os.Stdin` 被多个 BufferedStdin.Run goroutine 并发读，输入会丢

**File:** `internal/cloudclaude/session.go:724-730` （`pTYAttachOnce`），`internal/cloudclaude/input_buffer.go:58-88`

`bs.Run` 在 line 729 由 goroutine 启动，监听 `bsCtx`。但 `bs.src.Read(buf)`（line 66）是 **阻塞 syscall**，`bsCtx.Cancel()` 不能让它中途退出 —— 必须等到 stdin 收到下一个字节、`Read` 返回后，goroutine 才会下次 select 到 `<-ctx.Done()` 退出。

reconnect 之后 `pTYAttachOnce` 重新被调，又开了一个新的 `bs.Run` goroutine 在读 `os.Stdin`。此时旧 goroutine 仍未退出（因为没有新输入），两个 goroutine 同时排队 `os.Stdin.Read` —— 任意一字节只会被其中一个收到，另一个 miss。最坏：用户输入的命令一半被「死亡」goroutine 吃掉，丢字符或行错位。

**Fix:** 不要让多个 goroutine 共享 `os.Stdin`。推荐做法：

1. `BufferedStdin` 作为 **进程单例**，跨多次 attach 复用同一个 `bs.Run` goroutine；
2. 通过 `Reconnect.StateAddr()` 共享态（也顺便闭合 carry-over REQ-F3-B）；
3. attach 周期只做 `bs.SwapPipe(newPipeW)`/`bs.AttachSession(session)` 类操作，不再 `NewBufferedStdin`。

短期最小修：在 `pTYAttachOnce` 顶部用 `sync.Once` 保护，仅第一次创建 BufferedStdin 并 cache 在外层闭包；后续 attach 复用同一实例。

---

### WR-04: `BufferedStdin.localEcho` / `grayOpen` 在 Run 与 Flush 跨 goroutine 无锁共享

**File:** `internal/cloudclaude/input_buffer.go:90-141`

`Run` goroutine 调 `closeGrayIfOpen()`（line 71）/ `handleReconnecting()`（line 90-119）会读写 `b.grayOpen` + `b.localEcho`。
`Flush` 由 `Reconnector.onReconnected` 回调调用，运行在 Reconnector 主 goroutine，也调 `closeGrayIfOpen()` + `b.pipeW.Write` + 读 `b.grayOpen`。

两个 goroutine：
- `b.grayOpen` 是普通 bool，无 atomic / mutex → data race；
- `b.localEcho` 同时被 `fmt.Fprint(...)` → 字节流可能交错（虽然每次写都是 atomic write 系统调用，但 `fmt.Fprintf("%c", c)` 可能拆为多 syscall）。

当前 plan 02 没把 BufferedStdin 与 Reconnector 真正接起来（carry-over），所以 Flush 永远不被调，race 暂未爆雷。但代码已经存在，未来 REQ-F3-B 集成后即翻车。

**Fix:** 用一个 mutex（可以复用 `ringMu`）覆盖 `grayOpen` + `localEcho` 的写：

```go
func (b *BufferedStdin) closeGrayIfOpen() {
    b.echoMu.Lock()
    defer b.echoMu.Unlock()
    if b.grayOpen && b.localEcho != nil && !b.noColor {
        fmt.Fprint(b.localEcho, ansiReset)
        b.grayOpen = false
    }
}
// handleReconnecting 内 echo 段也包入 echoMu。
// Flush 内 closeGrayIfOpen 已经间接拿锁。
```

---

### WR-05: `buildTmuxRemoteCmd` 的 fallback 分支用 `exec cd ...`，bash 不允许对 builtin exec

**File:** `internal/cloudclaude/session.go:200-209`

```go
return fmt.Sprintf(
    "cd %s && command -v tmux >/dev/null 2>&1 && exec tmux new-session -A -d -s %s %s \\; attach-session -t %s || exec %s",
    cwdQ, sessionQ, wrapQ, sessionQ, wrapCmd,
)
```

注意最后一段是 `|| exec %s` 中 `%s = wrapCmd` 字面值（**未引号包裹**），等于 `exec cd /path && claude args`。bash 会先解析为 `(exec cd /path) && (claude args)`，而 `exec` builtin 拒绝对 cd 这种 shell builtin 做 exec，结果是 `bash: exec: cd: not found`，整个 fallback 失败。

由于前面 `command -v tmux >/dev/null && exec tmux ...` 在 DetectTmux 已通过的前提下几乎必走，fallback 分支生产环境很难命中。但留着会让未来 tmux 远端突然消失 / 启动失败时 cloud-claude 直接报错而非降级，违背设计意图。

**Fix:** fallback 改用 sh -c 包装 wrapCmd 字面值：

```go
return fmt.Sprintf(
    "cd %s && command -v tmux >/dev/null 2>&1 && exec tmux new-session -A -d -s %s %s \\; attach-session -t %s || exec sh -c %s",
    cwdQ, sessionQ, wrapQ, sessionQ, shellescape.Quote(wrapCmd),
)
```

`exec sh -c '...'` 对 `cd` builtin 来说是 sh 内部解释，OK。

---

## Info

### IN-01: `sync_lock.go::lockPath` 直接拼 accountID，缺路径穿越防御

**File:** `internal/cloudclaude/sync_lock.go:63`

```go
lockPath := fmt.Sprintf("/tmp/cloud-claude/locks/sync-%s.lock", accountID)
```

后续 `shellescape.Quote(lockPath)` 防 shell 注入（OK），但若 `accountID` 含 `/` 或 `..`，flock 仍会去 `/tmp/cloud-claude/locks/sync-../../etc/passwd.lock` 这种位置创锁文件。当前 accountID 来自 gateway AuthResponse（受信），属于深度防御问题。

**Fix:** 在 `AcquireSyncLock` 入口对 accountID 做 `[a-zA-Z0-9_-]+` 校验，或先 `simpleHash8(accountID)` 取 hex 当做 lockPath 后缀：

```go
if !validAccountID.MatchString(accountID) {
    return nil, fmt.Errorf("AcquireSyncLock: 非法 accountID %q", accountID)
}
```

---

### IN-02: `WriteLastSession` 用固定 `path+".tmp"` 后缀，并发写会互相 clobber

**File:** `internal/cloudclaude/last_session.go:67-74`

`WriteLastSession` 路径固定 `path + ".tmp"`，`writeLastSessionTmuxField` / `writeLastSessionReconnectCount` / `mount_strategy.writeLastSessionWarn` 都可能调它。当前所有调用都在主 goroutine 串行，没问题；但未来若把 ReconnectCount 写入挪到 reconnect 成功的回调中并行执行，两个 goroutine 同时 `os.WriteFile(tmp, ...)` → 内容可能被截断 / `os.Rename` 拿到部分写入的 tmp。

**Fix:** tmp 后缀加 pid + 随机数（如 `path + ".tmp." + strconv.Itoa(os.Getpid()) + "." + randStr`），或在包内加一个 `var lastSessionMu sync.Mutex` 串行所有写。

---

### IN-03: `keepalive.go::sendKeepaliveWithTimeout` 用 `time.After`，无法取消

**File:** `internal/cloudclaude/keepalive.go:51-67`

`time.After(timeout)` 在 select 命中其它 case 后仍持有底层 timer 直到 timeout 触发才被 GC。每 15s 调一次问题不大，但模式上更稳的写法是 `timer := time.NewTimer(timeout); defer timer.Stop()`。

另外 `SendRequest` goroutine 在 `conn` 永久阻塞时只会被 `conn.Close()` 唤醒（countMax 失败后），最多泄漏 countMax-1 个 goroutine，可接受。

**Fix:**

```go
timer := time.NewTimer(timeout)
defer timer.Stop()
select {
case <-timer.C:
    return false, errors.New("keepalive timeout")
case r := <-ch:
    return r.ok, r.err
}
```

---

### IN-04: `pTYAttachOnce` 返回 `(int, error, error)` —— 两个 error 返回值容易混淆

**File:** `internal/cloudclaude/session.go:671-673`

```go
func pTYAttachOnce(...) (int, error, error)
```

文档说 `exitErr` 是「session.Wait 原始错误」，`reconnectableErr` 是「触发 Reconnector 的可重连错误」。语义重叠：line 781 `return 0, waitErr, waitErr` —— 两返回值持有同一指针。调用方不得不写 `if exitErr == nil { ... } else if reconnectableErr == nil { ... }`，可读性差且易写反。

**Fix:** 用枚举 + 单 error：

```go
type attachOutcome int
const (
    attachExitedNormally attachOutcome = iota
    attachReconnectable
    attachFatal
)
func pTYAttachOnce(...) (int, attachOutcome, error)
```

---

### IN-05: `RunSessionsAttach` 用 `ExitConfigError`(=4) 表示 session not found

**File:** `internal/cloudclaude/session.go:881-883`

`ExitConfigError` 语义是「配置错误」，session 不存在更接近 `ExitNetworkError` 或一个新的 `ExitSessionNotFound`。当前共用 4 不影响功能但会给 doctor / CI 解读 exit code 带来歧义。

**Fix:** 新增 `ExitSessionNotFound = 8` 之类，或复用 `ExitOAuthNotFound` 模式。如果不动，至少在 long help 里写明 sessions attach 用 4 表示「会话不存在」。

---

### IN-06: `cmd/cloud-claude/sessions.go::runSessionsAttach` 在 os.Exit 前不会跑 deferred conn.Close

**File:** `cmd/cloud-claude/sessions.go:58-75`

```go
defer conn.Close()
code, err := cloudclaude.RunSessionsAttach(...)
if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(code)        // defer 不执行，conn 留在内核 socket 里
}
if code != 0 {
    os.Exit(code)        // 同上
}
```

进程退出系统会回收 socket，影响很小；但语义不一致：成功路径走 defer 关闭、错误路径不走。

**Fix:** 不直接 `os.Exit`，把 exitCode 用 named return / 全局变量带回 `main()`，让 cobra `Execute` 后 `os.Exit(rc)`。或者显式 `conn.Close()` 再 `os.Exit`。

---

### IN-07: `NET_RECONNECT_BACKOFF` 错误码已注册但似乎从未通过 `errcodes.Format` 输出

**File:** `internal/cloudclaude/errcodes/net.go:31-36` + `internal/cloudclaude/reconnect.go`

`renderDisconnectStatus`（reconnect.go:204）渲染三态横幅时直接拼 ANSI 字符串，没用 `errcodes.Format(NET_RECONNECT_BACKOFF, ...)`。如果 v3.0 的 doctor / explain 假设所有用户可见状态都来自 errcodes registry，这条注册了却没在生产路径输出会让 doctor 列表有"幽灵"条目。

**Fix:** 二选一：
1. 在 30s 阈值的 disconnected 文本里调一次 `errcodes.Format(NET_RECONNECT_BACKOFF, elapsed)` 让它被实际渲染；
2. 或者从 net.go 移除 NET_RECONNECT_BACKOFF 注册，避免文档承诺与实现脱节。

---

_Reviewed: 2026-04-20_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
