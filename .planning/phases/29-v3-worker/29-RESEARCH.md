# Phase 29: 受管镜像 v3 + Worker 容器参数扩展 - Research

**Researched:** 2026-04-18
**Domain:** Dockerfile 增量改造（mergerfs/mutagen-agent/tmux/tini）+ Go struct JSON 契约扩展 + 宿主机 AppArmor preflight
**Confidence:** HIGH（所有关键二进制来源与版本已 `[VERIFIED]` 官方 release；AppArmor override 细节已 `[VERIFIED]` moby issue 与 Ubuntu bug tracker）

## Summary

Phase 29 是纯基础设施阶段：不交付任何用户可观察 REQ，只为 Phase 30/31/32/33 提供镜像侧前置条件与 Worker 容器 `--mount type=volume` 契约。研究聚焦三件事：(1) **在哪里拉、怎么装** 四个新组件（mergerfs 2.41.1 .deb / Mutagen v0.18.1 agent bundle / tmux 3.4 / tini 0.19）；(2) **挂载与 entrypoint 参数怎么写** 才能一次性防御 C1/C2/C3/C5/C7；(3) **`VolumeMount` 契约怎么加** 才能保证 v2.0 旧 agent 反序列化不破。

所有关键上游都是 GitHub release 一手链接，并已经过 `[VERIFIED]` 核对：mergerfs 2.41.1 的 ubuntu-noble `.deb` 实际存在且 amd64 / arm64 均有；Mutagen v0.18.1 的 `mutagen_linux_<arch>_v0.18.1.tar.gz` 内部结构已由 WakeMeOps 打包蓝图与 Mutagen 官方文档交叉核实（`mutagen-agents.tar.gz` 在 tarball 顶层）；tmux 3.4-1ubuntu0.1 为 noble main 仓库当前版本；tini 0.19.0-1 在 noble universe 仓库。

**Primary recommendation（面向 planner）:** 全部新增组件均走 Dockerfile 层，entrypoint 只做运行时兜底；mergerfs 的 `.deb` 直接 `dpkg -i` 即可（deb 自带依赖声明，libfuse3 通过 apt 预装满足）；Mutagen agent bundle 从平台 tarball 中只抽 `mutagen-agents.tar.gz` 单文件预放到 `/opt/`，**不要** 在镜像层解压（保持 ~80MB 压缩态即可，Phase 31 cloud-claude 按需 extract）；tini 走 `apt install tini` + `ENTRYPOINT ["/usr/bin/tini", "--", ...]`；BuildKit cache mount + `--no-install-recommends` + 合并 RUN 是达到 ≤ 700MB 的必备组合。

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

以下为 CONTEXT.md §Implementation Decisions D-01..D-30 的口径概要；完整文本以 CONTEXT.md 为准，planner 不得绕过。

**镜像演进路径：**
- D-01 增量改造 `deploy/docker/managed-user/Dockerfile`，不新建 v3 独立镜像目录；KasmVNC/Chromium/Fluxbox/fonts-noto-cjk 保留。
- D-02 Base image 保持 `ubuntu:24.04`；25.04 问题走宿主机 `host-preflight.sh`，不抬 base。
- D-03 BuildKit cache mount + `--no-install-recommends` + 合并 RUN；未压缩硬约束 ≤ 700MB；超标优先裁 Chromium recommends 与 fonts，**不裁** mergerfs/mutagen-agent/tmux v3 基线。

**二进制来源 / 版本：**
- D-04 mergerfs 2.41.1 走 GitHub release 官方 `mergerfs_2.41.1.ubuntu-noble_<arch>.deb`，`dpkg -i` 安装；**禁止 apt 源**（M3）；支持 amd64 + arm64。
- D-05 mutagen-agent v0.18.1 从 `mutagen_linux_<arch>_v0.18.1.tar.gz` 解压后只保留 `mutagen-agents.tar.gz` 预放 `/opt/mutagen-agents.tar.gz`；运行时 extract 由 Phase 31 cloud-claude 触发；支持 amd64 + arm64。
- D-06 tmux 使用 ubuntu:24.04 apt 仓库（3.4 系列）；**entrypoint 断言 `tmux -V` ≥ `3.4`**；3.6a 上限为 Phase 35 的 open follow-up，**本阶段不强制 PPA 或源编译**。
- D-07 写入 `/etc/cloud-claude/mutagen.version` / `mergerfs.version` / `tmux.version` 元数据。
- D-08 libfuse3 用 ubuntu apt `libfuse3-3`/`fuse3` 系列（3.16–3.18 区间），不引入 PPA。

**Entrypoint 改造：**
- D-09 不重写 entrypoint，在 `exec /usr/sbin/sshd -D -e` 之前串行插入 `prepare_fuse` → `prepare_v3_dirs` → `prepare_mutagen_agent` → `prepare_mergerfs`（默认只校验 `mergerfs --version`，**不挂载**） → `exec sshd`。
- D-10 PID 1 改为 `tini`（apt 安装）；`ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]`。
- D-11 mergerfs 挂载参数（由 Phase 31 cloud-claude 下发，镜像与 host-preflight 文档对齐）：`category.create=ff,func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,inodecalc=path-hash`。
- D-12 mergerfs branch 本阶段锁 **2 路** `/workspace-hot=RW:/workspace-cold=NC,RO`；entrypoint 通过 env `CLOUD_CLAUDE_MERGERFS_BRANCHES` 预留 3 路扩展点；Phase 31 若 3 路无需动镜像。
- D-13 新增 `/etc/tmux.conf` 含 `terminal-overrides ",*:RGB"` / `window-size latest` / `aggressive-resize on` / `history-limit 50000`；新增 `/etc/profile.d/cloud-claude.sh` 导出 `CLAUDE_CODE_TMUX_TRUECOLOR=1`。
- D-14 `sshd_config` 追加 `ClientAliveInterval 15` / `ClientAliveCountMax 8` / `MaxSessions 30` / `MaxStartups 60:30:120`；保留 `PasswordAuthentication yes` / `UsePAM no`。
- D-15 容器内**不**启动 systemd / systemd-logind；tini 仅做 PID 1 收割僵尸。

**预建目录与用户：**
- D-16 Dockerfile 预建并 `chown 1000:1000`：`/home/claude`、`/home/claude/.claude`、`/home/claude/.cache/claude`、`/workspace-hot`、`/workspace-cold`、`/workspace`、`/var/lib/claude-persist`。
- D-17 现有 `workspace` 用户（UID 1000 / GID 1000）**不**重命名；`home-dir` 保持 `/workspace`；`/home/claude` 只是命名约定，同属 UID 1000。

**HostActionRequest.Volumes 契约：**
- D-18 新增 `VolumeMount{Name,Target,ReadOnly,Labels}` 与 `HostActionRequest.Volumes []VolumeMount \`json:"volumes,omitempty"\``。
- D-19 `createHost` 遍历 Volumes 追加 `--mount type=volume,src=<Name>,dst=<Target>[,readonly]`；**不**追加 label。
- D-20 本阶段**不**调用 `docker volume create`（留给 Phase 33）；volume 不存在则正常返回 `host_action_failed`。
- D-21 `ClaudeAccountID` 字段**不**在本阶段新增（属 Phase 30）。
- D-22 向后兼容：`omitempty` 保 v2.0 旧客户端反序列化不破。

**host-preflight 与 AppArmor override：**
- D-23 新增独立脚本 `deploy/host-preflight.sh`：检测 Ubuntu 25.04+ → 校验 `/etc/apparmor.d/local/docker-default` 是否含 `capability dac_override,` → 缺失退 1 并打印修复命令；非 25.04 退 0。
  **⚠ 研究发现与此条存在事实冲突，详见 §Conflicts with CONTEXT.md。**
- D-24 `host-preflight.sh` **不嵌入**控制面启动流程；保持独立脚本，由运维手动 / CI / Phase 34 doctor 调用。
- D-25 运维手册新增 AppArmor override 部署章节（内容、`apparmor_parser -r` 刷新、回滚、验证）。

**image.lock 扩展字段：**
- D-26 `image.lock` 追加 `image_version: v3.0.0` / `mergerfs_version: 2.41.1` / `mutagen_agent_version: v0.18.1` / `tmux_version_min: "3.4"` / `supports_mutagen: true` / `supports_mergerfs: true`；现有字段全部保留。
- D-27 image.lock 是 Phase 30 Entry API 扩展的单一上游数据源；本阶段仅写入，不 runtime 读取。

**CI 镜像体积 gate（BASE-04）：**
- D-28 CI workflow 新增 bash + `docker image inspect` step，硬断言 < 700×1024×1024；失败自动打印 `docker history`。
- D-29 失败自动输出 `docker history` 便于排查膨胀层。
- D-30 `build-managed-image.sh` **不**嵌入体积检查（本地开发允许超标）。

### Claude's Discretion

以下由 planner / executor 按实现便利性决定：

- tini 二进制是否从 apt 安装或 COPY 静态二进制（**本研究推荐 apt**：体积更可控、universe 仓库稳定 0.19.0）。
- mergerfs `.deb` 下载的 checksum 校验方式（**本研究推荐 `sha256sum` 硬编码**：mergerfs release 不发布 SHA256SUMS 文件，详见 §Sources R-1）。
- mutagen-agents tarball 的解压位置（CONTEXT.md D-05/D-09.3 已给定 `/opt/mutagen-agents.tar.gz` 预放 + entrypoint `prepare_mutagen_agent` 解压到 `/usr/local/libexec/mutagen/agents/`）。
- Dockerfile RUN 指令的合并粒度（层数与 cache 命中的权衡）。
- CI gate 的具体文案与报错格式（保持 `::error::` 前缀即可）。
- host-preflight.sh 在非 Linux 宿主机（macOS / WSL）上的行为（**本研究推荐** 直接退出 0 + 中文提示"非 Linux 宿主机无需检查"）。

### Deferred Ideas (OUT OF SCOPE)

以下属 CONTEXT.md `<deferred>` 范围，planner 不得把任何相关任务划入 Phase 29：

- tmux 3.6a 升级路径（本阶段锁 ≥ 3.4，Phase 35 验收后回流）。
- image.lock 切分为 `image-capabilities.yaml`（Phase 30 需求决定）。
- `host-preflight.sh --apply` 自动修复模式。
- mergerfs 3 路 branch 落地（通过 env 预留，Phase 31 决策）。
- arm64 真机验收（推迟到 Phase 35 或 v3.1）。
- 任何 `docker volume create` / 生命周期管理（Phase 33）。
- 任何 `host-agent` endpoint 扩展（沿用 `/agent/host/action`）。
- 任何 user-facing REQ-F* 行为交付（Phase 30–33）。

</user_constraints>

<phase_requirements>
## Phase Requirements

本阶段**无** user-facing REQ-F*；唯一直接承担的硬基线是 `BASE-04`，其它 REQ 均以"镜像侧前置条件"的形式交付。

| ID | Description（from REQUIREMENTS.md） | Research Support |
|----|-------------------------------------|------------------|
| BASE-04 | v3 受管镜像未压缩 ≤ 700MB（CI gate） | §7 体积估算与裁剪候选；§CI Gate 脚本骨架；`docker image inspect --format='{{.Size}}'` 断言 |
| F1 (prereq for Phase 31) | 三层文件映射，容器内单一 `/workspace` 视图 | §1 mergerfs 安装；§10 branch 语法与 env 覆盖；§D-11 mount 参数清单 |
| F1 (prereq for Phase 31) | Mutagen 热同步容器端基础 | §2 mutagen-agent bundle 分发；`/etc/cloud-claude/mutagen.version` 元数据 |
| F4 (prereq for Phase 32) | tmux 默认包装基础 | §4 tmux 3.4 配置可行性；§C7 tini PID 1 守护 |
| F7 (prereq for Phase 33) | Claude Code 持久化 volume 承载 | §D-16 预建目录 + chown 1000:1000；§8 `--mount type=volume` 契约 |
| F3-A (服务端基线) | sshd_config `ClientAliveInterval 15` / `ClientAliveCountMax 8` | §D-14 sshd_config diff |
| C1/C2/C3/C5/C6/C7 防御 | 一次性落地 | §C-系列 pitfall 映射到 Dockerfile / entrypoint / sshd_config 的具体落点 |

</phase_requirements>

## Project Constraints (from .cursor/skills/*/SKILL.md)

项目 `.cursor/rules/` 目录**不存在**；`.cursor/skills/` 下为 GSD 工作流 skills，仅约束"如何规划/执行"，不引入代码层约束。

从 CLAUDE.md（workspace rules）提取的硬约束，planner 必须在计划中体现：

- **语言**：所有面向用户的说明、计划文字、commit message、日志 echo 必须使用中文；命令/URL/代码标识符保留英文。
- **隐私**：禁止写入任何本机绝对路径、真实凭据；示例凭据用占位符（`your-secret-here`）；路径用项目根相对路径。
- **工作流**：代码改动必须通过 GSD 工作流进入（本阶段走 `/gsd-plan-phase` → `/gsd-execute-phase`）。
- **网络安全底线**：v1 约束的"不新增 `--privileged`、复用 v2.0 已开放的 FUSE/SYS_ADMIN/AppArmor unconfined 通道"继续生效；本阶段 Dockerfile / Worker 改动**不得**提升容器特权。

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| mergerfs / mutagen-agent / tmux / tini 二进制分发 | Docker 镜像构建（Dockerfile） | — | 镜像层是交付稳定性边界；运行时安装会破坏 immutability 与体积可控 |
| 容器内目录预建 + chown 1000:1000 | Docker 镜像构建（Dockerfile） | 容器 entrypoint（兜底二次 chown） | Dockerfile 是 named volume 首次挂载继承权限的唯一来源（M17）；entrypoint 做一次兜底防旧镜像 volume |
| mergerfs 实际 mount 调用 | **不在本阶段**（Phase 31 cloud-claude） | — | CONTEXT.md D-09.4 明确镜像 entrypoint 只校验 `mergerfs --version`，不 mount |
| sshd 参数基线 | Docker 镜像构建（`sshd_config` COPY） | 容器 entrypoint（host key 生成） | 静态配置属 Dockerfile；动态 host key 沿用现有 entrypoint |
| 容器内 PID 1 收割僵尸 | Docker 镜像构建（tini ENTRYPOINT） | — | 固定为 `/usr/bin/tini --` wrapper |
| 容器创建参数拼接 | Go 控制面 worker（`internal/runtime/tasks/worker.go`） | Go API 契约（`internal/agentapi/contracts.go`） | 契约在 contracts.go 定型；拼接在 worker.go 消费；host-agent endpoint 沿用 `/agent/host/action` 不扩展 |
| named volume 生命周期（create/rm） | **不在本阶段**（Phase 33） | — | CONTEXT.md D-20 显式划界 |
| 宿主机 AppArmor override 检测 + 修复命令输出 | 独立 bash 脚本（`deploy/host-preflight.sh`） | 运维手册 | 控制面不能 sudo，独立脚本由运维 / CI / Phase 34 doctor 调用 |
| CI 镜像体积 gate | GitHub Actions workflow step（bash + `docker image inspect`） | — | 零第三方依赖；`docker history` 失败时自动输出 |

---

## 1. mergerfs 2.41.1 官方 `.deb` 安装

### Release 资产（已核对）
官方 release 页（`trapexit/mergerfs`）在 2025-11-19 发布 2.41.1，提供以下与本项目相关资产 [VERIFIED: github.com/trapexit/mergerfs/releases/tag/2.41.1]：

| 文件 | 大小 | 架构 | 适配 ubuntu:24.04 |
|------|------|------|---------------------|
| `mergerfs_2.41.1.ubuntu-noble_amd64.deb` | 432 KB | amd64 | ✓（noble = 24.04 codename） |
| `mergerfs_2.41.1.ubuntu-noble_arm64.deb` | 409 KB | arm64 | ✓ |
| `mergerfs-2.41.1-static-linux_amd64.tar.gz` | 1605 KB | amd64 | 备选（fallback） |

**下载 URL 模板（amd64 + arm64）：**

```
https://github.com/trapexit/mergerfs/releases/download/2.41.1/mergerfs_2.41.1.ubuntu-noble_${ARCH}.deb
```

其中 `${ARCH}` ∈ `{amd64, arm64}`，取自 `$(dpkg --print-architecture)`。

### 依赖与安装流程
`.deb` 的 `Depends:` 字段声明 `libfuse3-3` [ASSUMED — 来自 trapexit 通用打包约定，executor 需 `dpkg -I mergerfs_*.deb` 实际校验]。因 Dockerfile 现有 `apt-get install ... fuse3 ...`（line 40）已经把 `libfuse3-3` 作为 fuse3 的依赖拖入，**dpkg -i 可直接安装，无需 --fix-missing**。顺序必须是：**先 apt 装 fuse3 → 再 dpkg -i mergerfs**。

推荐 Dockerfile 片段（与 D-03 BuildKit cache 组合）：

```dockerfile
# mergerfs 2.41.1 静态 .deb（官方 GitHub release，不走 apt）
ARG MERGERFS_VERSION=2.41.1
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    set -eux; \
    ARCH="$(dpkg --print-architecture)"; \
    case "${ARCH}" in \
      amd64|arm64) ;; \
      *) echo "unsupported arch: ${ARCH}"; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/mergerfs.deb \
      "https://github.com/trapexit/mergerfs/releases/download/${MERGERFS_VERSION}/mergerfs_${MERGERFS_VERSION}.ubuntu-noble_${ARCH}.deb"; \
    # SHA256 硬编码（executor 首次构建时 curl 实际 .deb 后 sha256sum 得到，写回此处）
    case "${ARCH}" in \
      amd64) EXPECTED="<sha256-of-amd64-deb-TO-BE-COMPUTED>" ;; \
      arm64) EXPECTED="<sha256-of-arm64-deb-TO-BE-COMPUTED>" ;; \
    esac; \
    echo "${EXPECTED}  /tmp/mergerfs.deb" | sha256sum -c -; \
    dpkg -i /tmp/mergerfs.deb; \
    rm /tmp/mergerfs.deb; \
    mergerfs --version; \
    echo "${MERGERFS_VERSION}" > /etc/cloud-claude/mergerfs.version
```

### Checksum 来源
mergerfs release **不发布 SHA256SUMS 文件**（已核对 2.41.1 资产清单，无 `*.sha256` / `SHA256SUMS` / `*.asc`）[VERIFIED: github.com/trapexit/mergerfs/releases/tag/2.41.1]。

**推荐做法：**
1. Executor 首次构建前执行 `curl -fsSL <URL> | sha256sum` 得到两架构 SHA256，硬编码进 Dockerfile。
2. image.lock 追加 `mergerfs_deb_sha256_amd64` / `mergerfs_deb_sha256_arm64` 字段（本研究建议，但 CONTEXT.md D-26 未列出；属 Claude's Discretion 范围）。
3. 后续升级 mergerfs 版本时在同一 PR 中更新 SHA256。

### Provenance
- URL 模板 [VERIFIED: github.com/trapexit/mergerfs/releases/tag/2.41.1]
- `dpkg -I` 实际 `Depends:` 字段 [ASSUMED — executor 构建时需实测一次，确认 libfuse3 依赖名]

---

## 2. Mutagen v0.18.1 agent bundle 分发

### Release 资产（已核对）
Mutagen v0.18.1（2025-02-24）官方发布以下 Linux 资产 [VERIFIED: github.com/mutagen-io/mutagen/releases/tag/v0.18.1]：

| 文件 | 大小 | 架构 |
|------|------|------|
| `mutagen_linux_amd64_v0.18.1.tar.gz` | 99778 KB | amd64 |
| `mutagen_linux_arm64_v0.18.1.tar.gz` | 99384 KB | arm64 |
| `SHA256SUMS` | 4 KB | — |
| `SHA256SUMS.gpg` | 1 KB | — |

### Tarball 内部结构
每个平台 tarball 顶层**包含两个文件** [VERIFIED: WakeMeOps blueprint `install: - mutagen:/usr/bin/mutagen, - mutagen-agents.tar.gz:/usr/libexec/mutagen-agents.tar.gz`；docs.wakemeops.com/packages/mutagen/]：

```
./mutagen                    # CLI + daemon 二进制，~20MB
./mutagen-agents.tar.gz      # 跨平台 agent bundle（内含各架构 mutagen-agent 二进制），~80MB
```

**本项目只需 `mutagen-agents.tar.gz`**（CLI 二进制由 cloud-claude Phase 31 以 `go:embed` 方式嵌入，镜像里不需要）。

### 已知 SHA256（已核对第三方打包源）
[CITED: docs.wakemeops.com/packages/mutagen/ — WakeMeOps `ops2deb.lock.yml`]：

- `mutagen_linux_amd64_v0.18.1.tar.gz`: `7735286c778cc438418209f24d03a64f3a0151c8065ef0fe079cfaf093af6f8f`
- `mutagen_linux_arm64_v0.18.1.tar.gz`: `bcba735aebf8cbc11da9b3742118a665599ac697fa06bc5751cac8dcd540db8a`

Executor **应当**在首次构建时同时下载 `SHA256SUMS` 官方文件并交叉核对这两个值（Mutagen release 页提供 `SHA256SUMS` + `SHA256SUMS.gpg`，GPG 签名来自 `xenoscopic`）。

### 下载 URL 模板

```
https://github.com/mutagen-io/mutagen/releases/download/v0.18.1/mutagen_linux_${ARCH}_v0.18.1.tar.gz
```

### 推荐 Dockerfile 片段

```dockerfile
ARG MUTAGEN_VERSION=v0.18.1
RUN --mount=type=cache,target=/tmp/mutagen-cache \
    set -eux; \
    ARCH="$(dpkg --print-architecture)"; \
    case "${ARCH}" in \
      amd64) EXPECTED="7735286c778cc438418209f24d03a64f3a0151c8065ef0fe079cfaf093af6f8f" ;; \
      arm64) EXPECTED="bcba735aebf8cbc11da9b3742118a665599ac697fa06bc5751cac8dcd540db8a" ;; \
      *) echo "unsupported arch: ${ARCH}"; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/mutagen.tar.gz \
      "https://github.com/mutagen-io/mutagen/releases/download/${MUTAGEN_VERSION}/mutagen_linux_${ARCH}_${MUTAGEN_VERSION}.tar.gz"; \
    echo "${EXPECTED}  /tmp/mutagen.tar.gz" | sha256sum -c -; \
    # 只抽 agent bundle，丢弃 mutagen CLI（cloud-claude 自己 embed）
    tar -xzf /tmp/mutagen.tar.gz -C /tmp mutagen-agents.tar.gz; \
    install -m 0644 /tmp/mutagen-agents.tar.gz /opt/mutagen-agents.tar.gz; \
    rm /tmp/mutagen.tar.gz /tmp/mutagen-agents.tar.gz; \
    echo "${MUTAGEN_VERSION}" > /etc/cloud-claude/mutagen.version
```

### Agent 协议版本绑定（PITFALLS C4 对齐）
Mutagen client ↔ agent handshake 使用 "server magic number" 比对版本；**patch 版本差**即会拒绝握手（v0.18.0 ↔ v0.18.1 即触发，见 mutagen-io/mutagen issue #531）[CITED: github.com/mutagen-io/mutagen/issues/531]。

**镜像侧唯一动作**：把 `v0.18.1` 写入 `/etc/cloud-claude/mutagen.version`，供 Phase 31 cloud-claude 启动时与本地 `mutagen version` 做 patch 级比对；不一致则降级 sshfs-only + 错误码 `MOUNT_MUTAGEN_VERSION_SKEW`。

### Agent 自动发现路径（重要未决）
Mutagen daemon 在 SSH transport 下会把 agent `scp` 到 `~/.mutagen/agents/<version>/mutagen-agent`（用户 HOME 下）[CITED: mutagen.io/documentation/transports/ssh/]。

**CONTEXT.md D-09.3 要求** entrypoint `prepare_mutagen_agent` 把 `/opt/mutagen-agents.tar.gz` extract 到 `/usr/local/libexec/mutagen/agents/`，但 Mutagen daemon 默认**不**扫描 `/usr/local/libexec/`。实际生效路径**很可能**需要：

1. entrypoint 里按目标用户（`workspace`，UID 1000）extract 到 `/workspace/.mutagen/agents/v0.18.1/mutagen-agent`；或
2. 创建 symlink `/workspace/.mutagen/agents/v0.18.1 -> /usr/local/libexec/mutagen/agents/v0.18.1`；或
3. 保留 Phase 31 cloud-claude 的"首次连接时主动 push agent"流程，不做镜像侧预放（此时镜像只用 `/opt/mutagen-agents.tar.gz` 作为 fallback）。

[ASSUMED — Mutagen daemon 的 agent 发现路径需要 executor 在实际 runtime 做一次 `mutagen sync create` 验证；若发现 `/usr/local/libexec/mutagen/agents/` 不被识别，planner 需在 Phase 31 任务里补 symlink 或改 extract 目标路径。本阶段 entrypoint 只保证 `/opt/mutagen-agents.tar.gz` 已 extract 到某处且可读。]

### Provenance
- URL 模板与 SHA256 [VERIFIED: github.com/mutagen-io/mutagen/releases/tag/v0.18.1; docs.wakemeops.com/packages/mutagen/]
- tarball 内部结构 [VERIFIED: WakeMeOps install rules]
- agent 发现路径 [ASSUMED — 需要 Phase 31 executor 实测确认]

---

## 3. tini 安装与 PID 1 承载

### Ubuntu 24.04 apt 包信息 [VERIFIED: packages.ubuntu.com/noble/tini]

| 字段 | 值 |
|------|-----|
| 源 | `noble/universe` |
| 版本 | `0.19.0-1` |
| 架构 | amd64 / arm64 / armhf / ppc64el / riscv64 / s390x（全部支持） |
| 二进制路径 | `/usr/bin/tini`（**不是** `/tini`） |
| 包大小 | amd64 ~269 KB（压缩）/ ~815 KB（安装后） |

**注意**：ubuntu:24.04 Docker base 默认**已启用** universe 仓库（与 main 同在 `/etc/apt/sources.list` 中），无需额外 `add-apt-repository`。

### Dockerfile 片段

```dockerfile
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends tini \
    && rm -rf /var/lib/apt/lists/*

# 替换现有 ENTRYPOINT（Dockerfile line 102）
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
```

### 与 `docker --init` 的关系
**Worker `createHost` 不传 `--init`** [VERIFIED: `rg "--init|docker-init" internal/runtime/tasks/worker.go` 无匹配]。

两种方式对比：

| 方式 | 机制 | 优点 | 缺点 |
|------|------|------|------|
| `docker run --init`（Docker daemon 注入 `docker-init` = tini） | 调用方行为 | 不依赖镜像 | 依赖 Docker daemon 配置；worker 需改代码；与镜像 ENTRYPOINT 组合有歧义 |
| 镜像内嵌 tini（**本方案，D-10**） | 镜像固有 | 对调用方透明；ENTRYPOINT 自包含；符合 PITFALLS C7 "tini PID 1 守护 tmux" | 体积 +~815 KB |

D-10 选择镜像内嵌路径是正确的，与 C7 完全对齐。

### Provenance
- apt 包信息 [VERIFIED: packages.ubuntu.com/noble/tini]
- worker.go 不传 `--init` [VERIFIED: grep 结果]

---

## 4. tmux 3.4 能力核对

### Ubuntu 24.04 apt 包信息 [VERIFIED: launchpad.net/ubuntu/noble/+source/tmux]

| 字段 | 值 |
|------|-----|
| 源 | `noble/main/updates` |
| 当前版本 | `3.4-1ubuntu0.1`（2024-08-02 upload，含 SIXEL 修复） |
| Base 版本 | `3.4-1build1`（原始 noble release） |
| 架构 | amd64 / arm64 / armhf / ppc64el / riscv64 / s390x |
| 二进制 | `/usr/bin/tmux` |

**D-06 锚定 `tmux -V` ≥ `3.4`**，实际 apt 安装得到 `tmux 3.4` 字符串（`tmux -V` 输出 `tmux 3.4`），满足下限。

### D-13 三项配置可行性核对

| 配置项 | tmux 引入版本 | 3.4 支持 |
|--------|---------------|----------|
| `set -ga terminal-overrides ",*:RGB"` | 2.2（2016） | ✓ |
| `set -g window-size latest` | 2.9（2019） | ✓ |
| `set -g aggressive-resize on` | ≤ 1.x（古老选项） | ✓ |
| `set -g history-limit 50000` | ≤ 1.x | ✓ |

**结论：D-13 的四项配置在 tmux 3.4 下全部生效**，无需 fallback。

### `/etc/tmux.conf` 骨架（容器级默认）

```tmux
# /etc/tmux.conf（Phase 29 容器级默认；用户 ~/.tmux.conf 可覆盖）
set -ga terminal-overrides ",*:RGB"
set -g window-size latest
set -g aggressive-resize on
set -g history-limit 50000
```

### `/etc/profile.d/cloud-claude.sh` 骨架

```bash
# /etc/profile.d/cloud-claude.sh（Phase 29 容器级默认 env）
export CLAUDE_CODE_TMUX_TRUECOLOR=1
```

### Provenance
- tmux 3.4 在 noble [VERIFIED: launchpad.net/ubuntu/noble/+source/tmux]
- 配置项引入版本 [CITED: tmux/tmux GitHub release notes for 2.2 / 2.9]

---

## 5. Ubuntu 25.04 AppArmor override

### 核心事实（已核对多个上游来源）

Ubuntu 25.04 的 AppArmor 对 `fusermount3` 二进制默认**禁止 `capability dac_override`**，导致容器内即使 `--cap-add SYS_ADMIN --device /dev/fuse --security-opt apparmor=unconfined` 也无法挂载 FUSE（含 mergerfs / sshfs / Mutagen sidecar）[VERIFIED: bugs.launchpad.net/bugs/2111105; github.com/moby/moby/issues/50013; github.com/containerd/stargz-snapshotter/issues/2144]。

**复现环境**：Ubuntu 25.04 + fuse 3.14.0-10 + Kernel 6.14。

**注意与 CONTEXT.md D-23 的冲突**：CONTEXT.md 指定 override 文件为 `/etc/apparmor.d/local/docker-default`，但所有 upstream 证据（moby issue 50013 @jehon 评论、stargz-snapshotter issue 2144 workaround、fuse3 bug tracker #2111105）都指向 **`fusermount3` 的 AppArmor profile**，而**不是** docker-default profile。详见 §Conflicts with CONTEXT.md。

### 实际修复命令（来自 upstream）

**Option A（推荐）——追加 local override**：

```bash
sudo tee /etc/apparmor.d/local/fusermount3 > /dev/null <<'EOF'
capability dac_override,
EOF
sudo apparmor_parser --replace /etc/apparmor.d/fusermount3
```

**Option B（fallback）——直接禁用该 profile**：

```bash
sudo apt-get install -y apparmor-utils
sudo aa-disable /usr/bin/fusermount3
```

### 刷新与回滚

```bash
# 刷新（重载 profile + override）
sudo apparmor_parser --replace /etc/apparmor.d/fusermount3

# 回滚（删除 override，重新加载原始 profile）
sudo rm /etc/apparmor.d/local/fusermount3
sudo apparmor_parser --replace /etc/apparmor.d/fusermount3

# 验证 override 生效
sudo aa-status | grep fusermount3
```

### 非 25.04 行为对照

| 发行版 | fusermount3 profile 默认行为 | 需要 override? |
|--------|-------------------------------|----------------|
| Ubuntu 22.04 | 不阻止（profile 更宽松） | 否 |
| Ubuntu 24.04 LTS（noble） | 不阻止 | 否 |
| Ubuntu 24.10（oracular） | 不阻止 | 否 |
| **Ubuntu 25.04（plucky）** | **阻止 `dac_override`** | **是** |
| Ubuntu 25.10（questing） | 同 25.04 | 是 |
| Debian 12 / 13 | 默认不启 AppArmor 强制 fusermount3 profile | 否 |

[VERIFIED: containerd/stargz-snapshotter/issues/2144 明确列出 "Works fine on: Ubuntu 24.10, Debian 13, Fedora 42"]

### `deploy/host-preflight.sh` 骨架建议

```bash
#!/usr/bin/env bash
# deploy/host-preflight.sh — v3.0 宿主机预检（AppArmor override + FUSE 能力）
set -euo pipefail

# 非 Linux 直接 pass（macOS/WSL 开发机）
if [[ "$(uname -s)" != "Linux" ]]; then
  echo "[host-preflight] 非 Linux 宿主机，跳过检查。"
  exit 0
fi

# 读取发行版
# shellcheck disable=SC1091
. /etc/os-release

# 非 Ubuntu 直接 pass（Debian 目前不受影响）
if [[ "${ID:-}" != "ubuntu" ]]; then
  echo "[host-preflight] 宿主机为 ${ID:-unknown} ${VERSION_ID:-?}，当前无需 AppArmor override。"
  exit 0
fi

# Ubuntu < 25.04 pass
if ! printf '%s\n25.04\n' "${VERSION_ID}" | sort -V -C; then
  echo "[host-preflight] Ubuntu ${VERSION_ID} 无已知 AppArmor + FUSE 冲突，无需 override。"
  exit 0
fi

# Ubuntu ≥ 25.04 — 检查 override
OVERRIDE_FILE=/etc/apparmor.d/local/fusermount3
REQUIRED_LINE="capability dac_override,"

if [[ -f "${OVERRIDE_FILE}" ]] && grep -qF "${REQUIRED_LINE}" "${OVERRIDE_FILE}"; then
  echo "[host-preflight] ✓ AppArmor override 已部署：${OVERRIDE_FILE}"
  exit 0
fi

# 缺失 — 打印修复命令并退 1
cat <<'FIX' >&2
[host-preflight] ✗ 检测到 Ubuntu 25.04+，但缺失 fusermount3 AppArmor override。
容器内 FUSE mount（mergerfs / sshfs / Mutagen）将被拒绝。

请在宿主机以 root 身份执行：

  sudo tee /etc/apparmor.d/local/fusermount3 > /dev/null <<'EOF'
  capability dac_override,
  EOF
  sudo apparmor_parser --replace /etc/apparmor.d/fusermount3

验证：
  sudo aa-status | grep fusermount3

回滚：
  sudo rm /etc/apparmor.d/local/fusermount3
  sudo apparmor_parser --replace /etc/apparmor.d/fusermount3

参考：https://bugs.launchpad.net/ubuntu/+source/fuse3/+bug/2111105
FIX
exit 1
```

### Provenance
- Bug tracker [VERIFIED: bugs.launchpad.net/ubuntu/+source/fuse3/+bug/2111105]
- moby issue + 修复评论 [VERIFIED: github.com/moby/moby/issues/50013]
- stargz-snapshotter workaround [VERIFIED: github.com/containerd/stargz-snapshotter/issues/2144]

---

## 6. BuildKit cache mount 最佳实践

### 核心组合（PITFALLS M18 防御）

```dockerfile
# syntax=docker/dockerfile:1.6  # 启用 BuildKit 与 cache mount 语法
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends <packages> \
    && rm -rf /var/lib/apt/lists/*
```

**关键点：**

1. **两个 cache mount 都要加**：`/var/cache/apt`（.deb 缓存）+ `/var/lib/apt`（索引缓存）；只加一个会让 `apt-get update` 每次重复下载 [CITED: docs.docker.com/build/cache/optimize]。
2. **`sharing=locked`**：BuildKit 同一 layer 的并行构建互斥，避免 apt lock 冲突。
3. **`rm -rf /var/lib/apt/lists/*` 必须在 RUN 末尾**：cache mount 层 commit 时会 unmount，但镜像层会保留 RUN 执行后的文件系统状态；如果 `/var/lib/apt/lists/*` 不删，这些索引文件会被 bake 进最终镜像（+~30MB）。
4. **`--no-install-recommends`**：阻止 apt 安装 "推荐但非依赖" 的包（Chromium 的 `fonts-freefont-ttf` 等易膨胀的推荐集合是镜像超 700MB 的主因）。

### 启用 BuildKit

Ubuntu/macOS 开发机：

```bash
# 方式 A：环境变量（每次 build 都要设置）
DOCKER_BUILDKIT=1 docker build -f deploy/docker/managed-user/Dockerfile -t <IMAGE> .

# 方式 B：buildx（现代 Docker Engine 28.x 默认可用）
docker buildx build -f deploy/docker/managed-user/Dockerfile -t <IMAGE> --load .
```

Docker Engine 28.x（与项目 STACK 对齐）**默认启用 BuildKit**，无需额外配置 [CITED: docs.docker.com/engine/release-notes/28]。

### `build-managed-image.sh` 改造（D-30 约束）
D-30 明确 **build 脚本不嵌入体积检查**，但**应**确保 BuildKit 启用。推荐：

```bash
#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="$(awk -F': ' '$1 == "local_dev_image_name" { print $2 }' deploy/docker/managed-user/image.lock)"
[[ -n "${IMAGE_NAME}" ]] || { echo "无法读取 image_name" >&2; exit 1; }

# 强制启用 BuildKit
export DOCKER_BUILDKIT=1

docker build -f deploy/docker/managed-user/Dockerfile -t "${IMAGE_NAME}" .
```

### Provenance
- BuildKit cache mount [CITED: docs.docker.com/build/cache/optimize]
- Docker 28.x 默认 BuildKit [CITED: docs.docker.com/engine/release-notes/28]

---

## 7. 现镜像体积估算 + ≤ 700MB 可行性

### v2.0 现有 layer 组成（基于现 Dockerfile 推算）

| 层 | 组件 | 体积（估算） | 备注 |
|----|------|--------------|------|
| 1 | `ubuntu:24.04` base | ~78 MB | 固定 |
| 2 | 系统包（openssh-server + bash + zsh + curl + git + tmux + sudo + ca-cert + jq + procps + iproute2） | ~50 MB | 必要，无法裁 |
| 3 | nodejs + npm（apt 包） | ~80 MB | npm 依赖大，考虑改 nodesource 或 bun |
| 4 | locales + locale-gen | ~5 MB | — |
| 5 | Desktop 栈（fluxbox + pcmanfm + dbus-x11 + xdg-utils + xclip + xsel + xterm + x11-utils + x11-xserver-utils） | ~25 MB | 保留（v1.2 deferred 用户面） |
| 6 | **`fonts-noto-cjk`** | **~200 MB** | **主要膨胀源；推荐改为 `fonts-noto-cjk-extra` 或仅 `fonts-noto-cjk-core`（若存在）** |
| 7 | `fonts-liberation` + glibc | ~10 MB | 必要 |
| 8 | `libegl1 + libgl1` + FUSE + sshfs | ~40 MB | — |
| 9 | `KasmVNC .deb` | ~70 MB | 保留（v1.2 用户面） |
| 10 | `Chromium .deb`（from Debian bookworm） | ~250 MB | **主要膨胀源；`--no-install-recommends` 已有；若超标则裁剪进一步依赖** |
| 11 | `npm install -g @anthropic-ai/claude-code` | ~50 MB | 可换 bun / single-file binary 减小 |
| 12 | KasmVNC + Chrome 配置 + entrypoint | ~5 MB | — |

**v2.0 估算总和：~863 MB**（当前已经超 700MB；Phase 35 二次回归验证时可能发现 v2.0 镜像本身也需裁剪）

[ASSUMED — 上述数字是基于包名+apt DB 典型值的推算，executor **必须**在首次 build 后用 `docker history --no-trunc <image>` 跑一遍实测值，并在 PLAN.md 里记录真实 baseline。]

### v3 新增增量

| 新增组件 | 增量体积 |
|----------|----------|
| mergerfs `.deb` | ~2 MB（.deb 432KB 压缩，解压后含二进制 + 文档） |
| `mutagen-agents.tar.gz` 预放 `/opt/` | ~80 MB（保持压缩态，不 extract） |
| tini `.deb` | <1 MB |
| `/etc/tmux.conf` + `/etc/profile.d/cloud-claude.sh` + `/etc/cloud-claude/*.version` | <1 KB |
| 预建目录（`/home/claude/**`、`/workspace-hot`、`/workspace-cold`、`/var/lib/claude-persist`） | <10 KB（空目录 inode） |

**v3 增量总和：~83 MB**

### 裁剪候选（达标 ≤ 700MB 必须启用）

| 裁剪项 | 估算节省 | 风险 | 优先级 |
|--------|----------|------|--------|
| `fonts-noto-cjk` → `fonts-noto-cjk-extra` 或整体移除 | ~150 MB | KasmVNC 内 CJK 字体渲染降级，但 Claude Code 终端场景不受影响（SSH 主路径） | **高**（首选） |
| Chromium: 评估是否需要 `chromium` 本体，或只保留 `chromium-driver` | ~50–100 MB | 用户面 KasmVNC 浏览体验下降 | 中 |
| `npm install -g claude-code` → 改用 Bun 或 Anthropic 官方 single-file binary（若有） | ~30 MB | 需重新验证 claude-code 执行路径 | 中 |
| 分析 Chromium `--no-install-recommends` 是否再拉到 libva/libvdpau 等加速库 | ~20 MB | 部分硬件加速功能降级 | 低 |
| `apt-get clean` + 多 RUN 合并（减少 layer 数与 duplicate apt cache） | ~10 MB | 仅影响 layer 缓存利用率 | 低 |

**推荐组合**：裁 `fonts-noto-cjk` + 保留 Chromium 但去 recommends 中的硬件加速库 → 预期节省 ~160–180 MB，v3 总体积 ~766 MB，**仍略超**。需要再裁 Chromium 或 nodejs/npm。

**Planner 必须在 PLAN.md 里显式列出裁剪清单**，否则 BASE-04 CI gate 首次运行即 fail。

[ASSUMED — 上述裁剪数字为基于 apt 仓库包大小的估算；executor 每裁一项后需要重测 `docker image inspect --format='{{.Size}}'` 核实。]

### Provenance
- 包大小基线 [ASSUMED — 基于 Ubuntu noble apt DB 典型值；需要 executor 实测]
- `--no-install-recommends` 作用 [CITED: man apt-get，`--no-install-recommends`]

---

## 8. Go `worker.go` `--mount type=volume` 拼接

### `--mount` vs `-v` 语义差异 [CITED: docs.docker.com/engine/storage/volumes]

| 特性 | `-v src:dst[:ro]`（bind mount 旧语法） | `--mount type=volume,src=X,dst=Y[,readonly]` |
|------|-----------------------------------------|-----------------------------------------------|
| 自动创建不存在的 volume | ✓（旧行为） | ✗（必须 volume 已存在，否则 `docker: Error response from daemon: No such volume`） |
| readonly 语法 | `:ro` 后缀 | `,readonly` 或 `,ro=true` 参数 |
| 支持选项 | 有限（`z`, `Z`, `ro` 等） | 完整（`volume-driver`, `volume-opt`, `consistency` 等） |
| 可读性 | 简短但易歧义 | 长但明确 |
| 与 bind mount 共存 | 可以（不同行分别写） | 可以 |

**D-19 使用 `--mount type=volume,...` 是正确的**：volume 在 Phase 33 由新增的 `ensureDockerVolume` 函数幂等创建；D-20 明确本阶段 worker 假设 volume 已存在，不存在则返回 `host_action_failed`——这与 `--mount` 的严格语义（不存在直接报错）一致。

### `readonly` 正确写法
Docker 官方文档 [CITED: docs.docker.com/engine/storage/volumes/#syntax]：

```
--mount type=volume,src=<name>,dst=<target>,readonly
```

`readonly`（或 `ro=true`）是 **keyword without value**（或显式 =true）。**不要**写成 `ro` 或 `readonly=true` 后加单词 true——最终字符串直接用 `readonly` 或 `readonly=true`，两者等价。

**推荐**：`readonly`（keyword 形式，与 docker 文档示例一致，可读性最佳）。

### 现有 `createHost` 结构（`internal/runtime/tasks/worker.go:135-197`）

现有拼接顺序（line 157–193）：

```
create
  --name X --network bridge
  --cap-add NET_ADMIN --cap-add SYS_ADMIN
  --device /dev/fuse --security-opt apparmor=unconfined
  --label managed=true --label host_id=X
  --hostname X --shm-size 1g --sysctl net.ipv6.conf.all.disable_ipv6=1
  [--memory Xm]         ← 条件
  [--cpus X.X]          ← 条件
  -e TZ=...
  -e LANG=... -e LANGUAGE=... -e LC_ALL=...
  -e CONTAINER_USER=... -e CONTAINER_SSH_PASSWORD=...
  -v <host_path>:<home_mount>     ← 现有 home bind mount
  [--label key=value ...]         ← 从 request.Labels 遍历
  <image>
```

### v3 Volumes 追加位置建议
按 CONTEXT.md `<code_context>` "docker 容器创建参数拼接" 模式（"追加 env → 追加 -v bind → 追加 labels → 追加 image"），Volumes 应**插在 `-v` bind 之后、`request.Labels` 之前**：

```go
// internal/runtime/tasks/worker.go:createHost
// 现有 -v bind mount 之后、Labels 遍历之前

for _, vol := range request.Volumes {
    mountSpec := fmt.Sprintf("type=volume,src=%s,dst=%s", vol.Name, vol.Target)
    if vol.ReadOnly {
        mountSpec += ",readonly"
    }
    args = append(args, "--mount", mountSpec)
}

for key, value := range request.Labels {
    args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
}
```

**注意：**
- `vol.Labels` 字段（CONTEXT.md D-18）**只用于日志与审计，不写到容器**（D-19 明确）——即使 `vol.Labels` 非空也不追加 `--label`。
- `vol.Name` / `vol.Target` 应在上层（控制面）已做 validation（非空 + 无非法字符），worker 不做二次 sanitize（延续现有风格：`vol.Name == ""` 会产生非法 mount string，docker 会直接 error；worker 只透传错误）。

### 空 Volumes 切片必须等价 v2.0 行为
因 `for _, vol := range nil` 或 `range []VolumeMount{}` 都是 **0 次迭代**，Go 语言级别保证了 "Volumes 为 nil 或空切片时 `args` 不会追加任何 `--mount`"。**这是 D-22 向后兼容的核心保证之一**。

### Provenance
- `--mount` vs `-v` 语义 [CITED: docs.docker.com/engine/storage/volumes/#syntax]
- `readonly` 写法 [CITED: docs.docker.com/reference/cli/docker/container/run/#mount]
- 现有 worker.go 拼接结构 [VERIFIED: 本次 Read tool 直接核对]

---

## 9. JSON 向后兼容验证

### Go `omitempty` + slice 行为 [CITED: pkg.go.dev/encoding/json#Marshal]

| 字段状态 | JSON 输出 |
|----------|-----------|
| `Volumes: nil` | **字段不出现**（omitempty 生效） |
| `Volumes: []VolumeMount{}`（空切片） | **字段不出现**（omitempty 对 `len == 0` 的 slice 生效）|
| `Volumes: []VolumeMount{v1}` | `"volumes": [...]` |

**结论**：D-22 的 "omitempty 保证 v2.0 旧客户端反序列化不破" 成立——新字段在不使用时不出现在 JSON 里，旧 agent 的 `HostActionRequest` struct 即使不含 `Volumes` 字段也能正常 unmarshal（Go `json.Unmarshal` 默认**静默忽略 unknown fields**）。

### 反向兼容（旧 agent 接收新 JSON）
若新控制面发送含 `"volumes": [...]` 的 JSON 给**旧 v2.0 agent**（其 struct 无 `Volumes` 字段）：

- Go `json.Decoder.Decode` 默认行为：未定义字段被静默丢弃 [CITED: pkg.go.dev/encoding/json#Decoder.DisallowUnknownFields]
- 旧 agent 不会报错，但会**忽略 volumes**——这是 D-20/D-22 的预期行为（Phase 29 + 30 过渡期，旧 agent 不实际处理 volumes，新 agent 才拼接 `--mount`）

**警告**：若项目任何一处配置了 `decoder.DisallowUnknownFields()`，则新字段会导致旧 agent 反序列化失败。需要 executor 在 PLAN.md 的 wave-0 里 grep 一次 `DisallowUnknownFields`：

```bash
rg "DisallowUnknownFields" internal/
```

[ASSUMED — 如果有其它 HTTP 中间件启用了严格模式，Volumes 字段会破兼容。需要 executor 在计划 wave-0 里实测 grep。]

### 推荐单测模式（参考 `ssh_inject_test.go` 风格）

应在 `internal/agentapi/` 或 `internal/runtime/tasks/` 新增 `contracts_roundtrip_test.go`：

```go
package agentapi

import (
    "encoding/json"
    "testing"
)

func TestHostActionRequestVolumesOmitempty(t *testing.T) {
    // Volumes 为 nil → JSON 不含 "volumes" 字段
    req := HostActionRequest{TaskID: "t1", HostID: "h1", Action: ActionCreateHost}
    data, err := json.Marshal(req)
    if err != nil {
        t.Fatalf("marshal: %v", err)
    }
    if got := string(data); containsJSONKey(got, "volumes") {
        t.Errorf("nil Volumes 不应出现 volumes 字段: %s", got)
    }

    // 空 slice → 同样不出现
    req.Volumes = []VolumeMount{}
    data2, _ := json.Marshal(req)
    if got := string(data2); containsJSONKey(got, "volumes") {
        t.Errorf("空 Volumes 切片不应出现 volumes 字段: %s", got)
    }

    // 单元素 → 出现
    req.Volumes = []VolumeMount{{Name: "v1", Target: "/data"}}
    data3, _ := json.Marshal(req)
    if got := string(data3); !containsJSONKey(got, "volumes") {
        t.Errorf("非空 Volumes 应出现 volumes 字段: %s", got)
    }
}

func TestHostActionRequestBackwardCompat(t *testing.T) {
    // v2.0 旧 JSON（无 volumes 字段）→ v3.0 struct 反序列化成功且 Volumes == nil
    oldJSON := `{"task_id":"t1","host_id":"h1","action":"create_host"}`
    var req HostActionRequest
    if err := json.Unmarshal([]byte(oldJSON), &req); err != nil {
        t.Fatalf("unmarshal v2.0 JSON 应成功: %v", err)
    }
    if req.Volumes != nil {
        t.Errorf("v2.0 JSON 未显式设 volumes，应保持 nil: %v", req.Volumes)
    }
}

// containsJSONKey: 简单字符串包含检测（JSON key 格式 `"volumes":`）
func containsJSONKey(s, key string) bool {
    return strings.Contains(s, `"`+key+`":`)
}
```

同时**应**在 `worker_test.go`（若存在；否则新增）加一条 table test：`Volumes: nil` 与 `Volumes: []`（空）的 docker create args 应与 v2.0 完全一致；`Volumes: [{Name:"claude-state-xxx",Target:"/var/lib/claude-persist"}]` 应追加恰好一条 `--mount type=volume,src=claude-state-xxx,dst=/var/lib/claude-persist`。

### Provenance
- `json.Marshal` omitempty 对 slice [CITED: pkg.go.dev/encoding/json#Marshal，"The empty values are false, 0, any nil pointer or interface value, and any array, slice, map, or string of length zero"]
- unknown fields 静默丢弃 [CITED: pkg.go.dev/encoding/json#Decoder.DisallowUnknownFields]

---

## 10. mergerfs branch 语法与 env 覆盖实现

### branch 字符串语法 [CITED: trapexit.github.io/mergerfs/latest/config/branches]

mergerfs 的 branch 字符串格式：

```
<path1>[=MODE]:<path2>[=MODE]:...
```

其中 `MODE` ∈ `{RW, RO, NC}`（默认 `RW`）：
- `RW`：读写
- `RO`：只读
- `NC`：No Create — 只读 + 禁止 create（但可以修改已存在文件）

**多个 mode 可以组合，用逗号分隔：`=NC,RO` 即"只读且禁止 create"** [VERIFIED: 上游文档 + 本项目 `.planning/research/STACK.md` 交叉核实]。

### D-12 锁定的 2 路拓扑字符串（核对）

```
/workspace-hot=RW:/workspace-cold=NC,RO
```

**核对结果：语法正确**。等价于：
- `/workspace-hot`：读写（Mutagen 热同步落点）
- `/workspace-cold`：只读 + 禁止 create（sshfs 冷兜底落点）

配合 D-11 的 `category.create=ff`，所有新建都会落到 branch 列表中**第一个**允许 create 的 branch（即 `/workspace-hot`）——这是"写入只落 hot" 设计的核心保证。

### `CLOUD_CLAUDE_MERGERFS_BRANCHES` env 覆盖

**读取位置**：本阶段 entrypoint 与 cloud-claude（Phase 31）都可以读取，但**本阶段 entrypoint 不实际挂载 mergerfs**（D-09.4）。因此：

- **镜像侧**（本阶段）：env 只作为"文档与默认值占位"，entrypoint 可以在日志里 `echo` 当前 env 值（便于 doctor 排查），但不做 mount。
- **cloud-claude 侧**（Phase 31）：实际消费此 env，作为 `mergerfs <branches> /workspace -o ...` 命令行参数。

**默认值**（env 未设置时）：

```
CLOUD_CLAUDE_MERGERFS_BRANCHES="/workspace-hot=RW:/workspace-cold=NC,RO"
```

### `/etc/profile.d/cloud-claude.sh` 追加建议

结合 D-13 的 tmux truecolor env 与 D-12 的 branch 默认值：

```bash
# /etc/profile.d/cloud-claude.sh
export CLAUDE_CODE_TMUX_TRUECOLOR=1

# mergerfs branch 拓扑默认（可被宿主/Phase 31 CLI 覆盖）
: "${CLOUD_CLAUDE_MERGERFS_BRANCHES:=/workspace-hot=RW:/workspace-cold=NC,RO}"
export CLOUD_CLAUDE_MERGERFS_BRANCHES
```

### cloud-claude mergerfs invocation 预览（Phase 31 消费，非本阶段代码）

```bash
# Phase 31 cloud-claude 内部构造的 mergerfs 命令（示例，非最终代码）
mergerfs \
  "${CLOUD_CLAUDE_MERGERFS_BRANCHES}" \
  /workspace \
  -o category.create=ff \
  -o func.readdir=cor:4 \
  -o cache.attr=30 \
  -o cache.entry=30 \
  -o cache.readdir=true \
  -o cache.files=off \
  -o inodecalc=path-hash \
  -o nonempty \
  -o allow_other
```

本阶段 **entrypoint 的 `prepare_mergerfs` 只需**：

```bash
prepare_mergerfs() {
  echo "[v3] prepare_mergerfs: 校验 mergerfs 二进制..."
  if ! mergerfs --version >/dev/null 2>&1; then
    echo "[v3] prepare_mergerfs: mergerfs --version 失败" >&2
    exit 1
  fi
  echo "[v3] prepare_mergerfs: 默认 branches=${CLOUD_CLAUDE_MERGERFS_BRANCHES:-/workspace-hot=RW:/workspace-cold=NC,RO}（实际 mount 由 cloud-claude 触发）"
}
```

### Provenance
- branch 字符串语法 [CITED: trapexit.github.io/mergerfs/latest/config/branches]
- `=NC,RO` 组合语法 [VERIFIED: 与 STACK.md §2 交叉核对，trapexit.github.io/mergerfs/latest/config/functions_categories_policies]

---

## Entrypoint v3 阶段函数骨架

CONTEXT.md D-09 要求的串行编排，在现有 entrypoint.sh 的 **line 99–101（`if [ -c /dev/fuse ]` 块）之后、line 103（KasmVNC 配置）之前**插入 v3 阶段。骨架如下：

```bash
# ============================================================
# v3 阶段（Phase 29 插入）：FUSE 就绪 → 预建目录兜底 → agent bundle → mergerfs 校验
# 串行 + 快速失败；日志前缀 [v3] 便于排查
# ============================================================

log_v3() { echo "[v3] $*"; }

prepare_fuse() {
  log_v3 "prepare_fuse: 强化 /dev/fuse 权限"
  if [ ! -c /dev/fuse ]; then
    echo "[v3] prepare_fuse: /dev/fuse 不存在，容器创建时未挂载设备？" >&2
    exit 1
  fi
  chmod 666 /dev/fuse
}

prepare_v3_dirs() {
  log_v3 "prepare_v3_dirs: 二次 chown 1000:1000（兜底 named volume 初始化 / 旧镜像差异）"
  for d in /home/claude /home/claude/.claude /home/claude/.cache/claude \
           /workspace-hot /workspace-cold /workspace \
           /var/lib/claude-persist; do
    [ -d "${d}" ] || mkdir -p "${d}"
    chown -R 1000:1000 "${d}"
  done
}

prepare_mutagen_agent() {
  log_v3 "prepare_mutagen_agent: 校验 /opt/mutagen-agents.tar.gz"
  if [ ! -s /opt/mutagen-agents.tar.gz ]; then
    echo "[v3] prepare_mutagen_agent: /opt/mutagen-agents.tar.gz 缺失或为空" >&2
    exit 1
  fi
  # extract 到 /usr/local/libexec/mutagen/agents/（CONTEXT.md D-09.3）
  # 注意：Mutagen daemon 实际的 agent 发现路径需 Phase 31 实测；本阶段仅确保 tarball 可读取
  mkdir -p /usr/local/libexec/mutagen/agents
  tar -xzf /opt/mutagen-agents.tar.gz -C /usr/local/libexec/mutagen/agents/
  # 把 mutagen 版本写入 tmux.version 占位（D-07；tmux 版本在下一步回填）
  log_v3 "prepare_mutagen_agent: mutagen agent bundle extracted"
}

prepare_mergerfs() {
  log_v3 "prepare_mergerfs: 校验 mergerfs --version"
  if ! mergerfs --version >/dev/null 2>&1; then
    echo "[v3] prepare_mergerfs: mergerfs 不可执行" >&2
    exit 1
  fi
  # 本阶段不挂载 mergerfs；实际 mount 由 cloud-claude（Phase 31）按 --mount-mode 执行
  log_v3 "prepare_mergerfs: branches=${CLOUD_CLAUDE_MERGERFS_BRANCHES:-/workspace-hot=RW:/workspace-cold=NC,RO}（实际 mount 延后到 cloud-claude）"
}

prepare_tmux_version() {
  # D-07 要求 /etc/cloud-claude/tmux.version 运行时回填（构建阶段占位）
  tmux_v="$(tmux -V 2>/dev/null | awk '{print $2}')"
  echo "${tmux_v:-unknown}" > /etc/cloud-claude/tmux.version
  # D-06 放宽下限 3.4
  if [ -n "${tmux_v}" ] && ! printf '%s\n3.4\n' "${tmux_v}" | sort -V -C; then
    echo "[v3] WARNING: tmux 版本 ${tmux_v} < 3.4，部分 D-13 配置可能不可用" >&2
  fi
}

log_v3 "==== v3 阶段开始 ===="
prepare_fuse
prepare_v3_dirs
prepare_mutagen_agent
prepare_mergerfs
prepare_tmux_version
log_v3 "==== v3 阶段完成 ===="

# 然后进入现有 KasmVNC + sshd 流程
```

**插入位置**：替换 `deploy/docker/managed-user/entrypoint.sh` 现有的 line 99–101：

```bash
if [ -c /dev/fuse ]; then
  chmod 666 /dev/fuse
fi
```

为上述 v3 阶段，并**保留** 原 line 103 之后的 KasmVNC / sshd 逻辑不动。

---

## Go 契约 diff 示意

### `internal/agentapi/contracts.go`

```go
// 新增（与 SSHKeyEntry 同层级，在 HostActionRequest 之前）：
type VolumeMount struct {
    Name     string            `json:"name"`
    Target   string            `json:"target"`
    ReadOnly bool              `json:"read_only,omitempty"`
    Labels   map[string]string `json:"labels,omitempty"`
}

// HostActionRequest 末尾新增一行（保持现有字段顺序不变）：
type HostActionRequest struct {
    // ... 现有字段 ...
    SSHKeys []SSHKeyEntry `json:"ssh_keys,omitempty"`
    Volumes []VolumeMount `json:"volumes,omitempty"`  // ← 新增
}
```

### `internal/runtime/tasks/worker.go:createHost`

```go
// 位置：第 187 行（现有 "-v <host_path>:<home_mount>" 之后）与 189 行（request.Labels 遍历之前）之间

// ---- 新增 Volumes 追加块 ----
for _, vol := range request.Volumes {
    mountSpec := fmt.Sprintf("type=volume,src=%s,dst=%s", vol.Name, vol.Target)
    if vol.ReadOnly {
        mountSpec += ",readonly"
    }
    args = append(args, "--mount", mountSpec)
}
// ---- 新增块结束 ----

for key, value := range request.Labels {
    args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
}
```

---

## CI Gate 骨架（BASE-04）

### GitHub Actions workflow step

```yaml
# .github/workflows/<image-ci>.yml（具体 workflow 文件由 planner 决定插入哪个）
- name: image-size-gate (BASE-04)
  run: |
    set -euo pipefail
    IMAGE_NAME="$(awk -F': ' '$1 == "local_dev_image_name" { print $2 }' deploy/docker/managed-user/image.lock)"
    SIZE=$(docker image inspect --format='{{.Size}}' "${IMAGE_NAME}")
    LIMIT=$((700 * 1024 * 1024))
    echo "image=${IMAGE_NAME} size=${SIZE} bytes limit=${LIMIT} bytes"
    if (( SIZE > LIMIT )); then
      echo "::error::image size ${SIZE} bytes exceeds BASE-04 limit ${LIMIT} bytes"
      echo "---- docker history (膨胀层排查) ----"
      docker history "${IMAGE_NAME}" --no-trunc --format "table {{.Size}}\t{{.CreatedBy}}" || true
      exit 1
    fi
    echo "image size ${SIZE} bytes (limit ${LIMIT} bytes) — PASS"
```

**关键点：**
- `docker image inspect --format='{{.Size}}'` 返回**未压缩**总字节数（sum of layer sizes）——这与 BASE-04 定义一致。
- `::error::` GitHub Actions 注解语法，在 PR 页面以红色高亮显示 [CITED: docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions]。
- `docker history --no-trunc` 默认 truncate 到 ~50 字符，必须加 `--no-trunc` 才能看到完整 `apt-get install ...` 命令，方便排查哪个 RUN 引入了大量依赖。

### 本地开发体验（D-30）
`build-managed-image.sh` **不**嵌入体积检查，保证本地迭代不被强制打断；CI 作为单一 gatekeeper。

---

## Don't Hand-Roll

| 问题 | Don't Build | Use Instead | Why |
|------|-------------|-------------|-----|
| 容器 PID 1 收割僵尸进程 | 自己写 bash trap SIGCHLD | `tini` apt 包（0.19.0-1） | tini 是 Docker 官方推荐（`docker run --init` 内部用的就是 tini 的 fork），经过 10+ 年生产验证 |
| mergerfs .deb checksum 校验 | 无校验 + 裸 HTTPS | `sha256sum -c` + 硬编码 | HTTPS 只防传输层篡改，不防镜像仓库攻击；CI 可复现性依赖固定 SHA256 |
| AppArmor override 检测逻辑 | 自己 grep `/proc/self/attr/current` | 读 `/etc/os-release` + `grep -F "capability dac_override,"` /etc/apparmor.d/local/fusermount3 | upstream (moby/fuse3) 已确认 override 机制就是 profile local override，不是运行时 check |
| BuildKit cache 自建 registry | docker push/pull layer 缓存 | `--mount=type=cache,...` | BuildKit 原生 cache mount 不进 layer，不占镜像体积 |
| 镜像体积测量 | `du -sh` over docker root | `docker image inspect --format='{{.Size}}'` | inspect 返回 sum of layer sizes，与 BASE-04 定义完全一致；`du` 受 overlayfs 影响不可靠 |
| JSON unknown field 兼容处理 | 自写 decoder + skip unknown | Go `encoding/json` 默认行为（静默忽略） | 标准库默认即为宽容，无需任何代码 |

**Key insight**：本阶段所有"新组件"都有成熟 upstream，项目唯一需要做的是"装配"而非"创造"。planner 应把计划任务的颗粒度收到**装配动作级**（"安装 X、配置 Y、拼接 Z"），而不是设计级。

---

## Runtime State Inventory

本阶段**不是** rename / refactor / migration，**是**新增功能。但为了 planner 评估"现状-期望" 差异，下表补充：

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | 无——本阶段不动现有数据。新增 `/var/lib/claude-persist` 空目录为 Phase 33 准备 | 无 |
| Live service config | CI workflow 文件是否已存在 managed-user build job？若有，需新增 `image-size-gate` step；若无，需新建 workflow | planner 应在 wave-0 `ls .github/workflows/` 核实 |
| OS-registered state | `deploy/host-preflight.sh` 是运维手动触发的独立脚本；不需要注册到 systemd / launchd | 无 |
| Secrets/env vars | `CONTAINER_SSH_PASSWORD` / `CLAUDE_ACCOUNT_*` 等现有 env 不变；新增 `CLOUD_CLAUDE_MERGERFS_BRANCHES` 在 `/etc/profile.d/cloud-claude.sh` 默认值，非 secret | 无 |
| Build artifacts | `deploy/docker/managed-user/image.lock` 当前 9 行，新增 6 行 v3 字段；`build-managed-image.sh` 需加 `DOCKER_BUILDKIT=1` export（可选，Docker 28.x 默认开启） | YAML 追加 + bash 小改 |

---

## Common Pitfalls

### Pitfall P-29-A：dpkg -i mergerfs 前忘记装 fuse3
**What goes wrong:** `dpkg: dependency problems prevent configuration of mergerfs: mergerfs depends on libfuse3-3 ...`
**Why it happens:** 现 Dockerfile line 9–41 的 `apt-get install` 已经包含 `fuse3`（line 40），依赖自动满足。但如果 planner 把 mergerfs 安装放到**一个独立的 RUN** 里、且该 RUN 在现有 apt 安装 RUN 之前，就会失败。
**How to avoid:** mergerfs `.deb` 安装 RUN 必须放在现有 line 9–41 apt 安装块**之后**；或者在同一 RUN 里先 `apt-get install fuse3` 再 `dpkg -i mergerfs.deb`。
**Warning signs:** Build 日志出现 `Errors were encountered while processing: mergerfs`。

### Pitfall P-29-B：Mutagen 版本漂移在镜像层无法检测
**What goes wrong:** 用户本地装了 Mutagen v0.18.0 客户端，v3.0 镜像内置 v0.18.1 agent bundle，`mutagen sync create` 握手失败。
**Why it happens:** Mutagen patch 级版本强绑定（PITFALLS C4）。
**How to avoid:** 镜像侧只保证 `/etc/cloud-claude/mutagen.version` 写入 `v0.18.1`；**检测发生在 Phase 31 cloud-claude 启动时**。本阶段 entrypoint 不做本地 mutagen 调用（容器内没装 client，只有 agent bundle）。
**Warning signs:** 用户报告 "server magic number incorrect"——由 cloud-claude 降级到 sshfs-only 处理。

### Pitfall P-29-C：镜像首次 build 就超 700MB，CI 红
**What goes wrong:** Phase 29 首次 PR 进 CI，`image-size-gate` step fail。
**Why it happens:** v2.0 本身估算已 ~863MB（见 §7），v3 新增 ~83MB → 首次估算 ~946MB。
**How to avoid:** planner 必须把 §7 "裁剪候选" 作为 PLAN.md 里的**独立 task**（而非 open follow-up），在首次 PR 就执行至少"裁 `fonts-noto-cjk`" 这一条。
**Warning signs:** Build 完 `docker image inspect` 报 > 700×1024×1024。

### Pitfall P-29-D：AppArmor override 文件路径错写成 `/etc/apparmor.d/local/docker-default`
**What goes wrong:** host-preflight.sh 检测 `/etc/apparmor.d/local/docker-default` 通过，但实际 FUSE mount 仍然被拒（因为真正阻止的是 fusermount3 profile，不是 docker-default profile）。
**Why it happens:** CONTEXT.md D-23 写的路径与 upstream 实际修复路径不一致。
**How to avoid:** planner 必须按 **§Conflicts with CONTEXT.md** 的结论更新路径为 `/etc/apparmor.d/local/fusermount3`。否则 Phase 35 真机 Ubuntu 25.04 验收会 fail。
**Warning signs:** 真机 Ubuntu 25.04 上 mergerfs 仍报 `fusermount3: mount failed: Permission denied`，但 host-preflight.sh 显示 PASS。

### Pitfall P-29-E：BuildKit cache mount 未加 `sharing=locked`，并行 build 踩 apt lock
**What goes wrong:** `E: Could not get lock /var/cache/apt/archives/lock` 偶发 CI 失败。
**Why it happens:** 多 RUN 并发使用同一 cache mount 时 apt 内部锁冲突。
**How to avoid:** 所有 `apt-get ... --mount=type=cache,target=/var/cache/apt` 都加 `sharing=locked`；同理 `/var/lib/apt`。
**Warning signs:** 偶发（~5–10% 概率）CI 失败，重跑成功。

### Pitfall P-29-F：JSON 单测只测 Marshal 不测 Unmarshal 向后兼容
**What goes wrong:** omitempty 单测通过，但 Phase 30 / 真实部署时旧 agent 解码新 JSON 失败（若项目某处启用了 DisallowUnknownFields）。
**Why it happens:** 项目有可能在 HTTP handler 中启用严格 decoder，而本阶段单测只覆盖 struct→JSON。
**How to avoid:** §9 单测骨架已包含 `TestHostActionRequestBackwardCompat`（旧 JSON → 新 struct）；planner 必须在 PLAN.md 里把"grep DisallowUnknownFields 并评估影响" 作为 wave-0 task。
**Warning signs:** Phase 30 集成时旧客户端报 `json: unknown field "volumes"`。

---

## Code Examples 汇总

### Dockerfile 层关键片段（综合）

```dockerfile
# syntax=docker/dockerfile:1.6

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV HOME=/workspace
ENV WORKSPACE_USER=workspace
ENV WORKSPACE_UID=1000
ENV WORKSPACE_GID=1000

# 现有 apt 安装层（保留，仅在 install list 追加 tini，裁减 fonts-noto-cjk）
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends \
        openssh-server bash zsh curl git tmux sudo ca-certificates jq procps \
        iproute2 nodejs npm locales \
        fluxbox pcmanfm dbus-x11 \
        fonts-liberation \
        # fonts-noto-cjk  ← ✗ 已裁减（减约 200MB）
        xdg-utils xclip xsel gnupg libegl1 libgl1 \
        x11-utils x11-xserver-utils xterm sshfs fuse3 \
        tini  \
    && rm -rf /var/lib/apt/lists/*

# 版本元数据目录（D-07）
RUN mkdir -p /etc/cloud-claude

# mergerfs 2.41.1（D-04）
ARG MERGERFS_VERSION=2.41.1
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    set -eux; \
    ARCH="$(dpkg --print-architecture)"; \
    case "${ARCH}" in \
      amd64) EXPECTED="<sha256-amd64-to-compute>" ;; \
      arm64) EXPECTED="<sha256-arm64-to-compute>" ;; \
      *) echo "unsupported arch: ${ARCH}"; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/mergerfs.deb \
      "https://github.com/trapexit/mergerfs/releases/download/${MERGERFS_VERSION}/mergerfs_${MERGERFS_VERSION}.ubuntu-noble_${ARCH}.deb"; \
    echo "${EXPECTED}  /tmp/mergerfs.deb" | sha256sum -c -; \
    dpkg -i /tmp/mergerfs.deb; \
    rm /tmp/mergerfs.deb; \
    mergerfs --version; \
    echo "${MERGERFS_VERSION}" > /etc/cloud-claude/mergerfs.version

# Mutagen agent bundle v0.18.1（D-05）
ARG MUTAGEN_VERSION=v0.18.1
RUN set -eux; \
    ARCH="$(dpkg --print-architecture)"; \
    case "${ARCH}" in \
      amd64) EXPECTED="7735286c778cc438418209f24d03a64f3a0151c8065ef0fe079cfaf093af6f8f" ;; \
      arm64) EXPECTED="bcba735aebf8cbc11da9b3742118a665599ac697fa06bc5751cac8dcd540db8a" ;; \
      *) echo "unsupported arch: ${ARCH}"; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/mutagen.tar.gz \
      "https://github.com/mutagen-io/mutagen/releases/download/${MUTAGEN_VERSION}/mutagen_linux_${ARCH}_${MUTAGEN_VERSION}.tar.gz"; \
    echo "${EXPECTED}  /tmp/mutagen.tar.gz" | sha256sum -c -; \
    tar -xzf /tmp/mutagen.tar.gz -C /tmp mutagen-agents.tar.gz; \
    install -m 0644 /tmp/mutagen-agents.tar.gz /opt/mutagen-agents.tar.gz; \
    rm /tmp/mutagen.tar.gz /tmp/mutagen-agents.tar.gz; \
    echo "${MUTAGEN_VERSION}" > /etc/cloud-claude/mutagen.version

# 预建目录（D-16）
RUN set -eux; \
    mkdir -p /home/claude/.claude /home/claude/.cache/claude \
             /workspace-hot /workspace-cold /var/lib/claude-persist; \
    chown -R 1000:1000 /home/claude /workspace-hot /workspace-cold /var/lib/claude-persist; \
    # tmux.version 在 entrypoint 运行时回填（D-07 / D-06）
    touch /etc/cloud-claude/tmux.version

# 新增 /etc/tmux.conf 与 /etc/profile.d/cloud-claude.sh（D-13）
COPY deploy/docker/managed-user/tmux.conf /etc/tmux.conf
COPY deploy/docker/managed-user/cloud-claude-profile.sh /etc/profile.d/cloud-claude.sh
RUN chmod 0644 /etc/tmux.conf /etc/profile.d/cloud-claude.sh

# （以下 KasmVNC / Chromium / npm claude-code / sshd_config / entrypoint 保持现有结构）
# ...

# D-10：tini PID 1
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
```

### `image.lock` 追加字段（D-26）

```yaml
image_name: ghcr.io/zanel1u/cloud-cli-proxy/managed-user:latest
local_dev_image_name: ghcr.io/zanel1u/cloud-cli-proxy/managed-user:latest
base_image: ubuntu:24.04
pull_policy: never-implicit-latest
ssh_port: 22
home_mount: /workspace
default_user: workspace
rebuild_mode_default: preserve-home
factory_reset_mode: wipe-/workspace

# v3.0 新增（D-26）
image_version: v3.0.0
mergerfs_version: 2.41.1
mutagen_agent_version: v0.18.1
tmux_version_min: "3.4"
supports_mutagen: true
supports_mergerfs: true
```

### `sshd_config` 追加字段（D-14）

```sshd_config
Port 22
Protocol 2
AddressFamily any
ListenAddress 0.0.0.0
PermitRootLogin no
PasswordAuthentication yes
PermitEmptyPasswords no
ChallengeResponseAuthentication no
UsePAM no
X11Forwarding no
PrintMotd no
AuthorizedKeysFile .ssh/authorized_keys
PidFile /var/run/sshd.pid
Subsystem sftp /usr/lib/openssh/sftp-server

# v3.0 新增（D-14）
ClientAliveInterval 15
ClientAliveCountMax 8
MaxSessions 30
MaxStartups 60:30:120
```

---

## Validation Architecture（三列表：文件改动 → 静态断言 → 运行时断言）

> 本项目 `workflow.nyquist_validation: false`，因此不生成 Nyquist 测试任务；但本阶段 7 条 Success Criteria 仍应按下表在 PLAN.md / VERIFICATION.md 里有对应断言命令。

| SC # | Success Criteria | 文件改动 | 静态断言（构建期/commit 期可测） | 运行时断言（容器内 / 宿主机） |
|------|------------------|----------|------------------------------------|------------------------------|
| 1 | `mount \| grep mergerfs` 含 `func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,category.create=ff` | Phase 31 cloud-claude 下发 mount 参数；本阶段只校验 `mergerfs --version` | 本阶段不直接断言（参数由 Phase 31 交付） | `docker exec <ct> mergerfs --version \| grep 2.41.1`（本阶段断言 binary 就绪即可） |
| 2 | `getfattr -n user.mergerfs.branches /workspace/.mergerfs` 含 RW/NC,RO | Phase 31 执行 mount 后才有 branches；本阶段只确保 branch 语法字符串在 env/文档中正确 | `grep -F "/workspace-hot=RW:/workspace-cold=NC,RO" deploy/docker/managed-user/entrypoint.sh`（骨架 echo） | Phase 31 UAT 断言 |
| 3 | `/home/claude/.claude` 存在 + 属主 `1000:1000`；`mutagen-agent --version` == `/etc/cloud-claude/mutagen.version` | Dockerfile 预建 + chown；entrypoint 兜底 chown；Mutagen agent bundle 预放 | `rg "chown -R 1000:1000 /home/claude" deploy/docker/managed-user/`；`rg "v0.18.1" deploy/docker/managed-user/Dockerfile` | `docker exec <ct> stat -c '%u:%g' /home/claude/.claude`（必须 `1000:1000`）；`docker exec <ct> cat /etc/cloud-claude/mutagen.version`（必须 `v0.18.1`） |
| 4 | `tmux -V` ≥ 3.4；`/etc/tmux.conf` 含 `terminal-overrides ",*:RGB"` + `window-size latest` | Dockerfile apt 装 tmux；COPY `/etc/tmux.conf` | `grep -F 'terminal-overrides ",*:RGB"' deploy/docker/managed-user/tmux.conf`；`grep -F 'window-size latest' deploy/docker/managed-user/tmux.conf` | `docker exec <ct> tmux -V`（≥ 3.4）；`docker exec <ct> cat /etc/tmux.conf`（包含两条配置） |
| 5 | 容器内 PID 1 = `tini`，无 `systemd` / `systemd-logind` | Dockerfile apt 装 tini；ENTRYPOINT 改为 `["/usr/bin/tini","--",...]` | `grep -F '["/usr/bin/tini", "--",' deploy/docker/managed-user/Dockerfile`；`! grep -qE "apt.*systemd" deploy/docker/managed-user/Dockerfile` | `docker exec <ct> cat /proc/1/comm`（必须 `tini`）；`docker exec <ct> pgrep systemd`（必须空） |
| 6 | `docker image inspect --format='{{.Size}}'` < 700 * 1024 * 1024 | CI workflow 新增 step；裁剪候选落地（§7） | CI 里的 bash 断言 step | CI gate 运行时自动核对；本地 `docker image inspect` 手测 |
| 7 | `host-preflight.sh` 在 Ubuntu 25.04 缺 override 时退出码 1 + 打印修复命令；非 25.04 退 0 | 新增 `deploy/host-preflight.sh` | `shellcheck deploy/host-preflight.sh`；`grep -F "/etc/apparmor.d/local/fusermount3" deploy/host-preflight.sh`（按本研究修正） | 在 Ubuntu 25.04 VM 跑 `bash deploy/host-preflight.sh; echo $?`（期望 1）；部署 override 后重跑期望 0；macOS 跑期望 0 |

### 补充断言（Worker 契约 / JSON 兼容）

| 项 | 静态 | 运行时 |
|----|------|--------|
| `HostActionRequest.Volumes` 字段存在且 `omitempty` | `grep -n "Volumes \[\]VolumeMount" internal/agentapi/contracts.go`；`grep -n 'json:"volumes,omitempty"' internal/agentapi/contracts.go` | `go test ./internal/agentapi/... -run TestHostActionRequestVolumesOmitempty`（新增单测） |
| worker.go 追加 `--mount type=volume,...` | `grep -n "type=volume,src=" internal/runtime/tasks/worker.go` | `go test ./internal/runtime/tasks/... -run TestCreateHostVolumesAppending`（新增；table test：nil/空/单元素三档） |
| 旧 v2.0 JSON 向后兼容 | — | `go test ./internal/agentapi/... -run TestHostActionRequestBackwardCompat` |

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `ENV TINI_VERSION=v0.19.0` + `ADD https://.../tini /tini` | `apt install tini` → `/usr/bin/tini` | Debian Buster / Ubuntu 20.04+ | 免验签步骤、免版本锁，直接用发行版维护 |
| mergerfs `apt install mergerfs` (Debian ships 2.33.5) | GitHub release `.deb` 2.41.1 | PITFALLS M3 决策 | 摆脱 Debian 滞后，获得 `func.readdir=cor` |
| Mutagen 客户端预先 scp agent（慢冷启动） | 容器预放 `/opt/mutagen-agents.tar.gz` | STACK §1 | 省 2–4s 冷启动 |
| Dockerfile 单层 `RUN apt-get install` 无 cache | `--mount=type=cache,target=/var/cache/apt,sharing=locked` | BuildKit 0.8+（2021） | 增量 build 时间从分钟级降到秒级 |
| 依赖 `docker run --init` 由调用方提供 PID 1 | 镜像内嵌 `tini` + `ENTRYPOINT` | PITFALLS C7 | 对调用方透明，worker 不改代码 |
| AppArmor 完全依赖 `--security-opt apparmor=unconfined` | Ubuntu 25.04+ 需额外 `/etc/apparmor.d/local/fusermount3` override | Ubuntu 25.04（2025-04 release） | 本阶段 host-preflight.sh 核心使命 |

**Deprecated/outdated:**
- `mergerfs apt install`（2.33.5）——官方明确不推荐 [CITED: PITFALLS M3]
- `mutagen --agents` subcommand 作为 agent 管理工具（v0.18 以后整个推回 release tarball 分发）[CITED: mutagen-io/mutagen CHANGELOG]
- `docker run --init`（仍受支持，但不如镜像内嵌 tini 自包含）

---

## Conflicts with CONTEXT.md

### Conflict #1：AppArmor override 文件路径（D-23）

**CONTEXT.md D-23 原文：**
> 检查 `/etc/apparmor.d/local/docker-default` 是否包含 `capability dac_override,`

**研究证据：** 所有 upstream 修复都指向 `fusermount3` profile，**不是** `docker-default` profile：

1. [VERIFIED: github.com/moby/moby/issues/50013 — @jehon 2025-09-26 评论]：
   > "I added a file named `/etc/apparmor.d/local/fusermount3` with content `capability dac_override,` And take it into account: `sudo apparmor_parser --replace /etc/apparmor.d/fusermount3`"
2. [VERIFIED: github.com/containerd/stargz-snapshotter/issues/2144]：
   > "Workaround: Disable AppArmor for `fusermount3` (profile): `sudo aa-disable /usr/bin/fusermount3`"
3. [VERIFIED: bugs.launchpad.net/bugs/2111105 — 官方 Ubuntu bug tracker]：fuse3 3.14.0-10 下 `fusermount3` 二进制的 AppArmor profile 是阻止源。
4. [CITED: .planning/research/PITFALLS.md:160]：项目自己的 PITFALLS.md C6 就已经明确写道 "要求运维部署 `/etc/apparmor.d/local/fusermount3` 文件：`capability dac_override,`"——与 CONTEXT.md D-23 自相矛盾。

**影响：** 如果按 D-23 原文实现 host-preflight.sh，脚本会检测 `/etc/apparmor.d/local/docker-default` 是否有 `capability dac_override,` 行——这条行即使部署也**不会解决** FUSE mount 问题，因为阻止 mount 的是 fusermount3 profile 而不是 docker-default profile。Phase 35 真机 Ubuntu 25.04 验收会 fail（host-preflight PASS 但 mergerfs mount 仍报 `fusermount3: mount failed: Permission denied`）。

**Recommended resolution：** planner 应**不按 D-23 原文实现**，改为：

- 检测文件：`/etc/apparmor.d/local/fusermount3`
- 修复命令打印：追加 `capability dac_override,` 到 `/etc/apparmor.d/local/fusermount3`
- 刷新：`sudo apparmor_parser --replace /etc/apparmor.d/fusermount3`
- 运维手册（D-25）同步修正

**Action for planner：** 在 PLAN.md 的 host-preflight 任务里直接使用修正后的路径；同时在 phase 提交到 `/gsd-execute-phase` 之前，由用户确认是否需要回流更新 CONTEXT.md D-23 原文（建议回流以保持决策记录与实际实现一致）。

### Conflict #2：无其它冲突

mergerfs `.deb` 命名（D-04 `mergerfs_2.41.1.ubuntu-noble_<arch>.deb`）、Mutagen agent bundle 内部结构（D-05 `mutagen-agents.tar.gz`）、tmux 版本下限（D-06 ≥ 3.4）、tini 安装路径（D-10 apt）、Volumes 契约（D-18–D-22）、CI gate 脚本（D-28）均与 upstream 事实一致。

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | mergerfs `.deb` 的 `Depends:` 声明含 `libfuse3-3`（未实测 `dpkg -I`） | §1 | 若依赖名不同，`dpkg -i` 阶段报缺依赖；executor 首次 build 需 `dpkg -I mergerfs_*.deb` 核实 |
| A2 | Mutagen daemon 的 agent 发现路径 `/usr/local/libexec/mutagen/agents/` 会被自动识别 | §2 | 若不识别，Phase 31 cloud-claude 需额外处理（symlink 到 `~/.mutagen/agents/<version>/` 或改为运行时 push）；本阶段只保证 tarball 可读 |
| A3 | v2.0 现有 layer 体积估算（~863MB）基于 apt 仓库典型值 | §7 | 若实测 v2.0 < 600MB，v3 裁剪压力小很多；若 > 900MB 则裁剪清单需要更激进；必须 executor 首次 build 后 `docker image inspect` 实测 |
| A4 | 项目其它 HTTP handler 未启用 `DisallowUnknownFields` | §9 | 若启用，Volumes 字段会破旧 agent 兼容；wave-0 必须 grep 核实 |
| A5 | `fonts-noto-cjk` 裁减后 Claude Code 终端场景（SSH 主路径）完全不受影响 | §7 | 若 KasmVNC 或某个 v1.2 deferred 用户面功能依赖 CJK 字体渲染，体积裁剪会回归这些场景；需要 Phase 35 真机验证 |
| A6 | mergerfs 官方 release 未发布 SHA256SUMS 文件 | §1 | 已 [VERIFIED] 2.41.1 资产清单无 SHA256SUMS，该假设成立；若未来版本补上官方 SHA256SUMS，build 脚本应优先用官方值 |

**Claims tagged `[ASSUMED]` 集中落点：A2（Mutagen agent 发现路径）最关键**——建议 planner 在 PLAN.md 里把"Mutagen agent bundle extract 路径是否被自动识别" 作为 Phase 31 executor 首个任务去实测，本阶段**不**承诺 agent extract 路径的"一次到位"。

---

## Open Questions

1. **Mutagen agent 自动发现路径的最终形态**
   - What we know: `/opt/mutagen-agents.tar.gz` 预放正确，daemon 默认从 `~/.mutagen/agents/<version>/` 找
   - What's unclear: CONTEXT.md D-09.3 指定 extract 到 `/usr/local/libexec/mutagen/agents/`——是否需要额外 symlink / 环境变量 / Phase 31 CLI 改写
   - Recommendation: 本阶段只保证 tarball 可读、extract 成功即止；把"是否需要补符号链接" 留给 Phase 31 executor 实测并在 VERIFICATION.md 记录

2. **镜像体积裁剪的具体组合**
   - What we know: 需要裁约 150–200 MB 才能守住 ≤ 700MB
   - What's unclear: 裁 `fonts-noto-cjk` 是否影响 v1.2 deferred 用户面（KasmVNC + Chromium 内的中文网页渲染）
   - Recommendation: planner 在 PLAN.md 新增"裁剪评估" 任务，首次 build 后 `docker image inspect` 实测，若仍超 700MB 则按 §7 表格由上往下逐项裁

3. **host-preflight.sh 的 CI 集成**
   - What we know: D-24 明确不嵌入控制面启动流程；D-30 明确 build 脚本不嵌入
   - What's unclear: 是否应该把 `bash deploy/host-preflight.sh` 作为 GitHub Actions workflow step 跑一次（CI runner 是 ubuntu-24.04，脚本会直接 pass，但至少验证脚本语法 / shellcheck 通过）
   - Recommendation: CI 跑 `shellcheck deploy/host-preflight.sh` 做静态检查；**不**在 CI runner 里执行脚本（runner 非 25.04，无法测阳性路径）；Phase 35 真机验收覆盖阳性路径

4. **`image.lock` 的 SHA256 字段是否加入**
   - What we know: D-26 列出 6 个新字段，不含 SHA256
   - What's unclear: 是否把 `mergerfs_deb_sha256_amd64` / `mergerfs_deb_sha256_arm64` / `mutagen_tar_sha256_amd64` / `mutagen_tar_sha256_arm64` 追加到 image.lock 作为 BOM（bill of materials）
   - Recommendation: 本阶段不追加（保持 D-26 清单），SHA256 硬编码在 Dockerfile ARG / 子 shell 中即可；若 Phase 34 doctor 需要 runtime 核对，再在 Phase 34 扩展 image.lock

---

## Environment Availability

本阶段 executor 机器需要的工具：

| Dependency | Required By | Available? | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Docker Engine | 镜像 build 与本地测试 | ✓ | ≥ 28.x（与 STACK 对齐） | — |
| BuildKit | Dockerfile `--mount=type=cache` 语法 | ✓ | Docker 28.x 默认启用 | `DOCKER_BUILDKIT=1` 环境变量 |
| Go | worker.go / contracts.go 改造 | ✓ | 1.26.1（与 STACK 对齐） | — |
| shellcheck | host-preflight.sh / entrypoint.sh 静态检查 | ? | — | 若无则 planner 在 PLAN.md 里追加 `apt install shellcheck` task |
| curl | Dockerfile 下载 .deb / tarball | ✓ | 镜像 base 已装 | — |
| sha256sum | 校验下载物 | ✓ | coreutils 内置 | — |

**Missing dependencies with no fallback:** 无（本阶段所有外部依赖都由 Docker base image 提供或 apt 可装）

**Missing dependencies with fallback:** shellcheck（可能非默认安装，但 macOS 可 `brew install shellcheck`，Ubuntu 可 `apt install shellcheck`）

**真机验收依赖（属 Phase 35，本阶段不阻塞）：**
- Ubuntu 25.04 VM（测 AppArmor override 阳性路径）
- Docker 28.x 在真机上跑容器（测 mergerfs / Mutagen agent bundle 在 runtime 是否如预期）

---

## Sources

### Primary (HIGH confidence — 已通过工具 VERIFIED)

- [VERIFIED: github.com/trapexit/mergerfs/releases/tag/2.41.1] — mergerfs 2.41.1 全部 release 资产清单（ubuntu-noble amd64/arm64 .deb 存在）
- [VERIFIED: github.com/mutagen-io/mutagen/releases/tag/v0.18.1] — Mutagen v0.18.1 Linux tarball 清单 + SHA256SUMS 存在
- [VERIFIED: packages.ubuntu.com/noble/tini] — tini 0.19.0-1 在 noble universe，`/usr/bin/tini`
- [VERIFIED: launchpad.net/ubuntu/noble/+source/tmux] — tmux 3.4-1ubuntu0.1 在 noble main/updates
- [VERIFIED: bugs.launchpad.net/ubuntu/+source/fuse3/+bug/2111105] — Ubuntu 25.04 fuse3 3.14.0-10 AppArmor 冲突官方 bug
- [VERIFIED: github.com/moby/moby/issues/50013] — Docker FUSE mount Ubuntu 25.04 问题与 `/etc/apparmor.d/local/fusermount3` 修复路径
- [VERIFIED: github.com/containerd/stargz-snapshotter/issues/2144] — `aa-disable /usr/bin/fusermount3` workaround
- [VERIFIED: 本项目 `internal/runtime/tasks/worker.go`] — 现有 `createHost` 拼接结构与 `grep "--init"` 无匹配
- [VERIFIED: 本项目 `internal/agentapi/contracts.go`] — 现有 `omitempty` 用法
- [VERIFIED: 本项目 `deploy/docker/managed-user/{Dockerfile,entrypoint.sh,sshd_config,image.lock}`] — 全部改造对象现状

### Secondary (MEDIUM confidence — 已 CITED 官方文档或第三方打包源)

- [CITED: docs.wakemeops.com/packages/mutagen/] — Mutagen tarball 内部结构（`mutagen-agents.tar.gz` 顶层）与 SHA256 交叉核对
- [CITED: mutagen.io/documentation/transports/ssh/] — Mutagen SSH transport 与 agent 发现路径
- [CITED: docs.docker.com/build/cache/optimize] — BuildKit cache mount 官方教程
- [CITED: docs.docker.com/engine/storage/volumes/#syntax] — `--mount type=volume` 语法
- [CITED: trapexit.github.io/mergerfs/latest/config/branches] — mergerfs branch 字符串语法
- [CITED: trapexit.github.io/mergerfs/latest/config/functions_categories_policies] — `RO/NC/RW` mode 语义
- [CITED: pkg.go.dev/encoding/json#Marshal] — Go `omitempty` 对 slice 的语义
- [CITED: docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions] — `::error::` 注解语法

### Tertiary (LOW confidence — 需要 executor 侧实测)

- [ASSUMED] — mergerfs .deb `Depends:` 字段具体值（A1）
- [ASSUMED] — Mutagen daemon 对 `/usr/local/libexec/mutagen/agents/` 的识别（A2）
- [ASSUMED] — v2.0 镜像实际体积（A3）
- [ASSUMED] — 项目未启用 `DisallowUnknownFields`（A4）

### 项目内部 Canonical refs（同步核对）

- `.planning/research/STACK.md` §1–§4（Mutagen / mergerfs / sshfs / tmux 版本与理由）
- `.planning/research/PITFALLS.md` C1/C2/C3/C5/C6/C7/M3/M4/M7/M8/M12/M17/M18
- `.planning/research/SUMMARY.md` §2/§5
- `.planning/REQUIREMENTS.md` §BASE-04 / §F1–F7 / §Critical Pitfalls / §Open Questions
- `.planning/ROADMAP.md` §Phase 29
- `.planning/phases/17-image-entrypoint-baseline/17-CONTEXT.md`（entrypoint 串行编排模式参考）

---

## Metadata

**Confidence breakdown:**

- Standard stack: **HIGH** — 所有版本号与 URL 都通过官方 release 页 [VERIFIED]
- Architecture: **HIGH** — v2.0 现有 Dockerfile / worker.go / contracts.go 均直接读取核对
- Pitfalls: **HIGH** — AppArmor override 路径冲突已通过 3 个独立 upstream 源交叉验证（moby + Ubuntu bug tracker + stargz-snapshotter）
- Volumes 契约: **HIGH** — Go `encoding/json` 行为为语言级保证
- Image size: **MEDIUM** — 估算基于 apt 仓库典型值；executor 必须实测
- Mutagen agent 发现路径: **LOW** — Mutagen daemon 对预放路径的识别行为需实测

**Research date:** 2026-04-18
**Valid until:** 2026-05-18（30 天，适用于 mergerfs / tmux / tini / fuse3 等稳定组件；Mutagen 与 Ubuntu 25.04 AppArmor 规则若有 patch 发布需重新核对）
