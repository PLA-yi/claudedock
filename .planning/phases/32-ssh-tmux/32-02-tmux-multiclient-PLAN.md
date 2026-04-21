---
phase: 32
plan: 02-tmux-multiclient
type: execute
wave: 2
depends_on:
  - 01-net-resilience
autonomous: true
requirements:
  - REQ-F4-A
  - REQ-F4-B
  - REQ-F4-C
  - REQ-F5-A
  - REQ-F5-B
  - REQ-F5-C
files_modified:
  - internal/cloudclaude/session.go
  - internal/cloudclaude/session_test.go
  - internal/cloudclaude/ssh.go
  - cmd/cloud-claude/sessions.go
  - cmd/cloud-claude/main.go
must_haves:
  truths:
    - "session.DetectTmux(conn) 远程执行 'command -v tmux >/dev/null 2>&1 && tmux -V 2>&1'，失败返回 (false, \"\", reason)，**不阻塞**启动；ConnectAndRunClaudeV3 在 OAuth 检查后调用，失败时仍走 v2.0 runClaude（D-15 / D-16 / REQ-F4-C）"
    - "session.buildTmuxSessionName(accountID) 返回 'claude-<account_id_short8>'（前 8 字符小写去 \"-\"）；空 accountID 退化 'claude-anon-<simpleHash8(cwd)>'（与 mount_strategy.simpleHash8 同函数复用，**禁止**重新发明）；非法字符替换为 '_'，长度 > 32 截断 + warning（D-07 / D-08 / D-09）"
    - "session.buildShortIDSessionName() 用 crypto/rand 取 6 字节 → base64.RawURLEncoding（8 字符）→ 'claude-<short_id>'，与默认 8-hex 命名空间正交（D-08）"
    - "远程命令模板（D-10）：'cd <cwd> && command -v tmux >/dev/null 2>&1 && exec tmux new-session -A -d -s <session> <wrapCmd> \\; attach-session -t <session> || exec <fallback>'，全部参数走 shellescape.Quote（SP-03）；wrapCmd = 'cd <cwd> && claude <args>'（QuoteCommand）；fallback = 'cd <cwd> && claude <args>'（同 wrapCmd 但裸跑，不进 tmux）"
    - "runClaudeWithSession(ctx, conn, sshCfg, claudeArgs, cwd, hasProxy, sessionCfg) 从 runClaude fork：保留 PTY 申请 / SIGWINCH / ExitError 解包；改 remoteCmd 为 D-10 tmux 包装；启动后挂三个 goroutine — (a) RunKeepAlive(ctx, conn, sessionCfg.KeepAliveInterval, sessionCfg.KeepAliveCountMax)；(b) Reconnector.Run(ctx) 在 session.Wait 返回 io.EOF 时启动；(c) BufferedStdin.Run(ctx) 包装 os.Stdin 接 session.Stdin（D-28 / D-29）"
    - "原 runClaude 函数**不动**，用于 (a) tmux 不可用降级 fallback；(b) sessions attach 子命令复用（直接传 args=[]string + 在 session.go 内部覆盖 remoteCmd 为 'tmux attach-session -t <name>'）"
    - "--take-over 在 attach 前序列化执行：tmux list-clients -t <session> -F '#{client_pid}' → 命中 ≥ 1 则 tmux display-message + sleep 3 + tmux detach-client -t <session> -a → 然后本端 attach；list-clients 命中 0 时静默直接 attach（D-11）"
    - "第二端 banner（D-12 完整文件注册表方案 — RESEARCH §5.2 (a)）：cloud-claude 在 attach 前先 'tmux list-clients -t <session> -F #{client_pid}|#{client_activity}|#{client_tty}' 取既有 client PID 列表；attach 成功后立即远程查询本端 'tmux display-message -p #{client_pid}' 得 remote_tmux_client_pid，写入 /workspace/.cloud-claude/clients/<remote_tmux_client_pid>.json 文件注册表（schema_version=1 + hostname + tmux_client_pid + tmux_session + attach_at_unix + claude_account_id + client_role）；runClaudeWithSession defer 远程 'rm -f /workspace/.cloud-claude/clients/<remote_pid>.json' 清理；readClientHostnames(otherPids) 批量 cat JSON 文件解析 hostname，文件不存在 / JSON parse 失败 → 该 PID 在返回 map 中赋 'unknown-host'，**不**阻塞 banner 渲染（孤儿条目 — 即 PID 已不在 tmux list-clients 输出中的注册表 entry — 在本端 readClientHostnames 时跳过；本阶段被动清扫，主动清扫留 v3.1）"
    - "banner 文案模板（仅在其它 client 数 N ≥ 1 时输出第二行）：'✓ 已 attach 到会话 <session>\\n  （另 N 个会话正在共享：<hostname1> / <活跃时间1>，<hostname2> / <活跃时间2>）'；hostname 取自文件注册表（缺失则字面值 'unknown-host'，禁止汉化兜底以保证可见性）；时间渲染 < 30s='刚刚活跃' / < 1h='N 分钟前活跃' / >= 1h='N 小时前活跃'"
    - "本地 hostname 通过 Go 端 os.Hostname() 取（在 cmd/cloud-claude/main.go 已注入 mountCfg.LocalHostname → SessionConfig.LocalHostname → writeClientFile 远程命令的 JSON 字段）；os.Hostname 失败 fallback 到 'unknown-host' 字面值；写入命令通过 shellescape.Quote 引用 JSON 字符串避免 shell 注入"
    - "cloud-claude sessions ls 远程 'tmux list-sessions -F #{session_name}|#{session_created}|#{session_attached}|#{session_windows}'，本地 text/tabwriter 渲染表格（SESSION / CREATED / CLIENTS / WINDOWS 4 列）；list-sessions 失败 → 输出 '当前容器内无活跃 tmux session'，退出码 0（D-13）"
    - "cloud-claude sessions attach <name>：远程 'tmux has-session -t <name>' 失败 → stderr [SESSION_NOT_FOUND] + exit ExitConfigError(=4)；成功 → 复用 runClaude 但 remoteCmd 替换为 'exec tmux attach-session -t <name>'（不包 claude）（D-14）"
    - "cmd/cloud-claude/main.go 注册 newSessionsCmd() + DisableFlagParsing switch 追加 'sessions' + runRoot 内手动剥离 --new-session / --take-over flag（与 --mount-mode 同一循环；--new-session / --take-over 是布尔无值 flag）+ 启动期校验 mountCfg.KeepAliveInterval >= 15s 失败时 stderr [SESSION_KEEPALIVE_TOO_AGGRESSIVE] + os.Exit(exitConfigError=4)（D-03 第 4 条）"
    - "ConnectAndRunClaudeV3 在 OAuth 检查后插入 sessionCfg 构造 + DetectTmux + 路由到 runClaudeWithSession 或 runClaude；SessionConfig.AccountID = mountCfg.ClaudeAccountID；ShortID/TakeOver 通过 MountConfig 透传（在 MountConfig 上追加 SessionShortID string / SessionTakeOver bool 两个 omitempty 字段，由 main.go 注入）"
    - "runClaudeWithSession 写 last-session.json：TmuxSession=<实际 attach 的 session 名>；ClientRole 由 Plan 03 写（本 plan 不写）；ReconnectCount 由 Reconnector.ReconnectCount() 拉取，每次 Run 成功 +1 后 rewrite"
  artifacts:
    - path: "internal/cloudclaude/session.go"
      provides: "DetectTmux / buildTmuxSessionName / buildAnonTmuxSessionName / buildShortIDSessionName / sanitizeSessionName / SessionConfig struct / runClaudeWithSession / RunSessionsLs / RunSessionsAttach / banner 渲染 helper / 文件注册表 helper（writeClientFile / removeClientFile / readClientHostnames）"
      contains: "func runClaudeWithSession"
    - path: "internal/cloudclaude/session_test.go"
      provides: "纯函数单测：命名 / sanitize / 时间渲染 / list-clients 解析 / list-sessions 表格渲染 / take-over 决策"
      contains: "TestBuildTmuxSessionName"
    - path: "cmd/cloud-claude/sessions.go"
      provides: "newSessionsCmd cobra subcommand 树（ls / attach）+ runSessionsLs + runSessionsAttach（cobra 路由层；业务在 internal/cloudclaude/session.go）"
      contains: "func newSessionsCmd"
    - path: "internal/cloudclaude/ssh.go"
      provides: "ConnectAndRunClaudeV3 在 OAuth 检查后插入 SessionConfig 构造 + DetectTmux + 路由；MountConfig 追加 SessionShortID / SessionTakeOver 字段"
      contains: "runClaudeWithSession"
    - path: "cmd/cloud-claude/main.go"
      provides: "AddCommand newSessionsCmd + DisableFlagParsing switch 追加 sessions + 剥离 --new-session/--take-over + KeepAliveInterval 启动校验"
      contains: "newSessionsCmd"
  key_links:
    - from: "ssh.go::ConnectAndRunClaudeV3"
      to: "session.go::DetectTmux"
      via: "OAuth 检查后调用，结果决定走 runClaudeWithSession 或 runClaude"
      pattern: "DetectTmux"
    - from: "ssh.go::ConnectAndRunClaudeV3"
      to: "session.go::runClaudeWithSession"
      via: "tmux 可用时调用；接收 SessionConfig + connA"
      pattern: "runClaudeWithSession"
    - from: "session.go::runClaudeWithSession"
      to: "keepalive.go::RunKeepAlive"
      via: "go RunKeepAlive(ctx, conn, sessionCfg.KeepAliveInterval, sessionCfg.KeepAliveCountMax)"
      pattern: "RunKeepAlive"
    - from: "session.go::runClaudeWithSession"
      to: "reconnect.go::Reconnector"
      via: "session.Wait 返回 io.EOF 时 NewReconnector + Run"
      pattern: "NewReconnector"
    - from: "session.go::runClaudeWithSession"
      to: "input_buffer.go::BufferedStdin"
      via: "NewBufferedStdin(os.Stdin, &reconnector.state, os.Stdout, noColor, reconnector.Trigger)"
      pattern: "NewBufferedStdin"
    - from: "cmd/cloud-claude/sessions.go"
      to: "internal/cloudclaude/session.go::RunSessionsLs/Attach"
      via: "cobra RunE 调用业务 helper"
      pattern: "RunSessionsLs"
---

<plan_dependencies>
- **Plan 01（Wave 1）必须先完成并 commit**：本 plan 直接 import：
  - `errcodes.SESSION_TMUX_UNAVAILABLE / SESSION_NOT_FOUND / SESSION_TAKEOVER_NOTIFIED / SESSION_TAKEOVER_FAILED / SESSION_KEEPALIVE_TOO_AGGRESSIVE`（Plan 01 注册）
  - `cloudclaude.RunKeepAlive / NewReconnector / NewBufferedStdin / RingBufCapacity / FormatGiveUpMessage / ErrReconnectGaveUp`（Plan 01 暴露）
  - `cloudclaude.LastSessionSnapshot.TmuxSession / ReconnectCount`（Plan 01 添加）
  - `cloudclaude.ansiGray`（Plan 01 添加）
- **不**与 Plan 03 抢 ssh.go 同函数：本 plan 改 ConnectAndRunClaudeV3 的 OAuth-check 之后段（构造 SessionConfig + DetectTmux + runClaudeWithSession 路由）；Plan 03 改 MountWorkspace 之前段（mountCfg.SyncSessionLock 注入）。两段在 ConnectAndRunClaudeV3 内不重叠，但 Plan 03 必须在本 plan 之后 commit 以避免 git 冲突。
- 本 plan 必须在 commit 前先 cherry-pick 验证 Plan 01 已 merge（Plan 01 提交后再起）。
</plan_dependencies>

<objective>
落地 Phase 32 会话层 + 多端共享 attach：

1. **tmux 默认包装（REQ-F4-A）**：远程命令由 v2.0 裸 `claude <args>` 改为 `tmux new-session -A -d -s <claude-account_id_short8> <wrap> \; attach-session`；session 命名 per-claude_account（D-07）
2. **多端共享 attach（REQ-F5-A / B）**：默认 `new-session -A` 行为不踢人；第二端 banner 通过 `/workspace/.cloud-claude/clients/<pid>.json` 文件注册表识别其它 client 的 hostname + 活跃时间（修订 D-12）
3. **--new-session / --take-over（REQ-F5-C）**：cobra flag 剥离 + session 命名独立 + take-over 串行 detach 流程（D-08 / D-11）
4. **tmux 不可用降级（REQ-F4-C）**：DetectTmux 探测失败 → banner [SESSION_TMUX_UNAVAILABLE] + 走 v2.0 裸 runClaude，**不阻塞启动**（D-15 / D-16）
5. **sessions ls / attach 子命令（REQ-F4-B）**：纯客户端逻辑零控制面改造（D-13 / D-14 / OOS-A20）
6. **网络层接入**：runClaudeWithSession 内挂 RunKeepAlive + Reconnector + BufferedStdin 三个 Plan 01 service；KeepAliveInterval 启动校验（D-03 第 4 条）

Purpose: 兑现 ROADMAP §Phase 32 Success Criteria 第 2 / 7 / 8 / 9 / 10 条（tmux 包装 + 30s 抖动重连不丢进程 + sessions ls/attach + tmux 降级 + 多端共享 + take-over）。
Output: 1 个新文件 + 1 个新 cmd 文件 + 2 个改造文件；不引入新依赖；现有所有 Phase 31 集成测试不破坏。
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
@.planning/phases/32-ssh-tmux/plans/01-net-resilience/PLAN.md
@.planning/phases/29-v3-worker/29-CONTEXT.md
@.planning/phases/30-entry-api/30-CONTEXT.md
@.planning/phases/31-cli/31-CONTEXT.md
@internal/cloudclaude/ssh.go
@internal/cloudclaude/oauth_check.go
@internal/cloudclaude/mount.go
@internal/cloudclaude/mount_strategy.go
@internal/cloudclaude/keepalive.go
@internal/cloudclaude/reconnect.go
@internal/cloudclaude/input_buffer.go
@internal/cloudclaude/colors.go
@internal/cloudclaude/last_session.go
@internal/cloudclaude/exitcodes.go
@internal/cloudclaude/errcodes/codes.go
@internal/cloudclaude/errcodes/session.go
@cmd/cloud-claude/main.go
@cmd/cloud-claude/sync.go

<interfaces>
<!-- 本 plan 创建的对外 API。 -->

internal/cloudclaude/session.go 导出：

```go
package cloudclaude

import (
    "context"
    "golang.org/x/crypto/ssh"
)

// SessionConfig 由 main.go 构造、ConnectAndRunClaudeV3 透传给 runClaudeWithSession（CONTEXT D-29）。
type SessionConfig struct {
    AccountID         string        // 来自 mountCfg.ClaudeAccountID；空时退化 anon
    ShortID           string        // --new-session 用；非空时强制 session 名 = "claude-" + ShortID
    TakeOver          bool          // --take-over flag
    TmuxAvailable     bool          // DetectTmux 结果
    KeepAliveInterval time.Duration // 通常 = mountCfg.KeepAliveInterval
    KeepAliveCountMax int
    ReconnectEnabled  bool          // 默认 true；测试可关
    NoColor           bool          // 来自 mountCfg.NoColor
    Cwd               string        // 容器内远程 cwd（runClaude 同含义）
    LocalHostname     string        // 写入文件注册表用，等于 os.Hostname()
}

// DetectTmux 远程探测 tmux 可用性；失败返回 (false, "", reason) 不阻塞启动（D-15 / D-16）。
// 远程命令: command -v tmux >/dev/null 2>&1 && tmux -V 2>&1
func DetectTmux(conn *ssh.Client) (available bool, version string, reason string)

// runClaudeWithSession fork 自 runClaude；包 D-10 tmux 命令 + 启动 RunKeepAlive / Reconnector / BufferedStdin。
// 返回退出码（0 / runClaude 内 ExitError code）+ error。
func runClaudeWithSession(ctx context.Context, conn *ssh.Client, sshCfg SSHConfig,
    claudeArgs []string, sessionCfg SessionConfig, hasProxy bool) (int, error)

// RunSessionsLs 由 cmd/cloud-claude/sessions.go 调用；远程 tmux list-sessions + 本地表格渲染。
// 失败（无 server）→ stdout 输出 "当前容器内无活跃 tmux session"，return nil（exit 0）。
func RunSessionsLs(conn *ssh.Client, w io.Writer) error

// RunSessionsAttach 由 cmd/cloud-claude/sessions.go 调用；远程 tmux has-session 校验 → attach。
// session 不存在 → 返回 (ExitConfigError, error 含 SESSION_NOT_FOUND)。
func RunSessionsAttach(conn *ssh.Client, sessionName string, hasProxy bool, cwd string) (int, error)

// 命名 helpers（纯函数，便于单测）
func buildTmuxSessionName(accountID, cwd string) string  // 默认 + anon 退化
func buildShortIDSessionName() string                     // --new-session 用
func sanitizeSessionName(name string) (sanitized string, warned bool)
func GenerateShortSessionID() string                      // 暴露给 cmd 层 main.go 在 --new-session 时调用

// 文件注册表 helpers（D-12 完整方案 — Q1 RESOLVED）
// 全部在 attach 之后调用；conn 必须存活；远程命令通过 shellescape.Quote 拼接。

// writeClientFile attach 成功后立即调一次。
//   - 远程查询本端 'tmux display-message -p #{client_pid}' → remoteTmuxClientPid
//   - 写入 /workspace/.cloud-claude/clients/<remoteTmuxClientPid>.json （JSON schema 见 <remote_command_templates>）
//   - 返回 remoteTmuxClientPid 供 removeClientFile 在 defer 中清理
//   - 失败仅 log warning + 返回 (0, err)，不阻塞 attach
func writeClientFile(conn *ssh.Client, sessionName, accountID, hostname string) (remoteTmuxClientPid int, err error)

// removeClientFile defer 在 runClaudeWithSession 退出时调用。
//   - 远程 'rm -f /workspace/.cloud-claude/clients/<pid>.json'，错误忽略（孤儿条目下次 attach 被动跳过）
func removeClientFile(conn *ssh.Client, remoteTmuxClientPid int) error

// readClientHostnames 在 printAttachBanner 内调用，批量读取注册表 hostname。
//   - 远程单 SSH session 多 cat：for pid in <pids>; do echo "===$pid==="; cat /workspace/.cloud-claude/clients/${pid}.json 2>/dev/null || true; done
//   - 解析输出按 '===<pid>===' 分隔；JSON parse 失败 / 文件不存在 → 该 PID 在返回 map 中赋字面值 "unknown-host"
//   - 永不返回 error（unknown-host 是合法兜底），返回 map 长度永远 == len(otherClientPids)
func readClientHostnames(conn *ssh.Client, otherClientPids []int) map[int]string
```

internal/cloudclaude/reconnect.go 接口补强（Q4 RESOLVED — 共享 state 不暴露 *atomic.Int32 指针）：

```go
// Plan 01 Task 1.3 必须在 reconnect.go 暴露下列两个方法：

// State 返回当前 ConnState（atomic load — Plan 01 已暴露，本段重申）。
func (r *Reconnector) State() ConnState

// RegisterStateListener 注册一个回调，当 state 变更（Connected ↔ Reconnecting ↔ GaveUp）时同步触发。
//   - listener 在 Run goroutine 内同步调用，listener 自身必须 fast / non-blocking
//   - 多次注册按注册顺序依次调用
//   - 用于 BufferedStdin.SetReconnector 内挂接（避免裸共享 atomic.Int32）
func (r *Reconnector) RegisterStateListener(listener func(ConnState))
```

internal/cloudclaude/input_buffer.go 接口补强（Q4 RESOLVED — 注入式连接 Reconnector）：

```go
// Plan 01 Task 1.3 必须在 input_buffer.go 暴露下列方法：

// SetReconnector 一行注入；内部调用 r.RegisterStateListener 监听 state 变更。
//   - listener 内部更新 BufferedStdin.state（unexported 字段，无需暴露指针）
//   - 同时把 b.onEnter 默认设为 r.Trigger（如调用方未自定义 onEnter）
func (b *BufferedStdin) SetReconnector(r *Reconnector)
```

> **注**：此处接口补强反向影响 Plan 01 Task 1.3（需在该 task 暴露 RegisterStateListener / SetReconnector 两个方法）。Plan 01 已 commit 后此补强通过 follow-up 微改进入；本 plan execute 期 grep 验证 Plan 01 是否落地。如未落地，本 plan 在 session.go 内 fallback 用 `r.State()` 轮询模式（每次 BufferedStdin.Run 循环顶部检查），但**优先**走 listener 注入。

internal/cloudclaude/ssh.go MountConfig 追加（在 mount_strategy.go 的 MountConfig 上）：

```go
// 注：MountConfig 定义在 mount_strategy.go；本 plan 在该 struct 末尾追加 2 字段（omitempty 不需要因为是 Go struct，仅 JSON 时才有意义；这里直接追加）
type MountConfig struct {
    // ... 现有字段
    SessionShortID  string  // 来自 cmd 层 --new-session（cmd 自生成 8 字符 base64url short_id 后写入）
    SessionTakeOver bool    // 来自 cmd 层 --take-over
    LocalHostname   string  // 来自 os.Hostname()，session.go banner 用
}
```

(若担心污染 MountConfig — alternative 是：在 ConnectAndRunClaudeV3 增加新参数 sessionCfg SessionConfig；但这破坏 v2.0 ExecProxy 调用方。**planner 决定走 MountConfig 追加**，因为 MountConfig 已经是 v3.0 的"上下文配置块"，性质一致。)

cmd/cloud-claude/main.go 改造点见下方 <main_changes>。
</interfaces>

<remote_command_templates>
<!-- 全部远程命令模板；执行器必须用 shellescape.Quote / QuoteCommand 拼接，不许手写引号。 -->

### tmux 包装（D-10 — runClaudeWithSession 内）

```bash
cd <cwd_q> && command -v tmux >/dev/null 2>&1 && \
  exec tmux new-session -A -d -s <session_q> <wrap_q> \; attach-session -t <session_q> \
  || exec <fallback_cmd>
```

- `<cwd_q>` = `shellescape.Quote(remoteCwd)`
- `<session_q>` = `shellescape.Quote(sessionName)`
- `<wrap_q>` = `shellescape.Quote(wrapCmd)` — 注意 wrapCmd 是单参数串入 tmux new-session 的 shell 命令，整体被 quote 一次
- `wrapCmd` = `cd <cwd_q> && claude <args>`（hasProxy 时多 `export PATH=<binDir_q>:$PATH && `）
- `<fallback_cmd>` = wrapCmd（不再 quote — 直接走 exec）

### tmux 探测（DetectTmux）

```bash
command -v tmux >/dev/null 2>&1 && tmux -V 2>&1
```

### 多端 banner 数据源（D-12 完整文件注册表方案 — RESEARCH §5.2 (a)）

**registry 文件路径**：`/workspace/.cloud-claude/clients/<remote_tmux_client_pid>.json`（UID 1000 默认可写 /workspace；不污染 mergerfs/Mutagen — Plan 02 沿用 Phase 31 mutagen ignore 中已有的 `.cloud-claude/` 顶层匹配）。

**JSON schema**（schema_version=1，新字段全 omitempty 友好；本阶段写满全字段）：

```json
{
  "schema_version": 1,
  "hostname": "<local_hostname>",
  "tmux_client_pid": <int>,
  "tmux_session": "<session_name>",
  "attach_at_unix": <int>,
  "claude_account_id": "<uuid_or_anon>",
  "client_role": "primary"
}
```

注：`client_role` 字段的 `secondary` 取值由 Plan 03 的 IsSecondaryClient 透传链负责赋值；本 plan 写入 entry 时永远写 `primary`（即"我能 attach 上即视为本端是 primary 视角"），Plan 03 在 ErrSyncLocked 路径覆写。

**写入命令**（远程 SSH 执行；attach 成功后立即调一次）：

```bash
mkdir -p /workspace/.cloud-claude/clients && \
printf '%s' <json_q> > /workspace/.cloud-claude/clients/$(tmux display-message -p '#{client_pid}').json
```

- `<json_q>` = `shellescape.Quote(<完整 JSON 字符串>)`，由 Go 端 `json.Marshal` 后再 quote
- `<local_hostname>` 由 Go 端 `os.Hostname()` 取得（main.go 已注入 mountCfg.LocalHostname → SessionConfig.LocalHostname）；`os.Hostname` 失败 fallback 字面值 `unknown-host`
- `tmux display-message -p '#{client_pid}'` 返回当前 SSH session 内 attach 的 tmux client PID（远端 PID，与本地 cloud-claude 进程 PID 不同）
- 命令使用 `printf '%s'` 而非 `echo`：避免 echo 的转义差异，shellescape.Quote 已处理特殊字符

**attach 前查询既有 client PID 列表**：

```bash
tmux list-clients -t <session_q> -F '#{client_pid}|#{client_activity}|#{client_tty}' 2>/dev/null
```

解析每行 `pid|unix_seconds|tty`；`pid` 用于查询注册表 hostname，`unix_seconds` 用于渲染活跃时间。

**批量读取注册表（readClientHostnames helper）**：单次 SSH session 内多次 `cat`，减少往返：

```bash
for pid in <pid1> <pid2> <pid3>; do
  echo "===$pid==="
  cat /workspace/.cloud-claude/clients/${pid}.json 2>/dev/null || true
done
```

Go 端按 `===<pid>===` 分隔解析；每段空 / JSON parse 失败 → 该 PID 在返回 `map[int]string` 中赋字面值 `unknown-host`，**不阻塞** banner 渲染。

**退出时清理（runClaudeWithSession defer）**：

```bash
rm -f /workspace/.cloud-claude/clients/<本端 remote_tmux_client_pid>.json
```

remote_tmux_client_pid 在 attach 后已被 Go 端缓存（writeClientFile 返回值），defer rm 引用本地变量；如 SSH 异常断开导致 rm 未执行 → 孤儿 entry 在下次本端 / 第三端 attach 时通过 `tmux list-clients` 输出对照被动跳过（本端不在 active client list 中即视为孤儿，readClientHostnames 不会查它）。

**本阶段不主动清扫孤儿条目**（启动期 GC 留 v3.1 — 与 Mutagen daemon GC 一并实现）。

### --take-over 序列（D-11）

```bash
# 1. 探测
tmux list-clients -t <session_q> -F '#{client_pid}' 2>/dev/null
# 2. 命中 ≥ 1 时
tmux display-message -t <session_q> '[cloud-claude] 另一端已通过 --take-over 接管会话，本会话将在 3s 后断开'
sleep 3
tmux detach-client -t <session_q> -a
# 3. 然后本端 attach（走 D-10 模板）
```

### sessions ls

```bash
tmux list-sessions -F '#{session_name}|#{session_created}|#{session_attached}|#{session_windows}' 2>/dev/null
```

### sessions attach

```bash
# 校验
tmux has-session -t <name_q> 2>/dev/null
# 成功后
exec tmux attach-session -t <name_q>
```
</remote_command_templates>

<main_changes>
<!-- cmd/cloud-claude/main.go 4 处改动 — 与 PATTERNS §main.go 改造段对齐 -->

### A. AddCommand（line 92）

```go
rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd())
```

### B. DisableFlagParsing switch（line 96-101）

```go
if len(os.Args) > 1 {
    switch os.Args[1] {
    case "init", "env", "ssh", "sync", "sessions", "help", "--help", "-h":
        rootCmd.DisableFlagParsing = false
    }
}
```

### C. runRoot 内剥离 --new-session / --take-over（line 244-269 现有 --mount-mode 循环改造）

```go
mountMode := "auto"
newSession := false
takeOver := false
filtered := args[:0]
for i := 0; i < len(args); i++ {
    switch {
    case args[i] == "--mount-mode" && i+1 < len(args):
        mountMode = args[i+1]; i++; continue
    case strings.HasPrefix(args[i], "--mount-mode="):
        mountMode = strings.TrimPrefix(args[i], "--mount-mode="); continue
    case args[i] == "--new-session":
        newSession = true; continue
    case args[i] == "--take-over":
        takeOver = true; continue
    }
    filtered = append(filtered, args[i])
}
args = filtered
```

### D. KeepAliveInterval 启动校验 + ShortID 生成 + MountConfig 注入

在 mountCfg 构造（line 335 附近）后插入：

```go
mountCfg.SessionTakeOver = takeOver
if newSession {
    mountCfg.SessionShortID = cloudclaude.GenerateShortSessionID() // 在 session.go 暴露 8 字符 base64url
}
if hostname, _ := os.Hostname(); hostname != "" {
    mountCfg.LocalHostname = hostname
}

// [Phase 32 D-03] keepalive_interval 启动校验
if mountCfg.KeepAliveInterval < 15*time.Second {
    fmt.Fprintln(os.Stderr,
        errcodes.Format(errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE, mountCfg.KeepAliveInterval.String()))
    os.Exit(exitConfigError) // = 4
}
```

注：`mountCfg.KeepAliveInterval` 默认值在 Phase 31 D-31 已经设为 15s；本校验是为未来用户自定义环境变量 / config 覆盖时的防御。
</main_changes>
</context>

<tasks>

<task type="auto">
  <name>Task 2.1a: session.go 基础层（DetectTmux + 命名 helpers + 远程命令模板 + 文件注册表 helpers + 纯函数单测）</name>
  <files>
    internal/cloudclaude/session.go
    internal/cloudclaude/session_test.go
  </files>
  <read_first>
    - internal/cloudclaude/oauth_check.go（DetectTmux mirror CheckOAuthCredentials line 44-63 — conn.NewSession + sess.Run + 错误收敛）
    - internal/cloudclaude/mount_strategy.go（buildSessionName line 435-459 + simpleHash8 line 461-470 — 直接复用 simpleHash8，不重新发明）
    - internal/cloudclaude/colors.go（colorize / colorEnabled — banner 渲染要用）
    - internal/cloudclaude/errcodes/codes.go（SESSION_TMUX_UNAVAILABLE / SESSION_NOT_FOUND）
    - internal/cloudclaude/mount.go（sshRun helper line 105-112 — 远程命令复用，不再新建）
    - .planning/phases/32-ssh-tmux/32-CONTEXT.md D-07 / D-08 / D-09 / D-10 / D-12 / D-15
    - .planning/phases/32-ssh-tmux/32-RESEARCH.md §5（关键修订点：D-12 文件注册表完整方案 / §5.4 sessions ls）
    - .planning/phases/32-ssh-tmux/32-PATTERNS.md session.go 段（必读）
    - .planning/phases/32-ssh-tmux/plans/02-tmux-multiclient/PLAN.md `<remote_command_templates>` 段（writeClientFile / readClientHostnames JSON schema 与远程命令字面值）
  </read_first>
  <action>
    本 task 落地 session.go 的**基础层**（无 PTY / 无 reconnect 协同）— Task 2.1b 在此之上接入 runClaudeWithSession / take-over / sessions ls/attach。

    1. **internal/cloudclaude/session.go** 包结构（package cloudclaude；imports 含 bytes / context / crypto/rand / encoding/base64 / encoding/json / fmt / io / os / strings / strconv / text/tabwriter / time + golang.org/x/crypto/ssh + al.essio.dev/pkg/shellescape + 内部 errcodes 包）。

    2. **SessionConfig struct** — 与 <interfaces> 块完全一致（含 LocalHostname 字段）。

    3. **DetectTmux**（mirror oauth_check.go::CheckOAuthCredentials 的 conn.NewSession + sess.Run + bytes.Buffer 收 stdout/stderr 模式）：

       ```go
       func DetectTmux(conn *ssh.Client) (available bool, version string, reason string) {
           if conn == nil { return false, "", "no connection" }
           sess, err := conn.NewSession()
           if err != nil { return false, "", err.Error() }
           defer sess.Close()
           var buf bytes.Buffer
           sess.Stdout = &buf
           sess.Stderr = &buf
           runErr := sess.Run("command -v tmux >/dev/null 2>&1 && tmux -V 2>&1")
           if runErr != nil {
               return false, "", strings.TrimSpace(buf.String())
           }
           return true, strings.TrimSpace(buf.String()), ""
       }
       ```

    4. **命名 helpers**：

       ```go
       // buildTmuxSessionName 默认 session 命名（D-07）。空 accountID 退化 anon（D-09）。
       // 长度 > 32 截断；非法字符替换 _ 并 stderr warning。
       func buildTmuxSessionName(accountID, cwd string) string {
           var raw string
           if accountID == "" {
               raw = "claude-anon-" + simpleHash8(cwd)
           } else {
               id8 := strings.ToLower(strings.ReplaceAll(accountID, "-", ""))
               if len(id8) > 8 { id8 = id8[:8] }
               raw = "claude-" + id8
           }
           sanitized, _ := sanitizeSessionName(raw)
           return sanitized
       }

       // buildShortIDSessionName 用于 --new-session（D-08）。crypto/rand 6 字节 → base64url 8 字符。
       func buildShortIDSessionName() string {
           id := GenerateShortSessionID()
           return "claude-" + id
       }

       // GenerateShortSessionID 暴露给 cmd/cloud-claude/main.go 在 --new-session flag 触发时调用。
       func GenerateShortSessionID() string {
           buf := make([]byte, 6)
           if _, err := rand.Read(buf); err != nil {
               // 极端情况退化到时间戳后缀
               return strconv.FormatInt(time.Now().UnixNano(), 36)[:8]
           }
           return base64.RawURLEncoding.EncodeToString(buf) // 8 字符
       }

       // sanitizeSessionName 字符集 [a-zA-Z0-9_-]，非法替换 _；长度 > 32 截断。返回 (sanitized, warned)。
       func sanitizeSessionName(name string) (string, bool) {
           warned := false
           var b strings.Builder
           for _, r := range name {
               if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
                   b.WriteRune(r)
               } else {
                   b.WriteRune('_')
                   warned = true
               }
           }
           sanitized := b.String()
           if len(sanitized) > 32 {
               sanitized = sanitized[:32]
               warned = true
           }
           return sanitized, warned
       }
       ```

       注：simpleHash8 直接复用 mount_strategy.go line 461-470 已有函数（同包，无需 import）。**禁止**重新实现。

    5. **远程命令构造 helpers**：

       ```go
       // buildTmuxRemoteCmd 构造 D-10 完整远程命令。所有参数已 shellescape。
       func buildTmuxRemoteCmd(remoteCwd, sessionName, claudeCmd string) string {
           cwdQ := shellescape.Quote(remoteCwd)
           sessionQ := shellescape.Quote(sessionName)
           // wrapCmd 整体被 tmux new-session 当一个参数（再次 quote）
           wrapCmd := fmt.Sprintf("cd %s && %s", cwdQ, claudeCmd)
           wrapQ := shellescape.Quote(wrapCmd)
           // fallback 是 wrapCmd 直接 exec（不进 tmux）
           return fmt.Sprintf(
               "cd %s && command -v tmux >/dev/null 2>&1 && exec tmux new-session -A -d -s %s %s \\; attach-session -t %s || exec %s",
               cwdQ, sessionQ, wrapQ, sessionQ, wrapCmd)
       }

       // buildClaudeCmd 复用 ssh.go::runClaude line 216-224 的逻辑，但单独拆为 helper。
       func buildClaudeCmd(claudeArgs []string, hasProxy bool, remoteCwd string) string {
           claudeCmd := shellescape.QuoteCommand(append([]string{"claude"}, claudeArgs...))
           if hasProxy {
               binDir := remoteCwd + "/.cloud-claude/bin"
               return fmt.Sprintf("export PATH=%s:$PATH && %s",
                   shellescape.Quote(binDir), claudeCmd)
           }
           return claudeCmd
       }
       ```

    4. **命名 helpers**（与 PATTERNS analog 一致 — 直接复用 mount_strategy.simpleHash8）：

       ```go
       // buildTmuxSessionName 默认 session 命名（D-07）。空 accountID 退化 anon（D-09）。
       func buildTmuxSessionName(accountID, cwd string) string {
           var raw string
           if accountID == "" {
               raw = "claude-anon-" + simpleHash8(cwd)
           } else {
               id8 := strings.ToLower(strings.ReplaceAll(accountID, "-", ""))
               if len(id8) > 8 { id8 = id8[:8] }
               raw = "claude-" + id8
           }
           sanitized, _ := sanitizeSessionName(raw)
           return sanitized
       }

       // GenerateShortSessionID — 暴露给 cmd 层 main.go 在 --new-session flag 触发时调用（D-08）。
       func GenerateShortSessionID() string {
           buf := make([]byte, 6)
           if _, err := rand.Read(buf); err != nil {
               return strconv.FormatInt(time.Now().UnixNano(), 36)[:8]
           }
           return base64.RawURLEncoding.EncodeToString(buf) // 8 字符
       }

       // sanitizeSessionName 字符集 [a-zA-Z0-9_-]，非法替换 _；长度 > 32 截断。
       func sanitizeSessionName(name string) (string, bool) { /* ... 见 PATTERNS session.go 段 ... */ }
       ```

    5. **远程命令构造 helpers**：`buildTmuxRemoteCmd(remoteCwd, sessionName, claudeCmd)` 与 `buildClaudeCmd(claudeArgs, hasProxy, remoteCwd)` — 完整代码见 32-PATTERNS.md session.go 段 + 本 plan `<remote_command_templates>` 段（D-10 模板字面值禁止改动）。

    6. **文件注册表 helpers**（D-12 完整方案 — Q1 RESOLVED；本 task 必须落地，Task 2.1b 调用）：

       ```go
       const clientsRegistryDir = "/workspace/.cloud-claude/clients"

       // clientFileSchema 是写入 JSON 的 schema_version=1 结构。
       type clientFileSchema struct {
           SchemaVersion   int    `json:"schema_version"`
           Hostname        string `json:"hostname"`
           TmuxClientPID   int    `json:"tmux_client_pid"`
           TmuxSession     string `json:"tmux_session"`
           AttachAtUnix    int64  `json:"attach_at_unix"`
           ClaudeAccountID string `json:"claude_account_id"`
           ClientRole      string `json:"client_role"` // 本 plan 始终写 "primary"；Plan 03 路径覆写为 "secondary"
       }

       // writeClientFile 在 attach 成功后立即调一次。
       //   1. 远程查询本端 client_pid: tmux display-message -p '#{client_pid}'
       //   2. 远程写入 /workspace/.cloud-claude/clients/<pid>.json
       //   3. 返回 pid 供调用方 defer 中 removeClientFile 用
       //   4. 失败仅返回 (0, err)；调用方 log warning 不阻塞
       func writeClientFile(conn *ssh.Client, sessionName, accountID, hostname string) (int, error) {
           if hostname == "" { hostname = "unknown-host" }

           // 1. 远程查 client_pid
           pidOut, err := sshOutput(conn, "tmux display-message -p '#{client_pid}' 2>/dev/null")
           if err != nil { return 0, fmt.Errorf("tmux display-message 失败: %w", err) }
           pid, err := strconv.Atoi(strings.TrimSpace(pidOut))
           if err != nil || pid <= 0 { return 0, fmt.Errorf("解析 client_pid 失败: %q", pidOut) }

           // 2. 构造 JSON + shellescape
           entry := clientFileSchema{
               SchemaVersion:   1,
               Hostname:        hostname,
               TmuxClientPID:   pid,
               TmuxSession:     sessionName,
               AttachAtUnix:    time.Now().Unix(),
               ClaudeAccountID: accountID,
               ClientRole:      "primary",
           }
           jsonBytes, err := json.Marshal(entry)
           if err != nil { return 0, err }

           // 3. 远程写文件 — 用 printf '%s' 避免 echo 转义差异
           writeCmd := fmt.Sprintf(
               "mkdir -p %s && printf '%%s' %s > %s/%d.json",
               shellescape.Quote(clientsRegistryDir),
               shellescape.Quote(string(jsonBytes)),
               shellescape.Quote(clientsRegistryDir),
               pid,
           )
           if err := sshRun(conn, writeCmd); err != nil {
               return pid, fmt.Errorf("写注册表失败: %w", err)
           }
           return pid, nil
       }

       // removeClientFile 在 runClaudeWithSession defer 退出时调用；失败忽略。
       func removeClientFile(conn *ssh.Client, remoteTmuxClientPid int) error {
           if remoteTmuxClientPid <= 0 { return nil }
           rmCmd := fmt.Sprintf("rm -f %s/%d.json",
               shellescape.Quote(clientsRegistryDir), remoteTmuxClientPid)
           return sshRun(conn, rmCmd)
       }

       // readClientHostnames 批量读取注册表 hostname；缺失 / parse 失败 → "unknown-host"。
       // 永不返回 error；返回 map 长度 == len(otherClientPids)。
       func readClientHostnames(conn *ssh.Client, otherClientPids []int) map[int]string {
           result := make(map[int]string, len(otherClientPids))
           for _, pid := range otherClientPids { result[pid] = "unknown-host" }
           if len(otherClientPids) == 0 { return result }

           // 单 SSH session 多 cat — for pid in <pids>; do echo "===<pid>==="; cat .../<pid>.json 2>/dev/null || true; done
           pidsList := make([]string, len(otherClientPids))
           for i, p := range otherClientPids { pidsList[i] = strconv.Itoa(p) }
           script := fmt.Sprintf(
               `for pid in %s; do echo "===${pid}==="; cat %s/${pid}.json 2>/dev/null || true; done`,
               strings.Join(pidsList, " "),
               shellescape.Quote(clientsRegistryDir),
           )
           out, err := sshOutput(conn, script)
           if err != nil { return result } // 全 unknown-host

           // 解析 ===PID===\n<json>\n===PID===\n... 段
           sections := strings.Split(out, "===")
           // sections[0] 通常空；之后成对：奇数 index = "<pid>===\n<json>"
           for i := 1; i < len(sections); i++ {
               sec := sections[i]
               eq := strings.Index(sec, "===")
               if eq < 0 { continue }
               pidStr := sec[:eq]
               body := strings.TrimSpace(sec[eq+3:])
               pid, e := strconv.Atoi(strings.TrimSpace(pidStr))
               if e != nil || body == "" { continue }
               var entry clientFileSchema
               if json.Unmarshal([]byte(body), &entry) == nil && entry.Hostname != "" {
                   result[pid] = entry.Hostname
               }
           }
           return result
       }

       // sshOutput / sshRun helpers —
       //   sshRun 已在 mount.go line 105-112 暴露（同包），直接调用
       //   sshOutput 在 mount 包未暴露；本 task 在 session.go 内私有定义（不修改 mount.go 减少冲突面）
       func sshOutput(conn *ssh.Client, cmd string) (string, error) {
           sess, err := conn.NewSession()
           if err != nil { return "", err }
           defer sess.Close()
           out, err := sess.CombinedOutput(cmd)
           return string(out), err
       }
       ```

    7. **list-clients / list-sessions 解析 + 时间渲染 helpers**（被 Task 2.1b 的 banner / RunSessionsLs 调用）：

       ```go
       type tmuxClient struct {
           PID      int
           Activity time.Time
           TTY      string
       }

       func parseTmuxListClients(out string) []tmuxClient {
           var clients []tmuxClient
           for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
               if line == "" { continue }
               fields := strings.SplitN(line, "|", 3)
               if len(fields) < 3 { continue }
               pid, _ := strconv.Atoi(fields[0])
               actSec, _ := strconv.ParseInt(fields[1], 10, 64)
               clients = append(clients, tmuxClient{PID: pid, Activity: time.Unix(actSec, 0), TTY: fields[2]})
           }
           return clients
       }

       func renderActivityAge(d time.Duration) string {
           switch {
           case d < 30*time.Second: return "刚刚活跃"
           case d < time.Hour: return fmt.Sprintf("%d 分钟前活跃", int(d.Minutes()))
           default: return fmt.Sprintf("%d 小时前活跃", int(d.Hours()))
           }
       }
       ```

    8. **session_test.go**（≥ 6 个纯函数单测；其它 4 个集成边界单测在 Task 2.1b 落地）：

       ```go
       func TestBuildTmuxSessionName_Default(t *testing.T)              // claude-abcdef12
       func TestBuildTmuxSessionName_AnonFallback(t *testing.T)          // claude-anon-<hash8>
       func TestSanitizeSessionName_IllegalChars(t *testing.T)           // / → _
       func TestSanitizeSessionName_TooLong(t *testing.T)                // 50 chars → 32
       func TestGenerateShortSessionID_8Chars(t *testing.T)              // crypto/rand 100 次
       func TestParseTmuxListClients_3Lines(t *testing.T)                // pipe 解析
       func TestParseTmuxListClients_Empty(t *testing.T)                 // 空输入 → 0 clients
       func TestRenderActivityAge_Thresholds(t *testing.T)               // 三档时间渲染
       func TestBuildTmuxRemoteCmd_ContainsAllParts(t *testing.T)        // D-10 模板字面
       func TestBuildClaudeCmd_WithProxy(t *testing.T)                   // hasProxy 路径
       ```

       具体测试代码骨架与原 PLAN（被本次拆分前）保持一致 — 执行器直接复制 PATTERNS / 历史 PLAN 的对应测试函数。

    9. **本 task 范围限制**：
       - **不**实现 runClaudeWithSession（Task 2.1b）
       - **不**实现 performTakeOver（Task 2.1b — 调 sshOutput 但属于 take-over 业务）
       - **不**实现 printAttachBanner（Task 2.1b — 但其依赖的 parseTmuxListClients / renderActivityAge / readClientHostnames 在本 task 落地）
       - **不**实现 RunSessionsLs / RunSessionsAttach（Task 2.1b — 这两个 helper 涉及 PTY，与 runClaudeWithSession 同属"高层"）
       - **不**改 ssh.go / mount_strategy.go / cmd/cloud-claude/*（Task 2.2 / 2.3）
  </action>
  <acceptance_criteria>
    - `go build ./internal/cloudclaude/...` 成功
    - `go vet ./internal/cloudclaude/...` 通过
    - `go test ./internal/cloudclaude/... -run "TestBuildTmuxSessionName|TestSanitizeSessionName|TestGenerateShortSessionID|TestParseTmuxListClients|TestRenderActivityAge|TestBuildTmuxRemoteCmd|TestBuildClaudeCmd" -count=1` 全部 PASS（≥ 10 用例）
    - `rg -n "func DetectTmux" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "type SessionConfig struct" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "func GenerateShortSessionID" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "func buildTmuxSessionName" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "func sanitizeSessionName" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "func buildTmuxRemoteCmd" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "tmux new-session -A -d -s" internal/cloudclaude/session.go` 命中（D-10 模板字面）
    - `rg -n "func writeClientFile\\(" internal/cloudclaude/session.go` 命中 1 行（**REQ-F5-B 关键**）
    - `rg -n "func removeClientFile\\(" internal/cloudclaude/session.go` 命中 1 行（**REQ-F5-B 关键**）
    - `rg -n "func readClientHostnames\\(" internal/cloudclaude/session.go` 命中 1 行（**REQ-F5-B 关键**）
    - `rg -n "schema_version" internal/cloudclaude/session.go` 命中（clientFileSchema struct）
    - `rg -n "/workspace/.cloud-claude/clients" internal/cloudclaude/session.go` 命中（registry 路径常量）
    - `rg -n "tmux display-message -p" internal/cloudclaude/session.go` 命中（取 client_pid）
    - `rg -n "shellescape.Quote" internal/cloudclaude/session.go` 命中（≥ 5 次）
    - `rg -n "simpleHash8" internal/cloudclaude/session.go` 命中（复用 mount_strategy.simpleHash8，禁止重新发明）
    - **不应**命中：`rg -n "func runClaudeWithSession" internal/cloudclaude/session.go` 应**为空**（Task 2.1b 落地）
    - **不应**命中：`rg -n "func performTakeOver|func printAttachBanner|func RunSessionsLs|func RunSessionsAttach" internal/cloudclaude/session.go` 应**为空**（Task 2.1b 落地）
  </acceptance_criteria>
  <verify>
    <automated>go test ./internal/cloudclaude/... -run "TestBuildTmuxSessionName|TestSanitizeSessionName|TestGenerateShortSessionID|TestParseTmuxListClients|TestRenderActivityAge|TestBuildTmuxRemoteCmd|TestBuildClaudeCmd" -count=1 &amp;&amp; go vet ./internal/cloudclaude/... &amp;&amp; rg -q "func writeClientFile" internal/cloudclaude/session.go &amp;&amp; rg -q "func removeClientFile" internal/cloudclaude/session.go &amp;&amp; rg -q "func readClientHostnames" internal/cloudclaude/session.go</automated>
  </verify>
  <done>session.go 基础层 + 文件注册表 helpers + 纯函数 helpers + 10 个纯函数单测 PASS。Task 2.1b 在其上添加 runClaudeWithSession / performTakeOver / printAttachBanner / RunSessionsLs/Attach。</done>
</task>

<task type="auto">
  <name>Task 2.1b: session.go 高层（runClaudeWithSession + take-over + printAttachBanner + sessions ls/attach + 集成边界单测）</name>
  <files>
    internal/cloudclaude/session.go
    internal/cloudclaude/session_test.go
  </files>
  <read_first>
    - internal/cloudclaude/session.go（Task 2.1a 落地的 DetectTmux / SessionConfig / 命名 helpers / 文件注册表 helpers / parseTmuxListClients / renderActivityAge / sshOutput）
    - internal/cloudclaude/ssh.go（runClaude line 168-241 — runClaudePTYBare 复刻蓝本；保留 PTY/SIGWINCH/ExitError）
    - internal/cloudclaude/keepalive.go（Plan 01）— RunKeepAlive 签名
    - internal/cloudclaude/reconnect.go（Plan 01）— NewReconnector / ConnState / ErrReconnectGaveUp / RegisterStateListener / ReconnectCount / FormatGiveUpMessage
    - internal/cloudclaude/input_buffer.go（Plan 01）— NewBufferedStdin / SetReconnector / Flush / Close
    - internal/cloudclaude/last_session.go（Plan 01）— LastSessionSnapshot + 现有 Read/Write API（grep 找现网名字 LoadLastSession / SaveLastSession，禁止改 schema）
    - internal/cloudclaude/colors.go（colorize / colorEnabled / ansiGreen / ansiGray）
    - internal/cloudclaude/exitcodes.go（ExitConfigError=4 / ExitNetworkError=2）
    - internal/cloudclaude/errcodes/codes.go（SESSION_NOT_FOUND / SESSION_TAKEOVER_NOTIFIED / SESSION_TAKEOVER_FAILED）
    - .planning/phases/32-ssh-tmux/32-CONTEXT.md D-10 / D-11 / D-12 / D-13 / D-14 / D-22 / D-27 / D-28 / D-29
    - .planning/phases/32-ssh-tmux/32-RESEARCH.md §3 / §5.3 / §5.4
    - .planning/phases/32-ssh-tmux/plans/02-tmux-multiclient/PLAN.md `<interfaces>` 段（Reconnector.RegisterStateListener + BufferedStdin.SetReconnector — Q4 RESOLVED 共享接口）
    - .planning/phases/32-ssh-tmux/plans/02-tmux-multiclient/PLAN.md `<remote_command_templates>` 段（writeClientFile / printf 模板）
  </read_first>
  <action>
    本 task 在 Task 2.1a 落地的基础上添加**高层 PTY / reconnect 协同 / 业务流程**层。

    1. **performTakeOver**（D-11，调 sshOutput / sshRun helper）：

       ```go
       func performTakeOver(conn *ssh.Client, sessionName string) error {
           sessQ := shellescape.Quote(sessionName)
           // 1. list-clients 探测既有客户端
           out, _ := sshOutput(conn, fmt.Sprintf("tmux list-clients -t %s -F '#{client_pid}' 2>/dev/null", sessQ))
           clientCount := 0
           if trim := strings.TrimSpace(out); trim != "" {
               clientCount = strings.Count(trim, "\n") + 1
           }
           if clientCount == 0 { return nil }

           // 2. display-message 通知（参数 shellescape）
           msg := "[cloud-claude] 另一端已通过 --take-over 接管会话，本会话将在 3s 后断开"
           _ = sshRun(conn, fmt.Sprintf("tmux display-message -t %s %s", sessQ, shellescape.Quote(msg)))

           // 3. sleep 3 + detach-client -a
           if err := sshRun(conn, fmt.Sprintf("sleep 3 && tmux detach-client -t %s -a", sessQ)); err != nil {
               return err
           }
           fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.SESSION_TAKEOVER_NOTIFIED, clientCount, sessionName))
           return nil
       }
       ```

    2. **printAttachBanner**（D-12 完整 — 调 readClientHostnames）：

       ```go
       func printAttachBanner(w io.Writer, conn *ssh.Client, sessionName string, noColor bool) {
           sessQ := shellescape.Quote(sessionName)
           out, _ := sshOutput(conn,
               fmt.Sprintf("tmux list-clients -t %s -F '#{client_pid}|#{client_activity}|#{client_tty}' 2>/dev/null", sessQ))
           clients := parseTmuxListClients(out)

           // 第一行 greeting（绿色）
           greeting := fmt.Sprintf("✓ 已 attach 到会话 %s", sessionName)
           if !noColor {
               // 复用 colors.go::colorize；fdHolder 通过 io.Writer.(fdHolder) 类型断言，无 fd 时 colorEnabled 默认 false
               if fh, ok := w.(fdHolder); ok && colorEnabled(noColor, fh) {
                   greeting = colorize(greeting, ansiGreen, true)
               }
           }
           fmt.Fprintln(w, greeting)

           if len(clients) == 0 { return }

           // 批量查 hostname（D-12 完整方案 — Q1 RESOLVED）
           pids := make([]int, len(clients))
           for i, c := range clients { pids[i] = c.PID }
           hostnames := readClientHostnames(conn, pids)

           parts := make([]string, 0, len(clients))
           now := time.Now()
           for _, c := range clients {
               age := now.Sub(c.Activity)
               host := hostnames[c.PID]
               if host == "" { host = "unknown-host" }
               parts = append(parts, fmt.Sprintf("%s / %s", host, renderActivityAge(age)))
           }
           fmt.Fprintf(w, "  （另 %d 个会话正在共享：%s）\n", len(clients), strings.Join(parts, "，"))
       }
       ```

    3. **runClaudeWithSession**（D-28 / D-29 — 主入口，挂三 goroutine）：

       ```go
       func runClaudeWithSession(ctx context.Context, conn *ssh.Client, sshCfg SSHConfig,
           claudeArgs []string, sessionCfg SessionConfig, hasProxy bool) (int, error) {

           // session 命名（D-07 / D-08）
           sessionName := buildTmuxSessionName(sessionCfg.AccountID, sessionCfg.Cwd)
           if sessionCfg.ShortID != "" {
               sessionName = "claude-" + sessionCfg.ShortID
           }

           // --take-over（D-11）
           if sessionCfg.TakeOver {
               if err := performTakeOver(conn, sessionName); err != nil {
                   fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.SESSION_TAKEOVER_FAILED, err.Error()))
               }
           }

           // attach 前 banner（D-12 — 完整文件注册表方案）
           printAttachBanner(os.Stderr, conn, sessionName, sessionCfg.NoColor)

           // 写 last-session.json TmuxSession + ClientRole=primary
           writeLastSessionTmuxField(sessionName, "primary")

           claudeCmd := buildClaudeCmd(claudeArgs, hasProxy, sessionCfg.Cwd)
           remoteCmd := buildTmuxRemoteCmd(sessionCfg.Cwd, sessionName, claudeCmd)

           // attach 成功后写文件注册表 + defer 清理（写入失败仅 warning，不阻塞）
           // 注：必须在 PTY attach 成功后才能 tmux display-message — 因此延后到 runClaudePTYWithReconnect 内首次 attach 后立即调
           // 但本 plan 简化为：在 session.Start 之后启动 goroutine 异步执行（避免阻塞 PTY 主循环）
           // 完整实现见 runClaudePTYWithReconnect

           return runClaudePTYWithReconnect(ctx, conn, sshCfg, remoteCmd, sessionName, sessionCfg)
       }
       ```

    4. **runClaudePTYWithReconnect**（PTY 主循环 + RunKeepAlive + Reconnector + BufferedStdin 协同）：

       骨架（关键路径，PTY 部分一字复刻 ssh.go::runClaude line 168-241）：

       ```go
       func runClaudePTYWithReconnect(ctx context.Context, initialConn *ssh.Client, sshCfg SSHConfig,
           remoteCmd, sessionName string, sessionCfg SessionConfig) (int, error) {

           conn := initialConn
           reconnectCount := 0
           registryPid := 0
           defer func() {
               if registryPid > 0 {
                   _ = removeClientFile(conn, registryPid) // 异常退出兜底；正常退出在 wait 后清理
               }
           }()

           for {
               session, err := conn.NewSession()
               if err != nil { return 0, fmt.Errorf("创建 SSH 会话失败: %w", err) }

               fd := int(os.Stdin.Fd())
               isTTY := term.IsTerminal(fd)
               // PTY 申请 + raw mode + SIGWINCH — 复刻 runClaude line 178-209
               if isTTY {
                   /* MakeRaw + RequestPty("xterm-256color", h, w, modes) + SIGWINCH 处理 */
               }

               // BufferedStdin 注入（Q4 RESOLVED — listener 模式）
               var bs *BufferedStdin
               var stdinSource io.Reader = os.Stdin
               if isTTY && sessionCfg.ReconnectEnabled {
                   bs0, pipeR := NewBufferedStdin(os.Stdin, nil /* state 由 SetReconnector 注入 */, os.Stdout, sessionCfg.NoColor, nil)
                   bs = bs0
                   stdinSource = pipeR
                   go bs.Run(ctx)
               }
               session.Stdin = stdinSource
               session.Stdout = os.Stdout
               session.Stderr = os.Stderr

               // KeepAlive goroutine
               keepCtx, cancelKeep := context.WithCancel(ctx)
               go func() { _ = RunKeepAlive(keepCtx, conn, sessionCfg.KeepAliveInterval, sessionCfg.KeepAliveCountMax) }()

               if err := session.Start(remoteCmd); err != nil {
                   cancelKeep()
                   return 0, fmt.Errorf("启动 tmux 包装命令失败: %w", err)
               }

               // attach 已开始 — 异步写文件注册表（不阻塞 PTY）
               if registryPid == 0 {
                   go func() {
                       pid, werr := writeClientFile(conn, sessionName, sessionCfg.AccountID, sessionCfg.LocalHostname)
                       if werr != nil {
                           fmt.Fprintln(os.Stderr, "[!] writeClientFile 失败（banner hostname 将显示 unknown）:", werr)
                           return
                       }
                       registryPid = pid
                   }()
               }

               waitErr := session.Wait()
               cancelKeep()

               if waitErr == nil || isExitError(waitErr) {
                   // 正常退出 — 清理注册表 + 写 last-session.ReconnectCount
                   if registryPid > 0 { _ = removeClientFile(conn, registryPid); registryPid = 0 }
                   writeLastSessionReconnectCount(reconnectCount)
                   if exitErr, ok := waitErr.(*ssh.ExitError); ok { return exitErr.ExitStatus(), nil }
                   return 0, nil
               }

               // io.EOF / net.OpError → 启动 Reconnector
               if !sessionCfg.ReconnectEnabled || !isReconnectableError(waitErr) {
                   return 0, fmt.Errorf("SSH 会话异常结束: %w", waitErr)
               }

               var newConn *ssh.Client
               t0 := time.Now()
               reconnector := NewReconnector(sshCfg,
                   func() {/* onConnLost: BufferedStdin 通过 listener 自动切到 Reconnecting */},
                   func(c *ssh.Client) error { newConn = c; return nil },
                   os.Stderr, sessionCfg.NoColor)
               // Q4 RESOLVED — 注入式连接
               if bs != nil { bs.SetReconnector(reconnector) }

               if err := reconnector.Run(ctx); err != nil {
                   if errors.Is(err, ErrReconnectGaveUp) {
                       fmt.Fprintln(os.Stderr, FormatGiveUpMessage(5, time.Since(t0)))
                       writeLastSessionReconnectCount(reconnectCount)
                       return ExitNetworkError, nil
                   }
                   return 0, err
               }
               reconnectCount += reconnector.ReconnectCount()
               conn = newConn
               // flush BufferedStdin 缓存 → 新 session.Stdin
               if bs != nil { _ = bs.Flush() }
               // 循环回到 conn.NewSession() 重 attach 同一 tmux session（registryPid 沿用，cleanup 在 defer / 正常退出时）
           }
       }

       func isReconnectableError(err error) bool {
           if err == nil { return false }
           if errors.Is(err, io.EOF) { return true }
           // 其它非 ExitError 一律视为网络层错误
           if _, ok := err.(*ssh.ExitError); ok { return false }
           return true
       }

       func isExitError(err error) bool { _, ok := err.(*ssh.ExitError); return ok }
       ```

       > **执行器拍板**：上面是骨架；PTY 申请段（line 178-209 一字复刻）执行器从 ssh.go 直接复制粘贴。如 Plan 01 未暴露 `RegisterStateListener` / `SetReconnector`（Q4 RESOLVED 接口），执行器在 reconnect.go / input_buffer.go 顺手补 follow-up（< 30 行）；属于 Plan 01 接口补强，commit 信息标 `chore(32-01-followup)`。

    5. **RunSessionsLs / RunSessionsAttach**（D-13 / D-14）：

       ```go
       func RunSessionsLs(conn *ssh.Client, w io.Writer) error {
           out, err := sshOutput(conn,
               "tmux list-sessions -F '#{session_name}|#{session_created}|#{session_attached}|#{session_windows}' 2>/dev/null")
           if err != nil || strings.TrimSpace(out) == "" {
               fmt.Fprintln(w, "当前容器内无活跃 tmux session")
               return nil
           }
           tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
           fmt.Fprintln(tw, "SESSION\tCREATED\tCLIENTS\tWINDOWS")
           now := time.Now()
           for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
               fields := strings.SplitN(line, "|", 4)
               if len(fields) < 4 { continue }
               createdSec, _ := strconv.ParseInt(fields[1], 10, 64)
               age := renderActivityAge(now.Sub(time.Unix(createdSec, 0)))
               fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", fields[0], age, fields[2], fields[3])
           }
           return tw.Flush()
       }

       func RunSessionsAttach(conn *ssh.Client, sessionName string, hasProxy bool, cwd string) (int, error) {
           sessQ := shellescape.Quote(sessionName)
           if err := sshRun(conn, fmt.Sprintf("tmux has-session -t %s 2>/dev/null", sessQ)); err != nil {
               fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.SESSION_NOT_FOUND, sessionName))
               return ExitConfigError, fmt.Errorf("session not found: %s", sessionName)
           }
           remoteCmd := fmt.Sprintf("exec tmux attach-session -t %s", sessQ)
           return runClaudePTYBare(conn, remoteCmd) // 私有 helper：runClaude line 168-241 主体复制（无 reconnect / keepalive）
       }
       ```

       **runClaudePTYBare** 是 ssh.go::runClaude 主体的本地复制（约 60 行）— ssh.go::runClaude 保持原样不动。

    6. **last-session.json 写入 helpers**：

       ```go
       func writeLastSessionTmuxField(sessionName, role string) {
           snap, _ := LoadLastSession() // 现网 helper 名 — 执行器 grep last_session.go 取实际名
           snap.TmuxSession = sessionName
           snap.ClientRole = role
           _ = SaveLastSession(snap)
       }
       func writeLastSessionReconnectCount(count int) {
           snap, _ := LoadLastSession()
           snap.ReconnectCount = count
           _ = SaveLastSession(snap)
       }
       ```

       现网 last_session.go 的 helper 实际名以 grep 为准；不存在时 fallback `os.WriteFile + json.Marshal`。

    7. **session_test.go 追加 ≥ 4 个集成边界单测**（无网络，纯 mock / fake conn）：

       ```go
       func TestRunClaudePTYWithReconnect_ExitErrorReturnsCode(t *testing.T) {
           // mock conn.NewSession() 返回的 session.Wait() 给 *ssh.ExitError(7)
           // 期望 runClaudePTYWithReconnect 返回 (7, nil)，不进入 reconnect 循环
       }

       func TestPerformTakeOver_ZeroClientsReturnsNil(t *testing.T) {
           // mock sshOutput 返回 ""（list-clients 命中 0）
           // 期望 performTakeOver 返回 nil 不调 detach-client
       }

       func TestPrintAttachBanner_NoOtherClients_NoSecondLine(t *testing.T) {
           // mock sshOutput 返回 ""
           // 期望 banner 仅 1 行 ✓ 已 attach
       }

       func TestPrintAttachBanner_WithRegistryEntries_ShowsHostnames(t *testing.T) {
           // mock list-clients 返回 1 条 + readClientHostnames 返回 {pid: "alice-mbp"}
           // 期望 banner 第二行含 "alice-mbp / "
           // 注：readClientHostnames 也 mock — 通过 sshOutput hook 返回 "===<pid>===\n{hostname:alice-mbp,...}\n"
       }
       ```

       执行器若发现 fake conn 实现成本高（Plan 01 fakeConn 已存在，但不支持 sess.CombinedOutput / NewSession 完整 mock），退化为：
       - **`TestPerformTakeOver_*`** / **`TestPrintAttachBanner_*`** 改为表驱动**纯函数子测试**（把 list-clients 输出 string + readClientHostnames map 提取到独立函数 `decideTakeOver(out string) int` / `formatBannerSecondLine(clients []tmuxClient, hostnames map[int]string) string`），覆盖等价边界
       - 关键：banner 第二行格式 + take-over 0/N client 决策必须有断言

  </action>
  <acceptance_criteria>
    - `go build ./internal/cloudclaude/...` 成功
    - `go vet ./internal/cloudclaude/...` 通过
    - `go test ./internal/cloudclaude/... -run "TestRunClaudePTYWithReconnect|TestPerformTakeOver|TestPrintAttachBanner|TestBuildTmuxSessionName|TestSanitizeSessionName|TestGenerateShortSessionID|TestParseTmuxListClients|TestRenderActivityAge|TestBuildTmuxRemoteCmd|TestBuildClaudeCmd" -count=1` 全部 PASS（≥ 14 用例 — Task 2.1a 的 10 + 本 task 的 4）
    - `rg -n "func runClaudeWithSession\\(" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "func runClaudePTYWithReconnect" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "func runClaudePTYBare" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "func performTakeOver" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "func printAttachBanner" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "func RunSessionsLs" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "func RunSessionsAttach" internal/cloudclaude/session.go` 命中 1 行
    - `rg -n "RunKeepAlive" internal/cloudclaude/session.go` 命中（runClaudePTYWithReconnect 内 goroutine）
    - `rg -n "NewReconnector" internal/cloudclaude/session.go` 命中
    - `rg -n "NewBufferedStdin" internal/cloudclaude/session.go` 命中
    - `rg -n "ErrReconnectGaveUp" internal/cloudclaude/session.go` 命中（兜底分支）
    - `rg -n "writeClientFile\\(" internal/cloudclaude/session.go` 命中 ≥ 2 处（定义 + runClaudeWithSession 内调用）
    - `rg -n "removeClientFile\\(" internal/cloudclaude/session.go` 命中 ≥ 2 处（定义 + defer 清理）
    - `rg -n "readClientHostnames\\(" internal/cloudclaude/session.go` 命中 ≥ 2 处（定义 + printAttachBanner 内调用）
    - `rg -n "writeLastSessionTmuxField\\(" internal/cloudclaude/session.go` 命中 ≥ 2 处（定义 + runClaudeWithSession 内调用）
    - `rg -n "primary" internal/cloudclaude/session.go` 命中（writeLastSessionTmuxField 调用时第二参数 = "primary"）
  </acceptance_criteria>
  <verify>
    <automated>go test ./internal/cloudclaude/... -run "TestRunClaudePTYWithReconnect|TestPerformTakeOver|TestPrintAttachBanner|TestBuildTmuxSessionName|TestSanitizeSessionName|TestGenerateShortSessionID|TestParseTmuxListClients|TestRenderActivityAge|TestBuildTmuxRemoteCmd|TestBuildClaudeCmd" -count=1 &amp;&amp; go vet ./internal/cloudclaude/... &amp;&amp; rg -q "func runClaudeWithSession" internal/cloudclaude/session.go &amp;&amp; rg -q "func performTakeOver" internal/cloudclaude/session.go &amp;&amp; rg -q "func printAttachBanner" internal/cloudclaude/session.go</automated>
  </verify>
  <done>session.go 高层业务全部就位；runClaudeWithSession 协调 RunKeepAlive / Reconnector / BufferedStdin 三 goroutine；文件注册表写入 / 清理 / 读取链路完整；4+ 集成边界单测 PASS。Task 2.2 / 2.3 在其上接入。</done>
</task>

<task type="auto">
  <name>Task 2.2: ssh.go ConnectAndRunClaudeV3 路由 + MountConfig 追加 SessionShortID/SessionTakeOver/LocalHostname</name>
  <files>
    internal/cloudclaude/ssh.go
    internal/cloudclaude/mount_strategy.go
  </files>
  <read_first>
    - internal/cloudclaude/ssh.go（ConnectAndRunClaudeV3 全函数 — 找到 OAuth 检查后、return runClaude 前的位置 line 130-141）
    - internal/cloudclaude/mount_strategy.go（MountConfig struct 定义位置 — 在末尾追加 3 字段）
    - internal/cloudclaude/session.go（Task 2.1 暴露的 SessionConfig + DetectTmux + runClaudeWithSession + GenerateShortSessionID）
    - .planning/phases/32-ssh-tmux/32-CONTEXT.md D-15 / D-16 / D-28 / D-29
    - .planning/phases/32-ssh-tmux/32-PATTERNS.md ssh.go 改造点 B
  </read_first>
  <action>
    1. **internal/cloudclaude/mount_strategy.go**：在 MountConfig struct 末尾追加 3 个字段（保持现有字段顺序与 JSON tag 不动）：

       ```go
       type MountConfig struct {
           // ... 现有字段（全部不动）

           // [Phase 32 新增] cobra --new-session / --take-over flag 透传
           SessionShortID  string  // --new-session 时 cmd 层生成的 8 字符 base64url；空 = 默认 session 命名
           SessionTakeOver bool    // --take-over flag
           LocalHostname   string  // os.Hostname()，session.go banner 文件注册表用
       }
       ```

       **不**给这 3 个字段加 JSON tag — MountConfig 不参与 JSON 序列化（仅运行时配置）。
       **不**修改 KeepAliveInterval / KeepAliveCountMax / SyncSessionLock / ClaudeAccountID 等 Phase 31 字段。

    2. **internal/cloudclaude/ssh.go**：在 ConnectAndRunClaudeV3 函数末尾（line 139-141 区间，OAuth 检查之后、`return runClaude(...)` 之前）插入 SessionConfig 构造 + DetectTmux + 路由：

       原 line 139-141：

       ```go
           }
       }

       return runClaude(connA, claudeArgs, cwd, len(proxyCommands) > 0)
       ```

       改为：

       ```go
           }
       }

       // [Phase 32 D-15 / D-28 / D-29] tmux 探测 + 路由
       sessionCfg := SessionConfig{
           AccountID:         mountCfg.ClaudeAccountID,
           ShortID:           mountCfg.SessionShortID,
           TakeOver:          mountCfg.SessionTakeOver,
           KeepAliveInterval: mountCfg.KeepAliveInterval,
           KeepAliveCountMax: mountCfg.KeepAliveCountMax,
           ReconnectEnabled:  true,
           NoColor:           mountCfg.NoColor,
           Cwd:               cwd,
           LocalHostname:     mountCfg.LocalHostname,
       }
       available, _, reason := DetectTmux(connA)
       sessionCfg.TmuxAvailable = available
       if !available {
           fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.SESSION_TMUX_UNAVAILABLE, reason))
           return runClaude(connA, claudeArgs, cwd, len(proxyCommands) > 0)
       }
       return runClaudeWithSession(cmd.Context(), connA, cfg, claudeArgs, sessionCfg, len(proxyCommands) > 0)
       ```

       **注 1**：`cmd.Context()` 这里 — ConnectAndRunClaudeV3 当前签名是 `(cfg SSHConfig, ...)` 不接 ctx；执行器需 grep 该函数当前实际签名。如已有 ctx 参数（Phase 31 应已加）则用；否则用 `context.Background()`（reconnect 永远不 cancel — 不理想但与 v2.0 行为兼容）。**优先**：从已有 cmd.Context 提取 — 函数签名应在 Phase 31 已是 `ConnectAndRunClaudeV3(ctx context.Context, cfg SSHConfig, ...)`。

       **注 2**：`mountCfg.Logger` — Phase 31 已有该字段；如不存在则改用 `os.Stderr`。

       **注 3**：`runClaude` / `runClaudeWithSession` 的 cfg 参数 — 当前 runClaude 签名 `(conn, claudeArgs, cwd, hasProxy)` 不接 SSHConfig；runClaudeWithSession 需要 SSHConfig 用于 reconnect 重新拨号。这里传入 ConnectAndRunClaudeV3 已有的 `cfg SSHConfig` 参数。

    3. **不**修改 sshConnect（Plan 01 已改）；**不**修改 ConnectAndRunClaudeV3 的 OAuth 检查段（Phase 31 落地）；**不**新增 endpoint 或 host-agent 调用（OOS-A20 守恒）。

    4. **不**修改 runClaude — 保留作 fallback 与 sessions attach 的 PTY 蓝本。
  </action>
  <acceptance_criteria>
    - `go build ./...` 成功
    - `go vet ./internal/cloudclaude/... ./cmd/cloud-claude/...` 通过
    - `rg -n "DetectTmux" internal/cloudclaude/ssh.go` 命中 1 行（在 ConnectAndRunClaudeV3 内）
    - `rg -n "runClaudeWithSession" internal/cloudclaude/ssh.go` 命中 1 行（路由 call site）
    - `rg -n "SESSION_TMUX_UNAVAILABLE" internal/cloudclaude/ssh.go` 命中 1 行（fallback banner）
    - `rg -n "SessionShortID" internal/cloudclaude/mount_strategy.go` 命中 1 行
    - `rg -n "SessionTakeOver" internal/cloudclaude/mount_strategy.go` 命中 1 行
    - `rg -n "LocalHostname" internal/cloudclaude/mount_strategy.go` 命中 1 行
    - `git diff internal/cloudclaude/ssh.go` 中 runClaude 函数体（line 168-241）应 zero diff（用 `git diff -U0 internal/cloudclaude/ssh.go | grep '^+' | grep -v '^+++' | wc -l` 复核：新增行数仅在 ConnectAndRunClaudeV3 内、约 20 行）
    - `go test ./internal/cloudclaude/... -count=1 -short` 现有所有非集成测试 PASS（不破坏 Phase 31 单测）
  </acceptance_criteria>
  <verify>
    <automated>go build ./... &amp;&amp; go vet ./internal/cloudclaude/... ./cmd/cloud-claude/... &amp;&amp; go test ./internal/cloudclaude/... -count=1 -short</automated>
  </verify>
  <done>ConnectAndRunClaudeV3 在 OAuth 检查后正确路由到 runClaudeWithSession（tmux 可用）或 runClaude（不可用 fallback）；MountConfig 已拓展 3 字段供 cmd 层注入；Phase 31 单测全 PASS。</done>
</task>

<task type="auto">
  <name>Task 2.3: cmd/cloud-claude/sessions.go cobra 子命令 + main.go 注册 + flag 剥离 + KeepAlive 启动校验</name>
  <files>
    cmd/cloud-claude/sessions.go
    cmd/cloud-claude/main.go
  </files>
  <read_first>
    - cmd/cloud-claude/sync.go（newSyncCmd line 18-37 — sessions.go 严格 mirror）
    - cmd/cloud-claude/main.go（line 92 AddCommand / line 96-101 DisableFlagParsing / line 244-269 runRoot 的 --mount-mode 剥离 / line 335 mountCfg 构造）
    - cmd/cloud-claude/main.go（runEnvCheck line 163-201 — sessions ls/attach 同 LoadConfig + AuthenticateAndWait + sshConnect 模板）
    - internal/cloudclaude/session.go（Task 2.1 暴露的 RunSessionsLs / RunSessionsAttach / GenerateShortSessionID / SessionConfig）
    - internal/cloudclaude/errcodes/codes.go（SESSION_KEEPALIVE_TOO_AGGRESSIVE）
    - .planning/phases/32-ssh-tmux/32-CONTEXT.md D-13 / D-14 / D-03 第 4 条 / D-29
    - .planning/phases/32-ssh-tmux/32-PATTERNS.md cmd/cloud-claude/sessions.go 段 + main.go 4 处改造
  </read_first>
  <action>
    1. **cmd/cloud-claude/sessions.go**（新文件，mirror sync.go 结构）：

       ```go
       package main

       import (
           "context"
           "fmt"
           "os"

           "github.com/spf13/cobra"

           "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
       )

       func newSessionsCmd() *cobra.Command {
           cmd := &cobra.Command{
               Use:           "sessions",
               Short:         "tmux 会话管理（v3.0 SSH 会话可靠性）",
               Long:          "查看 / attach 容器内由 cloud-claude 创建的 tmux 会话；零控制面改造，纯客户端逻辑。",
               SilenceUsage:  true,
               SilenceErrors: true,
           }
           lsCmd := &cobra.Command{
               Use: "ls", Short: "列出当前 tmux 会话",
               RunE: runSessionsLs,
               SilenceUsage: true, SilenceErrors: true,
           }
           attachCmd := &cobra.Command{
               Use: "attach <name>", Short: "attach 到指定 tmux 会话",
               Args: cobra.ExactArgs(1),
               RunE: runSessionsAttach,
               SilenceUsage: true, SilenceErrors: true,
           }
           cmd.AddCommand(lsCmd, attachCmd)
           return cmd
       }

       func runSessionsLs(cmd *cobra.Command, args []string) error {
           conn, _, err := connectForSessions(cmd.Context())
           if err != nil { return err }
           defer conn.Close()
           return cloudclaude.RunSessionsLs(conn, os.Stdout)
       }

       func runSessionsAttach(cmd *cobra.Command, args []string) error {
           conn, authResp, err := connectForSessions(cmd.Context())
           if err != nil { return err }
           defer conn.Close()
           // hasProxy=false 因为 attach 不跑 claude — 直接 attach
           code, err := cloudclaude.RunSessionsAttach(conn, args[0], false, "/workspace")
           if err != nil {
               fmt.Fprintln(os.Stderr, err)
               os.Exit(code)
           }
           if code != 0 { os.Exit(code) }
           _ = authResp
           return nil
       }

       // connectForSessions 复用 runEnvCheck 模板：LoadConfig → AuthenticateAndWait → sshConnect。
       func connectForSessions(ctx context.Context) (*ssh.Client, *cloudclaude.AuthResponse, error) {
           cfg, err := cloudclaude.LoadConfig()
           if err != nil { return nil, nil, err }
           client := cloudclaude.NewEntryClient(cfg.Gateway)
           authResp, err := client.AuthenticateAndWait(ctx, cfg.ShortID, cfg.Password, func(string) {})
           if err != nil { return nil, nil, fmt.Errorf("认证失败: %w", err) }
           sshCfg := cloudclaude.SSHConfig{
               Host: authResp.SSHHost, Port: authResp.SSHPort,
               User: authResp.SSHUser, Password: authResp.SSHPass,
           }
           // sshConnect 是 internal/cloudclaude unexported；需暴露 SSHConnect 或用现有 ConnectAndRunClaudeV3 的 sshConnect 同等入口
           // 现有 cmd 层调用 sshConnect 的方式：通过 cloudclaude.RunEnvCheck / cloudclaude.RunSSHDoctor 内部调
           // 本 task 直接复用：在 internal/cloudclaude 暴露 SSHConnect(SSHConfig) (*ssh.Client, error) — 极简包装
           conn, err := cloudclaude.SSHConnect(sshCfg)
           if err != nil { return nil, nil, err }
           return conn, authResp, nil
       }
       ```

       > **planner 注**：`cloudclaude.SSHConnect` 当前可能不导出（小写 sshConnect）；执行器在 ssh.go 末尾追加：
       > ```go
       > // SSHConnect 暴露给 cmd 层 sessions 子命令使用（Phase 32）。仅是 sshConnect 的 export 包装。
       > func SSHConnect(cfg SSHConfig) (*ssh.Client, error) { return sshConnect(cfg) }
       > ```
       > 这是本 task 在 ssh.go 的**唯一新增** — 与 Task 2.2 改动不冲突（ssh.go 末尾追加 vs ConnectAndRunClaudeV3 内插）。

    2. **cmd/cloud-claude/main.go** 4 处改动（按 PATTERNS §main.go 改造段）：

       **A. line 92 AddCommand 追加 newSessionsCmd()**：

       ```go
       rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd())
       ```

       **B. line 96-101 DisableFlagParsing switch 追加 "sessions"**：

       ```go
       if len(os.Args) > 1 {
           switch os.Args[1] {
           case "init", "env", "ssh", "sync", "sessions", "help", "--help", "-h":
               rootCmd.DisableFlagParsing = false
           }
       }
       ```

       **C. runRoot 内 --mount-mode 循环改为同时剥离 --new-session / --take-over**（line 244-269 区间）：

       把现有 for 循环改为下方模板（保持 `mountMode` 既有逻辑不变）：

       ```go
       mountMode := "auto"
       newSession := false
       takeOver := false
       filtered := args[:0]
       for i := 0; i < len(args); i++ {
           switch {
           case args[i] == "--mount-mode" && i+1 < len(args):
               mountMode = args[i+1]; i++; continue
           case strings.HasPrefix(args[i], "--mount-mode="):
               mountMode = strings.TrimPrefix(args[i], "--mount-mode="); continue
           case args[i] == "--new-session":
               newSession = true; continue
           case args[i] == "--take-over":
               takeOver = true; continue
           }
           filtered = append(filtered, args[i])
       }
       args = filtered
       ```

       **D. mountCfg 构造后注入 + KeepAlive 启动校验**（line 335 附近 — 找到 `mountCfg := cloudclaude.MountConfig{...}` 块后）：

       ```go
       mountCfg.SessionTakeOver = takeOver
       if newSession {
           mountCfg.SessionShortID = cloudclaude.GenerateShortSessionID()
       }
       if hostname, _ := os.Hostname(); hostname != "" {
           mountCfg.LocalHostname = hostname
       }

       // [Phase 32 D-03 第 4 条] keepalive_interval 启动校验 — REQ-F3-A / PITFALLS M11
       if mountCfg.KeepAliveInterval > 0 && mountCfg.KeepAliveInterval < 15*time.Second {
           fmt.Fprintln(os.Stderr,
               errcodes.Format(errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE, mountCfg.KeepAliveInterval.String()))
           os.Exit(exitConfigError)
       }
       ```

       注：`errcodes` 在 main.go 应已 import（Phase 31 已用）；如未则补上 `"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"` 与 `"time"`。

    3. **不**新增 PersistentFlags 注册 --new-session / --take-over（DisableFlagParsing=true 模式下手动剥离即可；与 --mount-mode 注册同模式 — line 86-90）。如希望 --help 显示，可选追加 `rootCmd.PersistentFlags().Bool("new-session", false, "...")` 与 `Bool("take-over", false, "...")`，但**不强制**。

    4. **不**改 runRoot 后续逻辑（cobra 解析后调 ConnectAndRunClaudeV3 — Task 2.2 已改造）。
  </action>
  <acceptance_criteria>
    - `go build ./...` 成功
    - `go vet ./...` 通过
    - `rg -n "newSessionsCmd" cmd/cloud-claude/main.go` 命中 1 行（AddCommand 调用）
    - `rg -n "func newSessionsCmd" cmd/cloud-claude/sessions.go` 命中 1 行
    - `rg -n "RunSessionsLs|RunSessionsAttach" cmd/cloud-claude/sessions.go` 各命中 1 行
    - `rg -n "\"sessions\"" cmd/cloud-claude/main.go` 命中（DisableFlagParsing switch case）
    - `rg -n -- "--new-session" cmd/cloud-claude/main.go` 命中（剥离逻辑）
    - `rg -n -- "--take-over" cmd/cloud-claude/main.go` 命中
    - `rg -n "SESSION_KEEPALIVE_TOO_AGGRESSIVE" cmd/cloud-claude/main.go` 命中（启动校验）
    - `rg -n "GenerateShortSessionID" cmd/cloud-claude/main.go` 命中（newSession 触发）
    - `rg -n "func SSHConnect" internal/cloudclaude/ssh.go` 命中 1 行（export 包装）
    - `./cloud-claude --help` 输出含 `sessions` 子命令（手测；CI 可用 `go run ./cmd/cloud-claude --help 2>&1 | grep sessions` 验证）
    - `./cloud-claude sessions --help` 输出含 `ls` / `attach` 两个子命令
    - `./cloud-claude sessions ls 2>&1` 在无 config 时优雅失败（"配置文件未找到" 等中文错误，**不**panic / **不**段错误）
  </acceptance_criteria>
  <verify>
    <automated>go build ./... &amp;&amp; go vet ./... &amp;&amp; go run ./cmd/cloud-claude --help 2>&amp;1 | grep -q sessions &amp;&amp; go run ./cmd/cloud-claude sessions --help 2>&amp;1 | grep -q "attach"</automated>
  </verify>
  <done>cobra `cloud-claude sessions ls` / `attach <name>` 子命令路由打通；--new-session / --take-over flag 正确剥离不透传给远端 claude；KeepAlive 启动校验已注入；--help 树正确显示 sessions 节点；现有 Phase 31 集成测试不破坏。</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| 客户端 → 容器内 tmux server | 远程 SSH 命令拼接 tmux new-session / list-clients / detach-client / attach-session — sessionName 来自客户端构造（accountID hash / cwd hash / base64url short_id），不接收外部输入 |
| --new-session / --take-over flag | 来自命令行参数；不写入磁盘、不通过网络回传 |
| 第二端 banner 文本 | tmux list-clients 远端原始输出 → 本地 stderr；包含 unix timestamp / TTY 路径（无敏感信息） |
| sessions attach <name> 参数 | 用户提供的 session 名 → shellescape 后远程 tmux has-session 校验；不存在直接拒绝 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-32-08 | Tampering | session.go remoteCmd 拼接（D-10 / D-11 / sessions ls/attach） | mitigate | 全部参数走 shellescape.Quote / QuoteCommand（SP-03）；session_test.go 用例 TestBuildTmuxRemoteCmd_ContainsAllParts 校验模板组成；禁止手写 '...' / "..." 引号（执行器若发现 grep -n "\\$" session.go 命中变量插值无 shellescape，task 失败） |
| T-32-09 | Information Disclosure | sessions ls 输出 SESSION 名（含 account_id hash） | accept | session 名格式 `claude-<8字符>`，account_id 已被 hash 截断（前 8 字符）；不泄露完整 UUID；与 Phase 31 mutagen session 命名同等级 |
| T-32-10 | DoS | --take-over 反复触发 detach-client | mitigate | take-over 是用户显式 flag；每次启动只触发一次；3s 通知期 + detach-client -a 是 tmux 内置原子操作；不会无限循环 |
| T-32-11 | Privilege Escalation | sessions attach 进入既有 session 等于继承前任 PTY | accept | tmux 多端默认共享是产品决策（OOS-A9）；sessions attach 进入 session 等价于 v2.0 直接 ssh 进容器 + tmux attach — 不引入新攻击面 |
| T-32-12 | Spoofing | 第二端 banner 显示"未知来源"被用户误以为是另一端 | accept | v0 已知限制（D-12 修订接受退化）；用户能从 sessions ls 的 CLIENTS 列得到客户端数实数；v3.1 文件注册表写入完整方案 |
| T-32-13 | Tampering | --take-over 触发 detach-client -a 误踢自己端 | mitigate | RESEARCH §5.3 已 verified — caller 是临时 SSH client（不在 list-clients 内），-a 只踢已 attached client；本端 attach 是 detach-client 命令之后单独执行，时序保证 |
| T-32-20 | DoS | conn-A 频率上限（多端 attach + take-over + reconnect 串行 SSH 拨号） | accept | Phase 29 D-14 sshd_config `MaxSessions 30` / `MaxStartups 60:30:120` 已就位（容器侧硬上限，单 host 任意 cloud-claude 端数总和 ≤ 30）+ Plan 01 reconnect 退避序列 1/2/4/8/30s 限制同 host 并发拨号节奏；M12 实质防御已在镜像层落地，本 plan 不引入新攻击面 |

**Severity 分布（block_on=high）**：
- High: 0 项
- Medium: T-32-08（关键 — shellescape 漏写即 RCE 面，已通过 SP-03 + 自动化 grep 检查防御）/ T-32-10 / T-32-13
- Low: T-32-09 / T-32-11 / T-32-12 / T-32-20（M12 防御已在 Phase 29 镜像层落地）

无 High 项 → 不阻断 plan 执行。
</threat_model>

<verification>
本 plan 完成时，可断言下列 ROADMAP §Phase 32 Success Criteria（其余 Plan 03 验收）：

1. **SC2（REQ-F4-A 30s 抖动 tmux 不丢进程）**：手动 / 集成测试（Plan 03 集成测试套件中验收，本 plan 提供必要 service）— `docker network disconnect <ctr> <net> ; sleep 30 ; docker network connect <ctr> <net>` 后 cloud-claude reconnect 成功 + tmux session 内 claude 进程仍在 + buffer 完整
2. **SC7（REQ-F4-C tmux 不可用降级）**：DetectTmux 返回 false 时 stderr 输出 [SESSION_TMUX_UNAVAILABLE]；cloud-claude **不退出非 0**，仍能进入 claude（fallback 走 v2.0 runClaude）
3. **SC8（REQ-F4-B sessions ls/attach）**：`go run ./cmd/cloud-claude sessions --help` 输出含 ls / attach 两个子命令；运行时 `tmux list-sessions` 失败（无 server）→ 输出"当前容器内无活跃 tmux session"，退出 0；`sessions attach nonexistent` → stderr [SESSION_NOT_FOUND] + 退出 ExitConfigError(=4)
4. **SC9（REQ-F5-A / B 多端默认共享 attach + banner）**：runClaudeWithSession 内远程命令含 `tmux new-session -A`（grep 验证）；attach 前调 list-clients 渲染 banner；当容器内已有 client 时 banner 输出 `（另 N 个会话正在共享：...）`
5. **SC10（REQ-F5-C --new-session / --take-over）**：cobra 剥离 + GenerateShortSessionID 生成 8 字符 base64url + performTakeOver 实现 detach-client -a 序列；冲突场景输出 [SESSION_TAKEOVER_NOTIFIED]
6. **SC1（REQ-F3-A）补强**：main.go 启动期校验 KeepAliveInterval < 15s → [SESSION_KEEPALIVE_TOO_AGGRESSIVE] + os.Exit(4)（Plan 01 已暴露错误码；本 plan 在 cmd 层接入）

**全 plan 综合 verify 命令**：

```bash
go build ./... \
  && go vet ./... \
  && go test ./internal/cloudclaude/... -run "TestBuildTmuxSessionName|TestSanitizeSessionName|TestGenerateShortSessionID|TestParseTmuxListClients|TestRenderActivityAge|TestBuildTmuxRemoteCmd|TestBuildClaudeCmd" -count=1 \
  && go test ./internal/cloudclaude/... -count=1 -short \
  && go run ./cmd/cloud-claude --help 2>&1 | grep -q sessions \
  && go run ./cmd/cloud-claude sessions --help 2>&1 | grep -E "^\s+(ls|attach)"
```
</verification>

<success_criteria>
- session.go 全部业务逻辑就位（≈ 400-500 LOC，含 docstring）
- session_test.go 单测 ≥ 10 个 PASS（纯函数为主：命名 / sanitize / 解析 / 时间渲染 / 模板组装）
- ssh.go ConnectAndRunClaudeV3 改动局限在 OAuth 检查后段；runClaude 函数体 zero diff（保留作 fallback / sessions attach 复用）
- mount_strategy.go MountConfig 仅追加 3 字段（SessionShortID / SessionTakeOver / LocalHostname），现有字段 zero diff
- cmd/cloud-claude/sessions.go 全新文件，mirror sync.go 结构
- main.go 4 处改动均小且独立（AddCommand / switch / 剥离循环 / mountCfg 注入与 KeepAlive 校验）
- Phase 31 既有单测全 PASS（不破坏 mount_*_test.go / oauth_check_test.go / errcodes/codes_test.go / last_session_test.go 等）
- `go run ./cmd/cloud-claude sessions --help` 输出有效；ls / attach 子命令可见
- 无新依赖加入 go.mod
- shellescape 在 session.go 中至少使用 ≥ 5 处（grep 验证 — 任何手写引号即 fail）
</success_criteria>

<output>
After completion, create `.planning/phases/32-ssh-tmux/32-02-SUMMARY.md` 描述：
- session.go 关键导出 API 实际签名（与 plan <interfaces> 块对照差异）
- runClaudeWithSession 内 RunKeepAlive / Reconnector / BufferedStdin 三个 goroutine 的协同时序图（文字版本）
- runClaude 与 runClaudeWithSession 的代码复用度（runClaudePTYBare 抽取了多少行）
- cmd/cloud-claude/sessions.go 与 sync.go 的结构对照
- KeepAlive 启动校验落地位置 + 默认值 (15s) 验证
- Phase 31 单测全 PASS 证据（go test 输出尾部 OK 行）
- 留给 Plan 03 的接口：MountConfig.SyncSessionLock 注入位置（ConnectAndRunClaudeV3 在 MountWorkspace 之前覆盖该字段）已经预留
</output>
