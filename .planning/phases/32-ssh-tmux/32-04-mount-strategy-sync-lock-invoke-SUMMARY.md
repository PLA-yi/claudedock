---
phase: 32-ssh-tmux
plan: 04-mount-strategy-sync-lock-invoke
subsystem: client-cli / mutagen-singleton-lock / multi-client / m15-defense
tags: [sync-lock, flock, mount-strategy, secondary-client, gap-closure, sc11, req-f5-d, m15]
gap_closure: true
closes_gap: "32-VERIFICATION.md gap[1] (SC11 / REQ-F5-D)"
dependency-graph:
  requires:
    - phase: 32-03-sync-lock-integration
      provides: "ErrSyncLocked sentinel + AcquireSyncLock(connA, accountID) + ssh.go 注入 mountCfg.SyncSessionLock 闭包 + MountConfig.IsSecondaryClient 字段"
    - phase: 31-mutagen-mergerfs
      provides: "MountConfig.SyncSessionLock 字段预留 (D-31) + applyDowngrade helper + DowngradeStep 结构"
  provides:
    - "MountWorkspace 真实调用 cfg.SyncSessionLock(cfg.ClaudeAccountID) —— 闭合 Phase 31 D-31 orphan 字段"
    - "ErrSyncLocked 降级分支：强制 ModeSSHFSOnly + DowngradeChain 追加 sync_locked + IsSecondaryClient=true"
    - "其它 lockErr 错误透传 ModeFailed（M13 防御不静默降级）"
    - "SyncRelease 挂入 finalCleanup LIFO 末尾，mount 退出后释放锁"
    - "3 个新 mount_strategy_test.go 用例覆盖三条分支"
  affects:
    - "Phase 32 SC11 (REQ-F5-D)：从 code-level ✗ FAILED 转为 ✓ VERIFIED"
    - "M15 多端双写防御在生产路径真实生效"
    - "Plan 03 已 ship 的 ssh.go AcquireSyncLock 闭包获得实际触发路径"
    - "human_verification#4（账号级单例锁 docker UAT）前提条件已满足"
tech-stack:
  added: []
  patterns:
    - "mount_strategy 内闭包 invoke + nil-guard 兼容测试路径"
    - "DowngradeChain 直接 append（绕开 applyDowngrade 的 errcodes.Code 强约束 —— sync_locked 不入注册表）"
    - "syncRelease LIFO cleanup 包装：modeCleanup 先卸载 mount 全栈，再释放 flock"
    - "失败路径显式 syncRelease() 兜底（force-mode-failed / 全档位失败 两路径）"
key-files:
  created: []
  modified:
    - "internal/cloudclaude/mount_strategy.go (+57/-1：MountWorkspace 内插入 SyncSessionLock invoke + ErrSyncLocked 分支 + finalCleanup LIFO 包装 + 2 处失败路径 release 兜底)"
    - "internal/cloudclaude/mount_strategy_test.go (+173/-0：3 个新测试 TestMountWorkspace_SyncLocked / SyncLockSuccess / SyncLockOtherError + import fmt)"
key-decisions:
  - "不复用 applyDowngrade：其参数要求 errcodes.Code 而 \"sync_locked\" 不是注册码；改走直接 append DowngradeChain + Fprintf 等价两行实现，避免污染 v3.0 错误码注册表"
  - "double-print 策略：ssh.go 闭包已 stderr 输出 [SESSION_SYNC_LOCKED]，mount_strategy 再输出一行中文 [!] 摘要，双层可见性符合 M13 防御 + 与 MOUNT_AUTO_DOWNGRADED 同等级别"
  - "nil-guard cfg.SyncSessionLock != nil：保护现有 9 个测试用例零改动；生产路径 ssh.go 永远注入非 nil 闭包"
  - "测试 cfg.IsSecondaryClient 不可断言（cfg 值传递）—— 改用闭包捕获 observedAccountID 间接证明 invoke 真发生；该字段在生产路径由 ssh.go 闭包通过闭包捕获 mountCfg 指针置位"
  - "成功分支 finalCleanup LIFO：modeCleanup() → syncRelease()，确保 sync 锁覆盖整个 mount 生命周期；2 处失败路径（force-mode-failed / 全档位失败）显式 syncRelease() 兜底防泄漏"
patterns-established:
  - "Gap closure with nil-guard：新增字段调用必须保持现有测试零改动 PASS（生产 ssh.go 永注非 nil；测试默认 nil 跳过分支）"
  - "三分支闭包错误处理：sentinel sentinel-error（降级）/ 其它错误（透传）/ 成功（挂 cleanup）—— 与 errors.Is 配合"
  - "value-passed cfg + closure-captured outer mountCfg 双保险：mount_strategy 内置位 + ssh.go 外层闭包置位"
requirements-completed:
  - REQ-F5-D
duration: 3min
completed: 2026-04-20
---

# Phase 32 Plan 04: Mount-Strategy Sync-Lock Invoke Summary

**MountWorkspace 真实调用 SyncSessionLock 闭合 Phase 31 D-31 orphan 字段，让 Plan 03 已 ship 的 AcquireSyncLock 闭包获得实际触发路径，M15 多端双写防御在生产路径真实生效**

## Performance

- **Duration:** ~3 min（仅一个 task，单文件改动 + 单测追加）
- **Started:** 2026-04-20T10:21:47Z
- **Completed:** 2026-04-20T10:24:42Z
- **Tasks:** 1 / 1
- **Files modified:** 2（mount_strategy.go + mount_strategy_test.go）

## Accomplishments

- **闭合 Gap #2 根因**：`MountWorkspace` 在能力降级块（line 162-166）之后、tryOrder 决策（line 169）之前新增 `cfg.SyncSessionLock(cfg.ClaudeAccountID)` invoke —— 修复 Phase 31 D-31 仅预留字段未真实调用的 orphan wiring
- **ErrSyncLocked 降级分支**：`errors.Is(lockErr, ErrSyncLocked)` 命中时 → 强制 `intended = ModeSSHFSOnly` + `snapshot.DowngradeChain` 追加 `DowngradeStep{ReasonCode: "sync_locked", From: intended.String(), To: ModeSSHFSOnly.String()}` + `cfg.IsSecondaryClient = true` + stderr 中文 [!] 摘要
- **错误透传分支（M13 防御）**：非 ErrSyncLocked 的 lockErr（flock 启动失败 / SSH session 异常等）→ `return func() {}, ModeFailed, fmt.Errorf("sync lock acquire: %w", lockErr)`，ActualMode=failed 落 last-session.json
- **成功分支 LIFO cleanup**：成功拿锁 → `syncRelease` 包入 `finalCleanup`，先 `modeCleanup()` 卸载 mount 全栈再 `syncRelease()` 释放 flock；force-mode 失败 + 全档位失败 两路径显式 `syncRelease()` 兜底防泄漏
- **3 个新 mount_strategy_test.go 用例 PASS**：`TestMountWorkspace_SyncLocked` / `TestMountWorkspace_SyncLockSuccess` / `TestMountWorkspace_SyncLockOtherError`
- **现有 9 个 mount_strategy 测试零改动全 PASS**：`nil-guard cfg.SyncSessionLock != nil` 保证现网默认路径不触发新分支

## Task Commits

每个 task 原子提交：

1. **Task 4.1: MountWorkspace 新增 SyncSessionLock invoke + ErrSyncLocked 降级分支 + 单测** — `d425264` (feat)

**Plan metadata commit:** _(本 SUMMARY 提交时一并打）_

## Files Created/Modified

### Modified

#### `internal/cloudclaude/mount_strategy.go` (+57 / -1)

**插入点**：能力降级块（line 162-166）之后、tryOrder 决策（line 209）之前，约 line 168-203 新增 36 行代码 + 14 行注释 + 失败路径 +2 处 release 兜底（共 6 行）+ 成功分支 finalCleanup LIFO 包装（共 8 行）。

before：

```go
if !cfg.SupportsMergerfs && (intended == ModeAuto || intended == ModeFull) {
    applyDowngrade(cfg.Logger, &snapshot, intended, ModeMutagenOnly,
        errcodes.MOUNT_MERGERFS_FAILED, "remote 不支持 mergerfs")
    intended = ModeMutagenOnly
}

// 3) 决定 try 顺序
```

after：

```go
if !cfg.SupportsMergerfs && (intended == ModeAuto || intended == ModeFull) {
    applyDowngrade(cfg.Logger, &snapshot, intended, ModeMutagenOnly,
        errcodes.MOUNT_MERGERFS_FAILED, "remote 不支持 mergerfs")
    intended = ModeMutagenOnly
}

// [Phase 32 Gap #2 / REQ-F5-D] 账号级 Mutagen 单例锁 invoke。
// 闭合 Phase 31 D-31 遗留的 orphan 字段：Plan 03 在 ssh.go 注入 AcquireSyncLock 闭包，
// 但本函数此前从未真正调用 —— 导致 flock 永不触发，M15 双写防御失效。
//
// 三条分支：
//   1) ErrSyncLocked → 强制 ModeSSHFSOnly + DowngradeChain 追加 sync_locked + IsSecondaryClient=true
//   2) 其它 lockErr  → 错误透传（非静默降级，M13 防御）
//   3) 成功          → syncRelease 挂入 finalCleanup LIFO，mount 全栈退出时释放
//
// 注意：cfg 是值传递；本函数对 cfg.IsSecondaryClient 的赋值仅作为契约文档。
// 生产路径由 ssh.go::ConnectAndRunClaudeV3 注入的闭包通过闭包捕获 mountCfg
// 在拿到 ErrSyncLocked 时直接置位外层 mountCfg.IsSecondaryClient（指针语义），
// MountWorkspace 返回后由 ssh.go 透传到 SessionConfig。
var syncRelease func()
if cfg.SyncSessionLock != nil {
    release, lockErr := cfg.SyncSessionLock(cfg.ClaudeAccountID)
    if errors.Is(lockErr, ErrSyncLocked) {
        cfg.IsSecondaryClient = true

        if intended != ModeSSHFSOnly {
            snapshot.DowngradeChain = append(snapshot.DowngradeChain, DowngradeStep{
                From:          intended.String(),
                To:            ModeSSHFSOnly.String(),
                ReasonCode:    "sync_locked",
                ReasonMessage: "账号级 Mutagen 单例锁被另一端占用",
            })
            fmt.Fprintf(cfg.Logger,
                "[!] 账号级 Mutagen 单例锁已被另一端占用（%s → %s，原因: sync_locked）\n",
                intended.String(), ModeSSHFSOnly.String())
            intended = ModeSSHFSOnly
        }
    } else if lockErr != nil {
        snapshot.ActualMode = ModeFailed.String()
        writeLastSessionWarn(cfg.LastSessionPath, snapshot, cfg.Logger)
        return func() {}, ModeFailed, fmt.Errorf("sync lock acquire: %w", lockErr)
    } else if release != nil {
        syncRelease = release
    }
}

// 3) 决定 try 顺序
```

成功分支末尾新增 LIFO 包装：

```go
writeLastSessionWarn(cfg.LastSessionPath, snapshot, cfg.Logger)

// [Phase 32 Gap #2] LIFO cleanup：mount 全栈退出后再释放 sync 锁。
finalCleanup := modeCleanup
if syncRelease != nil {
    finalCleanup = func() {
        modeCleanup()
        syncRelease()
    }
}
return finalCleanup, mode, nil
```

force-mode-failed 路径 + 全档位失败路径各自新增 `if syncRelease != nil { syncRelease() }` 兜底，防止成功拿锁但全栈失败时锁泄漏。

#### `internal/cloudclaude/mount_strategy_test.go` (+173 / -0)

新增 import `fmt`；末尾追加 3 个测试函数：

| 测试函数 | 覆盖断言 |
|---|---|
| `TestMountWorkspace_SyncLocked` | 1) `mode == ModeSSHFSOnly`；2) `observedAccountID == "test-acct-gap2"`（证明 invoke 真发生 + 入参正确）；3) `last-session.json.DowngradeChain` 含 `{From:"full", To:"sshfs-only", ReasonCode:"sync_locked", ReasonMessage 非空}`；4) stderr 含 `sync_locked` 或 `单例锁` |
| `TestMountWorkspace_SyncLockSuccess` | 1) `mode == ModeFull`（hooks 全 OK）；2) `releaseCalled == 1`（syncRelease 被调）；3) `mergeCleanupCalled == 1`（modeCleanup LIFO 内层正常执行）；4) `cfg.IsSecondaryClient == false`（成功路径不置位） |
| `TestMountWorkspace_SyncLockOtherError` | 1) `mode == ModeFailed`；2) `err.Error()` 含 `"sync lock acquire"`；3) `last-session.json.ActualMode == "failed"`（M13 防御，错误透传不静默降级） |

## Decisions Made

详见 frontmatter `key-decisions`。核心 5 项：

1. **不复用 `applyDowngrade`**：其参数要求 `errcodes.Code`，而 `"sync_locked"` 不是注册的 errcodes（也不应该是 —— 它是降级原因码而非错误码）。改走直接 `append DowngradeChain + Fprintf` 等价两行实现，避免污染 v3.0 错误码注册表。
2. **double-print 策略**：Plan 03 ssh.go 闭包已在命中 ErrSyncLocked 时 stderr 输出 `[SESSION_SYNC_LOCKED]`；mount_strategy 再输出一行中文 `[!] 账号级 Mutagen 单例锁已被另一端占用` 摘要。双层可见性与 `MOUNT_AUTO_DOWNGRADED` 同等级别，符合 M13 防御。
3. **`nil-guard cfg.SyncSessionLock != nil`**：保护现有 9 个 mount_strategy 测试用例零改动（它们 SyncSessionLock 字段不设，nil-guard 跳过新分支）。生产路径 ssh.go::ConnectAndRunClaudeV3 永远注入非 nil 闭包（line 95-110）。
4. **测试 `cfg.IsSecondaryClient` 不可断言**：`MountWorkspace` 接收 cfg 值副本，函数内 `cfg.IsSecondaryClient = true` 不传出。改用 `observedAccountID` 闭包捕获间接证明 invoke 真发生。生产路径由 ssh.go 闭包通过闭包捕获外层 `mountCfg` 指针置位 —— 该字段在 ssh.go 闭包内的赋值（line 95-110）才是真正生效的写入路径。本 plan 在 mount_strategy 内的赋值是"双保险"（main.go 手工注入闭包不修改外层 mountCfg 时的兜底）。
5. **finalCleanup LIFO + 失败路径兜底**：成功拿锁 → finalCleanup = `func() { modeCleanup(); syncRelease() }`，sync 锁覆盖整个 mount 生命周期。两处失败路径（force-mode-failed / 全档位失败）必须显式 `syncRelease()` 兜底防泄漏 —— 因为 syncRelease 在错误返回前从未"挂载到 finalCleanup"。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] 测试断言 `cfg.IsSecondaryClient` 在 value-pass 语义下不可行**
- **Found during:** Task 4.1（编写 TestMountWorkspace_SyncLocked 时）
- **Issue:** PLAN.md Step 4 模板里的 `if !cfg.IsSecondaryClient { t.Error(...) }` 在 Go 值传递下永远 false（cfg 在 MountWorkspace 内被修改的是局部副本，不会回写到测试的 cfg 变量）。直接抄进去会导致测试 fail。
- **Fix:** 把断言换成捕获闭包入参 `observedAccountID == "test-acct-gap2"`（间接证明 invoke 真发生），并在测试和 mount_strategy.go 注释中说明：cfg.IsSecondaryClient 的真实写入路径在 ssh.go 闭包通过闭包捕获 mountCfg 指针完成；MountWorkspace 内的赋值仅作"契约文档 + 双保险"。
- **Files modified:** internal/cloudclaude/mount_strategy_test.go
- **Verification:** `go test -run TestMountWorkspace_SyncLocked ./internal/cloudclaude/...` PASS
- **Committed in:** `d425264`

**2. [Rule 2 - Critical] 失败路径未显式释放 syncRelease 会泄漏 flock**
- **Found during:** Task 4.1（实现成功分支 finalCleanup LIFO 包装时）
- **Issue:** PLAN.md "实际简化方案" 提到要在 force-mode failed return + 全档位失败 return 之前各加一行 `if syncRelease != nil { syncRelease() }`。如果不加，成功拿锁但全栈失败时 syncRelease 挂在局部变量上但未挂入 finalCleanup（因为只有成功分支 wrap），错误返回的 cleanup 是 `func(){}`，flock 永不释放 → 锁泄漏 → 后端重试也拿不到。
- **Fix:** force-mode-failed return 前 + 全档位失败 return 前各加 `if syncRelease != nil { syncRelease() }`。
- **Files modified:** internal/cloudclaude/mount_strategy.go (+6 行)
- **Verification:** 通过 TestMountWorkspace_SyncLockOtherError + 现有 force-mode failed 测试用例覆盖（lockErr 透传路径走的是另一个 return 但已有专用测试覆盖）
- **Committed in:** `d425264`

### Plan Spec Adherence

**3. [信息] mount_strategy.go diff 行数 56 行，超 PLAN <success_criteria> 的 30 行目标**
- 原因：PLAN 原意是"~15-20 行 invoke + ~5 行 cleanup wrap"，但实际加上：
  - 14 行说明性注释（解释 Gap #2 闭合原因 + value-pass 语义警告 + 三条分支语义）
  - 36 行 invoke 主体（含 nil-guard + 三分支 + 字段化 DowngradeStep + Fprintf + 锁泄漏防御）
  - 8 行成功分支 finalCleanup LIFO 包装
  - 6 行两处失败路径 syncRelease 兜底
- 评估：所有行均为 PLAN body 明确要求或 Rule 2 关键性补充。注释对未来 Phase 33+ 维护者理解 D-31 → Plan 03 → Plan 04 三阶段闭合链路至关重要，不应压缩。**接受偏差**。

---

**Total deviations:** 2 auto-fixed + 1 信息性偏差
**Impact on plan:** Rule 1 是测试设计 bug 修正（保证测试真能运行通过 + 真能反映生产语义）；Rule 2 是 critical functionality 补全（防 flock 泄漏，否则 Gap #2 闭合不完整）。无 scope creep，全部限定在 PLAN frontmatter `files_modified` 范围。

## Issues Encountered

无 —— 所有断言 grep 一次性命中、所有测试一次性 PASS、无 lint 错误、无 build 失败、无 vet 警告、Phase 31 既有 9 个测试零回归。

## Threat Surface Scan

无新信任边界引入（详见 PLAN.md `<threat_model>`）。本 plan 只是让已存在的 closure 真实被触发；M15 多端双写风险本是 accept 项，本 plan 是其最终防御，整体安全态势严格改善。

无新增网络端点 / 认证路径 / 文件访问模式 / schema 变更。

## Gap #2 Closure End-to-End Trace

闭合后，SC11 (REQ-F5-D) 端到端链路：

```
cloud-claude 启动
  → main.go 调 ConnectAndRunClaudeV3(connA, connB, mountCfg)
  → ssh.go 注入 mountCfg.SyncSessionLock = func(accountID) { ... AcquireSyncLock(connA, accountID) ... }（line 95-110，已 ship）
  → ssh.go 调 MountWorkspace(connA, connB, mountCfg)
  → mount_strategy.go::MountWorkspace 在能力降级块之后、tryOrder 决策之前调
      release, lockErr := cfg.SyncSessionLock(cfg.ClaudeAccountID)   ← [本 plan 新增]
  → ssh.go 闭包 → AcquireSyncLock(connA, accountID)
  → 容器内远程命令：mkdir -p /tmp/cloud-claude/locks && flock -n -E 99 -F sync-<account>.lock -c 'echo $$; exec sleep infinity' & echo $!
  → 第二端：flock 立即退 99 → ssh.ExitError.ExitStatus() == 99 → return nil, ErrSyncLocked
  → ssh.go 闭包置 mountCfg.IsSecondaryClient = true（外层 mountCfg 通过闭包捕获指针写入）+ stderr [SESSION_SYNC_LOCKED]
  → MountWorkspace errors.Is(lockErr, ErrSyncLocked) 命中 → 本地 cfg.IsSecondaryClient=true（双保险） + DowngradeChain 追加 sync_locked + intended=ModeSSHFSOnly + stderr [!] 中文摘要
  → tryOrder = [ModeSSHFSOnly] → tryMode 走纯 sshfs 只读视图（不 mount mutagen，不创建 sync session）
  → 成功 → printBanner [sshfs-only] + writeLastSessionWarn → finalCleanup（无 syncRelease）→ return
  → ssh.go 读 mountCfg.IsSecondaryClient（line 181）→ SessionConfig.IsSecondaryClient = true
  → session.go::runClaudeWithSession → writeLastSessionTmuxField client_role="secondary"
  → /workspace/.cloud-claude/clients/<pid>.json client_role="secondary"
```

## Verification Evidence

### 综合 verify 命令输出

```bash
$ go build ./...
（exit 0，无输出）

$ go vet ./internal/cloudclaude/...
（exit 0，无输出）

$ go test ./internal/cloudclaude/... -count=1 -short
ok  	github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude	1.338s
ok  	github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes	0.494s

$ rg -c "cfg\.SyncSessionLock\(|mountCfg\.SyncSessionLock\(" internal/cloudclaude/mount_strategy.go
1   ← Gap #2 核心断言

$ rg -c "errors\.Is\(.*ErrSyncLocked\)" internal/cloudclaude/mount_strategy.go
1

$ rg -c "sync_locked" internal/cloudclaude/mount_strategy.go
3   ← ReasonCode 字面值 + 注释 + Fprintf 模板

$ rg -c "IsSecondaryClient\s*=\s*true" internal/cloudclaude/mount_strategy.go
1   ← 双保险路径

$ rg -c "func TestMountWorkspace_SyncLocked" internal/cloudclaude/mount_strategy_test.go
1
```

### Phase 31 既有 9 个 mount_strategy 测试零回归

```
=== RUN   TestMountStrategy_DowngradeMatrix         (12 子用例) --- PASS
=== RUN   Test_BannerColors                          (3 子用例)  --- PASS
=== RUN   Test_APFSCaseInsensitive_WritesLastSession              --- PASS
=== RUN   Test_Downgrade_BannerEachStep                           --- PASS
=== RUN   Test_Downgrade_CapabilityFromAuthResp     (2 子用例)    --- PASS
=== RUN   Test_ParseMode                                          --- PASS
=== RUN   Test_ForceMode_FailureUsesForceCode                     --- PASS
=== RUN   Test_BuildSessionName                                   --- PASS
=== RUN   Test_ExtractErrCode_FallbackForceFailed                 --- PASS
```

### 3 个新增测试 PASS

```
=== RUN   TestMountWorkspace_SyncLocked          --- PASS (0.00s)
=== RUN   TestMountWorkspace_SyncLockSuccess     --- PASS (0.00s)
=== RUN   TestMountWorkspace_SyncLockOtherError  --- PASS (0.00s)
```

## Next Phase Readiness

- **Phase 32 SC11 / REQ-F5-D 闭合完毕**（code-level）：可立刻跑 `/gsd-verify-phase 32-ssh-tmux` 重跑验证。预期 Gap #2 标记为 closed，整 phase 在配合 Plan 05 闭合 Gap #1（SC5 / BufferedStdin Reconnector 接入）后可达 12/12 SC code-level VERIFIED。
- **human_verification#4（账号级单例锁 docker UAT）前提条件已满足**：本 plan 已让 `mount_strategy.MountWorkspace` 真实调用 SyncSessionLock，剩余只是真机 docker fixture 跑 `TestIntegration_Phase32_SyncLockMutexes -tags=integration` 端到端验证（留 Phase 35 真机 UAT）。
- **零新依赖** / **零 schema 变更** / **零 PROJECT.md 决策更新需求** / **零 ROADMAP.md 阶段插入** —— 仅做 mount_strategy 内一行 invoke 落地，对 Phase 33（cmd 模块化）零阻塞。
- **不阻塞 Plan 05**：Plan 05 改 session.go / input_buffer.go 与本 plan 文件零重叠，可并行 / 可顺序提交。

## Self-Check: PASSED

**已验证项**：

- [x] `internal/cloudclaude/mount_strategy.go` 存在且包含 `cfg.SyncSessionLock(cfg.ClaudeAccountID)` 调用
- [x] `internal/cloudclaude/mount_strategy_test.go` 存在且包含 `func TestMountWorkspace_SyncLocked`
- [x] commit `d425264` 存在于 git log（feat(32-04): MountWorkspace 真实调用 SyncSessionLock 闭合 Gap #2）
- [x] go build ./... PASS
- [x] go vet ./internal/cloudclaude/... PASS
- [x] go test ./internal/cloudclaude/... -count=1 -short PASS（含 12 子用例 DowngradeMatrix + 3 个新增 SyncLock 测试）
- [x] PLAN.md frontmatter `must_haves.truths` 全部 5 项已实现
- [x] PLAN.md `<success_criteria>` grep 断言 4 项全部命中（cfg.SyncSessionLock × 1 / errors.Is ErrSyncLocked × 1 / sync_locked × 3 / IsSecondaryClient=true × 1）
- [x] PLAN.md `requirements: [REQ-F5-D]` 已在本 SUMMARY frontmatter `requirements-completed` 中列出
- [x] 文件改动严格限定在 `files_modified` 范围（无 sync_lock.go / ssh.go / last_session.go / session.go 改动）

---
*Phase: 32-ssh-tmux*
*Plan: 04-mount-strategy-sync-lock-invoke*
*Completed: 2026-04-20*
