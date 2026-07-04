---
phase: 32-ssh-tmux
verified: 2026-04-20T11:00:00Z
status: passed
score: 8/12 must-haves verified (code-level), 4 awaiting human UAT
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 6/12
  gaps_closed:
    - "Gap #1 — SC5 / REQ-F3-B：BufferedStdin 与 Reconnector 共享 atomic.Int32 state（reconnector.StateAddr()）+ onReconnected 回调 bs.Flush() ringBuf 按序回放 + echoMu 修 WR-04 race"
    - "Gap #2 — SC11 / REQ-F5-D：mount_strategy.MountWorkspace 真实调用 cfg.SyncSessionLock(cfg.ClaudeAccountID) + ErrSyncLocked 降级分支 + DowngradeStep{ReasonCode:\"sync_locked\"} + finalCleanup LIFO syncRelease 兜底"
  gaps_remaining: []
  regressions: []
gaps: []
deferred: []
human_verification:
  - test: "30s 网络抖动 UAT（SC2 / REQ-F4-A / BASE-03）"
    expected: "claudedock 进入 tmux 后运行 claude → `docker network disconnect <ctr> bridge` 持续 30s → `docker network connect <ctr> bridge` → claudedock 自动 reconnect 成功；tmux capture-pane 对比前后，claude 进程 PID 不变、scrollback buffer 完整、无丢字。"
    why_human: "TestIntegration_Phase32_NetworkDisconnect30s 已落框架但 t.Skip（docker network 权限 + 端到端 PTY 交互工程量超本 plan）；RESEARCH §Test Matrix 明确留 Phase 35 真机 UAT。Gap #1 闭合后 BufferedStdin Reconnecting 分支已可触发，但端到端"无丢字 / 无乱序"仍需真拔网验证。"
  - test: "pkill -SIGHUP sshd 后 tmux 存活 UAT（SC3 / C7）"
    expected: "`ssh container 'tmux new -d -s test; sleep 1; pkill -SIGHUP sshd'` 执行后，本端重新 ssh 进容器 `tmux attach -t test` 必须成功；session 窗口内容保留。"
    why_human: "integration_test.go TestIntegration_Phase32_TmuxSurvivesSighupSshd / TestIntegration_Phase32_PgrepNoSystemdLogind 需要实际 docker fixture（scripts/test-fixture-up.sh 需本地 docker）；CI 环境不保证权限，TestMain 会 os.Exit(0) 优雅 skip。手测链：run fixture → go test -tags=integration -run TestIntegration_Phase32_TmuxSurvives -v。"
  - test: "多端 banner 第二行 UAT（SC9 / REQ-F5-A / REQ-F5-B）"
    expected: "同一 shortID 容器内启两个 claudedock 进程：第二端 banner 必须含 `✓ 已 attach 到会话 claude-<id>`，紧跟第二行 `（另 1 个会话正在共享：<第一端 hostname> / 刚刚活跃）`；hostname 来自 /workspace/.claudedock/clients/<pid>.json 文件注册表的 hostname 字段，活跃时间来自 tmux list-clients client_activity。"
    why_human: "printAttachBanner + readClientHostnames 代码路径在 session.go 存在且单测覆盖纯函数层（parseTmuxListClients / formatBannerSecondLine / decideTakeOverClientCount），但 writeClientFile 是在容器内远程执行 printf '%s' > .json 的异步 goroutine；真端到端验收需要两个活 ssh session + 两个 tmux client + 注册表文件落盘时序 —— 属 docker UAT 范畴。"
  - test: "账号级单例锁 docker UAT（SC11 / REQ-F5-D 端到端）"
    expected: "**前提：Gap #2（mount_strategy 未调 SyncSessionLock）已闭合（本次 Plan 04）**，容器内同一 claude_account 启两个 claudedock：第一个 `docker exec` 容器 `ls /tmp/claudedock/locks/` 应见 `sync-<account>.lock`；`ps aux | grep 'sleep infinity'` 应只见 1 个；第二个 claudedock 启动时 stderr 必含 `[SESSION_SYNC_LOCKED]` + last-session.json `client_role=\"secondary\"` + mergerfs runtime branch 无 full 档位（仅 sshfs）。"
    why_human: "Gap #2 闭合（mount_strategy.MountWorkspace 已真实调 SyncSessionLock + ErrSyncLocked 降级 + DowngradeStep{ReasonCode:sync_locked} + finalCleanup LIFO 已落地，TestMountWorkspace_SyncLocked + SyncLockSuccess + SyncLockOtherError 三测试 PASS），**前提条件就绪**；剩余只是端到端多端 claudedock 启动（含 mount_strategy 降级 + last-session 写入 + sshfs-only 视图）需要完整 docker 环境真机 UAT。"
  - test: "sshfs 抖动 30s 不挂死 UAT（SC12 / C3）"
    expected: "启动 claudedock 进 full 档位 → 容器内 `docker network disconnect` 30s → ls /workspace 不 hang；sshfs_watcher 在 15s 内把 cold branch 从 mergerfs 摘除（MOUNT_SSHFS_DISCONNECTED 警告可见）；网络恢复后 mergerfs 自动接回或手动重挂成功。"
    why_human: "sshfs_watcher 在 Phase 31 ship，Phase 32 只做联合验收；需要完整挂载链（sshfs + mergerfs + mutagen）+ docker network 权限。"
---

# Phase 32: SSH-Tmux Verification Report (Re-Verification)

**Phase Goal:** 把 SSH 会话从"断线重来"升级为"30s 抖动无感知 + 进程不丢 + 多端共享 attach"；交付 F3（SSH 弱网容忍 + 自动重连）+ F4（tmux 默认包装 + 会话恢复）+ F5（多端同账号 attach），并在容器侧账号粒度强制 Mutagen 单例锁；必须防御 C3（sshfs 抖动级联）+ C7（systemd-logind 杀 tmux）。

**Verified:** 2026-04-20T11:00:00Z
**Status:** `passed`（code-level；5 项 docker UAT 留 Phase 35 真机）
**Re-verification:** **Yes** — 第二次验证，前次（2026-04-20T00:00:00Z）`gaps_found` 6/12，本次确认两个 gap 已闭合。

---

## Re-Verification Summary

| 项                | 前次 (init)              | 本次 (re-verify)                                                                       |
| ----------------- | ----------------------- | -------------------------------------------------------------------------------------- |
| status            | `gaps_found`            | **`passed`**                                                                           |
| code-level score  | 6/12                    | **8/12**（新增 SC5 + SC11，SC9 仍按 code-level VERIFIED 计在 8 中之外的"已就绪部分"内） |
| Gap #1 (SC5)      | ✗ FAILED                | **✓ VERIFIED** — `reconnector.StateAddr()` 共享 + `bs.Flush()` onReconnected 回调      |
| Gap #2 (SC11)     | ✗ FAILED                | **✓ VERIFIED** — `cfg.SyncSessionLock(cfg.ClaudeAccountID)` invoke + ErrSyncLocked 降级 |
| Required Artifacts ⚠️ ORPHANED | 2（input_buffer / sync_lock） | **0** — 全部 ✓ WIRED                                                                  |
| Key Links ✗ NOT_WIRED | 2 行                     | **0** — 全部 ✓ WIRED                                                                  |
| Regressions       | n/a                     | **0** — 原 ✓ 项全部保持，新加 echoMu 锁 + Reconnector 单例化通过 -race 全测              |
| Human verification| 5 项                     | 5 项（SC11 docker UAT 项追加 "Gap #2 已闭合，前提条件就绪" 注记）                        |

---

## Goal Achievement

### Observable Truths — 12 Success Criteria

| #   | Truth (SC)                                                | Status (re-verify) | Evidence                                                                                                                                                                                                                                  |
| --- | --------------------------------------------------------- | ------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | SC1  — `ServerAliveInterval < 15s` 启动失败 + 明确错误       | ✓ VERIFIED         | `cmd/claudedock/main.go:363-366` 启动期校验 + errcodes session.go SESSION_KEEPALIVE_TOO_AGGRESSIVE Fatal Severity；`keepalive.go::RunKeepAlive` 入口 defensive check。无 regression。                                                  |
| 2   | SC2  — 断网 30s 内 claudedock 不退出 / claude 不丢 / tmux 可 attach      | ? UNCERTAIN        | 代码路径 = tmux new-session -A 包装 + RunKeepAlive + Reconnector 单例（Gap #1 闭合后 BufferedStdin 真实接入）；端到端"进程不丢 + buffer 完整"仍需 docker network disconnect UAT。见 human_verification#1。                          |
| 3   | SC3  — `pkill -SIGHUP sshd` 后 `tmux attach -t test` 成功        | ? UNCERTAIN        | `TestIntegration_Phase32_TmuxSurvivesSighupSshd` + `TestIntegration_Phase32_PgrepNoSystemdLogind` 代码就位；需实际 docker fixture 跑 `-tags=integration`。见 human_verification#2。                                            |
| 4   | SC4  — 退避序列 `[1s,2s,4s,8s,30s]` + 不重新弹密码                    | ✓ VERIFIED         | `reconnect.go:30-37` `var backoffSeq = []time.Duration{1s,2s,4s,8s,30s}`；`TestBackoffSeq` 单测逐元素断言。无 regression。                                                                                                              |
| 5   | SC5  — 断网灰色未确认 echo + 重连后按序提交 / 无丢字 / 无乱序        | **✓ VERIFIED** (上次 ✗ FAILED) | **Gap #1 闭合**：（a）`session.go:646` `bs, bufferedPipeR = NewBufferedStdin(os.Stdin, reconnector.StateAddr(), ...)` —— BufferedStdin 与 Reconnector 共享同一 `*atomic.Int32` 指针；（b）`session.go:641` onReconnected 回调内 `_ = bs.Flush()` —— ringBuf 按序回放；（c）`session.go` 已删除局部 `var state atomic.Int32`（grep = 0 hits）；（d）`input_buffer.go` 新增 `echoMu sync.Mutex` 修 WR-04 race；（e）`session_test.go:373 TestPTYReconnect_BufferedInputFlush` 6 条断言（ansiGray + 原始字符 + ringBuf "abc" + Flush 后 pipeR 读到 "abc" + ringBuf 清空 + ansiReset）全 PASS（含 -race）。 |
| 6   | SC6  — 重连失败 prompt 两要素（原因 + 下一步）                         | ✓ VERIFIED         | `errcodes/net.go:33-46` NET_RECONNECT_GAVE_UP Message + NextAction 已注册 + `reconnect.go::FormatGiveUpMessage` 调用；无 regression。                                                                                                |
| 7   | SC7  — 容器内 tmux 不可用：不阻塞启动 + banner `[!] 容器内 tmux 不可用...`   | ✓ VERIFIED         | `ssh.go::ConnectAndRunClaudeV3` 在 OAuth 检查后 `DetectTmux(connA)`；false 分支 `errcodes.Format(SESSION_TMUX_UNAVAILABLE, reason)` + 走 v2.0 裸路径，不 `os.Exit`。无 regression。                                                |
| 8   | SC8  — `claudedock sessions ls` / `attach <name>` 列表 + 切换        | ✓ VERIFIED         | `cmd/claudedock/sessions.go::newSessionsCmd` 注册 `ls` / `attach`；`session.go::RunSessionsLs/Attach` 实装。无 regression。                                                                                                          |
| 9   | SC9  — 多端默认共享 attach + 第二端 banner `（另 N 个会话正在共享）`          | ✓ VERIFIED (code) / ? HUMAN (UAT) | `printAttachBanner` + `readClientHostnames` + `writeClientFile` 全链路实装；`tmux new-session -A` 默认不踢人。端到端 "另 N 个会话共享" 文字输出需 docker 双端 UAT。无 regression。见 human_verification#3。                       |
| 10  | SC10 — `--new-session` 生成 `claude-<short_id>` / `--take-over` 通知其它端 | ✓ VERIFIED         | `main.go:245-275` 剥离 `--new-session`/`--take-over` + `GenerateShortSessionID()` 调用；`session.go::performTakeOver` 实现 list-clients/display-message/sleep 3/detach-client -a。无 regression。                                |
| 11  | SC11 — 账号级 Mutagen 单例锁（后连端只读 sshfs / mergerfs 视图）         | **✓ VERIFIED** (上次 ✗ FAILED) | **Gap #2 闭合**：（a）`mount_strategy.go:183` `release, lockErr := cfg.SyncSessionLock(cfg.ClaudeAccountID)` 真实调用（grep ≥1 PASS）；（b）`mount_strategy.go:184-198` `errors.Is(lockErr, ErrSyncLocked)` 命中 → `cfg.IsSecondaryClient = true` + `intended = ModeSSHFSOnly` + `DowngradeChain` 追加 `DowngradeStep{ReasonCode:"sync_locked", From: intended.String(), To: "sshfs-only"}` + 中文 [!] 摘要；（c）`mount_strategy.go:199-205` 其它 lockErr 透传 ModeFailed（M13 防御）；（d）`mount_strategy.go:241-248` 成功分支 finalCleanup LIFO 包装 syncRelease；（e）`mount_strategy_test.go:415/490/538` 三个新测试（SyncLocked / SyncLockSuccess / SyncLockOtherError）PASS；（f）现有 9 个 mount_strategy 测试零 regression。 |
| 12  | SC12 — sshfs 抖动 30s 后 mergerfs 不整体挂死（与 Phase 31 联合）          | ? UNCERTAIN        | sshfs_watcher 在 Phase 31 ship 不由本 phase 重写；`TestIntegration_Phase32_NetworkDisconnect30s` 仅落框架并 t.Skip；需 docker + 真拔网 UAT。见 human_verification#5。                                                                  |

**Score:**
- **Code-level VERIFIED：8** — SC1, SC4, SC5（新闭）, SC6, SC7, SC8, SC10, SC11（新闭）
- **Code-level VERIFIED + UAT 待验：1** — SC9（code 部分 ✓；端到端文字落盘 UAT 留 docker）
- **? UNCERTAIN（需 human UAT）：4** — SC2, SC3, SC9 端到端部分, SC12（外加 SC11 的 docker 端到端 UAT 项也保留在 human_verification#4）
- **FAILED：0** ✅
- **Regressions：0** ✅

### Deferred Items

无。所有未通过 code-level 项均属 docker UAT 性质，由 human_verification 兜底；未匹配"later milestone phase 吸收"判据。

---

### Required Artifacts (per-plan must_haves artifacts)

| Artifact                                                    | Plan    | Status (re-verify) | Details                                                                                                                      |
| ----------------------------------------------------------- | ------- | ------------------ | ---------------------------------------------------------------------------------------------------------------------------- |
| `internal/claudedock/keepalive.go` + 三平台 build-tag       | 01      | ✓ VERIFIED         | RunKeepAlive + sendKeepaliveWithTimeout + ConfigureTCPKeepAlive；三平台 build-tag 文件就位。                                  |
| `internal/claudedock/reconnect.go`                         | 01      | ✓ VERIFIED         | backoffSeq / Reconnector / renderDisconnectStatus / FormatGiveUpMessage / **StateAddr() getter 现已被 session.go 实际调用**。 |
| `internal/claudedock/input_buffer.go`                      | 01 / 05 | **✓ VERIFIED** (上次 ⚠️ ORPHANED) | **Gap #1 闭合**：runClaudePTYWithReconnect 外层共享 reconnector.StateAddr() 进 NewBufferedStdin；Plan 05 新增 `echoMu sync.Mutex` 修 WR-04 race；TestPTYReconnect_BufferedInputFlush 端到端 PASS。 |
| `internal/claudedock/errcodes/{codes,session,net}.go`      | 01      | ✓ VERIFIED         | 10 条新码全部注册，codes_test.go PASS。                                                                                       |
| `internal/claudedock/last_session.go` TmuxSession/ClientRole/ReconnectCount | 01 | ✓ VERIFIED   | 3 字段 omitempty；round-trip 测试 PASS。                                                                                     |
| `internal/claudedock/session.go`                           | 02 / 05 | ✓ VERIFIED         | runClaudePTYWithReconnect 重构为外层 Reconnector + BufferedStdin 单例（line 610-693）；pTYAttachOnce 新增 bufferedPipeR io.Reader 参数；删除 sync/atomic import；DetectTmux / buildTmuxSessionName / writeClientFile / performTakeOver / RunSessionsLs 等全部保持 ✓。 |
| `cmd/claudedock/sessions.go`                              | 02      | ✓ VERIFIED         | newSessionsCmd + ls / attach + connectForSessions。                                                                           |
| `internal/claudedock/sync_lock.go`                         | 03 / 04 | **✓ VERIFIED** (上次 ⚠️ ORPHANED) | **Gap #2 闭合**：AcquireSyncLock 现由 mount_strategy.MountWorkspace 通过 cfg.SyncSessionLock 闭包真实调用；生产路径锁实际生效。 |
| `internal/claudedock/mount_strategy.go`                    | 04      | ✓ VERIFIED (Plan 04 新增) | line 183 `cfg.SyncSessionLock(cfg.ClaudeAccountID)` invoke + line 184-198 ErrSyncLocked 降级分支 + line 199-205 错误透传 + line 241-248 finalCleanup LIFO；3 个新测试 PASS，现有 9 测试零 regression。 |
| `internal/claudedock/integration_test.go` 6 个 TestIntegration_Phase32_* | 03  | ✓ VERIFIED (build) / ? HUMAN (run) | 6 个用例函数名就位，`go build -tags=integration` PASS；执行需 docker fixture。                                                |

---

### Key Link Verification

| From                                  | To                                    | Via                                                              | Status (re-verify) | Details                                                                                                                                           |
| ------------------------------------- | ------------------------------------- | ---------------------------------------------------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ssh.go::sshConnect`                  | `keepalive.go::ConfigureTCPKeepAlive` | 拨号成功后立即调用，best-effort                                    | ✓ WIRED            | grep: `ConfigureTCPKeepAlive(tc, 15*time.Second)` 在 sshConnect 内调用。无 regression。                                                            |
| `reconnect.go::Reconnector`           | `ssh.go::sshConnect`                  | 复用 SSHConfig 重新拨号                                            | ✓ WIRED            | Reconnector.Run 内 `sshConnect(r.cfg)` — r.cfg 含启动期缓存 Password。无 regression。                                                              |
| `input_buffer.go::BufferedStdin`      | `reconnect.go::Reconnector`           | 共享 `*atomic.Int32` state 指针 via reconnector.StateAddr()         | **✓ WIRED** (上次 ✗ NOT_WIRED) | **Gap #1 闭合**：`session.go:646` `bs, bufferedPipeR = NewBufferedStdin(os.Stdin, reconnector.StateAddr(), os.Stderr, sessionCfg.NoColor, reconnector.Trigger)` —— 共享指针就位；`session.go:641` onReconnected 回调 `_ = bs.Flush()` 按序回放。验证：`rg "reconnector\.StateAddr\(\)" session.go` = 2 hits（注释 + 调用），`rg "bs\.Flush\(\)" session.go` = 2 hits，`rg "^var state atomic\.Int32" session.go` = 0 hits。 |
| `ssh.go::ConnectAndRunClaudeV3`       | `session.go::DetectTmux`              | OAuth 检查后调用，结果决定走 runClaudeWithSession 或 runClaude      | ✓ WIRED            | DetectTmux 调用可见；无 regression。                                                                                                              |
| `cmd/claudedock/sessions.go`        | `internal/claudedock/RunSessionsLs/Attach` | cobra RunE → 业务 helper                                           | ✓ WIRED            | runSessionsLs / runSessionsAttach 委托。无 regression。                                                                                            |
| `ssh.go::ConnectAndRunClaudeV3`       | `sync_lock.go::AcquireSyncLock`       | MountWorkspace 调用之前把闭包注入 mountCfg.SyncSessionLock         | ✓ WIRED            | ssh.go:95-109 闭包赋值（保持原状）。                                                                                                              |
| `mount_strategy.go::MountWorkspace`   | `mountCfg.SyncSessionLock` 闭包        | mountMutagen 之前调用拿锁，失败 errors.Is(err, ErrSyncLocked) 降级 | **✓ WIRED** (上次 ✗ NOT_WIRED) | **Gap #2 闭合**：`mount_strategy.go:183` `release, lockErr := cfg.SyncSessionLock(cfg.ClaudeAccountID)`；`mount_strategy.go:184` `errors.Is(lockErr, ErrSyncLocked)`；`mount_strategy.go:191` `ReasonCode: "sync_locked"`；`mount_strategy.go:185` `cfg.IsSecondaryClient = true`。验证：`rg "cfg\.SyncSessionLock\(" mount_strategy.go` = 1 hit，`rg "sync_locked" mount_strategy.go` = 3 hits（ReasonCode + 注释 + Fprintf 模板）。 |
| `session.go::writeLastSessionTmuxField` | `last_session.go::ClientRole`       | role 由 SessionConfig.IsSecondaryClient 决定 primary/secondary     | ✓ WIRED            | `session.go:587-590` `role := "primary"; if sessionCfg.IsSecondaryClient { role = "secondary" }`；现 IsSecondaryClient 由 ssh.go 闭包（捕获 mountCfg 指针）+ mount_strategy.go:185（双保险）置位 —— wiring 全链路通。 |

---

### Data-Flow Trace (Level 4)

| Artifact / 数据目标                             | Data Variable         | Source                                                  | Produces Real Data  | Status (re-verify) |
| ----------------------------------------------- | --------------------- | ------------------------------------------------------- | ------------------- | ------------------ |
| BufferedStdin.ringBuf → SSH session.Stdin (SC5) | ringBuf bytes         | runClaudePTYWithReconnect 外层 reconnector.StateAddr() 共享 *atomic.Int32 | **是**（Reconnector.Run 写 StateReconnecting → bs.Run 读到 → 切 ringBuf + 灰色 echo 分支；onReconnected 回调 bs.Flush 按序回写 pipeW）| **✓ FLOWING** (上次 ✗ DISCONNECTED) |
| mount_strategy 降级路径 → last-session client_role=secondary (SC11) | IsSecondaryClient     | ssh.go 闭包（捕获 mountCfg 指针）在 ErrSyncLocked 时置位 + mount_strategy.go:185 双保险 | **是**（cfg.SyncSessionLock 真实调用 → ErrSyncLocked → IsSecondaryClient=true 透传到 SessionConfig → writeLastSessionTmuxField client_role="secondary"） | **✓ FLOWING** (上次 ✗ DISCONNECTED) |
| banner 第二行 hostname (SC9)                    | readClientHostnames map | /workspace/.claudedock/clients/<pid>.json 文件注册表 | 是（writeClientFile 写入 + readClientHostnames 读）  | ✓ FLOWING (代码路径) — 端到端需 docker UAT 验证实际值落盘 |
| Reconnector renderDisconnectStatus (SC4 UX)     | disconnectStart       | Reconnector.Run 入口 `r.disconnectStart.Store(time.Now().UnixNano())` | 是                   | ✓ FLOWING          |
| keepalive 拦截 io.EOF → reconnect 路径 (SC2 底层) | conn.Close() 发起     | RunKeepAlive 连续 countMax 次超时 → conn.Close()        | 是                   | ✓ FLOWING          |

---

### Behavioral Spot-Checks

| Behavior                                                              | Command                                                                                   | Result                     | Status   |
| --------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- | -------------------------- | -------- |
| 全工程 build PASS                                                       | `go build ./...`                                                                          | exit 0，无输出              | ✓ PASS   |
| 单元测试 + race 全套 PASS（含 Plan 04 + Plan 05 新测）                    | `go test -race -short -timeout 180s ./internal/claudedock/...`                            | `ok internal/claudedock 2.143s` / `ok internal/claudedock/errcodes 1.792s` | ✓ PASS |
| Gap #2 invoke 实测                                                     | `rg 'cfg\.SyncSessionLock\(' internal/claudedock/mount_strategy.go`                      | 1 hit (line 183)           | ✓ PASS   |
| Gap #1 共享 atomic 实测                                                 | `rg 'reconnector\.StateAddr\(\)' internal/claudedock/session.go`                         | 2 hits (line 620 注释 + line 646 调用) | ✓ PASS |
| Gap #1 Flush 回调实测                                                   | `rg 'bs\.Flush\(\)' internal/claudedock/session.go`                                      | 2 hits (line 624 注释 + line 641 调用) | ✓ PASS |
| Gap #1 局部 atomic 已删除                                                | `rg '^var state atomic\.Int32' internal/claudedock/session.go`                           | 0 hits ✅                   | ✓ PASS   |
| WR-04 echoMu 已落地                                                     | `rg 'echoMu' internal/claudedock/input_buffer.go`                                        | 14 hits（field + Lock/Unlock 跨 Run-Connected / handleReconnecting / Flush / closeGrayIfOpen 多路径） | ✓ PASS |
| Gap #2 测试就位                                                         | `rg 'TestMountWorkspace_SyncLocked' internal/claudedock/mount_strategy_test.go`          | 2 hits (注释 + func 定义 line 415) | ✓ PASS |
| Gap #1 集成测试就位                                                     | `rg 'TestPTYReconnect_BufferedInputFlush' internal/claudedock/session_test.go`           | 2 hits (注释 + func 定义 line 373) | ✓ PASS |
| Gap #2 DowngradeStep ReasonCode                                       | `rg 'sync_locked' internal/claudedock/mount_strategy.go`                                 | 3 hits (line 173 注释 + line 191 ReasonCode + line 195 Fprintf 模板) | ✓ PASS |
| 30s 抖动 / pkill sshd / 多端 banner / docker 锁 UAT                     | `go test -tags=integration -run TestIntegration_Phase32_NetworkDisconnect30s ...`         | t.Skip（无 docker / short） | ? SKIP → human_verification |

---

### Requirements Coverage

| Requirement                 | Source Plan | Description (from REQUIREMENTS.md)                                                         | Status (re-verify) | Evidence                                                                                      |
| --------------------------- | ----------- | ------------------------------------------------------------------------------------------ | ------------------ | --------------------------------------------------------------------------------------------- |
| REQ-F3-A                    | 01          | SSH KeepAlive 间隔 < 15s 启动失败                                                            | ✓ SATISFIED        | SC1                                                                                           |
| REQ-F3-B                    | 01 / 05     | 断网期间本地键入灰色未确认样式，重连后按序提交                                                  | **✓ SATISFIED** (上次 ✗ BLOCKED) | SC5 — Plan 05 重构生效，BufferedStdin 与 Reconnector 共享 atomic + onReconnected Flush ringBuf |
| REQ-F3-C                    | 01          | 重连失败 prompt 必须显示原因 + 下一步操作                                                      | ✓ SATISFIED        | SC6                                                                                           |
| REQ-F3-D                    | 01          | 退避 1s→2s→4s→8s→30s + 不弹密码                                                              | ✓ SATISFIED        | SC4                                                                                           |
| REQ-F4-A                    | 02          | 30s 抖动内 tmux / claude 进程不丢                                                            | ? NEEDS HUMAN      | SC2 UAT                                                                                       |
| REQ-F4-B                    | 02          | `claudedock sessions ls / attach <name>`                                                 | ✓ SATISFIED        | SC8                                                                                           |
| REQ-F4-C                    | 02          | 容器内 tmux 不可用场景降级不阻塞启动                                                          | ✓ SATISFIED        | SC7                                                                                           |
| REQ-F5-A                    | 02          | 多端默认共享 attach（不踢人）                                                                | ✓ SATISFIED (code) | SC9 — `tmux new-session -A` + printAttachBanner                                               |
| REQ-F5-B                    | 02          | 第二端 banner 显示其它会话 hostname + 活跃时间                                                | ✓ SATISFIED (code) / ? HUMAN (UAT) | SC9 端到端文字渲染需 docker UAT                                                              |
| REQ-F5-C                    | 02          | `--new-session` / `--take-over`                                                            | ✓ SATISFIED        | SC10                                                                                          |
| REQ-F5-D                    | 03 / 04     | 账号级 Mutagen 单例锁                                                                        | **✓ SATISFIED** (上次 ✗ BLOCKED) | SC11 — Plan 04 闭合 mount_strategy.MountWorkspace SyncSessionLock invoke                     |

无 orphaned requirement：Phase 32 REQUIREMENTS.md 绑定的 11 条 REQ-F3/F4/F5-* 全部出现在 5 个 plan 的 `requirements:` frontmatter 中（含 gap-closure Plan 04/05）。

---

### Anti-Patterns Found

本次 verifier 未独立扫描代码（Phase 32 已有 32-REVIEW.md 覆盖 5 Warning + 7 Info）。Plan 05 已 co-fix WR-03（多 goroutine 读 os.Stdin）+ WR-04（grayOpen race），降级如下：

| File                                             | Line        | Pattern                                      | Severity (re-verify) | Impact                                                                                                           |
| ------------------------------------------------ | ----------- | -------------------------------------------- | -------------------- | ---------------------------------------------------------------------------------------------------------------- |
| internal/claudedock/session.go                  | 703-712     | SIGWINCH goroutine 泄漏（channel 不 close）     | ⚠️ Warning（保留）    | 每次 reconnect +1 泄漏；不直接 block 任何 SC，但长时间重连会累积。归 REVIEW.md WR-01。Phase 33+ 处置。               |
| internal/claudedock/session.go                  | 614-764     | `*registryPid` data race                       | ⚠️ Warning（保留）    | 极端 race 时可能漏 removeClientFile → 孤儿 registry entry。WR-02。Phase 33+ 处置。                                |
| internal/claudedock/session.go + input_buffer.go | 多处         | 多个 BufferedStdin.Run goroutine 并发读 os.Stdin | **✓ Resolved**（Plan 05 闭合） | Plan 05 把 BufferedStdin 提升为单例，bs.Run 单 goroutine 跨所有 attach 周期；旧 WR-03 根因消除。                  |
| internal/claudedock/input_buffer.go             | 90-141      | grayOpen / localEcho 无 mutex 跨 goroutine    | **✓ Resolved**（Plan 05 闭合） | Plan 05 新增 echoMu 锁；锁顺序文档化（echoMu 外 / ringMu 内，Flush 唯一嵌套点）；race 测试 PASS。                |
| internal/claudedock/session.go                  | 200-209     | `\|\| exec <wrapCmd字面值>` fallback 对 cd builtin 失败 | ⚠️ Warning（保留）    | fallback 路径不可用；主路径 `command -v tmux` 通过时无感。WR-05。Phase 33+ 处置。                                  |
| internal/claudedock/sync_lock.go                | 63          | lockPath 未校验 accountID 路径穿越             | ℹ️ Info               | 深度防御建议。IN-01。                                                                                            |
| internal/claudedock/last_session.go             | 67-74       | WriteLastSession 固定 .tmp 后缀               | ℹ️ Info               | IN-02。                                                                                                          |
| internal/claudedock/keepalive.go                | 51-67       | `time.After` 不可取消                         | ℹ️ Info               | IN-03。                                                                                                          |
| internal/claudedock/session.go                  | 671-673     | `pTYAttachOnce` `(int, error, error)` 两 error 易混淆 | ℹ️ Info       | IN-04。                                                                                                          |
| internal/claudedock/session.go                  | 881-883     | RunSessionsAttach 用 ExitConfigError=4 表示 session not found | ℹ️ Info | IN-05。                                                                                                          |
| cmd/claudedock/sessions.go                     | 58-75       | os.Exit 前 defer 不执行                       | ℹ️ Info               | IN-06。                                                                                                          |
| internal/claudedock/errcodes/net.go             | 31-36       | NET_RECONNECT_BACKOFF 注册但未实际 Format 输出  | ℹ️ Info               | IN-07。                                                                                                          |

**REVIEW.md 5 个 Warning 状态：3 个 Warning（WR-01/02/05）保留，2 个 Warning（WR-03/04）被 Plan 05 顺带 co-fix Resolved。**

---

### Human Verification Required

见 YAML frontmatter `human_verification:` —— 5 项保持，但 SC11 docker UAT 项已追加 "Gap #2 已闭合，前提条件就绪" 注记：

1. **30s 网络抖动 UAT（SC2 / BASE-03）** — docker network disconnect/connect + tmux capture-pane 前后对比；Phase 35 真机兜底。Gap #1 闭合后 BufferedStdin 已可触发 Reconnecting 分支，但端到端"无丢字 / 无乱序"仍需真拔网验证。
2. **pkill -SIGHUP sshd 存活 UAT（SC3 / C7）** — docker fixture 跑 `TestIntegration_Phase32_TmuxSurvivesSighupSshd` + `TestIntegration_Phase32_PgrepNoSystemdLogind`。
3. **多端 banner 第二行 UAT（SC9 / REQ-F5-B）** — 同一 shortID 两端 claudedock，第二端 stderr 必须含 `（另 1 个会话正在共享：<hostname> / 刚刚活跃）`。
4. **账号级单例锁端到端 UAT（SC11 / REQ-F5-D）** — **Gap #2 已闭合（Plan 04），前提条件就绪** — 后两端启动 + ls /tmp lockfile + ps sleep infinity + stderr SESSION_SYNC_LOCKED + last-session secondary。
5. **sshfs 抖动 30s 不挂死 UAT（SC12 / C3）** — 完整挂载链 + docker network 权限 + mergerfs runtime branch 观察。

---

### Gap Closure Detail（本次 re-verify 重点）

#### Gap #1 — SC5 (REQ-F3-B) BufferedStdin Reconnect Wiring（Plan 05 闭合）

**前次诊断**：`session.go::pTYAttachOnce:724-726` 使用独立局部 `var state atomic.Int32`（恒为 StateConnected），Plan 01 的 BufferedStdin Reconnecting 分支永不激活。

**本次验证（Plan 05 commit 9aa1bd3 + 12a479c + 7ca1fea）**：

| 断言                                         | 命令                                                                       | 期望 | 实测                          | 结果   |
| -------------------------------------------- | -------------------------------------------------------------------------- | ---- | ----------------------------- | ------ |
| 局部 atomic.Int32 已删除                       | `rg '^var state atomic\.Int32' session.go`                                 | 0    | 0                             | ✓ PASS |
| BufferedStdin 与 Reconnector 共享 atomic 指针 | `rg 'reconnector\.StateAddr\(\)' session.go`                               | ≥ 1  | 2 (line 620 注释 + line 646 调用) | ✓ PASS |
| onReconnected 回调内 Flush                    | `rg 'bs\.Flush\(\)' session.go`                                            | ≥ 1  | 2 (line 624 注释 + line 641 调用) | ✓ PASS |
| WR-04 echoMu 落地                              | `rg 'echoMu' input_buffer.go`                                              | ≥ 1  | 14 (field + Lock/Unlock 多路径) | ✓ PASS |
| 集成测试就位                                  | `rg 'TestPTYReconnect_BufferedInputFlush' session_test.go`                 | ≥ 1  | 2 (注释 + func)               | ✓ PASS |
| 集成测试 + race 测试 PASS                       | `go test -race -short ./internal/claudedock/...`                          | ok   | ok 2.143s                     | ✓ PASS |

**端到端机制确认**（session.go:617-693 重构后逻辑）：
```
runClaudePTYWithReconnect 外层（仅一次）：
  reconnector := NewReconnector(sshCfg, nil, onReconnected=func(c){ pendingNewConn=c; bs.Flush() }, ...)
  bs, bufferedPipeR := NewBufferedStdin(os.Stdin, reconnector.StateAddr(), ...)
  go bs.Run(bsCtx)         ← 单 goroutine 跨所有 attach（WR-03 co-fix）
  defer bs.Close()

循环 iter：
  pTYAttachOnce(conn, ..., bufferedPipeR):
    session.Stdin = bufferedPipeR     ← 共享外层 atomic
  reconnector.Run(ctx):
    state.Store(StateReconnecting)    ← bs 立即看到（共享指针！）→ ringBuf + 灰色 echo
    sshConnect retry（退避 1/2/4/8/30s）
    onReconnected → pendingNewConn=newConn; bs.Flush()  ← ringBuf 按序写 pipeW
    state.Store(StateConnected)
  conn = pendingNewConn → continue
```

**结论：Gap #1 ✓ 闭合，SC5 / REQ-F3-B 从 ✗ FAILED 转 ✓ VERIFIED（code-level）**。

#### Gap #2 — SC11 (REQ-F5-D) Mount-Strategy SyncSessionLock Invoke（Plan 04 闭合）

**前次诊断**：`mount_strategy.MountWorkspace` 函数体内从未调用 `mountCfg.SyncSessionLock(accountID)`，Phase 31 D-31 仅在 MountConfig 上预留字段，落地调用缺失。

**本次验证（Plan 04 commit d425264）**：

| 断言                                              | 命令                                                                       | 期望 | 实测                          | 结果   |
| ------------------------------------------------- | -------------------------------------------------------------------------- | ---- | ----------------------------- | ------ |
| MountWorkspace 真实 invoke                         | `rg 'cfg\.SyncSessionLock\(' mount_strategy.go`                            | ≥ 1  | 1 (line 183)                  | ✓ PASS |
| ErrSyncLocked sentinel 命中                        | `rg 'errors\.Is\(.*ErrSyncLocked\)' mount_strategy.go`                     | ≥ 1  | 1                             | ✓ PASS |
| DowngradeStep ReasonCode = sync_locked             | `rg 'sync_locked' mount_strategy.go`                                       | ≥ 1  | 3 (line 173 注释 + line 191 ReasonCode + line 195 Fprintf 模板) | ✓ PASS |
| IsSecondaryClient 双保险置位                       | `rg 'IsSecondaryClient\s*=\s*true' mount_strategy.go`                      | ≥ 1  | 1 (line 185)                  | ✓ PASS |
| 测试 TestMountWorkspace_SyncLocked 就位             | `rg 'TestMountWorkspace_SyncLocked' mount_strategy_test.go`                | ≥ 1  | 2 (注释 + func line 415)       | ✓ PASS |
| 三个新测试 + 现有 9 测试零 regression                | `go test -short ./internal/claudedock/...`                                | PASS | PASS                          | ✓ PASS |

**端到端机制确认**（mount_strategy.go:182-205 + 240-248 重构后逻辑）：
```go
var syncRelease func()
if cfg.SyncSessionLock != nil {                                        // nil-guard 兼容现网测试零改动
    release, lockErr := cfg.SyncSessionLock(cfg.ClaudeAccountID)
    if errors.Is(lockErr, ErrSyncLocked) {                              // 第二端命中
        cfg.IsSecondaryClient = true                                    // 双保险
        if intended != ModeSSHFSOnly {
            snapshot.DowngradeChain = append(snapshot.DowngradeChain, DowngradeStep{
                From:          intended.String(),
                To:            ModeSSHFSOnly.String(),
                ReasonCode:    "sync_locked",
                ReasonMessage: "账号级 Mutagen 单例锁被另一端占用",
            })
            fmt.Fprintf(cfg.Logger, "[!] 账号级 Mutagen 单例锁已被另一端占用（%s → %s，原因: sync_locked）\n", ...)
            intended = ModeSSHFSOnly
        }
    } else if lockErr != nil {                                          // M13 防御：错误透传
        snapshot.ActualMode = ModeFailed.String()
        return func() {}, ModeFailed, fmt.Errorf("sync lock acquire: %w", lockErr)
    } else if release != nil {                                          // 成功分支
        syncRelease = release
    }
}
// ... tryOrder + tryMode ...
finalCleanup := modeCleanup
if syncRelease != nil {
    finalCleanup = func() {
        modeCleanup()                                                   // LIFO 内层先卸 mount 全栈
        syncRelease()                                                   // 再释放 sync 锁
    }
}
```

**结论：Gap #2 ✓ 闭合，SC11 / REQ-F5-D 从 ✗ FAILED 转 ✓ VERIFIED（code-level）**。

---

### Final Verdict

**Status: `passed`（code-level）**

| Dimension                       | Score / 状态                                                            |
| ------------------------------- | ----------------------------------------------------------------------- |
| Code-level VERIFIED truths      | **8 / 12**（含两个 gap-closure：SC5 + SC11）                              |
| Code-level VERIFIED + UAT 待验    | 1 SC9（code 部分通过 + 端到端 UAT 留 docker）                               |
| ? UNCERTAIN（human UAT 兜底）     | 4（SC2 / SC3 / SC9 端到端 / SC12 + SC11 docker 端到端）                    |
| Code-level FAILED                | **0** ✅                                                                 |
| Regressions                     | **0** ✅（含 -race 测试 PASS）                                            |
| Required Artifacts ⚠️ ORPHANED   | **0**（前次 2 项 input_buffer / sync_lock 全部 ✓ WIRED）                  |
| Key Links ✗ NOT_WIRED            | **0**（前次 2 行全部 ✓ WIRED）                                            |
| Anti-Patterns                    | 3 Warning（WR-01/02/05）保留 / 2 Warning（WR-03/04）Resolved（Plan 05 co-fix） / 7 Info 保留 |

**含义**：
- **Phase 32 phase goal 在 code-level 完整闭环**：F3（弱网容忍 + 自动重连）+ F4（tmux 包装 + 会话恢复）+ F5（多端共享 attach）+ 账号级单例锁全部代码路径生效。
- **5 项 docker UAT 留 Phase 35 真机**：human_verification 不影响本 phase code-level passed 判定，但 phase 在生产开服前需在 Phase 35 docker fixture 上跑端到端确认。
- **零 regression**：Plan 04 + Plan 05 改动不影响 Phase 31/32 之前 ship 的任何 ✓ 项；含 -race 全测 PASS。
- **下一步建议**：可直接进入下一 phase（Phase 33）；docker UAT 5 项排队在 Phase 35 真机闭环。

---

_Re-verified: 2026-04-20T11:00:00Z_
_Verifier: Claude (gsd-verifier)_
_Previous: 2026-04-20T00:00:00Z (gaps_found, 6/12)_
_Gap closures: Plan 04 (commit `d425264`) + Plan 05 (commits `9aa1bd3` + `12a479c` + `7ca1fea`)_
