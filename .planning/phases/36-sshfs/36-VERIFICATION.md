---
phase: 36-sshfs
verified: 2026-04-24T06:16:25Z
status: passed
score: 12/12 tests passed
---

# Phase 36: 映射前置约束 + sshfs 内核缓存 Verification Report

**Phase Goal:** 把"无约束 mount + 全透传 sshfs"升级为"git 仓库强约束 + 单文件 50MB 熔断 + FUSE page cache 命中"
**Verified:** 2026-04-24T06:16:25Z
**Status:** passed
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | MOUNT_REQUIRE_GIT_REPO 与 MOUNT_OVERSIZED_FILE_SKIPPED 错误码已注册，explain 子进程退出 0 且输出中文长说明 >=200 字 | VERIFIED | TestExplain_MountRequireGitRepo_Exit0_MinLen PASS (1.20s); TestExplain_MountOversizedFileSkipped_Exit0_MinLen PASS (0.01s); explanations.go 实际 rune 计数 754 和 830 |
| 2 | Config.HotSyncMaxFileMB 零值/负值时 EffectiveHotSyncMaxFileMB() 返回 50，正值返回自身 | VERIFIED | config.go: EffectiveHotSyncMaxFileMB() 实现；defaultHotSyncMaxFileMB=50 常量 |
| 3 | LastSessionSnapshot.OversizedFiles 序列化向后兼容，旧版 json 无 oversized_files 字段时不报错 | VERIFIED | TestLastSession_OversizedFiles_Roundtrip/OmitemptyEmpty/OmitemptyNil 3 条全 PASS |
| 4 | HotSyncEngine 单文件熔断：60MB 未 ignore 文件被标记 oversized 并 delete，30MB 正常通过 | VERIFIED | TestHotSyncOversized_60MB_NotIgnored PASS; TestHotSyncOversized_30MB_NotOversized PASS |
| 5 | syncOnce 静默跳过超阈文件，不写入 oversized 记录，不刷屏 stderr | VERIFIED | TestHotSyncOversized_IgnoreHit_NotCounted PASS; hot_sync.go applyOversizedFilter(recordOversized=false) |
| 6 | tryModeReal 在 HotOnly 和 Full 两条路径都注入 MaxFileBytes；mount 成功后 stderr 一次性输出跳过大文件提示 | VERIFIED | mount_strategy.go: 双路径 MaxFileBytes 注入；D-08 stderr 提示块 |
| 7 | 非 git 目录 cloud-claude 立即拒绝挂载，stderr 含 MOUNT_REQUIRE_GIT_REPO，退出码 4 | VERIFIED | git_check.go: requireGitRepo 实现；main.go line 306 调用；TestRequireGitRepo_NotAGitRepo PASS |
| 8 | git 检测时序先于 AuthenticateAndWait（行号 306 < 315） | VERIFIED | main.go: os.Getwd() line 301, requireGitRepo line 306, AuthenticateAndWait line 315 |
| 9 | sshfs 命令含 4 个 FUSE page cache 参数：cache=yes,kernel_cache,auto_cache,cache_timeout=300 | VERIFIED | mount_sshfs.go 字面量命中；顺序在 ConnectTimeout=10 之后、-f 之前 |
| 10 | doctor mount 新增 5 项 check，mount 维度从 4 项扩展到 9 项 | VERIFIED | doctor/mount.go 5 个 checker 函数；doctor.go 5 行 append；mount_test.go 13 条矩阵测试全 PASS |
| 11 | doctor 矩阵测试覆盖 pass/warn/fail/skip 全分支 | VERIFIED | 13 条新测试 + 既有测试全 PASS (go test ./internal/cloudclaude/doctor/... -v) |
| 12 | CI 闸门通过：go test ./... + ci-doctor-grep.sh + uat dry-run | VERIFIED | make ci-gate PASS; 7 PASS, 0 FAIL, 3 SKIP |

**Score:** 12/12 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| internal/cloudclaude/errcodes/codes.go | MOUNT_REQUIRE_GIT_REPO / MOUNT_OVERSIZED_FILE_SKIPPED 常量 | VERIFIED | 两条常量存在 |
| internal/cloudclaude/errcodes/mount.go | MustRegister 两条 Entry | VERIFIED | severity/message/next_action 完整 |
| internal/cloudclaude/errcodes/explanations.go | >=200 字中文长说明 | VERIFIED | 754 rune / 830 rune |
| cmd/cloud-claude/explain_test.go | 两条 explain 子进程测试 | VERIFIED | TestExplain_MountRequireGitRepo / TestExplain_MountOversizedFileSkipped PASS |
| internal/cloudclaude/config.go | HotSyncMaxFileMB + EffectiveHotSyncMaxFileMB() | VERIFIED | 字段 + accessor 完整 |
| internal/cloudclaude/last_session.go | OversizedFiles []OversizedFile omitempty | VERIFIED | 字段存在，schema_version=1 不变 |
| internal/cloudclaude/last_session_test.go | 3 条序列化测试 | VERIFIED | Roundtrip/OmitemptyEmpty/OmitemptyNil PASS |
| internal/cloudclaude/hot_sync.go | applyOversizedFilter + maxFileBytes | VERIFIED | initialSync/syncOnce 双路径调用 |
| internal/cloudclaude/hot_sync_oversized_test.go | 3+ 场景测试 | VERIFIED | 4 条测试全 PASS |
| internal/cloudclaude/mount_strategy.go | MaxFileBytes 注入 + D-08 stderr | VERIFIED | HotOnly/Full 双路径 + snapshot.OversizedFiles |
| cmd/cloud-claude/git_check.go | requireGitRepo(cwd) | VERIFIED | 非 git / git 不可用统一处理 |
| cmd/cloud-claude/git_check_test.go | 3 场景单测 | VERIFIED | InGitRepo/NotAGitRepo/GitUnavailable PASS |
| cmd/cloud-claude/main.go | os.Getwd 前移 + requireGitRepo 调用 | VERIFIED | line 301/306 < 315 |
| internal/cloudclaude/mount_sshfs.go | 4 个 cache 参数 | VERIFIED | cache=yes,kernel_cache,auto_cache,cache_timeout=300 |
| internal/cloudclaude/mount_sshfs_test.go | fixture SFTP counting 测试 | VERIFIED | TestSSHFSCacheHitsKernelPageCache SKIP(Darwin)/PASS(Linux) |
| internal/cloudclaude/doctor/mount.go | 5 个 checker 函数 | VERIFIED | 10 处 grep 命中(5定义+5引用) |
| internal/cloudclaude/doctor/doctor.go | 5 项 check 注册 | VERIFIED | 5 行 append |
| internal/cloudclaude/doctor/mount_test.go | 13 条矩阵测试 | VERIFIED | 全 PASS |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| main.go:runRoot | git_check.go:requireGitRepo | os.Getwd() + requireGitRepo(cwd) | WIRED | line 301 -> 306 |
| mount_strategy.go:tryModeReal | hot_sync.go:StartHotSync | MaxFileBytes 注入 | WIRED | HotOnly + Full 双路径 |
| hot_sync.go:applyOversizedFilter | mount_strategy.go:MountWorkspace | HotSyncStatus.OversizedFiles | WIRED | snapshot.OversizedFiles = hotStatus.OversizedFiles |
| doctor/mount.go:checkRequireGitRepo | git_check.go | exec.Command("git", "rev-parse") | WIRED | 同语义，doctor 包独立实现 |
| doctor/mount.go:checkOversizedFilesCount | last_session.go | LoadLastSession() | WIRED | 真实读取 ~/.cloud-claude/last-session.json |
| doctor/mount.go:checkSSHFSCacheArgs | mount_sshfs.go | sshfsCmd 字面量 grep | WIRED | runner.RunScript + strings.Contains |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| config.go | HotSyncMaxFileMB | config.yaml 或零值 | Yes - EffectiveHotSyncMaxFileMB() 兜底 50 | FLOWING |
| hot_sync.go | maxFileBytes | MountConfig.MaxFileBytes | Yes - 50MB 默认值或用户配置 | FLOWING |
| mount_strategy.go | snapshot.OversizedFiles | hotStatus.OversizedFiles | Yes - initialSync 真实记录 | FLOWING |
| last_session.go | oversized_files | snapshot.OversizedFiles | Yes - WriteLastSession 真实写入 json | FLOWING |
| doctor/mount.go | checkOversizedFilesCount | LoadLastSession() | Yes - 读真实 last-session.json | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Go build compiles | go build ./... | Clean, no errors | PASS |
| Go vet passes | go vet ./... | Clean, no warnings | PASS |
| Explain tests | go test ./cmd/cloud-claude/... -run TestExplain_Mount | 3/3 PASS | PASS |
| Git check tests | go test ./cmd/cloud-claude/... -run TestRequireGitRepo | 3/3 PASS | PASS |
| LastSession tests | go test ./internal/cloudclaude/... -run TestLastSession_OversizedFiles | 3/3 PASS | PASS |
| HotSync oversized tests | go test ./internal/cloudclaude/... -run TestHotSyncOversized | 4/4 PASS | PASS |
| Doctor mount tests | go test ./internal/cloudclaude/doctor/... -v | 67/67 PASS (4 SKIP) | PASS |
| Full test suite | go test ./... -count=1 -short | All PASS | PASS |
| CI gate | make ci-gate | PASS (7 UAT PASS, 0 FAIL, 3 SKIP) | PASS |
| sshfs cache args | grep mount_sshfs.go | 4 参数命中 | PASS |
| Git timing | grep -n main.go | 306 < 315 | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| REQ-MOUNT-V31-01 | 36-04 | 非 git 仓库拒绝挂载 | SATISFIED | requireGitRepo + main.go 时序 + 3 条单测 |
| REQ-MOUNT-V31-02 | 36-02, 36-03 | 单文件大小熔断 | SATISFIED | HotSyncMaxFileMB + applyOversizedFilter + 4 条测试 |
| REQ-MOUNT-V31-03 | 36-02, 36-03 | oversized_files 持久化 | SATISFIED | LastSessionSnapshot.OversizedFiles + 3 条序列化测试 |
| REQ-MOUNT-V31-04 | 36-05 | sshfs FUSE page cache | SATISFIED | mount_sshfs.go 4 参数 + fixture 测试 |
| REQ-MOUNT-V31-05 | 36-06 | doctor mount +5 check | SATISFIED | 5 checker + 13 矩阵测试 + CI gate |
| REQ-MOUNT-V31-06 | 36-01 | 错误码注册 + explain | SATISFIED | 2 条 Code + MustRegister + explanations + 2 子进程测试 |

**All 6/6 requirements SATISFIED.** Zero orphaned requirements.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | - | - | - | - |

No TODO/FIXME/placeholder comments, no empty implementations, no hardcoded empty data found in any phase 36 production files.

### Gaps Summary

No gaps found. All 12 observable truths verified. All 6 requirements satisfied. All 19 artifacts exist and are properly wired. All unit tests pass. CI gate passes.

---

_Verified: 2026-04-24T06:16:25Z_
_Verifier: Claude (gsd-verifier)_
