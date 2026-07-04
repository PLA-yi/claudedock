---
status: partial
phase: 32-ssh-tmux
source: [32-VERIFICATION.md]
started: 2026-04-20T11:05:00Z
updated: 2026-04-20T11:05:00Z
---

## Current Test

[awaiting human testing — 5 项 docker UAT 留 Phase 35 真机环境一并执行]

## Tests

### 1. 30s 网络抖动 UAT（SC2 / REQ-F4-A / BASE-03）

expected: claudedock 进入 tmux 后运行 claude → `docker network disconnect <ctr> bridge` 持续 30s → `docker network connect <ctr> bridge` → claudedock 自动 reconnect 成功；tmux capture-pane 对比前后，claude 进程 PID 不变、scrollback buffer 完整、无丢字。
result: [pending]
why_human: TestIntegration_Phase32_NetworkDisconnect30s 已落框架但 t.Skip（docker network 权限 + 端到端 PTY 交互工程量超本 plan）；RESEARCH §Test Matrix 明确留 Phase 35 真机 UAT。Gap #1 闭合后 BufferedStdin Reconnecting 分支已可触发，但端到端"无丢字 / 无乱序"仍需真拔网验证。

### 2. pkill -SIGHUP sshd 后 tmux 存活 UAT（SC3 / C7）

expected: `ssh container 'tmux new -d -s test; sleep 1; pkill -SIGHUP sshd'` 执行后，本端重新 ssh 进容器 `tmux attach -t test` 必须成功；session 窗口内容保留。
result: [pending]
why_human: integration_test.go TestIntegration_Phase32_TmuxSurvivesSighupSshd / TestIntegration_Phase32_PgrepNoSystemdLogind 需要实际 docker fixture（scripts/test-fixture-up.sh 需本地 docker）；CI 环境不保证权限，TestMain 会 os.Exit(0) 优雅 skip。手测链：run fixture → go test -tags=integration -run TestIntegration_Phase32_TmuxSurvives -v。

### 3. 多端 banner 第二行 UAT（SC9 / REQ-F5-A / REQ-F5-B）

expected: 同一 shortID 容器内启两个 claudedock 进程：第二端 banner 必须含 `✓ 已 attach 到会话 claude-<id>`，紧跟第二行 `（另 1 个会话正在共享：<第一端 hostname> / 刚刚活跃）`；hostname 来自 /workspace/.claudedock/clients/<pid>.json 文件注册表的 hostname 字段，活跃时间来自 tmux list-clients client_activity。
result: [pending]
why_human: printAttachBanner + readClientHostnames 代码路径在 session.go 存在且单测覆盖纯函数层（parseTmuxListClients / formatBannerSecondLine / decideTakeOverClientCount），但 writeClientFile 是在容器内远程执行 printf '%s' > .json 的异步 goroutine；真端到端验收需要两个活 ssh session + 两个 tmux client + 注册表文件落盘时序 —— 属 docker UAT 范畴。

### 4. 账号级单例锁 docker UAT（SC11 / REQ-F5-D 端到端）

expected: **前提：Gap #2（mount_strategy 未调 SyncSessionLock）已闭合（本次 Plan 04）**，容器内同一 claude_account 启两个 claudedock：第一个 `docker exec` 容器 `ls /tmp/claudedock/locks/` 应见 `sync-<account>.lock`；`ps aux | grep 'sleep infinity'` 应只见 1 个；第二个 claudedock 启动时 stderr 必含 `[SESSION_SYNC_LOCKED]` + last-session.json `client_role="secondary"` + mergerfs runtime branch 无 full 档位（仅 sshfs）。
result: [pending]
why_human: Gap #2 闭合（mount_strategy.MountWorkspace 已真实调 SyncSessionLock + ErrSyncLocked 降级 + DowngradeStep{ReasonCode:sync_locked} + finalCleanup LIFO 已落地，TestMountWorkspace_SyncLocked + SyncLockSuccess + SyncLockOtherError 三测试 PASS），**前提条件就绪**；剩余只是端到端多端 claudedock 启动（含 mount_strategy 降级 + last-session 写入 + sshfs-only 视图）需要完整 docker 环境真机 UAT。

### 5. sshfs 抖动 30s 不挂死 UAT（SC12 / C3）

expected: 启动 claudedock 进 full 档位 → 容器内 `docker network disconnect` 30s → ls /workspace 不 hang；sshfs_watcher 在 15s 内把 cold branch 从 mergerfs 摘除（MOUNT_SSHFS_DISCONNECTED 警告可见）；网络恢复后 mergerfs 自动接回或手动重挂成功。
result: [pending]
why_human: sshfs_watcher 在 Phase 31 ship，Phase 32 只做联合验收；需要完整挂载链（sshfs + mergerfs + mutagen）+ docker network 权限。

## Summary

total: 5
passed: 0
issues: 0
pending: 5
skipped: 0
blocked: 0

## Gaps
