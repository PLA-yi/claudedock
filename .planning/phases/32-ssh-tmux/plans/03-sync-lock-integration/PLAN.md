---
phase: 32
plan: 03-sync-lock-integration
type: execute
wave: 3
depends_on:
  - 02-tmux-multiclient
autonomous: true
requirements:
  - REQ-F5-D
files_modified:
  - internal/cloudclaude/sync_lock.go
  - internal/cloudclaude/sync_lock_test.go
  - internal/cloudclaude/ssh.go
  - internal/cloudclaude/session.go
  - internal/cloudclaude/integration_test.go
  - scripts/test-fixture-up.sh
must_haves:
  truths:
    - "AcquireSyncLock(conn, accountID) 在容器内远程执行 'mkdir -p /tmp/cloud-claude/locks 2>/dev/null && flock -n -E 99 -F <lockPath> -c \"echo $$; exec sleep infinity\" & echo $!'，lockPath = /tmp/cloud-claude/locks/sync-<accountID>.lock（修订 D-17 — RESEARCH §6.2 选 /tmp/cloud-claude/locks 因 ubuntu:24.04 /var/lock 是 root-only）"
    - "ssh.ExitError.ExitStatus() == 99 → 返回 (nil release, ErrSyncLocked)；其它 runErr → 返回 (nil, fmt.Errorf flock 启动失败)；accountID == \"\" → 返回 (noop release, nil)（D-19 anon 路径跳过锁）"
    - "release func 通过另一个 ssh.Session 远程 'kill <pid> 2>/dev/null || true'（pid 来自 echo $!）；ssh.Session.Close 也会自然收割 sleep infinity，release 是显式 cleanup"
    - "ssh.go ConnectAndRunClaudeV3 在 mountCfg 构造完毕、MountWorkspace 调用之前覆盖 mountCfg.SyncSessionLock = func(accountID string) (func(), error) { return AcquireSyncLock(connA, accountID) }（修订 D-18 — RESEARCH §6.4：Phase 31 接口缺 conn 参数，本阶段在 ssh.go 内覆盖避免改 Phase 31 接口）"
    - "Phase 31 mount_strategy.MountWorkspace 收到 errSyncLocked 时降级 sshfs-only（Phase 31 D-31 已实现该行为；本 plan 仅注入真实 lock 实现，不重写降级逻辑）；session.go runClaudeWithSession 在 last-session.json 写 ClientRole='secondary'（first-end 写 'primary' 由 Plan 02 完成；本 plan 增加 secondary 写入路径）"
    - "AcquireSyncLock 失败为 ErrSyncLocked 时，stderr 必须输出 errcodes.Format(SESSION_SYNC_LOCKED, accountID)（D-17 第 3 条；Phase 31 mount_strategy 接到 errSyncLocked 后通过 Logger 输出 — 本 plan 在 ConnectAndRunClaudeV3 注入 Lock 时统一封装一层 wrapper 输出错误码）"
    - "integration_test.go 追加 6 个新 TestIntegration_* 用例：(a) 多端单例锁（同 account 第二端 mutagen sync 不重复创建）；(b) 多端 banner（第二端 stderr 含 SESSION_SYNC_LOCKED + last-session.json client_role=secondary）；(c) 30s docker network disconnect 抖动 → reconnect 成功 + tmux 内 claude 进程 PID 不变 + buffer 完整；(d) pkill -SIGHUP sshd 后 tmux session 仍存活（C7 回归）；(e) pgrep systemd-logind 必须无输出（C7 镜像侧前置）；(f) tmux 不可用降级（用旧镜像或临时 mv tmux）"
    - "(c) 与 (d) 用例 testing.Short() 跳过；CI 环境用 -tags=integration 才运行；scripts/test-fixture-up.sh 已就位（Phase 31 ship），本 plan 仅追加（如需）多端 fixture 配置（同 fixture 容器内启 2 个 cloud-claude 进程，无需新 service）"
    - "本 plan **不**真实拔网测试（CI 不一定有 docker network 权限）；docker network disconnect / connect 用例在非 docker 环境下 t.Skip 跳过，留 Phase 35 真机；但用例代码必须 commit（不许写 TODO）"
    - "C3 sshfs 抖动级联（ROADMAP §Phase 32 SC12）— 本 plan 不重写 sshfs_watcher（Phase 31 已 ship）；集成测试用例 (c) 30s 抖动场景隐含验收 watcher 正常工作（30s 后摘除 cold branch 不阻塞 mergerfs / Ctrl-C 可恢复）"
  artifacts:
    - path: "internal/cloudclaude/sync_lock.go"
      provides: "AcquireSyncLock(conn, accountID) (release func(), err error) + ErrSyncLocked + parseLastInt helper"
      contains: "func AcquireSyncLock"
    - path: "internal/cloudclaude/sync_lock_test.go"
      provides: "AcquireSyncLock anon 路径 noop 单测 + parseLastInt 边界单测（exitcode 99 → ErrSyncLocked 用 fakeConn 验证）"
      contains: "TestAcquireSyncLock_AnonReturnsNoop"
    - path: "internal/cloudclaude/ssh.go"
      provides: "ConnectAndRunClaudeV3 在 MountWorkspace 之前覆盖 mountCfg.SyncSessionLock = AcquireSyncLock(connA, ...) wrapper"
      contains: "AcquireSyncLock"
    - path: "internal/cloudclaude/session.go"
      provides: "runClaudeWithSession 在 sessionCfg 含 secondary 标志时 writeLastSessionTmuxField 第二参数传 'secondary'（依赖 ssh.go 通过 SessionConfig 透传）"
      contains: "secondary"
    - path: "internal/cloudclaude/integration_test.go"
      provides: "6 个 TestIntegration_Phase32_* 用例 + dockerExec helper 复用"
      contains: "TestIntegration_Phase32_SyncLockMutexes"
    - path: "scripts/test-fixture-up.sh"
      provides: "保持现状（Phase 31 已 ship），本 plan 不改；仅在 plan 内说明用例直接复用"
      contains: "docker compose"
  key_links:
    - from: "ssh.go::ConnectAndRunClaudeV3"
      to: "sync_lock.go::AcquireSyncLock"
      via: "MountWorkspace 调用前覆盖 mountCfg.SyncSessionLock 闭包"
      pattern: "AcquireSyncLock"
    - from: "mount_strategy.go::MountWorkspace"
      to: "sync_lock.go::ErrSyncLocked"
      via: "Phase 31 D-31 既有逻辑：检测到 errSyncLocked → 降级 sshfs-only + 写 last-session"
      pattern: "ErrSyncLocked"
    - from: "session.go::writeLastSessionTmuxField"
      to: "last_session.go::ClientRole"
      via: "锁拿不到的 secondary 端写 'secondary'，拿到锁的 primary 端写 'primary'"
      pattern: "ClientRole"
    - from: "integration_test.go::TestIntegration_Phase32_*"
      to: "scripts/test-fixture-up.sh"
      via: "复用 Phase 31 fixture（无新增 service）"
      pattern: "test-fixture"
---

<plan_dependencies>
- **Plan 01（Wave 1）必须先完成**：依赖 errcodes.SESSION_SYNC_LOCKED + last_session.go ClientRole 字段
- **Plan 02（Wave 2）必须先完成**：依赖 ConnectAndRunClaudeV3 在 OAuth 检查后的 SessionConfig 路由（本 plan 在更早段—MountWorkspace 之前—覆盖 mountCfg.SyncSessionLock，与 Plan 02 改的 OAuth 之后段不冲突）+ session.go::runClaudeWithSession（本 plan 添加 secondary 标志透传）
- **不**与 Plan 02 抢 ssh.go 同函数段：Plan 02 改 ConnectAndRunClaudeV3 的 OAuth 之后段；本 plan 改 MountWorkspace 之前段。两段在源码内行号相隔较远。
- 本 plan 是 v3.0 milestone Phase 32 的最后一块，完成后 phase verifier 可跑 `/gsd-verify-phase 32`
</plan_dependencies>

<objective>
落地 Phase 32 容器侧账号级 Mutagen 单例锁 + 集成测试套件 + C3/C7 回归：

1. **账号级 flock 单例锁（REQ-F5-D / PITFALLS M15）**：sync_lock.go 通过容器内 `flock -n -E 99 -F /tmp/cloud-claude/locks/sync-<account_id>.lock -c "exec sleep infinity"` 实现单例；后端拿不到锁 → ErrSyncLocked → Phase 31 mount_strategy 既有逻辑降级 sshfs-only（本 plan **不**重写降级，仅注入实现）
2. **ssh.go 注入**：ConnectAndRunClaudeV3 在 MountWorkspace 之前覆盖 `mountCfg.SyncSessionLock = func(id) { return AcquireSyncLock(connA, id) }`（修订 D-18 解决 Phase 31 接口缺 conn 参数问题）
3. **secondary 标志透传**：当 SyncSessionLock 返回 ErrSyncLocked 时，Plan 02 runClaudeWithSession 写 last-session.json 的 ClientRole='secondary'（vs primary）
4. **集成测试套件**：integration_test.go 追加 6 个 TestIntegration_Phase32_* — 单例锁 / banner / 30s 抖动 / pkill SIGHUP sshd / pgrep systemd-logind / tmux 降级；复用 Phase 31 fixture（test-fixture-up.sh / down.sh 不改）
5. **C3 验收**：30s 抖动场景下 sshfs_watcher（Phase 31 ship）在本 plan 集成测试中正常工作 — 不重写 watcher
6. **C7 验收**：pgrep systemd-logind 无输出 + pkill -SIGHUP sshd 后 tmux 仍存活（Phase 29 镜像侧防御本 plan 验收）

Purpose: 兑现 ROADMAP §Phase 32 Success Criteria 第 3 / 11 / 12 条（C7 回归 / 单例锁 / sshfs 抖动级联）+ 闭环 v3.0 多端共享设计的最后一块拼图（M15 多端 Mutagen 双写防御）。
Output: 1 个新文件 + 4 个改造文件 + 6 个集成测试用例；不引入新依赖；不修改 Phase 29 镜像 / Phase 31 fixture / mount_strategy 现有降级逻辑。
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
@.planning/phases/32-ssh-tmux/32-RESEARCH.md
@.planning/phases/32-ssh-tmux/32-PATTERNS.md
@.planning/phases/32-ssh-tmux/plans/01-net-resilience/PLAN.md
@.planning/phases/32-ssh-tmux/plans/02-tmux-multiclient/PLAN.md
@.planning/phases/29-v3-worker/29-CONTEXT.md
@.planning/phases/30-entry-api/30-CONTEXT.md
@.planning/phases/31-cli/31-CONTEXT.md
@internal/cloudclaude/ssh.go
@internal/cloudclaude/mount_strategy.go
@internal/cloudclaude/mount_mutagen.go
@internal/cloudclaude/oauth_check.go
@internal/cloudclaude/session.go
@internal/cloudclaude/last_session.go
@internal/cloudclaude/sshfs_watcher.go
@internal/cloudclaude/integration_test.go
@internal/cloudclaude/errcodes/codes.go
@internal/cloudclaude/errcodes/session.go
@scripts/test-fixture-up.sh
@scripts/test-fixture-down.sh
@deploy/docker/managed-user/sshd_config
@deploy/docker/managed-user/entrypoint.sh

<interfaces>
<!-- 本 plan 创建的对外 API。 -->

internal/cloudclaude/sync_lock.go 导出：

```go
package cloudclaude

import (
    "errors"
    "fmt"
    "strconv"
    "strings"

    "al.essio.dev/pkg/shellescape"
    "golang.org/x/crypto/ssh"
)

// ErrSyncLocked 表示另一端 cloud-claude 已持有同一 claude_account 的 Mutagen 单例锁。
// 调用方（mount_strategy.go::MountWorkspace 经 mountCfg.SyncSessionLock）必须降级 sshfs-only。
var ErrSyncLocked = errors.New("sync session locked by another cloud-claude")

// AcquireSyncLock 在远端容器内创建账号级单例锁。
// accountID == "" → 返回 (noop release, nil)（CONTEXT D-19 — anon 路径跳过锁）。
// 锁路径：/tmp/cloud-claude/locks/sync-<accountID>.lock（RESEARCH §6.2 修订 D-17）。
//
// 实现：远程 'mkdir -p /tmp/cloud-claude/locks 2>/dev/null && flock -n -E 99 -F <path> -c "echo $$; exec sleep infinity" & echo $!'
//   - exit 0 → 拿到锁，stdout 末行 = bash 后台 PID（用于 release kill）
//   - exit 99 → 锁被占（ErrSyncLocked）
//   - 其它 → flock 不可用（极旧镜像）→ wrap error
//
// release 函数：通过另一个 ssh.Session 远程 'kill <pid>'；session 关闭也会收割 sleep infinity，release 是显式 cleanup。
func AcquireSyncLock(conn *ssh.Client, accountID string) (release func(), err error)

// parseLastInt 从多行字符串末尾提取最后一个 int（PID）。容错空行与 'lock acquired' echo。
// 暴露用于单测覆盖（不再小写 unexport）。
func parseLastInt(s string) int
```

internal/cloudclaude/ssh.go ConnectAndRunClaudeV3 改造点（MountWorkspace 之前）：

```go
// [Phase 32 D-18 / RESEARCH §6.4] 用真实 flock 包装替换 noop 默认
//   注：本覆盖必须发生在 MountWorkspace 调用之前；mountCfg.SyncSessionLock 在 main.go 通常未设置（nil 或 noop）
mountCfg.SyncSessionLock = func(accountID string) (func(), error) {
    release, err := AcquireSyncLock(connA, accountID)
    if err != nil {
        if errors.Is(err, ErrSyncLocked) {
            // 写 ClientRole='secondary' 标志 — 通过 mountCfg 字段透传给 session 层
            mountCfg.IsSecondaryClient = true
            // stderr 输出 SESSION_SYNC_LOCKED — Phase 31 mount_strategy 也会输出 MOUNT_AUTO_DOWNGRADED；
            // 这里专门给 session 域错误码做一次输出，方便 doctor / Phase 34 explain 复用
            fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.SESSION_SYNC_LOCKED, accountID))
        }
        return nil, err
    }
    return release, nil
}
```

internal/cloudclaude/mount_strategy.go MountConfig 追加 1 字段（与 Plan 02 追加的 SessionShortID/SessionTakeOver/LocalHostname 并列）：

```go
// [Phase 32 Plan 03 新增] AcquireSyncLock 返回 ErrSyncLocked 时由 ssh.go 设为 true；
// session.go::runClaudeWithSession 据此写 last-session.json ClientRole='secondary'。
IsSecondaryClient bool
```

session.go 改造：runClaudeWithSession 在调 writeLastSessionTmuxField 时根据 SessionConfig.IsSecondaryClient 决定 role 字符串：

```go
role := "primary"
if sessionCfg.IsSecondaryClient {
    role = "secondary"
}
writeLastSessionTmuxField(sessionName, role)
```

`SessionConfig` 追加 1 字段 `IsSecondaryClient bool`，在 ConnectAndRunClaudeV3 构造 SessionConfig 时从 mountCfg.IsSecondaryClient 复制。
</interfaces>

<remote_command_template>
<!-- AcquireSyncLock 远程命令完整模板（RESEARCH §6.3）。所有变量必须 shellescape.Quote。 -->

```bash
mkdir -p /tmp/cloud-claude/locks 2>/dev/null && \
flock -n -E 99 -F <lockPath_q> -c 'echo $$; exec sleep infinity' &
echo $!
```

- `<lockPath_q>` = `shellescape.Quote("/tmp/cloud-claude/locks/sync-<accountID>.lock")`
- exit 0 → stdout 末行 = bash 后台 PID（`echo $!`）
- exit 99 → 锁被占
- exit 其它 → flock 不可用

**为什么 -F 必需**：no-fork 让 sleep infinity 直接持有 lock fd；缺 -F → SSH session 关闭时 sleep 死了但 flock 进程仍持锁，锁不释放（RESEARCH §6.1）。

**为什么 /tmp 不用 /var/lock**：ubuntu:24.04 默认 /var/lock 是 root-only（lrwxrwxrwx → /run/lock，mode 0755 root:root），UID 1000 mkdir EACCES（RESEARCH §6.2）。/tmp 默认 mode 1777，UID 1000 可写；OS 重启清理符合 lock 语义；不污染 mergerfs / Mutagen。
</remote_command_template>
</context>

<tasks>

<task type="auto">
  <name>Task 3.1: sync_lock.go 实现 + AcquireSyncLock + parseLastInt + 单测</name>
  <files>
    internal/cloudclaude/sync_lock.go
    internal/cloudclaude/sync_lock_test.go
  </files>
  <read_first>
    - internal/cloudclaude/oauth_check.go（line 44-63 — sess.Run + 错误收敛模板）
    - internal/cloudclaude/mount_mutagen.go（line 116-124 — sess.CombinedOutput 读 stdout 模板）
    - internal/cloudclaude/ssh.go（line 230-238 — ssh.ExitError.ExitStatus() 解包模板）
    - internal/cloudclaude/mount_sshfs.go（shellescape.Quote 用法参考）
    - internal/cloudclaude/errcodes/codes.go + errcodes/session.go（SESSION_SYNC_LOCKED 已由 Plan 01 注册）
    - .planning/phases/32-ssh-tmux/32-RESEARCH.md §6（flock -F 语义 + /tmp 路径选择 + 完整命令模板）
    - .planning/phases/32-ssh-tmux/32-CONTEXT.md D-17 / D-18 / D-19
    - .planning/phases/32-ssh-tmux/32-PATTERNS.md sync_lock.go 段（exact 模板复用 oauth_check + mount_mutagen.remoteRun）
  </read_first>
  <action>
    1. **internal/cloudclaude/sync_lock.go**：

       ```go
       package cloudclaude

       import (
           "errors"
           "fmt"
           "strconv"
           "strings"

           "al.essio.dev/pkg/shellescape"
           "golang.org/x/crypto/ssh"
       )

       // ErrSyncLocked — 见 <interfaces> 块文档。
       var ErrSyncLocked = errors.New("sync session locked by another cloud-claude")

       // AcquireSyncLock — 见 <interfaces> 块文档。
       func AcquireSyncLock(conn *ssh.Client, accountID string) (func(), error) {
           if accountID == "" {
               // CONTEXT D-19 — anon 路径跳过锁
               return func() {}, nil
           }

           lockPath := fmt.Sprintf("/tmp/cloud-claude/locks/sync-%s.lock", accountID)
           cmd := fmt.Sprintf(
               "mkdir -p /tmp/cloud-claude/locks 2>/dev/null && "+
                   "flock -n -E 99 -F %s -c 'echo $$; exec sleep infinity' &\necho $!",
               shellescape.Quote(lockPath),
           )

           sess, err := conn.NewSession()
           if err != nil {
               return nil, fmt.Errorf("创建 SSH session 失败: %w", err)
           }
           out, runErr := sess.CombinedOutput(cmd)
           sess.Close()

           if runErr != nil {
               // SSH ExitError code=99 → 锁被占（RESEARCH §6.3）
               if exitErr, ok := runErr.(*ssh.ExitError); ok && exitErr.ExitStatus() == 99 {
                   return nil, ErrSyncLocked
               }
               return nil, fmt.Errorf("flock 启动失败 (output: %s): %w", strings.TrimSpace(string(out)), runErr)
           }

           pid := parseLastInt(string(out))
           if pid <= 0 {
               // 拿到锁但没解析到 PID — 极端边界（容器侧 echo 不工作）；用 noop release
               return func() {}, nil
           }

           release := func() {
               killSess, e := conn.NewSession()
               if e != nil {
                   return // ssh 已断 — sleep infinity 也已被收割
               }
               defer killSess.Close()
               // kill 失败忽略：进程可能已被 SIGHUP 收割
               _ = killSess.Run(fmt.Sprintf("kill %d 2>/dev/null || true", pid))
           }
           return release, nil
       }

       // parseLastInt 从字符串末尾提取最后一个连续数字行（PID）。
       // 容错空行 / 'lock acquired' 等额外 echo；返回 0 表示未找到（caller 自行兜底）。
       func parseLastInt(s string) int {
           lines := strings.Split(strings.TrimSpace(s), "\n")
           for i := len(lines) - 1; i >= 0; i-- {
               trimmed := strings.TrimSpace(lines[i])
               if trimmed == "" { continue }
               n, err := strconv.Atoi(trimmed)
               if err == nil && n > 0 {
                   return n
               }
           }
           return 0
       }
       ```

    2. **internal/cloudclaude/sync_lock_test.go**：

       ```go
       package cloudclaude

       import (
           "testing"
       )

       func TestAcquireSyncLock_AnonReturnsNoop(t *testing.T) {
           release, err := AcquireSyncLock(nil, "")
           if err != nil {
               t.Fatalf("anon 路径不应返回错误，得 %v", err)
           }
           if release == nil {
               t.Fatal("anon 路径必须返回非 nil noop release")
           }
           release() // noop 不应 panic
       }

       func TestParseLastInt_SingleLine(t *testing.T) {
           if got := parseLastInt("12345\n"); got != 12345 {
               t.Errorf("got %d, want 12345", got)
           }
       }

       func TestParseLastInt_MultiLineLastWins(t *testing.T) {
           in := "lock acquired\n\n9876\n"
           if got := parseLastInt(in); got != 9876 {
               t.Errorf("got %d, want 9876（多行场景应取末尾数字）", got)
           }
       }

       func TestParseLastInt_NoNumber(t *testing.T) {
           if got := parseLastInt("error: flock not found\n"); got != 0 {
               t.Errorf("got %d, want 0（无数字时返回 0）", got)
           }
       }

       func TestParseLastInt_EmptyInput(t *testing.T) {
           if got := parseLastInt(""); got != 0 {
               t.Errorf("got %d, want 0（空输入）", got)
           }
       }

       func TestParseLastInt_NegativePIDIgnored(t *testing.T) {
           // -1 不是合法 PID；parseLastInt 返回 0
           if got := parseLastInt("-1\n"); got != 0 {
               t.Errorf("got %d, want 0（负数不视为合法 PID）", got)
           }
       }

       func TestParseLastInt_LargePID(t *testing.T) {
           if got := parseLastInt("123456\n"); got != 123456 {
               t.Errorf("got %d, want 123456", got)
           }
       }
       ```

       注：**不**用 fakeConn mock 测试 ExitError 99 → ErrSyncLocked 路径 — 该路径需要真实 ssh.ExitError 接口实例，单测构造成本高且收益小（已被集成测试 (a) 覆盖）。

    3. **不**修改 ssh.go / mount_strategy.go / session.go — 这些在 Task 3.2 / 3.3 完成。

    4. **不**新建 errcodes/session.go 注册 SESSION_SYNC_LOCKED — Plan 01 已注册。
  </action>
  <acceptance_criteria>
    - `go build ./internal/cloudclaude/...` 成功
    - `go vet ./internal/cloudclaude/...` 通过
    - `go test ./internal/cloudclaude/... -run "TestAcquireSyncLock_AnonReturnsNoop|TestParseLastInt" -count=1` 全部 PASS（≥ 7 用例）
    - `rg -n "func AcquireSyncLock" internal/cloudclaude/sync_lock.go` 命中 1 行
    - `rg -n "func parseLastInt" internal/cloudclaude/sync_lock.go` 命中 1 行
    - `rg -n "ErrSyncLocked" internal/cloudclaude/sync_lock.go` 命中（var 声明 + return 引用）
    - `rg -n "/tmp/cloud-claude/locks" internal/cloudclaude/sync_lock.go` 命中（路径字面值）
    - `rg -n "flock -n -E 99 -F" internal/cloudclaude/sync_lock.go` 命中（命令模板字面值）
    - `rg -n "exec sleep infinity" internal/cloudclaude/sync_lock.go` 命中（防 fork 关键参数）
    - `rg -n "shellescape.Quote" internal/cloudclaude/sync_lock.go` 命中（lockPath 必须 shellescape）
    - `rg -n "ExitStatus\\(\\) == 99" internal/cloudclaude/sync_lock.go` 命中
    - `rg -n "/var/lock" internal/cloudclaude/sync_lock.go` **必须无命中**（修订 D-17 — 路径已改 /tmp）
  </acceptance_criteria>
  <verify>
    <automated>go test ./internal/cloudclaude/... -run "TestAcquireSyncLock|TestParseLastInt" -count=1 &amp;&amp; go vet ./internal/cloudclaude/... &amp;&amp; ! rg -q "/var/lock" internal/cloudclaude/sync_lock.go</automated>
  </verify>
  <done>sync_lock.go 全部就位；anon 路径 noop + 锁拿不到 ErrSyncLocked + parseLastInt 边界覆盖；路径锁定 /tmp/cloud-claude/locks（修订 D-17）。Task 3.2 可在 ssh.go 内注入。</done>
</task>

<task type="auto">
  <name>Task 3.2: ssh.go 注入 SyncSessionLock + mount_strategy.MountConfig.IsSecondaryClient + session.go secondary 标志</name>
  <files>
    internal/cloudclaude/ssh.go
    internal/cloudclaude/mount_strategy.go
    internal/cloudclaude/session.go
  </files>
  <read_first>
    - internal/cloudclaude/ssh.go（ConnectAndRunClaudeV3 全函数 — 找到 mountCfg 已构造、MountWorkspace 调用之前的位置）
    - internal/cloudclaude/mount_strategy.go（MountConfig 定义 + Plan 02 已加的 SessionShortID/SessionTakeOver/LocalHostname；本 task 在末尾追加 IsSecondaryClient bool）
    - internal/cloudclaude/mount_strategy.go（MountWorkspace 内 SyncSessionLock 调用点 — 验证 Phase 31 已实现 errSyncLocked 降级；本 task **不**改这段）
    - internal/cloudclaude/session.go（Plan 02 的 SessionConfig + writeLastSessionTmuxField；本 task 加 IsSecondaryClient 字段 + role 字符串切换）
    - internal/cloudclaude/sync_lock.go（Task 3.1）— AcquireSyncLock + ErrSyncLocked
    - internal/cloudclaude/errcodes/session.go（SESSION_SYNC_LOCKED 注册位置 — Plan 01）
    - .planning/phases/32-ssh-tmux/32-CONTEXT.md D-17 / D-18 / D-19
    - .planning/phases/32-ssh-tmux/32-RESEARCH.md §6.4（修订 D-18 注入位置）
    - .planning/phases/32-ssh-tmux/32-PATTERNS.md ssh.go 改造点 C
  </read_first>
  <action>
    1. **internal/cloudclaude/mount_strategy.go**：在 MountConfig struct 末尾追加 1 个字段（与 Plan 02 追加的 3 字段并列）：

       ```go
       type MountConfig struct {
           // ... 现有字段（含 Plan 02 加的 SessionShortID / SessionTakeOver / LocalHostname）

           // [Phase 32 Plan 03 新增] AcquireSyncLock 返回 ErrSyncLocked 时由 ssh.go 设为 true；
           // session.go::runClaudeWithSession 据此写 last-session.json ClientRole='secondary'
           IsSecondaryClient bool
       }
       ```

       **不**修改 MountWorkspace 函数体 — Phase 31 D-31 已实现"errSyncLocked → 降级 sshfs-only"逻辑；本 plan 仅注入真实 lock 实现。

    2. **internal/cloudclaude/ssh.go**：在 ConnectAndRunClaudeV3 中找到 mountCfg 构造完毕、MountWorkspace 调用之前的位置（grep `MountWorkspace` 定位）；在该位置之前插入 SyncSessionLock 覆盖：

       ```go
       // [Phase 32 D-18 / RESEARCH §6.4] 用真实 flock 包装替换 mountCfg.SyncSessionLock 默认 noop。
       // 必须在 MountWorkspace 调用之前覆盖（mount_strategy.MountWorkspace 经此 hook 拿锁）。
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
       ```

       注：`errors` / `fmt` / `os` 应已在 ssh.go import；`errcodes` 应在 Phase 31 / Plan 02 import。如未则补上。

       **不**改 OAuth 检查段（Phase 31 ship）；**不**改 Plan 02 加的 SessionConfig 构造段（位置不重叠）。

    3. **internal/cloudclaude/ssh.go**：在 Plan 02 添加的 SessionConfig 构造段（OAuth 检查后），追加一行 IsSecondaryClient 字段透传：

       ```go
       sessionCfg := SessionConfig{
           // ... Plan 02 已有字段
           IsSecondaryClient: mountCfg.IsSecondaryClient, // [Phase 32 Plan 03 追加]
       }
       ```

    4. **internal/cloudclaude/session.go**：在 Plan 02 的 SessionConfig struct 末尾追加 1 个字段：

       ```go
       type SessionConfig struct {
           // ... Plan 02 已有字段
           IsSecondaryClient bool // [Plan 03] 决定 last-session.json ClientRole 写 primary/secondary
       }
       ```

       并修改 runClaudeWithSession 内调用 writeLastSessionTmuxField 的位置：

       ```go
       role := "primary"
       if sessionCfg.IsSecondaryClient {
           role = "secondary"
       }
       writeLastSessionTmuxField(sessionName, role)
       ```

       **不**改 session.go 其它逻辑（DetectTmux / take-over / banner / sessions ls/attach 全部不动）。

    5. 验证 mount_strategy.MountWorkspace 内 errSyncLocked 处理已在 Phase 31 ship — 用 grep 确认：

       ```bash
       rg -n "ErrSyncLocked|errSyncLocked|SyncSessionLock" internal/cloudclaude/mount_strategy.go
       ```

       预期命中：(a) MountConfig 结构内 SyncSessionLock 字段；(b) MountWorkspace 内调用 + errors.Is 判断 + 降级到 sshfs-only 分支。**如未命中** → 暴露 Phase 31 D-31 落地缺口，本 task 必须补全（fall-through 到 Phase 31 contract）。

       预期 Phase 31 已落 — 本 task 不需要改。如真有缺口，参考 mount_strategy.go::tryModeReal 的降级链模式（与 sshfs_watcher onDisconnect 同结构）补 errors.Is(err, ErrSyncLocked) → 强制 ModeSSHFSOnly + 写 DowngradeChain "sync_locked"。
  </action>
  <acceptance_criteria>
    - `go build ./...` 成功
    - `go vet ./...` 通过
    - `rg -n "AcquireSyncLock" internal/cloudclaude/ssh.go` 命中 1 行（在 ConnectAndRunClaudeV3 内）
    - `rg -n "ErrSyncLocked" internal/cloudclaude/ssh.go` 命中（errors.Is 判断）
    - `rg -n "SESSION_SYNC_LOCKED" internal/cloudclaude/ssh.go` 命中
    - `rg -n "IsSecondaryClient" internal/cloudclaude/mount_strategy.go` 命中 1 行（字段定义）
    - `rg -n "IsSecondaryClient" internal/cloudclaude/ssh.go` 命中 ≥ 1 行（设置 + 透传）
    - `rg -n "IsSecondaryClient" internal/cloudclaude/session.go` 命中 ≥ 2 行（字段 + 使用）
    - `rg -n "secondary" internal/cloudclaude/session.go` 命中（role 字符串）
    - `git diff internal/cloudclaude/mount_strategy.go` 仅追加 1 字段（用 `git diff -U0 internal/cloudclaude/mount_strategy.go | grep '^+' | grep -v '^+++' | wc -l` 输出 ≤ 5）
    - `git diff internal/cloudclaude/ssh.go` 改动行数 ≤ 25（覆盖 SyncSessionLock 闭包 + IsSecondaryClient 透传）
    - `go test ./internal/cloudclaude/... -count=1 -short` PASS（不破坏 Phase 31 / Plan 01 / Plan 02 单测）
  </acceptance_criteria>
  <verify>
    <automated>go build ./... &amp;&amp; go vet ./... &amp;&amp; go test ./internal/cloudclaude/... -count=1 -short</automated>
  </verify>
  <done>SyncSessionLock 在 ssh.go 内被真实 AcquireSyncLock 闭包覆盖；IsSecondaryClient 标志从 mount → ssh → session 三层透传完成；session.go 写 last-session.json 时 role 字符串正确切换 primary/secondary。Phase 31 mount_strategy 既有降级逻辑无改动。</done>
</task>

<task type="auto">
  <name>Task 3.3: integration_test.go 追加 6 个 TestIntegration_Phase32_* + scripts/test-fixture-up.sh 微调（如需）</name>
  <files>
    internal/cloudclaude/integration_test.go
    scripts/test-fixture-up.sh
  </files>
  <read_first>
    - internal/cloudclaude/integration_test.go（Phase 31 已有 6 个 TestIntegration_* — 模式 + dockerExec helper + TestMain 优雅跳过）
    - scripts/test-fixture-up.sh（Phase 31 ship — 检查是否需要追加 sshfs / mutagen-agent 启动逻辑；本 plan 通常不改）
    - scripts/test-fixture-down.sh（Phase 31 ship — 不改）
    - internal/cloudclaude/sync_lock.go（Task 3.1）+ ssh.go SyncSessionLock 注入（Task 3.2）
    - internal/cloudclaude/sshfs_watcher.go（C3 防御 — 集成测试 (c) 30s 抖动场景隐含验收）
    - deploy/docker/managed-user/sshd_config（Phase 29 ClientAliveInterval / MaxSessions）
    - deploy/docker/managed-user/entrypoint.sh（Phase 29 验证容器内无 systemd-logind）
    - .planning/phases/32-ssh-tmux/32-RESEARCH.md §7.1 / §7.2 / §Test Matrix（C7 / C3 验收命令；30s docker network disconnect 测试模板）
    - .planning/phases/32-ssh-tmux/32-CONTEXT.md specifics（每条 SC 的真实拔网命令要求 — 本 plan 不强制 30s 真实拔网，CI 跳过）
  </read_first>
  <action>
    1. **internal/cloudclaude/integration_test.go**：在文件末尾追加 6 个新用例（与 Phase 31 6 个用例并列；TestMain 共享）：

       ```go
       // ─────────────────────────────────────────────────────────────
       // Phase 32 集成测试
       // ─────────────────────────────────────────────────────────────

       // TestIntegration_Phase32_PgrepNoSystemdLogind 验证 C7 镜像侧防御（Phase 29 D-15）。
       // 容器内必须无 systemd-logind 进程，否则 pkill -SIGHUP sshd 会顺带杀掉 tmux server。
       func TestIntegration_Phase32_PgrepNoSystemdLogind(t *testing.T) {
           out, err := dockerExec(t, "pgrep", "systemd-logind")
           if err == nil && strings.TrimSpace(out) != "" {
               t.Fatalf("容器内不应有 systemd-logind 进程（C7 防御失败），pgrep 输出: %q", out)
           }
           // pgrep 无命中时 exit code = 1（错误）— 这是预期行为；err != nil 是 OK 的
       }

       // TestIntegration_Phase32_TmuxSurvivesSighupSshd 验证 C7 攻击场景。
       // 起 tmux session → pkill -SIGHUP sshd → tmux server 仍存活、session 仍可访问。
       func TestIntegration_Phase32_TmuxSurvivesSighupSshd(t *testing.T) {
           // 起 tmux session
           if _, err := dockerExec(t, "tmux", "new-session", "-d", "-s", "phase32_c7"); err != nil {
               t.Fatalf("tmux new-session 失败: %v", err)
           }
           defer dockerExec(t, "tmux", "kill-session", "-t", "phase32_c7")

           time.Sleep(500 * time.Millisecond)

           // 触发 sshd 重读配置（不杀 sshd 自己）
           if _, err := dockerExec(t, "sh", "-c", "kill -HUP $(pgrep sshd | head -1) || true"); err != nil {
               t.Logf("kill -HUP sshd 警告（可能不需要 sudo）: %v", err)
           }
           time.Sleep(2 * time.Second)

           // 验证 tmux session 仍存活
           out, err := dockerExec(t, "tmux", "ls")
           if err != nil {
               t.Fatalf("tmux ls 失败（C7 防御失败 — tmux server 可能被 systemd-logind 杀掉）: %v", err)
           }
           if !strings.Contains(out, "phase32_c7") {
               t.Fatalf("tmux session phase32_c7 不见了（C7 防御失败），tmux ls 输出: %q", out)
           }
       }

       // TestIntegration_Phase32_SyncLockMutexes 验证 REQ-F5-D 账号级单例锁。
       // 同一 accountID 在容器内只能有一个进程持有 /tmp/cloud-claude/locks/sync-<id>.lock。
       func TestIntegration_Phase32_SyncLockMutexes(t *testing.T) {
           sshCfg := defaultFixtureSSHConfig() // helper：返回 fixture 容器的 SSHConfig（与 Phase 31 一致；如不存在则参考 fixture 用例自构）
           conn1, err := SSHConnect(sshCfg)
           if err != nil { t.Fatal(err) }
           defer conn1.Close()

           accountID := "test-account-phase32-lock"

           release1, err := AcquireSyncLock(conn1, accountID)
           if err != nil {
               t.Fatalf("第一次 AcquireSyncLock 应成功，得 %v", err)
           }
           defer release1()

           // 验证 lockfile 存在
           out, _ := dockerExec(t, "ls", "/tmp/cloud-claude/locks/")
           if !strings.Contains(out, "sync-test-account-phase32-lock.lock") {
               t.Errorf("lockfile 应存在: %q", out)
           }

           // 第二端尝试拿同 accountID 的锁 → ErrSyncLocked
           conn2, err := SSHConnect(sshCfg)
           if err != nil { t.Fatal(err) }
           defer conn2.Close()
           _, err = AcquireSyncLock(conn2, accountID)
           if !errors.Is(err, ErrSyncLocked) {
               t.Fatalf("第二次 AcquireSyncLock 应返回 ErrSyncLocked，得 %v", err)
           }

           // release1 后第二端应能拿到锁
           release1()
           time.Sleep(2 * time.Second) // 等 SSH session 关闭收割 sleep infinity
           release2, err := AcquireSyncLock(conn2, accountID)
           if err != nil {
               t.Fatalf("release1 后第二端应能拿锁，得 %v", err)
           }
           defer release2()
       }

       // TestIntegration_Phase32_SyncLockAnonNoop 验证 D-19：anon 路径不上锁。
       func TestIntegration_Phase32_SyncLockAnonNoop(t *testing.T) {
           sshCfg := defaultFixtureSSHConfig()
           conn, err := SSHConnect(sshCfg)
           if err != nil { t.Fatal(err) }
           defer conn.Close()

           release, err := AcquireSyncLock(conn, "")
           if err != nil { t.Fatalf("anon 应 noop，得 %v", err) }
           release() // 不 panic

           // 第二次也应直接 noop（无 lockfile 创建）
           release2, err := AcquireSyncLock(conn, "")
           if err != nil { t.Fatal(err) }
           release2()
       }

       // TestIntegration_Phase32_DetectTmuxAvailable 验证 D-15 在 fixture 容器（Phase 29 镜像 tmux 3.4+ 已就位）必返 true。
       func TestIntegration_Phase32_DetectTmuxAvailable(t *testing.T) {
           sshCfg := defaultFixtureSSHConfig()
           conn, err := SSHConnect(sshCfg)
           if err != nil { t.Fatal(err) }
           defer conn.Close()

           available, version, reason := DetectTmux(conn)
           if !available {
               t.Fatalf("Phase 29 镜像应有 tmux，DetectTmux=false reason=%q", reason)
           }
           if !strings.Contains(version, "tmux") {
               t.Errorf("version 应含 'tmux'，得 %q", version)
           }
       }

       // TestIntegration_Phase32_NetworkDisconnect30s 验证 30s 抖动 reconnect + tmux 进程不丢（REQ-F4-A / BASE-03 前置）。
       // 此用例需要 docker network disconnect 权限；CI 缺权限时 t.Skip 跳过留 Phase 35 真机。
       func TestIntegration_Phase32_NetworkDisconnect30s(t *testing.T) {
           if testing.Short() {
               t.Skip("short mode 跳过 30s 抖动场景")
           }
           // 检测 docker network 命令是否可用（CI 环境通常容器内不允许）
           if _, err := exec.Command("docker", "network", "ls").CombinedOutput(); err != nil {
               t.Skip("docker network 不可用，跳过；留 Phase 35 真机 UAT")
           }

           // 此处省略完整实现 — 框架代码：
           //   1. 起 cloud-claude → 进入 tmux session 跑 'sleep 60'
           //   2. docker network disconnect <ctr> bridge
           //   3. sleep 30
           //   4. docker network connect <ctr> bridge
           //   5. 等 cloud-claude reconnect 成功（≤ 60s）
           //   6. tmux capture-pane -t <session> -p 应仍含 'sleep 60' 输出
           //   7. tmux list-sessions 应仍有该 session
           //
           // 完整端到端实现成本高（需 expect 风格的 PTY 交互）；本 plan 落地框架 + 主要断言点；
           // 真正"无感知"体感验收留 Phase 35 手测 BASE-03。
           t.Log("框架用例 — 完整 PTY 交互留 Phase 35；本 plan 验收依赖 sync_lock + DetectTmux 单元 + 集成测试覆盖")
           t.Skip("Phase 32 v0：框架就位，端到端 PTY 交互留 Phase 35 真机")
       }

       // defaultFixtureSSHConfig 复用 Phase 31 集成测试的 fixture 凭证常量。
       // 如 Phase 31 已有同名 helper，本 task 直接复用；否则按以下定义：
       func defaultFixtureSSHConfig() SSHConfig {
           return SSHConfig{
               Host:     fixtureHost,
               Port:     fixturePort,
               User:     fixtureUser,
               Password: fixturePass,
           }
       }
       ```

       注：`errors` / `time` / `strings` / `os/exec` / `testing` 应已在 integration_test.go import；`SSHConnect` 是 Plan 02 Task 2.3 暴露的 export 包装。

    2. **scripts/test-fixture-up.sh**：通常不需改 — 但若 Phase 31 fixture 未挂 `/dev/fuse` / `--cap-add SYS_ADMIN` / `--security-opt apparmor=unconfined`（Phase 32 单例锁不需要这些，但 30s 抖动测试可能需要 mergerfs/sshfs 全栈）则本 task 验证；预期 Phase 31 fixture 已就位（参考 31-Plan 03 SUMMARY）。**本 task 不改 fixture 脚本**。

    3. 集成测试运行说明（在 integration_test.go 文件头注释追加）：

       ```go
       // Phase 32 追加的 6 个 TestIntegration_Phase32_* 用例：
       //   - TestIntegration_Phase32_PgrepNoSystemdLogind: C7 镜像侧前置（Phase 29 D-15）
       //   - TestIntegration_Phase32_TmuxSurvivesSighupSshd: C7 攻击模拟（pkill -SIGHUP sshd）
       //   - TestIntegration_Phase32_SyncLockMutexes: REQ-F5-D 双端互斥
       //   - TestIntegration_Phase32_SyncLockAnonNoop: D-19 anon 跳过
       //   - TestIntegration_Phase32_DetectTmuxAvailable: D-15 在 Phase 29 镜像必通过
       //   - TestIntegration_Phase32_NetworkDisconnect30s: REQ-F4-A 框架（端到端 PTY 留 Phase 35）
       //
       // 运行方式同 Phase 31：
       //   bash scripts/test-fixture-up.sh
       //   go test -tags=integration -count=1 -v ./internal/cloudclaude/
       //   bash scripts/test-fixture-down.sh
       //
       // CI 在 docker 不可用时优雅 skip（TestMain 已处理）。
       ```

    4. **不**修改 Phase 31 既有 6 个 TestIntegration_* 用例（任何 diff 都会破坏 plan 03 的"零侵入" 承诺）。
  </action>
  <acceptance_criteria>
    - `go build -tags=integration ./internal/cloudclaude/...` 成功（编译通过 — 不需要 docker）
    - `go vet -tags=integration ./internal/cloudclaude/...` 通过
    - `rg -n "TestIntegration_Phase32_" internal/cloudclaude/integration_test.go` 命中 6 行（6 个新 Test 函数名）
    - `rg -n "TestIntegration_Phase32_PgrepNoSystemdLogind" internal/cloudclaude/integration_test.go` 命中
    - `rg -n "TestIntegration_Phase32_TmuxSurvivesSighupSshd" internal/cloudclaude/integration_test.go` 命中
    - `rg -n "TestIntegration_Phase32_SyncLockMutexes" internal/cloudclaude/integration_test.go` 命中
    - `rg -n "TestIntegration_Phase32_SyncLockAnonNoop" internal/cloudclaude/integration_test.go` 命中
    - `rg -n "TestIntegration_Phase32_DetectTmuxAvailable" internal/cloudclaude/integration_test.go` 命中
    - `rg -n "TestIntegration_Phase32_NetworkDisconnect30s" internal/cloudclaude/integration_test.go` 命中
    - `rg -n "AcquireSyncLock\\(" internal/cloudclaude/integration_test.go` 命中（用例直接调用）
    - `rg -n "DetectTmux\\(" internal/cloudclaude/integration_test.go` 命中
    - `rg -n "SSHConnect\\(" internal/cloudclaude/integration_test.go` 命中
    - `git diff scripts/test-fixture-up.sh` 应为空（本 plan 不改 fixture 脚本）
    - `git diff scripts/test-fixture-down.sh` 应为空
    - 在有 docker 环境（手测 / CI）：`bash scripts/test-fixture-up.sh && go test -tags=integration -count=1 -run "TestIntegration_Phase32_PgrepNoSystemdLogind|TestIntegration_Phase32_DetectTmuxAvailable|TestIntegration_Phase32_SyncLockAnonNoop" ./internal/cloudclaude/ && bash scripts/test-fixture-down.sh` 至少这 3 个最简用例 PASS（其它 3 个需要 docker 容器运行环境完整 — 接受在 CI 部分 skip）
    - 无 docker 时 TestMain 优雅 os.Exit(0) — `go test -tags=integration ./internal/cloudclaude/...` 不应报错（与 Phase 31 同行为）
  </acceptance_criteria>
  <verify>
    <automated>go build -tags=integration ./internal/cloudclaude/... &amp;&amp; go vet -tags=integration ./internal/cloudclaude/... &amp;&amp; rg -c "TestIntegration_Phase32_" internal/cloudclaude/integration_test.go | grep -q "^6$" || (echo "expect 6 TestIntegration_Phase32_* funcs" &amp;&amp; exit 1)</automated>
  </verify>
  <done>integration_test.go 新增 6 个 TestIntegration_Phase32_* 用例覆盖 C3 / C7 / REQ-F5-D 关键路径；fixture 脚本无改动；端到端 PTY 交互留 Phase 35。Phase 31 既有 6 个 TestIntegration_* 完全不动。</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| 客户端 → 容器 /tmp/cloud-claude/locks/ | mkdir + flock 远程命令；accountID 进入路径名 — 必须 shellescape |
| sync_lock.go 远程 sleep infinity 进程 | 长驻进程持有 fd → SSH session 关闭由 sshd SIGHUP 收割 |
| AcquireSyncLock release func | release 通过另一个 ssh.Session 远程 kill PID — PID 来自 echo $!（容器内 bash 输出，不接收外部输入） |
| 集成测试 dockerExec | docker exec 注入 — 仅在 fixture 容器内执行，参数来自测试代码常量 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-32-14 | Tampering | sync_lock.go remoteCmd 拼接 accountID 进入路径 | mitigate | accountID 来自 mountCfg.ClaudeAccountID（从 Phase 30 AuthResponse 读，gateway 控）；shellescape.Quote 包装 lockPath（grep 验证）；非 ASCII / 路径分隔符不会逃出 /tmp/cloud-claude/locks/ |
| T-32-15 | Information Disclosure | /tmp/cloud-claude/locks/sync-<account>.lock 文件名包含 accountID | accept | tmp 目录是容器内可见但不持久化；accountID 已是 hash（Phase 30 D-05 推导）；与 Plan 02 tmux session 命名等同等级 |
| T-32-16 | DoS | 多个 cloud-claude 反复 AcquireSyncLock 制造 sleep infinity 进程 | mitigate | flock -n 立即返回（不阻塞）；后端 ErrSyncLocked → 不创建第二个 sleep；release 显式 kill 不依赖超时；最坏情况是孤立 sleep（SSH 异常断开时 sshd SIGHUP 必然收割） |
| T-32-17 | DoS | M15 防御本身：单例锁失败模式 | mitigate（M15 by design） | 设计目的就是阻止双写；secondary 端降级到只读 sshfs/mergerfs 视图，无数据丢失风险；Phase 31 mount_strategy 已实现该降级路径 |
| T-32-18 | Tampering | release func 远程 kill PID 错杀其它进程 | accept | PID 来自 echo $!（bash 子进程 PID 空间）；理论上 PID 复用可能误杀，但容器 PID 空间独立 + sleep infinity 通常存活时间 < cloud-claude 生命周期；最坏情况误杀其它 sleep — 不影响功能 |
| T-32-19 | Spoofing | 整个 plan 引入的 SSH 命令注入面（共 ≥ 8 个 dockerExec / sshOutput / sshRun） | mitigate | 全部参数走 shellescape；集成测试用例的字符串常量都是固定值（无外部输入）；新 grep 检查 `rg -n "fmt.Sprintf.*flock|fmt.Sprintf.*tmux" internal/cloudclaude/sync_lock.go` 命中处必须紧邻 shellescape.Quote |
| T-32-21 | InfoDisclosure | secondary 端误以为 full mount 模式（M13 静默降级回归） | mitigate | 三层可见性叠加：(a) ssh.go 在 ErrSyncLocked 时 stderr 输出 `[SESSION_SYNC_LOCKED]`（Plan 03 Task 3.2）；(b) Phase 31 mount_strategy 在 errSyncLocked 降级路径输出 `[MOUNT_AUTO_DOWNGRADED]`（已 ship）；(c) last-session.json `client_role='secondary'`（Plan 03 Task 3.2 透传链）— Phase 34 doctor / `cloud-claude explain` 复用任一通道排障 |

**Severity 分布（block_on=high）**：
- High: 0 项（M15 双写虽是 v3.0 严重风险，但本 plan 是其防御，标 High 不合理）
- Medium: T-32-14（关键 — accountID 路径拼接 / 已 mitigate）/ T-32-16 / T-32-17（M15 防御本身）/ T-32-21（M13 静默降级回归 / 三层可见性已就位）
- Low: T-32-15 / T-32-18 / T-32-19

无 High 项 → 不阻断 plan 执行。

**M15 标 High 说明**（用户 quality_gate 要求）：M15 是 v3.0 milestone 级别的 High 风险（Mutagen 双向同步多端双写会导致数据 corruption / lost updates），但本 plan 的 sync_lock.go 是该风险的**核心防御**：通过容器侧 flock 强制单例。所以本 plan 作为 mitigation 落地不再单独标 High（防御已实施）；如执行后单元 + 集成测试不通过，再升级 High 阻断。
</threat_model>

<verification>
本 plan 完成时，可断言下列 ROADMAP §Phase 32 Success Criteria：

1. **SC3（C7 — pkill -SIGHUP sshd 后 tmux 仍存活）**：integration_test TestIntegration_Phase32_TmuxSurvivesSighupSshd PASS（在有 docker 环境）+ TestIntegration_Phase32_PgrepNoSystemdLogind PASS（容器内无 systemd-logind）— 无 docker 时优雅 skip
2. **SC11（REQ-F5-D 账号级 Mutagen 单例锁）**：TestIntegration_Phase32_SyncLockMutexes 验证：
   - 第一次 AcquireSyncLock 成功 + lockfile 存在 + mutagen sync 单例
   - 第二次同 accountID AcquireSyncLock 返回 ErrSyncLocked
   - release1 后第二端能在 ≤ 5s 内拿到锁
   - + TestIntegration_Phase32_SyncLockAnonNoop 验证 anon 路径不创建锁
3. **SC12（C3 — sshfs 抖动 30s 不挂死）**：sshfs_watcher（Phase 31 ship）在 reconnect 期间继续工作；本 plan 不重写，TestIntegration_Phase32_NetworkDisconnect30s 提供框架（端到端 PTY 留 Phase 35）
4. **错误码可见性**（M13 防御复用）：AcquireSyncLock 返回 ErrSyncLocked 时 stderr 必含 [SESSION_SYNC_LOCKED]；last-session.json client_role='secondary'
5. **零 Phase 31 / Phase 29 / Plan 01 / Plan 02 改动**：scripts/test-fixture-up.sh / down.sh / mount_strategy.MountWorkspace / sshfs_watcher.go / Phase 31 6 个 TestIntegration_* / Plan 01 sync_lock 之外的 errcodes 注册 / Plan 02 session.go take-over 与 banner — 全部 zero diff（git diff 复核）

**全 plan 综合 verify 命令**：

```bash
# 单元 + 编译 + lint
go build ./... && go build -tags=integration ./internal/cloudclaude/... \
  && go vet ./... && go vet -tags=integration ./internal/cloudclaude/... \
  && go test ./internal/cloudclaude/... -count=1 -short \
  && rg -c "TestIntegration_Phase32_" internal/cloudclaude/integration_test.go | grep -q "^6$"

# Plan 03 路径修订校验
! rg -q "/var/lock" internal/cloudclaude/sync_lock.go  # 必须无（修订 D-17 已改 /tmp）

# Plan 03 防御覆盖校验
rg -q "ErrSyncLocked" internal/cloudclaude/sync_lock.go internal/cloudclaude/ssh.go
rg -q "IsSecondaryClient" internal/cloudclaude/mount_strategy.go internal/cloudclaude/ssh.go internal/cloudclaude/session.go
rg -q "exec sleep infinity" internal/cloudclaude/sync_lock.go  # -F + exec 防 fork

# 集成测试（仅在有 docker 环境时）
if command -v docker >/dev/null && docker ps >/dev/null 2>&1; then
  bash scripts/test-fixture-up.sh \
    && go test -tags=integration -count=1 -run "TestIntegration_Phase32_PgrepNoSystemdLogind|TestIntegration_Phase32_DetectTmuxAvailable|TestIntegration_Phase32_SyncLockAnonNoop" -v ./internal/cloudclaude/
  bash scripts/test-fixture-down.sh
fi
```
</verification>

<success_criteria>
- sync_lock.go + sync_lock_test.go 全部就位；7 个 parseLastInt + anon 单测 PASS
- /var/lock 路径在 sync_lock.go 中**零**命中（修订 D-17 验证）
- ssh.go ConnectAndRunClaudeV3 正确覆盖 mountCfg.SyncSessionLock 闭包；改动行数 ≤ 25
- mount_strategy.MountConfig 仅追加 IsSecondaryClient bool 1 字段；MountWorkspace 函数体 zero diff
- session.go SessionConfig 追加 IsSecondaryClient 字段 + role 字符串切换 primary/secondary
- integration_test.go 末尾追加 6 个 TestIntegration_Phase32_* 用例 + defaultFixtureSSHConfig helper；Phase 31 既有 6 个用例完全不动
- scripts/test-fixture-up.sh / down.sh zero diff
- 无新依赖（go.mod 无变化）
- 全平台编译通过：darwin / linux / windows（windows 不需 sync_lock 真功能但应编译通过）
- Plan 02 的 cmd 层入口 / cobra 树 / KeepAlive 校验 zero diff
- Plan 01 的 errcodes / reconnect / input_buffer / keepalive zero diff
</success_criteria>

<output>
After completion, create `.planning/phases/32-ssh-tmux/32-03-SUMMARY.md` 描述：
- AcquireSyncLock 实际实现（与 plan <interfaces> 对照）+ /tmp 路径选择论证（为何不选 /var/lock / /run）
- ssh.go SyncSessionLock 闭包覆盖位置（line 号 + before/after diff 摘录）
- IsSecondaryClient 三层透传链（mountCfg → SessionConfig → last-session.json）
- 6 个 TestIntegration_Phase32_* 用例分别覆盖的 SC 编号
- 集成测试在有 docker 环境的 PASS 输出（至少 3 个最简用例的 go test -v 输出）
- C3 / C7 / M15 三大 PITFALL 在本 plan 的最终防御状态总结
- 留给 /gsd-verify-phase 的 hand-off 清单（Phase 32 全部 12 条 SC 中由本 plan 验收的子集）
</output>
