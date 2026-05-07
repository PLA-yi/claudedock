---
gsd_state_version: 1.0
milestone: v3.2
milestone_name: "多形态容器接入"
status: executing
last_updated: "2026-05-07T11:22:07Z"
last_activity: 2026-05-07
progress:
  total_phases: 4
  completed_phases: 0
  total_plans: 13
  completed_plans: 1
  percent: 8
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-07 — v3.2 milestone started)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** Phase 38 — SSH Proxy 端口转发支持 (ready to plan)

## Current Position

Milestone: v3.2 多形态容器接入
Phase: 38 of 41 (SSH Proxy 端口转发支持)
Plan: 01 of 02 (direct-tcpip Channel Forwarding — COMPLETE)
Status: Executing Phase 38 — Plan 01 complete, Plan 02 pending
Last activity: 2026-05-07 — Completed 038-01: direct-tcpip channel forwarding + security validation

Progress: [░░░░░░░░░░] 8%

## Performance Metrics

**Velocity:**
- Total plans completed: 1 (v3.2)
- Average duration: 3min 17s
- Total execution time: 3min 17s

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 038 | 1/2 | 3min 17s | 3min 17s |

*Updated after each plan completion*

## Accumulated Context

### Decisions

Full decision log in PROJECT.md Key Decisions table.

v3.2 初始决策：

- Cloud 版与本地版 **并行推进**，不冲突
- 本地版也强制 sing-box tun 全隧道，保持产品一致性
- 架构方向（一套代码 vs 两套入口）待研究后决策

Phase 38 Plan 01 决策：

- `channelOpenDirectMsg` 字段使用导出名（Raddr/Rport/Laddr/Lport），因为 `ssh.Marshal` 通过反射读取字段，未导出字段会导致 panic
- `dialContainer` 在 forward.go 中提取（而非 proxy.go），因为 `handleDirectTCPIP` 需要调用它
- `isForbiddenTarget` 设计为纯函数，不依赖 Server 结构体，便于单元测试

### Pending Todos

- Phase 38 Plan 02: SSH Proxy session 扩展（如 exec、pty 等）或集成测试
- Phase 39: Cloud/Local 两版架构边界分析
- Phase 39: Dev Containers 配置设计

### Blockers/Concerns

无。

### Quick Tasks Completed

v3.1 quick tasks 见归档 STATE。

### Roadmap Evolution

v3.2 roadmap 已创建：
- Phase 38: SSH-01..04 (端口转发 + 安全校验)
- Phase 39: LOCAL-01..04 + UX-02 (本地 Dev Containers)
- Phase 40: SSH-05 + SEC-01..02 (E2E 验证 + 安全)
- Phase 41: UX-01 (doctor 扩展)

## Session Continuity

Last session: 2026-05-07T11:22:07Z
Stopped at: Completed 038-01-PLAN.md (direct-tcpip channel forwarding)
Resume file: None

## Deferred Items

v3.1 遗留 deferred items 保持原状态，见 MILESTONES.md。

---
*State updated: 2026-05-07 after v3.2 roadmap creation*
