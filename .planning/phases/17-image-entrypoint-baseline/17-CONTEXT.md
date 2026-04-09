# Phase 17: 镜像与 Entrypoint 基线 - Context

**Gathered:** 2026-04-09
**Status:** Ready for planning

<domain>
## Phase Boundary

交付 claude-shell 专用的 Docker 镜像和 entrypoint 编排脚本。镜像包含 sing-box 二进制和基础开发工具，Claude Code 通过官方 curl 安装脚本（Bun standalone）预装，entrypoint 按"网络配置 → 指纹伪造 → 反检测 → Claude Code"顺序编排，各步骤失败时输出明确错误。DISABLE_AUTOUPDATER=1 阻止 Claude Code 自动更新。

本阶段不涉及网络隔离实现（Phase 18）、CLI 骨架（Phase 19）或指纹伪造逻辑（Phase 21），但 entrypoint 需预留这些步骤的调用点。

</domain>

<decisions>
## Implementation Decisions

### 镜像基线与工具选择
- **D-01:** 基础镜像使用 Ubuntu 24.04，与现有 managed-user 镜像基线一致，保证 glibc 兼容性和包可用性
- **D-02:** 工具集精简为最小必要集：curl、git、jq、procps、iproute2、ca-certificates、bash、sudo、nftables。不包含 SSH Server、KasmVNC、Chromium 等 GUI 组件
- **D-03:** sing-box 二进制从 GitHub release 下载安装，版本与现网对齐（1.13.x 系列），安装方式参考 `deploy/docker/sing-box-gateway/Dockerfile` 的模式

### Claude Code 安装策略
- **D-04:** Claude Code 在镜像构建时通过官方 curl 安装脚本预装（Bun standalone），不依赖 npm 或 spoof.js
- **D-05:** 不锁定 Claude Code 具体版本，使用官方安装脚本获取当时最新版本，符合"使用官方安装的最新版本"要求
- **D-06:** 运行时通过 DISABLE_AUTOUPDATER=1 环境变量阻止 Claude Code 自动更新

### Entrypoint 编排模式
- **D-07:** entrypoint 使用 Shell 脚本（bash），`set -euo pipefail` 快速失败模式
- **D-08:** 各阶段拆为独立函数：`setup_network()`、`setup_fingerprint()`、`setup_anti_detect()`、`start_claude()`，按序调用
- **D-09:** 每个步骤失败时输出明确的错误描述（含步骤名称和失败原因），使用不同的退出码区分失败阶段
- **D-10:** 最终以 `exec` 启动 Claude Code 作为容器 PID 1，保证信号能直接传递到 Claude Code 进程
- **D-11:** Phase 17 中 `setup_network()` 和 `setup_fingerprint()` 为占位函数（echo 提示 + 跳过），实际实现分别在 Phase 18 和 Phase 21

### 镜像位置与项目结构
- **D-12:** 镜像构建文件放在 `claude-shell/docker/` 子目录下，与 `deploy/docker/managed-user/` 完全独立
- **D-13:** 与 BUILD-02 要求一致，claude-shell/ 子目录拥有独立 go.mod，镜像构建不依赖 cloud-cli-proxy 主项目的任何文件
- **D-14:** 复用 sing-box 配置与路由设计理念，但不直接引用 `internal/network/` 代码

### Claude's Discretion
- 容器内工作用户的 UID/GID 策略
- entrypoint 日志格式（纯文本 vs 结构化）
- 镜像层优化策略（合并 RUN 指令的具体方式）

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 容器基础设施需求
- `.planning/REQUIREMENTS.md` — INFRA-01、INFRA-02、INFRA-03 定义了镜像、entrypoint 和自动更新禁用的具体要求

### 架构与网络设计
- `.planning/research/ARCHITECTURE.md` — §10 建议构建顺序：先镜像 entrypoint 再 sing-box 配置模板；§3 bridge + 容器内 sing-box tun 的网络选型
- `.planning/research/FEATURES.md` — §6 Claude Code 遥测与退出环境变量（DISABLE_AUTOUPDATER、DISABLE_TELEMETRY 等）
- `.planning/research/STACK.md` — 技术栈版本基线（Go 1.26.1、sing-box 1.13.x、Ubuntu 24.04）

### 现有镜像参考（设计理念参考，不直接复用）
- `deploy/docker/managed-user/Dockerfile` — 现有受管用户镜像，可参考 sing-box 安装和用户创建模式
- `deploy/docker/sing-box-gateway/Dockerfile` — sing-box 二进制安装的参考实现
- `deploy/docker/sing-box-gateway/entrypoint.sh` — sing-box 启动模式参考

### 项目约束
- `.planning/PROJECT.md` — Key Decisions 表中的网络和镜像决策

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `deploy/docker/sing-box-gateway/Dockerfile`: sing-box 二进制的下载和安装模式（ARG SINGBOX_VERSION + curl + tar + install），可直接复用安装逻辑
- `deploy/docker/managed-user/Dockerfile`: Ubuntu 24.04 基础镜像的用户创建（useradd + sudoers）和 locale 配置模式可参考

### Established Patterns
- sing-box 安装统一使用 GitHub release 下载 + 版本号 ARG，便于升级
- 容器内工作用户使用 UID 1000 + sudoers NOPASSWD 配置
- entrypoint 使用 `/usr/local/bin/entrypoint.sh` 路径

### Integration Points
- Phase 18 将在 `setup_network()` 占位处填入 sing-box tun + nftables 配置逻辑
- Phase 19 的 CLI 将通过 `docker run` 启动此镜像
- Phase 21 将在 `setup_fingerprint()` 和 `setup_anti_detect()` 占位处填入伪造逻辑

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 17-image-entrypoint-baseline*
*Context gathered: 2026-04-09*
