---
phase: 36-sshfs
fixed_at: 2026-04-24T03:32:00Z
fix_scope: critical_warning
findings_in_scope: 5
fixed: 5
skipped: 0
iteration: 1
status: all_fixed
---

# Phase 36：代码审查修复报告

**修复时间：** 2026-04-24
**源审查报告：** `.planning/phases/36-sshfs/36-REVIEW.md`
**修复范围：** Critical + Warning（IN-01..IN-04 已按指示跳过，留待后续 plan）
**迭代轮次：** 1

## 摘要

- 范围内 findings：5（CR-01 + WR-01..WR-04）
- 已修复：5
- 跳过：0
- 状态：`all_fixed`
- 总迭代提交：5（1 commit / finding，全部以 `fix(36): <ID> ...` 开头）
- 全量回归：`go build ./...` 与 `go test ./...` 均通过（48s, 17 包 0 失败）

## 已修复 Findings

### CR-01：HotOnly + 大文件熔断 → 远端文件被静默删除（P0）

**Commit：** `2128389`
**修改文件：**
- `internal/claudedock/hot_sync.go`（applyOversizedFilter / initialSync / syncOnce）
- `internal/claudedock/hot_sync_oversized_test.go`（新增 1 条回归测试）

**应用的修复（同时落地 REVIEW.md 的方案 1 + 方案 3）：**

1. **方案 1（filter 同步清理 base 集）**：`applyOversizedFilter` 现在在 `delete(localFiles, rel)` 旁追加 `delete(e.last, rel)`。这切断了 syncOnce 把「本地不存在 + e.last 存在 + 远端存在」误判为本地删除 → `applyLocal({}, false)` → `deleteRemote` → 远端被静默删除的链路。
2. **方案 3（caller 显式排除 oversized）**：`applyOversizedFilter` 现在返回 `oversizedSet map[string]struct{}`。`initialSync`（非 reset 分支）和 `syncOnce` 拿到该集合后立即 `delete(remoteFiles, rel)`，从源头切断 initialSync 阶段 chooseConflictWinner 命中 `!hasLocal && hasRemote` → `applyRemote` → `copyRemoteToLocal` → 本地大文件被远端旧版本反向覆盖的链路。

**回归测试：** 新增 `TestHotSyncOversized_HotOnly_DoesNotClobberLocalOrDeleteRemote`。
- Fixture：`maxFileBytes=50MB`、`localFiles={big.bin: 60MB}`、`remoteFiles={big.bin: 30MB old}`、`e.last={big.bin: 30MB old base}`，对应 REVIEW.md 列出的 HotOnly + resetRemote=false + Full 模式残留 base 场景。
- 断言（一次跑通 4 项不变量）：
  1. `localFiles` 中 `big.bin` 已被移除（不会推到 hot）。
  2. `e.last` 中 `big.bin` 已被移除（防 syncOnce 误判本地删除 → 远端被删）。
  3. 返回的 `oversizedSet` 含 `big.bin`（防 initialSync 反向覆盖本地）。
  4. `e.oversized` 记录该 60MB 文件供 last-session.json / doctor 复用。
- 修复前模拟该 fixture 会让 `e.last` 仍含 `big.bin`，本测试会失败；修复后通过。
- 测试不依赖 SSH/SFTP 实链路（直接构造 `*HotSyncEngine`），跑得快、依赖最少。

**测试结果：** `TestHotSyncOversized_*` 全部 PASS（`go test ./internal/claudedock -run TestHotSyncOversized -v`）。

---

### WR-02：`Config.HotSyncMaxFileMB` 用户配置失效

**Commit：** `b201390`
**修改文件：** `cmd/claudedock/main.go:352-363`

**应用的修复：** 在 `runRoot` 构造 `mountCfg` 时追加一行（按 REVIEW.md 给出的最小修复字面量）：

```go
HotSyncMaxFileMB: cfg.EffectiveHotSyncMaxFileMB(),
```

确认了字段与方法均存在：
- `claudedock.Config.HotSyncMaxFileMB` 在 `internal/claudedock/config.go:25` 已通过 yaml tag `hot_sync_max_file_mb,omitempty` 解析。
- `Config.EffectiveHotSyncMaxFileMB()` 在 `config.go:40` 实现，零值/负值兜底为 50MB。
- `MountConfig.HotSyncMaxFileMB` 在 `mount_strategy.go:102` 已存在；下游 `effectiveHotSyncMaxFileMB()` 与 `tryModeReal` 均已读取该字段。

**测试结果：** `go build ./...` 通过；现有 `TestHotSyncOversized_*` / `TestExplain*` / doctor 测试无破坏。

---

### WR-01：`checkGitProxyEnabled` / `checkDefaultIgnoreLoaded` 误用 `AUTH_CONFIG_MISSING`

**Commit：** `7a7b419`
**修改文件：**
- `internal/claudedock/errcodes/codes.go`（+2 Code 常量）
- `internal/claudedock/errcodes/mount.go`（+2 MustRegister Entry）
- `internal/claudedock/errcodes/explanations.go`（+2 ExtendedExplanations，均 ≥ 200 中文字符）
- `internal/claudedock/doctor/mount.go`（两处 newWarn 改用新 Code）
- `internal/claudedock/doctor/mount_test.go`（两处断言改为新 Code）

**应用的修复：** 选择 REVIEW.md 给出的「注册新 Code」方案而非「绕开 newWarn 直接构造 Check」方案，原因：
- 让 `errcodes` 注册表在错误码层面也表达正确语义（Severity=Warn，Message 不带占位符）。
- 与 `TestExplainExemptOnlyInformational` / `TestAllCodesHaveExplanations` / `TestAllDomainsClosed` / `TestNoLegacyLowercaseCodes` 等防御性测试自然协同（新 Code 注册即合规）。
- doctor 侧调用更简洁（`newWarn(domain, name, code)` 一行不带 sprintf 参数）。

**新增 Code：**
| Code | Severity | Message | NextAction |
|---|---|---|---|
| `MOUNT_GIT_PROXY_DISABLED` | Warn | proxy_commands 未包含 git，git 子命令不会走本地代理转发 | 编辑 ~/.claudedock/config.yaml 的 proxy_commands 字段加入 git 后重启 claudedock |
| `MOUNT_DEFAULT_IGNORE_DISABLED` | Warn | CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1，默认二进制黑名单已禁用，大文件可能进入热同步 | 如非排查需要，请 unset CLOUD_CLAUDE_NO_DEFAULT_IGNORE 后重启 claudedock |

两条 ExtendedExplanations 各有「触发场景 / 根本原因 / 复现方式 / 修复路径 / 关联文档」五段且均 > 200 中文字符，符合 D-18 契约。

**测试结果：**
- `go test ./internal/claudedock/errcodes/` 全 PASS（含 TestAllCodesHaveExplanations、TestExplainExemptOnlyInformational、TestAllDomainsClosed、TestNoLegacyLowercaseCodes）。
- `go test ./internal/claudedock/doctor/` 全 PASS（含本次更新的 TestCheckGitProxyEnabled_Warn_NoGit、TestCheckDefaultIgnoreLoaded_Warn_Set）。

---

### WR-03：HotOnly 模式下 D-08 stderr 文案「由 cold 兜底」与事实不符

**Commit：** `db62289`
**修改文件：** `internal/claudedock/mount_strategy.go:257-275`

**应用的修复：** 按 REVIEW.md 建议把 fallback 文本按 `mode` 分叉：

```go
fallback := "由 cold sshfs 兜底"
if mode == ModeHotOnly {
    fallback = "未挂载 — 大文件需手工 ssh 进容器读取"
}
fmt.Fprintf(cfg.Logger, "[!] 跳过大文件 %d 个（>%dMB），%s:\n",
    n, cfg.effectiveHotSyncMaxFileMB(), fallback)
```

Full / Auto 路径输出保持原文案，HotOnly 用户拿到准确语义。

**测试结果：** `go build ./...` 通过；现有 `TestMount*` 与 `TestHotSync*` 无破坏。

---

### WR-04：`mount_strategy.go:197` `cfg.IsSecondaryClient = true` 死赋值

**Commit：** `a3d1bd1`
**修改文件：** `internal/claudedock/mount_strategy.go:189-218`

**应用的修复：** 选择方案 1（最小改动，零回归）：删除 `cfg.IsSecondaryClient = true` 这一行；保留并改写注释，明确说明真实副作用由 `ssh.go::ConnectAndRunClaudeV3` 注入的闭包通过闭包捕获 `mountCfg` 完成（指针语义）。

未改造 `MountWorkspace` 接收 `*MountConfig` 的方案 2，原因是该签名变更会触达多个测试 fixture 与一处 v2 兼容入口（`ConnectAndRunClaude`），属于范围外重构。当前 fix 已消除 staticcheck SA4006 / go vet 报警与未来维护者「这行做了什么」的认知摩擦。

**测试结果：** `go build ./...` 通过；现有 `TestMountWorkspace*` / `TestSync*` 全 PASS（行为不变，因为闭包路径才是真正写 caller 的字段）。

---

## 跳过 Findings

无（IN-01..IN-04 按 task 指示不在本次范围）。

## 新增测试

| 测试名 | 路径 | 目的 |
|---|---|---|
| `TestHotSyncOversized_HotOnly_DoesNotClobberLocalOrDeleteRemote` | `internal/claudedock/hot_sync_oversized_test.go` | CR-01 回归：HotOnly + resetRemote=false + 60MB 本地 / 30MB 远端旧版本 + e.last 含 base，断言 filter 同时清空 localFiles/e.last/oversizedSet 三个不变量。 |

## 测试失败与处理

无。每条 fix 提交前都做过：
1. Tier 1 — 重读修改段；
2. Tier 2 — `go build ./...`（每条 fix 后立即跑）；
3. 关键模块 `go test`（hot_sync / errcodes / doctor）。

最后一次全量 `go test ./...`：48.8s，所有 14 个含测试的包 PASS。

## 修复后结构变化（供下一轮 review 参考）

- `applyOversizedFilter` 签名从 `func(...)` 改为 `func(...) map[string]struct{}`，所有 caller（initialSync 非 reset 分支、syncOnce）都已同步更新。
- `errcodes` Registry 新增 2 条 MOUNT 域 Warn Code，依旧符合 8 域闭合（CONTEXT D-23）与 ExplainExempt-only-Info 约束。
- `MountConfig.HotSyncMaxFileMB` 现在有真实 caller（之前是 dead config）；与 `Config.EffectiveHotSyncMaxFileMB()` 形成闭环。
- `MountWorkspace` 的 `cfg.IsSecondaryClient` 死写已删，未来想改造副作用方向需走 `*MountConfig` 重构（不在本次范围）。

---

_Fixed: 2026-04-24_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
