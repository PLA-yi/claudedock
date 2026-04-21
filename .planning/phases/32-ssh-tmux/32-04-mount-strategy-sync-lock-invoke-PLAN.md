---
phase: 32
plan: 04-mount-strategy-sync-lock-invoke
type: execute
wave: 1
depends_on:
  - 03-sync-lock-integration
autonomous: true
gap_closure: true
requirements:
  - REQ-F5-D
files_modified:
  - internal/cloudclaude/mount_strategy.go
  - internal/cloudclaude/mount_strategy_test.go
must_haves:
  truths:
    - "SC11（REQ-F5-D）同一 claude_account 第二个 cloud-claude 启动时，Mutagen sync 不重复创建（账号级单例锁）：mount_strategy.MountWorkspace 在 mountMutagen 之前调用 mountCfg.SyncSessionLock(mountCfg.ClaudeAccountID)，后连端只读 sshfs / mergerfs 视图"
    - "mount_strategy.MountWorkspace 在能力降级之后、tryOrder 循环之前调用 mountCfg.SyncSessionLock(mountCfg.ClaudeAccountID) —— `rg 'mountCfg\\.SyncSessionLock\\(' internal/cloudclaude/mount_strategy.go` 命中 ≥ 1 次（从 Phase 31 D-31 '字段预留' 升级为 '真实 invoke'）"
    - "命中 errors.Is(lockErr, ErrSyncLocked) 时：intended 强制 ModeSSHFSOnly + snapshot.DowngradeChain 追加 DowngradeStep{From: intended.String(), To: ModeSSHFSOnly.String(), ReasonCode: \"sync_locked\", ReasonMessage: \"账号级 Mutagen 单例锁被另一端占用\"} + mountCfg.IsSecondaryClient=true（同时透传到 SessionConfig → last-session.json / 文件注册表 client_role=secondary）"
    - "其它 lockErr（非 ErrSyncLocked）—— 例如 flock 启动失败、SSH session 错误 —— 直接 `return func(){}, ModeFailed, fmt.Errorf(\"sync lock acquire: %w\", lockErr)`，不触发静默降级（M13 防御）"
    - "成功拿到锁 → release 非 nil 时，final cleanup 必须 LIFO 调用 release（sleep infinity 被 kill + flock 解除）；release 在 mount 全栈 cleanup 之后执行（锁生命周期覆盖 mount 全程）"
    - "mountCfg.SyncSessionLock == nil（现网生产路径永不 nil，但测试场景和 anon 路径兼容）时跳过 invoke，不触发降级 —— 保证 mount_strategy_test.go 现有 9 个用例零改动全部通过"
  artifacts:
    - path: "internal/cloudclaude/mount_strategy.go"
      provides: "MountWorkspace 内在能力降级块之后、tryOrder 决策块之前新增 SyncSessionLock invoke + ErrSyncLocked 降级分支；其它逻辑 zero diff"
      contains: "mountCfg.SyncSessionLock("
    - path: "internal/cloudclaude/mount_strategy_test.go"
      provides: "新增 TestMountWorkspace_SyncLocked 用 fake SyncSessionLock 返回 ErrSyncLocked，断言 ActualMode + DowngradeChain + IsSecondaryClient 透传"
      contains: "TestMountWorkspace_SyncLocked"
  key_links:
    - from: "mount_strategy.go::MountWorkspace"
      to: "mount_strategy.go::MountConfig.SyncSessionLock 闭包"
      via: "新增调用 release, lockErr := mountCfg.SyncSessionLock(mountCfg.ClaudeAccountID) —— 修复 Phase 31 D-31 orphan 字段"
      pattern: "mountCfg\\.SyncSessionLock\\("
    - from: "mount_strategy.go::MountWorkspace"
      to: "last_session.go::DowngradeStep{ReasonCode: \"sync_locked\"}"
      via: "applyDowngrade 扩展调用 / 直接 append snapshot.DowngradeChain"
      pattern: "sync_locked"
    - from: "mount_strategy.go::MountWorkspace"
      to: "mount_strategy.go::MountConfig.IsSecondaryClient"
      via: "ErrSyncLocked 时置 true，供 ssh.go SessionConfig 复用原有 line 181 透传链"
      pattern: "IsSecondaryClient\\s*=\\s*true"
---

<plan_dependencies>
- **Plan 03（Wave 3 原始）必须已 ship**：本 plan 直接消费：
  - `cloudclaude.ErrSyncLocked`（Plan 03 Task 3.1）
  - `MountConfig.SyncSessionLock func(accountID string) (release func(), err error)` 字段（Phase 31 D-31 定义，Plan 03 在 ssh.go 注入真实闭包）
  - `MountConfig.IsSecondaryClient` 字段（Plan 03 Task 3.2）
  - `DowngradeStep{From, To, ReasonCode, ReasonMessage}` 结构（Phase 31 已 ship）
  - `applyDowngrade(...)` helper（Phase 31 已 ship —— 本 plan 选择直接 append DowngradeChain + Fprintln 而非复用 applyDowngrade，因为 applyDowngrade 依赖 errcodes.Code 而 "sync_locked" 不是已注册错误码；改走等价手写一段即可，保留与 SESSION_SYNC_LOCKED stderr 输出的独立性）
- **Plan 05（本批次另一个 gap 闭合）与本 plan 无文件重叠**：本 plan 只改 mount_strategy.go / mount_strategy_test.go；Plan 05 只改 session.go / input_buffer.go；两者同 wave 可并行。
- **不**改 Plan 03 的 sync_lock.go / ssh.go 注入闭包：本 plan 的 Gap #2 闭合范围严格限定在 "mount_strategy 调用链落地"，一行 invoke + ErrSyncLocked 分支处理 + 1 个 fake-hook 测试。
</plan_dependencies>

<objective>
闭合 32-VERIFICATION.md Gap #2（SC11 / REQ-F5-D）。

**根因**：`internal/cloudclaude/mount_strategy.go::MountWorkspace` 函数体内**从未调用** `mountCfg.SyncSessionLock(accountID)`。Phase 31 D-31 仅在 `MountConfig` 上预留字段 + Plan 03 在 ssh.go 注入真实 `AcquireSyncLock` 闭包 + `TestIntegration_Phase32_SyncLockMutexes` 能证明锁语义本身正确，但下游 `MountWorkspace` 从未触发该闭包 —— 生产多端启动场景 flock 永远不会被拿到，两端 cloud-claude 仍会并行 mutagen create / sync，**M15 双写风险未被实际防御**。

**修复**：在 `MountWorkspace` 能力降级块（APFS + SupportsMutagen / SupportsMergerfs 校验）之后、tryOrder 决策之前插入：

```go
release, lockErr := mountCfg.SyncSessionLock(mountCfg.ClaudeAccountID)
if errors.Is(lockErr, ErrSyncLocked) {
    // 强制 ModeSSHFSOnly + DowngradeChain 追加 sync_locked + IsSecondaryClient=true
} else if lockErr != nil {
    // 非 ErrSyncLocked 错误（flock 不可用等）—— 传递错误，不静默降级
    return func() {}, ModeFailed, fmt.Errorf("sync lock acquire: %w", lockErr)
}
// 成功分支 → 把 release 包进最终 cleanup LIFO
```

Purpose: 让 Phase 32 SC11 从 code-level ✗ FAILED 转为 ✓ VERIFIED；让 Plan 03 已 ship 的 `ssh.go` 注入闭包 + `IsSecondaryClient` 三层透传链（mount → ssh → session → last-session.json / 文件注册表）获得实际触发路径；M15 多端双写防御在生产路径真实生效。
Output: `mount_strategy.go` ~20 行 diff + `mount_strategy_test.go` ~40 行 1 个新测试用例；零新依赖；Phase 31 既有 9 个 MountWorkspace 测试用例全 PASS（nil-guard 保证）。
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
@.planning/phases/32-ssh-tmux/32-VERIFICATION.md
@.planning/phases/32-ssh-tmux/32-REVIEW.md
@.planning/phases/32-ssh-tmux/plans/03-sync-lock-integration/PLAN.md
@.planning/phases/32-ssh-tmux/plans/03-sync-lock-integration/SUMMARY.md
@internal/cloudclaude/mount_strategy.go
@internal/cloudclaude/mount_strategy_test.go
@internal/cloudclaude/sync_lock.go
@internal/cloudclaude/ssh.go
@internal/cloudclaude/last_session.go

<interfaces>
<!-- Plan 03 已 ship 的关键接口（本 plan 直接消费，不修改）。 -->

internal/cloudclaude/sync_lock.go（Plan 03 Task 3.1）：

```go
// Plan 03 已 ship：accountID == "" → (noop release, nil)；锁被占 → ErrSyncLocked；
// 其它 flock/SSH 错误 → wrap error。
func AcquireSyncLock(conn *ssh.Client, accountID string) (func(), error)

var ErrSyncLocked = errors.New("sync session locked by another cloud-claude")
```

internal/cloudclaude/ssh.go ConnectAndRunClaudeV3（Plan 03 Task 3.2，line 87-111）：

```go
// Plan 03 已 ship：main.go 不设 SyncSessionLock 时，ssh.go 注入真实闭包
if mountCfg.SyncSessionLock == nil {
    mountCfg.SyncSessionLock = func(accountID string) (func(), error) {
        release, err := AcquireSyncLock(connA, accountID)
        if err != nil {
            if errors.Is(err, ErrSyncLocked) {
                mountCfg.IsSecondaryClient = true
                // stderr 输出 [SESSION_SYNC_LOCKED]
            }
            return nil, err
        }
        return release, nil
    }
}
```

internal/cloudclaude/mount_strategy.go MountConfig 现状（Plan 03 Task 3.2 已加 IsSecondaryClient）：

```go
type MountConfig struct {
    // ...
    SyncSessionLock   func(accountID string) (release func(), err error)  // Phase 31 D-31 字段 —— 本 plan 之前 MountWorkspace 从未调用
    IsSecondaryClient bool                                                 // Plan 03 已加；ssh.go 注入闭包在 ErrSyncLocked 时置位
    // ...
}
```

internal/cloudclaude/last_session.go DowngradeStep（Phase 31 ship）：

```go
type DowngradeStep struct {
    From          string `json:"from"`
    To            string `json:"to"`
    ReasonCode    string `json:"reason_code"`     // 注意：字段名是 ReasonCode 不是 Reason
    ReasonMessage string `json:"reason_message"`
}
```
</interfaces>

<current_mount_workspace_layout>
<!-- 定位本 plan 的插入点 —— MountWorkspace 现状 line 129-227（Phase 31）。 -->

```
func MountWorkspace(connA, connB *ssh.Client, cfg MountConfig) (cleanup, finalMode, err) {
    // 1) Logger 默认
    // 2) snapshot 初始化
    // 3) [APFS 检测段]                           ← 现状 line 143-153
    // 4) [能力降级块 — SupportsMutagen / SupportsMergerfs 检查]  ← 现状 line 155-166
    //   （applyDowngrade + intended 更新）
    //
    // ⬇⬇⬇ [本 plan 新增插入点] — SyncSessionLock invoke         ⬇⬇⬇
    //      errors.Is(err, ErrSyncLocked) → 强制 intended = ModeSSHFSOnly
    //         + snapshot.DowngradeChain append DowngradeStep{ReasonCode:"sync_locked"}
    //         + mountCfg.IsSecondaryClient = true
    //      其它 err → return func(){}, ModeFailed, wrapped err
    //      success → syncRelease 放入 cleanup LIFO
    // ⬆⬆⬆                                                         ⬆⬆⬆
    //
    // 5) [决定 tryOrder — Auto / Full / MutagenOnly / SSHFSOnly]  ← 现状 line 168-181
    // 6) [tryMode loop + printProgress + printBanner + MOUNT_AUTO_DOWNGRADED 输出]
    // 7) [成功分支 writeLastSessionWarn + return 模式 cleanup]
}
```

**为什么选这个位置**：
- 必须在 tryOrder 之前 —— 锁拿不到时要求 intended = ModeSSHFSOnly 重新构造 tryOrder
- 必须在能力降级之后 —— 如果 !SupportsMutagen 已经 force ModeSSHFSOnly，再拿锁意义不大（但仍调用，因为 accountID 不空时 noop 代价低且让 last-session 路径记录一致）
- 必须在 printProgress 之前 —— 现状 printProgress 在 tryMode 循环内，无冲突
</current_mount_workspace_layout>
</context>

<tasks>

<task type="auto">
  <name>Task 4.1: MountWorkspace 新增 SyncSessionLock invoke + ErrSyncLocked 降级分支 + 单测</name>
  <files>
    internal/cloudclaude/mount_strategy.go
    internal/cloudclaude/mount_strategy_test.go
  </files>
  <read_first>
    - internal/cloudclaude/mount_strategy.go（line 115-227 MountWorkspace 全函数体；定位能力降级块 line 155-166 末尾 —— 必须在此之后、tryOrder 决策 line 168-181 之前插入 SyncSessionLock invoke）
    - internal/cloudclaude/mount_strategy.go（MountConfig struct line 74-101 —— 确认 SyncSessionLock + IsSecondaryClient 两字段 spelling；确认 applyDowngrade helper line 408-416 签名）
    - internal/cloudclaude/mount_strategy_test.go（现有 9 个测试；注意用例 line 133-160 MountWorkspace_Auto 表驱动用例 / line 187-218 Test_APFSCaseInsensitive_WritesLastSession / line 225-... hooks 注入方式 —— 本 task 新测试严格 mirror hooks 模板，**不污染**现有 table 用例）
    - internal/cloudclaude/sync_lock.go（AcquireSyncLock 签名 + ErrSyncLocked sentinel）
    - internal/cloudclaude/ssh.go line 87-111（Plan 03 已 ship 的 SyncSessionLock 注入闭包 —— 确认生产路径 mountCfg.SyncSessionLock 永不 nil；但本 plan 仍在 MountWorkspace 加 nil-guard 兼容 test 路径）
    - internal/cloudclaude/last_session.go line 32-40（DowngradeStep 字段名精确确认：From / To / ReasonCode / ReasonMessage，**不是** Reason）
    - .planning/phases/32-ssh-tmux/32-VERIFICATION.md gaps[1] 完整 missing[] 列表（本 task 必须闭合 missing#1 + missing#2；missing#3 "可选验证项" 说明 ssh.go 闭包已 IsSecondaryClient=true 因此无需改链路下游）
    - .planning/phases/32-ssh-tmux/plans/03-sync-lock-integration/SUMMARY.md decisions[3]（Plan 03 自述 "严格遵守 user '不重写 mount_strategy' 指令，闭包安装到位即停手；mount_strategy 调用链需由 verify_phase_goal / Phase 31 后续 hotfix 闭环" —— 本 plan 正是该 hotfix）
  </read_first>
  <acceptance_criteria>
    - `go build ./...` 成功
    - `go vet ./internal/cloudclaude/...` 通过
    - `go test ./internal/cloudclaude/... -count=1 -short` PASS（包含现有 9 个 mount_strategy 测试 + 1 新增 TestMountWorkspace_SyncLocked）
    - `rg -n "mountCfg\\.SyncSessionLock\\(" internal/cloudclaude/mount_strategy.go` 命中 ≥ 1 次（**Gap #2 核心断言**）
    - `rg -n "errors\\.Is\\(.*ErrSyncLocked\\)" internal/cloudclaude/mount_strategy.go` 命中 1 次
    - `rg -n "sync_locked" internal/cloudclaude/mount_strategy.go` 命中 ≥ 1 次（ReasonCode 字符串字面值）
    - `rg -n "mountCfg\\.IsSecondaryClient\\s*=\\s*true" internal/cloudclaude/mount_strategy.go` 命中 1 次
    - `go test -run TestMountWorkspace_SyncLocked ./internal/cloudclaude/...` 退出码 0 且用例 PASS
    - `rg -n "func TestMountWorkspace_SyncLocked" internal/cloudclaude/mount_strategy_test.go` 命中 1 次
    - `git diff internal/cloudclaude/mount_strategy.go | grep '^+' | grep -v '^+++' | wc -l` 输出 ≤ 30（最小侵入：插入点 ~15-20 行 + 如需改动 cleanup 包装 ~5 行）
    - Phase 31 既有 9 个测试 `go test -run "TestMountWorkspace_Auto|Test_BannerColors|Test_APFSCaseInsensitive_WritesLastSession|TestMountWorkspace_Force" ./internal/cloudclaude/...` 全 PASS（nil-guard 保证现有用例 SyncSessionLock=nil 不触发新分支）
  </acceptance_criteria>
  <action>
    本 task **单一目标**：闭合 32-VERIFICATION.md Gap #2 —— 让 `mount_strategy.MountWorkspace` 真实调用 `mountCfg.SyncSessionLock(mountCfg.ClaudeAccountID)`。

    ### Step 1 — 定位插入点

    读 `internal/cloudclaude/mount_strategy.go`，找到能力降级块的末尾（现状 line 166）：

    ```go
    if !cfg.SupportsMergerfs && (intended == ModeAuto || intended == ModeFull) {
        applyDowngrade(cfg.Logger, &snapshot, intended, ModeMutagenOnly,
            errcodes.MOUNT_MERGERFS_FAILED, "remote 不支持 mergerfs")
        intended = ModeMutagenOnly
    }

    // ← 插入点在这里（line 167 空行之后、line 169 `// 3) 决定 try 顺序` 之前）
    ```

    ### Step 2 — 新增 SyncSessionLock invoke 代码块

    在上述插入点插入**以下完整代码**（不要简化，不要合并分支，逐字复制）：

    ```go
    // [Phase 32 Gap #2 / REQ-F5-D] 账号级 Mutagen 单例锁 invoke。
    // 闭合 Phase 31 D-31 遗留的 orphan 字段：Plan 03 在 ssh.go 注入 AcquireSyncLock 闭包，
    // 但本函数此前从未真正调用 —— 导致 flock 永不触发，M15 双写防御失效。
    //
    // 三条分支：
    //   1) ErrSyncLocked   → 强制 ModeSSHFSOnly + DowngradeChain 追加 sync_locked + IsSecondaryClient=true
    //   2) 其它 lockErr    → 错误透传（非静默降级，M13 防御）
    //   3) 成功            → syncRelease 挂入 finalCleanup LIFO，mount 全栈退出时释放
    var syncRelease func()
    if cfg.SyncSessionLock != nil {
        release, lockErr := cfg.SyncSessionLock(cfg.ClaudeAccountID)
        if errors.Is(lockErr, ErrSyncLocked) {
            // ssh.go 注入闭包命中 ErrSyncLocked 时已 stderr 输出 [SESSION_SYNC_LOCKED]
            // 并置 cfg.IsSecondaryClient=true；本处再次置位以兼容 main.go 手工注入闭包的场景。
            cfg.IsSecondaryClient = true

            // 强制降级到 SSHFSOnly（只读 sshfs / mergerfs 视图；兼容 anon 场景已在降级块走过）
            if intended != ModeSSHFSOnly {
                // 不复用 applyDowngrade（其参数要求 errcodes.Code；sync_locked 未注册）
                // 直接 append DowngradeStep 并 stderr 输出等价可见性。
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
            // ErrSyncLocked 不挂 release（闭包返回 nil release）
        } else if lockErr != nil {
            // 非 ErrSyncLocked 错误：flock 启动失败 / SSH session 异常 等 —— 错误透传。
            snapshot.ActualMode = ModeFailed.String()
            writeLastSessionWarn(cfg.LastSessionPath, snapshot, cfg.Logger)
            return func() {}, ModeFailed, fmt.Errorf("sync lock acquire: %w", lockErr)
        } else if release != nil {
            // 成功拿锁 —— defer 到 mount 全栈 cleanup 之后（确保 mount 生命周期内锁持续有效）
            syncRelease = release
        }
    }
    ```

    注意要点：
    - **nil-guard**（`cfg.SyncSessionLock != nil`）：现有 9 个 mount_strategy 测试用例不设置该字段，必须保证现有测试零改动 PASS。生产路径 ssh.go 永远注入非 nil 闭包。
    - **ReasonCode / ReasonMessage** 字段名严格匹配 `DowngradeStep` struct（line 32-40 last_session.go）；不要写成 `Reason`。
    - **errors / fmt** 已在 mount_strategy.go 顶部 import（line 3-10 verify），无需新增 import。
    - **不复用 applyDowngrade**：applyDowngrade 签名是 `(w, snap, from, to, code errcodes.Code, reason)` ——  `errcodes.Code` 是注册表枚举，`"sync_locked"` 字符串不匹配。直接 append + Fprintln 两行实现等价语义，避免伪造 errcodes 污染 v3.0 code registry。
    - **stderr 输出**：Plan 03 ssh.go 注入闭包已在命中 ErrSyncLocked 时输出 `[SESSION_SYNC_LOCKED]`；本处再输出一行中文 `[!]` 摘要让 mount_strategy 流程自身可读，双层可见性符合 M13 防御（与 MOUNT_AUTO_DOWNGRADED 行同等级别）。

    ### Step 3 — 把 syncRelease 包入 finalCleanup

    现有 `MountWorkspace` 成功分支（line 190-200）：

    ```go
    printBanner(cfg.Logger, mode, cfg.NoColor)
    if mutagenStatus.ConflictCount > 0 { ... }
    writeLastSessionWarn(cfg.LastSessionPath, snapshot, cfg.Logger)
    return modeCleanup, mode, nil
    ```

    改为：

    ```go
    printBanner(cfg.Logger, mode, cfg.NoColor)
    if mutagenStatus.ConflictCount > 0 { ... }
    writeLastSessionWarn(cfg.LastSessionPath, snapshot, cfg.Logger)

    // [Phase 32 Gap #2] 把 SyncSessionLock release 挂到 mode cleanup 的 LIFO 末尾
    finalCleanup := modeCleanup
    if syncRelease != nil {
        finalCleanup = func() {
            modeCleanup()  // 先卸 mount
            syncRelease()  // 后释放 flock
        }
    }
    return finalCleanup, mode, nil
    ```

    同样地，force-mode-failed 路径（line 206-210）和全档位失败路径（line 221-226）**不**需要 release —— ErrSyncLocked 路径本身 release=nil；非 ErrSyncLocked 错误已在 Step 2 提前 return；成功拿锁但 mount 全档失败时 syncRelease 尚未被 "挂载到 finalCleanup"（因为我们只在成功分支里包装），此时直接 return func(){}, ModeFailed 会泄漏 syncRelease。修正：在 Step 2 成功拿锁后立刻 defer syncRelease() 的兜底，避免漏释放。

    **修正方案（合并 Step 2/3）**：在 Step 2 成功分支改用 `defer-on-failure` 模式：

    ```go
    } else if release != nil {
        syncRelease = release
        // 兜底：确保函数无论从哪个错误路径退出都释放锁
        // （只有成功分支 return modeCleanup + syncRelease 包装时才跳过此 defer）
        // 用 defer + guard 模式：
        var lockReleased bool
        defer func() {
            if !lockReleased && syncRelease != nil {
                syncRelease()
            }
        }()
        // 成功分支里必须设置 lockReleased=true 并把 syncRelease 挂给 finalCleanup
        _ = lockReleased // 提示执行器下一步 wire
    }
    ```

    **实际简化方案**（避免 defer 闭包复杂化函数控制流，执行器优先采纳这个）：

    把 Step 2 成功分支改为**不提前存 syncRelease**，而是**直接用局部变量**，在每个错误路径显式调 `release()`：

    ```go
    } else if release != nil {
        syncRelease = release  // 传出到外层，Step 3 成功分支打包进 finalCleanup
    }
    ```

    然后在：
    - **force-mode 失败 return**（line 206-210）前：`if syncRelease != nil { syncRelease() }`
    - **全档位失败 return**（line 221-226）前：`if syncRelease != nil { syncRelease() }`

    这个简化方案更直观且 diff 行数少。执行器走这条路。

    ### Step 4 — 新增 TestMountWorkspace_SyncLocked 单测

    追加到 `internal/cloudclaude/mount_strategy_test.go` 末尾（复用现有 `newHooks` / `hooks` / `strategyHooks` 模板）：

    ```go
    // TestMountWorkspace_SyncLocked 验证 Gap #2 闭合：
    //   - SyncSessionLock 返回 ErrSyncLocked → intended=ModeFull 强制降级到 ModeSSHFSOnly
    //   - DowngradeChain 含 {From:"full", To:"sshfs-only", ReasonCode:"sync_locked"}
    //   - cfg.IsSecondaryClient = true
    //   - mount 最终走 SSHFSOnly 档位成功（hooks.trySSHFS 返回 nil）
    func TestMountWorkspace_SyncLocked(t *testing.T) {
        var buf bytes.Buffer
        cfg := MountConfig{
            Mode:             ModeFull,
            SupportsMutagen:  true,
            SupportsMergerfs: true,
            ClaudeAccountID:  "test-acct-gap2",
            NoColor:          true,
            Logger:           &buf,
            LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
            SyncSessionLock: func(accountID string) (func(), error) {
                if accountID != "test-acct-gap2" {
                    t.Errorf("SyncSessionLock 收到非预期 accountID: %q", accountID)
                }
                return nil, ErrSyncLocked
            },
            hooks: &strategyHooks{
                trySSHFS: func() (func(), error) { return func() {}, nil },
            },
        }

        cleanup, mode, err := MountWorkspace(nil, nil, cfg)
        if err != nil {
            t.Fatalf("MountWorkspace err 应 nil（降级成功）: %v", err)
        }
        if mode != ModeSSHFSOnly {
            t.Errorf("ErrSyncLocked 必须强制 ModeSSHFSOnly，得 %s", mode)
        }
        if cleanup == nil {
            t.Fatal("cleanup 不应 nil")
        }
        cleanup()

        // 断言 MountConfig.IsSecondaryClient 被置位（ssh.go 会把它透传到 SessionConfig → last-session.json client_role=secondary）
        if !cfg.IsSecondaryClient {
            t.Error("ErrSyncLocked 路径必须置 cfg.IsSecondaryClient=true（Plan 03 透传链依赖此标志）")
        }

        // 断言 last-session.json 落盘 downgrade_chain 含 sync_locked
        data, rerr := os.ReadFile(cfg.LastSessionPath)
        if rerr != nil {
            t.Fatalf("读 last-session.json 失败: %v", rerr)
        }
        var snap LastSessionSnapshot
        if jerr := json.Unmarshal(data, &snap); jerr != nil {
            t.Fatalf("解析 last-session.json 失败: %v", jerr)
        }

        var found bool
        for _, step := range snap.DowngradeChain {
            if step.ReasonCode == "sync_locked" {
                if step.From != "full" {
                    t.Errorf("DowngradeStep.From = %q, want \"full\"", step.From)
                }
                if step.To != "sshfs-only" {
                    t.Errorf("DowngradeStep.To = %q, want \"sshfs-only\"", step.To)
                }
                if step.ReasonMessage == "" {
                    t.Error("DowngradeStep.ReasonMessage 不应为空")
                }
                found = true
            }
        }
        if !found {
            t.Errorf("DowngradeChain 应含 ReasonCode=sync_locked 的 step，实际: %+v", snap.DowngradeChain)
        }

        // 断言 stderr 含中文摘要（双层可见性与 ssh.go [SESSION_SYNC_LOCKED] 并存）
        out := buf.String()
        if !strings.Contains(out, "sync_locked") && !strings.Contains(out, "单例锁") {
            t.Errorf("stderr 应含 sync_locked 或 '单例锁' 摘要，实际: %q", out)
        }
    }

    // TestMountWorkspace_SyncLockSuccess 验证成功分支：SyncSessionLock 返回非 nil release。
    //   - mount 全栈成功（Full 模式）
    //   - cleanup 调用时先卸 mount 再释放锁（LIFO 顺序由 syncRelease 内部计数器断言）
    func TestMountWorkspace_SyncLockSuccess(t *testing.T) {
        var releaseCalled int
        var mergeCleanupCalled int

        cfg := MountConfig{
            Mode:             ModeFull,
            SupportsMutagen:  true,
            SupportsMergerfs: true,
            ClaudeAccountID:  "test-acct-success",
            NoColor:          true,
            Logger:           new(bytes.Buffer),
            LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
            SyncSessionLock: func(accountID string) (func(), error) {
                return func() { releaseCalled++ }, nil
            },
            hooks: &strategyHooks{
                tryMutagen: func() (func(), MutagenSyncStatus, error) {
                    return func() {}, MutagenSyncStatus{}, nil
                },
                trySSHFS: func() (func(), error) { return func() {}, nil },
                tryMerge: func() (func(), error) {
                    return func() { mergeCleanupCalled++ }, nil
                },
            },
        }
        cleanup, mode, err := MountWorkspace(nil, nil, cfg)
        if err != nil {
            t.Fatalf("应成功: %v", err)
        }
        if mode != ModeFull {
            t.Errorf("mode = %s, want full", mode)
        }
        if cfg.IsSecondaryClient {
            t.Error("成功拿锁时 IsSecondaryClient 应保持 false")
        }
        cleanup()
        if releaseCalled != 1 {
            t.Errorf("syncRelease 应被调用 1 次，实际 %d", releaseCalled)
        }
        if mergeCleanupCalled != 1 {
            t.Errorf("mergeCleanup 应被调用 1 次（LIFO 保证在 syncRelease 之前）", )
        }
    }

    // TestMountWorkspace_SyncLockOtherError 验证非 ErrSyncLocked 错误透传（不静默降级）。
    func TestMountWorkspace_SyncLockOtherError(t *testing.T) {
        cfg := MountConfig{
            Mode:             ModeFull,
            SupportsMutagen:  true,
            SupportsMergerfs: true,
            ClaudeAccountID:  "test-acct-err",
            NoColor:          true,
            Logger:           new(bytes.Buffer),
            LastSessionPath:  filepath.Join(t.TempDir(), "last.json"),
            SyncSessionLock: func(accountID string) (func(), error) {
                return nil, fmt.Errorf("flock not installed")
            },
            hooks: &strategyHooks{
                trySSHFS: func() (func(), error) { return func() {}, nil },
            },
        }
        _, mode, err := MountWorkspace(nil, nil, cfg)
        if err == nil {
            t.Fatal("非 ErrSyncLocked 错误应透传，得 nil")
        }
        if mode != ModeFailed {
            t.Errorf("mode = %s, want failed", mode)
        }
        if !strings.Contains(err.Error(), "sync lock acquire") {
            t.Errorf("err 应含 'sync lock acquire'，实际: %v", err)
        }
    }
    ```

    注意：
    - 新增 imports 到 `mount_strategy_test.go`（如缺）：`encoding/json`；`bytes` / `strings` / `filepath` / `os` / `errors` / `fmt` 应已有。
    - `newHooks` / `strategyHooks` / `MutagenSyncStatus` 是现网同包名，直接使用。
    - **不**修改现有 9 个测试用例的任何一行 —— 它们的 SyncSessionLock 字段保持 nil，本 plan 的 nil-guard 让它们完全不走新分支。
  </action>
  <verify>
    <automated>go test ./internal/cloudclaude/... -count=1 -short &amp;&amp; rg -q "mountCfg\\.SyncSessionLock\\(|cfg\\.SyncSessionLock\\(" internal/cloudclaude/mount_strategy.go &amp;&amp; rg -q "errors\\.Is\\(.*ErrSyncLocked\\)" internal/cloudclaude/mount_strategy.go &amp;&amp; rg -q "sync_locked" internal/cloudclaude/mount_strategy.go &amp;&amp; rg -q "func TestMountWorkspace_SyncLocked\\b" internal/cloudclaude/mount_strategy_test.go</automated>
  </verify>
  <done>
    - mount_strategy.MountWorkspace 真实调用 mountCfg.SyncSessionLock(mountCfg.ClaudeAccountID)（grep 验证 ≥ 1 hit —— Gap #2 核心断言）
    - ErrSyncLocked 分支：intended 强制 ModeSSHFSOnly + DowngradeChain 追加 sync_locked step + cfg.IsSecondaryClient=true + stderr 中文摘要
    - 非 ErrSyncLocked 错误透传 return ModeFailed（不静默降级）
    - 成功拿锁 → syncRelease 包装到 finalCleanup LIFO 末尾，mount 退出时释放
    - 3 个新增测试全 PASS（SyncLocked / SyncLockSuccess / SyncLockOtherError）
    - 现有 9 个 mount_strategy 测试零改动全 PASS（nil-guard 保证）
    - mountCfg.IsSecondaryClient 透传链完整：mount_strategy → ssh.go line 181（SessionConfig 构造）已 ship → session.go → last-session.json client_role="secondary" —— 生产多端启动真正防御 M15 双写
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| mountCfg.SyncSessionLock 闭包入口 | 本 plan 新增调用点；accountID 从 mountCfg.ClaudeAccountID 读（Phase 30 AuthResponse 控），无外部输入穿透 |
| DowngradeStep.ReasonCode 字符串常量 | 硬编码 `"sync_locked"`，不走 errcodes 注册表；不进入日志检索命名空间，可读性优先 |
| ErrSyncLocked 错误传递 | 从 sync_lock.go → ssh.go 闭包 → mount_strategy.MountWorkspace → errors.Is 三跳；所有跳都在同进程内存 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-32G2-01 | Tampering | MountWorkspace 的 cfg.IsSecondaryClient 写入 | accept | cfg 是本函数入参 struct 副本；Go 值传递 —— 主调用方 ssh.go::ConnectAndRunClaudeV3 的 mountCfg 同指针吗？实际测：MountWorkspace 签名是 `(connA, connB, cfg MountConfig)` 值传递，主调用方对 mountCfg.IsSecondaryClient 的读取（ssh.go line 181 SessionConfig 构造）**发生在 MountWorkspace 之后**，所以必须确认赋值路径 —— 因为 ssh.go 注入闭包（line 95-110）已经会在命中 ErrSyncLocked 时直接修改 closure-captured mountCfg（指针语义，因为闭包形成）；本 plan 在 MountWorkspace 里再置一次是**双保险**。双保险无害。 |
| T-32G2-02 | DoS | 非 ErrSyncLocked 错误让 cloud-claude 启动失败 | accept | 原设计意图：flock 不可用（极旧镜像）意味着 M15 防御失效，应拒绝启动让用户感知；替代方案"静默降级"违反 M13；该用户体验权衡在 VERIFICATION.md `missing[2]` 明确"其它 lockErr → 错误传递"已认可 |
| T-32G2-03 | Information Disclosure | stderr 摘要泄漏 account_id short 片段 | accept | 与 Plan 03 ssh.go [SESSION_SYNC_LOCKED] 同等级别；account_id 已是 hash；与 Plan 02 tmux session 命名等级一致 |

**Severity 分布（block_on=high）**：
- High: 0
- Medium: T-32G2-01（双保险设计 / accept）/ T-32G2-02（策略决策 / accept）
- Low: T-32G2-03

无 High 项 → 不阻断 plan 执行。

**本 plan 不新增信任边界**（只是让已存在的 closure 真实被触发）；M15 双写风险本是 accept 项，本 plan 是其最终防御，整体安全态势严格改善。
</threat_model>

<verification>
本 plan 完成后，Phase 32 SC11（REQ-F5-D）从 code-level ✗ FAILED 转为 ✓ VERIFIED：

1. **根因修复 grep 断言**：`rg -n "mountCfg\\.SyncSessionLock\\(|cfg\\.SyncSessionLock\\(" internal/cloudclaude/mount_strategy.go` ≥ 1 hit —— 从 Phase 31 "字段预留" 升级为 "真实 invoke"
2. **ErrSyncLocked 降级分支**：TestMountWorkspace_SyncLocked 断言：
   - SyncSessionLock 返回 ErrSyncLocked → mode == ModeSSHFSOnly
   - DowngradeChain 含 `{ReasonCode: "sync_locked", From: "full", To: "sshfs-only"}`
   - cfg.IsSecondaryClient == true
   - stderr 含 "sync_locked" 或 "单例锁"
3. **成功分支 LIFO cleanup**：TestMountWorkspace_SyncLockSuccess 断言：
   - mode == ModeFull
   - cleanup() 调用后 releaseCalled == 1 && mergeCleanupCalled == 1
   - IsSecondaryClient 保持 false
4. **错误透传**（M13 防御）：TestMountWorkspace_SyncLockOtherError 断言：
   - 非 ErrSyncLocked 错误 → mode == ModeFailed + err 含 "sync lock acquire"
5. **Phase 31 回归**：现有 9 个 mount_strategy 测试零改动全 PASS（nil-guard）

**综合 verify 命令**：

```bash
go build ./... \
  && go vet ./internal/cloudclaude/... \
  && go test ./internal/cloudclaude/... -count=1 -short \
  && rg -c "mountCfg\\.SyncSessionLock\\(|cfg\\.SyncSessionLock\\(" internal/cloudclaude/mount_strategy.go \
  && rg -c "errors\\.Is\\(.*ErrSyncLocked\\)" internal/cloudclaude/mount_strategy.go \
  && rg -c "sync_locked" internal/cloudclaude/mount_strategy.go \
  && go test -run "TestMountWorkspace_SyncLocked|TestMountWorkspace_SyncLockSuccess|TestMountWorkspace_SyncLockOtherError" ./internal/cloudclaude/... -v
```

**验收报告推荐路径（闭环 Gap #2 后立即 /gsd-verify-phase 32 重跑）**：本 plan 完成后，再配合 Plan 05 闭合 Gap #1，整个 Phase 32 12 条 SC 在 code-level 应全 VERIFIED；剩余 5 项 human_verification 留 Phase 35 真机 UAT。
</verification>

<success_criteria>
- mount_strategy.go diff ≤ 30 行；仅在 MountWorkspace 内插入（函数签名 zero diff / MountConfig struct zero diff / tryModeReal zero diff / printProgress / printBanner / applyDowngrade / buildSessionName / simpleHash8 全部 zero diff）
- mount_strategy_test.go 追加 3 个新测试函数；现有 9 个测试零改动 PASS
- 零新依赖（go.mod / go.sum 无变化）
- 全平台 `go build ./...` PASS（linux / darwin / windows）
- Gap #2 grep 验证：
  - `rg "mountCfg\\.SyncSessionLock\\(|cfg\\.SyncSessionLock\\(" mount_strategy.go` ≥ 1 hit
  - `rg "errors\\.Is.*ErrSyncLocked" mount_strategy.go` = 1 hit
  - `rg "Reason:.*sync_locked|ReasonCode.*sync_locked|\"sync_locked\"" mount_strategy.go` ≥ 1 hit
  - `go test -run TestMountWorkspace_SyncLocked ./internal/cloudclaude/...` exit 0
- Phase 31 既有所有测试 PASS（证据：`go test ./internal/cloudclaude/... -count=1 -short` 尾部 OK）
</success_criteria>

<output>
完成后，create `.planning/phases/32-ssh-tmux/plans/04-mount-strategy-sync-lock-invoke/SUMMARY.md` 描述：
- MountWorkspace 内新增 SyncSessionLock invoke 的精确 line 号 + before/after diff 摘录
- 3 个新增测试函数的覆盖断言清单
- cfg.IsSecondaryClient 双保险路径（ssh.go line 95-110 闭包 + 本 plan MountWorkspace 行）解释
- Phase 31 既有 9 个 mount_strategy 测试的 PASS 证据（go test -v 尾部 `--- PASS:` 行）
- Gap #2 闭合后 SC11 端到端链路文字描述：`cloud-claude 启动 → MountWorkspace 调 mountCfg.SyncSessionLock → ssh.go 闭包调 AcquireSyncLock → 容器内 flock -n -E 99 → 第二端拿不到 → ErrSyncLocked → MountWorkspace 强制 ModeSSHFSOnly + DowngradeChain + IsSecondaryClient=true → SessionConfig 透传 → last-session.json client_role=secondary`
- 留给 /gsd-verify-phase 的断言清单（Gap #2 完全闭合的 12 条 SC 中第 11 条）
</output>
