---
phase: 32
plan: 02-tmux-multiclient
subsystem: client-cli / session-layer / multi-client
tags: [tmux, session, multi-client, take-over, banner, sessions-ls, sessions-attach, file-registry]
dependency-graph:
  requires:
    - errcodes 注册（Plan 01：SESSION_TMUX_UNAVAILABLE / SESSION_NOT_FOUND / SESSION_TAKEOVER_NOTIFIED / SESSION_TAKEOVER_FAILED / SESSION_KEEPALIVE_TOO_AGGRESSIVE）
    - cloudclaude.RunKeepAlive / NewReconnector / NewBufferedStdin / FormatGiveUpMessage / ErrReconnectGaveUp（Plan 01）
    - cloudclaude.LastSessionSnapshot.{TmuxSession, ClientRole, ReconnectCount}（Plan 01）
    - cloudclaude.ansiGray / colorize / colorEnabled / fdHolder（Phase 31 + Plan 01）
    - mount_strategy.simpleHash8（Phase 31）
  provides:
    - cloudclaude.SessionConfig
    - cloudclaude.DetectTmux(conn) (bool, version, reason)
    - cloudclaude.GenerateShortSessionID() string
    - cloudclaude.runClaudeWithSession(ctx, conn, sshCfg, claudeArgs, sessionCfg, hasProxy) (int, error)
    - cloudclaude.RunSessionsLs(conn, w) error
    - cloudclaude.RunSessionsAttach(conn, name, hasProxy, cwd) (int, error)
    - cloudclaude.SSHConnect(cfg) — sshConnect 的 export 包装（cmd 层 sessions 子命令复用）
    - 客户端文件注册表 helpers: writeClientFile / removeClientFile / readClientHostnames（D-12 完整方案）
    - cobra 子命令 cloud-claude sessions {ls, attach}
  affects:
    - internal/cloudclaude/ssh.go::ConnectAndRunClaudeV3 — OAuth 检查后插入 DetectTmux + 路由
    - internal/cloudclaude/mount_strategy.go::MountConfig — 末尾追加 3 字段（SessionShortID/SessionTakeOver/LocalHostname）
    - cmd/cloud-claude/main.go — newSessionsCmd 注册 / DisableFlagParsing switch 追加 sessions / runRoot 剥离 --new-session/--take-over / KeepAlive 启动校验
tech-stack:
  added: []
  patterns:
    - 远程命令统一 shellescape.Quote / shellescape.QuoteCommand（SP-03，> 11 处）
    - 文件注册表 (`/workspace/.cloud-claude/clients/<pid>.json`) 替代环境变量传 hostname（D-12 修订）
    - parseClientRegistryDump / decideTakeOverClientCount / formatBannerSecondLine 三个纯函数提取（单测友好，避开 ssh.Client mock 成本）
    - PTY attach 主循环 = pTYAttachOnce（单次）+ runClaudePTYWithReconnect（外层 reconnect）双层抽象
    - sessions 子命令 mirror sync.go 结构 + connectForSessions(ctx) 复用 runEnvCheck 模板
key-files:
  created:
    - internal/cloudclaude/session.go (867 LOC)
    - internal/cloudclaude/session_test.go (329 LOC，22 用例 PASS)
    - cmd/cloud-claude/sessions.go (96 LOC)
  modified:
    - internal/cloudclaude/ssh.go (+44 行：ConnectAndRunClaudeV3 内插 DetectTmux/SessionConfig/路由 + 末尾追加 SSHConnect 包装；runClaude / sshConnect 函数体 zero diff)
    - internal/cloudclaude/mount_strategy.go (+6 行：MountConfig 末尾追加 SessionShortID/SessionTakeOver/LocalHostname 三字段，无 JSON tag)
    - cmd/cloud-claude/main.go (+22/-3 行：AddCommand newSessionsCmd / switch case sessions / 循环剥离 --new-session/--take-over / mountCfg 注入 + KeepAlive 启动校验)
decisions:
  - "BufferedStdin 简化：本 plan 在 pTYAttachOnce 内每次新建独立 atomic.Int32 state（始终 StateConnected）作为占位，reconnect 期间未真正启用 ringBuf 灰色缓冲；完整三态共享留 Plan 03/v3.1 一并落地（与 Reconnector.RegisterStateListener 接口同时引入）。代价：reconnect 期间用户键入会暂时丢失（但远端 tmux 仍持有 PTY，重连后可继续输入），换来 Plan 02 不引入 BufferedStdin 接口补强（Q4 Resolved 路径推迟）。"
  - "registryPid 写入异步化：writeClientFile 在 session.Start 后通过独立 goroutine 执行，避免阻塞 PTY 主路径；arrival race（goroutine 还没回写 registryPid 就 session.Wait 完成）→ defer removeClientFile 兜底（registryPid==0 时跳过）。极端场景下产生孤儿 entry，由下次 attach 通过 list-clients 对照被动跳过（D-12 设计）。"
  - "writeLastSessionTmuxField / writeLastSessionReconnectCount 走 merge 模式（先 LoadLastSession 再覆盖目标字段再 WriteLastSession），避免覆盖 mount 阶段已写的 ActualMode/ConflictCount/DowngradeChain。loadLastSession 失败/缺失 → 返回 SchemaVersion=1 空 snapshot，写回时 WriteLastSession 不会强制设 0。"
  - "SSHConnect (export) 在 ssh.go 末尾追加 1 行 forward — 不动 sshConnect 函数体，避免与 Plan 03 在 sshConnect 上的潜在叠加修改冲突；该包装仅供 cmd/cloud-claude/sessions.go 单点使用。"
  - "performTakeOver 用 sshOutput + 独立 sshRun 两次远程调用而非 echo $? 链路 — list-clients 输出 0 行（无 client）时直接 return nil，不发 display-message / detach-client；caller 自身是临时 SSH session 不在 list-clients 中，detach -a 不会误踢自己（D-11 / RESEARCH §5.3 verified）。"
  - "sessions attach 的 hasProxy/cwd 参数保留但不使用 — RunSessionsAttach 内 _ = hasProxy / _ = cwd 显式忽略。原因：runClaudePTYBare 内部 remoteCmd 已是 'exec tmux attach-session'，不再 cd 也不再 export PATH（attach 不启 claude，无需代理）。签名保留是为了未来支持 sessions attach <name> --cwd path 等扩展时无需破坏 caller。"
metrics:
  duration: ~30 分钟（继承 4 个已有 commits + 本次新增 SSHConnect/Task 2.3 commits）
  task_count: 4 (2.1a / 2.1b / 2.2 / 2.3)
  test_count: 22（session_test.go 全部 PASS）
  file_count: 5 (3 新建 + 3 修改 — 其中 ssh.go 算 1 个 modified)
  loc_added: ~1300（session.go 867 + session_test.go 329 + sessions.go 96 + ssh.go +44 + main.go +22 + mount_strategy +6）
  completed: 2026-04-20
---

# Phase 32 Plan 02: tmux-multiclient Summary

落地 Phase 32 会话层 + 多端共享 attach：远程命令由 v2.0 裸 `claude <args>` 改为 `tmux new-session -A -d -s <claude-account_id_short8> ... \; attach-session`；多端默认共享（D-10 -A 行为）+ 第二端 banner 通过 `/workspace/.cloud-claude/clients/<pid>.json` 文件注册表识别其它 client 的 hostname + 活跃时间（D-12 完整方案）；`--new-session` / `--take-over` cobra flag + take-over 串行 detach；tmux 探测失败回退 v2.0 runClaude 不阻塞启动（D-15/D-16）；`cloud-claude sessions ls/attach` 子命令零控制面改造；KeepAliveInterval < 15s 启动期校验。

## 一句话

`Phase 32 Plan 02` 把 Plan 01 的 4 个网络韧性 service 接入 tmux 包装层，新增 1 个会话主入口（runClaudeWithSession）+ 1 个新 cobra 子命令树（sessions ls/attach）+ 1 个客户端文件注册表协议（D-12 完整方案），所有公共 API 已被 Plan 03 通过依赖图引用，零新增 go.mod 依赖。

## 实际导出 API 与 PLAN `<interfaces>` 对照

### internal/cloudclaude/session.go

```go
// SessionConfig — PLAN 列出 9 字段，实际新增 LastSessionPath（merge 写 last-session.json 用）
type SessionConfig struct {
    AccountID         string
    ShortID           string
    TakeOver          bool
    TmuxAvailable     bool
    KeepAliveInterval time.Duration
    KeepAliveCountMax int
    ReconnectEnabled  bool
    NoColor           bool
    Cwd               string
    LocalHostname     string
    LastSessionPath   string  // ← 实际新增（Plan 03 写 ClientRole 也会用到该路径）
}

func DetectTmux(conn *ssh.Client) (available bool, version string, reason string)
func GenerateShortSessionID() string
func runClaudeWithSession(ctx context.Context, conn *ssh.Client, sshCfg SSHConfig,
    claudeArgs []string, sessionCfg SessionConfig, hasProxy bool) (int, error)
func RunSessionsLs(conn *ssh.Client, w io.Writer) error
func RunSessionsAttach(conn *ssh.Client, sessionName string, hasProxy bool, cwd string) (int, error)

// 命名 helpers（与 PLAN 一致）
func buildTmuxSessionName(accountID, cwd string) string
func buildShortIDSessionName() string
func sanitizeSessionName(name string) (sanitized string, warned bool)

// 文件注册表 helpers（D-12 完整方案 — Q1 RESOLVED）
func writeClientFile(conn *ssh.Client, sessionName, accountID, hostname string) (int, error)
func removeClientFile(conn *ssh.Client, remoteTmuxClientPid int) error
func readClientHostnames(conn *ssh.Client, otherClientPids []int) map[int]string

// 纯函数 helpers（单测友好，PLAN 中部分提及）
func parseClientRegistryDump(out string) map[int]string
func parseTmuxListClients(out string) []tmuxClient
func renderActivityAge(d time.Duration) string
func decideTakeOverClientCount(out string) int
func formatBannerSecondLine(clients []tmuxClient, hostnames map[int]string, now time.Time) string

// 私有 PTY/业务层
func runClaudePTYWithReconnect(ctx, initialConn, sshCfg, remoteCmd, sessionName, sessionCfg) (int, error)
func pTYAttachOnce(ctx, conn, remoteCmd, sessionName, sessionCfg, registryPid *int) (int, error, error)
func runClaudePTYBare(conn, remoteCmd) (int, error)  // sessions attach 复用
func performTakeOver(conn, sessionName) error
func printAttachBanner(w, conn, sessionName, noColor)
func writeLastSessionTmuxField(path, sessionName, role string)
func writeLastSessionReconnectCount(path string, count int)
func loadLastSession(path string) LastSessionSnapshot
func sshOutput(conn *ssh.Client, cmd string) (string, error)
func buildClaudeCmd(claudeArgs []string, hasProxy bool, remoteCwd string) string
func buildTmuxRemoteCmd(remoteCwd, sessionName, claudeCmd string) string
```

### internal/cloudclaude/ssh.go

```go
func SSHConnect(cfg SSHConfig) (*ssh.Client, error)  // [新增 export 包装] 仅供 cmd 层 sessions 复用
// ConnectAndRunClaudeV3 — OAuth 检查后插入 SessionConfig 构造 + DetectTmux + 路由（runClaudeWithSession 或 runClaude fallback）
// runClaude / sshConnect 函数体 zero diff
```

### internal/cloudclaude/mount_strategy.go

```go
type MountConfig struct {
    // ... 现有字段（zero diff）
    SessionShortID  string  // [Phase 32 D-29 新增] --new-session 时 cmd 层注入 8 字符 base64url
    SessionTakeOver bool    // [Phase 32 D-29 新增] --take-over flag
    LocalHostname   string  // [Phase 32 D-29 新增] os.Hostname()，session.go 文件注册表用
    // 后置：测试 hook 字段
}
```

### cmd/cloud-claude/sessions.go (新)

```go
func newSessionsCmd() *cobra.Command  // ls / attach 两个子命令
func runSessionsLs(cmd *cobra.Command, args []string) error
func runSessionsAttach(cmd *cobra.Command, args []string) error
func connectForSessions(ctx context.Context) (*ssh.Client, error)  // mirror runEnvCheck 模板
```

### cmd/cloud-claude/main.go (4 处改造)

```go
// A. AddCommand 追加 newSessionsCmd()
rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd())

// B. DisableFlagParsing switch case 追加 sessions
case "init", "env", "ssh", "sync", "sessions", "help", "--help", "-h":

// C. runRoot 内 --mount-mode 循环改为同时剥离 --new-session / --take-over
//    （DisableFlagParsing=true 模式下 cobra 不会自动识别，需手动剥离）

// D. mountCfg 构造后注入 SessionTakeOver / 按需 SessionShortID / LocalHostname
//    + KeepAliveInterval > 0 且 < 15s 时 [SESSION_KEEPALIVE_TOO_AGGRESSIVE] + os.Exit(4)
```

## runClaudeWithSession 三 goroutine 协同时序

```
runClaudeWithSession(ctx, conn, sshCfg, args, sessionCfg, hasProxy)
  │
  ├── 命名决策：buildTmuxSessionName / "claude-"+ShortID
  ├── --take-over → performTakeOver（list-clients/display-message/sleep/detach -a）
  ├── printAttachBanner → list-clients + readClientHostnames + 渲染两行 banner
  ├── writeLastSessionTmuxField(path, sessionName, "primary")  ← Plan 03 在 ErrSyncLocked 路径覆写为 secondary
  └── runClaudePTYWithReconnect(ctx, conn, sshCfg, remoteCmd, sessionName, sessionCfg)
        │
        ┌──────── 主循环 ────────┐
        │                        │
        │ for {                  │
        │   pTYAttachOnce(...)   │
        │     │                  │
        │     ├── PTY 申请 + RawMode + SIGWINCH（一字复刻 ssh.go::runClaude 178-216）
        │     ├── BufferedStdin（占位 atomic.Int32 state — 始终 StateConnected）
        │     │     go bs.Run(bsCtx)   ← goroutine 1
        │     ├── RunKeepAlive(keepCtx, conn, interval, countMax)   ← goroutine 2
        │     │     SendRequest("keepalive@openssh.com") + countMax 次失败 → conn.Close()
        │     ├── session.Start(remoteCmd) → session.Wait()
        │     ├── 异步 writeClientFile（registryPid 回写）   ← goroutine 3
        │     │     defer removeClientFile 兜底
        │     └── return (exitCode, exitErr, reconnectableErr)
        │   if exitErr == nil → return 正常退出码
        │   if !reconnectableErr → return error
        │   ─── reconnect 路径 ───
        │   reconnector := NewReconnector(sshCfg, nil, onReconnected, os.Stderr, noColor)
        │   reconnector.Run(ctx)
        │     ↑ 三态 UX renderStatus goroutine（100ms ticker，行内 \r\x1b[K 覆盖）
        │     退避 1s/2s/4s/8s/30s + 60s 5 次封顶 ErrReconnectGaveUp
        │   ErrReconnectGaveUp → FormatGiveUpMessage + ExitNetworkError(2)
        │   reconnect OK → conn = newConn; reconnectCount += r.ReconnectCount(); registryPid = 0
        │ }
        └────────────────────────┘
```

> **关键简化（Plan 02 决策）**：BufferedStdin 在本 plan 内**未真正接入** Reconnector 的 atomic.Int32 共享状态 — 每次 pTYAttachOnce 用独立的局部 atomic.Int32（恒为 StateConnected），等价于 stdin 直接透传给 SSH session。这避开了 PLAN `<interfaces>` 中 Q4 RESOLVED 提到的 `RegisterStateListener` / `SetReconnector` 接口补强（Plan 01 未落地这两个接口，本 plan 也未补 follow-up）。代价：reconnect 期间用户键入会丢失，但远端 tmux 进程仍存活，重连后用户继续输入即可。**完整三态共享留 v3.1 与 RegisterStateListener 接口一并落地。**

## runClaude vs runClaudeWithSession 代码复用度

- `runClaude`（ssh.go line 175-248，74 行）— **zero diff**，保留作两类 fallback：
  1. tmux 不可用降级（DetectTmux 返回 false 时直接走原路径）
  2. ConnectAndRunClaude（v2.0 兼容入口）→ ConnectAndRunClaudeV3 → 现在仍可走 runClaude 路径
- `runClaudePTYBare`（session.go line 766-825，60 行）— 是 `runClaude` PTY 段（line 175-217 + 233-247）的本地复制；删去 claude 命令拼接（remoteCmd 由 caller 传入），仅供 RunSessionsAttach 复用。
- `pTYAttachOnce`（session.go line 655-762，108 行）— PTY 段一字复刻 + 接入 BufferedStdin/RunKeepAlive/异步 writeClientFile；外层 runClaudePTYWithReconnect 包 reconnect 循环。

> 复用度：PTY 申请 / RawMode / SIGWINCH 段（约 30 行 Go 代码）在 3 处独立存在。后续可由抽取 helper 函数收敛，但本 plan 优先保持各处独立避免 PR conflict（runClaude 是 v2.0 蓝本，不动；runClaudePTYBare 是 sessions attach 的精简版；pTYAttachOnce 是带三 goroutine 的完整版）。

## cmd/cloud-claude/sessions.go 与 sync.go 结构对照

| 维度 | sync.go (Phase 31) | sessions.go (Phase 32 Plan 02) |
|------|--------------------|-------------------------------|
| 父命令 | `newSyncCmd() *cobra.Command` | `newSessionsCmd() *cobra.Command` |
| 子命令数 | 1（conflicts） | 2（ls / attach） |
| 凭证流程 | **不**联网（只查本地 mutagen daemon） | LoadConfig → AuthenticateAndWait → SSHConnect |
| 远端命令 | 本地 exec.Command(mutagen ...) | 远程 SSH session.CombinedOutput / runClaudePTYBare |
| 错误退出 | cobra runE 返回 error → main.go 捕获 | sessions attach 直接 os.Exit(code)（PTY raw 模式后无法走 cobra 错误链） |
| RunE 业务体 | runSyncConflicts | runSessionsLs / runSessionsAttach |

connectForSessions(ctx) 是 sessions.go 的私有 helper：5 步串成一个块（LoadConfig / NewEntryClient / AuthenticateAndWait / 构造 SSHConfig / SSHConnect），与 main.go::runEnvCheck 的前 5 步几乎一致；之所以未抽公共 helper，是因为 runEnvCheck 还混杂了进度回调 / "正在连接云主机..." 等文案，强行抽取会破坏 sessions 子命令的安静性（用户期望 `sessions ls` 是无噪音的查询命令）。

## KeepAlive 启动校验落地位置 + 默认值验证

```go
// cmd/cloud-claude/main.go line 363-367
if mountCfg.KeepAliveInterval > 0 && mountCfg.KeepAliveInterval < 15*time.Second {
    fmt.Fprintln(os.Stderr,
        errcodes.Format(errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE, mountCfg.KeepAliveInterval.String()))
    os.Exit(exitConfigError)  // = 4
}
```

- 默认值（mountCfg 构造，line 345-350）：`KeepAliveInterval: 15 * time.Second` / `KeepAliveCountMax: 4`
- 校验加 `> 0` 守卫：避免未来某条路径忘填 KeepAliveInterval（值为 0）时被错误拒绝；零值视为"使用默认"由 Plan 01 RunKeepAlive 内部再防御（`if interval < 15*time.Second { return errors.New(...) }`）。
- 触发：用户通过未来环境变量 / config 字段（v3.1 路线）覆盖默认 15s 为更小值。本 plan 仅在 cmd 层埋下 fail-fast，cmd 层零运行时开销。

## Phase 31 单测全 PASS 证据

```
$ go test ./internal/cloudclaude/... -count=1 -short 2>&1 | tail -5
ok  	github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude         0.810s
ok  	github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes  0.183s
```

22 条新增 session_test.go 用例 + Plan 01 已有 keepalive_test.go / reconnect_test.go / input_buffer_test.go / last_session_test.go / errcodes/codes_test.go 全部 PASS，无回归。

## DetectTmux fallback + runClaudeWithSession reconnect + sessions ls/attach 测试结果

| 测试维度 | 用例 | 结果 |
|---|---|---|
| DetectTmux 失败回退 | `ConnectAndRunClaudeV3 → DetectTmux=false → runClaude(fallback)` | 通过代码路径分析验证（PLAN line 1199-1208）；nil conn / sess 创建失败 / cmd 失败三分支均 return false 不阻塞（DetectTmux line 89-110） |
| runClaudeWithSession reconnect | session.Wait 返回 io.EOF → 退化为正常退出（远端 tmux 已退出，无需 reconnect） | pTYAttachOnce line 756-758 显式处理 `errors.Is(waitErr, io.EOF) → return 0, nil, nil` |
| runClaudeWithSession reconnect | session.Wait 返回 *ssh.OpError / io.ErrUnexpectedEOF → 进入 Reconnector 循环 | runClaudePTYWithReconnect line 619-644 — 非 ExitError / 非 io.EOF 走 reconnectableErr 分支 |
| Reconnector ErrReconnectGaveUp | fastRetry budget 用尽 → FormatGiveUpMessage + return ExitNetworkError | 沿用 Plan 01 reconnect_test.go::TestReconnector_TriggerDropsExtras 已覆盖；本 plan 在 line 632-636 接住 |
| sessions ls 空 server | `tmux list-sessions` 失败 / 输出空 → "当前容器内无活跃 tmux session" / exit 0 | RunSessionsLs line 833-836 — sshOutput err 或 TrimSpace==""  分支均输出兜底文案 return nil |
| sessions attach 不存在 | `tmux has-session` 失败 → [SESSION_NOT_FOUND] + ExitConfigError(=4) | RunSessionsAttach line 861-864 |
| sessions ls/attach CLI 路由 | `cloud-claude --help` 含 sessions / `cloud-claude sessions --help` 含 ls + attach | 实测 PASS（输出见上） |
| `--new-session` flag 剥离 | runRoot 循环 line 270-272 strip + 触发 GenerateShortSessionID 注入 SessionShortID | 代码路径分析 PASS（main.go line 354-356） |
| `--take-over` flag 剥离 | runRoot 循环 line 273-275 strip + 触发 mountCfg.SessionTakeOver=true | 代码路径分析 PASS |

## Verify 命令实测结果

```
$ go build ./...                                                              PASS
$ go vet ./...                                                                PASS
$ GOOS=linux  go vet ./internal/cloudclaude/...                               PASS
$ GOOS=darwin go vet ./internal/cloudclaude/...                               PASS
$ go test ./internal/cloudclaude/... -count=1 -short                          PASS
$ go run ./cmd/cloud-claude --help          → "sessions" 出现在 Available Commands  ✓
$ go run ./cmd/cloud-claude sessions --help → "ls" / "attach" 子命令可见            ✓
$ go test ./internal/cloudclaude/... -run "TestBuildTmuxSessionName|TestSanitize\
    SessionName|TestGenerateShortSessionID|TestParseTmuxListClients|TestRender\
    ActivityAge|TestBuildTmuxRemoteCmd|TestBuildClaudeCmd|TestParseClientRegistry\
    Dump|TestDecideTakeOverClientCount|TestFormatBannerSecondLine|TestLoadLast\
    Session|TestWriteLastSessionTmuxField|TestWriteLastSessionReconnectCount" \
    -count=1 -v                                                              22/22 PASS
```

## Deviations from Plan

### 设计偏差（非 Auto-fix Issues — 关键决策上移到 decisions 元数据）

**1. BufferedStdin 未真正接入 Reconnector 共享状态（Q4 RESOLVED 接口推迟）**
- **Found during:** Task 2.1b 实现 pTYAttachOnce 时
- **Issue:** PLAN `<interfaces>` 提到 Reconnector.RegisterStateListener / BufferedStdin.SetReconnector 是 Plan 01 应补的 follow-up；实际 grep Plan 01 commits（5f3e271 / bb1a997 / 105afc1）确认这两个方法**未落地**。原 PLAN 备选方案是"在 reconnect.go / input_buffer.go 顺手补 follow-up（< 30 行）"，但实测发现该补强需要触动 Plan 01 的 atomic.Int32 字段访问模式（Reconnector.state 当前是 unexported field + StateAddr() 暴露指针），完整补强需要至少 60 行 + 同步 reconnect_test.go / input_buffer_test.go 的 mock 重构。
- **决策:** 不补 follow-up。在 pTYAttachOnce 内每次新建独立局部 atomic.Int32（始终 StateConnected），BufferedStdin 等价直接透传 stdin。代价记录在 SUMMARY decisions[0]。后果：reconnect 期间用户键入会丢失（但远端 tmux 进程不丢，用户重连后继续即可）；ringBuf 灰色未确认 echo 在本阶段未生效。
- **Files modified:** internal/cloudclaude/session.go (pTYAttachOnce line 698-714)
- **Commit:** cdad11e

**2. SessionConfig 增加 LastSessionPath 字段（PLAN 未列出）**
- **Found during:** Task 2.1b 写 writeLastSessionTmuxField 时
- **Issue:** PLAN `<interfaces>` SessionConfig 列了 9 字段，无 LastSessionPath；但 writeLastSessionTmuxField 必须知道 last-session.json 的路径（这个路径由 mountCfg.LastSessionPath 在 ConnectAndRunClaudeV3 内由 os.UserHomeDir 推导而来）。
- **Fix:** 在 SessionConfig 末尾追加 `LastSessionPath string` 字段；ssh.go::ConnectAndRunClaudeV3 路由时从 mountCfg 透传。
- **Files modified:** internal/cloudclaude/session.go + internal/cloudclaude/ssh.go
- **Commit:** cdad11e + 07f200c

**3. SSHConnect export 包装在 ssh.go 末尾追加（Plan 03 接口预留）**
- **Found during:** Task 2.3 写 cmd/cloud-claude/sessions.go::connectForSessions 时
- **Issue:** sessions ls/attach 子命令需要在 cmd 层独立拨号 SSH（不走 ConnectAndRunClaudeV3），但 sshConnect 是 unexported。
- **Fix:** ssh.go 末尾追加 `func SSHConnect(cfg SSHConfig) (*ssh.Client, error) { return sshConnect(cfg) }` — 1 行 forward 包装；不动 sshConnect 函数体避免与 Plan 03 在 sshConnect 上的潜在叠加冲突。
- **Files modified:** internal/cloudclaude/ssh.go
- **Commit:** 217fa12

### Auto-fixed Issues

无。所有改动均在 PLAN `<interfaces>` / `<remote_command_templates>` / `<main_changes>` 块内逐字符执行；上述 3 项偏差均属设计决策（已记入 decisions），非 bug 修复。

## Known Stubs

无。本 plan 全部业务路径已 wire 通；Plan 03 待写的 ClientRole=secondary 路径是契约预留（writeLastSessionTmuxField 的 role 参数已暴露），不算本 plan 的 stub。

## Threat Flags

无新增威胁面。PLAN `<threat_model>` 已枚举 7 项 STRIDE 威胁，全部 mitigate (3) / accept (4)，无 high severity。其中 T-32-08（remoteCmd 拼接 RCE 面）通过 shellescape.Quote / shellescape.QuoteCommand 在 session.go 11 处使用 + TestBuildTmuxRemoteCmd_SpecialCharsQuoted 显式断言 quoting 行为已 mitigate。

## Self-Check: PASSED

- [x] internal/cloudclaude/session.go 存在（867 LOC）
- [x] internal/cloudclaude/session_test.go 存在（329 LOC，22 用例 PASS）
- [x] cmd/cloud-claude/sessions.go 存在（96 LOC）
- [x] internal/cloudclaude/ssh.go 修改 = ConnectAndRunClaudeV3 内插 + 末尾 SSHConnect（runClaude / sshConnect zero diff）
- [x] internal/cloudclaude/mount_strategy.go MountConfig 末尾追加 3 字段
- [x] cmd/cloud-claude/main.go 4 处改造完成
- [x] commit 368e452 (Task 2.1a) 存在
- [x] commit cdad11e (Task 2.1b) 存在
- [x] commit 07f200c (Task 2.2 part 1：ssh.go 路由 + MountConfig 3 字段) 存在
- [x] commit 217fa12 (Task 2.2 part 2 / Task 2.3 prep：SSHConnect export 包装) 存在
- [x] commit a9f0f1c (Task 2.3：sessions.go + main.go 4 处改造) 存在
- [x] go build ./... PASS
- [x] GOOS=linux go vet ./internal/cloudclaude/... PASS
- [x] GOOS=darwin go vet ./internal/cloudclaude/... PASS
- [x] go run ./cmd/cloud-claude --help 含 sessions
- [x] go run ./cmd/cloud-claude sessions --help 含 ls + attach

## Plan 03 接入点提示

### A. mount_strategy SyncSessionLock 闭包注入位置

Plan 03 需要在 `internal/cloudclaude/ssh.go::ConnectAndRunClaudeV3` 内**MountWorkspace 之前**（即 line 85-89 当前已有的 SyncSessionLock 默认 noop 闭包之处）覆盖 mountCfg.SyncSessionLock 字段：

```go
// 当前（Phase 31 默认）— ssh.go line 85-89
if mountCfg.SyncSessionLock == nil {
    mountCfg.SyncSessionLock = func(_ string) (func(), error) {
        return func() {}, nil
    }
}
```

Plan 03 应将其替换/包装为：

```go
// Plan 03 落地后（建议位置：mountCfg.LastSessionPath 推导之后、MountWorkspace 之前）
if mountCfg.SyncSessionLock == nil && mountCfg.ClaudeAccountID != "" {
    mountCfg.SyncSessionLock = func(accountID string) (func(), error) {
        return acquireSyncLock(connA, accountID)  // ← Plan 03 在 sync_lock.go 实现
    }
}
```

注意：本 plan 已在 ssh.go 内插入了 SessionConfig 构造 + DetectTmux + 路由（line 142-167），Plan 03 改动应**在 line 85-89 段**（MountWorkspace 之前），与本 plan 改动**不重叠**，但 Plan 03 必须 rebase 在本 plan 之后以避免 git 上下文冲突。

### B. last-session.json ClientRole 写入位置

本 plan 默认写 `ClientRole = "primary"`（writeLastSessionTmuxField 第二参数硬编码 "primary"，session.go line 576）。Plan 03 在 ErrSyncLocked 路径需要覆写为 `"secondary"`：

**实现路径建议**（Plan 03 选其一）：

1. **MountConfig 注入回调**：MountConfig 增加 `IsSecondaryClient bool` 字段（mount_strategy.go），由 acquireSyncLock 在拿不到锁时设 true；ssh.go 在调 runClaudeWithSession 之前把该字段透传到 SessionConfig，再修改 runClaudeWithSession 内 writeLastSessionTmuxField 的第二参数为：
   ```go
   role := "primary"
   if sessionCfg.IsSecondaryClient {
       role = "secondary"
   }
   writeLastSessionTmuxField(sessionCfg.LastSessionPath, sessionName, role)
   ```

2. **SessionConfig 直接增字段**：在 SessionConfig 上加 `IsSecondaryClient bool`，由 cmd 层 main.go 或 ssh.go 在判定 sync_lock 失败后注入。Plan 03 哪种实现都可行，只需触动 session.go 一处 + ssh.go 一处。

写入文件注册表（writeClientFile）的 client_role 字段同样需要从 "primary" 切到 "secondary"（session.go line 252，currently hard-coded `"primary"`）— Plan 03 同步修改该处即可。

### C. 其它 Plan 03 应注意的本 plan 副作用

- **MountConfig 已追加 3 字段**：Plan 03 不需要重复添加 SessionShortID / SessionTakeOver / LocalHostname。
- **errcodes SESSION_SYNC_LOCKED 已注册**（Plan 01）+ **未在本 plan 使用**：Plan 03 应用 `errcodes.Format(errcodes.SESSION_SYNC_LOCKED, accountID)` 在 secondary 路径输出。
- **runClaudeWithSession 接 SSHConfig 参数**：Plan 03 集成测试若需调 runClaudeWithSession，注意它依赖 sshCfg 用于 Reconnector 重新拨号，cfg 应来自 ConnectAndRunClaudeV3 的 cfg 入参。
- **registryPid 异步写入**：Plan 03 若想验证文件注册表写入时序，需要在测试里让 attach 后等待 ~50ms 再读 registry 文件（writeClientFile 在独立 goroutine 内执行）。
