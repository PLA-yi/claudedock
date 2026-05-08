# Phase 44: doctor sshd_config 主动验证 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions captured in CONTEXT.md — this log preserves the analysis.

**Date:** 2026-05-08
**Phase:** 44-doctor-sshd-config
**Mode:** --auto (discuss mode, all gray areas auto-selected, recommended defaults applied)
**Areas discussed:** 检查范围, 严重级别, 归属维度, 测试策略

---

## 检查范围

| Option | Description | Selected |
|--------|-------------|----------|
| 仅 AllowTcpForwarding | 最小变更，只检查 roadmap 明确提到的指令 | |
| 全部三个转发指令 | AllowTcpForwarding + AllowStreamLocalForwarding + GatewayPorts | ✓ |

**Auto-selected:** 全部三个转发指令（推荐默认 — 与镜像 sshd_config 中的 3 条显式配置对齐，一次检查消除整个端口转发配置面的隐患）
**Notes:** 基准值来自 `deploy/docker/managed-user/sshd_config`：AllowTcpForwarding=yes, AllowStreamLocalForwarding=yes, GatewayPorts=no

---

## 严重级别

| Option | Description | Selected |
|--------|-------------|----------|
| Fail | 配置错误视为严重问题，doctor 退出码 2 | |
| Warn | 配置偏差警告，与其他 sshd 检查一致 | ✓ |

**Auto-selected:** Warn（推荐默认 — 与现有 SSH_SSHD_KEEPALIVE_DRIFT 严重级别一致；Fail 过于强硬，SSH 本身不会崩溃，只是端口转发不可用）

---

## 归属维度

| Option | Description | Selected |
|--------|-------------|----------|
| remote_ssh.go | 放在 VS Code 特有维度中 | |
| ssh.go | 放在通用 SSH 健康检查维度中 | ✓ |

**Auto-selected:** ssh.go（推荐默认 — sshd_config 解析不依赖 VS Code，与 checkSSHDKeepaliveDrift 同属通用 SSH 健康检查）

---

## 测试策略

| Option | Description | Selected |
|--------|-------------|----------|
| 独立函数测试 | 每个测试场景一个 Test 函数 | |
| table-driven + parseSSHDConfig 提取 | 解析逻辑抽成独立函数，test cases 用 table 驱动 | ✓ |

**Auto-selected:** table-driven + parseSSHDForwarding 函数提取（推荐默认 — 复用 parseSSHDKeepalive 的函数提取模式，解析逻辑可独立测试）

---

## Claude's Discretion

- sshd -T 的具体 grep 命令由实现决定
- 错误码 NextAction 文字格式由实现决定
- 无用户提供的 "you decide" 选择

## Deferred Ideas

None — all gray areas resolved within phase scope
