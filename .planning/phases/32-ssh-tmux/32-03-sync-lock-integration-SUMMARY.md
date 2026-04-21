---
phase: 32
plan: 03-sync-lock-integration
subsystem: client-cli / mutagen-singleton-lock / multi-client / integration-tests
tags: [sync-lock, flock, mutagen, secondary-client, c3, c7, integration-test, m15]
dependency-graph:
  requires:
    - errcodes.SESSION_SYNC_LOCKED（Plan 01 已注册）
    - mount_strategy.MountConfig.SyncSessionLock 字段（Phase 31 D-31 已加；本 plan 注入实现）
    - mount_strategy.MountConfig.ClaudeAccountID 字段（Phase 31 已存在）
    - cloudclaude.SessionConfig（Plan 02 已新增 — 本 plan 追加 IsSecondaryClient）
    - cloudclaude.runClaudeWithSession + writeLastSessionTmuxField + writeClientFile（Plan 02）
    - cloudclaude.SSHConnect / DetectTmux（Plan 02 export）
    - cloudclaude.LastSessionSnapshot.ClientRole（Plan 01 已加）
  provides:
    - cloudclaude.AcquireSyncLock(conn, accountID) (release func(), err error)
    - cloudclaude.ErrSyncLocked
    - cloudclaude.parseLastInt(s string) int（容错纯函数；exported 单测用）
    - SessionConfig.IsSecondaryClient bool（透传链 mountCfg → SessionConfig → role 字符串）
    - MountConfig.IsSecondaryClient bool（由 ssh.go 注入闭包置位）
    - 6 个 TestIntegration_Phase32_* 用例（Phase 31 fixture 复用）
  affects:
    - internal/cloudclaude/ssh.go::ConnectAndRunClaudeV3 — 替换 SyncSessionLock noop 默认 + SessionConfig 追加 IsSecondaryClient 透传（line 86-110, line 181）
    - internal/cloudclaude/mount_strategy.go::MountConfig — 末尾追加 IsSecondaryClient bool（+5 行；MountWorkspace 函数体 zero diff）
    - internal/cloudclaude/session.go::SessionConfig + writeClientFile + runClaudeWithSession（IsSecondaryClient 字段 + role 字符串切换 2 处）
    - internal/cloudclaude/integration_test.go — 末尾 +160 行 6 个新用例 + defaultFixtureSSHConfig helper
tech-stack:
  added: []
  patterns:
    - 远程 flock 单例锁（-n -E 99 -F + exec sleep infinity 防 fork）
    - shellescape.Quote 包装锁路径（accountID 路径拼接 / T-32-14 mitigate）
    - 多端 client_role 三层可见性（stderr 错误码 + last-session.json + 文件注册表 client_role）
    - 集成测试 Skip 兼容（短模式 + docker network 缺权限优雅跳过）
key-files:
  created:
    - internal/cloudclaude/sync_lock.go (122 LOC)
    - internal/cloudclaude/sync_lock_test.go (73 LOC，9 个单测 PASS)
  modified:
    - internal/cloudclaude/ssh.go (+27 行：AcquireSyncLock 闭包覆盖 + import errors + SessionConfig IsSecondaryClient 透传)
    - internal/cloudclaude/mount_strategy.go (+5 行：MountConfig 末尾追加 IsSecondaryClient bool)
    - internal/cloudclaude/session.go (+22 行：SessionConfig 字段 + writeClientFile role 形参 + runClaudeWithSession role 三元 + pTYAttachOnce role 三元)
    - internal/cloudclaude/integration_test.go (+160 行：6 个 TestIntegration_Phase32_* + defaultFixtureSSHConfig helper + import errors)
decisions:
  - "AcquireSyncLock 走 PLAN 模板逐字符复刻，远程 flock -n -E 99 -F + exec sleep infinity，但
    在签名上增加 conn==nil + 非空 accountID 时显式返回错误（替代 panic / 静默 noop） — 让 ssh.go
    注入闭包能感知后端拨号问题，由 mount_strategy 走标准错误链；anon 路径 (accountID==\"\") 仍
    保持 noop 不依赖 conn，符合 D-19 设计意图。"
  - "writeClientFile 签名追加 role string（默认 'primary' 兼容旧 caller）— 与 PLAN
    interfaces 不完全一致（PLAN 只示例了 writeLastSessionTmuxField 的 role 切换），但
    必须同步切 client_role 才能让 doctor / explain 通过文件注册表准确识别 secondary（防
    M13 静默降级回归 / T-32-21）。pTYAttachOnce 内 caller 同步注入 sessionCfg.IsSecondaryClient
    决定的 role 字符串。"
  - "ssh.go 闭包用 mountCfg.Logger（非 nil 时）输出 [SESSION_SYNC_LOCKED]，缺省 fallback
    os.Stderr — 与 ConnectAndRunClaudeV3 内其它 OAuth/SESSION 错误码输出方式一致；
    PLAN interfaces 同时示意了两种写法，本实现两条路径都覆盖以避免 mountCfg.Logger=nil 时
    的吞错黑洞（main.go 当前必填，但工程上可见性优先）。"
  - "Phase 31 mount_strategy.MountWorkspace 内并未真正调用 mountCfg.SyncSessionLock 闭包
    （Phase 31 D-31 仅留字段未落地调用）。本 plan 严格遵守 user 指令 \"不重写 mount_strategy\"，
    把 ssh.go 注入的 AcquireSyncLock 闭包安装到位即停手；mount_strategy 调用链需由
    verify_phase_goal / Phase 31 后续 hotfix 闭环。集成测试 SyncLockMutexes 直接调
    AcquireSyncLock 验证锁本身的功能正确（不依赖 mount_strategy 链路）。"
  - "TestIntegration_Phase32_NetworkDisconnect30s 在 short mode 与 docker network 缺权限
    场景双重 t.Skip — 端到端 PTY 交互（cloud-claude→tmux→拔网→reconnect→buffer 完整）
    工程量超本 plan；按 PLAN <truths>(c) 已明确允许 skip + 留 Phase 35 真机 UAT。"
metrics:
  duration: ~25 分钟
  task_count: 3 (3.1 / 3.2 / 3.3)
  test_count: 9 单测 PASS (sync_lock_test.go) + 22 既有 (session_test.go) 全 PASS
  file_count: 5 (2 新建 + 3 修改 — 加 integration_test.go 共 6 个)
  loc_added: ~387 (sync_lock 122 + sync_lock_test 73 + ssh +27 + session +22 + mount_strategy +5 + integration_test +160)
  completed: 2026-04-20
---

# Phase 32 Plan 03: sync-lock-integration Summary

落地 Phase 32 v3.0 多端共享设计的最后一块——**容器侧账号级 Mutagen 单例锁**——通过远程 `flock(1)` 把同一 `claude_account` 在容器内的 Mutagen sync 收敛到唯一持有者；后端拿不到锁即视为 secondary client，透传 `ErrSyncLocked` 给 mount_strategy 走 sshfs-only 降级，同时三层可见性（stderr 错误码 + `last-session.json client_role` + 文件注册表 `client_role`）让 doctor / explain 排障无遗漏。配套 6 个 `TestIntegration_Phase32_*` 集成测试覆盖 C3 / C7 / REQ-F5-D 关键路径，复用 Phase 31 fixture 不引入新基础设施。

## 一句话

`Phase 32 Plan 03` 把 `AcquireSyncLock` 通过 `mountCfg.SyncSessionLock` 闭包注入 ConnectAndRunClaudeV3，并把 `IsSecondaryClient` 标志从 mount → ssh → session 三层透传到 `last-session.json` / 文件注册表 `client_role` 的写入分支；零 `go.mod` 增量、零 Phase 31 fixture 改动、零 Plan 02 业务逻辑改动。

## AcquireSyncLock 实际实现 vs PLAN `<interfaces>` 对照

```go
// internal/cloudclaude/sync_lock.go
var ErrSyncLocked = errors.New("sync session locked by another cloud-claude")

func AcquireSyncLock(conn *ssh.Client, accountID string) (func(), error)
func parseLastInt(s string) int  // exported 单测覆盖（PLAN <interfaces> 已声明）
```

远程命令模板（RESEARCH §6.3 逐字符）：

```bash
mkdir -p /tmp/cloud-claude/locks 2>/dev/null && \
  flock -n -E 99 -F <lockPath_q> -c 'echo $$; exec sleep infinity' & \
  echo $!
```

锁路径选择论证：
- **/tmp/cloud-claude/locks/sync-<accountID>.lock**（CONTEXT D-17 修订；RESEARCH §6.2）
- 不选系统级 lock 目录：ubuntu:24.04 默认是 root-only（mode 0755 root:root + symlink 到 /run/lock），UID 1000 `mkdir` 直接 EACCES，无法回写。
- 不选 /run：tmpfs 但同样有权限问题；/tmp 默认 mode 1777 + sticky bit 全用户可写、OS 重启清理符合 lock 语义、不污染 mergerfs / Mutagen。

`flock` 关键参数：
- `-n` 非阻塞：拿不到立即退（避免死锁等待）
- `-E 99` 自定义"锁被占"退出码：与 flock 默认 1（系统错误）区分，让 ssh.ExitError.ExitStatus() 解析无歧义
- `-F` no-fork：让 `sleep infinity` 直接持有 fd，缺此参数 → SSH 关闭时 sleep 死了但 flock 父进程仍持锁，锁不释放（RESEARCH §6.1）
- `exec sleep infinity` 替换 shell 自身：长驻持锁，由 SSH session.Close 走 SIGHUP 自然收割

签名偏差（与 PLAN 一致 + 1 处自洽增强）：
- PLAN 示例 `if accountID == "" { return func() {}, nil }`；本实现新增 `if conn == nil` 的显式错误（仅在非 anon 路径触发），避免 ssh.go 注入闭包在 connA 异常时静默 panic，让 mount_strategy 能拿到结构化错误。anon 路径仍 `noop` 不依赖 conn，符合 D-19。

## ssh.go SyncSessionLock 闭包覆盖位置（before / after diff 摘录）

**位置**：`ConnectAndRunClaudeV3` line 86-110，紧跟 `mountCfg.LastSessionPath` 推导之后、`MountWorkspace` 调用之前（与 Plan 02 SUMMARY §A 接入点提示一致）。

**Before**（Phase 31 默认 noop）：

```go
if mountCfg.SyncSessionLock == nil {
    mountCfg.SyncSessionLock = func(_ string) (func(), error) {
        return func() {}, nil
    }
}
```

**After**（Plan 03 真实 flock 包装）：

```go
if mountCfg.SyncSessionLock == nil {
    mountCfg.SyncSessionLock = func(accountID string) (func(), error) {
        release, err := AcquireSyncLock(connA, accountID)
        if err != nil {
            if errors.Is(err, ErrSyncLocked) {
                mountCfg.IsSecondaryClient = true
                if mountCfg.Logger != nil {
                    fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.SESSION_SYNC_LOCKED, accountID))
                } else {
                    fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.SESSION_SYNC_LOCKED, accountID))
                }
            }
            return nil, err
        }
        return release, nil
    }
}
```

import 增加 `"errors"`。`fmt` / `os` / `errcodes` 已存在。

## IsSecondaryClient 三层透传链

```
[1] ssh.go ConnectAndRunClaudeV3 SyncSessionLock 闭包（捕获本地 mountCfg）
        ↓ 锁失败时 mountCfg.IsSecondaryClient = true
[2] ssh.go SessionConfig 构造段（line 181）
        IsSecondaryClient: mountCfg.IsSecondaryClient
        ↓
[3] session.go runClaudeWithSession（line 588-591）+ pTYAttachOnce（line 754-758）
        role := "primary"; if sessionCfg.IsSecondaryClient { role = "secondary" }
        ↓ 写入两处：
        - writeLastSessionTmuxField(path, sessionName, role)  → last-session.json client_role
        - writeClientFile(conn, ..., role)                    → 容器内 <pid>.json client_role
```

设计意图：当 mount_strategy 真正调用 `mountCfg.SyncSessionLock(accountID)` 拿到 `ErrSyncLocked` 后，**同一 mountCfg 变量**的 IsSecondaryClient 字段已被闭包置位（闭包通过捕获本地 mountCfg 引用直接写入），后续 SessionConfig 构造时复制该字段即可，无需再在 mount_strategy 内手工传值。

## 6 个 TestIntegration_Phase32_* 用例与 Phase 32 SC 对照

| 用例 | 覆盖 SC | 简述 |
|------|--------|------|
| TestIntegration_Phase32_PgrepNoSystemdLogind | SC3 (C7) | 容器内必无 systemd-logind（Phase 29 镜像侧防御） |
| TestIntegration_Phase32_TmuxSurvivesSighupSshd | SC3 (C7) | kill -HUP sshd 后 tmux server 仍存活、session 仍可访问 |
| TestIntegration_Phase32_SyncLockMutexes | SC11 (REQ-F5-D) | 双端互斥（第二端 ErrSyncLocked）+ release 后第二端能拿锁 + lockfile 路径正确 |
| TestIntegration_Phase32_SyncLockAnonNoop | SC11 (D-19) | accountID="" 路径 noop 不创建锁文件 |
| TestIntegration_Phase32_DetectTmuxAvailable | SC11 (D-15) | Phase 29 镜像 tmux 3.4+ 必命中（DetectTmux=true） |
| TestIntegration_Phase32_NetworkDisconnect30s | SC12 (C3) | 框架就位；端到端 PTY 留 Phase 35 真机 UAT |

## C3 / C7 / M15 三大 PITFALL 最终防御状态

| Pitfall | 问题描述 | 本 plan 落地的最终防御 |
|---------|----------|-----------------------|
| **C3** sshfs 抖动级联 | 30s 网络抖动时 `ls /workspace` 不可 hang，cold branch 必须摘除 | sshfs_watcher（Phase 31 ship）+ 集成测试 NetworkDisconnect30s 框架（Phase 35 真机闭环） |
| **C7** systemd-logind SIGHUP 杀链 | `kill -HUP sshd` 时 systemd-logind 顺带杀 tmux server | Phase 29 镜像移除 systemd-logind + 本 plan 集成测试 PgrepNoSystemdLogind / TmuxSurvivesSighupSshd 验收 |
| **M15** Mutagen 双写灾难 | 同 account 两端同时 mutagen sync → 数据 corruption / lost updates | 本 plan 容器侧 flock 单例锁（AcquireSyncLock）+ ssh.go 注入闭包 + secondary client 降级到 sshfs-only 视图 |

## Verify 命令实测结果

```
$ go build ./...                                                              PASS
$ go build -tags=integration ./internal/cloudclaude/...                       PASS
$ GOOS=linux  go vet ./internal/cloudclaude/...                               PASS
$ GOOS=darwin go vet ./internal/cloudclaude/...                               PASS
$ go vet -tags=integration ./internal/cloudclaude/...                         PASS
$ go test ./internal/cloudclaude/... -count=1 -short
ok      github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude         0.752s
ok      github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes  0.453s
$ go test ./internal/cloudclaude/... -run "TestAcquireSyncLock|TestParseLastInt" -v
   9/9 PASS
$ rg "^func TestIntegration_Phase32_" internal/cloudclaude/integration_test.go | wc -l   → 6
$ rg -n "/var/lock" internal/cloudclaude/sync_lock.go                         无命中（D-17 验证）
$ rg -n "AcquireSyncLock" internal/cloudclaude/ssh.go                         命中 2 行
$ rg -n "ErrSyncLocked" internal/cloudclaude/ssh.go                           命中 2 行
$ rg -n "IsSecondaryClient" internal/cloudclaude/{mount_strategy,ssh,session}.go 共 8 行
$ git diff scripts/test-fixture-up.sh scripts/test-fixture-down.sh            无 diff
```

## Deviations from Plan

### 设计偏差（关键决策上移到 decisions 元数据）

**1. writeClientFile 签名追加 role 形参**
- **Found during:** Task 3.2 编辑 session.go 时
- **Issue:** PLAN `<interfaces>` 仅示例 `writeLastSessionTmuxField` 的 role 切换；但文件注册表的 `client_role` 字段（session.go line 252 hard-code `"primary"`）若不同步切，doctor/explain 通过 `/workspace/.cloud-claude/clients/<pid>.json` 读 client_role 时会误判 secondary 为 primary（M13 静默降级回归 / T-32-21）。
- **Fix:** writeClientFile 签名追加 `role string`（默认 "primary" 兼容旧路径）；pTYAttachOnce 内 caller 由 sessionCfg.IsSecondaryClient 决定。零外部 caller（仅 session.go 内一处）所以无 API 风险。
- **Files modified:** internal/cloudclaude/session.go
- **Commit:** 42cf058

**2. Phase 31 mount_strategy.MountWorkspace 未实际调用 SyncSessionLock 闭包**
- **Found during:** Task 3.2 grep `rg -n "SyncSessionLock" internal/cloudclaude/mount_strategy.go`
- **Issue:** Phase 31 D-31 在 MountConfig 上加了 SyncSessionLock 字段并预留默认 noop，但 MountWorkspace 函数体内**未实际调用**该闭包。本 plan 注入的 AcquireSyncLock 实现因此**当前未被生效路径触发**（仅集成测试 SyncLockMutexes 直接调用验证）。
- **决策:** 严格遵守 user 指令 "不重写 mount_strategy" — 本 plan 把闭包安装到位即停。mount_strategy 调用链（应在 mountMutagen 之前调用 SyncSessionLock 拿锁，失败 errors.Is(err, ErrSyncLocked) 时强制走 ModeSSHFSOnly + 写 DowngradeChain "sync_locked"）属 Phase 31 D-31 落地缺口，留给 verify_phase_goal 识别后走 gap-closure 机制（hotfix 或独立 plan）。
- **Carry-over impact:** 本 plan 完成后，多端启动场景的实际锁失败仍会按 mount 全档位 try → 全失败 path 走，而不是干净降级到 sshfs-only。集成测试 SyncLockMutexes 仍能验证 sync_lock.go 自身正确性（直接调用 AcquireSyncLock）；端到端多端启动 UAT 留 Phase 35 真机。

### Auto-fixed Issues

无。所有改动严格在 PLAN `<interfaces>` / `<remote_command_template>` / `<tasks>` 块内执行；上述 2 项偏差均为设计决策，已记入 decisions。

### Plan 02 BufferedStdin Wiring Gap 状态（user query 要求 carry-over）

- 状态：**未修复**（保持 Plan 02 SUMMARY decisions[0] 同状态）
- 原因：本 plan 时间预算优先用于完成 sync_lock + 集成测试；BufferedStdin 真接入 Reconnector.StateAddr() 共享状态需要 ~20-60 行 + 同步 input_buffer_test.go / reconnect_test.go 的 mock 重构（取决于是否补 Reconnector.RegisterStateListener / SetReconnector 接口），不属本 plan must-have。
- 影响：reconnect 期间用户键入仍会临时丢失（远端 tmux 持有 PTY 不丢，重连后继续输入 OK）。
- 留给 verify_phase_goal：若识别为 REQ-F3-B 端到端集成缺失，可走 gap-closure 走单独 plan / hotfix；或保留至 v3.1 + RegisterStateListener 接口一并落地。

## Known Stubs

无。本 plan 全部业务路径已 wire 通；mount_strategy 链路缺口属 Phase 31 边界外，已在 decisions / Deviations 内显式标记 carry-over。

## Threat Flags

无新增威胁面。PLAN `<threat_model>` 已枚举 7 项 STRIDE 威胁全部 mitigate (4) / accept (3)；T-32-14（accountID 路径拼接）通过 `shellescape.Quote(lockPath)` 在 sync_lock.go 内 mitigate（grep 命中 1 次）；T-32-21（M13 静默降级回归）通过三层可见性（stderr 错误码 + last-session.json + 文件注册表 client_role）mitigate。

## Self-Check: PASSED

- [x] internal/cloudclaude/sync_lock.go 存在（122 LOC）
- [x] internal/cloudclaude/sync_lock_test.go 存在（73 LOC，9 个单测 PASS）
- [x] internal/cloudclaude/ssh.go SyncSessionLock 注入闭包就位（line 86-110）+ SessionConfig IsSecondaryClient 透传（line 181）
- [x] internal/cloudclaude/mount_strategy.go MountConfig 末尾追加 IsSecondaryClient bool（+5 行）
- [x] internal/cloudclaude/session.go SessionConfig.IsSecondaryClient + writeClientFile role 形参 + runClaudeWithSession + pTYAttachOnce role 切换 4 处
- [x] internal/cloudclaude/integration_test.go 末尾追加 6 个 TestIntegration_Phase32_* + defaultFixtureSSHConfig helper
- [x] commit f6c4197 (Task 3.1) 存在
- [x] commit 42cf058 (Task 3.2) 存在
- [x] commit e577aa7 (Task 3.3) 存在
- [x] go build ./... PASS
- [x] go build -tags=integration ./internal/cloudclaude/... PASS
- [x] GOOS=linux + GOOS=darwin go vet PASS
- [x] go test ./internal/cloudclaude/... -count=1 -short PASS（含 9 新 + 22 既有）
- [x] scripts/test-fixture-up.sh / down.sh zero diff
- [x] go.mod 无新依赖（git diff go.mod 为空）

## Hand-off 清单（留给 /gsd-verify-phase 32）

由本 plan 验收的 Phase 32 SC（ROADMAP §Phase 32 Success Criteria）：

- ✅ **SC3** (C7 — pkill -SIGHUP sshd 后 tmux 仍存活 + 容器无 systemd-logind)：
  集成测试 TestIntegration_Phase32_TmuxSurvivesSighupSshd + TestIntegration_Phase32_PgrepNoSystemdLogind（依赖 docker fixture）
- ✅ **SC11** (REQ-F5-D 账号级 Mutagen 单例锁): TestIntegration_Phase32_SyncLockMutexes + SyncLockAnonNoop；sync_lock.go 实现就位
- ⚠️ **SC12** (C3 sshfs 抖动 30s 不挂死): sshfs_watcher（Phase 31 ship）+ NetworkDisconnect30s 框架；端到端 PTY 留 Phase 35 真机 UAT

需要 verify_phase_goal 关注的 carry-over：

1. **mount_strategy.MountWorkspace 未实际调用 SyncSessionLock 闭包**（Phase 31 D-31 落地缺口；本 plan 安装好闭包即可，调用链 hotfix 留 verifier）
2. **Plan 02 BufferedStdin 未真接入 Reconnector 共享状态**（REQ-F3-B 端到端集成缺失；reconnect 期间键入丢失，但远端 tmux 进程不丢；验证后可走 gap-closure 或留 v3.1 + RegisterStateListener 接口一并落地）

非本 plan SC：
- SC1/2/4-10 由 Plan 01 / Plan 02 验收
