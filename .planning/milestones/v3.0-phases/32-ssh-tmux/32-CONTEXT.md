# Phase 32: SSH 会话可靠性 + tmux 包装 + 多端 - Context

**Gathered:** 2026-04-20
**Status:** Ready for planning
**Mode:** `--auto`（所有 gray area 自动选定推荐项，决策来源标注于 `<decisions>` 各条尾注）

<domain>
## Phase Boundary

把 claudedock 的 SSH 会话从「v2.0 断线即终止」升级为：
**30s 抖动客户端无感知 + 容器内进程不丢 + 多端默认共享 attach**。

本阶段交付：

1. 新增 `internal/claudedock/session.go`：tmux `has-session ? attach : new` 决策、`--new-session` / `--take-over` 处理、多端 banner 渲染、容器内 tmux 不可用降级（不阻塞 SSH 启动 + banner 提示）
2. 改造 `internal/claudedock/ssh.go:runClaude`：远程命令由 `claude <args>` 包成 `exec tmux new-session -A -s <session_name> -- claude <args>`；session 命名遵循 D-09（per-claude_account）
3. SSH 客户端 KeepAlive：在 `sshConnect` 上叠加 `ServerAliveInterval=15s` / `ServerAliveCountMax=4` 等价行为（go.crypto/ssh 用 `SendRequest("keepalive@openssh.com")` 周期 + count 触发 fail），并对配置项做 `< 15s` 启动期校验（REQ-F3-A / PITFALLS M11）
4. TCP 层 KeepAlive：在 `sshConnect` 拨号成功后，对底层 `*net.TCPConn` 启用 `SO_KEEPALIVE=1`；Linux 走 `setsockopt(TCP_USER_TIMEOUT=30000)`、macOS 走 `setsockopt(TCP_KEEPALIVE=15)`（D-04 / RESEARCH §SSH 弱网研究）
5. 自实现重连状态机：`internal/claudedock/reconnect.go`，退避序列 `1s → 2s → 4s → 8s → 30s 上限`，不重新弹密码（复用启动期已缓存的 `SSHConfig.Password`），三态 UX 阈值渲染 `>1.5s 灰色 …` / `>8s 黄色 网络抖动中（N 秒未响应）` / `>30s 红色 网络已断 N 秒，正在自动重试…`（REQ-F3-D / REQ-F3-C）
6. 本地输入缓冲：`internal/claudedock/input_buffer.go`，把 `os.Stdin` 包成「灰色未确认」FIFO，断网期间渲染 ANSI 灰色字符到本地 stdout，重连成功后按序提交（REQ-F3-B）
7. 多端共享 attach：默认 `tmux new-session -A -s <name>`（已存在则 attach 不踢），第二端 banner 中文显示其它 client 来源 + 活跃时间（REQ-F5-A / REQ-F5-B）
8. `--new-session` 创建独立 session 命名 `claude-<short_id>`（cobra flag，base64-url(rand:6)）；`--take-over` 强制独占并通知其它端，冲突时返回明确中文提示（REQ-F5-C）
9. 账号级 Mutagen 单例锁：容器侧 flock `/var/lock/claudedock/sync-<account_id>.lock`（远端 SSH `flock -n -E 99` 命令包装），后连端只 attach tmux + 启 sshfs 观察文件，**不**参与文件同步（REQ-F5-D / PITFALLS M15）
10. `claudedock sessions ls` / `claudedock sessions attach <name>` 子命令：通过远程 `tmux list-sessions -F` 解析，零控制面改造（REQ-F4-B / OOS-A20 边界守恒）
11. 容器内 tmux 不可用降级：claudedock 启动早期（mount ready 后）远程 `command -v tmux && tmux start-server`，失败即把 `runClaude` 退化到 v2.0 裸 `claude <args>` 路径并打印 banner `[!] 容器内 tmux 不可用，会话恢复已禁用`（REQ-F4-C）
12. 错误码扩展：`internal/claudedock/errcodes/session.go` + `errcodes/net.go` 新增 `SESSION_*` / `NET_*` ≥ 8 条（D-15）
13. 防御 PITFALLS C3（sshfs 抖动级联——本阶段验收 Phase 31 的 watcher 在 30s 抖动场景下仍工作，**不**重写 watcher）+ C7（systemd-logind——本阶段验收 Phase 29 已禁 systemd 后 `pkill -SIGHUP sshd` 不杀 tmux）

**不在本阶段交付**：

- 容器侧 sshd `ClientAliveInterval` 调整 → 已在 Phase 29 D-14 落地，本阶段只校验
- Docker named volume 创建 / admin DELETE 联动 / entrypoint symlink → Phase 33
- `claudedock doctor` 五维度实现 / `claudedock explain <code>` → Phase 34（消费本阶段注册的错误码）
- 真机 30s/2min 弱网 UAT 与 BASE-03 验收 → Phase 35
- Mutagen daemon GC、sessions session 锁的跨主机协调 → v3.1
- "下次回车前 prompt 上方插入" 严格 PTY 拦截渲染（沿用 Phase 31 D-28 的近似实现：banner 后立即输出）

</domain>

<decisions>
## Implementation Decisions

### 文件结构

- **D-01**：本阶段文件按职责拆分，避免膨胀 `ssh.go`：
  - `internal/claudedock/session.go` — tmux session 决策（has-session 探测 / new vs attach / `--new-session` / `--take-over`）、session 命名、`claudedock sessions ls/attach` 子命令实现
  - `internal/claudedock/reconnect.go` — 重连状态机 + 退避计时器 + 三态 UX 阈值渲染
  - `internal/claudedock/input_buffer.go` — 断网期间 stdin 本地缓冲 + 灰色未确认渲染
  - `internal/claudedock/keepalive.go` — SSH 层周期心跳 goroutine（`SendRequest("keepalive@openssh.com")`） + 失败计数 + TCP `SO_KEEPALIVE` / `TCP_USER_TIMEOUT` / `TCP_KEEPALIVE` 平台分发
  - `internal/claudedock/sync_lock.go` — 账号级 Mutagen 单例锁的 SSH `flock -n -E 99` 包装
  - `internal/claudedock/errcodes/session.go` — `SESSION_*` 错误码注册
  - `internal/claudedock/errcodes/net.go` — 追加 `NET_*` 重连相关码（与 Phase 31 OAuth 码同包）
  - `cmd/claudedock/sessions.go` — `claudedock sessions ls / attach` cobra 子命令
  - `cmd/claudedock/main.go` — 注册 `--new-session` / `--take-over` flag、`sessions` 子命令
- **D-02**：本阶段**不**对 `mount_strategy.go` / `mount_*.go` 做结构改动；通过 `MountConfig.SyncSessionLock` 注入 hook（Phase 31 D-31 已预留接口），将 D-12 的 flock 包装注入。`MountConfig.KeepAliveInterval` / `KeepAliveCountMax` 字段（Phase 31 D-31 预留）由本阶段在 `cmd/claudedock/main.go` 写入并向下传给 `sshConnect`（D-03）。

### SSH KeepAlive + 重连机制选型

- **D-03**：客户端 KeepAlive 采用 **(a) 在 `golang.org/x/crypto/ssh` 上自实现**，**不**切换到 OpenSSH 子进程 + ControlMaster：
  1. 复用 v2.0 既有 `sshConnect` 工厂函数与 `ssh.NewClient` 流程，零依赖变化
  2. 启动 `keepalive.Run(ctx, conn, interval, countMax)` goroutine：每 `interval` 调一次 `conn.SendRequest("keepalive@openssh.com", true, nil)`，连续 `countMax` 次失败（超时返回 error）即关闭 `conn`（让上层重连状态机感知 io.EOF / `*ssh.ExitError`）
  3. 默认 `interval = 15s` / `countMax = 4`（来自 `MountConfig.KeepAliveInterval` / `KeepAliveCountMax`，Phase 31 D-31 已预留）
  4. CLI 启动期 `cmd/claudedock/main.go` 校验 `keepAliveInterval >= 15 * time.Second`，否则 `os.Exit(ExitConfigError)` + 中文错误码 `SESSION_KEEPALIVE_TOO_AGGRESSIVE`（防御 REQ-F3-A / PITFALLS M11）  
  *[auto] 推荐项：自实现（替代 OpenSSH 子进程：(a) v2.0 已用 `golang.org/x/crypto/ssh`，切换会破坏既有 ExecProxy / sftp 复用；(b) ControlMaster 在 macOS 与 Windows 上行为不一致；(c) 自实现使本阶段 input buffer / take-over 等能直接操作 `*ssh.Client`，无需进程间通信）*
- **D-04**：TCP 层 KeepAlive 在 `sshConnect` 拨号成功后立即配置：
  1. `tcpConn.SetKeepAlive(true)` + `tcpConn.SetKeepAlivePeriod(15 * time.Second)`（跨平台基础）
  2. Linux 平台 build tag（`//go:build linux`）：`syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, 18 /* TCP_USER_TIMEOUT */, 30000)` — 30s 内 ACK 不到关闭连接（防御 NAT idle drop）
  3. macOS 平台 build tag（`//go:build darwin`）：`syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, 0x10 /* TCP_KEEPALIVE */, 15)` — 与 Linux `tcp_keepalive_time` 等价
  4. 配置失败仅 `log.Warn`，不阻塞连接建立（best-effort 加固）
- **D-05**：重连状态机 `reconnect.Run(ctx, sshCfg, onConnLost, onReconnected) error`：
  1. 退避序列硬编码 `[]time.Duration{1*s, 2*s, 4*s, 8*s, 30*s}`，30s 后维持 30s 周期重试（不无限增长，便于"网络已断 N 秒"渲染稳定）
  2. 每次重连调 `sshConnect(sshCfg)`（复用启动期已缓存 password — REQ-F3-D 不弹密码的核心）
  3. 重连成功 → `onReconnected(*ssh.Client)` 重新挂 keepalive + 把缓冲输入 flush 到新 session
  4. 用户按 `Enter` 触发立即重试（`reconnect.go` 暴露 `Trigger()` 方法，由 input loop 在 `\r` / `\n` 命中时调用）
  5. 重连永久失败兜底（用户连续 5 次 `Enter` 触发的快速重试都失败 ≥ 60s）→ 渲染 `SESSION_RECONNECT_GAVE_UP` 错误码 + 中文 `重连失败，请检查网络后重新运行 claudedock，或运行 claudedock doctor 诊断` + 退出 `ExitNetworkError`（REQ-F3-C 两要素）
  6. 重连过程中 mount 层（mutagen / sshfs / mergerfs）**不**重启——Mutagen 自身有 transport reconnect、sshfs 有 `reconnect` 选项（Phase 31 D-27 已配）、mergerfs 在容器内常驻不受影响

### 本地输入缓冲（REQ-F3-B）

- **D-06**：`input_buffer.go` 实现 `BufferedStdin{ pipe io.WriteCloser, conn ConnState, ringBuf []byte }`：
  1. 启动期把 `os.Stdin` 用一个中间 `io.Pipe()` 替代，写入端连 SSH session.Stdin
  2. `ConnState` 是原子 enum `Connected | Reconnecting | Disconnected`；进入 `Reconnecting` 后所有用户键入字符进入 ringBuf（最大 4KB，超容则丢最早的一段并打 `[!] 输入缓冲已满，部分历史已丢弃` 警告）
  3. 渲染策略：进入 `Reconnecting` 立即把每个键入字符以 ANSI 灰色（`\x1b[90m...\x1b[0m`）echo 到 `os.Stdout`（本地 echo），重连成功后**先**把 ringBuf 通过 `pipe` flush 给远端，**再**让远端 echo 还原（远端真正的 `\x1b[0m` 颜色覆盖本地灰色）
  4. 不支持 backspace 修正未确认字符（OOS-A：与 Mosh 简化版对齐，避免实现复杂度爆炸；按下 backspace 时同样进入缓冲队列，远端按真实顺序处理）
  5. 非 TTY 模式（管道 / `claudedock --` 透传）→ 完全不启用 buffer，stdin 直接传给 SSH session，避免脚本场景被改写
  *[auto] 推荐项：客户端 wrap stdin（替代 tmux 控制模式：(a) 控制模式需要 `tmux -CC`，与默认 attach 流程不兼容；(b) wrap stdin 与本阶段 reconnect 状态机直接共享 `ConnState`，逻辑闭环）*

### tmux session 命名（解决 Q8）

- **D-07**：tmux session 命名锁定 **per-claude_account**（与 Phase 31 D-06 mutagen session 命名 / Phase 33 volume 命名 `claude-state-{account_id}` 拓扑一致）：
  - 默认 session 名：`claude-<account_id_short8>`，`account_id_short8` 取 UUID 前 8 字符（小写，去连字符）
  - `claude_account_id` 缺失（旧 gateway / Phase 30 D-05 兜底）→ 退化为 `claude-anon-<cwd_hash8>`，与 Phase 31 D-06 anon 路径一致；同时 stderr 打 `[!] gateway 未返回 claude_account_id，tmux session 退化为 anon 隔离`
- **D-08**：`--new-session` 创建独立 session 命名 `claude-<short_id>`，`short_id` 由 `crypto/rand` 取 6 字节 → `base64url`（去填充）= 8 字符，确保不与默认 session 撞名（默认是 8 hex；short_id 是 base64url 包含 `-`/`_` 字面值，命名空间正交）
- **D-09**：session 名长度上限 32 chars（tmux 没有强限制，但 banner 渲染 / `tmux ls -F` parsing 友好）；命名字符集仅允许 `[a-zA-Z0-9_-]`，不合法字符替换为 `_` 并打 warning
  *[auto] Q8 推荐项：per-claude_account（替代 per-user：(a) 与 Phase 31/33 命名拓扑一致，doctor 维度排查容易；(b) 同一用户多 claude_account 切换时 session 自然隔离，符合"账号 = 工作环境"的产品定位）*

### tmux 包装与多端 attach

- **D-10**：远程命令模板（替换 v2.0 `runClaude` 的 `claude <args>`）：
  ```bash
  cd <remoteCwd> && command -v tmux >/dev/null 2>&1 && \
    exec tmux new-session -A -d -s <session> "<wrap_cmd>" \; attach-session -t <session> \
    || exec <fallback_cmd>
  ```
  - `<wrap_cmd>` = `cd <remoteCwd> && claude <args>`（shellescape 引用）
  - `<fallback_cmd>` 仅在 `command -v tmux` 失败时执行——直接 `claude <args>` 裸跑（v2.0 行为）
  - `new-session -A` = 已存在则 attach（多端共享 attach 默认行为，REQ-F5-A）
  - `-d` + `attach-session` 分两步是为了在 detached 状态下完成 session 创建后再 attach，避免 `-A` 在 attach 时同时创建产生竞态
- **D-11**：`--take-over` 实现：
  1. 启动后先在 conn-A 远程执行 `tmux list-clients -t <session> -F "#{client_tty}|#{client_activity}|#{client_name}"` 解析活跃 client
  2. 命中 ≥1 个时，先在 conn-A 远程 `tmux display-message -t <session> "[claudedock] 另一端 (<source>) 已通过 --take-over 接管会话，本会话将在 3s 后断开"` + `sleep 3` + `tmux detach-client -t <session> -a`（`-a` = 踢掉除了 current 之外所有）
  3. 然后本端 attach（已经成为唯一 client）
  4. 命中 0 个时，与默认行为一致（直接 attach）
- **D-12**：第二端 banner 文案模板（REQ-F5-B）：
  ```
  ✓ 已 attach 到会话 <session>
    （另 <N> 个会话正在共享：<source1> / <活跃时间1>，<source2> / <活跃时间2>）
  ```
  - 数据源：`tmux list-clients -t <session> -F "#{client_name}|#{client_activity}|#{client_tty}"`
  - `<source>` = 优先取 `client_name`（claudedock 在 attach 时通过环境变量 `CLOUD_CLAUDE_CLIENT_NAME=<local_hostname>` 写入，tmux 自动从 SSH `SSH_CLIENT` 环境取主机名做 fallback）
  - `<活跃时间>` = `now - client_activity_unix_seconds`，渲染为中文 `N 分钟前活跃` / `N 秒前活跃`
  - 若 `tmux list-clients` 返回为空（首端 attach），不输出"另 N 个会话正在共享"行，只输出 `✓ 已 attach 到会话 <session>`
- **D-13**：`claudedock sessions ls` 实现（REQ-F4-B）：
  1. 远程执行 `tmux list-sessions -F "#{session_name}|#{session_created}|#{session_attached}|#{session_windows}"`
  2. 本地渲染表格：
     ```
     SESSION              CREATED               CLIENTS  WINDOWS
     claude-abc12345      2 小时前              2        1
     claude-shortid01     5 分钟前              0        1
     ```
  3. 若 `tmux list-sessions` 失败（无 server / 无 session）→ 输出 `当前容器内无活跃 tmux session`，退出 0
- **D-14**：`claudedock sessions attach <name>` 实现：完整复用 `runClaude` 但 session 名为 args[0]，跳过 `claude` 启动命令包装（直接 `tmux attach-session -t <name>`）；session 不存在时返回 `SESSION_NOT_FOUND` 错误 + 中文 `指定 session 不存在；运行 claudedock sessions ls 查看当前 session 列表`

### 容器内 tmux 不可用降级（REQ-F4-C）

- **D-15**：tmux 探测时机：mount ready 后、OAuth 检查后、`runClaude` 调用前；远程执行：
  ```bash
  command -v tmux >/dev/null 2>&1 && tmux -V 2>&1
  ```
  失败（exit != 0 或 stderr 非空）→ 设置 `mountCfg.TmuxAvailable=false`，`runClaude` 走 v2.0 裸 `claude <args>` 路径 + banner：
  ```
  [!] 容器内 tmux 不可用，会话恢复已禁用
      原因: <stderr 第一行>
      错误码: SESSION_TMUX_UNAVAILABLE
      建议: 检查容器镜像是否升级到 v3.0.0 或运行 claudedock doctor
  ```
- **D-16**：探测**不**阻塞启动——任何 tmux 故障都不退出非 0；用户始终能进入 claude，只是失去会话恢复能力（REQ-F4-C 字面要求）

### 账号级 Mutagen 单例锁（REQ-F5-D / PITFALLS M15）

- **D-17**：单例锁实现采用 **(a) 容器内 flock**，**不**在客户端本地：
  1. claudedock 在 mount 启动前（即将创建 Mutagen sync session 前），通过 conn-A 远程执行：
     ```bash
     mkdir -p /var/lock/claudedock && \
     flock -n -E 99 -F /var/lock/claudedock/sync-<account_id>.lock -c 'echo "lock acquired"; sleep infinity' &
     echo $!
     ```
     启动一个常驻 sleep 进程持有 flock，记录 PID 到本地 cleanup 注册表
  2. 退出码：0 = 锁拿到（首端）；99 = 锁被占（后端）；其它 = flock 不可用（极旧镜像，跳过锁机制 + warning）
  3. 后端拿不到锁 → `mountCfg.SyncSessionLock` 注入返回 `errSyncLocked`，`MountWorkspace` 看到该错误后**降级**到 sshfs-only（不算 mount 失败，是策略性降级）+ stderr 中文 `[!] 账号 <account_id> 已有另一端在执行 Mutagen sync，本端只读 sshfs 视图`
  4. cleanup 时通过本地记录的 PID 远程 `kill <pid>`（flock 自然释放）；claudedock 异常崩溃 → flock 在 sleep 进程被 OS 收割时自动释放（最坏 30s 内 sleep infinity 还在跑，靠 sshd channel close 收割）
- **D-18**：与 Phase 31 D-31 `MountConfig.SyncSessionLock` 接口对接：本阶段提供 `func acquireSyncLock(conn *ssh.Client, accountID string) (release func(), err error)` 实现，注入到 `MountConfig.SyncSessionLock`，使本阶段对 `mount_strategy.go` 零侵入
- **D-19**：单例锁仅在 `claude_account_id != ""` 时启用；anon 路径（D-07）跳过锁，因为 anon 隔离已经按 `cwd_hash8` 自然区分

### 错误码注册（REQ-F8-A / B 的 phase-32 落码 + PITFALLS C8）

- **D-20**：本阶段必须落地的错误码（注册到 `errcodes/session.go` + 追加 `errcodes/net.go`，每条三要素：`Code` + 中文 `Message` 模板 + 中文 `NextAction`）：

  | Code | 触发 | 防御 |
  |------|------|------|
  | `SESSION_KEEPALIVE_TOO_AGGRESSIVE` | CLI flag / config `keepalive_interval < 15s` | REQ-F3-A / M11 |
  | `SESSION_TMUX_UNAVAILABLE` | 容器内 `command -v tmux` 失败 / `tmux -V` 异常 | REQ-F4-C |
  | `SESSION_NOT_FOUND` | `claudedock sessions attach <name>` 但 session 不存在 | REQ-F4-B |
  | `SESSION_TAKEOVER_NOTIFIED` | `--take-over` 成功踢人后的 informational | REQ-F5-C |
  | `SESSION_TAKEOVER_FAILED` | `--take-over` `tmux detach-client` 命令失败 | REQ-F5-C |
  | `SESSION_SYNC_LOCKED` | 后连端拿不到 Mutagen 单例锁 → 降级 sshfs 观察 | REQ-F5-D / M15 |
  | `SESSION_BUFFER_OVERFLOW` | 本地输入缓冲 ringBuf 满（4KB） | REQ-F3-B |
  | `NET_RECONNECT_BACKOFF` | 重连退避中（informational，按 `1s/2s/4s/8s/30s` 渲染） | REQ-F3-D |
  | `NET_RECONNECT_GAVE_UP` | 重连永久失败兜底 | REQ-F3-C |
  | `NET_TCP_KEEPALIVE_UNSUPPORTED` | TCP `setsockopt` 平台不支持 / 失败（warning） | D-04 |

  10 条新增 + Phase 31 已注册的 11 条 mount + 3 条 net OAuth = ≥ 24 条，命名空间与 v2.0 现网 `auth_*` / `host_action_failed` / `entry_*` 全大写形态正交（Phase 31 D-20 同样结论）
- **D-21**：所有错误输出延续 Phase 31 D-21 `errcodes.Format(code, args...) string` helper 格式（`[<code>] <Message>` + `\n  建议: <NextAction>`），不引入新 helper

### 三态 UX 阈值渲染（REQ-F3-C）

- **D-22**：渲染时机由 `reconnect.go` 内部 ticker 驱动（每 100ms 评估当前断网时长）：
  - `disconnectDuration > 1.5s` → 在 prompt 上方插入灰色 `…`（持续 echo，不刷屏：用 `\r\x1b[K\x1b[90m… <N.N>s\x1b[0m`）
  - `disconnectDuration > 8s` → 替换为黄色 `\r\x1b[K\x1b[33m⚠ 网络抖动中（<N> 秒未响应）\x1b[0m`
  - `disconnectDuration > 30s` → 替换为红色 `\r\x1b[K\x1b[31m✗ 网络已断 <N> 秒，正在自动重试…\x1b[0m`
  - 重连成功 → 清空当前行 `\r\x1b[K`，恢复 stdout 原状（用户继续输入）
- **D-23**：颜色尊重 `NO_COLOR` 环境变量（与 Phase 31 D-17 banner 一致策略）：`NO_COLOR=1` 时去掉所有 ANSI 序列，仅保留文本 `[disconnected 30s]` 等纯字符串

### 与 Phase 29 / 31 / 33 / 34 的接口

- **D-24**：从 Phase 30 `AuthResponse` 继续读取（与 Phase 31 D-29 一致）：
  - `claude_account_id` → tmux session 命名（D-07）+ 单例锁 key（D-17）
  - 其它字段（`image_version` / `supports_*`）已由 Phase 31 mount 路径消费，本阶段不重复读
- **D-25**：从 Phase 29 落地的容器侧基线（验证而非改造）：
  - `sshd_config` `ClientAliveInterval 15` / `ClientAliveCountMax 8`（Phase 29 D-14）—— 本阶段 claudedock 启动期可选 `assert_ssh_alive`：远程 `sshd -T 2>/dev/null | grep -E "clientalive(interval|countmax)"`，缺失时 warning（不阻塞）
  - tmux 3.4+ 已通过 entrypoint `assert_tmux_version`（Phase 29 D-06）保证；本阶段 D-15 探测仍执行兜底
  - tini PID 1 + 无 systemd（Phase 29 D-10 / D-15）—— C7 防御已在镜像侧完成，本阶段验收用例 `pkill -SIGHUP sshd` 后 tmux 仍存活
- **D-26**：为 Phase 33 预留接口：本阶段 tmux session 与持久化无直接耦合（claude OAuth credentials 在 `~/.claude/`，由 Phase 33 named volume 挂载）；session 内的 claude 进程在容器重建后**会丢失**（这是 v3.0 设计：会话恢复 = 同一容器内重连；跨容器重建留 v3.1）
- **D-27**：为 Phase 34 预留接口：
  - `errcodes/session.go` 与 `errcodes/net.go` 通过 init() 自动注册到全局 Registry（与 Phase 31 D-32 一致），Phase 34 doctor `claudedock explain <code>` 直接复用
  - `~/.claudedock/last-session.json` 增加可选字段 `tmux_session: string` / `client_role: "primary"|"secondary"` / `reconnect_count: int`（schema_version 保持 1，新字段全部 `omitempty`）
  - tmux 探测结果（成功 / 失败 / 错误码）写入 last-session.json，doctor mount/ssh 维度可读

### 启动流程改造（在 ConnectAndRunClaudeV3 内插入）

- **D-28**：`ConnectAndRunClaudeV3` 流程在 Phase 31 基础上插入两步（不修改函数签名，保持 Phase 31 兼容入口）：
  1. **现状**：sshConnect → MountWorkspace → OAuth Check → runClaude
  2. **本阶段**：
     ```
     sshConnect (含 KeepAlive D-03 / D-04)
       ↓
     MountWorkspace (Phase 31，注入 SyncSessionLock = D-17 实现)
       ↓
     OAuth Check (Phase 31)
       ↓
     [新] tmux 探测 D-15 → 设置 sessionMode (tmux | bare)
       ↓
     [新] runClaudeWithSession (D-10 wrap, 含 reconnect.Run + input_buffer 包装)
     ```
- **D-29**：`runClaudeWithSession` 从 `runClaude` fork 而来（保留 v2.0 `runClaude` 不动用于 fallback / `sessions attach` 子命令直接复用）；新函数签名：
  ```go
  func runClaudeWithSession(ctx context.Context, conn *ssh.Client, sshCfg SSHConfig,
      claudeArgs []string, remoteCwd string, hasProxy bool,
      sessionCfg SessionConfig) (int, error)
  ```
  其中 `SessionConfig`：
  ```go
  type SessionConfig struct {
      AccountID    string  // D-07 决定 session 命名
      ShortID      string  // --new-session 用
      TakeOver     bool    // --take-over flag
      TmuxAvailable bool   // D-15 探测结果
      KeepAliveInterval time.Duration
      KeepAliveCountMax int
      ReconnectEnabled  bool // 默认 true；测试可关
  }
  ```

### Claude's Discretion

以下细节由 planner / executor 按实现便利性决定：

- `keepalive.Run` goroutine 是否复用 errgroup vs 独立 `context.WithCancel`（建议 errgroup 与 reconnect 协同）
- TCP `setsockopt` 在 Windows 平台的处理（v2.0 claudedock 是否支持 Windows 未定；建议 Windows build tag 默认 noop + warning，与 D-04 一致）
- `input_buffer.go` 的 ringBuf 容量（4KB 够典型 30s 慢速键入；planner 可调到 8KB 不影响内存）
- `sessions ls` 输出表格的列宽与中文宽字符对齐（建议用 `text/tabwriter` 简化）
- `tmux list-clients` 解析时分隔符 `|`（容器侧 username 不含 `|`，安全）
- 重连状态机 `Trigger()` 的实现是否引入 channel buffer（建议 size=1 + drop 多余 trigger 防 spam）
- 单例锁 sleep 进程的 PID 跟踪（建议直接用远程 `flock -n -F` 返回的 fd → 由 ssh session 自然管理；session.Close() 即释放）
- 灰色未确认渲染如果命中非 ASCII（中文）输入：本阶段允许局部错位（每个 rune 一组 ANSI 包裹），完美对齐留 v3.1
- `--new-session` short_id 长度（默认 8 字符；planner 可调至 6 不破坏命名空间）
- 多端 attach 时 banner 是否在 client_count == 1 时也输出 `（仅本端）`：建议不输出（避免噪音）
- `claudedock sessions ls` 是否支持 `--json`：本阶段不要求；可作为 deferred

### Folded Todos

无（`todo match-phase 32` 返回 0 条匹配）。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 规划与需求

- `.planning/PROJECT.md` — v3.0 总目标、Constraints（沟通中文 / 部署单宿主机 / 零增量特权）、Key Decisions
- `.planning/REQUIREMENTS.md` §B1（F3 SSH 弱网容忍）/ §B2（F4 tmux 包装）/ §B3（F5 多端 attach） — 本阶段交付的 11 条 REQ
- `.planning/REQUIREMENTS.md` §性能与体验验收基线 — BASE-03（弱网 30s 无感知，Phase 35 真机验收，本阶段为前置实现）
- `.planning/REQUIREMENTS.md` §Critical Pitfalls — **C3 / C7 / M11 / M12 / M13 / M15** 必须显式防御
- `.planning/REQUIREMENTS.md` §Open Questions — Q8（tmux session 命名 per-user vs per-claude_account）本阶段拍板（D-07）
- `.planning/REQUIREMENTS.md` §Out of Scope — OOS-A6（不用 UDP / Mosh）/ OOS-A7（断网不杀进程）/ OOS-A8（tmux 不永不过期）/ OOS-A9（默认共享不踢人）/ OOS-A10（无实时协作光标）/ OOS-A20（不新增 session REST endpoint）
- `.planning/ROADMAP.md` §Phase 32 — 官方 Goal / Scope / 12 条 Success Criteria（plan-checker 验收锚点）
- `.planning/STATE.md` — v3.0 milestone 进度（当前 Phase 31 已完成 3/3 plan，本阶段为下一个）

### 前置阶段上下文（必读，避免重复决策）

- `.planning/phases/29-v3-worker/29-CONTEXT.md` §D-06 / D-13 / D-14 / D-15 — 容器侧 tmux 3.4+ / `/etc/tmux.conf` truecolor / sshd KeepAlive 15s/8 / 不跑 systemd（C7 镜像侧防御）
- `.planning/phases/30-entry-api/30-CONTEXT.md` §D-05 / D-09 — `claude_account_id` 解析路径与 `HostActionRequest.ClaudeAccountID` 字段（本阶段读取 AuthResponse 的来源）
- `.planning/phases/31-cli/31-CONTEXT.md` §D-06 / D-29 / D-30 / D-31 / D-32 — Mutagen session 命名 / AuthResponse 消费 / `ConnectAndRunClaudeV3` 入口 / `MountConfig` 预留接口（KeepAlive / SyncSessionLock）/ errcodes 注册表与 last-session.json schema
- `.planning/phases/31-cli/31-CONTEXT.md` §D-27 / D-28 — sshfs 抖动主动摘除 watcher（C3 防御，本阶段验收复用）+ Mutagen conflict 冒泡近似实现（"下次回车前"近似为 banner 后立即输出，本阶段沿用）

### 研究基线

- `.planning/research/SUMMARY.md` §3 / §5 / §7 — v3.0 需求清单、TOP10 pitfalls、Out of Scope
- `.planning/research/STACK.md` — tmux / sshd / SSH 客户端版本与理由
- `.planning/research/PITFALLS.md` C3 / C7 / M11 / M12 / M13 / M15 — sshfs 抖动级联 / systemd-logind / SSH KeepAlive 反向坑 / sshd MaxSessions / 静默降级 / 多端 Mutagen 双写
- `.planning/research/FEATURES.md` §会话可靠性 — F3 / F4 / F5 设计参考
- `.planning/research/ARCHITECTURE.md` §claudedock 端 / §host-agent 边界 — 本阶段不扩展 host-agent endpoint（OOS-A20），全部走 SSH

### 既有代码（直接改造对象）

- `internal/claudedock/ssh.go` — `sshConnect` / `runClaude` / `ConnectAndRunClaudeV3`；本阶段在 `sshConnect` 上叠 KeepAlive、`runClaude` 拆出 `runClaudeWithSession`、`ConnectAndRunClaudeV3` 插入 tmux 探测与 reconnect 包装
- `internal/claudedock/mount_strategy.go` — `MountConfig.SyncSessionLock` / `KeepAliveInterval` / `KeepAliveCountMax` 字段（Phase 31 D-31 已预留）；本阶段在 `cmd/claudedock/main.go` 注入这三个字段
- `internal/claudedock/exitcodes.go` — 退出码常量（本阶段需要复用 `ExitNetworkError` / `ExitConfigError` / `ExitInternalError`，不新增退出码避免与 v2.0 / Phase 31 撞码）
- `internal/claudedock/errcodes/codes.go` — `MustRegister` / `Format` / `Lookup` / `Registry`；本阶段在 `errcodes/session.go` + `errcodes/net.go` 追加注册
- `internal/claudedock/last_session.go` — `LastSessionSnapshot` schema；本阶段追加 `tmux_session` / `client_role` / `reconnect_count` 字段（全部 `omitempty`，schema_version 保持 1）
- `internal/claudedock/sshfs_watcher.go` — Phase 31 已有 watcher（C3 防御）；本阶段验收 30s 抖动场景，**不**重写
- `cmd/claudedock/main.go` — cobra root 注册 `--new-session` / `--take-over` flag、注册 `sessions` 子命令、把 KeepAlive / SyncSessionLock 注入 MountConfig 与 SessionConfig
- `cmd/claudedock/sync.go` — Phase 31 sync 子命令实现参考（cobra subcommand 注册模式）

### 容器侧基线（验收对象，不改）

- `deploy/docker/managed-user/Dockerfile` — tmux 3.4+ 安装、`/etc/tmux.conf`、`/etc/profile.d/claudedock.sh` 已就位（Phase 29 D-13）
- `deploy/docker/managed-user/entrypoint.sh` — `assert_tmux_version` / 不跑 systemd（Phase 29 D-06 / D-15）
- `deploy/docker/managed-user/sshd_config` — `ClientAliveInterval 15` / `ClientAliveCountMax 8` / `MaxSessions 30` / `MaxStartups 60:30:120`（Phase 29 D-14）

### 新增文件预告

- `internal/claudedock/session.go` — tmux 决策 + sessions 子命令逻辑
- `internal/claudedock/reconnect.go` — 重连状态机 + 三态 UX 渲染
- `internal/claudedock/input_buffer.go` — 本地输入缓冲 + 灰色未确认渲染
- `internal/claudedock/keepalive.go` — SSH + TCP 双层 KeepAlive
- `internal/claudedock/sync_lock.go` — 账号级 Mutagen 单例锁（容器侧 flock 包装）
- `internal/claudedock/errcodes/session.go` — `SESSION_*` 错误码注册
- `cmd/claudedock/sessions.go` — `claudedock sessions ls/attach` 子命令

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `sshConnect`（`ssh.go`）已封装 TCP dial + SSH handshake，本阶段在其上叠加 KeepAlive 与 TCP `setsockopt` 即可（一行 `tcpConn.SetKeepAlive(true)`）；返回值改为可选 `*net.TCPConn` 暴露以便平台特化 setsockopt
- `runClaude`（`ssh.go`）已实现 PTY 申请、SIGWINCH 处理、stdin/stdout/stderr 透传、ExitError 解包；本阶段 `runClaudeWithSession` 直接 fork + 在 `remoteCmd` 拼接处插入 tmux 包装；fallback 直接走原 `runClaude`
- `MountConfig.SyncSessionLock`（`mount_strategy.go`）已预留 `func(accountID string) (release, error)` 接口，本阶段实现 `acquireSyncLock` 注入
- `MountConfig.KeepAliveInterval` / `MountConfig.KeepAliveCountMax`（`mount_strategy.go`）已预留 `time.Duration` / `int` 字段，本阶段在 `main.go` 与 `sshConnect` 之间打通
- `errcodes/codes.go` 的 `MustRegister` + 包级 init() 注册模式可直接复用（与 Phase 31 `errcodes/mount.go` / `errcodes/net.go` 一致）
- `LastSessionSnapshot.SchemaVersion=1` + 全字段 `omitempty` 模式可平滑追加新字段
- `colors.go` 已有 ANSI 颜色 helper（`ansiYellow` / `ansiGreen` / `colorize` / `colorEnabled`），本阶段 reconnect 三态 UX 与 banner 直接复用，新增 `ansiGray=90` / `ansiRed=31` 常量

### Established Patterns

- **错误返回**：v3.0 mount 路径已统一 `errcodes.Format(code, ...) + fmt.Errorf("...: %w", err)` 包装（Phase 31 D-21）；本阶段 session / reconnect / sync_lock 路径完全延续，禁止新建错误格式
- **SSH 远程命令构造**：统一 `shellescape.QuoteCommand`（v2.0 ssh.go 与 Phase 31 mount 各 .go 已用），本阶段 tmux 包装 / `tmux list-clients` 解析等延续
- **超时控制**：v3.0 已统一 `context.WithTimeout` + errgroup（Phase 31 mount_strategy）；本阶段 reconnect / keepalive / sync_lock 全部用 context，不引入 `time.NewTimer` 裸用
- **平台分发**：v2.0 / Phase 31 已采用 `runtime.GOOS == "darwin"` 条件 + build tag 双层（envcheck.go）；本阶段 TCP setsockopt 走 `//go:build linux` / `//go:build darwin` 文件分发，与既有模式一致
- **cobra 子命令注册**：v2.0 `cmd/claudedock/main.go` 已建立子命令树（`init` / `env` / `ssh` / `sync`），本阶段 `sessions` 子命令延续：`sessionsCmd.AddCommand(lsCmd, attachCmd)` 并在 root `AddCommand` 注册
- **DisableFlagParsing 处理**：v2.0 main.go runRoot 因 `DisableFlagParsing=true` 手动剥离 `--mount-mode`；本阶段新增 `--new-session` / `--take-over` 同样手动剥离，避免透传给远端 claude

### Integration Points

- Phase 30 `AuthResponse.ClaudeAccountID` 是 tmux session 命名（D-07）+ 单例锁 key（D-17）的源头；缺失时降级 anon（与 Phase 31 D-29 一致）
- Phase 29 容器侧 tmux 3.4+ / sshd KeepAlive / 不跑 systemd 是本阶段功能正常运行的前提，本阶段通过 `assert_*` 探测验收（D-15 / D-25）
- Phase 31 mount 全链路必须在本阶段重连过程中**保持不动**：Mutagen 自身有 transport reconnect、sshfs `reconnect=yes`、mergerfs 容器内常驻，claudedock reconnect 只重建 conn-A 上的 SSH session（用于 tmux attach）
- Phase 33 Docker named volume 与本阶段 tmux session **解耦**：volume 是 `~/.claude/.credentials.json` 的持久化容器，session 是 PTY 状态的容器；两者命名都用 `claude_account_id` 是巧合（拓扑一致便于排查），但生命周期独立
- Phase 34 doctor `mount` / `ssh` 维度复用本阶段注册的 `SESSION_*` / `NET_*` 错误码 + last-session.json 新字段（`tmux_session` / `reconnect_count`）

</code_context>

<specifics>
## Specific Ideas

- 用户对 v3.0 的核心承诺是「日常开发主战场」——本阶段 30s 抖动无感知 + 进程不丢 + 多端共享 attach 是这条承诺的**直接体感证据**；planner 在 Plan 切分时应把 D-03 KeepAlive / D-05 重连状态机 / D-06 输入缓冲 / D-10 tmux 包装视为 P0，禁止简化成「只装 tmux + 配 KeepAlive」
- ROADMAP §Phase 32 Success Criteria 第 2 条 `断网 30s 内 claudedock 不退出、运行中 claude 进程不丢、tmux session 可重连 attach 同一会话且历史 buffer 完整`：测试用例必须真实拔网（`docker network disconnect <ctr-name> <net-name>` 30s 后 reconnect），不能用 mock；buffer 完整由 `tmux capture-pane -p` 验证
- ROADMAP §Phase 32 Success Criteria 第 3 条 `pkill -SIGHUP sshd 后 tmux attach 仍成功`：是 PITFALLS C7（systemd-logind kill tmux）的关键回归 — 由 Phase 29 D-15 不跑 systemd 完成镜像侧防御，本阶段必须有用例断言**容器侧无 systemd-logind**（`pgrep systemd-logind` 必须无结果）
- ROADMAP §Phase 32 Success Criteria 第 11 条 `同一 claude_account 第二个 claudedock 启动时，Mutagen sync 不重复创建（账号级单例锁），后连端只读 sshfs / mergerfs 视图`：测试场景必须真实跑两个 claudedock 进程（同一 account 不同 cwd），断言只有一个 Mutagen sync session 存在（`mutagen sync list` 验证）
- 用户已在 Phase 30 D-05 / 31 D-06 显式选择 `claude_account_id` 作为隔离维度；本阶段 D-07 / D-17 严格延续，**禁止**用 user_id 或 host_id 替代
- v3.0 PROJECT.md 强调「零增量特权」——本阶段所有改造（KeepAlive / TCP setsockopt / tmux / flock）依然在 v2.0 已开放的容器特权与本地用户权限内，禁止新增 `--privileged` 或新 capability
- 多端默认共享 attach 是产品差异化决策（OOS-A9 明确：违反"两个屏都想看"的用户直觉就是 bug）；planner 在 Plan 切分时应优先实现 D-10 默认共享 + D-12 banner，再做 `--take-over`，确保即使 take-over 没做完产品也能 ship

</specifics>

<deferred>
## Deferred Ideas

### 阶段内确认但不交付的 follow-up

- **Mosh-style 真正的 UDP 协议**：被 OOS-A6 排除（与 sing-box tun + nftables 默认拒绝模型不兼容），本阶段以"客户端缓冲 + 三态 UX + tmux 服务端持久"近似实现 Mosh 的核心体感
- **跨容器重建的 tmux session 持久化**：v3.0 设计是"会话恢复 = 同一容器内重连"；跨容器（如 admin 重启 host）的 session 持久化需要 dtach + 文件系统 buffer，留 v3.1
- **`sessions ls --json`**：本阶段 `claudedock sessions ls` 仅渲染人类表格；JSON 输出留 Phase 34 doctor `--json` 框架统一处理
- **`--take-over --notify-only`** 模式（仅通知不踢人）：本阶段 `--take-over` 直接踢人；只通知场景留 v3.1（产品价值低）
- **Mutagen 跨主机单例锁**（多 host 同一 claude_account）：本阶段单例锁绑定容器内 flock，仅覆盖单 host；跨 host 协调需要控制面介入，违反 OOS-A20，留 v3.1
- **input buffer 的 backspace 修正**：本阶段灰色未确认字符不支持本地 backspace 修正；与 Mosh 简化版对齐
- **完美的中文宽字符灰色对齐**：本阶段允许 ANSI 包裹错位（每个 rune 一组），完美对齐留 v3.1
- **重连过程中 mount 层主动重启**：本阶段 reconnect 仅重建 SSH session；mutagen / sshfs / mergerfs 不重启（依赖各自 reconnect 机制）；如真机 30s/2min UAT（Phase 35）发现 mount 层无法自愈，回流到本阶段补"mount 层重启 hook"
- **`sessions ls --remote-host`**：本阶段 sessions 子命令仅查询当前已 init 的网关；跨 gateway 查询留 v3.1（admin 后台范畴）
- **PTY 真正"下次回车前"prompt 上方插入**：沿用 Phase 31 D-28 决策，本阶段不做严格 PTY 拦截渲染

### Reviewed Todos (not folded)

无（`todo match-phase 32` 返回 0 条匹配）。

</deferred>

---

*Phase: 32-ssh-tmux*
*Context gathered: 2026-04-20*
*Mode: --auto（所有 gray area 自动选定推荐项）*
