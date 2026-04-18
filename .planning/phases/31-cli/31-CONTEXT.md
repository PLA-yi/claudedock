# Phase 31: CLI 三层文件映射重构 - Context

**Gathered:** 2026-04-18
**Status:** Ready for planning
**Mode:** `--auto`（所有 gray area 自动选定推荐项，决策来源标注于 `<decisions>` 各条尾注）

<domain>
## Phase Boundary

把 cloud-claude 端的文件映射从 v2.0 单层 sshfs 升级为 **Mutagen 热同步 + sshfs 冷兜底 + mergerfs 联合视图** 三层架构，实现 `--mount-mode={auto|full|mutagen-only|sshfs-only}` 显式降级状态机；并把 F7-C（连接前 OAuth 过期中文提示）织入 SSH 握手成功后、`claude` 进程拉起前的窗口期。

本阶段交付：

1. `internal/cloudclaude/mount.go` 拆分为 `mount_sshfs.go` / `mount_mutagen.go` / `mount_merge.go` / `mount_strategy.go` 四文件 + `errcodes/codes.go` 错误码注册表雏形
2. `go:embed` 集成 Mutagen v0.18.1 多平台二进制（darwin/linux × amd64/arm64），首次运行 extract 到 `~/.cloud-claude/bin/mutagen`；daemon 长期复用（不随 cloud-claude 退出停止）
3. 启动时 mutagen 客户端版本与容器内 `/etc/cloud-claude/mutagen.version` 比对（不一致直接降级 sshfs-only + 错误码 `MOUNT_MUTAGEN_VERSION_SKEW`，防御 PITFALLS C4）
4. Mutagen sync 默认 `--mode=two-way-resolved`（Q3 拍板，详见 D-08）+ ignore 默认列表（`.git/`、`node_modules/`、`target/`、`dist/`、`*.pyc`、`.venv/`、`__pycache__/`、`.next/`、`build/`）
5. 启动前轻量安全门（≤300ms 远程探测）：alpha empty + beta non-empty 命中即拒绝并报错 `MOUNT_MUTAGEN_SAFETY_GUARD`（防御 PITFALLS C5 / Q7 倾向）
6. 候选目录 `du -sb` > 50MB 拒绝热同步 + 自动降级 sshfs + 中文 ignore 配置建议（REQ-F1-D）
7. 降级状态机：`full → mutagen-only → sshfs-only → failed`；`auto` 任一层失败 ≤2s 内降级到下一档；`full` / `mutagen-only` / `sshfs-only` 各自单档失败即退出；**任何静默降级视为缺陷**（REQ-F2-B / PITFALLS M13）
8. 三段式中文进度（stderr）：`初始化文件映射 (1/3) 热同步源码中…` → `(2/3) 启动冷兜底…` → `(3/3) 合并视图…`；连接成功 banner 显示彩色 `[<mode>]` 标签 + `NO_COLOR` 关色（REQ-F1-B / REQ-F2-C）
9. macOS APFS case-insensitive 启动检测（`diskutil info /`），命中时打印 `MOUNT_APFS_CASE_INSENSITIVE` informational 信息，不论默认 mode 强制走 `--mode=two-way-resolved`（PITFALLS M5）
10. Mutagen conflict 冒泡：每次启动从 `mutagen sync list --json` 读 conflict 计数，>0 时下次回车前 prompt 上方插入中文警告 `⚠ 有 N 个文件同步冲突，运行 cloud-claude sync conflicts 查看`（REQ-F1-E）
11. 并发时序：cloud-claude 启动两个 SSH connection（控制 conn-A / 数据 conn-B）；Mutagen sync 与 sshfs mount 在 conn-B 上由两个 goroutine 并发拉起；mergerfs 在两层均 ready 后由 SSH 远程命令在容器内挂载 `/workspace-hot=RW:/workspace-cold=NC,RO /workspace`（branch 拓扑由 Phase 29 D-12 锁定）
12. sshfs 抖动主动摘除（PITFALLS C3）：sshfs 挂参 `reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10`；cloud-claude 后台 watcher 每 5s `mountpoint -q /workspace-cold`，连续 3 次失败（15s）从 mergerfs runtime branch 摘除并打 `MOUNT_SSHFS_DISCONNECTED` 警告
13. OAuth 过期检查：SSH 握手成功后、Mutagen sync create / sshfs mount 启动**之前**（窗口期内并发执行），通过远程命令读 `/home/claude/.claude/.credentials.json`，按 D-22 三态分支处理（REQ-F7-C）
14. 错误码注册表雏形：本阶段引入 `MOUNT_*` / `NET_*` 前缀（≥ 11 条），注册到 `internal/cloudclaude/errcodes/codes.go`，每条带中文 `Message` + `NextAction`；Phase 34 doctor / explain 复用同一注册表（REQ-F8-A / REQ-F8-B 的 phase-31 落码部分）

**不在本阶段交付**：
- tmux 包装、SSH KeepAlive 客户端基线、自动重连退避、本地输入缓冲、多端共享 attach、账号级 Mutagen 单例锁 → Phase 32
- Docker named volume 创建 / admin DELETE 联动 / entrypoint symlink → Phase 33
- `cloud-claude doctor`（doctor 仅消费本阶段注册表，自身实现在 Phase 34）/ `cloud-claude explain` / `--fix` / `--json` → Phase 34
- 真机性能基线验收（10k 文件 1.5×、首连 ≤8s 5/4 通过、镜像体积二次回归） → Phase 35

</domain>

<decisions>
## Implementation Decisions

### 文件结构

- **D-01**：`internal/cloudclaude/mount.go` 拆分为 4 个文件，原始单文件保留为 `mount_legacy_test.go` 仅用于回归（最终删除由 plan 决定）：
  - `mount_sshfs.go` — v2.0 sshfs + 嵌入 SFTP server 逻辑剥离，作为 `sshfs-only` / `cold` 分支唯一实现；导出 `mountSSHFS(conn, localDir, remoteCold) (cleanup, err)`
  - `mount_mutagen.go` — Mutagen daemon 引导、二进制 extract、版本比对、sync session create/list/terminate；导出 `mountMutagen(conn, localDir, remoteHot, opts) (cleanup, err)`
  - `mount_merge.go` — 远端 mergerfs 挂载 / 卸载 / runtime branch 摘除（C3 主动摘除）；导出 `mountMerge(conn, branches, target) (cleanup, err)`
  - `mount_strategy.go` — 顶层入口 `MountWorkspace(conn, cfg) (cleanup, mode, err)`，承载 `--mount-mode` 状态机、并发 goroutine、降级转移、stderr 三段式进度、banner 输出
- **D-02**：错误码注册表新建包 `internal/cloudclaude/errcodes`：
  - `errcodes/codes.go` — `Code` 类型、`Entry{Code, Message, NextAction}` struct、全局 `Registry map[Code]Entry`、`Lookup(code)` / `MustRegister(...)` 工具函数
  - `errcodes/mount.go` — 本阶段 `MOUNT_*` 注册（D-19）
  - `errcodes/net.go` — 本阶段 `NET_*` 注册（D-19）
  - 单元测试遍历 `Registry` 断言：无重复 code、`Message` / `NextAction` 均非空、code 命名匹配 `^[A-Z]+_[A-Z]+_[A-Z0-9]+$`（防御 PITFALLS C8 / 给 Phase 34 留接口）

### 二进制分发与 daemon（Q1 + Q2）

- **D-03**：Mutagen 客户端二进制采用 `go:embed` 嵌入 4 个平台变体：`darwin_amd64` / `darwin_arm64` / `linux_amd64` / `linux_arm64`，每个 ~3MB，总计 ~12MB；版本严格锁定为 `v0.18.1`（与 Phase 29 D-05 容器内 mutagen-agent 一致）。embed 文件位于 `internal/cloudclaude/mutagen_bin/<platform>/mutagen`，构建脚本 `scripts/fetch-mutagen-bins.sh` 从 GitHub release 拉取 + sha256 校验 + 裁剪 `mutagen.exe` 等无关产物。仓库提交时 4 个二进制纳入 git LFS（如团队拒绝 LFS 则裸提交，4×3MB = 12MB 可接受）。  
  *[auto] Q1 推荐项：go:embed（替代：检测 brew install 引入用户摩擦、首次运行下载需出网且违背"零增量特权"目标）*
- **D-04**：首次运行时 extract 到 `~/.cloud-claude/bin/mutagen`，权限 `0755`；如 extract 目录已存在同名二进制，校验 `mutagen version` 输出包含 `0.18.1` 后直接复用，否则覆盖（防止用户旧版本污染）。
- **D-05**：Mutagen daemon 由 cloud-claude 启动时 `mutagen daemon start`（Mutagen 自身天然幂等，daemon 已在则 noop）；**cloud-claude 退出时不停 daemon**（长期复用，避免每次 ~500ms 启动开销 / 触发 ≤8s 基线）。daemon data 目录强制为 `~/.cloud-claude/mutagen/`，通过 `MUTAGEN_DATA_DIRECTORY` 环境变量隔离，避免与用户已装的 brew mutagen 冲突。  
  *[auto] Q2 推荐项：长期复用（替代：每次起停 daemon 增加首连耗时、违背 ≤8s 基线）*
- **D-06**：多 cloud-claude 并发同一 daemon：依赖 Mutagen daemon 自身的单实例约束（同一 `MUTAGEN_DATA_DIRECTORY` 只能跑一个 daemon），多 cloud-claude 共享同一 daemon；session 命名约定 `cloud-claude-{claude_account_id}-{cwd_hash8}`（cwd_hash8 = sha256(absCwd)[:8]），保证不同项目目录不碰撞。**账号级单例锁（REQ-F5-D）由 Phase 32 落地**，本阶段只确保 session 命名唯一、不主动抢占。
- **D-07**：daemon 启动失败（如 unix socket 权限错误、版本损坏）→ 错误码 `MOUNT_MUTAGEN_DAEMON_UNAVAILABLE`，自动降级到 `sshfs-only`（除非 `--mount-mode=full|mutagen-only`，那种情况下退出非 0）。

### Mutagen 同步模式（Q3 + macOS APFS / M5）

- **D-08**：Mutagen 默认 `--mode=two-way-resolved`，`alpha` = 本地 cwd（用户视角"权威"），`beta` = `/workspace-hot`（容器内 mergerfs 上层 RW branch）。理由：
  1. cloud-claude 典型工作流是"本地编辑 + 容器内 claude code 偶尔写入"，本地权威符合用户直觉
  2. 冲突自动按 alpha 解决，**避免冲突堆积阻塞 claude**（与 OOS-A2 一致：不暴露 5 种冲突模式）
  3. macOS APFS case-insensitive 兼容性强制走 `two-way-resolved`（M5），统一默认值减少"双语义"
  4. Mutagen 内置 safety mode（拒绝 alpha 完全清空时同步到 beta）依然开启，配合 D-13 安全门防御 C5
  *[auto] Q3 推荐项：two-way-resolved（替代 two-way-safe：冲突堆积人工介入会高频触发 REQ-F1-E 警告，影响"日常开发主战场"目标）*
- **D-09**：macOS APFS 检测：cloud-claude 启动时执行 `diskutil info / | grep "Case-sensitive"`（macOS）/ 跳过（Linux），命中 `Case-sensitive: No` 时：
  1. stderr 打印 `MOUNT_APFS_CASE_INSENSITIVE` informational 行：`已启用 macOS APFS 兼容模式（two-way-resolved）`
  2. 已是默认模式，不需额外强制；但写入决策日志供 doctor 读取
  3. mergerfs 内部 case-sensitive，跨 case 文件冲突由 Mutagen `two-way-resolved` 自然解决
- **D-10**：强制 owner / group：Mutagen `--default-owner-beta=id:1000 --default-group-beta=id:1000`（与 Phase 29 D-17 容器 UID / GID 对齐）；mode 留 Mutagen 默认（`0644` 文件 / `0755` 目录），不显式设置避免与 git 默认行为冲突。

### 首次同步安全门 + 白名单（Q7 + REQ-F1-D）

- **D-11**：候选目录大小检查：本地执行 `du -sb {cwd}` 取 byte 数，> 50MB（52428800）触发拒绝热同步：
  1. 错误码 `MOUNT_MUTAGEN_WHITELIST_REJECT` 输出到 stderr
  2. 中文提示包含 ignore 配置建议：`同步候选目录 {cwd} 体积 {N}MB（>50MB），已自动降级 sshfs。建议在 .mutagen.yml 添加 ignore 规则；当前最大子目录: {top1} {top2} {top3}`
  3. `top1/2/3` 来自 `du -sh {cwd}/* | sort -hr | head -3`
  4. 自动降级到 `sshfs-only`（`auto` 模式）/ 退出非 0（`full` / `mutagen-only` 模式）
- **D-12**：默认 ignore 列表（写入运行时生成的 `~/.cloud-claude/mutagen-defaults.yml`）：`.git/` `node_modules/` `target/` `dist/` `*.pyc` `.venv/` `__pycache__/` `.next/` `build/` `.cache/` `.DS_Store`。用户工程级 `.mutagen.yml` 优先级高于默认（Mutagen 自身合并语义）。
- **D-13**：首次同步前安全门（防御 PITFALLS C5）：
  1. **轻量探测**（≤300ms，并发执行不阻塞 ≤8s 基线）：
     - alpha 端：`os.ReadDir(cwd)` 本地直接读，过滤掉 ignore 列表后判断是否为 empty
     - beta 端：在 conn-B 远程执行 `find /workspace-hot -mindepth 1 -maxdepth 1 -not -name '.*' | head -1`（非空即视为 beta non-empty）
  2. **触发条件**：alpha=empty AND beta=non-empty（"本地空目录 + 远端有文件" 是 C5 反向清空场景）
  3. **触发动作**：错误码 `MOUNT_MUTAGEN_SAFETY_GUARD` + 中文提示`检测到本地目录为空但容器内 /workspace-hot 已有文件，拒绝执行同步以防止反向清空。如确认要从远端拉取，请先 cloud-claude exec rsync /workspace-hot/ ./` + **退出非 0**（不静默降级，C5 是数据安全风险，必须用户确认）
  4. **范围**：仅在 Mutagen sync 首次创建时检查（已存在同名 session 则跳过，复用已建立的 sync）
  *[auto] Q7 推荐项：是 + 轻量级（替代：完整 diff 阻塞 ≤8s 基线 / 信任 Mutagen safety mode 违背 C5）*

### 降级状态机 + `--mount-mode`（REQ-F2 + M13）

- **D-14**：`--mount-mode` CLI flag 四档枚举（cobra 校验，无效值直接报错）：
  - `auto`（默认）：尝试 `full → mutagen-only → sshfs-only`，每档失败 ≤2s 内自动降级
  - `full`：必须 mutagen + sshfs + mergerfs 全部 ready，任一层失败立即退出非 0 + 错误码 `MOUNT_FORCE_MODE_FAILED`
  - `mutagen-only`：仅启动 Mutagen + 容器内 `/workspace = bind /workspace-hot`（不挂 sshfs / 不挂 mergerfs），适用于"已知小项目想最快"场景
  - `sshfs-only`：跳过 Mutagen，直接 v2.0 行为（sshfs → `/workspace`）
- **D-15**：状态机实现：`mount_strategy.go` 用 enum `Mode { ModeFull, ModeMutagenOnly, ModeSSHFSOnly, ModeFailed }`：
  1. `auto` 模式按 `[ModeFull, ModeMutagenOnly, ModeSSHFSOnly]` 依次尝试，每档启动超时 2s（context.WithTimeout）；超时 / 失败立即 cleanup 当前档资源并降级
  2. 每次降级在 stderr 输出一行降级 banner：`[!] MOUNT_AUTO_DOWNGRADED: <from> → <to>，原因 <错误码> <中文>，运行 cloud-claude doctor 查看修复建议`（M13 防御：**任何静默降级视为缺陷**）
  3. 最终模式作为 `MountWorkspace` 返回值之一，由调用方在 banner 上展示
  4. 测试矩阵：`(--mount-mode × 各层失败注入) = 4 × 3 = 12` 个用例，断言最终 mode 与 stderr 输出
- **D-16**：「禁止静默降级」实现细节（M13 验收）：
  - stderr 写入用专用 logger（带 `[mount]` 前缀），与 cloud-claude 其它日志区分
  - 降级事件同时写入 `~/.cloud-claude/last-session.json`（schema：`{"timestamp", "intended_mode", "actual_mode", "downgrade_chain": [{"from", "to", "reason_code", "reason_message"}]}`）
  - Phase 34 doctor 第一屏读取 `last-session.json` 展示降级历史（M13 终验由 Phase 34 验证，本阶段产出数据）
- **D-17**：banner UI（连接成功后 cloud-claude 主流程拉起 claude 之前打印一次）：
  ```
  ✓ 文件映射就绪 [<mode>]
  ```
  颜色规则：
  - `full` → ANSI 32（绿色）
  - `mutagen-only` → ANSI 33（黄色）
  - `sshfs-only` → ANSI 33（黄色，附带降级原因行）
  - `NO_COLOR=1` 或非 TTY → 关色，仅输出 `[<mode>]`
  使用现有 `internal/cloudclaude/colors.go`（如不存在则在本阶段新增 `colors.go` 极简实现，避免引入第三方 color 库）
- **D-18**：三段式中文进度（REQ-F1-B）：
  - `(1/3) 热同步源码中…`：在 Mutagen sync create 之前打印；如 mutagen 被跳过（sshfs-only）则改为 `(1/3) 跳过 Mutagen（模式: sshfs-only）`
  - `(2/3) 启动冷兜底…`：在 sshfs mount 之前打印；mutagen-only 模式下改为 `(2/3) 跳过 sshfs（模式: mutagen-only）`
  - `(3/3) 合并视图…`：在 mergerfs 远端挂载之前打印；mutagen-only / sshfs-only 模式下改为 `(3/3) 跳过 mergerfs（模式: <mode>）`
  - 三段顺序按降级状态机最终决策呈现（即先决策 mode 再分别打印），不会出现"打了 (1/3) 又改主意"

### 错误码命名空间（REQ-F8-A / B 的 phase-31 落码 + PITFALLS C8）

- **D-19**：本阶段必须落地的错误码（注册到 `errcodes/mount.go` + `errcodes/net.go`，每条三要素：`Code` + 中文 `Message` 模板 + 中文 `NextAction`）：
  | Code | 触发 | 防御 |
  |------|------|------|
  | `MOUNT_MUTAGEN_VERSION_SKEW` | 客户端 mutagen 版本 ≠ 容器 `/etc/cloud-claude/mutagen.version` | C4 |
  | `MOUNT_MUTAGEN_WHITELIST_REJECT` | cwd > 50MB | REQ-F1-D |
  | `MOUNT_MUTAGEN_SAFETY_GUARD` | alpha empty + beta non-empty | C5 / Q7 |
  | `MOUNT_MUTAGEN_DAEMON_UNAVAILABLE` | `mutagen daemon start` 失败 | — |
  | `MOUNT_MUTAGEN_SYNC_FAILED` | `mutagen sync create` 失败 / 超时 | — |
  | `MOUNT_SSHFS_FAILED` | sshfs 启动失败 / mountpoint 检查超时 | — |
  | `MOUNT_SSHFS_DISCONNECTED` | watcher 检测 sshfs 抖动 ≥15s | C3 |
  | `MOUNT_MERGERFS_FAILED` | 容器内 mergerfs 挂载命令失败 | — |
  | `MOUNT_AUTO_DOWNGRADED` | 状态机降级（informational） | M13 |
  | `MOUNT_FORCE_MODE_FAILED` | `--mount-mode=full|mutagen-only|sshfs-only` 任一层失败 | — |
  | `MOUNT_APFS_CASE_INSENSITIVE` | macOS APFS 检测命中（informational） | M5 |
  | `NET_OAUTH_EXPIRED` | credentials 已过期 | REQ-F7-C |
  | `NET_OAUTH_EXPIRING_SOON` | credentials < 5min 即将过期 | REQ-F7-C |
  | `NET_OAUTH_NOT_FOUND` | `/home/claude/.claude/.credentials.json` 不存在 | REQ-F7-C |

  ≥ 14 条已超过 ROADMAP 暗示的 ≥ 11 条下限，覆盖率充分。
- **D-20**：错误码命名空间与 v2.0 现有 7 个错误码无冲突（C8）：v2.0 现网码均为 `auth_*` / `host_action_failed` / `entry_*` 形态（小写下划线），本阶段全部 `<DOMAIN>_<KIND>` 大写形态，命名规则上即不可能撞码。Phase 34 在收口时会执行更严格的 `^[A-Z]+_[A-Z]+_[A-Z0-9]+$` 注册表校验。
- **D-21**：错误输出格式统一通过 `errcodes.Format(code, args...) string` helper：
  ```
  [<code>] <Message>
    建议: <NextAction>
  ```
  本阶段只在 mount 路径使用，Phase 34 doctor 与 explain 复用同一 helper。

### OAuth 过期检查（REQ-F7-C）

- **D-22**：检查时机：SSH 握手成功 + conn-A / conn-B 建立完毕之后、Mutagen sync create / sshfs mount 启动**之前**，作为独立 goroutine 在 conn-A 上并发执行（不阻塞 mount 路径）：
  1. 远程执行 `cat /home/claude/.claude/.credentials.json 2>/dev/null`，超时 2s
  2. 解析 JSON：取 `expiresAt`（毫秒级 Unix 时间戳）
  3. 三态分支：
     - 文件不存在 / 输出为空 → `NET_OAUTH_NOT_FOUND`：中文提示 `Claude 账号未登录（账号: {claude_account_id}），请在容器内运行 cloud-claude exec claude login`，cleanup mount + 退出码 4
     - `expiresAt < now` → `NET_OAUTH_EXPIRED`：中文提示 `Claude OAuth 凭证已过期（账号: {claude_account_id}），请先重新登录`，cleanup mount + 退出码 5
     - `expiresAt - now < 5min` → `NET_OAUTH_EXPIRING_SOON`：警告打印（`[!] Claude OAuth 凭证将在 {N} 分钟后过期，建议尽快 cloud-claude exec claude login`）但**继续启动**
  4. JSON 解析失败 / 字段缺失 → 视为 `NET_OAUTH_NOT_FOUND`（保守降级，避免无意义阻塞）
- **D-23**：检查与 mount 并发执行（同一 errgroup）：任一方失败先 cleanup 已起的资源；OAuth 失败优先级高于 mount 失败（错误信息以 OAuth 为主，避免用户被 mount 错误带偏方向）。
- **D-24**：检查依赖 Phase 30 D-05 返回的 `claude_account_id`；若 Entry API 响应未带（旧 gateway / 字段为空），跳过 OAuth 检查 + 打印 `[!] gateway 未返回 claude_account_id，跳过 OAuth 过期检查（建议升级 gateway 至 v3.0）`，**不阻塞** mount。

### 并发挂载时序

- **D-25**：cloud-claude 启动两个 SSH connection：
  - **conn-A（控制 / 命令通道）**：SSH 握手、OAuth 检查（D-22）、mergerfs 远端挂载命令、tmux session（Phase 32 接管）、最终 claude 进程 attach
  - **conn-B（数据通道）**：sshfs sftp（v2.0 既有 `mountWorkspace` 在 conn-B 上跑）+ Mutagen sync 走 mutagen-agent 自带的 ssh transport（透传 SSH 凭证由 Mutagen 自身处理）
  - 两 conn 各自独立握手，避免 SSH multiplexing 在弱网下相互拖垮（Phase 32 KeepAlive 调整后两 conn 同步生效）
- **D-26**：Mutagen sync ‖ sshfs mount 在 conn-B 上由两个 goroutine 并发拉起（errgroup），两者均 ready 后由 conn-A 远程执行 mergerfs 挂载：
  ```bash
  sudo mergerfs -o category.create=ff,func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,inodecalc=path-hash \
    /workspace-hot=RW:/workspace-cold=NC,RO /workspace
  ```
  挂载参数与 Phase 29 D-11 完全一致；branch 拓扑遵循 Phase 29 D-12 的 2 路 + `CLOUD_CLAUDE_MERGERFS_BRANCHES` 扩展点（本阶段读环境变量但默认 2 路）。
- **D-27**：sshfs 抖动主动摘除（PITFALLS C3）：
  - sshfs 挂载命令在 v2.0 `sshfs : <path> -o passive -f` 基础上追加：`reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10`
  - cloud-claude 启动后台 watcher goroutine：每 5s 在 conn-A 远程 `mountpoint -q /workspace-cold`，连续 3 次失败（≥15s）：
    1. 在 conn-A 远程执行 mergerfs runtime branch 摘除：`echo /workspace-cold > /workspace/.mergerfs/branches-`（mergerfs 2.41 `branches-` xattr 协议）
    2. 输出 `MOUNT_SSHFS_DISCONNECTED` 警告到 stderr
    3. **不**自动重挂 sshfs（避免无限循环）；恢复留给 Phase 34 doctor `--fix`
  - watcher 在 cloud-claude 退出时 stop（context cancel）

### Mutagen conflict 冒泡（REQ-F1-E）

- **D-28**：cloud-claude 启动后期（mount 全部 ready 之后）调用 `mutagen sync list --json`，若 `conflicts` 数组非空，记录 N 到 `~/.cloud-claude/last-session.json` 的 `conflict_count` 字段；同时在 banner 之后插入一行：
  ```
  ⚠ 有 N 个文件同步冲突，运行 cloud-claude sync conflicts 查看
  ```
  - `cloud-claude sync conflicts` 子命令本阶段实现最小可行版本：调用 `mutagen sync list --json` 后渲染冲突文件列表（路径 + alpha/beta side + last-modified），不做 resolve（resolve 由用户决定 mutagen 命令）
  - 「下次回车前 prompt 上方插入」是 REQ-F1-E 描述的理想效果，本阶段以"启动 banner 后立即输出"实现（"下次回车前"需 PTY 拦截，较复杂留 Phase 34 / v3.1）

### 与 Phase 30 / Phase 32 / Phase 34 的接口

- **D-29**：从 Phase 30 `AuthResponse` 读取的字段（必须容忍缺失）：
  - `image_version` → 无 / 不等于 `v3.0.0` 时直接降级 `sshfs-only`，错误码 `MOUNT_MUTAGEN_VERSION_SKEW`（因为客户端只 embed 了 v0.18.1 二进制，与未升级镜像不兼容）
  - `supports_mutagen` → `false` 直接降级 `sshfs-only`
  - `supports_mergerfs` → `false` 时降级到 `mutagen-only`（如果 mutagen 也不支持，最终落到 `sshfs-only`）
  - `claude_account_id` → 缺失则跳过 OAuth 检查（D-24）+ Mutagen session 名退化为 `cloud-claude-{cwd_hash8}`（无账号维度隔离）
- **D-30**：本阶段不修改 `internal/cloudclaude/ssh.go:ConnectAndRunClaude` 函数签名；改动方式：
  1. 新增 `ConnectAndRunClaudeV3(cfg SSHConfig, claudeArgs []string, cwd string, proxyCommands []string, mountCfg MountConfig, authResp *AuthResponse) (int, error)`
  2. `ConnectAndRunClaude` 保留为兼容入口，内部转调 V3 + `--mount-mode=sshfs-only` 默认值
  3. `cmd/cloud-claude/main.go` 切到 V3 入口，`--mount-mode` flag 由 V3 接收
- **D-31**：本阶段为 Phase 32 预留接口：`MountConfig` struct 包含 `KeepAliveInterval` / `KeepAliveCountMax` 字段（Phase 32 注入），本阶段使用默认 v2.0 值；conn-A / conn-B 的 `ssh.ClientConfig.Auth` 复用同一 password（Entry API token 由 Phase 32 接管缓存）。
- **D-32**：本阶段为 Phase 34 预留接口：
  - `errcodes/codes.go` 导出 `Registry()` 与 `Lookup(Code)` 供 doctor 直接复用
  - `~/.cloud-claude/last-session.json` 写入约定 schema，doctor 第一屏读取
  - mount 各层独立可调用：`mount_mutagen.go` 暴露 `MutagenHealthCheck(daemonReady, agentReady, syncReady) Status`，doctor mount 维度复用

### Claude's Discretion

以下细节由 planner / executor 按实现便利性决定：

- 二进制 fetch 脚本是否纳入 CI（如不纳入，README 中说明手动运行步骤）
- `cloud-claude sync conflicts` 子命令的 cobra 注册位置（建议 `cmd/cloud-claude/sync.go` 新文件）
- `errcodes` 包是否生成 `code` → `messages.go` 静态映射（i18n 留 v3.1，本阶段直接硬编码中文字符串）
- watcher goroutine 的具体退出协议（context cancel + sync.WaitGroup vs channel close）
- macOS APFS 检测的 fallback 逻辑（`diskutil` 未安装、退出非 0 时是否 panic：建议 silent skip，仅记录到 last-session.json）
- 测试矩阵的 mock 方式（mock SSH conn vs 真实 docker-in-docker）：建议主用例走真实 docker（`testcontainers-go`），降级注入用 mock
- `MountConfig` struct 的字段扩展粒度

### Folded Todos

无（`todo match-phase 31` 返回 0 条匹配）。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 规划与需求

- `.planning/PROJECT.md` — v3.0 总目标、Constraints、Key Decisions
- `.planning/REQUIREMENTS.md` §A1 / A2 / B (REQ-F7-C) — 本阶段交付的 REQ-F1-A..E、REQ-F2-A..C、REQ-F7-C
- `.planning/REQUIREMENTS.md` §性能与体验验收基线 — BASE-01 / BASE-02（Phase 35 验收，本阶段为前置）
- `.planning/REQUIREMENTS.md` §Critical Pitfalls — **C1 / C2 / C3 / C4 / C5 / C8 / M13 / M5** 必须显式防御
- `.planning/REQUIREMENTS.md` §Open Questions — Q1 / Q2 / Q3 / Q7 / Q10（Q10 由 Phase 29 D-12 锁定 2 路）本阶段全部拍板（见 D-03/D-05/D-08/D-13）
- `.planning/REQUIREMENTS.md` §Out of Scope — OOS-A2（不暴露 5 种冲突模式）/ OOS-A4（不做 `--mount-mode=none`）/ OOS-A5（不做运行时升级 mode）/ OOS-A20（不新增 session REST endpoint）
- `.planning/ROADMAP.md` §Phase 31 — 官方 Goal / Scope / Success Criteria（10 条 success criteria 是 plan-phase 验收基线）
- `.planning/STATE.md` — v3.0 milestone 进度

### 前置阶段上下文（必读，避免重复决策）

- `.planning/phases/29-v3-worker/29-CONTEXT.md` §D-04..D-08 / D-11..D-12 — Mutagen agent / mergerfs / tmux 镜像侧版本与挂载参数；本阶段 `mount_merge.go` 必须与 D-11 完全一致
- `.planning/phases/29-v3-worker/29-CONTEXT.md` §D-12 — mergerfs 2 路 branch 拓扑 + `CLOUD_CLAUDE_MERGERFS_BRANCHES` 扩展点
- `.planning/phases/29-v3-worker/29-CONTEXT.md` §D-26..D-27 — `image.lock` 字段是 Phase 30 Entry API 的上游
- `.planning/phases/30-entry-api/30-CONTEXT.md` §D-03/D-05/D-06/D-07 — `AuthResponse` 字段语义（本阶段读取的来源）
- `.planning/phases/30-entry-api/30-CONTEXT.md` §D-09 — `HostActionRequest.ClaudeAccountID` 字段（本阶段不直接消费，但 OAuth 检查依赖 `AuthResponse.ClaudeAccountID`）

### 研究基线

- `.planning/research/SUMMARY.md` §3 / §5 / §7 — v3.0 需求、TOP10 pitfalls、Out of Scope
- `.planning/research/STACK.md` — Mutagen v0.18.1 / mergerfs 2.41.1 / sshfs 版本与理由
- `.planning/research/PITFALLS.md` C1 / C2 / C3 / C4 / C5 / C8 / M5 / M13 — 本阶段防御重点
- `.planning/research/FEATURES.md` §三层文件映射架构 — 设计参考
- `.planning/research/ARCHITECTURE.md` §cloud-claude 端 / §host-agent 边界 — 本阶段不扩展 host-agent endpoint，全部走 SSH

### 既有代码（直接改造对象）

- `internal/cloudclaude/mount.go` — 拆分目标，v2.0 单层 sshfs 实现的剥离起点
- `internal/cloudclaude/ssh.go` — `ConnectAndRunClaude` / `runClaude` / `sshConnect`；本阶段新增 V3 入口（D-30）
- `internal/cloudclaude/entry.go` — `AuthResponse` 字段（Phase 30 已扩展）
- `internal/cloudclaude/config.go` — `--mount-mode` flag 的 viper 绑定参考
- `cmd/cloud-claude/main.go` — cobra root command + flag 注册位置
- `internal/cloudclaude/mount_test.go` — v2.0 mount 测试，本阶段拆分时迁移到对应文件

### 二进制资源（本阶段引入）

- `internal/cloudclaude/mutagen_bin/{darwin_amd64,darwin_arm64,linux_amd64,linux_arm64}/mutagen` — go:embed 资源
- `scripts/fetch-mutagen-bins.sh` — 拉取脚本（新增）
- `~/.cloud-claude/bin/mutagen` — 运行时 extract 路径
- `~/.cloud-claude/mutagen/` — `MUTAGEN_DATA_DIRECTORY`
- `~/.cloud-claude/last-session.json` — 降级历史 + conflict count（Phase 34 doctor 第一屏读取）

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `internal/cloudclaude/mount.go` 现有 `mountWorkspace` / `waitForMount` / `fusermountCleanup` / `cleanupStaleFUSE` / `rmdirChain` / `sshRun` / `shellQuote` 函数全部可复用：剥离到 `mount_sshfs.go` 后是 sshfs 分支的完整实现
- `internal/cloudclaude/ssh.go:sshConnect` 工厂函数可被 `MountConfig` 复用（建立 conn-A / conn-B）
- `internal/cloudclaude/entry.go:AuthResponse` Phase 30 已扩展 4 个 v3 字段，本阶段直接读取
- `internal/cloudclaude/envcheck.go` 已有 macOS / Linux 检测模式（如 `runtime.GOOS == "darwin"`），APFS 检测可借鉴位置
- `internal/cloudclaude/ssh_doctor.go` 已实现一个 doctor 子命令骨架（v2.0 ssh 维度），Phase 34 在其上扩展五维度；本阶段 `errcodes` 包必须与之兼容

### Established Patterns

- **错误返回**：v2.0 mount 错误使用 `fmt.Errorf("...失败: %w", err)` + 中文短描述；本阶段升级为 `errcodes.Format(code, ...) + fmt.Errorf` 包装，保持 `errors.As` / `errors.Is` 兼容
- **SSH session 生命周期**：`session.Close()` + 等待 `sftpDone` channel + cleanup 远端挂载点的 LIFO 模式（mount.go 已固化），本阶段三层 cleanup 必须按 mergerfs → sshfs / mutagen → connections 的反向顺序
- **超时控制**：v2.0 用 `time.NewTimer` + `time.NewTicker` 轮询；本阶段升级到 `context.WithTimeout` + errgroup（更易于状态机超时与降级）
- **shellescape**：远程命令构造统一用 `shellescape.QuoteCommand`（v2.0 ssh.go 已用），本阶段 mergerfs / OAuth 检查 / mutagen 远程命令延续

### Integration Points

- Phase 30 D-03..D-07 `AuthResponse` 是 mount 路径的 capability source（D-29 消费）
- Phase 29 D-04..D-12 镜像侧 mergerfs / mutagen-agent / 元数据文件是本阶段必须信任的契约
- Phase 32 将注入 KeepAlive 配置到 `MountConfig`（D-31 预留接口）
- Phase 33 的 Docker named volume 创建对本阶段透明（mount 不感知 volume 是否存在，只挂 `/workspace-hot` `/workspace-cold` 路径）
- Phase 34 doctor / explain 复用 `errcodes/codes.go` 注册表 + `last-session.json` schema（D-32 预留接口）
- 现有 `internal/cloudclaude/ssh_doctor.go` 已落 `cloud-claude ssh doctor` 子命令（quick-task 260417-0w4），Phase 34 doctor 在其上扩展五维度（不在本阶段动）

</code_context>

<specifics>
## Specific Ideas

- 用户对 v3.0 的核心承诺是「日常开发主战场」——本阶段的「禁止静默降级」与「错误码 + 中文 next_action」是这条承诺的兑现机制；planner 在 Plan 切分时应把 D-15 / D-16 / D-19 / D-21 视为最高优先级 task，禁止简化
- ROADMAP §Phase 31 Success Criteria 第 4 条 `强 kill 容器内 mutagen-agent 后 ≤2s 内 cloud-claude stderr 中文输出降级到 sshfs-only` 是 plan-checker 验收锚点；测试用例必须真实 kill mutagen-agent（不能 mock），可用 `docker exec <ctr> pkill -9 mutagen-agent` 触发
- ROADMAP §Phase 31 Success Criteria 第 6 条 `alpha 空 + beta 非空场景下 CLI 必须中止并输出 MOUNT_MUTAGEN_SAFETY_GUARD，**不允许执行 sync**` 与 D-13 完全对齐；测试用例必须断言 Mutagen 未真正创建 sync session（`mutagen sync list` 应为空）
- 用户已在 Phase 29 / 30 显式选择「单 volume 命名」「Entry API 加字段（不另加 endpoint）」「不扩展 host-agent」三条路径；本阶段 D-29 / D-32 严格延续，**不**通过 host-agent 探测能力
- v3.0 PROJECT.md 强调「零增量特权」——本阶段所有挂载操作（mergerfs / sshfs）依然依赖 v2.0 已开放的 `--cap-add SYS_ADMIN` + `--device /dev/fuse` + `apparmor=unconfined`，禁止新增 `--privileged` 或新 capability
- macOS 真机 APFS case-insensitive 是已知风险（PITFALLS M5 是 BLOCKER），D-09 的检测必须在 cloud-claude 启动早期完成（早于 Mutagen sync create），避免 case-sensitive 假设失败后 mutagen sync 已经创建产生脏数据

</specifics>

<deferred>
## Deferred Ideas

### 阶段内确认但不交付的 follow-up

- **`--mutagen-force` flag 覆盖 50MB 白名单**：D-11 默认拒绝；高级用户场景留给 v3.1（需配套 `cloud-claude sync resume` 命令）
- **`cloud-claude sync resolve <pattern>` 自动解决冲突**：D-28 仅实现 `sync conflicts` 列表查看；自动解决需谨慎（OOS-A2 不暴露 5 种 mode），留 v3.1
- **「下次回车前 prompt 上方插入」严格语义**：REQ-F1-E 字面要求 PTY 拦截渲染，本阶段以「启动 banner 之后立即输出」近似实现；完整 PTY 拦截留 Phase 34 doctor 或 v3.1
- **Mutagen daemon 退出时 GC**：D-05 选定 daemon 长期复用不停；GC（最后一个 cloud-claude 退出后 30min 停 daemon）留 v3.1（涉及跨进程 ref count，复杂）
- **错误码 i18n（英文版本）**：本阶段中文硬编码；i18n 框架留 v3.1（OOS-A 未明确禁止但优先级低）
- **`cloud-claude doctor mount` 维度的真实实现**：本阶段只搭骨架（errcodes + last-session.json + MutagenHealthCheck 接口），完整实现在 Phase 34
- **arm64 真机集成测试**：本阶段 4 平台 embed，但 CI 真机以 amd64 为主线（与 Phase 29 D-37 一致），arm64 集成留 Phase 35 / v3.1
- **mergerfs 3 路 branch（含本地覆盖层）**：Phase 29 D-12 已锁 2 路 + 环境变量扩展点；本阶段读 `CLOUD_CLAUDE_MERGERFS_BRANCHES` 但默认 2 路，3 路启用留 v3.1

### Reviewed Todos (not folded)

无（`todo match-phase 31` 返回 0 条匹配）。

</deferred>

---

*Phase: 31-cli*
*Context gathered: 2026-04-18*
*Mode: --auto（所有 gray area 自动选定推荐项）*
