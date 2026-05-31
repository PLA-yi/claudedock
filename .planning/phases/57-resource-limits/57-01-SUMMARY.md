---
phase: 57-resource-limits
plan: 01
subsystem: database
tags: [postgres, pgx, migration, nullable, resource-limits]

requires: []
provides:
  - "hosts 表资源限制列改为 nullable（NULL=无限制）"
  - "Go 数据模型资源字段改为指针类型（*int/*float64）"
  - "Repository.UpdateHostResources 方法（COALESCE 部分更新）"
affects: [57-02-api-worker]

tech-stack:
  added: []
  patterns:
    - "Repository 层不注入默认值，默认值逻辑收拢到 API 层"
    - "COALESCE($N, column) 实现 nullable 字段的部分更新语义"

key-files:
  created:
    - internal/store/migrations/0022_host_resource_limits_nullable.sql
  modified:
    - internal/store/repository/models.go
    - internal/store/repository/queries.go

key-decisions:
  - "NULL = 无限制，正值 = 具体限制，0 = 无限制（API 层将 0 转为 nil 写入 NULL）"
  - "默认值注入从 Repository 层移除，由 API 层（Plan 57-02）在创建/PATCH 时负责"

requirements-completed:
  - RES-01

duration: 15min
completed: 2026-05-31
---

# Phase 57 Plan 01: 数据库迁移 + Go 数据模型改造 Summary

**将 hosts 表资源限制列改为 nullable，Go 类型改为指针，移除 Repository 层默认值兜底，新增 COALESCE 部分更新方法**

## Performance

- **Duration:** ~15min
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments

- 迁移文件 0022：3 条 DROP NOT NULL + 3 条 DROP DEFAULT，清理列级默认值
- Host/UpsertHostParams 资源字段改为指针类型（`*int`/`*float64`），pgx 原生支持 NULL↔nil 映射
- UpsertHost 中移除 12 行 defaultIfZero 兜底逻辑，参数直接透传给 SQL
- 新增 UpdateHostResources 方法，COALESCE($N, column) 实现三态部分更新（nil=跳过，0=NULL=无限制，正值=限制）

## Task Commits

1. **Task 1: 创建数据库迁移 0022** — `6050b3f` (feat)
2. **Task 2: Go 数据模型类型变更** — `a51e055` (feat)
3. **Task 3: 新增 Repository.UpdateHostResources 方法** — `a51e055` (feat)

## Decisions Made

- pgx 对 `**int`（`&item.MemoryLimitMB` 当字段类型为 `*int` 时）的 NULL 扫描行为已验证：所有 13 处 Scan 调用点无需修改代码

## Deviations from Plan

None — plan executed exactly as written.

## Issues Encountered

None

## Next Phase Readiness

数据层准备就绪，等待 Plan 57-02（API + Worker 层）消费三态指针语义。

---
*Phase: 57-resource-limits*
*Completed: 2026-05-31*
