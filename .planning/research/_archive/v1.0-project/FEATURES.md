# Feature Research

**Domain:** 透明替代 `claude` 的远程 CLI（cloud-claude / v2.0 里程碑）  
**Researched:** 2026-04-15  
**Confidence:** MEDIUM（竞品行为以官方文档与公开手册为主；与 Claude Code 具体集成的细节需在实现阶段用集成测试验证）

## Feature Landscape

### Table Stakes (Users Expect These)

对「把本地 `claude` 调用透明转发到远端容器」这一类工具，用户会**默认**具备以下预期；缺失则产品会显得不完整或不可信。

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **参数原样透传** | 用户脚本、`npm`/`pnpm` 包装、`CI` 里写的 `claude ...` 不应因中间层而变形 | MEDIUM | 需处理 `--`、引号、环境变量展开边界；与 shell 包装行为对齐 |
| **TTY / 窗口尺寸 / 信号 / 退出码透传** | 交互式 CLI 的体验与可脚本化行为（Ctrl+C、SIGPIPE、`stty`）必须与本地一致 | HIGH | 远程执行 + 伪终端转发是硬要求；与 SSH 会话模型一致 |
| **可重复的配置与凭据** | 网关地址、租户/用户身份、认证方式需可保存、可迁移、可自动化 | MEDIUM | `init` + 配置文件路径约定（如 `~/.cloud-claude/`）属于行业惯例 |
| **当前工作目录与远端工作区的稳定映射** | 用户期望「在我本机这个 repo 里跑 claude」等价于「在远端同一棵树里跑」 | HIGH | 实时同步或挂载任一方案都有运维与一致性成本；见依赖图 |
| **连接与会话失败时的可读错误** | 认证失败、主机未就绪、出口未绑定、网络中断时需可行动提示 | LOW–MEDIUM | 与平台已有错误码/中文提示能力应对齐 |
| **与既有接入方式共存（SSH / 管理后台）** | 用户可能混用「纯 SSH」与「cloud-claude」；不应要求推翻现有心智 | MEDIUM | 文档与行为上明确：cloud-claude 是叠加路径，而非唯一入口 |
| **私有部署可配置** | 企业/自托管需要改网关/base URL | LOW | PROJECT.md 已列为目标 |

**竞品对照（模式层面）：**

- **GitHub Codespaces CLI（`gh codespace`）**：围绕「列出/创建/停止/SSH/端口/日志/拷贝」提供完整生命周期与辅助命令；强调与 `gh auth` 体系一致的非交互/可脚本体验。[Using GitHub Codespaces with GitHub CLI](https://docs.github.com/en/codespaces/developing-in-a-codespace/using-github-codespaces-with-github-cli)、[gh codespace 手册](https://cli.github.com/manual/gh_codespace)。
- **VS Code Remote - SSH**：打开远程文件夹、远程终端、**端口转发**（含 `LocalForward` 持久化）是桌面端远程开发的事实标准。[Remote Development using SSH](https://code.visualstudio.com/docs/remote/ssh)。
- **Gitpod `gp` CLI**：典型是**工作区内**命令（端口 URL、任务、超时、资源 `top` 等）；与「桌面侧透明替换全局命令」路径不同，但「端口/预览/环境信息」对云开发 CLI 仍是常见表功能。[Gitpod Workspace CLI](https://www.gitpod.io/docs/configure/workspaces/gitpod-cli)。
- **Cursor Remote SSH**：实现层面与 VS Code Remote SSH 同族（远程扩展主机、SSH 传输）；可参考 VS Code 文档中的终端与隧道模型，不宜假设与 Codespaces 完全相同的 CLI 子命令集。

### Differentiators (Competitive Advantage)

与通用远程开发 CLI 相比，本产品的差异化应绑定 **PROJECT.md 的核心价值**：受控出口 IP + 全隧道出网 + 单宿主机可控交付。

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **透明命令替换（`alias claude=cloud-claude`）** | 零迁移成本，脚本与肌肉记忆不改 | MEDIUM | 比「先 ssh 再手动执行」少一步认知负担 |
| **出口 IP 与泄漏约束可证明** | 「代码与请求走哪条出口」是合规与风控刚需；与平台代理测试能力形成闭环 | HIGH | 可与现有一键测试、nftables/tun 模型联动宣传 |
| **单一 Go 二进制、可私有部署** | 企业侧偏好可审计、可内网分发；与「一条 curl 入门」叙事一致 | MEDIUM | 与 `gh` 插件式生态不同，更像专用 shim |
| **与受管容器生命周期深度集成** | 启动/到期/重建策略由平台统一治理，而非用户自管 VM | MEDIUM | 对标 Codespaces 的「组织级策略」但部署形态更小 |
| **目录实时映射方案可演进** | sshfs / Mutagen / 自定义同步可在不影响 `claude`  argv 契约下替换 | HIGH | 差异化在「网络与运维可承受」而非单一技术名词 |

### Anti-Features (Commonly Requested, Often Problematic)

看起来「很爽」但容易把 v2.0 拖进泥潭或损害安全/可支持性的需求：

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **默认双向实时同步整个 `$HOME` 或超大 monorepo** | 「像网盘一样什么都同步」 | 延迟、冲突、CPU/IO、排除规则爆炸；难以解释「为什么慢」 | 默认仅映射当前 repo；文档化 `.cloud-claudeignore`；大仓用显式子目录或阶段性同步 |
| **在 CLI 内嵌「迷你 IDE」或重图形能力** | 一站式 | 与产品「SSH + CLI」边界冲突；维护面陡增 | 继续 SSH；端口转发可选、薄封装 |
| **自动绕过企业代理 / 忽略 TLS 校验** | 临时打通 | 安全与合规雷区；难审计 | 显式配置信任链、系统代理、文档化 CONNECT |
| **静默降级为「本地 claude」** | 离线也能用 | 用户以为走了出口 IP，实际没有 | 失败即失败；可选 `--local-fallback` 且**强提示** |
| **多宿主自动调度对用户可见** | 弹性 | v1 明确单宿主；引入调度与状态同步 | 延后；当前仅单宿主 |
| **用户自选任意远端镜像** | 灵活 | 就绪性、供应链、支持成本 | 管理员受管镜像（与现架构一致） |

## Feature Dependencies

```text
[认证与配置持久化]
    └──requires──> [与网关/API 的安全通信]
                       └──requires──> [主机就绪 / 容器生命周期状态]

[透明执行 claude]
    └──requires──> [远端可执行环境与 Claude Code 可用]
    └──requires──> [argv/TTY/信号/退出码透传]

[本地目录 ↔ 远端 /workspace]
    └──requires──> [会话内稳定路径约定]
    └──requires──> [同步或挂载栈：FUSE/sshfs 或 Mutagen 等]
    └──conflicts──> [「零延迟本地 FS」预期]（需设定合理 SLA 与说明）

[出口合规叙事]
    └──enhances──> [透传执行路径]
    └──requires──> [现有隧道 + 校验 + 测试 API]（平台已具备）

[可选：端口转发 / 本地访问远端 HTTP]
    └──requires──> [长期 SSH 会话或等价隧道]
    └──enhances──> [与 VS Code 用户习惯对齐]
```

### Dependency Notes

- **透传执行 依赖 远端环境就绪：** 仅有 SSH 不够，还需容器内 `claude`/runtime 与 cwd 一致。
- **目录映射 依赖 稳定会话：** 多数同步/挂载方案需要长连接或重连语义，与「一次性 SSH 命令」模型不同。
- **合规/出口故事 增强 透传路径：** 若执行不在绑定出口的主机上，产品承诺不成立；属于强依赖而非附加文案。

## MVP Definition

以下与 **`.planning/PROJECT.md` v2.0** 对齐，作为「最小可验证透明远程 CLI」范围。

### Launch With（v2.0 MVP）

- [ ] **单一二进制 `cloud-claude` + `init` 配置** — 可安装、可指向私有网关；凭据落盘路径清晰。
- [ ] **`claude` 参数透传 + TTY/信号/窗口/退出码** — 无此则「透明替代」不成立。
- [ ] **当前目录到远端工作区映射（一种可交付方案即可）** — 先求可用与可解释，再求极致性能。
- [ ] **容器侧 FUSE/sshfs 或等价前置条件** — 与镜像、`--device /dev/fuse` 等约束一致。
- [ ] **失败路径清晰** — 与平台错误语义对齐，避免 silent hang。

### Add After Validation（v2.x）

- [ ] **端口转发或本地↔远端文件单路径拷贝** — 当用户需要「辅助 HTTP 预览」时再加；触发信号：高频工单/竞品 parity 压力。
- [ ] **多映射后端可切换（Mutagen / 其他）** — 触发信号：大仓延迟或冲突问题显著。
- [ ] **会话复用、连接池** — 触发信号：冷启动时延成为主投诉。

### Future Consideration（v3+ / 其他里程碑）

- [ ] **与 paused `claude-shell` 本地 Docker 模式统一 UX** — 明确不同里程碑边界后再做。
- [ ] **组织级策略 CLI（配额、允许的映射根目录）** — 多租户成熟后。

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| argv + TTY/信号/退出码透传 | HIGH | HIGH | P1 |
| 配置与认证 | HIGH | MEDIUM | P1 |
| 目录映射（可用方案） | HIGH | HIGH | P1 |
| 清晰错误与可观测日志 | MEDIUM | LOW | P1 |
| 出口/合规叙事与测试联动 | HIGH | MEDIUM（平台已有能力） | P1 |
| 端口转发 | MEDIUM | MEDIUM | P2 |
| 映射后端多种可选 | MEDIUM | HIGH | P2 |
| 静默离线降级 | LOW | MEDIUM | P3（默认不做） |

**Priority key:**

- P1: v2.0 必须交付，否则里程碑不成立  
- P2: 验证后增强  
- P3: 明确不做或极后置  

## Competitor Feature Analysis

| Feature | VS Code Remote SSH | GitHub Codespaces CLI | Gitpod `gp` | Our Approach（cloud-claude） |
|---------|-------------------|------------------------|-------------|------------------------------|
| 远程工作区 / 文件夹 | 打开远程文件夹；核心 IDE 能力在远端 | 云侧 devcontainer + `ssh`/`code` 等入口 | 浏览器/桌面打开 workspace；`gp` 偏工作区内 | 非 IDE：**单一命令**转发到容器内 cwd |
| 终端语义 | 远程集成终端 | `gh codespace ssh` | 工作区内 shell | **必须**对齐本地 `claude` 的 TTY/信号 |
| 端口转发 | 内建 Ports / `LocalForward` | `gh codespace ports forward` 等 | `gp url` / `gp ports` | 可选 P2；非 MVP 核心 |
| 生命周期管理 | 无（假设你已有机子） | `create/stop/delete/rebuild` 一等公民 | `gp stop`/timeout 等 | 复用平台主机生命周期；CLI 薄封装即可 |
| 身份与配置 | SSH config、`known_hosts` | `gh auth`、GitHub 身份 | Gitpod 账户体系 | **自有网关 + 本地配置目录** |
| 差异化卖点 | 编辑器体验 | GitHub 生态与 Codespaces SLA | 云工作区 + prebuild | **强制出口路径 + 私有单宿主部署** |

## Sources

- GitHub Docs: [Using GitHub Codespaces with GitHub CLI](https://docs.github.com/en/codespaces/developing-in-a-codespace/using-github-codespaces-with-github-cli)（HIGH：官方）  
- GitHub CLI Manual: [gh codespace](https://cli.github.com/manual/gh_codespace)（HIGH：官方手册）  
- Visual Studio Code Docs: [Remote Development using SSH](https://code.visualstudio.com/docs/remote/ssh)（HIGH：官方）  
- Gitpod Docs: [Gitpod Workspace CLI (`gp`)](https://www.gitpod.io/docs/configure/workspaces/gitpod-cli)（HIGH：官方）  
- 项目内上下文：`.planning/PROJECT.md`（v2.0 目标与约束）  

---
*Feature research for: cloud-claude 透明远程 CLI（v2.0）*  
*Researched: 2026-04-15*
