# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-14)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** v2.0 cloud-claude 透明远程 CLI — 定义需求中

## Current Position

Phase: Not started (defining requirements)
Plan: —
Status: Defining requirements
Last activity: 2026-04-14 — Milestone v2.0 started

Progress: [░░░░░░░░░░] 0%

## Performance Metrics

**Velocity:**
- Total plans completed: 0 (v2.0)
- Average duration: —
- Total execution time: 0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| — | — | — | — |

*Updated after each plan completion*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [v2.0 design]: 客户端为独立 Go 二进制 cloud-claude，与 cloud-cli-proxy 服务端零依赖
- [v2.0 design]: 目录映射首选 sshfs -o slave over SSH 多路复用，备选 Mutagen
- [v2.0 design]: 用户配置存储在 ~/.cloud-claude/config.yaml
- [v2.0 design]: SSH Proxy 已支持多 session channel + exec 转发，零改造可用

### Pending Todos

None yet.

### Blockers/Concerns

- sshfs -o slave 方案需在真实 SSH Proxy 上端到端验证
- 容器 FUSE 设备权限对 AppArmor/SELinux 环境的兼容性待测
- 目录映射在高延迟网络下的 I/O 性能需评估

## Session Continuity

Last session: 2026-04-14
Stopped at: Milestone v2.0 initialized, defining requirements
Resume file: —
