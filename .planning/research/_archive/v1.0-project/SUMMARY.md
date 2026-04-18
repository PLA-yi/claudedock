# Project Research Summary

**Project:** Cloud CLI Proxy（v2.0 `cloud-claude` 透明远程 CLI 里程碑）
**Domain:** 用户侧 Go 单二进制 + SSH 数据面 + 本地目录与远端容器实时映射，叠加既有 SSH Proxy 与受管容器网络
**Researched:** 2026-04-15
**Confidence:** MEDIUM–HIGH（栈与架构与仓库现状可对齐；目录映射与 FUSE/Mutagen 组合需在目标 Linux 宿主上集成验证）

## Executive Summary

本里程碑交付的是「透明替代本机 `claude`」的远程 CLI：用户通过单一 `cloud-claude` 二进制，在既有 **SSH Proxy → 容器 OpenSSH** 链路上完成 **argv 透传、PTY、SIGWINCH、信号与退出码** 语义，并把本机当前工程目录映射到容器内约定工作区，使 **Claude Code** 在受控出口与全隧道模型下运行。业界同类能力（Codespaces CLI、VS Code Remote SSH、`gp` 等）表明：**表功能**是参数与终端语义一致、可脚本化配置与稳定工作区映射；**差异化**应绑定本项目已有承诺：受控出口 IP、单宿主可私有部署、与受管容器生命周期一致。

研究推荐的实现路径是：**以 SSH `session` 为唯一数据面**（与「SSH Proxy 默认零改造」一致），在客户端用 `golang.org/x/crypto/ssh` 与现有 `internal/sshproxy` **统一 `x/crypto` 版本线**；目录映射主路径为 **容器内 `sshfs -o slave` + 本机 SFTP**（与 `tools/cloud-dev` 已验证模式一致），**Mutagen** 作为性能或冲突场景下的备选，**WebSocket 自定义隧道**仅在网络强约束下再评审。构建顺序上应先 **镜像/FUSE 与 Worker 参数硬化**，再 **单 session `exec` 冒烟**，最后 **双 session（映射 + claude）** 与 TTY 专项。

主要风险集中在：**`--network=none` 下不可假设容器能自连用户机或随意开第二条 TCP**（映射管道必须在设计阶段固定）；**FUSE + AppArmor/seccomp** 导致「加了 `/dev/fuse` 仍失败」；**sshfs 与既有 `/workspace` bind 混用**带来的 UID 与双写；**SSH 多路复用或长驻同步**与会话上限、断线僵死；以及 **TTY/信号** 在 `x/crypto/ssh` 与跨平台上必须由调用方完整接好。缓解策略是：Linux 宿主 + 当前 nftables/sing-box 矩阵上验证；镜像与挂载选项契约化；单一写路径或互斥 mountpoint；连接与同步状态机文档化；映射与出口探测联合回归。

## Key Findings

### Recommended Stack

v2.0 增量栈以 **Go 单二进制** 为中心，与仓库 `go` directive 对齐（建议 ≥ 1.25，并与 `golang.org/x/crypto` 声明一致）。SSH 客户端侧统一使用 **`golang.org/x/crypto/ssh`**（建议 **v0.50.0** 线，与全仓 `sshproxy` 同版本升级），配套 **`golang.org/x/term`**（**v0.42.0**）处理 **TTY** 尺寸与 raw 模式。CLI 面推荐 **`github.com/spf13/cobra` v1.10.2**，配置可选 **`github.com/spf13/viper` v1.21.0** + **`gopkg.in/yaml.v3` v3.0.1** 对应 `~/.cloud-claude/`。目录映射若走 **SFTP/sshfs**，使用 **`github.com/pkg/sftp` v1.13.10**（勿上 **v2 alpha**）。**WebSocket** 仅在为穿透 **443/WSS** 必要时引入 **`github.com/coder/websocket`**，并避免已弃用的 `x/net/websocket` 与 `nhooyr.io/websocket`。详见 `.planning/research/STACK.md`。

**Core technologies:**

- **Go + `golang.org/x/crypto/ssh`：** 与现有 `internal/sshproxy` 同栈，避免两套 SSH 行为分叉。
- **`golang.org/x/term`：** **PTY**、**SIGWINCH**、raw 与 `ReadPassword` 的标准组合。
- **`github.com/pkg/sftp`：** **sshfs slave** 数据路径与 **SFTP** 语义，与 **OpenSSH** 子系统对齐。
- **`cobra` /（可选）`viper`：** 子命令、`init`、与 `~/.cloud-claude/config.yaml` 约定一致。

### Expected Features

用户对「透明转发 `claude`」的默认预期包括：**argv 原样透传**、**TTY/窗口/信号/退出码** 与本地一致、**可重复的配置与凭据**、**本机 cwd 与远端工作区稳定对应**、失败时可行动的错误信息、与既有 **SSH** / 后台路径共存，以及私有部署下网关可配置。差异化应强调：**透明命令替换**、**出口与防泄漏叙事可验证**、**单一 Go 二进制与私有部署**、与受管容器生命周期深度集成、目录映射后端可演进。**应推迟或警惕的**包括：默认同步整个 `$HOME`、CLI 内嵌重图形、静默降级为本地 `claude`、多宿主调度可见化等。详见 `.planning/research/FEATURES.md`。

**Must have (table stakes):**

- **argv 透传 + TTY/信号/窗口/退出码** — 否则「透明替代」不成立。
- **init 与 `~/.cloud-claude/` 配置** — 可安装、可自动化、可迁移。
- **当前目录到远端工作区映射（一种可交付方案）** — 先可用、可解释，再谈极致性能。
- **清晰错误与可观测性** — 与平台错误语义对齐，避免 silent hang。

**Should have (competitive):**

- **出口 IP 与泄漏约束可证明** — 与现有一键测试、tun/**nftables** 模型联动。
- **与受管镜像/到期策略一致** — 组织级叙事弱于 Codespaces，但部署形态更可控。

**Defer（v2.x / 更后）：**

- **端口转发 / 本地↔远端单路径拷贝**（P2，验证后再做）。
- **多映射后端切换（Mutagen 等）**（触发后再做）。
- **会话复用、连接池**（延迟成为主诉时）。

### Architecture Approach

v2.0 **不**改变控制面、**host-agent**、Docker 创建、**ContainerProxyProvider** 与后台前端的主线；新增 **用户侧 `cloud-claude`** 与 **目录映射策略**。数据流为：用户机 **`cloud-claude`** → **TCP SSH** 至 **`internal/sshproxy`**（**RepoResolver** 解析 **short_id** → 容器）→ 容器 **OpenSSH**。**Pattern 1（推荐）：** 同一 SSH 连接上多 **session**——其一跑容器内 **sshfs slave**，本机侧 **SFTP**；其二 **RequestPty** 或 **exec** 跑 **`claude`**。**Pattern 2（可选）：** **Mutagen** 与用户机/宿主机目录同步，与 **Proxy** 正交。**反模式：** 在 **SSH Proxy** 内解析 **exec** 注入挂载；**sshfs** 与 **Mutagen** 同目录双写；未经评审的第二数据面。详见 `.planning/research/ARCHITECTURE.md`。

**Major components:**

1. **`cloud-claude`（`cmd/cloud-claude`）** — 配置、SSH 客户端、映射编排、**TTY**/信号/退出码、**argv** 透传。
2. **`internal/sshproxy`** — 每客户端 **session** 独立 **`ssh.Dial`** 至容器，**pty-req** / **exec** / **window-change** / **exit-status** 透明转发；默认无需改语义。
3. **`internal/runtime/tasks/worker.go`** — **`docker create`**、**`-v` homeDir**、**`--device /dev/fuse`** 等；与镜像内 **fuse/sshfs** 对齐。
4. **目录映射（sshfs 路径）** — 客户端 **SFTP** + 容器 **sshfs**；**Mutagen** 为平行侧车进程。

### Critical Pitfalls

1. **`--network=none` 下错误连通假设** — 不可假设容器能直连用户机或随意第二条 **TCP**；映射管道必须在设计阶段固定（仅 **SSH** 子系统、已存在连接、**host-agent** 显式通道等）。**避免：** 目录映射方案选型评审先于编码。
2. **FUSE/sshfs 与 Docker 安全模块** — **`/dev/fuse` + `CAP_SYS_ADMIN`** 仍可能被 **AppArmor/seccomp** 拦截。**避免：** 在目标 Linux 发行版上做最小复现矩阵，契约写入镜像与 **CI**。
3. **sshfs 与 `/workspace` bind 的 UID/双写** — 权限位、可执行位、属主错乱。**避免：** 单一写路径或互斥 **mountpoint**；固定挂载选项并与运行用户对齐；自动化权限与 **git** 用例。
4. **SSH 多路复用与会话上限** — **ControlMaster** 僵死、**MaxSessions**、同步与 **TTY** 同命。**避免：** 明确共享 **TCP** 或独立连接策略；**keepalive**、陈旧 **socket** 清理；服务端 **MaxSessions** 评估。
5. **TTY/信号/退出码缺口** — **`SIGWINCH`**、raw 恢复、无 **PTY** 时信号语义。**避免：** **`WindowChange` + defer `Restore`**；跨平台（如 Windows）降级；与 **Proxy** 联调专项。

（其余：**同步一致性幻觉**、**sing-box**/**nftables** 与「文件平面」混淆、**macOS** 与 **Linux** 验证矩阵——见 **PITFALLS.md** 全文。）

## Implications for Roadmap

基于依赖关系与仓库证据，建议按以下阶段组织工作（可与正式 **roadmap** 编号对齐或拆分合并）：

### Phase 1：受管镜像与容器侧 FUSE 硬化
**Rationale：** **FUSE/sshfs** 在目标宿主上不可用则后续全废；与 **Pitfall 2** 同源。  
**Delivers：** 镜像内 **fuse/sshfs** 与运行契约；**Worker** 参数与文档一致；最小容器内挂载冒烟。  
**Addresses：** **FEATURES** 中「容器侧 **FUSE/sshfs** 前置条件」。  
**Avoids：** 仅在 **Docker Desktop** 上「看起来能挂载」、生产 **Linux** 翻车（**PITFALLS** 技术债表）。

### Phase 2：`cloud-claude` MVP — 单 session 透传
**Rationale：** 先验证 **SSH**、**Resolver** 认证与 **exec/PTY** 闭环，再叠映射复杂度（**ARCHITECTURE** 建议构建顺序 2）。  
**Delivers：** **`cmd/cloud-claude`**、**init**、配置路径、到 **Proxy** 的 **SSH**、单 **session** 在容器既有 **`/workspace`** 上跑 **`claude`**（冒烟）。  
**Addresses：** **argv**、基础 **TTY**/**退出码**、**P1** 配置。  
**Avoids：** 未验证透传就上双 **session**（排障面过大）。

### Phase 3：双 session 目录映射（sshfs slave + SFTP）
**Rationale：** 与 **Pattern 1**、**tools/cloud-dev** 证据一致；**SSH Proxy** 仍保持协议无关（**ARCHITECTURE** 步骤 3）。  
**Delivers：** **Session A** 映射 + **Session B** **`cd` mountpoint && claude**；清理与失败路径。  
**Addresses：** **cwd ↔ 远端工作区** **P1**、**MVP Launch With** 映射条目。  
**Avoids：** **Pitfall 1**（映射走未设计的第二条外连）、**Pitfall 3**（与 **bind** 双写——需在设计里定 **mountpoint** 与写模型）。

### Phase 4：终端与信号专项 + 错误与可观测性
**Rationale：** **表功能**中 **HIGH** 复杂度项集中在此；与 **Pitfall 4/6** 对应。  
**Delivers：** **SIGWINCH**、信号转发策略、非交互/交互分支、退出码映射、结构化日志与对用户可读错误。  
**Addresses：** **FEATURES** **P1** 的 **TTY** 与「清晰失败路径」。  
**Avoids：** 异常路径未 **Restore** **TTY**；**mux** 无清理策略。

### Phase 5：合规叙事与联合回归（映射 + 出口）
**Rationale：** 差异化依赖平台已有隧道与测试能力；变更 **nftables/sing-box** 时必须同时验映射（**Pitfall 7**）。  
**Delivers：** 文档与 **CLI** 输出中与「受控出口」一致；回归清单：**出口探测** + **映射通道** 同测。  
**Addresses：** **FEATURES** **P1**「出口/合规叙事与测试联动」。  
**Avoids：** 仅验证出口 **IP** 却破坏文件平面。

### Phase 6（可选）：P2 增强 — 端口转发、Mutagen、连接复用
**Rationale：** **FEATURES** 列为验证后增强；**ARCHITECTURE** 将 **Proxy** 连接池标为非 **v2** 必选。  
**Delivers：** 按触发条件择一或多项交付。  
**Addresses：** **P2** 功能与性能投诉。  
**Avoids：** **Pitfall 5**（无状态机的双向同步）、过早引入 **WebSocket** 自定义栈。

### Phase Ordering Rationale

- **先容器与镜像、再单 session、再双 session**：符合依赖与 **ARCHITECTURE**「建议构建顺序」。  
- **映射与 TTY 分拆**：降低并行排障维度；**合规联合回归**独立成阶段，防止网络变更与映射脱钩。  
- **Mutagen/WebSocket/端口转发** 后置：与 **FEATURES** **P2** 与 **STACK** 变体路径一致。

### Research Flags

Phases likely needing deeper research during planning:

- **Phase 3（双 session 映射）：** **`--network=none` 与数据管道**、**Mutagen** 若并行评估时的拓扑与生命周期（**ARCHITECTURE** **MEDIUM** 注记）。
- **Phase 5（网络联合回归）：** 与 **RoutingProvider**/**nftables** 变更同发的探测设计（**PITFALLS**）。

Phases with standard patterns (skip extra research-phase if scope unchanged):

- **Phase 2：** **`golang.org/x/crypto/ssh`** **Session**/**PTY** 模式文档与仓库 **sshproxy** 已有范式。
- **Phase 4：** **SIGWINCH** + **`x/term`** 为常见组合；重点在联调而非理论空白。

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | **pkg.go.dev** 版本与仓库模块可对齐；**sshfs/Mutagen** 组合需在集成阶段实测（研究已标明）。 |
| Features | MEDIUM | 竞品为模式级对照；与 **Claude Code** 集成的细节需集成测试（研究已标明）。 |
| Architecture | HIGH | 与 **`sshproxy`**、**worker**、**tools/cloud-dev** 可逐条对照；**Mutagen** 拓扑为 **MEDIUM**。 |
| Pitfalls | MEDIUM | 约束与项目 **PROJECT** 一致，部分为场景归纳，需 **E2E** 验证闭环。 |

**Overall confidence:** MEDIUM–HIGH

### Gaps to Address

- **`golang.org/x/crypto` 全仓统一升级：** 当前 `go.mod` 可能与 **STACK** 建议版本不一致，客户端开发前需 **`go mod tidy`** 与回归 **SSH** 行为。
- **Mutagen vs sshfs 二选一或互斥策略：** 需在规划中写清默认路径与切换触发条件，避免双写（**PITFALLS**）。
- **跨平台：** **Windows** 无 **SIGWINCH** 的降级；**macOS** 开发 vs **Linux** 生产验证矩阵（**PITFALLS 8**）。
- **性能：** 大仓 **`node_modules`** 与 **sshfs** 小文件 **IO**（**PITFALLS Performance**）——需 **ignore** 策略与文档化 **SLA**。

## Sources

### Primary（HIGH confidence）

- **pkg.go.dev：** `golang.org/x/crypto`、`golang.org/x/term`、`github.com/spf13/cobra`、`github.com/spf13/viper`、`gopkg.in/yaml.v3`、`github.com/pkg/sftp`、`github.com/coder/websocket`、`github.com/urfave/cli/v3` — 版本与依赖关系  
- **GitHub Docs：** [Using GitHub Codespaces with GitHub CLI](https://docs.github.com/en/codespaces/developing-in-a-codespace/using-github-codespaces-with-github-cli)  
- **GitHub CLI Manual：** [gh codespace](https://cli.github.com/manual/gh_codespace)  
- **Visual Studio Code Docs：** [Remote Development using SSH](https://code.visualstudio.com/docs/remote/ssh)  
- **Gitpod Docs：** [Gitpod Workspace CLI (`gp`)](https://www.gitpod.io/docs/configure/workspaces/gitpod-cli)  
- **仓库内代码：** `internal/sshproxy/proxy.go`、`internal/runtime/tasks/worker.go`、`tools/cloud-dev/main.go`  
- **`.planning/PROJECT.md`：** v2.0 目标与约束  

### Secondary（MEDIUM confidence）

- **Mutagen：** [GitHub Releases v0.18.1](https://github.com/mutagen-io/mutagen/releases/tag/v0.18.1)（示例版本）  
- **Docker/FUSE 社区讨论：** 如 `docker/for-linux` **issue** 等（挂载与安全模块）  
- **SSH multiplexing：** 社区文章与 **ServerFault**（**ControlMaster**、**MaxSessions**）  

### Tertiary（LOW confidence / 待验证）

- 具体宿主 **kernel**/**moby** 版本与 **AppArmor** 组合下的最小权限挂载参数 — 需在目标环境实测  

---
*Research completed: 2026-04-15*  
*Ready for roadmap: yes*
