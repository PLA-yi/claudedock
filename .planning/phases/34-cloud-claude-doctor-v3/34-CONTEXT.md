# Phase 34: cloud-claude doctor v3 + 错误码统一 - Context

**Gathered:** 2026-04-21
**Status:** Ready for planning
**Mode:** `--auto`（所有 gray area 自动选定推荐项，决策来源标注于 `<decisions>` 各条尾注）

<domain>
## Phase Boundary

把 v3.0 在 Phase 31 / 32 / 33 各处落码完毕的错误路径**收口**到统一的 `<DOMAIN>_<KIND>_<NUM>` 错误码体系，并交付：

1. **`cloud-claude doctor` 五维度自检**（network / auth / ssh / mount / disk），每项输出 `[符号] 中文原因（建议: ... | 错误码: ...）` 四要素（REQ-F6-A / B、PITFALLS M14）
2. **`cloud-claude doctor --fix`** 至少 5 类自动修复（mutagen agent restart / FUSE 残留挂载清理 / known_hosts 冲突 / OAuth token 过期 refresh / DNS 缓存污染 flush），Q9 拍板默认全幂等 + 危险操作 stdin `y/N` + CI `--yes` 跳过（REQ-F6-C）
3. **`cloud-claude doctor --verbose|--json|NO_COLOR`** + 退出码 `0/1/2`（与 `brew doctor` 语义对齐，REQ-F6-D）
4. **`cloud-claude explain <code>`** 子命令（对标 `rustc --explain`），数据源 = Registry Entry 的 `Message`/`NextAction` + 同包追加的 `ExtendedExplanations map[Code]string`（REQ-F8-C）
5. **错误码注册表收口**：补全 `STATE_*` / `SYSTEM_*` / `SSH_*` / `AUTH_*` / `DISK_*` 五个新前缀（≥ 10 条新增），与 v2.0 `auth_*` / `entry_*` / `host_action_failed` 命名空间无冲突（PITFALLS C8）；Registry 单元测试遍历断言「无重复 + 每条非空中文 Message + 每条非空 NextAction + 命名匹配 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`」（REQ-F8-A / B）
6. **降级历史第一屏**：doctor 启动时**首屏**即从 `~/.cloud-claude/last-session.json` 读 `DowngradeChain` + `ActualMode` + `ConflictCount` + `ReconnectCount`，渲染历史 banner 后再跑 5 维度检查（PITFALLS M13 终验）
7. **mergerfs / Mutagen 版本 / AppArmor 三大已知坑**通过 mount 维度检查项落地：
   - mergerfs：`getfattr -n user.mergerfs.branches /workspace/.mergerfs` 必须命中 RW + NC,RO + `mount` 输出含 `func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,category.create=ff`（PITFALLS C2）
   - Mutagen：客户端 embed 版本 vs 容器内 `/etc/cloud-claude/mutagen.version` 比对（PITFALLS C4）
   - AppArmor：复用 Phase 29 `deploy/scripts/host-preflight.sh:check_apparmor_fusermount3` 检测逻辑（PITFALLS C6）

本阶段**不**交付：

- 任何新的 host-agent endpoint（doctor 完全本地 + SSH 实现，沿用 Phase 29 / 30 / 33 边界，对应 ARCHITECTURE §6 / SUMMARY §4.4 / OOS-A20）
- 性能基线真机验收（10k 文件 1.5× / 首连 ≤8s / 30s 抖动 / 镜像 ≤700MB）→ Phase 35
- doctor 自动上报「诊断报告」到服务端 → OOS-A11 永久禁
- 错误码 i18n（英文版本）→ 与 Phase 31 D-22 一致延续到 v3.1
- 「下次回车前 prompt 上方插入」严格 PTY 拦截渲染 → 沿用 Phase 31 D-28 决策
- 重写 Phase 31/32 既有 `internal/cloudclaude/ssh_doctor.go`（v2.0 quick task 260417-0w4 已 ship 的 SSH 密钥体检子命令）—— 本阶段把 `ssh_doctor` 作为 doctor `ssh` 维度的**一个检查项**集成进来，不动既有签名

</domain>

<decisions>
## Implementation Decisions

### 文件结构

- **D-01**：本阶段新增 `internal/cloudclaude/doctor/` 子包（与 `errcodes/` 同级），按 5 个维度 + 共享渲染拆分：
  - `doctor/doctor.go` — 顶层入口 `RunDoctor(ctx, opts) (*Report, error)`、`Options{Domain, Fix, Verbose, JSON, NoColor, Yes}` struct、`Report{StartedAt, Domains [], DowngradeHistory []DowngradeStep, Summary{Pass, Warn, Fail, Total, DurationMS}}` struct、`Status enum {StatusPass, StatusWarn, StatusFail, StatusSkip}`
  - `doctor/check.go` — `Check{Domain, Name, Status, Code, Message, NextAction, Details, FixApplied, FixFailed, DurationMS}` struct、`Checker interface{ Run(ctx) Check; Fix(ctx, opts) Check }`、`runWithTimeout(ctx, name, fn, timeout) Check` helper（每检查项 5s 默认 timeout，Verbose 模式 30s）
  - `doctor/network.go` — DNS 解析 / 出口 IP 探测 / 网关连通（本地 `net.Resolver` + Entry API health 探测）
  - `doctor/auth.go` — 本地 config 完整性 / Entry API token 有效性（`AuthenticateAndWait` dry-run 不真启动 SSH）/ OAuth credentials 过期（远端读 `/home/claude/.claude/.credentials.json` 复用 Phase 31 `oauth_check.go`）
  - `doctor/ssh.go` — SSH 客户端 KeepAlive 配置一致性（`MountConfig.KeepAliveInterval >= 15s` 校验）/ 远端 `sshd -T | grep clientalive` 校验 / 远端 `/workspace/.ssh` 复用 `RunSSHDoctor` 作为子检查项（不重复实现）
  - `doctor/mount.go` — Mutagen 客户端 vs agent 版本比对（C4）/ mergerfs branches xattr + mount 参数断言（C2）/ sshfs mountpoint 健康 / FUSE 残留挂载扫描（`mountpoint -q` + `fusermount -u` 候选清单）
  - `doctor/disk.go` — 本地 `~/.cloud-claude/` 可用空间 / 远端 `/workspace` `/var/lib/claude-persist` 容器内 disk usage / mutagen daemon data 目录大小（无限增长警告）
  - `doctor/fix.go` — `FixerRegistry map[errcodes.Code]Fixer` 注册各错误码的修复函数；危险操作走 `confirmDestructive(ctx, opts, prompt) bool`（stdin `y/N`，`--yes` 跳过）
  - `doctor/render.go` — 终端 + JSON 双输出，沿用 `colors.go` 的 ansi helper；Banner 渲染降级历史（M13 第一屏）+ 维度分组 + 汇总
  - `doctor/render_test.go` / `doctor/check_test.go` / `doctor/mount_test.go` 等，按文件配单测
- **D-02**：`internal/cloudclaude/errcodes/` 包追加 5 个新文件 + 1 个 explanations 文件：
  - `errcodes/state.go` — `STATE_*`（持久化 / volume / 容器状态相关，例如 `STATE_VOLUME_IN_USE_001`、`STATE_CONTAINER_NOT_RUNNING`、`STATE_LAST_SESSION_MISSING`）
  - `errcodes/system.go` — `SYSTEM_*`（宿主机 / 容器 OS 层级，例如 `SYSTEM_APPARMOR_FUSERMOUNT3_MISSING`、`SYSTEM_FUSE_RESIDUAL_MOUNT`、`SYSTEM_DNS_RESOLVE_FAILED`）
  - `errcodes/ssh.go` — `SSH_*`（SSH 通道 / KeepAlive / 已知 hosts 冲突，例如 `SSH_KNOWN_HOSTS_CONFLICT`、`SSH_SSHD_KEEPALIVE_DRIFT`）
  - `errcodes/auth.go` — `AUTH_*`（CLI 配置 / Entry token / OAuth refresh，例如 `AUTH_CONFIG_MISSING`、`AUTH_TOKEN_EXPIRED`、`AUTH_OAUTH_REFRESH_FAILED`）
  - `errcodes/disk.go` — `DISK_*`（本地 + 容器 disk usage，例如 `DISK_LOCAL_LOW`、`DISK_CONTAINER_LOW`、`DISK_MUTAGEN_DATA_BLOAT`）
  - `errcodes/explanations.go` — 包级 `ExtendedExplanations map[Code]string`，每条对应一段 ≥ 200 中文字符的 `cloud-claude explain` 长说明（含触发场景 / 根本原因 / 复现方式 / 修复路径），由 `init()` 注册（与 `MustRegister` 同样防御重复）
  - `errcodes/codes_test.go`（既有）扩展：遍历 `Registry()` 断言「(a) 命名匹配 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`（既有）；(b) 全部 Code 在 `ExtendedExplanations` 中有 entry 或在新增 `ExplainExempt set` 中显式豁免（informational 类如 `MOUNT_APFS_CASE_INSENSITIVE` / `SESSION_TAKEOVER_NOTIFIED` / `MOUNT_AUTO_DOWNGRADED` / `NET_RECONNECT_BACKOFF` 可豁免）；(c) v2.0 lower-case 现网码（`auth_*` / `entry_*` / `host_action_failed`）必须不出现在 Registry（防御 C8）」
- **D-03**：`cmd/cloud-claude/main.go` 新增两个 cobra 子命令：
  - `doctor [domain]`（domain ∈ `network|auth|ssh|mount|disk|all`，默认 `all`），flag：`--fix` / `--verbose` / `--json` / `--yes`；尊重环境变量 `NO_COLOR`
  - `explain <code>`，args 必填（CODE 大小写敏感即可，与 Registry 保持一致），输出 = `Format(code)` 两段 + 空行 + ExtendedExplanations[code]（缺失时 fallback 到「未提供详细说明，运行 cloud-claude doctor <domain> 查看相关检查项」）
  - 在 `runRoot` 旁边的 `case` 列表（DisableFlagParsing 关闭判定）追加 `"doctor", "explain"` 两个 keyword
- **D-04**：保持 `cloud-claude ssh doctor` 子命令字面入口不变（v2.0 quick task 260417-0w4 已发布，不破坏用户肌肉记忆），但 `cloud-claude doctor ssh` 内部调用同一个 `RunSSHDoctor`（共享底层 + 双入口共存，便于过渡）；后续 v3.1 可考虑 deprecate 旧入口

### `cloud-claude doctor` 子命令结构（GA-2）

- **D-05**：cobra 结构选定 **`doctor [domain]` 子命令 + 顶层 flag**，**不**采用 `doctor --domain=mount` flag 风格：
  1. 用户更习惯 `brew doctor` / `cargo check` 风格，无 flag 跑全量
  2. cobra `Args: cobra.MaximumNArgs(1) + 自定义 ValidArgs={"network","auth","ssh","mount","disk","all"}` 内置校验
  3. 历史 `cloud-claude ssh doctor` 子命令 = `cloud-claude doctor ssh`（D-04 双入口）正交不撞
  4. `--fix`/`--verbose`/`--json`/`--yes` 都是顶层 flag（不是 `doctor` 子命令独占），便于 v3.1 给 `cloud-claude env check --fix` 等其它子命令复用
- **D-06**：`doctor` 子命令**不需要先 init**也能跑 `network` / `auth`（仅本地 config / DNS）维度；`mount`/`ssh`/`disk` 需要 init + Entry API auth 才能 SSH 到容器；缺 init 时把对应维度标记 `StatusSkip` + 中文原因「未配置网关，跳过；运行 cloud-claude init 配置后重试」（不算 fail）
- **D-07**：默认 `domain=all`，串行执行 `network → auth → ssh → mount → disk`（依赖关系：disk 需 mount 拿到 mountpoint；ssh 需 auth；mount 需 ssh）；`--verbose` 时每个 check 打印开始 / 结束时间戳；并行执行（per-domain goroutine）推迟 v3.1 backlog（doctor 是低频运维场景，串行一致性优先）
- **D-08**：单个 check timeout 默认 5s，可被 `Options.CheckTimeout` 覆盖；`--verbose` 模式自动放宽到 30s；timeout 命中时检查项标记 `StatusFail` + 中文原因「检查超时（>%s）」 + 错误码 `SYSTEM_CHECK_TIMEOUT`

  *[auto] GA-2 推荐项：subcommand 风格（替代 flag 风格：(a) 命令行行业惯例；(b) 与 cobra ValidArgs 内置校验天然契合；(c) `cloud-claude doctor ssh` 自然兼容现有 `ssh doctor` 入口的双向迁移）*

### `--fix` 自动修复幂等性（Q9 拍板 + GA-3）

- **D-09**：Q9 答案拍板为 **「默认全幂等 + 危险操作 stdin `y/N` + CI `--yes` 跳过」**；具体 5 类（REQ-F6-C 字面要求至少 5 类）+ 幂等性分类：
  | 错误码 | 修复动作 | 幂等性 | 危险性 | 二次确认 |
  |--------|----------|--------|--------|----------|
  | `MOUNT_MUTAGEN_DAEMON_UNAVAILABLE` | `mutagen daemon stop && mutagen daemon start` | ✓ | 低（数据无关） | 否 |
  | `SYSTEM_FUSE_RESIDUAL_MOUNT` | `fusermount -u <path>` 列出后逐个解挂 | ✓（已挂的 noop） | 中（挂着的目录无活跃 fd 时） | **是**（列出每个 mountpoint，y/N 确认整批） |
  | `SSH_KNOWN_HOSTS_CONFLICT` | `ssh-keygen -R <host>:<port>` | ✓ | 低（仅清缓存条目） | 否 |
  | `AUTH_TOKEN_EXPIRED` / `AUTH_OAUTH_REFRESH_FAILED` | 重新调 `EntryClient.AuthenticateAndWait` 刷新 token；OAuth 过期则提示用户在容器内 `claude login`（CLI 不自动启动登录） | ✓ | 低 | 否（仅刷 Entry token，OAuth 必须用户介入） |
  | `SYSTEM_DNS_RESOLVE_FAILED` | macOS `dscacheutil -flushcache && killall -HUP mDNSResponder`；Linux `systemd-resolve --flush-caches` 或 `resolvectl flush-caches` | ✓ | 低 | **是**（涉及系统级 daemon HUP，stdin `y/N`） |

  以上 5 类是**必交付下限**；planner 可在 plan-phase 添加更多（建议候选：`MOUNT_SSHFS_DISCONNECTED` 重新挂 sshfs / `STATE_LAST_SESSION_MISSING` 写一个空 snapshot / `SSH_SSHD_KEEPALIVE_DRIFT` 自动重连）。
- **D-10**：`confirmDestructive(ctx, opts, promptZH) bool` helper：
  1. `opts.Yes` 为 true → 直接返回 true（CI 友好）
  2. `opts.JSON` 为 true → 直接返回 false + 在 `Check.FixFailed` 追加「JSON 模式禁止交互式修复，请在终端模式重试或追加 --yes」
  3. 否则交互式 stdin 提示 `提示中文(y/N) > `（与 Mutagen prompt 风格一致），读到 `y` / `Y` / `yes` 才返回 true，其它一律 false
- **D-11**：`--fix` 失败时**不**回滚已成功的修复动作（避免引入分布式事务 / 半状态）；每个 check 独立记录 `FixApplied []string` / `FixFailed []string`，最终 summary 同时计 `pass + (warn|fail with FixApplied count)`
- **D-12**：`doctor --fix` 在 `JSON` 模式下输出标准 schema（D-15）；非 JSON 模式在每个 check 后立刻输出 `       ✓ 已修复: ...` / `       ✗ 修复失败: ...`（沿用 ssh_doctor `Print()` 风格）

  *[auto] Q9 / GA-3 推荐项：默认全幂等（替代「全部需二次确认」：增加交互负担；替代「全部静默自动」：违反 PROJECT.md「优雅好用运维清晰」；FUSE 解挂 + DNS flush 两类系统级动作显式 y/N 是最小必要的安全门）*

### 第一屏布局 + 降级历史（M13 终验 + GA-4）

- **D-13**：doctor 启动 banner 输出顺序（M13 验收锚点）：
  1. **第一屏**：`Cloud Claude Doctor` 标题 + 当前 cloud-claude 版本 + 远端 image_version（如 init + auth 已成功）
  2. **第二段**：上次会话快照（来源 `~/.cloud-claude/last-session.json`）：
     - 时间戳（中文相对时间，例：「上次连接 5 分钟前」）
     - 实际 mode + 意图 mode（不一致时高亮）
     - **降级历史**（如非空）：每行 `[降级] <from> → <to> | 原因 [<reason_code>] <reason_message>`（M13 必须出现：用户「以为在 full 模式下跑」就会被这一段反驳）
     - Mutagen conflict 计数（>0 时彩色警告）
     - reconnect 计数（>0 时 informational）
     - last-session.json 缺失时 → `[!] 未找到上次会话快照（首次运行 cloud-claude 后再 doctor 即可看到）` + 错误码 `STATE_LAST_SESSION_MISSING` 但不算 fail
  3. **第三段**：5 维度检查矩阵（每个 check 一行 `[符号] <name> <message>（建议: ... | 错误码: ...）`）
  4. **末尾**：汇总 `共 N 项检查：M pass / W warn / F fail（耗时 X.Xs）`

  渲染由 `doctor/render.go` 的 `RenderText(*Report) string` / `RenderJSON(*Report) []byte` 统一处理。
- **D-14**：彩色规则沿用 Phase 31 D-17 `colors.go` 与 `NO_COLOR` 兼容策略：
  - `[✓]` 绿色 32 / `[!]` 黄色 33 / `[✗]` 红色 31 / `[~]`（Skip）灰色 90
  - `NO_COLOR=1` 或非 TTY → 全部退回纯字符 `[ok]/[warn]/[fail]/[skip]`
  - `--json` 模式天然不染色

### `--json` Schema（GA-6）

- **D-15**：`--json` 输出固定 schema：
  ```json
  {
    "schema_version": 1,
    "started_at": "2026-04-21T15:30:00Z",
    "duration_ms": 1234,
    "cloud_claude_version": "v3.0.0-rc1",
    "remote_image_version": "v3.0.0",
    "downgrade_history": {
      "snapshot_age_seconds": 300,
      "intended_mode": "full",
      "actual_mode": "sshfs-only",
      "downgrade_chain": [
        {"from": "full", "to": "mutagen-only", "reason_code": "MOUNT_MUTAGEN_VERSION_SKEW", "reason_message": "..."},
        {"from": "mutagen-only", "to": "sshfs-only", "reason_code": "MOUNT_AUTO_DOWNGRADED", "reason_message": "..."}
      ],
      "conflict_count": 0,
      "reconnect_count": 2,
      "tmux_session": "claude-abc12345",
      "client_role": "primary"
    },
    "summary": {"total": 17, "pass": 14, "warn": 2, "fail": 1, "skip": 0},
    "checks": [
      {
        "domain": "mount",
        "name": "mergerfs_branches",
        "status": "fail",
        "code": "MOUNT_MERGERFS_FAILED",
        "message": "...",
        "next_action": "...",
        "details": {"branches_xattr": "<空>", "expected": "RW + NC,RO"},
        "fix_applied": [],
        "fix_failed": [],
        "duration_ms": 87
      }
    ]
  }
  ```
  - schema_version=1 锁死，新字段全部 `omitempty` 追加，不破坏 jq 脚本
  - `details` 是开放字段（map[string]any），各 check 可放任意调试信息（mount 维度放 mergerfs branches xattr / Mutagen agent 版本字符串等）；`--verbose` 模式下文本渲染才显示，JSON 永远输出（脚本消费）
- **D-16**：退出码与 `brew doctor` 对齐：`0`（全 pass + 全 skip）、`1`（≥1 warn 但 0 fail）、`2`（≥1 fail）；`--fix` 模式下，**修复成功的 fail 不降级为 0**，仍按原始 status 计数（用户需要看到「本次有 fail 但已修」），但 stdout 顶部追加一行 `[fix] N 项已修复 / M 项修复失败`

### `cloud-claude explain <code>` 数据源（GA-5）

- **D-17**：`explain` 子命令流程：
  1. 大小写敏感匹配 args[0] 到 Registry；未注册 → stderr `未找到错误码 %s；运行 cloud-claude doctor 查看可用检查项` + exit 4（与 ConfigError 一致）
  2. 输出三段：
     - 段 1：`Format(code)` 两段（错误码 + 中文原因 + 建议），与 doctor / 其它路径完全一致的字面量
     - 段 2：空行后 `详细说明：` + `ExtendedExplanations[code]` 或 fallback 到「未提供详细说明，运行 cloud-claude doctor <domain> 查看相关检查项」
     - 段 3（Verbose 时追加，`--verbose` flag）：触发场景 / 根本原因 / 复现命令（如有）/ 相关 doctor check 名（来源：`internal/cloudclaude/doctor/explain_index.go` 的 `RelatedChecks map[Code][]string`，非必填字段）
  3. 退出码：找到 = 0；未找到 = 4
- **D-18**：`ExtendedExplanations` 单条建议 200~500 中文字符；模板：
  ```
  触发场景：<什么操作或环境组合会触发，最多 2 句>
  根本原因：<技术层面的解释，最多 3 句>
  复现方式（可选）：<最小 shell 命令组合，便于运维复现>
  修复路径：<完整修复步骤，与 doctor --fix 自动修复重叠时显式标注「也可运行 cloud-claude doctor mount --fix 自动处理」>
  关联文档（可选）：<.planning/research/PITFALLS.md C2 / .planning/phases/29-v3-worker/29-CONTEXT.md §D-11 等绝对相对路径>
  ```
  本阶段 Plan 01 必须为所有**非 informational** Code 写一条 ExtendedExplanations（≥ 18 条，informational 类如 `*_BACKOFF` / `*_NOTIFIED` / `MOUNT_APFS_CASE_INSENSITIVE` 显式豁免登记到 `ExplainExempt set`）

  *[auto] GA-5 推荐项：Registry + ExtendedExplanations map（替代 markdown embed：(a) 不引入新文件类型 / 资源管理；(b) `go:embed` 单一 .md 切换需要 markdown parser 才能格式化输出，复杂度反而上升；(c) Go map 构造一致与 Registry 同步演进，CI 测试覆盖天然）*

### 远端 vs 本地拆分（GA-7）

- **D-19**：5 维度检查项分布（**总计 ≥ 17 项**，REQ-F6-A 「五维度」字面达标）：

  | 维度 | check name | 远端/本地 | 错误码（命中 fail/warn 时） |
  |------|-----------|-----------|------------------------------|
  | network | `dns_resolve` | 本地 | `SYSTEM_DNS_RESOLVE_FAILED` |
  | network | `gateway_reachable` | 本地（HTTP） | `AUTH_GATEWAY_UNREACHABLE` |
  | network | `egress_ip_visible` | 远端（`curl ifconfig.io`） | `NET_EGRESS_IP_DRIFT` |
  | auth | `config_present` | 本地 | `AUTH_CONFIG_MISSING` |
  | auth | `entry_token_valid` | 本地 + Entry API | `AUTH_TOKEN_EXPIRED` |
  | auth | `oauth_credentials` | 远端（复用 Phase 31 `oauth_check.go`） | `NET_OAUTH_EXPIRED` / `NET_OAUTH_NOT_FOUND` |
  | ssh | `keepalive_config` | 本地（`MountConfig.KeepAliveInterval` ≥ 15s） | `SESSION_KEEPALIVE_TOO_AGGRESSIVE` |
  | ssh | `sshd_keepalive_drift` | 远端（`sshd -T \| grep clientalive`） | `SSH_SSHD_KEEPALIVE_DRIFT` |
  | ssh | `known_hosts` | 本地（`~/.ssh/known_hosts` 解析 + 与 Entry API host 比对） | `SSH_KNOWN_HOSTS_CONFLICT` |
  | ssh | `workspace_ssh_keys` | 远端（复用 `RunSSHDoctor`） | 沿用 ssh_doctor 既有错误（无错误码，本阶段不强迁移；记 details 即可） |
  | mount | `mutagen_version_match` | 远端（`/etc/cloud-claude/mutagen.version` 比对） | `MOUNT_MUTAGEN_VERSION_SKEW` |
  | mount | `mergerfs_branches` | 远端（`getfattr -n user.mergerfs.branches` + `mount` 输出参数断言） | `MOUNT_MERGERFS_FAILED` |
  | mount | `sshfs_mountpoint` | 远端（`mountpoint -q /workspace-cold`） | `MOUNT_SSHFS_DISCONNECTED` |
  | mount | `fuse_residual` | 本地（macOS `mount \| grep macfuse` / Linux `mount \| grep fuse.sshfs` 扫旧 mount） | `SYSTEM_FUSE_RESIDUAL_MOUNT` |
  | mount | `apparmor_fusermount3` | 本地（复用 `deploy/scripts/host-preflight.sh:check_apparmor_fusermount3` 逻辑，go 改写） | `SYSTEM_APPARMOR_FUSERMOUNT3_MISSING` |
  | disk | `local_disk` | 本地（`statfs(~/.cloud-claude/)`，< 500MB warn / < 100MB fail） | `DISK_LOCAL_LOW` |
  | disk | `container_disk` | 远端（`df -BM /workspace`，< 500MB warn / < 100MB fail） | `DISK_CONTAINER_LOW` |
  | disk | `mutagen_data_size` | 本地（`du -sh ~/.cloud-claude/mutagen/`，> 1GB warn） | `DISK_MUTAGEN_DATA_BLOAT` |

- **D-20**：远端 SSH 检查统一通过 `internal/cloudclaude/doctor/remote_runner.go` 的 `RemoteRunner{conn *ssh.Client}.RunScript(name, script string) (stdout, stderr, err)` 包装（沿用 `ssh_doctor.go` 的 `runSSHSession` 模式）；conn 由 `RunDoctor` 在第一次需要远端的 check 之前 lazy 建立，跑完后 `defer conn.Close()`；如远端连接失败，所有标记 `RequiresRemote=true` 的 check 标记 `StatusSkip` + 中文原因「未能连接远端容器（原因: ...）」；不让一次远端断开把整轮 doctor 拖死

  *[auto] GA-7 推荐项：本地 + 远端混合（替代「全部走 SSH」：失去 doctor 在容器宕机时的诊断价值；替代「全部本地 mock 远端」：mergerfs xattr / Mutagen agent 版本必须容器内验，无法绕过）*

### 错误码命名空间补全（C8 + REQ-F8-A / B + GA-1）

- **D-21**：本阶段必须新增的错误码（≥ 11 条，与既有 25 条合计 ≥ 36 条；命名匹配 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`，已 Phase 31 D-02 放宽允许 4 段）：

  | Code | Severity | 触发场景 | 防御 / REQ |
  |------|----------|----------|-----------|
  | `STATE_LAST_SESSION_MISSING` | Info | doctor 第一屏读 `~/.cloud-claude/last-session.json` 不存在 | M13 / D-13 |
  | `STATE_VOLUME_IN_USE_001` | Error | admin DELETE claude_account 时 docker volume 仍被持有（沿用 Phase 33 D-19 已硬编码的字面量，本 phase 仅注册到 Registry） | REQ-F7-D |
  | `STATE_CONTAINER_NOT_RUNNING` | Warn | doctor 远端连不上容器、Entry API 显示 host 状态非 running | M13 |
  | `SYSTEM_APPARMOR_FUSERMOUNT3_MISSING` | Error | Ubuntu 25.04+ 缺 `/etc/apparmor.d/local/fusermount3` `capability dac_override,` 行 | C6 |
  | `SYSTEM_FUSE_RESIDUAL_MOUNT` | Warn | 本地 `mount` 输出仍含旧的 sshfs / macfuse 挂载（cloud-claude 异常退出留下） | C3 / REQ-F6-C |
  | `SYSTEM_DNS_RESOLVE_FAILED` | Error | `net.LookupHost(gateway)` 失败 | REQ-F6-C |
  | `SYSTEM_CHECK_TIMEOUT` | Warn | doctor 单 check 超时（D-08） | — |
  | `SSH_KNOWN_HOSTS_CONFLICT` | Warn | `~/.ssh/known_hosts` 含 Entry API 解析出的 host 但 fingerprint 与本次握手不一致 | REQ-F6-C / M14 |
  | `SSH_SSHD_KEEPALIVE_DRIFT` | Warn | 远端 `sshd -T` 输出 `clientaliveinterval`/`clientalivecountmax` 与 Phase 29 D-14 基线（15/8）不一致 | REQ-F3-A 服务端基线漂移 |
  | `AUTH_CONFIG_MISSING` | Fatal | `~/.cloud-claude/config.yaml` 不存在 / 解析失败 | REQ-F6-A 前置 |
  | `AUTH_GATEWAY_UNREACHABLE` | Error | gateway HTTP 探测 5xx / connection refused | REQ-F6-A |
  | `AUTH_TOKEN_EXPIRED` | Warn | Entry API 401 / 403 | REQ-F6-C `--fix` 自动 refresh |
  | `AUTH_OAUTH_REFRESH_FAILED` | Error | `--fix` 触发后 EntryClient.AuthenticateAndWait 仍失败 | REQ-F6-C |
  | `NET_EGRESS_IP_DRIFT` | Warn | 远端 `curl ifconfig.io` 与 Entry API 期望出口 IP 不一致 | REQ-F6-A |
  | `DISK_LOCAL_LOW` | Warn / Error | `statfs(~/.cloud-claude/)` 可用 < 500MB / 100MB | REQ-F6-A |
  | `DISK_CONTAINER_LOW` | Warn / Error | 远端 `df /workspace` 可用 < 500MB / 100MB | REQ-F6-A |
  | `DISK_MUTAGEN_DATA_BLOAT` | Warn | `du -sh ~/.cloud-claude/mutagen/` > 1GB | M16 边缘 |

  17 条新 + 25 条既有 = 42 条，超过 ROADMAP 暗示的 ≥ 30 条下限。
- **D-22**：v2.0 现网下划线小写错误码（`auth_*` / `entry_*` / `host_action_failed`）保持原状不迁移到 Registry（避免破坏运维已经熟悉的字面量）；CI 测试在 Registry 中**显式断言不出现**这些 lower-case 字符串（防止意外注册）。Phase 34 后续如要统一，独立开 v3.1 phase 做迁移 + 客户端兼容映射，本阶段不动。
- **D-23**：错误码命名最终模式：`<DOMAIN>_<KIND>_<NAME[_QUALIFIER]>`，全大写下划线分段；DOMAIN ∈ `MOUNT|SESSION|NET|STATE|SYSTEM|SSH|AUTH|DISK`（共 8 个，一级闭合），新增 DOMAIN 必须经过 plan-checker round + 在 PLAN.md 显式登记。

  *[auto] GA-1 推荐项：8 域闭合（替代「全合并到 SYSTEM_*」：粒度太粗 grep 不便；替代「按 phase 分前缀如 P34_*」：与 phase 数字耦合，未来重命名 / 删除 phase 时破坏字面量）*

### 测试矩阵（GA-8）

- **D-24**：Plan 02 / 03 的测试策略：
  1. **纯单测**：`doctor/check_test.go` 用 `RemoteRunner` interface mock，预设 stdout/stderr/err 三元组覆盖各检查项的 pass/warn/fail/skip 4 路径；`doctor/render_test.go` 输入构造好的 `*Report` 断言 stdout 字符串 + JSON 字段命中
  2. **修复幂等单测**：`doctor/fix_test.go` 注入 `var execMutagenDaemon = realExecMutagenDaemon` / `var execFusermountUnmount = ...` 等包级 var（沿用 Phase 29.1 Plan 02 / Phase 33 Plan 01 的可测试模式），断言 `--fix` 二次执行 noop / `--yes` 跳过交互 / 拒绝 stdin 时 `FixFailed` 含中文原因
  3. **远端集成测试**（Plan 03）：沿用 Phase 31 `integration_test.go` 风格，docker compose fixture（已有 `internal/cloudclaude/test-fixture-up.sh`）跑 1 个 happy path（`doctor mount` 在 v3 镜像上 5 项全 pass）+ 1 个 fail 注入（`docker exec` 内 `pkill mutagen-agent` 后 doctor mount 必命中 `MOUNT_MUTAGEN_VERSION_SKEW` 或 `MOUNT_MUTAGEN_DAEMON_UNAVAILABLE` 之一）；docker daemon 不可用时 `t.Skip` 留 Phase 35 真机
  4. **explain 测试**：遍历 `errcodes.Registry()`，断言每个 Code 在 `ExtendedExplanations` 或 `ExplainExempt` 中至少出现一次；`cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW` 子进程级测试（用 `os/exec` 跑 build 出来的 cloud-claude 二进制）放 `cmd/cloud-claude/explain_test.go`
  5. **CI grep 断言**（M14 终验）：`go run ./cmd/cloud-claude doctor --json` 输出 jq 解析所有 status=warn|fail 的 check 必须含 `next_action` 字段；非 JSON 模式 `grep -L "建议:"` 结果必须为空（doctor 二进制 sample 输出在 CI artifact 里）

  *[auto] GA-8 推荐项：mock-first + 单 fixture happy path（替代「全 docker fixture」：CI 时间膨胀 + 与 Phase 31/32/33 已建立的 t.Skip 模式不一致；替代「纯 mock 不跑 docker」：mergerfs xattr / mount 参数无法靠 mock 信任）*

### 与 Phase 29 / 31 / 32 / 33 的接口

- **D-25**：从 Phase 31 D-32 / Phase 32 D-27 预留接口直接复用：
  - `errcodes/codes.go` `Registry()` / `Lookup(Code)` / `Format(Code, args...)` 已就位
  - `~/.cloud-claude/last-session.json` schema 已含 `DowngradeChain` / `ConflictCount` / `TmuxSession` / `ClientRole` / `ReconnectCount`，**本阶段读取，不扩 schema**（如需 doctor-only 字段，开 v3.1 phase 升 schema_version=2）
  - `internal/cloudclaude/oauth_check.go` 在 Phase 31 已实现远端 OAuth 三态判定（NotFound / Expired / ExpiringSoon），doctor `auth.oauth_credentials` check 直接复用
- **D-26**：从 Phase 29 落地的容器侧基线（doctor 远端验收对象）：
  - `/etc/cloud-claude/mutagen.version`（D-05）→ doctor `mount.mutagen_version_match`
  - `mergerfs` 挂载参数 + branches xattr（D-11）→ doctor `mount.mergerfs_branches`
  - `sshd_config` ClientAliveInterval=15 / ClientAliveCountMax=8（D-14）→ doctor `ssh.sshd_keepalive_drift`
  - `deploy/scripts/host-preflight.sh:check_apparmor_fusermount3`（D-23）→ doctor 本地 Go 改写（不调用 shell 脚本，便于跨 OS 单测）
- **D-27**：从 Phase 33 admin DELETE 路径已硬编码 `STATE_VOLUME_IN_USE_001`（line 11989dd 的 admin_claude_accounts.go），本阶段把它**注册到 Registry**（D-21）+ 写一条 ExtendedExplanations + 加到 `errcodes/state.go`；admin 路径的字面量不变（兼容已部署 frontend）
- **D-28**：本阶段**不**让 doctor 直接调用 host-agent endpoint（与 Phase 29 D-22 / Phase 30 D-04 / Phase 33 D-16 一致），所有远端检查走 SSH conn 在容器内 exec；Entry API 调用复用 `EntryClient.AuthenticateAndWait` 入口，不扩接口

### Plan 拆分

- **D-29**：本阶段拆分为 **3 plans**（与 ROADMAP 字面 `0/3 plans` 一致）：
  - **Plan 01 — 错误码注册表收口 + `cloud-claude explain`**（Wave 1，无依赖）
    - `errcodes/state.go` / `system.go` / `ssh.go` / `auth.go` / `disk.go` 五新文件 + ≥ 17 条新 Code
    - `errcodes/explanations.go` 新建：`ExtendedExplanations map[Code]string` + `ExplainExempt set` + init 注册（同 MustRegister 防御重复）
    - `errcodes/codes_test.go` 扩展：所有 Code 在 ExtendedExplanations / ExplainExempt 二选一覆盖；v2.0 lower-case 显式 not-in-Registry 断言
    - `cmd/cloud-claude/explain.go` 新建：cobra 子命令 + Args 校验 + 输出三段
    - `cmd/cloud-claude/main.go`：`AddCommand(newExplainCmd())` + DisableFlagParsing case 追加 `"explain"`
    - 单元测试：`cmd/cloud-claude/explain_test.go`（os/exec 跑二进制）+ `errcodes/explanations_test.go`（map 完备性）
  - **Plan 02 — `cloud-claude doctor` 5 维度框架**（Wave 2，depends_on Plan 01）
    - `internal/cloudclaude/doctor/` 新包：`doctor.go` / `check.go` / `network.go` / `auth.go` / `ssh.go` / `mount.go` / `disk.go` / `remote_runner.go` / `render.go`（不含 `fix.go`，Plan 03 落）
    - 17 项 check 的 pass/warn/fail/skip 4 路径全实现
    - 第一屏降级历史渲染（D-13）+ JSON schema（D-15）+ 退出码（D-16）
    - `cmd/cloud-claude/doctor.go` 新建：cobra 子命令 + ValidArgs + `--verbose|--json|--yes`（`--fix` 此 plan 仅注册 flag，转发到 Plan 03 的 fixer registry，但 fixer 为空时只输出 `[!] --fix 自动修复将在 doctor.fix.go 实现（Plan 03）`）
    - `cmd/cloud-claude/main.go`：`AddCommand(newDoctorCmd())` + DisableFlagParsing case 追加 `"doctor"`
    - 单元测试：每维度独立 `*_test.go`，远端走 RemoteRunner mock；`render_test.go` 覆盖文本 + JSON 双输出
  - **Plan 03 — `doctor --fix` 5 类自动修复 + 集成测试**（Wave 3，depends_on Plan 02）
    - `doctor/fix.go` 新建：`FixerRegistry` + 5 类修复函数（D-09 表）+ `confirmDestructive` helper
    - 各维度文件的 `Checker.Fix(ctx, opts)` 实现
    - `integration_test.go` 沿用 Phase 31 模式：1 个 docker fixture happy path（`doctor mount` 5 项 pass）+ 1 个 fail 注入（kill mutagen-agent）；`-short` 模式 t.Skip
    - CI grep 断言（M14 终验）脚本：`scripts/ci-doctor-grep.sh` 跑一次 `cloud-claude doctor --json` + jq 校验所有 warn/fail 含 `next_action`，非 JSON 输出 `grep -L "建议:"` 结果为空
- **D-30**：Plan 间依赖严格 Wave 1 → Wave 2 → Wave 3 串行；Plan 02 的 17 项 check 在 Plan 03 落 fix 前必须**先全部输出 status + code**（即使 `--fix` flag 给了也只是注册进 FixerRegistry 占位返回 nil），避免 Plan 03 阻塞 Plan 02 的 ship

### Claude's Discretion

以下细节由 planner / executor 按实现便利性决定：

- `RemoteRunner` 是用 interface 还是 struct + 包级 var（建议 interface，单测注入更干净）
- `confirmDestructive` 的 stdin 读取是否兼容非 TTY（建议非 TTY 时直接拒绝并写 FixFailed，与 D-10 第 2 条一致）
- `mount` 维度 mergerfs xattr 解析的容错（getfattr 输出格式跨发行版细节差异，建议 substring 匹配 `RW` 与 `NC,RO` 而非严格列分割）
- `--verbose` 模式的输出冗余度（建议每 check 显示开始/结束时间戳 + Details 全字段；不要把整个 ssh script stdout 都打印）
- `disk` 维度 1GB / 500MB / 100MB 三档阈值是否走 config 文件（建议本阶段硬编码，v3.1 再 config 化）
- `cloud-claude doctor --json` 是否兼容 `jq -c` 的紧凑输出（建议默认 indent 2，由 jq 自己处理）
- `AUTH_OAUTH_REFRESH_FAILED` 的修复路径是否打印"请在容器内 `claude login`"（建议是，与 Phase 31 `NET_OAUTH_NOT_FOUND` next_action 一致）
- `SSH_KNOWN_HOSTS_CONFLICT` 的 `ssh-keygen -R` 是否支持指定 known_hosts 文件路径（建议读 `~/.ssh/known_hosts` 默认即可，与 Phase 33 `cloud-claude ssh doctor` 一致）
- explain 是否支持模糊匹配（如输入 `MOUNT_MUTAGEN` 列出全部 6 条 MOUNT_MUTAGEN_*）（建议本阶段不做，留 v3.1）
- `cloud-claude doctor` 不带 init 时是否自动跳过远端 check（建议是，D-06 行为）

### Folded Todos

无（`todo match-phase 34` 返回 0 条匹配）。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 规划与需求

- `.planning/PROJECT.md` — v3.0 总目标 / Constraints（沟通中文 / 部署单宿主机 / 零增量特权）/ Key Decisions
- `.planning/REQUIREMENTS.md` §C1（F6 doctor 五维度）/ §C3（F8 错误码统一） — 本阶段交付的 7 条 REQ：REQ-F6-A/B/C/D + REQ-F8-A/B/C
- `.planning/REQUIREMENTS.md` §性能与体验验收基线 — BASE-04（CI gate 镜像体积，本阶段不动；只是 doctor 不能拖大二进制 size）
- `.planning/REQUIREMENTS.md` §Critical Pitfalls — **C2（mergerfs 默认参数）/ C4（Mutagen 版本漂移）/ C6（AppArmor）/ C8（错误码命名空间冲突）/ M13（静默降级）/ M14（doctor 必须给修复命令）** 必须显式防御
- `.planning/REQUIREMENTS.md` §Open Questions — Q9（doctor `--fix` 幂等性边界）本阶段拍板（D-09）
- `.planning/REQUIREMENTS.md` §Out of Scope — OOS-A11（doctor 不上报诊断到服务端）/ OOS-A12（doctor 不改用户 SSH config）/ OOS-A15（不用 emoji 提示，本阶段渲染坚持 ASCII `✓!✗~`）/ OOS-A20（不新增 host-agent endpoint）
- `.planning/ROADMAP.md` §Phase 34 — 官方 Goal / Scope / 9 条 Success Criteria（plan-checker 验收锚点）
- `.planning/STATE.md` — v3.0 milestone 进度（Phase 33 已 ship，Phase 34 为下一个）

### 前置阶段上下文（必读，避免重复决策）

- `.planning/phases/29-v3-worker/29-CONTEXT.md` §D-05 / D-11 / D-14 / D-23 — `/etc/cloud-claude/mutagen.version` 文件位置 / mergerfs 挂载参数（C2 验收锚点）/ sshd KeepAlive 基线 / AppArmor override 路径（C6）
- `.planning/phases/30-entry-api/30-CONTEXT.md` §D-03 / D-05 — `AuthResponse.{ImageVersion, SupportsMutagen, SupportsMergerfs, ClaudeAccountID}` 字段（doctor banner 显示来源）
- `.planning/phases/31-cli/31-CONTEXT.md` §D-02 / D-19 / D-21 / D-32 — errcodes 包结构 / MOUNT_*+NET_* 14 条已注册 / Format helper / Phase 34 接口预留
- `.planning/phases/31-cli/31-CONTEXT.md` §D-16 / D-22 — last-session.json schema（DowngradeChain）/ OAuth 三态检查（doctor auth 维度复用）
- `.planning/phases/32-ssh-tmux/32-CONTEXT.md` §D-20 / D-25 / D-27 — SESSION_*+NET_* 10 条已注册 / 容器侧 sshd 基线验收 / last-session.json 新字段（tmux_session/client_role/reconnect_count）
- `.planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md` §D-21（admin handler `STATE_VOLUME_IN_USE_001` 字面量已硬编码，本阶段补到 Registry）+ §D-22（admin host detail 含 persistent_volume_name）
- `.planning/phases/29.1-gethost-entry-password-workspace/29.1-CONTEXT.md` — runtime/worker fail-fast + audit event + 包级 var mock 模式（本阶段 fix.go 沿用）

### 研究基线

- `.planning/research/SUMMARY.md` §3 / §5 / §7 — REQ 清单、TOP10 pitfalls（特别 C2/C4/C6/C8/M13/M14）、OOS 边界
- `.planning/research/STACK.md` — Mutagen / mergerfs / sshfs / sshd 版本与挂载参数
- `.planning/research/PITFALLS.md` C2 / C4 / C6 / C8 / M13 / M14 — 本阶段防御重点
- `.planning/research/FEATURES.md` §运维与体验配套 — F6 doctor / F8 错误码设计参考
- `.planning/research/ARCHITECTURE.md` §6（doctor 边界 — 完全本地 + SSH，不调 host-agent endpoint）

### 既有代码（直接改造 / 必读对象）

- `internal/cloudclaude/errcodes/codes.go` — `Registry` / `MustRegister` / `Lookup` / `Format` / `Severity` / `codeRe`（本阶段 5 个新文件遵循同样模式）
- `internal/cloudclaude/errcodes/mount.go` — Phase 31 12 条 MOUNT_* + NET_* 注册示范
- `internal/cloudclaude/errcodes/session.go` — Phase 32 7 条 SESSION_* + NET_* 注册示范
- `internal/cloudclaude/errcodes/net.go` — NET_* 已有 OAuth + reconnect 5 条
- `internal/cloudclaude/ssh_doctor.go` — `RunSSHDoctor` / `runSSHSession` / `parseScanOutput` / `applyFixes` / `Print`（本阶段 doctor `ssh.workspace_ssh_keys` check 复用 + render.go 借鉴样式）
- `internal/cloudclaude/last_session.go` — `LastSessionSnapshot` schema（doctor 第一屏数据源）+ `WriteLastSession` 原子写入模式
- `internal/cloudclaude/oauth_check.go` — Phase 31 OAuth 三态检查（doctor `auth.oauth_credentials` check 复用）
- `internal/cloudclaude/exitcodes.go` — `ExitOK/AuthFailed/NetworkError/Timeout/ConfigError/InternalError/OAuthNotFound/OAuthExpired/MountForceFailed`（doctor 退出码语义对齐）
- `internal/cloudclaude/colors.go` — ansi helper（render.go 直接复用）
- `internal/cloudclaude/entry.go` — `EntryClient.AuthenticateAndWait`（doctor `auth.entry_token_valid` 复用 + `--fix` AUTH_TOKEN_EXPIRED 调）
- `internal/cloudclaude/mount_strategy.go` — `MountConfig` 字段（KeepAliveInterval / NoColor / 等，doctor `ssh.keepalive_config` 校验入口）
- `internal/cloudclaude/sshfs_watcher.go` — sshfs 抖动 watcher 状态（doctor `mount.sshfs_mountpoint` 可读取最近一次状态）
- `internal/cloudclaude/mount_mutagen.go` — Mutagen daemon / sync session helper（doctor mount 检查 + `--fix` daemon restart 复用）
- `internal/cloudclaude/mutagen_bin.go` — embed 二进制版本字符串（doctor `mount.mutagen_version_match` 客户端侧版本来源）
- `cmd/cloud-claude/main.go` — cobra root + `DisableFlagParsing` 关闭判定 case 列表 + 子命令注册位置
- `cmd/cloud-claude/sessions.go` / `sync.go` — 现有 cobra 子命令注册示范
- `deploy/scripts/host-preflight.sh:check_apparmor_fusermount3` — AppArmor override 检测逻辑（本阶段 doctor `mount.apparmor_fusermount3` Go 改写参考）
- `deploy/docker/managed-user/sshd_config` — `ClientAliveInterval 15` / `ClientAliveCountMax 8`（doctor `ssh.sshd_keepalive_drift` 验收基线）
- `deploy/docker/managed-user/Dockerfile` — `/etc/cloud-claude/mutagen.version` 写入位置（doctor `mount.mutagen_version_match` 远端读取来源）
- `internal/controlplane/http/admin_claude_accounts.go` — `STATE_VOLUME_IN_USE_001` 字面量（D-27 本阶段补 Registry 注册）
- `internal/cloudclaude/integration_test.go` — Phase 31/32 docker fixture 测试样例（Plan 03 集成测试沿用）
- `internal/cloudclaude/test-fixture-up.sh` / `test-fixture-down.sh` — docker compose fixture 启停脚本

### 新增文件预告

- `internal/cloudclaude/doctor/doctor.go` — 顶层 RunDoctor + Options + Report
- `internal/cloudclaude/doctor/check.go` — Check struct + Checker interface + runWithTimeout
- `internal/cloudclaude/doctor/network.go` / `auth.go` / `ssh.go` / `mount.go` / `disk.go` — 5 维度检查项
- `internal/cloudclaude/doctor/remote_runner.go` — SSH 远端命令执行包装
- `internal/cloudclaude/doctor/fix.go` — FixerRegistry + 5 类修复 + confirmDestructive
- `internal/cloudclaude/doctor/render.go` — Text + JSON 双输出
- `internal/cloudclaude/doctor/explain_index.go` — RelatedChecks map[Code][]string（可选）
- `internal/cloudclaude/errcodes/state.go` / `system.go` / `ssh.go` / `auth.go` / `disk.go` — ≥ 17 条新 Code 注册
- `internal/cloudclaude/errcodes/explanations.go` — ExtendedExplanations map + ExplainExempt set
- `internal/cloudclaude/errcodes/explanations_test.go` — map 完备性 + Registry 覆盖断言
- `cmd/cloud-claude/doctor.go` — cobra 子命令
- `cmd/cloud-claude/explain.go` — cobra 子命令
- `cmd/cloud-claude/explain_test.go` — os/exec 子进程级测试
- `scripts/ci-doctor-grep.sh` — M14 终验脚本（jq + grep）

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `errcodes.Registry()` / `Lookup` / `MustRegister` / `Format` 已就位（Phase 31 D-32 接口）；本阶段直接 import 不引入新 helper
- `ssh_doctor.go:runSSHSession` / `parseScanOutput` / `applyFixes` / `Print` 全部可借鉴；`doctor/render.go` 沿用相同 ASCII 框架（`╭─╮│╰─╯`）+ `printGroup` 风格
- `oauth_check.go` 已实现「远端 read /home/claude/.claude/.credentials.json + JSON 解析 + expiresAt 三态」逻辑；doctor `auth.oauth_credentials` check 直接 wrap
- `last_session.go:LastSessionSnapshot` 全字段就位（DowngradeChain / ConflictCount / TmuxSession / ClientRole / ReconnectCount）；doctor 第一屏只读不写，零 schema 变更
- `mutagen_bin.go` embed 二进制中已有 `MUTAGEN_VERSION` 字符串常量（Phase 31 D-03 落地），doctor `mount.mutagen_version_match` 客户端侧版本来源
- `EntryClient.AuthenticateAndWait` 已支持 dry-run（callback 不会真启动 SSH）；doctor `auth.entry_token_valid` 直接复用做活性探测
- `colors.ansiYellow / ansiGreen / ansiRed / ansiGray / colorize / colorEnabled` 全部就位；render.go 不引入新颜色 helper

### Established Patterns

- **错误返回**：v3.0 全路径已统一 `errcodes.Format(code, args...) + fmt.Errorf("...: %w", err)` 包装（Phase 31 D-21 → 32 D-21 → 33 沿用）；本阶段 doctor / explain 完全延续，禁止自创格式
- **包级 var mock**：runtime/worker fail-fast `var execInContainer = realExecInContainer`（Phase 29.1 Plan 02）+ Phase 33 `var ensureDockerVolume = realEnsureDockerVolume` 模式；本阶段 `doctor/fix.go` 用同样模式 mock `mutagen daemon`/`fusermount`/`ssh-keygen`/`dscacheutil` 等系统调用
- **SSH 远端命令构造**：统一 `shellescape.QuoteCommand`（v2.0 / Phase 31 / 32 / 33 一致）；本阶段 `remote_runner.go` 延续
- **超时控制**：`context.WithTimeout` + errgroup（Phase 31 / 32 已统一）；本阶段每个 check 5s 默认 + Verbose 30s
- **cobra 子命令注册**：`AddCommand(newXxxCmd())` + DisableFlagParsing case 追加（Phase 32 sessions.go 示范）；本阶段 `doctor` / `explain` 按同样模式
- **JSON schema 演进**：`omitempty` 新字段 + 不动 schema_version（Phase 30 D-03 / 31 D-32 / 32 D-27 / 33 D-23 全延续）；doctor JSON 输出 schema_version=1 锁死
- **测试 mock**：interface vs 包级 var 二选一；本阶段 `RemoteRunner` interface（注入更干净）+ `fix.go` 走包级 var（与 Phase 33 一致）

### Integration Points

- Phase 30 `AuthResponse` 字段（`image_version` / `supports_*` / `claude_account_id`）→ doctor banner 显示 + `mount.mutagen_version_match` / `auth.oauth_credentials` 必读
- Phase 31 errcodes Registry + last-session.json 双接口 → 本阶段 doctor / explain 全链路依赖
- Phase 32 SESSION_* / NET_* + last-session.json 新字段 → doctor `ssh` 维度 + 第一屏 reconnect_count 渲染
- Phase 33 `STATE_VOLUME_IN_USE_001` → 本阶段 D-27 补 Registry + ExtendedExplanations
- 现有 `cloud-claude ssh doctor` 子命令（v2.0 quick task 260417-0w4）→ 不动入口，本阶段 `doctor ssh` 维度内部调用 `RunSSHDoctor` 共享底层

### 既有反模式 / 已知坑

- `cloud-claude` 二进制在 Phase 31 已 embed Mutagen 4 平台二进制 ~12MB；本阶段不能再大幅膨胀（doctor 文件全 Go 源代码不影响 size，但 ExtendedExplanations 17 条 × ~400 字符 ≈ 7KB 字符串可忽略）
- `internal/controlplane/http/admin_hosts.go:getDockerStatuses` 直接 `exec.Command("docker", "ps", ...)` 是 Phase 29.1 已识别的测试基础设施债务 — doctor 与控制面无依赖，不受影响
- 历史 `cloud-claude ssh doctor` 子命令的 cobra 入口字面量是 `cloud-claude ssh doctor`（subcommand 嵌套），本阶段新加的 `cloud-claude doctor [domain]` 是顶层 `doctor` + ValidArgs，不与之冲突；但 `--help` 会同时列出，文档需要解释清楚两者关系（Plan 02 README 章节）

</code_context>

<specifics>
## Specific Ideas

- 用户对 v3.0 的核心承诺是「日常开发主战场」——本阶段 doctor 5 维度 + 错误码统一是 v3.0 ship 给用户的「自助排查工具」；planner 在 Plan 切分时应把 D-13 第一屏降级历史 / D-21 错误码注册表收口 / D-09 `--fix` 5 类视为 P0，禁止简化为「只跑几个 check 就 ship」
- ROADMAP §Phase 34 Success Criteria 第 3 条 `CI grep cloud-claude doctor 输出，所有 [!] 与 [✗] 行必须含"建议:"子串`：是 PITFALLS M14 的关键回归 — 本阶段必须有 `scripts/ci-doctor-grep.sh` 并接入 CI（建议挂 Plan 03 收尾 task）
- ROADMAP §Phase 34 Success Criteria 第 6 条 `强制让 cloud-claude 上次连接静默降级到 sshfs-only 后，cloud-claude doctor 第一屏必须展示降级历史`：M13 终验由本阶段闭合 — Plan 02 必须有用例构造 last-session.json 含 DowngradeChain 后跑 doctor 断言 stdout 包含 `[降级]` 行
- ROADMAP §Phase 34 Success Criteria 第 7 条 `mergerfs 参数被人为篡改后，doctor mount 必须输出错误码 + 修复命令`：是 C2 + M14 联合验收 — Plan 03 集成测试构造 `docker exec <ctr> umount /workspace && mount -o cor:1 ...`（修改 readdir 参数）后断言 doctor mount 输出含 `MOUNT_MERGERFS_FAILED` + next_action 中的 `cloud-claude doctor mount --fix`
- ROADMAP §Phase 34 Success Criteria 第 8 条 `cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW 等任意错误码均能返回中文详细说明 + 常见修复步骤`：本阶段 ExtendedExplanations 必须覆盖所有非 informational Code（D-18），CI 测试遍历 Registry 断言（D-24 第 4 条）
- ROADMAP §Phase 34 Success Criteria 第 9 条 `doctor 全程不调用 host-agent 任何新 endpoint（边界守恒）`：本阶段对 host-agent 严格只读 — 远端验证全走 SSH conn 在容器内 exec，禁止给 host-agent 加 `/v1/diagnose` 类 endpoint（与 Phase 33 D-16 / Phase 30 D-04 一致）
- v3.0 PROJECT.md 强调「零增量特权」——本阶段 doctor 在容器内 exec 的所有命令都不需要新增 capability（mergerfs xattr / sshd -T / df / du 全部用户态）；本地 fix 涉及 `dscacheutil killall -HUP mDNSResponder`（macOS）需要 root 时，提示用户 `sudo` 而非 cloud-claude 自动 elevation
- 多端使用场景下（Phase 32 多端 attach），doctor 在 secondary 端跑也应该 work（不需要 Mutagen 单例锁）；本阶段 `mount` 维度的 Mutagen check 应识别 secondary client（last-session.json `client_role=secondary`）并标记 `[~] 当前为 secondary 端，跳过 Mutagen sync 状态检查`

</specifics>

<deferred>
## Deferred Ideas

### 阶段内确认但不交付的 follow-up

- **错误码 i18n（英文版本）**：本阶段中文硬编码；i18n 框架留 v3.1（OOS 未明确禁但优先级低，与 Phase 31 D-22 一致）
- **doctor `--fix` 模糊匹配 / 批量修复**：如 `doctor --fix all` 一次扫所有 check 跑全 fix；本阶段 `--fix` 只对当前 domain 的 fail/warn 跑，all domain 时遍历也是串行；批量并发推迟 v3.1
- **doctor 历史报告归档**：每次 `doctor` 把 Report 落盘到 `~/.cloud-claude/doctor-reports/<timestamp>.json`；运维场景有用但 v3.0 不做（与 OOS-A11 不上报服务端配套）
- **`cloud-claude explain --list` 列出所有 Code**：本阶段 explain 必须传一个 Code；列表能力推迟 v3.1（也可以直接 `cloud-claude doctor --json | jq '.checks[].code'` 间接获得）
- **doctor 检查项并发执行**：D-07 选定串行；并发推迟 v3.1（doctor 是低频运维场景，串行一致性 + 调试容易优先）
- **doctor `--fix` 自动修复后再跑一次自检确认 pass**：当前实现单次 fix 后输出 `FixApplied`；自动 re-run 推迟 v3.1（避免无限循环风险）
- **跨主机 doctor**（如 `doctor --gateway=other.example.com`）：本阶段 doctor 仅查询当前 init 的网关；多 gateway 切换推迟 v3.1
- **doctor JSON schema_version=2**：如未来需要新增大量字段（如 `details` 强类型化、`fix_history` 数组），开 v3.1 升 schema_version；本阶段锁 v1
- **explain 模糊匹配 / 子串搜索**（如 `explain MOUNT_MUTAGEN` 列 6 条）：本阶段精确匹配；模糊推迟 v3.1
- **doctor `--watch` 模式**：周期性重跑直到所有 check pass / Ctrl+C；本阶段单次跑完退出；watch 推迟 v3.1
- **v2.0 lower-case 错误码迁移到 Registry**：D-22 显式不动；独立 v3.1 phase 做迁移 + 客户端兼容映射
- **doctor `mount.mergerfs_branches` 自动 remount 修复**：mergerfs 重挂涉及容器侧 mount 操作（C3 风险），本阶段 `--fix` 不自动重挂；只输出 next_action 提示用户 `cloud-claude exec sudo mount -o ...`，自动重挂留 Phase 35 真机验证后再决定
- **`cloud-claude doctor --report-bug`**：把 Report 复制到剪贴板 + 打开浏览器到 GitHub Issues；OOS-A11 周边，推迟 v3.1
- **doctor 远端 ssh.workspace_ssh_keys 错误码化**：当前 RunSSHDoctor 报告无错误码，本阶段保留为 details 即可；统一错误码化推迟 v3.1（涉及 v2.0 quick task 入口的兼容性）

### Reviewed Todos (not folded)

无（`todo match-phase 34` 返回 0 条匹配）。

</deferred>

---

*Phase: 34-cloud-claude-doctor-v3*
*Context gathered: 2026-04-21*
*Mode: --auto（所有 gray area 自动选定推荐项）*
