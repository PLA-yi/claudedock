---
phase: 31-cli
plan: 02-mount-three-layer
type: execute
wave: 2
depends_on:
  - 01-errcodes-mutagen-embed
files_modified:
  - internal/cloudclaude/mount.go
  - internal/cloudclaude/mount_sshfs.go
  - internal/cloudclaude/mount_mutagen.go
  - internal/cloudclaude/mount_merge.go
  - internal/cloudclaude/mount_strategy.go
  - internal/cloudclaude/mount_strategy_test.go
  - internal/cloudclaude/mount_mutagen_test.go
  - internal/cloudclaude/askpass.go
  - internal/cloudclaude/askpass_test.go
  - internal/cloudclaude/sshfs_watcher.go
  - internal/cloudclaude/sshfs_watcher_test.go
  - internal/cloudclaude/last_session.go
  - internal/cloudclaude/last_session_test.go
  - internal/cloudclaude/colors.go
  - internal/cloudclaude/exitcodes.go
  - internal/cloudclaude/exitcodes_test.go
  - internal/cloudclaude/ssh.go
  - cmd/cloud-claude/main.go
autonomous: true
requirements:
  - REQ-F1-A
  - REQ-F1-B
  - REQ-F1-C
  - REQ-F1-D
  - REQ-F2-A
  - REQ-F2-B
  - REQ-F2-C
must_haves:
  truths:
    - "mount.go 拆为 4 个文件：mount_sshfs.go / mount_mutagen.go / mount_merge.go / mount_strategy.go；原 mount.go 仅保留共享 helper（waitForMount / shellQuote / sshRun / fusermountCleanup / channelRWC），公开 API 不变"
    - "MountWorkspace(connA, connB *ssh.Client, cfg MountConfig) (cleanup func(), mode Mode, err error) 是顶层入口；按 Mode={Auto,Full,MutagenOnly,SSHFSOnly} 调度，--mount-mode=auto 任一层失败 ≤2s 内自动降级到下一档"
    - "降级 banner 必现：每次降级 stderr 输出一行 errcodes.Format(MOUNT_AUTO_DOWNGRADED, from, to, code, msg)；M13 防御「禁止静默降级」"
    - "Mutagen sync 创建前必跑安全门：alpha 本地 ReadDir empty AND 远端 conn-A find /workspace-hot 非空 → 退出非 0 + MOUNT_MUTAGEN_SAFETY_GUARD；mutagen sync session 不被创建"
    - "Mutagen sync 创建前必跑 du -sb 检查：cwd > 50MB → MOUNT_MUTAGEN_WHITELIST_REJECT + 中文 ignore 建议 + 自动降级 sshfs（auto 模式）/ 退出非 0（force 模式）"
    - "macOS APFS case-insensitive 检测命中时强制 --mode=two-way-resolved（默认就是该模式，但要写入 last-session.json apfs_case_insensitive=true）"
    - "sshfs 挂载追加 reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10 四个参数"
    - "AskpassHelper 创建临时 helper 0700 + 在父进程退出时 defer 清理；密码不进 ps args"
    - "sshfs_watcher goroutine 每 5s 在 conn-A 上 timeout 2 mountpoint -q /workspace-cold；连续 3 次失败（≥15s）远程 setfattr -n user.mergerfs.branches -v '-/workspace-cold' /workspace + MOUNT_SSHFS_DISCONNECTED 警告"
    - "ConnectAndRunClaudeV3(cfg, args, cwd, proxyCmds, mountCfg, authResp) 是新公开入口；ConnectAndRunClaude 保留为兼容入口（默认 sshfs-only）"
    - "cmd/cloud-claude/main.go 注册 cobra flag --mount-mode={auto,full,mutagen-only,sshfs-only}，默认 auto；非法值 cobra 自动报错"
    - "三段式中文进度按最终决策的 mode 渲染：(1/3) 热同步源码中… / (2/3) 启动冷兜底… / (3/3) 合并视图…；mutagen-only / sshfs-only 模式下对应段改为 跳过 提示"
    - "banner 「✓ 文件映射就绪 [<mode>]」按 mode 着色：full=绿、mutagen-only/sshfs-only=黄；NO_COLOR=1 或非 TTY 关色"
    - "降级状态机 + conflict count 写入 ~/.cloud-claude/last-session.json，schema_version=1，含 downgrade_chain / actual_mode / apfs_case_insensitive 字段"
    - "internal/cloudclaude/exitcodes.go 暴露 9 个命名退出码常量（ExitOK=0..ExitMountForceFailed=8），全部 ≤ 125（POSIX 限制）；Plan 03 OAuth 与 mount_strategy 引用常量而非裸数字；与 v2.0 cmd/cloud-claude/main.go 现有 exitOK/exitAuthFailed/.../exitInternalError 0-5 数值完全对齐，新增 OAuth 与 mount 退出码占用 6-8，无冲突"
  artifacts:
    - path: "internal/cloudclaude/mount_strategy.go"
      provides: "MountWorkspace 入口 + Mode 枚举 + 状态机降级 + 三段式进度 + banner + last-session 写入"
      contains: "func MountWorkspace"
    - path: "internal/cloudclaude/mount_mutagen.go"
      provides: "Mutagen daemon start (幂等) + 版本握手 + 50MB / safety guard / sync create 经 conn-C + askpass"
      contains: "MutagenHealthCheck"
    - path: "internal/cloudclaude/mount_sshfs.go"
      provides: "v2.0 mountWorkspace 重命名 mountSSHFS + 追加 reconnect / ServerAlive 参数"
      contains: "reconnect,ServerAliveInterval=15"
    - path: "internal/cloudclaude/mount_merge.go"
      provides: "远程 sudo mergerfs 挂载（参数与 Phase 29 D-11 一致）+ runtime branch 摘除（setfattr）"
      contains: "setfattr -n user.mergerfs.branches"
    - path: "internal/cloudclaude/askpass.go"
      provides: "ssh-askpass helper 临时脚本生成 + 环境变量包装 + defer 清理；密码不进 ps args"
      contains: "SSH_ASKPASS"
    - path: "internal/cloudclaude/sshfs_watcher.go"
      provides: "5s 周期 mountpoint 检查 + 3 次失败摘除 cold branch + MOUNT_SSHFS_DISCONNECTED 警告"
      contains: "timeout 2 mountpoint"
    - path: "internal/cloudclaude/last_session.go"
      provides: "WriteLastSession(path, snapshot) — RESEARCH §8 schema_version=1"
      contains: "schema_version"
    - path: "internal/cloudclaude/colors.go"
      provides: "极简 ANSI 着色 helper（30/32/33/35/0），NO_COLOR / 非 TTY 自动关色"
      contains: "NO_COLOR"
    - path: "cmd/cloud-claude/main.go"
      provides: "--mount-mode flag + 切到 ConnectAndRunClaudeV3 入口"
      contains: "--mount-mode"
    - path: "internal/cloudclaude/exitcodes.go"
      provides: "9 个命名退出码常量（ExitOK..ExitMountForceFailed），与 v2.0 main.go 0-5 对齐 + 新增 OAuth/mount 6-8"
      contains: "ExitOAuthExpired"
  key_links:
    - from: "internal/cloudclaude/mount_strategy.go MountWorkspace"
      to: "mount_mutagen.go mountMutagen / mount_sshfs.go mountSSHFS / mount_merge.go mountMerge"
      via: "errgroup 并发拉起 + 顺序降级 fallback"
      pattern: "mountMutagen\\(|mountSSHFS\\(|mountMerge\\("
    - from: "mount_mutagen.go mountMutagen"
      to: "errcodes.MOUNT_MUTAGEN_*"
      via: "errcodes.Format(...) 包装的中文 stderr 输出"
      pattern: "errcodes\\.MOUNT_MUTAGEN"
    - from: "sshfs_watcher.go"
      to: "mount_merge.go RemoveBranch"
      via: "watcher 检测到 ≥15s 抖动后调 RemoveBranch(connA, /workspace-cold)"
      pattern: "RemoveBranch"
    - from: "cmd/cloud-claude/main.go runRoot"
      to: "cloudclaude.ConnectAndRunClaudeV3"
      via: "新入口替代 v2.0 ConnectAndRunClaude"
      pattern: "ConnectAndRunClaudeV3"
    - from: "mount_strategy.go"
      to: "~/.cloud-claude/last-session.json"
      via: "WriteLastSession(snapshot)"
      pattern: "WriteLastSession"
---

<plan_dependencies>
- **Plan 01（Wave 1）必须先完成**：本 plan 重度依赖
  - `internal/cloudclaude/errcodes`（Format / Lookup / 15 个 Code 常量）
  - `cloudclaude.ExtractMutagenBinary`（mountMutagen 启动时调用）
  - `cloudclaude.IsCaseInsensitiveFS`（mount_strategy 启动早期检测 APFS）
  - `internal/cloudclaude/mutagen_bin/<plat>/mutagen`（embed 二进制必须存在才能编译）
- 同 Wave 内禁止与本 plan 并发改动 `internal/cloudclaude/mount.go` / `ssh.go` / `cmd/cloud-claude/main.go`
</plan_dependencies>

<objective>
把 v2.0 单层 sshfs 升级为「Mutagen 热同步 + sshfs 冷兜底 + mergerfs 联合视图」三层架构，实现 `--mount-mode={auto,full,mutagen-only,sshfs-only}` 显式降级状态机；落地 CONTEXT D-13 安全门、D-15 状态机、D-16 last-session.json、D-17 banner、D-18 三段式进度、D-25/D-26 三连接模型（含 RESEARCH §1.2 修订的 conn-C + askpass）、D-27 抖动 watcher + RESEARCH §2.2 修订的 setfattr branch 协议。

兑现 REQ-F1-A（单 /workspace 视图）/ B（≤8s 三段式）/ C（10k 文件埋点）/ D（50MB 拒绝）+ REQ-F2-A/B/C（四档切换 + 禁止静默降级 + banner 着色）。

Purpose: Phase 31 主交付，所有 mount 路径错误都必须经 errcodes.Format 包装，banner / 三段式 / last-session.json 三个 surface 100% 覆盖降级历史。
Output: 4 个 mount_*.go 文件 + 4 个辅助文件（askpass / watcher / last_session / colors）+ ssh.go 新 V3 入口 + main.go cobra flag；12 个降级矩阵单元测试 + 至少 3 个安全门 / 50MB / banner / NO_COLOR 用例 PASS。
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/execute-plan.md
@.cursor/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/phases/31-cli/31-CONTEXT.md
@.planning/phases/31-cli/31-RESEARCH.md
@.planning/phases/29-v3-worker/29-CONTEXT.md
@.planning/phases/30-entry-api/30-CONTEXT.md
@.planning/phases/31-cli/plans/01-errcodes-mutagen-embed/PLAN.md
@internal/cloudclaude/mount.go
@internal/cloudclaude/ssh.go
@internal/cloudclaude/entry.go
@internal/cloudclaude/config.go
@cmd/cloud-claude/main.go

<interfaces>
<!-- 本 plan 创建/扩展的对外 API；Plan 03 必须按此调用。 -->

internal/cloudclaude/mount_strategy.go 导出：

```go
package cloudclaude

type Mode int
const (
    ModeAuto         Mode = iota // 仅作为 cfg.Mode 的输入；返回时永远是 Full / MutagenOnly / SSHFSOnly / Failed 之一
    ModeFull
    ModeMutagenOnly
    ModeSSHFSOnly
    ModeFailed
)

func (m Mode) String() string // "auto" / "full" / "mutagen-only" / "sshfs-only" / "failed"
func ParseMode(s string) (Mode, error)

type MountConfig struct {
    Mode              Mode
    KeepAliveInterval time.Duration       // Phase 32 注入，本阶段默认 15*time.Second
    KeepAliveCountMax int                 // Phase 32 注入，本阶段默认 4
    ClaudeAccountID   string              // 来自 Phase 30 AuthResponse.ClaudeAccountID
    ImageVersion      string              // 同上
    SupportsMutagen   bool                // 同上
    SupportsMergerfs  bool                // 同上
    Cwd               string              // 本地 cwd
    NoColor           bool                // os.Getenv("NO_COLOR") != "" || !isTTY(stderr)
    Logger            io.Writer           // stderr 写入器（默认 os.Stderr）
    LastSessionPath   string              // ~/.cloud-claude/last-session.json
    SyncSessionLock   func(accountID string) (release func(), err error) // Phase 32 接管；本阶段默认 noop
}

// MountWorkspace 顶层入口。返回 cleanup（按 mergerfs → mutagen/sshfs → connections 反向 LIFO 释放）+ 实际生效 mode + error。
// 任何失败必须先 cleanup 已起的资源；error 已被 errcodes.Format 包装为可直接 stderr 的字符串（Plan 03 / main.go 不再二次包装）。
func MountWorkspace(connA, connB *ssh.Client, cfg MountConfig) (cleanup func(), mode Mode, err error)
```

internal/cloudclaude/mount_mutagen.go 导出：

```go
type MutagenStatus struct {
    DaemonReady bool
    AgentReady  bool
    SyncReady   bool
    Conflicts   int
    Reason      string
}

// MutagenHealthCheck 供 Phase 34 doctor 复用。本阶段只在 mount_strategy 内部使用一次。
func MutagenHealthCheck(daemonReady, agentReady, syncReady bool, conflicts int) MutagenStatus
```

internal/cloudclaude/last_session.go 导出：

```go
type LastSessionSnapshot struct {
    SchemaVersion       int                 `json:"schema_version"`
    Timestamp           time.Time           `json:"timestamp"`
    IntendedMode        string              `json:"intended_mode"`
    ActualMode          string              `json:"actual_mode"`
    DowngradeChain      []DowngradeStep     `json:"downgrade_chain"`
    ConflictCount       int                 `json:"conflict_count"`
    ClaudeAccountID     string              `json:"claude_account_id,omitempty"`
    ImageVersion        string              `json:"image_version,omitempty"`
    APFSCaseInsensitive bool                `json:"apfs_case_insensitive"`
}

type DowngradeStep struct {
    From          string `json:"from"`
    To            string `json:"to"`
    ReasonCode    string `json:"reason_code"`
    ReasonMessage string `json:"reason_message"`
}

func WriteLastSession(path string, snap LastSessionSnapshot) error
```

internal/cloudclaude/ssh.go 新增：

```go
// ConnectAndRunClaudeV3 是 Phase 31 主入口：建立 conn-A / conn-B，按 cfg.Mode 调 MountWorkspace，
// 完成后启动远程 claude（兼容 v2.0 runClaude 实现）。
// 注意：proxyCommands 与 cwd 的语义沿用 v2.0；mountCfg.SupportsMutagen=false 时强制走 sshfs-only 路径。
func ConnectAndRunClaudeV3(cfg SSHConfig, claudeArgs []string, cwd string, proxyCommands []string,
    mountCfg MountConfig, authResp *AuthResponse) (int, error)

// ConnectAndRunClaude 保留为 v2.0 兼容入口；内部转 V3 + Mode=ModeSSHFSOnly + 默认 mountCfg。
// （签名与 v2.0 完全一致，cmd/cloud-claude/main.go 旧调用如保留则仍走 sshfs-only）
```
</interfaces>

<mergerfs_remote_command>
<!-- 远程在 conn-A 上执行（与 Phase 29 D-11 完全一致 + 2 路 branch）；mount_merge.go 必须按此构造命令。 -->

```bash
sudo mergerfs \
  -o category.create=ff,func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,inodecalc=path-hash \
  /workspace-hot=RW:/workspace-cold=NC,RO \
  /workspace
```

runtime branch 摘除（cold 抖动 watcher 触发，与 RESEARCH §2.2 完全一致）：

```bash
setfattr -n user.mergerfs.branches -v "-/workspace-cold" /workspace
```

如返回「Operation not permitted」，wrapped sudo 重试：

```bash
sudo setfattr -n user.mergerfs.branches -v "-/workspace-cold" /workspace
```
</mergerfs_remote_command>

<mutagen_sync_command>
<!-- mount_mutagen.go mountMutagen 函数构造的核心命令；必须用 conn-C（Mutagen 自起 ssh 子进程）。 -->

环境变量：
```
MUTAGEN_DATA_DIRECTORY=$HOME/.cloud-claude/mutagen
SSH_ASKPASS=$HOME/.cloud-claude/run/ssh-askpass.sh
SSH_ASKPASS_REQUIRE=force
DISPLAY=:0
CLOUD_CLAUDE_SSH_PASS=<password from cfg>
```

daemon 启动（幂等，daemon already started 也算成功）：
```
$HOME/.cloud-claude/bin/mutagen daemon start
```

sync create（alpha = 本地 cwd / beta = ssh://user@host:port/workspace-hot）：
```
$HOME/.cloud-claude/bin/mutagen sync create \
  --name=cloud-claude-{account_id_or_anon}-{cwd_hash8} \
  --mode=two-way-resolved \
  --default-owner-beta=id:1000 \
  --default-group-beta=id:1000 \
  --ignore-vcs \
  --global-config=$HOME/.cloud-claude/mutagen-defaults.yml \
  . \
  {ssh_user}@{ssh_host}:{ssh_port}:/workspace-hot
```

sync list（解析 conflict 计数，使用 go-template 因为 v0.18.1 不支持 --json）：
```
$HOME/.cloud-claude/bin/mutagen sync list \
  --template '{{range .}}{{.Name}}|{{len .Conflicts}}|{{.LastError}}{{"\n"}}{{end}}'
```

mutagen-defaults.yml（运行时生成，写入 ~/.cloud-claude/mutagen-defaults.yml）：
```yaml
sync:
  defaults:
    ignore:
      vcs: true
      paths:
        - "node_modules/"
        - "target/"
        - "dist/"
        - "*.pyc"
        - ".venv/"
        - "__pycache__/"
        - ".next/"
        - "build/"
        - ".cache/"
        - ".DS_Store"
```
</mutagen_sync_command>
</context>

<tasks>

<task type="auto">
  <name>Task 2.1: 拆分 mount.go → mount_sshfs.go + 抽取共享 helper + 新增 colors.go / askpass.go / last_session.go / sshfs_watcher.go 骨架</name>
  <files>
    internal/cloudclaude/mount.go
    internal/cloudclaude/mount_sshfs.go
    internal/cloudclaude/colors.go
    internal/cloudclaude/askpass.go
    internal/cloudclaude/askpass_test.go
    internal/cloudclaude/last_session.go
    internal/cloudclaude/last_session_test.go
    internal/cloudclaude/sshfs_watcher.go
    internal/cloudclaude/sshfs_watcher_test.go
  </files>
  <read_first>
    - internal/cloudclaude/mount.go（v2.0 单文件实现 — 本任务原样剥离 mountWorkspace）
    - internal/cloudclaude/mount_test.go（v2.0 waitForMount 测试 — 必须继续 PASS）
    - .planning/phases/31-cli/31-CONTEXT.md（D-01 拆分四文件、D-17 banner 颜色、D-22 OAuth、D-27 watcher）
    - .planning/phases/31-cli/31-RESEARCH.md §1.2（conn-C + askpass + SSH_ASKPASS_REQUIRE=force）、§2.2（setfattr 协议）、§2.3（timeout 2 mountpoint）、§8（last-session.json schema）
    - .planning/phases/31-cli/plans/01-errcodes-mutagen-embed/PLAN.md（errcodes 的 15 个 Code 常量与 Format 模板）
  </read_first>
  <action>
    1. **mount.go 重构**：保留以下共享 helper 在 mount.go：
       - `MountNotReadyError` struct + Error() + Unwrap()
       - `channelRWC` struct + Close()
       - `waitForMount(mountPath, check, interval, timeout)` 函数（保持签名不变）
       - `fusermountCleanup(conn, remotePath)` 函数
       - `cleanupStaleFUSE(conn, remotePath)` 函数
       - `rmdirChain(conn, path)` 函数
       - `sshRun(conn, cmd)` 函数
       - `shellQuote(s string)` 函数
       
       删除 `mountWorkspace` 函数（移到 mount_sshfs.go）。包注释保留 v2.0 风格描述并加注「Phase 31 拆分后 mount.go 仅承载共享 helper；具体 mount 实现见 mount_{sshfs,mutagen,merge,strategy}.go」。

    2. **mount_sshfs.go**：
       - 新增 `mountSSHFS(conn *ssh.Client, localDir, remoteCold string) (cleanup func(), err error)` 函数
       - 函数体复制 v2.0 mountWorkspace 完整逻辑，**唯一改动**：sshfsCmd 字符串追加四个参数，从：
         `sshfs : %s -o passive -f`
         改为：
         `sshfs : %s -o passive,reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10 -f`
       - 错误返回保留 v2.0 中文 wrap 风格，但替换为 `errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, err.Error())` 包装的 fmt.Errorf：
         ```go
         return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, err.Error()), err)
         ```
       - 新增 `mountWorkspace(conn, localDir, remotePath)` 兼容 alias：
         ```go
         // 兼容 v2.0 旧调用入口（ConnectAndRunClaude 仍调用此函数）；内部直接转 mountSSHFS。
         func mountWorkspace(conn *ssh.Client, localDir, remotePath string) (func(), error) {
             return mountSSHFS(conn, localDir, remotePath)
         }
         ```

    3. **colors.go**（极简实现，不引入第三方包）：
       ```go
       package cloudclaude

       import (
           "os"
           "golang.org/x/term"
       )

       const (
           ansiReset  = "\033[0m"
           ansiGreen  = "\033[32m"
           ansiYellow = "\033[33m"
           ansiRed    = "\033[31m"
           ansiCyan   = "\033[36m"
       )

       // colorEnabled 判定是否输出 ANSI 颜色：NO_COLOR=任意非空 + 非 TTY 都关色。
       func colorEnabled(noColor bool, w interface{ Fd() uintptr }) bool {
           if noColor {
               return false
           }
           if os.Getenv("NO_COLOR") != "" {
               return false
           }
           if w == nil {
               return false
           }
           return term.IsTerminal(int(w.Fd()))
       }

       // colorize 包装文本为 ANSI 着色。enabled=false 时返回原文。
       func colorize(s, ansi string, enabled bool) string {
           if !enabled {
               return s
           }
           return ansi + s + ansiReset
       }
       ```

    4. **askpass.go**（CONTEXT D-25 + RESEARCH §1.2 落实）：
       ```go
       package cloudclaude

       import (
           "fmt"
           "os"
           "path/filepath"
       )

       // AskpassHelper 创建 ssh-askpass 临时 helper 脚本。Mutagen fork ssh 子进程时通过 SSH_ASKPASS 拿密码。
       // 密码经环境变量 CLOUD_CLAUDE_SSH_PASS 透传，**不**进入 ps 输出的命令行参数。
       type AskpassHelper struct {
           ScriptPath string   // 脚本绝对路径（0700）
           cleanup    func()
       }

       // NewAskpassHelper 在 ~/.cloud-claude/run/ 下创建临时 helper 脚本。
       // 调用方必须在 Mutagen 命令完全退出后调用 Helper.Cleanup() 删除脚本。
       func NewAskpassHelper() (*AskpassHelper, error) {
           home, err := os.UserHomeDir()
           if err != nil {
               return nil, fmt.Errorf("无法获取用户主目录: %w", err)
           }
           runDir := filepath.Join(home, ".cloud-claude", "run")
           if err := os.MkdirAll(runDir, 0700); err != nil {
               return nil, fmt.Errorf("创建 askpass 目录失败: %w", err)
           }
           f, err := os.CreateTemp(runDir, "ssh-askpass-*.sh")
           if err != nil {
               return nil, err
           }
           // 脚本内容：直接 echo 环境变量；MUST end with newline
           const body = "#!/bin/sh\nprintf '%s' \"$CLOUD_CLAUDE_SSH_PASS\"\n"
           if _, err := f.WriteString(body); err != nil {
               f.Close()
               os.Remove(f.Name())
               return nil, err
           }
           if err := f.Close(); err != nil {
               os.Remove(f.Name())
               return nil, err
           }
           if err := os.Chmod(f.Name(), 0700); err != nil {
               os.Remove(f.Name())
               return nil, err
           }
           return &AskpassHelper{
               ScriptPath: f.Name(),
               cleanup:    func() { _ = os.Remove(f.Name()) },
           }, nil
       }

       // Env 返回供 exec.Cmd.Env 使用的 5 个变量；调用方在 cmd.Env = append(os.Environ(), helper.Env(password)...) 中合并。
       func (h *AskpassHelper) Env(password string) []string {
           return []string{
               "SSH_ASKPASS=" + h.ScriptPath,
               "SSH_ASKPASS_REQUIRE=force",
               "DISPLAY=:0",
               "SETSID=1",
               "CLOUD_CLAUDE_SSH_PASS=" + password,
           }
       }

       // Cleanup 删除临时脚本。可重复调用（幂等）。
       func (h *AskpassHelper) Cleanup() {
           if h.cleanup != nil {
               h.cleanup()
               h.cleanup = nil
           }
       }
       ```

       **askpass_test.go**：
       - Test_AskpassHelper_CreateAndCleanup：创建后断言 stat ScriptPath 0700、Cleanup 后文件不存在
       - Test_AskpassHelper_Env：断言返回 5 个 KV，CLOUD_CLAUDE_SSH_PASS=<password> 存在
       - Test_AskpassHelper_PasswordNotInPath：断言 ScriptPath 不含 password 字符（防回归到「写密码到文件名」）

    5. **last_session.go**：按 <interfaces> 中 LastSessionSnapshot / DowngradeStep 类型 + WriteLastSession 函数实现。
       - WriteLastSession：os.MkdirAll 父目录 0700 → json.MarshalIndent → 写临时文件 → os.Rename（atomic）
       - 失败返回 error 但不 panic（调用方在 stderr 输出 warning，不阻断 mount 路径）

       **last_session_test.go**：
       - Test_WriteLastSession_Schema：写一个 snapshot，读回来验证 JSON 字段全集（尤其 schema_version=1 和 downgrade_chain 数组结构）
       - Test_WriteLastSession_Atomic：写后断言无 ".tmp" 残留文件

    6. **sshfs_watcher.go**（CONTEXT D-27 + RESEARCH §2.3）：
       ```go
       package cloudclaude

       import (
           "context"
           "fmt"
           "io"
           "time"

           "golang.org/x/crypto/ssh"
           "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
       )

       // SSHFSWatcher 在 conn-A 上每 interval 检查一次 mountpoint，连续 failures 次失败后调 onDisconnect。
       // ctx cancel 即停止；可重复调 Stop()。
       type SSHFSWatcher struct {
           conn         *ssh.Client
           coldPath     string
           interval     time.Duration
           failureLimit int
           logger       io.Writer
           onDisconnect func() error
       }

       func NewSSHFSWatcher(conn *ssh.Client, coldPath string, logger io.Writer, onDisconnect func() error) *SSHFSWatcher {
           return &SSHFSWatcher{
               conn:         conn,
               coldPath:     coldPath,
               interval:     5 * time.Second,
               failureLimit: 3,
               logger:       logger,
               onDisconnect: onDisconnect,
           }
       }

       // Run 阻塞运行；调用方应放在 goroutine 里。
       func (w *SSHFSWatcher) Run(ctx context.Context) {
           t := time.NewTicker(w.interval)
           defer t.Stop()
           failures := 0
           for {
               select {
               case <-ctx.Done():
                   return
               case <-t.C:
                   if w.checkOnce() {
                       failures = 0
                       continue
                   }
                   failures++
                   if failures >= w.failureLimit {
                       fmt.Fprintln(w.logger, errcodes.Format(errcodes.MOUNT_SSHFS_DISCONNECTED))
                       if w.onDisconnect != nil {
                           _ = w.onDisconnect()
                       }
                       return
                   }
               }
           }
       }

       // checkOnce 远程 timeout 2 mountpoint -q <coldPath>；exit 0 = mounted。
       func (w *SSHFSWatcher) checkOnce() bool {
           cmd := fmt.Sprintf("timeout 2 mountpoint -q %s", shellQuote(w.coldPath))
           return sshRun(w.conn, cmd) == nil
       }
       ```

       **sshfs_watcher_test.go**：
       - Test_SSHFSWatcher_OnDisconnectCalledAfter3Failures：用 mock checker（重写 checkOnce 为函数注入 / 用 interface 抽象）；快速 interval=10ms，3 次失败后断言 onDisconnect 被调
       - Test_SSHFSWatcher_RecoverResetsCounter：失败 2 次后成功，再失败 1 次不触发
       - 注：因 checkOnce 直接用 ssh.Client，单测应将 SSHFSWatcher 的 check 函数抽象为 `check func() bool` 字段（非 interface 注入更简洁），并在测试中直接构造。executor 在实现时可改为：
         ```go
         type SSHFSWatcher struct {
             check func() bool   // 默认值在 NewSSHFSWatcher 中绑定为 w.checkOnce
             ...
         }
         ```
  </action>
  <acceptance_criteria>
    - `gofmt -l internal/cloudclaude/` 输出为空
    - `go build ./...` 退出码 0
    - 拆分正确性：
      `! grep -F 'func mountWorkspace(' internal/cloudclaude/mount.go`（mountWorkspace 已搬走）
      `grep -F 'func mountSSHFS(' internal/cloudclaude/mount_sshfs.go`（命中 1 行）
      `grep -F 'reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10' internal/cloudclaude/mount_sshfs.go`（命中 1 行）
      `grep -F 'func waitForMount(' internal/cloudclaude/mount.go`（保留 helper）
      `grep -F 'func sshRun(' internal/cloudclaude/mount.go`（保留 helper）
      `grep -F 'func shellQuote(' internal/cloudclaude/mount.go`（保留 helper）
    - 单测：`go test ./internal/cloudclaude/ -run 'TestWaitForMount|Test_AskpassHelper|Test_WriteLastSession|Test_SSHFSWatcher' -count=1 -v` 全 PASS
    - 安全检查：`! grep -F 'CLOUD_CLAUDE_SSH_PASS=' internal/cloudclaude/askpass.go` 误用为命令行参数 — 应只出现在 Env() 返回的 KV 中
    - schema_version: `grep -F '"schema_version"' internal/cloudclaude/last_session.go` 命中 1 行
  </acceptance_criteria>
  <verify>
    <automated>go build ./... && go test ./internal/cloudclaude/ -run 'TestWaitForMount|Test_AskpassHelper|Test_WriteLastSession|Test_SSHFSWatcher' -count=1 -v && grep -F 'func mountSSHFS(' internal/cloudclaude/mount_sshfs.go && grep -F 'reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10' internal/cloudclaude/mount_sshfs.go && grep -F 'func waitForMount(' internal/cloudclaude/mount.go</automated>
  </verify>
  <done>
    mount.go 拆分完成，mountSSHFS 含四个新参数；askpass / last_session / sshfs_watcher / colors 四个辅助文件就位 + 各自单测 PASS；v2.0 兼容 alias mountWorkspace 仍可被旧 ConnectAndRunClaude 调用。
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2.2: mount_mutagen.go + mount_merge.go + mount_strategy.go 三层实现 + 12 个降级矩阵单测</name>
  <files>
    internal/cloudclaude/mount_mutagen.go
    internal/cloudclaude/mount_mutagen_test.go
    internal/cloudclaude/mount_merge.go
    internal/cloudclaude/mount_strategy.go
    internal/cloudclaude/mount_strategy_test.go
  </files>
  <behavior>
    mount_strategy_test.go 12 个降级矩阵用例（Mode × failure injection）：
    - cfg.Mode=ModeAuto，注入 mutagenFn=fail / sshfsFn=ok / mergeFn=ok → final=ModeMutagenOnly + 降级 banner 含 MOUNT_AUTO_DOWNGRADED；wait≤2s
    - cfg.Mode=ModeAuto，mutagenFn=ok / sshfsFn=fail / mergeFn=ok → final=ModeMutagenOnly（mergerfs 仍需 cold ready 才能挂；只有 mutagen 时 fallback 到「单层 mutagen → /workspace」由 mount_strategy 内部调度）
    - cfg.Mode=ModeAuto，mutagenFn=ok / sshfsFn=ok / mergeFn=fail → final=ModeMutagenOnly（mergerfs 失败 → 只剩 mutagen）/ 或 ModeSSHFSOnly（取决于实现选择，executor 必须在 PLAN 注释中说明降级方向，本 plan 推荐 → ModeMutagenOnly）
    - cfg.Mode=ModeAuto，三层全 fail → final=ModeFailed + err != nil
    - cfg.Mode=ModeFull，任一层 fail → err != nil + errcodes.MOUNT_FORCE_MODE_FAILED + final=ModeFailed
    - cfg.Mode=ModeMutagenOnly，mutagen ok → final=ModeMutagenOnly；mutagen fail → err != nil
    - cfg.Mode=ModeSSHFSOnly，sshfs ok → final=ModeSSHFSOnly；sshfs fail → err != nil

    mount_mutagen_test.go：
    - Test_SafetyGuard_AlphaEmptyBetaNonEmpty：alpha=空目录 + remoteFindFn 返回 stdout 非空 → 退出 + errcodes.MOUNT_MUTAGEN_SAFETY_GUARD + sync 未创建（mock createFn 不被调）
    - Test_50MBReject：duFn 返回 60_000_000 → errcodes.MOUNT_MUTAGEN_WHITELIST_REJECT + 中文 ignore 建议 + sync 未创建
    - Test_VersionSkew：localVersion=v0.18.1 / remoteVersionFn 返回 "v0.99.0" → errcodes.MOUNT_MUTAGEN_VERSION_SKEW + 调用方收到 sentinel error 触发降级
    - Test_BannerColors：mode=ModeFull + noColor=false + tty=true → banner 含 ansiGreen；noColor=true → banner 不含 ANSI
    - Test_APFSCaseInsensitive_WritesLastSession：用 t.TempDir 不可能命中 APFS，但用 cfg.OverrideCaseInsensitive=true（测试 hook） → last-session.json 含 apfs_case_insensitive: true
    - Test_DowngradeBannerEachStep：注入 mutagenFn=fail 后 mergeFn=fail，断言 stderr 出现两条 MOUNT_AUTO_DOWNGRADED 行
  </behavior>
  <read_first>
    - .planning/phases/31-cli/31-CONTEXT.md（D-08~D-13 安全门 + ignore 列表 + 50MB / D-14~D-18 状态机 / D-25~D-27 三连接模型 / D-29 AuthResponse 字段降级 / D-31~D-32 接口预留）
    - .planning/phases/31-cli/31-RESEARCH.md §1.1（mutagen 命令清单 + daemon 协议 + 没有 --json）、§1.2（conn-C + askpass）、§1.3（mode flag + ignore yaml）、§2.1（mergerfs 命令）、§2.2（setfattr 协议）、§5.2（错误码文案 — 已在 errcodes 包，本 task 直接 errcodes.Format 调用）、§8（last-session.json schema）
    - .planning/phases/29-v3-worker/29-CONTEXT.md D-11 / D-12（mergerfs 参数 + 2 路 branch + CLOUD_CLAUDE_MERGERFS_BRANCHES 扩展点）
    - .planning/phases/30-entry-api/30-CONTEXT.md D-03 / D-05 / D-06 / D-07（AuthResponse 字段语义）
    - internal/cloudclaude/mount_sshfs.go（Task 2.1 产出）
    - internal/cloudclaude/errcodes/codes.go（15 个 Code 常量）
    - internal/cloudclaude/mutagen_bin.go（ExtractMutagenBinary）
    - internal/cloudclaude/envcheck_fs.go（IsCaseInsensitiveFS）
    - internal/cloudclaude/last_session.go（WriteLastSession）
    - internal/cloudclaude/sshfs_watcher.go（NewSSHFSWatcher）
    - internal/cloudclaude/colors.go
    - internal/cloudclaude/askpass.go
  </read_first>
  <action>
    1. **mount_mutagen.go**：

       核心导出函数：
       ```go
       // mountMutagen 启动 Mutagen daemon → 版本握手 → 50MB 检查 → 安全门 → sync create（conn-C）。
       // 返回 cleanup（terminate sync session + Cleanup askpass helper）。
       // 入参 connA 用于远程探测（du / find / cat 版本文件 / mountpoint），不是 mutagen sync 的 transport。
       // mutagenSyncCfg 含 ssh_user / ssh_host / ssh_port / password（从 SSHConfig 透传）。
       func mountMutagen(connA *ssh.Client, mutagenSyncCfg MutagenSyncConfig, deps mountMutagenDeps) (cleanup func(), err error)

       // MutagenSyncConfig 来自上层 MountConfig + SSHConfig，由 mount_strategy 拼装。
       type MutagenSyncConfig struct {
           AlphaCwd        string         // 本地 cwd
           BetaPath        string         // 远端路径，固定 /workspace-hot
           SSHUser         string
           SSHHost         string
           SSHPort         int
           Password        string         // conn-C askpass 通过环境变量传递
           ClaudeAccountID string
           SessionName     string         // cloud-claude-{account_or_anon}-{cwd_hash8}
       }

       // mountMutagenDeps 是供测试注入的接口集合，生产代码使用默认实现：defaultMutagenDeps()。
       type mountMutagenDeps struct {
           extractBinary  func(dst string) error                                  // ExtractMutagenBinary
           runLocal       func(name string, args []string, env []string) (string, error) // exec.Command 包装
           remoteRun      func(conn *ssh.Client, cmd string) (string, error)      // 含 stdout 收集
           remoteVersion  func(conn *ssh.Client) (string, error)                  // cat /etc/cloud-claude/mutagen.version
           remoteFindBeta func(conn *ssh.Client, path string) (bool, error)       // find -mindepth 1 -maxdepth 1 -not -name '.*' | head -1 是否非空
           localDuBytes   func(path string) (int64, error)                        // du -sb {path}
           localDuTopN    func(path string) ([]string, error)                     // du -sh {path}/* | sort -hr | head -3
           writeIgnoreYML func(path string) error                                 // 生成 ~/.cloud-claude/mutagen-defaults.yml
           newAskpass     func() (*AskpassHelper, error)                          // NewAskpassHelper
       }
       ```

       函数体伪代码（执行顺序与防御点）：

       a. ExtractMutagenBinary 到 ~/.cloud-claude/bin/mutagen → fail 返回 errcodes.MOUNT_MUTAGEN_TRANSPORT_FAILED 或 MOUNT_MUTAGEN_DAEMON_UNAVAILABLE
       
       b. mutagen daemon start（env: MUTAGEN_DATA_DIRECTORY=$HOME/.cloud-claude/mutagen）→ 输出含 "daemon already started" 视为 OK；其它非 0 退出 → MOUNT_MUTAGEN_DAEMON_UNAVAILABLE
       
       c. **版本握手（C4 防御）**：本地 mutagen version | grep "v0.18.1" + remoteVersion(connA) → 不一致返回 sentinel error 包装 errcodes.MOUNT_MUTAGEN_VERSION_SKEW
       
       d. **50MB 检查（REQ-F1-D）**：localDuBytes(cwd) > 52_428_800 → 收集 localDuTopN → 返回包装 errcodes.MOUNT_MUTAGEN_WHITELIST_REJECT 的 sentinel error；调用方（mount_strategy）按 mode 决定降级 / 退出
       
       e. **安全门（C5 / D-13）**：os.ReadDir(cwd) 过滤 ignore 列表后 entries empty AND remoteFindBeta(connA, "/workspace-hot")=true → 直接返回 errcodes.MOUNT_MUTAGEN_SAFETY_GUARD（**Fatal，不可降级**，调用方必须退出非 0）
       
       f. writeIgnoreYML(~/.cloud-claude/mutagen-defaults.yml) — 内容按 <mutagen_sync_command> 中 yaml 模板
       
       g. NewAskpassHelper → 拿 helper.ScriptPath 与 Env(password)
       
       h. exec.Command(mutagenBin, "sync", "create", ...) 命令按 <mutagen_sync_command> 拼装；env = append(os.Environ(), helper.Env(password)...) + MUTAGEN_DATA_DIRECTORY；命令超时 30s（context.WithTimeout）；非 0 → MOUNT_MUTAGEN_SYNC_FAILED + helper.Cleanup() + 返回 error
       
       i. cleanup = func() { mutagen sync terminate <SessionName>；helper.Cleanup() }

       **注意：daemon 不停**（CONTEXT D-05），cleanup **不**调 daemon stop。

       **导出的 MutagenHealthCheck**（按 <interfaces>）：仅是 status struct 包装，本 plan 内部不使用，留给 Phase 34 doctor。

    2. **mount_merge.go**：

       ```go
       // mountMerge 在 connA 上执行 sudo mergerfs（参数与 Phase 29 D-11 完全一致），target 默认 /workspace。
       // branches 默认 ["/workspace-hot=RW", "/workspace-cold=NC,RO"]；可由 CLOUD_CLAUDE_MERGERFS_BRANCHES 环境变量覆盖（CONTEXT D-26）。
       // 返回 cleanup：sudo fusermount -uz /workspace。
       func mountMerge(connA *ssh.Client, branches []string, target string) (cleanup func(), err error)

       // RemoveBranch 在 connA 上远程摘除指定 branch（cold 抖动 watcher 触发）。
       // 命令：setfattr -n user.mergerfs.branches -v "-<branchPath>" <target>
       // 失败先尝试无 sudo，再尝试 sudo 包装。
       func RemoveBranch(connA *ssh.Client, branchPath, target string) error
       ```

       函数体：mergerfs 命令字符串硬编码（按 <mergerfs_remote_command>）；branches 用 ":" 拼接；非 0 → errcodes.MOUNT_MERGERFS_FAILED 包装。

    3. **mount_strategy.go** —— 顶层入口与状态机：

       核心逻辑：

       a. **入口签名**按 <interfaces> 中 MountWorkspace。

       b. **APFS 检测**：在最早执行（cfg.Cwd 上调 IsCaseInsensitiveFS）；命中 → snapshot.APFSCaseInsensitive=true + 在 stderr 输出 errcodes.Format(MOUNT_APFS_CASE_INSENSITIVE)。两路同步模式默认就是 two-way-resolved，此处只是 informational。

       c. **能力降级（CONTEXT D-29）**：
          - cfg.SupportsMutagen=false → 强制 cfg.Mode = ModeSSHFSOnly + 记录降级原因 MOUNT_MUTAGEN_VERSION_SKEW
          - cfg.SupportsMergerfs=false → 如 cfg.Mode=ModeAuto/ModeFull，降级到 ModeMutagenOnly
          - cfg.ImageVersion != "v3.0.0" → 同上 MOUNT_MUTAGEN_VERSION_SKEW

       d. **状态机**：按 cfg.Mode 调 sequence：
          - ModeAuto: try [Full, MutagenOnly, SSHFSOnly]，每档 ctx, _ = context.WithTimeout(ctx, 2*time.Second)；超时或 err 即 cleanup 当前档 → 在 stderr 输出 errcodes.Format(MOUNT_AUTO_DOWNGRADED, from, to, code, msg) → 转下一档
          - ModeFull / ModeMutagenOnly / ModeSSHFSOnly: 单档跑，失败即 errcodes.MOUNT_FORCE_MODE_FAILED + 退出非 0
          - ModeFailed: 兜底 sshfs-only 也失败时返回

       e. **三段式中文进度**：在每档 try 之前（按 finalMode 决策后顺序打印 3 行）：
          - mutagen tier ready 之前：`(1/3) 热同步源码中…` 或 `(1/3) 跳过 Mutagen（模式: <mode>）`
          - sshfs tier ready 之前：`(2/3) 启动冷兜底…` 或 `(2/3) 跳过 sshfs（模式: <mode>）`
          - mergerfs tier ready 之前：`(3/3) 合并视图…` 或 `(3/3) 跳过 mergerfs（模式: <mode>）`
          - **顺序**：先决策 finalMode 再打印（CONTEXT D-18 强约束 — 不会出现「打 (1/3) 又改主意」）

       f. **banner 输出**：mount 全 ready 后打印一次：
          ```
          ✓ 文件映射就绪 [<mode>]
          ```
          着色：full=ansiGreen, mutagen-only=ansiYellow, sshfs-only=ansiYellow；NO_COLOR / 非 TTY 关色

       g. **conflict 冒泡**：mount ready 后调 mountMutagen 提供的 conflict count（通过 mutagen sync list --template 解析）；> 0 时 banner 后插入：
          `⚠ 有 N 个文件同步冲突，运行 cloud-claude sync conflicts 查看`
          （注：完整 sync conflicts 子命令在 Plan 03 落地，本 task 只输出 banner 行）

       h. **last-session.json 写入**：mount 路径完成（成功 or 失败均写）后调 WriteLastSession(cfg.LastSessionPath, snapshot)；失败 stderr warning 不阻塞

       i. **cleanup LIFO 顺序**：mergerfs → mutagen / sshfs → connections；mount_strategy 返回的 cleanup 内部按此顺序调子层 cleanup

       j. **sshfs watcher 启动**：full 模式下 mount ready 后 NewSSHFSWatcher(connA, "/workspace-cold", logger, onDisconnect = func() error { return RemoveBranch(connA, "/workspace-cold", "/workspace") })；放在 errgroup 里 ctx cancel 即停（cleanup 时 cancel）

    4. **mount_strategy_test.go**：按 <behavior> 12 个用例。
       - 用 deps 注入：mountFns map[string]func() error；mount_strategy 接受可选 testHooks 字段（仅 build tag _test 暴露）实现：
         ```go
         type strategyHooks struct {
             tryMutagen func() (cleanup func(), err error)
             trySSHFS   func() (cleanup func(), err error)
             tryMerge   func() (cleanup func(), err error)
         }
         ```
         生产代码 hook = nil 时调真实实现；测试代码注入 mock。
       - 12 个组合表驱动测试 + 输出 stderr 断言（用 bytes.Buffer 替代 cfg.Logger）

    5. **mount_mutagen_test.go**：按 <behavior> 6 个用例，全部用 mountMutagenDeps 注入 mock。
  </action>
  <acceptance_criteria>
    - 文件存在性：
      `test -f internal/cloudclaude/mount_mutagen.go`
      `test -f internal/cloudclaude/mount_merge.go`
      `test -f internal/cloudclaude/mount_strategy.go`
    - 关键函数与签名：
      `grep -E 'func MountWorkspace\(connA, connB \*ssh\.Client, cfg MountConfig\)' internal/cloudclaude/mount_strategy.go` 命中
      `grep -E 'func mountMutagen\(' internal/cloudclaude/mount_mutagen.go` 命中
      `grep -E 'func mountMerge\(' internal/cloudclaude/mount_merge.go` 命中
      `grep -E 'func RemoveBranch\(' internal/cloudclaude/mount_merge.go` 命中
      `grep -F 'MutagenHealthCheck' internal/cloudclaude/mount_mutagen.go` 命中
    - 关键命令拼接（grep 严格匹配）：
      `grep -F 'category.create=ff,func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,inodecalc=path-hash' internal/cloudclaude/mount_merge.go` 命中
      `grep -F 'setfattr -n user.mergerfs.branches' internal/cloudclaude/mount_merge.go` 命中
      `grep -F '--mode=two-way-resolved' internal/cloudclaude/mount_mutagen.go` 命中
      `grep -F '--default-owner-beta=id:1000' internal/cloudclaude/mount_mutagen.go` 命中
      `grep -F '--default-group-beta=id:1000' internal/cloudclaude/mount_mutagen.go` 命中
      `grep -F '--ignore-vcs' internal/cloudclaude/mount_mutagen.go` 命中
      `grep -F 'MUTAGEN_DATA_DIRECTORY' internal/cloudclaude/mount_mutagen.go` 命中
      `grep -F 'mutagen daemon start' internal/cloudclaude/mount_mutagen.go` 命中
      `grep -F '--template' internal/cloudclaude/mount_mutagen.go` 命中（不能是 --json）
      `! grep -F 'sync list --json' internal/cloudclaude/mount_mutagen.go`（修订 D-28 落实）
    - 50MB / 安全门 / 中文进度文案：
      `grep -F '52428800' internal/cloudclaude/mount_mutagen.go` 命中（50MB 阈值）
      `grep -F '热同步源码中' internal/cloudclaude/mount_strategy.go` 命中
      `grep -F '启动冷兜底' internal/cloudclaude/mount_strategy.go` 命中
      `grep -F '合并视图' internal/cloudclaude/mount_strategy.go` 命中
      `grep -F '✓ 文件映射就绪' internal/cloudclaude/mount_strategy.go` 命中
      `grep -F '同步冲突' internal/cloudclaude/mount_strategy.go` 命中
    - 单元测试：
      `go test ./internal/cloudclaude/ -run 'Test_SafetyGuard|Test_50MBReject|Test_VersionSkew|Test_BannerColors|Test_APFS|Test_Downgrade' -count=1 -v` 全 PASS
      `go test ./internal/cloudclaude/ -run TestMountStrategy_DowngradeMatrix -count=1 -v` 至少 12 个子用例 PASS
    - 整仓回归：`go test ./... -count=1` 退出码 0
    - `gofmt -l internal/cloudclaude/` 输出为空
  </acceptance_criteria>
  <verify>
    <automated>go build ./... && go test ./internal/cloudclaude/ -run 'TestMountStrategy_DowngradeMatrix|Test_SafetyGuard|Test_50MBReject|Test_VersionSkew|Test_BannerColors|Test_APFS|Test_Downgrade' -count=1 -v && grep -F 'category.create=ff,func.readdir=cor:4' internal/cloudclaude/mount_merge.go && grep -F 'setfattr -n user.mergerfs.branches' internal/cloudclaude/mount_merge.go && grep -F '--mode=two-way-resolved' internal/cloudclaude/mount_mutagen.go && grep -F 'MOUNT_AUTO_DOWNGRADED' internal/cloudclaude/mount_strategy.go && ! grep -F 'sync list --json' internal/cloudclaude/mount_mutagen.go</automated>
  </verify>
  <done>
    三层 mount 文件齐备；状态机按 4 档 mode × 3 层失败注入跑出 12 个降级矩阵全 PASS；安全门 / 50MB / 版本握手 / banner 颜色 / APFS / 降级 banner 6 项专项测试 PASS；mergerfs 命令与 setfattr 协议与 Phase 29 / RESEARCH §2.2 字符级一致。
  </done>
</task>

<task type="auto">
  <name>Task 2.3: ConnectAndRunClaudeV3 入口 + cmd/cloud-claude/main.go --mount-mode flag 接线</name>
  <files>
    internal/cloudclaude/ssh.go
    cmd/cloud-claude/main.go
  </files>
  <read_first>
    - internal/cloudclaude/ssh.go（v2.0 ConnectAndRunClaude / sshConnect / runClaude）
    - cmd/cloud-claude/main.go（cobra root + DisableFlagParsing + runRoot）
    - internal/cloudclaude/entry.go（AuthResponse 字段，含 v3 扩展）
    - internal/cloudclaude/mount_strategy.go（Task 2.2 产出 — MountConfig / Mode / MountWorkspace / ParseMode）
    - .planning/phases/31-cli/31-CONTEXT.md（D-30 兼容入口策略 / D-14 cobra flag 四档枚举）
  </read_first>
  <action>
    1. **internal/cloudclaude/ssh.go**：

       a. **新增 ConnectAndRunClaudeV3**：
          ```go
          func ConnectAndRunClaudeV3(cfg SSHConfig, claudeArgs []string, cwd string,
              proxyCommands []string, mountCfg MountConfig, authResp *AuthResponse) (int, error) {

              // 1) 建 conn-A
              connA, err := sshConnect(cfg)
              if err != nil { return 0, err }
              defer connA.Close()

              // 2) 建 conn-B（数据通道）
              connB, err := sshConnect(cfg)
              if err != nil { return 0, err }
              defer connB.Close()

              // 3) 注入 AuthResponse 字段到 mountCfg（如 mountCfg 字段未填）
              if authResp != nil {
                  if mountCfg.ClaudeAccountID == "" { mountCfg.ClaudeAccountID = authResp.ClaudeAccountID }
                  if mountCfg.ImageVersion == "" { mountCfg.ImageVersion = authResp.ImageVersion }
                  // 注意：bool 字段没法用 != "" 判断；按 authResp 优先
                  mountCfg.SupportsMutagen = authResp.SupportsMutagen
                  mountCfg.SupportsMergerfs = authResp.SupportsMergerfs
              }
              if mountCfg.LastSessionPath == "" {
                  if home, err := os.UserHomeDir(); err == nil {
                      mountCfg.LastSessionPath = filepath.Join(home, ".cloud-claude", "last-session.json")
                  }
              }
              if mountCfg.Logger == nil {
                  mountCfg.Logger = os.Stderr
              }
              if mountCfg.SyncSessionLock == nil {
                  mountCfg.SyncSessionLock = func(_ string) (func(), error) { return func() {}, nil }
              }
              mountCfg.Cwd = cwd

              // 4) MountWorkspace 调度
              cleanupMount, mode, err := MountWorkspace(connA, connB, mountCfg)
              if err != nil {
                  return 0, fmt.Errorf("文件映射失败: %w", err)
              }
              defer cleanupMount()
              _ = mode // 已经 banner 显示过

              // 5) ExecProxy（沿用 v2.0）
              var proxy *ExecProxy
              if len(proxyCommands) > 0 {
                  proxy = NewExecProxy(cwd)
                  if err := proxy.Start(); err != nil {
                      return 0, fmt.Errorf("启动命令代理失败: %w", err)
                  }
                  defer proxy.Stop()
                  if err := InstallWrappers(cwd, proxyCommands, cwd); err != nil {
                      return 0, fmt.Errorf("安装命令代理脚本失败: %w", err)
                  }
              }

              // 6) Plan 03 将在此插入 OAuth 检查（CheckOAuthCredentials），本 task 留 TODO 注释：
              //    TODO(plan-03): OAuth credentials 检查（mount.Cfg.ClaudeAccountID）
              //    位置：mount ready 之后、runClaude 之前

              // 7) 启动远程 claude（runClaude 由 conn-A 承载，沿用 v2.0）
              return runClaude(connA, claudeArgs, cwd, len(proxyCommands) > 0)
          }
          ```

       b. **改 ConnectAndRunClaude（v2.0 兼容入口）**：保持签名 `ConnectAndRunClaude(cfg SSHConfig, claudeArgs []string, cwd string, proxyCommands []string) (int, error)`，函数体改为：
          ```go
          func ConnectAndRunClaude(cfg SSHConfig, claudeArgs []string, cwd string, proxyCommands []string) (int, error) {
              mountCfg := MountConfig{
                  Mode:              ModeSSHFSOnly, // v2.0 默认行为
                  KeepAliveInterval: 15 * time.Second,
                  KeepAliveCountMax: 4,
              }
              return ConnectAndRunClaudeV3(cfg, claudeArgs, cwd, proxyCommands, mountCfg, nil)
          }
          ```
          这样所有未升级的旧代码继续以 sshfs-only 模式工作，无回归。

    2. **cmd/cloud-claude/main.go**：

       a. **rootCmd 加 PersistentFlags（在 cobra Cmd 创建之后、AddCommand 之前）**：
          ```go
          rootCmd.PersistentFlags().String("mount-mode", "auto", "文件映射模式: auto|full|mutagen-only|sshfs-only")
          ```
          注意：因 rootCmd.DisableFlagParsing=true，flag 解析会被跳过；需要在 runRoot 内手工解析或在 DisableFlagParsing 切换分支增加 mount-mode 处理。**简化方案**：保留 DisableFlagParsing=true，但在 runRoot 入口前先 manual scan os.Args 找 --mount-mode 值后从 args 中剥离，再透传剩余 args 给 claude。
          
          手工解析片段：
          ```go
          // 在 runRoot 顶部：
          mountMode := "auto"
          var filtered []string
          for i := 0; i < len(args); i++ {
              if args[i] == "--mount-mode" && i+1 < len(args) {
                  mountMode = args[i+1]
                  i++
                  continue
              }
              if strings.HasPrefix(args[i], "--mount-mode=") {
                  mountMode = strings.TrimPrefix(args[i], "--mount-mode=")
                  continue
              }
              filtered = append(filtered, args[i])
          }
          args = filtered

          mode, err := cloudclaude.ParseMode(mountMode)
          if err != nil {
              fmt.Fprintln(os.Stderr, "错误: --mount-mode 必须是 auto / full / mutagen-only / sshfs-only 之一")
              os.Exit(exitConfigError)
          }
          ```

       b. **runRoot 切换调用**：把
          ```go
          exitCode, err := cloudclaude.ConnectAndRunClaude(sshCfg, args, cwd, cfg.EffectiveProxyCommands())
          ```
          改为：
          ```go
          mountCfg := cloudclaude.MountConfig{
              Mode:              mode,
              KeepAliveInterval: 15 * time.Second,
              KeepAliveCountMax: 4,
              NoColor:           os.Getenv("NO_COLOR") != "",
          }
          exitCode, err := cloudclaude.ConnectAndRunClaudeV3(sshCfg, args, cwd, cfg.EffectiveProxyCommands(), mountCfg, authResp)
          ```

       c. **保留 v2.0 行为不破坏**：`init` / `env` / `ssh` 子命令路径完全不动；只有 runRoot（默认无子命令）走 V3 入口。

    3. 不要改 `runEnvCheck` / `runSSHDoctor`（它们仍用 v2.0 的 RunEnvCheck / RunSSHDoctor，与 mount 无关）。
  </action>
  <acceptance_criteria>
    - `gofmt -l internal/cloudclaude/ssh.go cmd/cloud-claude/main.go` 输出为空
    - `go build ./...` 退出码 0
    - 关键签名：
      `grep -F 'func ConnectAndRunClaudeV3(' internal/cloudclaude/ssh.go` 命中 1 行
      `grep -F 'func ConnectAndRunClaude(' internal/cloudclaude/ssh.go` 命中 1 行（兼容入口保留）
      `grep -F 'ConnectAndRunClaudeV3(' cmd/cloud-claude/main.go` 命中至少 1 行
      `grep -F '--mount-mode' cmd/cloud-claude/main.go` 命中至少 2 行（flag 注册 + 解析）
      `grep -F 'cloudclaude.ParseMode' cmd/cloud-claude/main.go` 命中 1 行
    - 兼容性：v2.0 ConnectAndRunClaude 签名未变（4 个参数）：
      `grep -E 'func ConnectAndRunClaude\(cfg SSHConfig, claudeArgs \[\]string, cwd string, proxyCommands \[\]string\) \(int, error\)' internal/cloudclaude/ssh.go` 命中
    - 整仓回归：`go test ./... -count=1` 退出码 0
    - 二进制可启动 + flag 可见：
      ```bash
      go build -o /tmp/cloud-claude-test ./cmd/cloud-claude
      /tmp/cloud-claude-test --help 2>&1 | grep -F '--mount-mode' || true
      # 注：因 DisableFlagParsing=true，--help 输出可能不显示 PersistentFlags；改测试 ParseMode 直接 reject 非法值
      /tmp/cloud-claude-test --mount-mode=invalid 2>&1 | grep -E '错误.*mount-mode' || exit 1
      ```
  </acceptance_criteria>
  <verify>
    <automated>go build -o /tmp/cc-test ./cmd/cloud-claude && /tmp/cc-test --mount-mode=invalid 2>&1 | grep -E '错误.*mount-mode' && grep -F 'func ConnectAndRunClaudeV3(' internal/cloudclaude/ssh.go && grep -F 'ConnectAndRunClaudeV3(' cmd/cloud-claude/main.go && grep -F 'cloudclaude.ParseMode' cmd/cloud-claude/main.go && go test ./... -count=1</automated>
  </verify>
  <done>
    cloud-claude 二进制接线完毕：runRoot 走 ConnectAndRunClaudeV3 + cobra --mount-mode 四档 flag；v2.0 ConnectAndRunClaude 签名兼容保留，旧 import 不破；非法 mount-mode 值有明确中文提示。
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2.4: 集中管理退出码（internal/cloudclaude/exitcodes.go）— 解决 OAuth 与 v2.0 main.go 的 4/5 冲突</name>
  <files>
    internal/cloudclaude/exitcodes.go
    internal/cloudclaude/exitcodes_test.go
  </files>
  <behavior>
    exitcodes_test.go 用例：
    - Test_ExitCodes_Unique：用 map[int]bool 遍历所有导出常量，断言无重复值
    - Test_ExitCodes_PosixLimit：每个常量值 ≤ 125（POSIX shell 退出码上限，> 125 与 SIGTERM/SIGKILL 等信号编码冲突）
    - Test_ExitCodes_V2Compat：断言 ExitOK=0、ExitAuthFailed=1、ExitNetworkError=2、ExitTimeout=3、ExitConfigError=4、ExitInternalError=5（与 cmd/cloud-claude/main.go v2.0 现有 exitOK..exitInternalError 完全对齐，避免行为回归）
    - Test_ExitCodes_NewCodesNotConflictV2：断言新增 OAuth 与 mount 退出码（6/7/8）不与 v2.0 0-5 撞码
    - Test_ExitCodes_NamesPresent：用 reflection / 直接访问，断言 ExitOAuthNotFound、ExitOAuthExpired、ExitMountForceFailed 三个常量存在且非 0
  </behavior>
  <read_first>
    - cmd/cloud-claude/main.go（行 16-23，v2.0 现有 6 个 exit* 常量声明位置；行 250-307，os.Exit 调用点）
    - .planning/phases/31-cli/31-CONTEXT.md（D-22 第 3 条 — 原约定 OAuth NotFound=4 / Expired=5；本 task 修订为 6/7 避开 v2.0 冲突，理由：v2.0 已用 4=ConfigError / 5=InternalError，行为不可回归）
    - .planning/phases/31-cli/plans/03-oauth-conflicts-integration/PLAN.md（Task 3.2 ssh.go OAuth 检查的 return 值会引用本 task 创建的常量）
  </read_first>
  <action>
    1. **创建 internal/cloudclaude/exitcodes.go**：

       ```go
       // Package cloudclaude 退出码常量（Phase 31 引入）。
       //
       // 设计原则：
       //   1. 与 v2.0 cmd/cloud-claude/main.go 现有 exit* 常量数值完全对齐（0-5）
       //   2. v3.0 新增 OAuth / mount 等错误路径占用 6-8（避开 v2.0 行为）
       //   3. 全部 ≤ 125（POSIX shell 退出码限制；> 125 与 SIGINT(130) / SIGKILL(137) 等信号编码冲突）
       //   4. ssh.go ConnectAndRunClaudeV3 / mount_strategy.go MountWorkspace 引用这些常量而非裸数字
       //
       // CONTEXT D-22 原约定 OAuth NotFound=4 / Expired=5，与 v2.0 ConfigError=4 / InternalError=5 撞码；
       // 本文件按 plan-checker 反馈修订为 6/7。Phase 34 doctor `cloud-claude explain` 子命令将复用此表。

       package cloudclaude

       const (
           // 0-5：与 v2.0 cmd/cloud-claude/main.go exit* 常量完全对齐（不可改值）
           ExitOK            = 0
           ExitAuthFailed    = 1
           ExitNetworkError  = 2
           ExitTimeout       = 3
           ExitConfigError   = 4
           ExitInternalError = 5

           // 6-8：v3.0 Phase 31 新增（OAuth + mount force）
           ExitOAuthNotFound    = 6 // /home/claude/.claude/.credentials.json 不存在或解析失败（D-22 第 3 条）
           ExitOAuthExpired     = 7 // expiresAt < now（D-22 第 3 条）
           ExitMountForceFailed = 8 // --mount-mode=full|mutagen-only|sshfs-only 任一档失败（D-15 / errcodes.MOUNT_FORCE_MODE_FAILED）
       )
       ```

       注意：本 plan **不**在本 task 中改 cmd/cloud-claude/main.go 的现有 exit* 常量（避免大范围 rename + git diff 噪音）；只是新声明 cloudclaude.Exit* 常量供 Plan 03 与 mount_strategy 引用。Phase 34 / 后续可考虑统一 rename，本 plan 范围外。

    2. **internal/cloudclaude/exitcodes_test.go**：按 <behavior> 5 个用例落地：

       ```go
       package cloudclaude

       import "testing"

       func Test_ExitCodes_Unique(t *testing.T) {
           seen := map[int]string{}
           for name, val := range allExitCodes() {
               if existing, ok := seen[val]; ok {
                   t.Fatalf("duplicate exit code value %d: %s and %s", val, existing, name)
               }
               seen[val] = name
           }
       }

       func Test_ExitCodes_PosixLimit(t *testing.T) {
           for name, val := range allExitCodes() {
               if val < 0 || val > 125 {
                   t.Errorf("%s = %d, must be in [0, 125] (POSIX shell limit)", name, val)
               }
           }
       }

       func Test_ExitCodes_V2Compat(t *testing.T) {
           cases := map[string]int{
               "ExitOK":            0,
               "ExitAuthFailed":    1,
               "ExitNetworkError":  2,
               "ExitTimeout":       3,
               "ExitConfigError":   4,
               "ExitInternalError": 5,
           }
           got := allExitCodes()
           for name, want := range cases {
               if got[name] != want {
                   t.Errorf("%s = %d, want %d (v2.0 main.go compat)", name, got[name], want)
               }
           }
       }

       func Test_ExitCodes_NewCodesNotConflictV2(t *testing.T) {
           v2 := map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true, 5: true}
           newCodes := map[string]int{
               "ExitOAuthNotFound":    ExitOAuthNotFound,
               "ExitOAuthExpired":     ExitOAuthExpired,
               "ExitMountForceFailed": ExitMountForceFailed,
           }
           for name, val := range newCodes {
               if v2[val] {
                   t.Errorf("%s = %d collides with v2.0 0-5 range", name, val)
               }
           }
       }

       func Test_ExitCodes_NamesPresent(t *testing.T) {
           // 编译期保证常量存在，运行期断言数值非 0
           if ExitOAuthNotFound == 0 || ExitOAuthExpired == 0 || ExitMountForceFailed == 0 {
               t.Fatal("new exit codes must be non-zero")
           }
       }

       // allExitCodes 返回所有导出常量的 (name, value) map。
       // 注：Go 没有运行时常量反射；本 helper 用硬编码 map 维护，添加新常量时同步。
       func allExitCodes() map[string]int {
           return map[string]int{
               "ExitOK":               ExitOK,
               "ExitAuthFailed":       ExitAuthFailed,
               "ExitNetworkError":     ExitNetworkError,
               "ExitTimeout":          ExitTimeout,
               "ExitConfigError":      ExitConfigError,
               "ExitInternalError":    ExitInternalError,
               "ExitOAuthNotFound":    ExitOAuthNotFound,
               "ExitOAuthExpired":     ExitOAuthExpired,
               "ExitMountForceFailed": ExitMountForceFailed,
           }
       }
       ```

    3. **不**在本 task 修改 cmd/cloud-claude/main.go 的现有 exit* 常量声明或调用点（v2.0 行为冻结，迁移留 Phase 34）。
  </action>
  <verify>
    <automated>go build ./... && go test ./internal/cloudclaude/ -run 'Test_ExitCodes' -count=1 -v && grep -F 'ExitOAuthNotFound    = 6' internal/cloudclaude/exitcodes.go && grep -F 'ExitOAuthExpired     = 7' internal/cloudclaude/exitcodes.go && grep -F 'ExitMountForceFailed = 8' internal/cloudclaude/exitcodes.go && grep -F 'ExitConfigError   = 4' internal/cloudclaude/exitcodes.go && grep -F 'ExitInternalError = 5' internal/cloudclaude/exitcodes.go</automated>
  </verify>
  <done>
    exitcodes.go 暴露 9 个命名常量（ExitOK..ExitMountForceFailed）；与 v2.0 main.go exit* 0-5 完全对齐 + 新增 OAuth/mount 6-8；5 个单测覆盖唯一性 / POSIX 上限 / v2.0 兼容 / 不撞码 / 命名存在；Plan 03 的 OAuth 检查与 mount_strategy.go 的 ModeForce 失败路径必须改用这些常量（Plan 03 Task 3.2 同步更新）。
  </done>
</task>

</tasks>

<verification>
本 plan 完成后执行以下端到端检查：

```bash
# 1. 文件结构
test -f internal/cloudclaude/mount_sshfs.go
test -f internal/cloudclaude/mount_mutagen.go
test -f internal/cloudclaude/mount_merge.go
test -f internal/cloudclaude/mount_strategy.go
test -f internal/cloudclaude/askpass.go
test -f internal/cloudclaude/sshfs_watcher.go
test -f internal/cloudclaude/last_session.go
test -f internal/cloudclaude/colors.go

# 2. v2.0 兼容性（mount.go 共享 helper 保留 + ConnectAndRunClaude 签名不变）
grep -F 'func waitForMount(' internal/cloudclaude/mount.go
grep -F 'func sshRun(' internal/cloudclaude/mount.go
grep -F 'func shellQuote(' internal/cloudclaude/mount.go
grep -E 'func ConnectAndRunClaude\(cfg SSHConfig' internal/cloudclaude/ssh.go

# 3. 关键命令字符串（与 Phase 29 / RESEARCH §2.2 字符级一致）
grep -F 'category.create=ff,func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,inodecalc=path-hash' internal/cloudclaude/mount_merge.go
grep -F 'setfattr -n user.mergerfs.branches' internal/cloudclaude/mount_merge.go
grep -F 'reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10' internal/cloudclaude/mount_sshfs.go
grep -F '--mode=two-way-resolved' internal/cloudclaude/mount_mutagen.go
grep -F '--default-owner-beta=id:1000' internal/cloudclaude/mount_mutagen.go

# 4. 防御「禁止静默降级」M13
grep -F 'MOUNT_AUTO_DOWNGRADED' internal/cloudclaude/mount_strategy.go

# 5. 单元测试矩阵
go test ./internal/cloudclaude/ -run 'TestMountStrategy_DowngradeMatrix' -count=1 -v
go test ./internal/cloudclaude/ -run 'Test_SafetyGuard|Test_50MBReject|Test_VersionSkew|Test_BannerColors|Test_APFS|Test_Downgrade' -count=1 -v
go test ./internal/cloudclaude/ -run 'Test_AskpassHelper|Test_WriteLastSession|Test_SSHFSWatcher' -count=1 -v
go test ./internal/cloudclaude/errcodes/ -count=1
go test ./... -count=1   # 整仓回归

# 6. CLI flag 接线
go build -o /tmp/cc-test ./cmd/cloud-claude
/tmp/cc-test --mount-mode=invalid 2>&1 | grep -E '错误.*mount-mode'

# 7. 格式与 vet
gofmt -l internal/cloudclaude/ cmd/cloud-claude/
go vet ./...
```

**不在本 plan 验证范围**（属于 Plan 03）：
- OAuth credentials 检查（CheckOAuthCredentials 函数尚未存在；本 plan 在 ssh.go 留 TODO 注释）
- mutagen sync conflicts 子命令（cmd/cloud-claude/sync.go 由 Plan 03 创建）
- 集成测试（//go:build integration）由 Plan 03 落地
</verification>

<threat_model>
## Trust Boundaries

| Boundary | 描述 |
|----------|------|
| cloud-claude 进程 → conn-C ssh 子进程 | Mutagen fork 出独立 ssh 进程；密码经 SSH_ASKPASS helper 传递（不进 ps args / argv） |
| 本地 askpass helper 文件 → 任意本机进程 | helper 脚本 0700 + 仅在 cloud-claude 进程 lifetime 内存在；defer cleanup |
| 本地 → 远端 conn-A | mergerfs / setfattr / OAuth cat 命令；shellQuote 包装防注入 |
| remote stdin (mutagen-defaults.yml 路径) → 本地解析 | yaml 路径硬编码 ~/.cloud-claude/mutagen-defaults.yml，不接受用户输入 |
| ~/.cloud-claude/last-session.json → 本机其它进程 | 文件 0600（写入时通过 os.Rename atomic）；不含 password / token，仅 mode / 错误码 / account_id |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-31-02-01 | Information Disclosure | conn-C 通过 askpass 传 SSH 密码 | mitigate | helper 脚本 0700；密码经环境变量 CLOUD_CLAUDE_SSH_PASS 传递（不进 argv 不进 ps）；defer Cleanup 在父进程退出时立刻删除 helper 文件；askpass.go 内禁止把密码写入文件名 / 日志（acceptance 已断言） |
| T-31-02-02 | Information Disclosure | last-session.json 暴露账号 ID 与降级原因 | accept | 文件位于用户家目录子路径 0700；不含 password / token；仅运维数据（mode / 错误码 / 时间戳） |
| T-31-02-03 | Tampering | mergerfs 远程命令注入（恶意 cwd / branch path） | mitigate | branch / target 路径在 mount_merge.go 中硬编码（"/workspace-hot=RW"、"/workspace-cold=NC,RO"、"/workspace"），不接受用户传入；CLOUD_CLAUDE_MERGERFS_BRANCHES 环境变量本阶段读但默认 2 路（Phase 33 才允许扩展），且解析时按 ":" split + shellQuote 每项 |
| T-31-02-04 | Tampering | setfattr 命令注入 | mitigate | RemoveBranch 的 branchPath 与 target 都用 shellQuote 包装；调用方仅传 "/workspace-cold" / "/workspace" 两个静态字符串 |
| T-31-02-05 | Tampering | ssh-askpass helper 临时文件被攻击者篡改后注入恶意 shell | mitigate | helper 脚本路径用 os.CreateTemp 生成（不可预测后缀）；写入后立刻 chmod 0700；exec.Command(mutagen, ...) 调用前不再 Stat / 二次校验 — 攻击者必须有用户级写权限才能篡改，已是 game over 场景 |
| T-31-02-06 | Denial of Service | sshfs_watcher 运行时无限循环 | mitigate | watcher 只检测一次 disconnect 即 return（不重入）；ctx cancel 强制停止；mount_strategy 在 cleanup 时 cancel watcher |
| T-31-02-07 | Spoofing | Mutagen daemon 与用户已装 brew mutagen daemon 冲突 | mitigate | MUTAGEN_DATA_DIRECTORY=$HOME/.cloud-claude/mutagen 强制隔离 socket；ExtractMutagenBinary 写到 ~/.cloud-claude/bin/mutagen 不污染 PATH |
| T-31-02-08 | Repudiation | 静默降级到 sshfs-only 用户不知道 | mitigate | M13 强约束：stderr 必输出 errcodes.Format(MOUNT_AUTO_DOWNGRADED, ...) + last-session.json 持久化 downgrade_chain；测试 Test_DowngradeBannerEachStep 强制断言 |
| T-31-02-09 | Elevation of Privilege | 远程 sudo mergerfs 提权 | accept | 容器内 SYS_ADMIN 已由 v2.0 / Phase 29 开放；本 plan 不引入新 capability / 不要求宿主机额外权限 |

</threat_model>

<success_criteria>
- 4 个 mount_*.go 文件齐备 + 4 个辅助文件齐备 + ssh.go V3 入口 + main.go cobra flag 全部就位
- 12 降级矩阵单测 + 6 个 mount_mutagen 专项单测 + 3 个 askpass / last_session / sshfs_watcher 辅助单测全 PASS
- mount_merge.go / mount_mutagen.go / mount_sshfs.go 中关键命令字符串通过严格 grep 断言（与 Phase 29 D-11 + RESEARCH §1.3 / §2.1 / §2.2 字符级一致）
- v2.0 ConnectAndRunClaude 签名兼容保留；mountWorkspace 兼容 alias 保留
- M13 防御：每次降级 stderr 必输出 MOUNT_AUTO_DOWNGRADED + last-session.json 含 downgrade_chain 字段
- 整仓 `go test ./... -count=1` + `gofmt -l` + `go vet` 全部通过
- cloud-claude --mount-mode=invalid 输出明确中文错误且退出码 != 0
</success_criteria>

<output>
完成后创建 `.planning/phases/31-cli/plans/02-mount-three-layer/SUMMARY.md`，列明：
- 4 个 mount_*.go 文件的导出函数清单
- MountConfig / Mode 完整字段表（含 Phase 32 / 34 预留接口字段）
- 12 降级矩阵的最终决策表（cfg.Mode × failure injection → finalMode）
- 已落地但**未在本 plan 测试**的项：sshfs watcher 真实抖动场景、Mutagen sync 真实运行（留给 Plan 03 集成测试）
- 与 Plan 03 的接口契约：
  - ssh.go ConnectAndRunClaudeV3 在 mount ready 后、runClaude 之前留有 OAuth 检查 hook 点（TODO 注释定位）
  - mount_strategy.go banner 后输出 conflict count 警告时调用 mountMutagen 暴露的 conflict count（Plan 03 在 mount_mutagen.go 内部实现 sync list --template 解析）
  - cmd/cloud-claude/sync.go 子命令由 Plan 03 创建
</output>
