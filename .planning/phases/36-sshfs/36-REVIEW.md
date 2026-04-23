---
phase: 36-sshfs
reviewed: 2026-04-23T13:00:00Z
depth: standard
files_reviewed: 18
files_reviewed_list:
  - internal/cloudclaude/errcodes/codes.go
  - internal/cloudclaude/errcodes/mount.go
  - internal/cloudclaude/errcodes/explanations.go
  - internal/cloudclaude/config.go
  - internal/cloudclaude/last_session.go
  - internal/cloudclaude/last_session_test.go
  - internal/cloudclaude/hot_sync.go
  - internal/cloudclaude/hot_sync_oversized_test.go
  - internal/cloudclaude/mount_strategy.go
  - internal/cloudclaude/mount_sshfs.go
  - internal/cloudclaude/mount_sshfs_test.go
  - internal/cloudclaude/doctor/mount.go
  - internal/cloudclaude/doctor/doctor.go
  - internal/cloudclaude/doctor/mount_test.go
  - cmd/cloud-claude/git_check.go
  - cmd/cloud-claude/git_check_test.go
  - cmd/cloud-claude/main.go
  - cmd/cloud-claude/explain_test.go
findings:
  critical: 1
  warning: 4
  info: 4
  total: 9
status: issues_found
---

# Phase 36：代码审查报告

**审查时间：** 2026-04-23
**审查深度：** standard
**审查文件数：** 18
**结论：** issues_found（含 1 项 P0 数据丢失风险，强烈建议下个 plan 修复后再合入主干）

## 摘要

Phase 36 在错误码、配置、单文件熔断、sshfs 缓存、git 前置闸门、doctor 体检面五个维度都按 plan 落地，结构与 PATTERNS 一致：

- 错误码注册唯一性、Severity / NextAction 一致性、explanations 长说明 200+ 字符的契约都成立。
- `LastSessionSnapshot.OversizedFiles` 用 `omitempty` 保持 schema_version=1 向后兼容，三条 round-trip 测试覆盖到位。
- `requireGitRepo` 时序前移到 `LoadConfig` 与 `NewEntryClient` 之间，命中 REQ-MOUNT-V31-01 字面要求，单测覆盖三场景。
- doctor mount 维度从 4 项提升到 9 项，新增 5 项 check 全部有矩阵测试覆盖（PASS / Fail / Warn / Skip 路径俱全）。
- sshfs 命令行追加 4 个 page cache 参数，`mount_sshfs_test.go` 用真实 SFTP 计数 fixture 验证 kernel page cache 第二次命中不回源。

但**单文件熔断在 HotOnly 模式下与双向同步状态机的交互存在 P0 级静默数据丢失**，详见 CR-01；另有 4 项 Warning 与 4 项 Info 影响生产可观测性与 UX 一致性。

## 关键 Issues

### CR-01：HotOnly 模式 + 大文件熔断 → 远端文件被静默删除（P0 数据丢失）

**File：** `internal/cloudclaude/hot_sync.go:421-433`（`applyOversizedFilter`）+ `internal/cloudclaude/hot_sync.go:242-326`（`syncOnce`）+ `internal/cloudclaude/hot_sync.go:162-224`（`initialSync` 非 reset 分支）

**Issue：**
`applyOversizedFilter` 仅 `delete(localFiles, rel)`，但**没有同步从 `e.last` 中移除 `rel`**。这在 HotOnly 模式（`resetRemote=false`，热同步根目录就是 `cfg.Cwd`）下产生两段式静默数据丢失：

1. **initialSync 阶段（首次启动）**：
   - 本地存在 60MB `big.bin`，且远端同路径已存在该文件的旧版本（典型场景：用户先用 Full 模式同步过，再切 HotOnly；或之前 cold sshfs 写入过）
   - filter 把 `big.bin` 从 `localFiles` 删除 → `hasLocal=false, hasRemote=true`
   - `chooseConflictWinner` 命中 `!hasLocal && hasRemote` 分支 → 返回 `"remote"`
   - `applyRemote(rel, remoteState, true)` → `copyRemoteToLocal` → **本地 big.bin 被远端旧版覆盖（用户最近的本地编辑丢失）**
   - `e.last[rel] = remoteState`（big.bin 进入 base 集）

2. **运行期 syncOnce（启动 1 秒后）**：
   - 同样 filter 把 `big.bin` 从 `localFiles` 移除 → `hasLocal=false`
   - 但 `e.last[big.bin]` 仍存在（base） → `localChanged = !sameSyncState({}, false, base, true) = true`
   - `remoteFiles[big.bin]` 与 `e.last` 一致 → `remoteChanged = false`
   - 命中 `case localChanged && !remoteChanged` → `applyLocal(rel, {}, false)` → `deleteRemote(rel)` → **远端 big.bin 被删除**
   - `next` 不写入该 rel → 下一轮三方都缺失 → 文件在远端被永久消失

**触发条件：**
- `--mount-mode=hot-only`（显式），或 `auto + SupportsMergerfs=false` 自动降级到 HotOnly
- cwd 中存在 ≥ `hot_sync_max_file_mb` 的文件
- 该文件未被 `.gitignore` / mount-ignore 命中（典型：模型权重、视频、构建产物、tarball）
- 远端同路径已有该文件的副本

**为何 Full 模式不受影响：** Full 走 `resetRemote=true` + 隐藏 hot staging（`/tmp/.cloud-claude-mounts/...`），`big.bin` 既未被上传到 hot 也不进入 `e.last`，状态机在 base / local / remote 三方都缺失，行为稳定。

**与 plan 设计意图的差距：** `MOUNT_OVERSIZED_FILE_SKIPPED` 的语义是「被 hot 跳过、由 cold 兜底」，REQUIREMENTS 也明确「文件依然可读，只是不会进入 hot tree」。当前实现却让 HotOnly 模式下的大文件**先被反向覆盖再被远端删除**，与 explain 长说明（`explanations.go:127-130`）公开承诺的语义直接相反。

**Fix（择一即可，建议 1 + 3 同时落地）：**

```go
// 方案 1（最小改动，零回归）：filter 同时清理 e.last，让大文件对状态机彻底「不存在」
func (e *HotSyncEngine) applyOversizedFilter(localFiles map[string]syncFileState, recordOversized bool) {
    if e.maxFileBytes <= 0 {
        return
    }
    for rel, state := range localFiles {
        if state.Size >= e.maxFileBytes {
            if recordOversized {
                e.oversized = append(e.oversized, OversizedFile{Path: rel, SizeBytes: state.Size})
            }
            delete(localFiles, rel)
            // CR-01 修复：从 base 集移除，避免在 syncOnce 中被误判为「本地删除」
            // 同时需要在 syncOnce 内对 remoteFiles 也跳过该 rel（见方案 3）
            delete(e.last, rel)
        }
    }
}

// 方案 3（杜绝 initialSync 反向覆盖）：把 oversized 集合显式传入 paths union 排除
//   在 initialSync 与 syncOnce 内构造 oversized set，从 paths / remoteFiles 中预先剔除
oversizedSet := make(map[string]struct{}, len(e.oversized))
for _, f := range e.oversized {
    oversizedSet[f.Path] = struct{}{}
}
for rel := range remoteFiles {
    if _, skip := oversizedSet[rel]; skip {
        delete(remoteFiles, rel)
    }
}
```

**回归测试（强烈建议补一条）：**
- HotOnly + resetRemote=false 路径下，本地 60MB / 远端同路径有 30MB 旧版本的 fixture，断言 `initialSync` 后本地 `big.bin` size 仍为 60MB（未被覆盖）、`syncOnce` 后远端 `big.bin` 仍存在（未被删除）

**Why critical：** HotOnly 是 Auto + 不支持 mergerfs 的兜底档，并非纯人工档；触发场景在「用户首次提交大文件 + 之前 Full 模式残留」非常常见，且失败链路完全静默（stderr 仅有一次「跳过大文件」提示，看不到反向覆盖与远端删除）。

## Warnings

### WR-01：`checkGitProxyEnabled` / `checkDefaultIgnoreLoaded` 误用 `AUTH_CONFIG_MISSING`，触发误导性 stderr 文案

**File：** `internal/cloudclaude/doctor/mount.go:236-257`

**Issue：**
两处 `newWarn(...)` 都把 `errcodes.AUTH_CONFIG_MISSING` 当 Code 传入，但该 Code 在 `errcodes/auth.go:9-13` 注册为：

- `Severity: SeverityFatal`
- `Message: "~/.cloud-claude/config.yaml 不存在或解析失败: %s"`
- `NextAction: "运行 cloud-claude init 重新配置网关与凭证"`

而 `newWarn` 的实现（`doctor/check.go:71-78`）会用 `fmt.Sprintf(entry.Message, args...)` 渲染。结果：

- `proxy_commands` 不含 git 时，doctor 输出："~/.cloud-claude/config.yaml 不存在或解析失败: proxy_commands 未包含 git"，并建议「运行 cloud-claude init」——而真实 config 完好、需要的是手工编辑 yaml 加 git。
- `CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1` 时，输出："~/.cloud-claude/config.yaml 不存在或解析失败: CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1，已禁用默认二进制黑名单"，同样语义错乱。

此外 Severity 双重失衡：注册值是 Fatal，doctor 用 StatusWarn——`explanations.go::ExplainExempt` 与 `TestExplainExemptOnlyInformational` 等防御性测试可能后续会因此误报。

**Fix：** 在 `errcodes/mount.go` 注册两个新 Code（建议命名 `MOUNT_GIT_PROXY_DISABLED` / `MOUNT_DEFAULT_IGNORE_DISABLED`，Severity=Warn，Message 不带占位符或自定义占位符），让 doctor 的两条 check 各用专属 Code。或者退化方案：直接构造 `Check{Status: StatusWarn, Code: ..., Message: "...", NextAction: "..."}`，绕开 newWarn 的 sprintf 拼接，并把 NextAction 改成准确的「编辑 ~/.cloud-claude/config.yaml 的 proxy_commands 字段」。

### WR-02：`Config.HotSyncMaxFileMB` 用户配置目前完全失效（main.go 未注入 mountCfg）

**File：** `cmd/cloud-claude/main.go:352-357` + `internal/cloudclaude/ssh.go:54-110` + `internal/cloudclaude/config.go:38-45`

**Issue：**
`Config.EffectiveHotSyncMaxFileMB()` 在生产代码中**没有任何调用方**：
- `main.go::runRoot` 构造 `mountCfg` 时，只设置 `Mode / KeepAliveInterval / KeepAliveCountMax / NoColor / SessionTakeOver / SessionShortID / LocalHostname`，未透传 `cfg.HotSyncMaxFileMB`。
- `ssh.go::ConnectAndRunClaudeV3` 在补全 `mountCfg` 字段时，也未读取 cfg 中的该字段。
- `MountConfig.effectiveHotSyncMaxFileMB()`（私有 accessor）兜底为 `mountDefaultHotSyncMaxFileMB=50`，**主入口永远走默认 50MB**。

结果：用户在 `~/.cloud-claude/config.yaml` 写 `hot_sync_max_file_mb: 200` 不会生效；启动也不会有任何提示。这与 `MOUNT_OVERSIZED_FILE_SKIPPED` 长说明（`explanations.go:129`）告诉用户「编辑 ~/.cloud-claude/config.yaml 调高 hot_sync_max_file_mb」直接矛盾。

`36-03-SUMMARY.md` 第 141 行已把这条接线列入「Future Work」，所以 36-03 plan 范围内不算 deviation；但**本审查阶段必须把这件事作为 Warning 暴露给主干**，否则整个 36-02 引入的 yaml 字段是 dead config，且会让 doctor 的 `oversized_files_count` Warn 在生产无法被用户自主消除。

**Fix（最小改动）：**
```go
// cmd/cloud-claude/main.go runRoot 内构造 mountCfg 后：
mountCfg.HotSyncMaxFileMB = cfg.EffectiveHotSyncMaxFileMB()
```
并在 36-04（或新开 36-07）补一条端到端验证：写 `hot_sync_max_file_mb: 100` 到 config，60MB 文件应不被 oversized 跳过。

### WR-03：HotOnly 模式下 D-08 stderr 文案「由 cold 兜底」与运行时事实不符

**File：** `internal/cloudclaude/mount_strategy.go:257-264`

**Issue：**
HotOnly 模式没有 cold sshfs 层（`tryModeReal` 在 `mode == ModeHotOnly` 分支只调一次 `StartHotSync` 直接挂在 `cfg.Cwd`，`mode_strategy.go:422-432`）。但成功路径打印的提示固定是：

```go
fmt.Fprintf(cfg.Logger, "[!] 跳过大文件 %d 个（>%dMB），由 cold 兜底:\n", n, cfg.effectiveHotSyncMaxFileMB())
```

Hot-only 用户根本没有「cold 兜底」可言，被熔断的文件实际只能通过用户手工 ssh 进容器或 sshfs 单独挂载读取。`36-03-SUMMARY.md` 也承认「与 D-08 文案在 HotOnly 模式存在轻微语义不一致」。

**Fix：** 把渲染逻辑按 `mode` 分叉：
```go
fallback := "cold sshfs"
if mode == ModeHotOnly {
    fallback = "未挂载 — 大文件需手工 ssh 进容器读取"
}
fmt.Fprintf(cfg.Logger, "[!] 跳过大文件 %d 个（>%dMB），%s:\n", n, cfg.effectiveHotSyncMaxFileMB(), fallback)
```

CR-01 修好后，本条 Warning 仍需独立修复（信息真实性问题）。

### WR-04：`mount_strategy.go:197` 给 cfg.IsSecondaryClient 赋值的语句无副作用且容易误导后续维护者

**File：** `internal/cloudclaude/mount_strategy.go:189-218`

**Issue：**
`MountWorkspace(cfg MountConfig)` 是值传参，第 197 行 `cfg.IsSecondaryClient = true` 仅修改本函数内的局部副本，函数返回后调用方（`ssh.go::ConnectAndRunClaudeV3`）拿到的依然是原值。注释里已经承认「`cfg` 是值传递；本函数对 `cfg.IsSecondaryClient` 的赋值仅作为契约文档」，但代码层面这就是死赋值——`go vet -copylocks`、`staticcheck SA4006` 都会标记。真正的副作用走 `ssh.go::ConnectAndRunClaudeV3` 闭包里直接修改外层 `mountCfg.IsSecondaryClient`（`ssh.go:99`）。

虽然 36 阶段没有改这条线，但 36-CONTEXT 的 mount_strategy 透传部分已经依赖该字段；建议改成显式对 `*MountConfig` 操作或干脆删除这行赋值，避免未来误以为它会传出来。

**Fix（择一）：**
1. 删除第 197 行赋值语句，注释保留「真实副作用由 ssh.go 闭包写入」即可。
2. 把 `MountWorkspace` 改成接收 `*MountConfig`，并把 `cfg.IsSecondaryClient = true` 改成对 `cfg` 指针字段写入，让代码与设计意图自洽。

> 不阻塞合入；列为 WR 因它和 CR-01 都属于「热同步状态机的副作用语义不清」。

## Info

### IN-01：`applyOversizedFilter` 阈值 `>=` 与错误码文案 `超过` 存在边界口径不一致

**File：** `internal/cloudclaude/hot_sync.go:425-433` + `internal/cloudclaude/errcodes/mount.go:106-111`

**Issue：** 代码用 `state.Size >= e.maxFileBytes`，但 `MOUNT_OVERSIZED_FILE_SKIPPED` Message 写的是「超过 hot_sync_max_file_mb=%d 阈值」。当文件大小恰等于阈值（如默认 50MB = 52,428,800 字节）时会被熔断，但用户语义上的「超过」是 `>`。

**Fix：** 二选一保持口径一致：
- 修代码：`state.Size > e.maxFileBytes`（更贴近「超过」字面）
- 修文案：「达到或超过 hot_sync_max_file_mb=%d 阈值」

### IN-02：`doctor.gitRevParseTopLevel` 与 `cmd/cloud-claude.requireGitRepo` 重复实现 4 行 exec 逻辑

**File：** `internal/cloudclaude/doctor/mount.go:152-158` + `cmd/cloud-claude/git_check.go:14-21`

**Issue：** 注释已说明 doctor 不能 import main package 而做了刻意复制。两份实现的退出码语义、PATH 解析行为完全一致，但当未来调整（例如加 `-c safe.directory='*'` 或 `--no-optional-locks`）时容易遗漏一边。

**Fix：** 把 `requireGitRepo` 的核心 4 行（exec + 错误吞并）下沉到 `internal/cloudclaude/gitcheck`（新包，仅依赖 stdlib + errcodes），让 doctor 与 main 都 import；保留各自层的 stderr / Check 渲染。可放到 36-07 或 v3.1 backlog。

### IN-03：`convertSnapshotToBanner` 没有把 `OversizedFiles` 透传到 `DowngradeBanner`

**File：** `internal/cloudclaude/doctor/doctor.go:271-293`

**Issue：** doctor 第一屏 banner 显示 conflict_count / reconnect_count / tmux_session 等，但忽略了同样属于「上次会话状态」的 oversized_files 计数。当前用户拿到的信号只在 mount 维度的 `checkOversizedFilesCount`（带 `--domain mount` 才必现）；如果用户跑 `cloud-claude doctor --domain network` 看不到这条线索，会与「为什么我的大文件没同步」的工单脱钩。

**Fix：** 在 `DowngradeBanner` 增加 `OversizedFilesCount int json:"oversized_files_count,omitempty"`，由 `convertSnapshotToBanner` 填 `len(snap.OversizedFiles)`。`omitempty` 保证 0 时不污染 JSON。属于体验改善，不阻塞合入。

### IN-04：`mount_strategy.go::extractErrCodeAndReason` 未识别的错误一律落到 `MOUNT_FORCE_MODE_FAILED`

**File：** `internal/cloudclaude/mount_strategy.go:529-535`

**Issue：** Auto 模式下，如果某档错误未实现 `codedError` 接口，扣回的 fallback Code 是 `MOUNT_FORCE_MODE_FAILED`，但 Auto 模式根本就不是 force——`applyDowngrade` 拼出的降级提示会带误导性 Code。当前 plan 范围内的 hot_sync_err / sshfs / mergerfs 都实现了 `codedError`，所以暂时观测不到，但属于隐患。

**Fix（v3.1 backlog 即可）：** 增加一个通用 `MOUNT_INTERNAL_UNCLASSIFIED` Code 兜底，或在 Auto 路径直接用 `mode.String() + " 层未知失败"` 不带 Code 渲染。

---

_Reviewed: 2026-04-23_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
