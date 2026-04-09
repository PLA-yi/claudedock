# Phase 17: 镜像与 Entrypoint 基线 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-09
**Phase:** 17-image-entrypoint-baseline
**Areas discussed:** 镜像基线与工具选择, Claude Code 安装策略, Entrypoint 编排模式, 镜像位置与项目结构
**Mode:** --auto (all decisions auto-selected)

---

## 镜像基线与工具选择

| Option | Description | Selected |
|--------|-------------|----------|
| Ubuntu 24.04 + 最小工具集 | 与现有 managed-user 一致，glibc 兼容，精简为 curl/git/jq/procps/iproute2/nftables | ✓ |
| Debian bookworm-slim | 更小体积，与 sing-box-gateway 一致 | |
| Alpine | 最小体积，但 musl 兼容性风险（Bun/Claude Code） | |

**User's choice:** [auto] Ubuntu 24.04 + 最小工具集 (recommended default)
**Notes:** Ubuntu 24.04 保证 Bun standalone 和 sing-box glibc 兼容，与 managed-user 保持一致的基线减少维护成本

---

## Claude Code 安装策略

| Option | Description | Selected |
|--------|-------------|----------|
| 构建时预装 + 不锁定版本 | 官方 curl 安装脚本在 docker build 时执行，运行时 DISABLE_AUTOUPDATER=1 | ✓ |
| 运行时安装 | entrypoint 中每次启动时安装，保证最新但增加启动延迟 | |
| 构建时预装 + 锁定版本 | 固定特定版本号，需手动维护 | |

**User's choice:** [auto] 构建时预装 + 不锁定版本 (recommended default)
**Notes:** 与 REQUIREMENTS 中"使用官方安装的最新版本，不做版本锁定"一致。构建时预装可复现且避免首次启动等待

---

## Entrypoint 编排模式

| Option | Description | Selected |
|--------|-------------|----------|
| Shell 脚本 + 分步函数 + 快速失败 | bash 脚本，各阶段独立函数，set -euo pipefail，exec 启动 Claude Code | ✓ |
| 独立二进制编排器 | Go 或 C 写的 init 进程，更精确的进程管理 | |
| 多进程 supervisor | 类似 s6/runit，管理多个服务 | |

**User's choice:** [auto] Shell 脚本 + 分步函数 + 快速失败 (recommended default)
**Notes:** Shell 脚本最简单直接，与现有 managed-user entrypoint 模式一致。Claude Code 作为 PID 1（通过 exec）保证信号透传

---

## 镜像位置与项目结构

| Option | Description | Selected |
|--------|-------------|----------|
| claude-shell/ 子目录独立 Dockerfile | 与 BUILD-02 一致，完全独立于 cloud-cli-proxy 主项目 | ✓ |
| deploy/docker/claude-shell/ | 放在现有 deploy 目录下，与其他镜像并列 | |
| managed-user 精简变体 | 多阶段构建，从 managed-user 裁剪 GUI 组件 | |

**User's choice:** [auto] claude-shell/ 子目录独立 Dockerfile (recommended default)
**Notes:** 与 BUILD-02 要求的独立 go.mod 一致，两个产品线不互相约束

---

## Claude's Discretion

- 容器内工作用户的 UID/GID 策略
- entrypoint 日志格式
- 镜像层优化策略

## Deferred Ideas

None — discussion stayed within phase scope
