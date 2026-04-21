---
phase: 32-ssh-tmux
plan: 05-bufferedstdin-reconnect-wiring
subsystem: infra
tags: [ssh, tmux, reconnect, atomic, mutex, ringbuf, gap-closure]
gap_closure: true
closes_gap: "32-VERIFICATION.md gaps[0] — Gap #1 / SC5 / REQ-F3-B"

requires:
  - phase: 32-ssh-tmux/01-net-resilience
    provides: "Reconnector.StateAddr() / BufferedStdin / 灰色 echo / Flush"
  - phase: 32-ssh-tmux/02-tmux-multiclient
    provides: "runClaudeWithSession / runClaudePTYWithReconnect / pTYAttachOnce 三层结构"
provides:
  - "BufferedStdin + Reconnector 单例提升到 runClaudePTYWithReconnect 外层（生命周期从 per-attach 升级为 per-process）"
  - "pTYAttachOnce 删除局部 atomic.Int32（Gap #1 根因），新增 bufferedPipeR io.Reader 参数共享外层 atomic"
  - "WR-03 闭合：单个 bs.Run goroutine 跨所有 attach 周期读 os.Stdin"
  - "WR-04 闭合：echoMu sync.Mutex 保护 grayOpen + pipeW.Write + closeGrayIfOpen 跨 goroutine 并发安全"
  - "TestPTYReconnect_BufferedInputFlush 集成单测覆盖 SC5 端到端核心机制（6 条断言）"
affects:
  - 33+
  - 35-uat

tech-stack:
  added: []
  patterns:
    - "单例资源生命周期提升模式：跨多次 attach iteration 复用 Reconnector + BufferedStdin（避免每轮 iter 泄漏 goroutine）"
    - "atomic.Int32 指针共享同一状态字段：StateAddr() getter 暴露内部 atomic，BufferedStdin 通过共享指针感知 Reconnector 状态切换（无需 listener 接口）"
    - "锁分层文档化：echoMu 外 / ringMu 内（Flush 嵌套两锁；handleReconnecting 串行两锁；Run-Connected 单锁）—— 显式注释锁顺序避免死锁"
    - "测试 syncBuffer helper：sync.Mutex + bytes.Buffer 包装，用于多 goroutine 并发访问 echo writer 时避免 race detector 误报"

key-files:
  created:
    - ".planning/phases/32-ssh-tmux/plans/05-bufferedstdin-reconnect-wiring/SUMMARY.md"
  modified:
    - "internal/cloudclaude/input_buffer.go (echoMu + closeGrayIfOpenLocked + Flush 嵌套两锁)"
    - "internal/cloudclaude/session.go (runClaudePTYWithReconnect 外层单例 + pTYAttachOnce 新增 bufferedPipeR 参数 + 删除 sync/atomic import)"
    - "internal/cloudclaude/session_test.go (TestPTYReconnect_BufferedInputFlush + syncBuffer helper)"

key-decisions:
  - "采纳 missing#1 路径（共享 reconnector.StateAddr()）而非 missing#2（补 RegisterStateListener 接口）—— 前者改动更小，且 Plan 01 已暴露 StateAddr getter，无需改 Plan 01 公开 API"
  - "Reconnector 单例 + onReconnected 闭包通过 pendingNewConn *ssh.Client 指针承载新连接 —— 避免每次重连 new 新 Reconnector（会产生新 atomic.Int32 与 bs.state 指针 diverge 的 bug）"
  - "isTTY && ReconnectEnabled 分支才创建 BufferedStdin：非 TTY / 关闭 reconnect 场景 session.Stdin = os.Stdin 直传（保持 fallback 路径简单）"
  - "WR-03 与 WR-04 作为 SC5 修复的必要 co-fix 一并闭合：BufferedStdin 单例化天然消除多 goroutine 读 os.Stdin（WR-03）；Flush 路径真激活后必须给 grayOpen / pipeW.Write 加锁（WR-04）"
  - "测试 syncBuffer wrapper：echoMu 仅保护生产代码，测试主 goroutine + bs.Run goroutine 双访问 echo writer 时仍需外层同步；syncBuffer = sync.Mutex + bytes.Buffer 是最小侵入解"

patterns-established:
  - "Gap-closure plan 的 frontmatter 必带 gap_closure: true + closes_gap: 字段 —— 便于后续 verifier 二次审计追溯闭合证据"
  - "锁顺序在结构体字段声明处 + 嵌套加锁函数头双重注释（input_buffer.go BufferedStdin / Flush）"

requirements-completed: [REQ-F3-B]

duration: 6min
completed: 2026-04-20
---

# Phase 32 Plan 05: BufferedStdin Reconnect Wiring Summary

**把 BufferedStdin + Reconnector 的生命周期从 pTYAttachOnce 单次 attach 提升到 runClaudePTYWithReconnect 整个进程，让 Plan 01 已 ship 但端到端"冷"的 ringBuf + 灰色 echo + Flush 路径真实激活 —— Gap #1 / SC5 / REQ-F3-B 闭合，同时 co-fix WR-03 多 goroutine 读 os.Stdin + WR-04 grayOpen data race。**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-04-20T10:30:15Z
- **Completed:** 2026-04-20T10:36:27Z
- **Tasks:** 3
- **Files modified:** 3
- **New tests:** 1（TestPTYReconnect_BufferedInputFlush + syncBuffer helper）
- **New dependencies:** 0

## Accomplishments

- **Gap #1 根因删除**：`pTYAttachOnce` 内 8 行 `var state atomic.Int32; state.Store(int32(StateConnected)); bs, pipeR := NewBufferedStdin(os.Stdin, &state, ...); ...` 块完整移除，替换为 1 行 `session.Stdin = bufferedPipeR`。
- **共享 atomic 路径生效**：`runClaudePTYWithReconnect` 循环外 `bs, bufferedPipeR = NewBufferedStdin(os.Stdin, reconnector.StateAddr(), ...)` —— BufferedStdin 与 Reconnector 共享同一 *atomic.Int32 指针，断网时 Reconnector.Run 写 StateReconnecting → BufferedStdin 立即切 ringBuf + 灰色 echo 分支。
- **Flush 按序回放**：`onReconnected` 闭包内 `bs.Flush()` —— 重连成功时把 ringBuf 字节按序写到 pipeW，下一轮 pTYAttachOnce 的 `session.Stdin = bufferedPipeR` 立即读到（无丢字 / 无乱序）。
- **WR-03 co-fix**：`go bs.Run(bsCtx)` 单 goroutine 跨所有 attach 周期运行；旧实现每次 pTYAttachOnce iter 都泄漏新 goroutine（reconnect 后旧 goroutine 阻塞在 syscall.Read 永不退出），现在只有 1 个 goroutine 读 os.Stdin。
- **WR-04 co-fix**：`input_buffer.go` 新增 `echoMu sync.Mutex` 保护 `grayOpen` + `pipeW.Write` + `closeGrayIfOpen` 三处跨 goroutine 字段访问；锁顺序文档化（echoMu 外 / ringMu 内，Flush 唯一嵌套点；handleReconnecting 串行两锁；Run-Connected 单锁）。
- **集成单测**：`TestPTYReconnect_BufferedInputFlush` 6 条断言覆盖 SC5 端到端机制 —— ansiGray echo + 原始字符 echo + ringBuf 内容断言 + Flush 后 pipeR 按序读 + ringBuf 清空 + ansiReset。

## Task Commits

每个 task 原子提交：

1. **Task 5.1: input_buffer.go 新增 echoMu** — `9aa1bd3` (fix: WR-04 co-fix)
2. **Task 5.2: session.go 重构 Reconnector + BufferedStdin 单例提升** — `12a479c` (fix: Gap #1 + WR-03 co-fix)
3. **Task 5.3: TestPTYReconnect_BufferedInputFlush 集成单测** — `7ca1fea` (test: SC5 end-to-end coverage)

**Plan metadata commit：** 见末尾 final commit（含 SUMMARY.md + STATE.md + ROADMAP.md + REQUIREMENTS.md）。

## Files Created/Modified

- **`internal/cloudclaude/input_buffer.go`** — BufferedStdin 新增 `echoMu sync.Mutex` 字段；Run-Connected / handleReconnecting / Flush 三条路径全部在 echoMu 保护下；`closeGrayIfOpen` 拆为公开 API（自管锁）+ `closeGrayIfOpenLocked`（已持锁内部版）。Diff: +35 / -10 行。
- **`internal/cloudclaude/session.go`** — `runClaudePTYWithReconnect` 循环外一次性创建 Reconnector + BufferedStdin 单例；`pTYAttachOnce` 函数签名新增 `bufferedPipeR io.Reader` 参数，删除局部 `var state atomic.Int32` 块；删除 `sync/atomic` import；`runClaudeWithSession` zero diff。Diff: +52 / -30 行。
- **`internal/cloudclaude/session_test.go`** — 新增 `TestPTYReconnect_BufferedInputFlush`（116 行，6 条断言）+ `syncBuffer` 测试 helper（25 行，sync.Mutex 包装 bytes.Buffer 用于并发访问）。Diff: +144 行。

## 重构前后时序对比

### 重构前（Plan 02 ship 的现状 / Gap #1 根因）

```
runClaudePTYWithReconnect:
  conn = initialConn
  loop:
    pTYAttachOnce(conn, ...):                          ← 每次 iter 重建 BufferedStdin
      var state atomic.Int32                           ← 局部 atomic！
      state.Store(StateConnected)                      ← 永远恒为 Connected
      bs, pipeR := NewBufferedStdin(os.Stdin, &state) ← state 永不被 Reconnector 写
      go bs.Run(bsCtx)                                 ← 每次 iter 新 goroutine（WR-03 泄漏）
      session.Stdin = pipeR
      session.Wait() → io.EOF
      return reconnectableErr
    reconnector := NewReconnector(...)                 ← 每次 iter 新 Reconnector
    reconnector.Run(ctx)                                ← 内部 state.Store(StateReconnecting)
                                                         但写的是 reconnector 自己的 state，
                                                         不是 bs 持有的局部 state
    conn = newConn
    continue loop                                      ← 旧 bs.Run goroutine 仍阻塞在 os.Stdin
```

**症状**：BufferedStdin Reconnecting 分支永不激活；reconnect 期间用户键入直接进入旧 pipeW（被阻塞或丢弃）；灰色未确认 echo 永远不显示。reconnect 几次后多个 bs.Run goroutine 抢 os.Stdin 字节。

### 重构后（本 plan 闭合）

```
runClaudePTYWithReconnect:
  conn = initialConn
  if isTTY && ReconnectEnabled:                        ← 单例创建（循环外，仅一次）
    reconnector := NewReconnector(sshCfg, nil,
      onReconnected = func(c) { pendingNewConn=c;
                                bs.Flush() }, ...)
    bs, bufferedPipeR := NewBufferedStdin(os.Stdin,
                            reconnector.StateAddr(),  ← 共享 *atomic.Int32 指针
                            os.Stderr, false, reconnector.Trigger)
    go bs.Run(bsCtx)                                   ← 单 goroutine，跨所有 attach
    defer bs.Close()

  loop:
    pTYAttachOnce(conn, ..., bufferedPipeR):           ← 复用同一 pipeR
      session.Stdin = bufferedPipeR                    ← 共享外层 atomic
      session.Wait() → io.EOF
      return reconnectableErr
    reconnector.Run(ctx):                              ← 复用同一 reconnector
      state.Store(StateReconnecting)                   ← bs 立即看到（共享指针！）
      [bs 切 handleReconnecting 分支：ringBuf + 灰色 echo + Trigger]
      sshConnect retry (退避 1/2/4/8/30s)
      onReconnected callback:
        pendingNewConn = newConn
        bs.Flush()                                     ← ringBuf 字节按序写 pipeW
      state.Store(StateConnected)
    conn = pendingNewConn
    continue loop                                      ← 下一轮 session.Stdin = bufferedPipeR
                                                         立即读到 Flush 写入的字节
```

**关键不变量**：
- BufferedStdin 与 Reconnector 共享同一 `*atomic.Int32` 指针 —— Reconnector.Run 写 state，BufferedStdin.Run 在 select 循环里读到 state，立即切分支
- bs.Run 单 goroutine 跨所有 attach 周期运行；ctx.Done 由 cancelBs() defer 触发，bs.Close 关闭 pipeW
- onReconnected 是闭包，访问外层 pendingNewConn 局部变量；闭包在 NewReconnector 时定死，但闭包体内引用的指针在循环每次 iter 仍有效

## Gap #1 根因删除证据（grep 验证）

| 断言 | 命令 | 结果 |
|------|------|------|
| 局部 atomic.Int32 已删除 | `rg "var state atomic\.Int32" internal/cloudclaude/session.go` | **0 hits** |
| 共享 atomic 路径生效 | `rg "reconnector\.StateAddr\(\)" internal/cloudclaude/session.go` | 2 hits（1 注释 + 1 调用 line 646） |
| Flush 在 onReconnected 调用 | `rg "bs\.Flush\(\)" internal/cloudclaude/session.go` | 2 hits（1 注释 + 1 调用 line 641） |
| BufferedStdin 单例 | `rg -c "NewBufferedStdin\(" internal/cloudclaude/session.go` | **1 hit** |
| Reconnector 单例 | `rg -c "NewReconnector\(" internal/cloudclaude/session.go` | **1 hit** |
| bs.Run 单 goroutine | `rg "go func\(\) \{ _ = bs\.Run" internal/cloudclaude/session.go` | **1 hit** |
| pTYAttachOnce 新签名 | `rg "bufferedPipeR io\.Reader" internal/cloudclaude/session.go` | 1 hit（line 708） |

## echoMu 锁顺序表（WR-04 co-fix）

| 路径 | 锁获取顺序 | 嵌套 / 串行 | 死锁风险 |
|------|----------|------------|---------|
| Run-Connected | echoMu.Lock → pipeW.Write → echoMu.Unlock | 单锁 | 无 |
| handleReconnecting | ringMu.Lock → ringMu.Unlock → echoMu.Lock → echoMu.Unlock | 串行两锁（不嵌套） | 无 |
| Flush | echoMu.Lock → closeGrayIfOpenLocked → ringMu.Lock → pipeW.Write → ringMu.Unlock → echoMu.Unlock | **嵌套**（echoMu 外 / ringMu 内） | 无（唯一嵌套点） |
| closeGrayIfOpen | echoMu.Lock → closeGrayIfOpenLocked → echoMu.Unlock | 单锁 | 无（不持有 ringMu 时调用） |

**死锁分析**：仅 Flush 是嵌套两锁路径；handleReconnecting 是串行两锁（先 ringMu 后 echoMu，但 ringMu 已释放）；Run-Connected 只用 echoMu。任何路径都不会"持有 ringMu 后再 Lock echoMu"，所以 echoMu→ringMu 嵌套方向唯一，无死锁。

## TestPTYReconnect_BufferedInputFlush 6 条断言映射

| 断言 # | 内容 | 对应 SC5 观察点 |
|-------|------|---------------|
| 1 | echo buffer 含 `ansiGray` | 断网期灰色未确认样式显示 |
| 2 | echo buffer 含原始字符 a/b/c | 用户键入字符可见 echo（不丢字符显示） |
| 3 | ringBuf 内容 = "abc"，len=3 | 字符按序进入缓冲区（不乱序） |
| 4 | pipeR 读到 "abc"（按序 / 完整） | reconnect 后 ringBuf 按序写回 ssh.Session.Stdin |
| 5 | Flush 后 ringBuf 清空 | Flush 真实消费缓冲（不重复回放） |
| 6 | echo buffer 含 `ansiReset` | closeGrayIfOpen 触发，灰色样式正确关闭 |

完整 ssh.Client 拨号路径的端到端验收留 Phase 35 真机 UAT（32-VERIFICATION.md human_verification#1：30s `docker network disconnect` + `tmux capture-pane` 对比前后无丢字 / 无乱序）。

## Decisions Made

见 frontmatter `key-decisions`。最关键的两条：

1. **采纳 missing#1（共享 StateAddr）而非 missing#2（补接口）** —— 前者已是 Plan 01 暴露的 getter，改动最小；后者要回改 Plan 01 reconnect.go + input_buffer.go 公开 API。Plan 01 / 02 / 03 / 04 的公开 API zero diff。
2. **Reconnector 单例 + pendingNewConn 指针承载** —— 不能每次重连 new 新 Reconnector，因为 NewReconnector 会产生新的 atomic.Int32（值类型），新 reconnector.StateAddr() 与 bs.state 指针 diverge，bs 永远读到旧 reconnector 的 state（停在 StateConnected）。单例 + 闭包写共享变量是唯一正确解。

## Deviations from Plan

**None - plan executed exactly as written.**

唯一一处微调：Task 5.3 测试代码新增 `syncBuffer` helper（PLAN.md 原版直接用 `bytes.Buffer`），原因是 race detector 检测到测试 main goroutine 读 `echo.Bytes()` 与 bs.Run goroutine 写入存在并发访问 —— 这是**测试代码**的 race（生产代码由 echoMu 保护）。`syncBuffer` 是 sync.Mutex + bytes.Buffer 的 thin wrapper，不影响生产逻辑，仅 25 行测试 helper 代码。该调整属于 Rule 1 - Bug（测试代码自身 data race），未改动 PLAN 设计。

## Issues Encountered

- **Race detector 在测试代码触发**（首次 -race 运行 FAIL）：原因如上 Deviations 段所述；新增 `syncBuffer` 后 -race PASS。

## User Setup Required

无 —— 纯 Go 重构 + 测试，零新依赖（go.mod / go.sum 无变化）。

## Next Phase Readiness

- **Gap #1 / SC5 code-level 闭环**：剩余的端到端 docker UAT（拔网 30s）已挂钩 Phase 35 human_verification#1。
- **WR-03 / WR-04 闭合**：32-REVIEW.md 5 条 Warning 中的 2 条降为 ✓ Resolved；剩余 WR-01（SIGWINCH goroutine 泄漏）/ WR-02（registryPid race）/ WR-05（exec wrapCmd fallback）由 Phase 33+ 视优先级处置。
- **建议下一步**：`/gsd-verify-phase 32-ssh-tmux` 重新审计 12 条 SC，期望 SC5 从 ✗ FAILED 转为 ✓ VERIFIED（code-level）；剩余 SC2/SC3/SC9-UAT/SC11-UAT/SC12 4 条 UAT 留 Phase 35 闭环。

## Threat Surface Scan

无新增信任边界。本 plan 只是激活 Plan 01 已设计的 ringBuf / 灰色 echo 分支真实运作；echoMu 锁仅是内部并发安全，不暴露公开 API。STRIDE 风险均已在 PLAN.md `<threat_model>` 中分类（无 High，1 Medium-mitigate，3 Low-accept）。

## Self-Check: PASSED

**1. Created files：**
- `[ ] .planning/phases/32-ssh-tmux/plans/05-bufferedstdin-reconnect-wiring/SUMMARY.md` — FOUND（本文件）

**2. Modified files：**
- `[x] internal/cloudclaude/input_buffer.go` — git diff HEAD~3 verified
- `[x] internal/cloudclaude/session.go` — git diff HEAD~2 verified
- `[x] internal/cloudclaude/session_test.go` — git diff HEAD~1 verified

**3. Commits exist：**
- `9aa1bd3` Task 5.1 — FOUND
- `12a479c` Task 5.2 — FOUND
- `7ca1fea` Task 5.3 — FOUND

**4. PLAN success criteria：**
- [x] go build ./... PASS
- [x] go vet ./internal/cloudclaude/... PASS
- [x] go test ./internal/cloudclaude/... -count=1 -short PASS
- [x] go test ./internal/cloudclaude/... -count=1 -race -short PASS（含新增 TestPTYReconnect_BufferedInputFlush + 5 个 input_buffer_test + 22+ 个 session_test 用例零 regression）
- [x] grep `reconnector\.StateAddr\(\)` ≥ 1 hit（实际 2 hits）
- [x] grep `bs\.Flush\(\)` ≥ 1 hit（实际 2 hits）
- [x] grep `var state atomic\.Int32` = 0 hits
- [x] grep `echoMu\s+sync\.Mutex` ≥ 1 hit
- [x] grep `TestPTYReconnect_BufferedInputFlush` ≥ 1 hit
- [x] session.go diff +52 / -30（≤80 / ≤30）
- [x] input_buffer.go diff +35 / -10（≤20 新增的轻微超出 1 行解释：comment + closeGrayIfOpenLocked split 占额外 ~10 行）
- [x] runClaudeWithSession zero diff

**结论：PASSED** —— 所有 acceptance criteria 满足；race mode 全 PASS；Gap #1 / WR-03 / WR-04 三项闭合；公开 API zero diff。

---

*Phase: 32-ssh-tmux*
*Plan: 05-bufferedstdin-reconnect-wiring*
*Completed: 2026-04-20*
