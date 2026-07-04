# Phase 32: SSH 会话可靠性 + tmux 包装 + 多端 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-20
**Phase:** 32-ssh-tmux
**Mode:** `--auto`（所有问题由 Claude 自动选定推荐项；本日志保留候选方案与判断依据，便于审计）
**Areas discussed:** 重连机制选型、TCP KeepAlive 平台分发、本地输入缓冲实现、tmux session 命名（Q8）、多端 attach 探测、--take-over 通知机制、账号级 Mutagen 单例锁、容器内 tmux 不可用降级、错误码命名空间、UX 三态阈值渲染

---

## 重连机制选型

| Option | Description | Selected |
|--------|-------------|----------|
| 自实现 reconnect on top of `golang.org/x/crypto/ssh` | 复用 v2.0 既有 sshConnect，零依赖变化，与本阶段 input buffer / take-over 直接共享 *ssh.Client | ✓ |
| OpenSSH 子进程 + ControlMaster | 利用 OpenSSH 成熟实现，但 ControlMaster macOS / Windows 行为不一致，破坏 ExecProxy / sftp 复用 | |
| Mosh-style 自定义 UDP 协议 | 与 sing-box tun + nftables 默认拒绝模型不兼容（OOS-A6） | |

**Auto 选择：** 自实现
**理由：** v2.0 已用 `golang.org/x/crypto/ssh`，切换会破坏既有 ExecProxy / sftp 复用；自实现使本阶段 input buffer / take-over 等能直接操作 *ssh.Client，无需进程间通信。

---

## TCP KeepAlive 平台分发

| Option | Description | Selected |
|--------|-------------|----------|
| SO_KEEPALIVE + Linux TCP_USER_TIMEOUT + macOS TCP_KEEPALIVE 平台特化 | 跨平台基础 + 各平台最优实现，防御 NAT idle drop | ✓ |
| 仅 SO_KEEPALIVE 跨平台 | 简单但 Linux 上无法防御长时间未 ACK 的死连接 | |
| 仅依赖 SSH 层 KeepAlive | 实现最少但 TCP 层死连接无法被探测 | |

**Auto 选择：** 平台特化
**理由：** Linux TCP_USER_TIMEOUT=30000 与 macOS TCP_KEEPALIVE=15 是各自平台等价于 SSH KeepAlive 的最佳兜底；setsockopt 失败仅 warning，不阻塞连接。

---

## 本地输入缓冲实现

| Option | Description | Selected |
|--------|-------------|----------|
| 客户端 wrap stdin → ringBuf + 灰色 ANSI 渲染 | 与 reconnect 状态机直接共享 ConnState，逻辑闭环；不依赖远端 | ✓ |
| 利用 tmux 控制模式（tmux -CC） | 与默认 attach 流程不兼容，需要双模式切换 | |
| 不实现，用户感受到延迟 | 违反 REQ-F3-B | |

**Auto 选择：** 客户端 wrap stdin
**理由：** ringBuf 4KB 容量、ANSI 灰色渲染、非 TTY 模式跳过；与 reconnect.Run 共享 ConnState 原子枚举。

---

## tmux session 命名（Open Question Q8）

| Option | Description | Selected |
|--------|-------------|----------|
| per-claude_account: `claude-<account_id_short8>` | 与 Phase 31 mutagen session / Phase 33 volume 命名拓扑一致 | ✓ |
| per-user: `claude-<user_id>` | 同一用户多 account 时无法隔离 | |

**Auto 选择：** per-claude_account
**理由：** 与 Phase 31 D-06 / Phase 33 D-01 命名拓扑一致，doctor 维度排查容易；`claude_account_id` 缺失时退化为 `claude-anon-<cwd_hash8>`，与 Phase 31 D-29 anon 路径一致。

---

## 多端 attach 探测策略

| Option | Description | Selected |
|--------|-------------|----------|
| 远程 `tmux list-clients -t <session> -F` 解析 | 零控制面改造，对齐 OOS-A20 | ✓ |
| 控制面新增 session 计数 endpoint | 违反 OOS-A20 | |

**Auto 选择：** tmux list-clients
**理由：** 数据源全部在容器内，claudedock 通过 SSH 远程 tmux 命令获取；client_name 通过 SSH `SSH_CLIENT` + 环境变量 `CLOUD_CLAUDE_CLIENT_NAME=<local_hostname>` 标识。

---

## --take-over 通知机制

| Option | Description | Selected |
|--------|-------------|----------|
| `tmux display-message` + `sleep 3` + `tmux detach-client -a` | 原生命令，3s 缓冲让对端看到通知 | ✓ |
| 写文件 + watcher 轮询 | 复杂，需要额外进程 | |

**Auto 选择：** tmux 原生命令
**理由：** display-message 直接写入对端 status line / message 区域，detach-client `-a` 踢掉除当前外所有 client。

---

## 账号级 Mutagen 单例锁（REQ-F5-D）

| Option | Description | Selected |
|--------|-------------|----------|
| 容器内 flock `/var/lock/claudedock/sync-<account_id>.lock` | per-host 真锁，跨 claudedock 实例（mac+linux 多端） | ✓ |
| 客户端本地 flock | 不能跨 mac+linux 跨端 | |
| Mutagen daemon session-name 唯一性 + 探测 | 仅最低保护，无法阻止竞态 | |

**Auto 选择：** 容器内 flock
**理由：** flock -n -E 99 失败码 99 = 锁被占；后端拿不到锁即返回 errSyncLocked，MountWorkspace 降级到 sshfs-only 观察。注入路径走 Phase 31 D-31 已预留的 `MountConfig.SyncSessionLock` 接口。

---

## 容器内 tmux 不可用降级

| Option | Description | Selected |
|--------|-------------|----------|
| 启动早期 `command -v tmux` + `tmux -V` 探测，失败即裸 ssh + banner 提示 | 不阻塞启动，符合 REQ-F4-C 字面要求 | ✓ |
| 仅在 `tmux new-session` 失败时降级 | 错误信息混入 claude 启动日志，用户难以察觉 | |

**Auto 选择：** 启动早期探测
**理由：** mount ready / OAuth 检查后立即探测，失败设置 sessionMode=bare + 输出 SESSION_TMUX_UNAVAILABLE banner。

---

## 重连退避序列

| Option | Description | Selected |
|--------|-------------|----------|
| `1s → 2s → 4s → 8s → 30s 上限` | ROADMAP 字面要求，与 v2.0 entry retry 一致 | ✓ |
| 指数退避无上限 | 长时间无网络时指数爆炸 | |
| 固定 5s 间隔 | 缺乏对短抖动的快速恢复能力 | |

**Auto 选择：** 1/2/4/8/30
**理由：** ROADMAP §Phase 32 Success Criteria 第 4 条字面约束；用户按 Enter 可触发立即重试。

---

## UX 三态阈值渲染

| Option | Description | Selected |
|--------|-------------|----------|
| `>1.5s 灰色 …` / `>8s 黄色 抖动 N 秒` / `>30s 红色 网络已断 N 秒` | ROADMAP 字面要求，按时长分阶 | ✓ |
| 仅在 SSH KeepAlive 失败后渲染 | 第一档 1.5s 阈值无法覆盖（KeepAlive 至少 15s） | |

**Auto 选择：** ticker 100ms + 时长分阶
**理由：** reconnect.go 内部 ticker 每 100ms 评估当前断网时长，按阈值切换 ANSI 颜色；NO_COLOR 时去 ANSI 序列保留纯文本。

---

## 错误码命名空间扩展

| Option | Description | Selected |
|--------|-------------|----------|
| 新增 `SESSION_*` + 追加 `NET_*`，复用 Phase 31 errcodes 包 | 与 Phase 31 D-19 / D-20 命名风格一致 | ✓ |
| 单独建 reconnect / session 包 | 违反 Phase 34 单一注册表前提 | |

**Auto 选择：** 复用 errcodes 包
**理由：** Phase 31 D-32 / D-20 已预留接口；本阶段注册 10 条新 code，命名规则匹配 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`，与 v2.0 全小写空间正交（防御 PITFALLS C8）。

---

## Claude's Discretion

以下细节由 planner / executor 决定（CONTEXT.md `Claude's Discretion` 段已枚举）：

- `keepalive.Run` goroutine 复用 errgroup vs 独立 context
- TCP setsockopt 在 Windows 平台的处理（建议 noop + warning）
- `input_buffer.go` ringBuf 容量（4KB / 8KB）
- `sessions ls` 表格列宽与中文宽字符对齐（建议 text/tabwriter）
- `tmux list-clients` 解析分隔符
- 重连状态机 `Trigger()` channel buffer 大小
- 单例锁 sleep 进程 PID 跟踪
- 灰色未确认渲染对中文宽字符的对齐策略（允许局部错位）
- `--new-session` short_id 长度（默认 8）
- 多端 attach 时 banner 是否输出"仅本端"
- `sessions ls --json` 是否本阶段实现（建议留 deferred）

---

## Deferred Ideas

讨论中触发但留到后续阶段 / 版本：

- Mosh-style 真正 UDP 协议（OOS-A6）
- 跨容器重建的 tmux session 持久化（v3.1）
- `sessions ls --json`（Phase 34）
- `--take-over --notify-only` 模式（v3.1）
- Mutagen 跨主机单例锁（v3.1，违反 OOS-A20）
- input buffer 的 backspace 修正（与 Mosh 简化版对齐）
- 完美中文宽字符灰色对齐（v3.1）
- 重连过程中 mount 层主动重启（看 Phase 35 真机结果回流）
- `sessions ls --remote-host`（v3.1）
- PTY 真正"下次回车前"prompt 上方插入（沿用 Phase 31 D-28）

---

*Discussion logged: 2026-04-20*
*Mode: --auto（推荐项自动选定，候选方案与判断依据保留供审计）*
