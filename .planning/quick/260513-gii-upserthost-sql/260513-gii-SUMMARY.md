---
phase: quick-260513-gii-upserthost-sql
plan: 01
subsystem: store/repository
tags: [bugfix, sql, postgres, hosts]
dependency-graph:
  requires: []
  provides:
    - "修复后的 UpsertHost SQL：列数 12 与 VALUES 占位符 12 严格对齐"
  affects:
    - "POST /v1/admin/hosts（创建主机接口）"
    - "Repository.UpsertHost 调用路径上的所有上游"
tech-stack:
  added: []
  patterns:
    - "保持 INSERT 列顺序 / Go 参数顺序 / Scan 顺序三者一致"
key-files:
  created: []
  modified:
    - internal/store/repository/queries.go
decisions:
  - "仅修复 SQL 字符串，不动 Go 参数列表，不新增/删除测试，确保改动范围最小化"
  - "保留 74c1502（彻底移除端口映射特性）的列结构（12 列），未恢复 host_ports"
metrics:
  duration: "< 5 分钟"
  completed: 2026-05-13
---

# Quick 260513-gii 修复总结：UpsertHost SQL 列数与占位符对齐

## 一句话总结

修复 `Repository.UpsertHost` INSERT 语句末尾多余的 `$13` 占位符及 `DO UPDATE SET` 中残留的空白行，恢复 `POST /v1/admin/hosts` 创建主机接口。

## 背景

commit `74c1502`（"彻底移除端口映射特性"）从 `hosts` 表移除了 `host_ports` 列，并相应缩短了 Go 调用参数（12 项），但 SQL 字符串未同步：

- VALUES 子句仍写着 `$1...$13`（13 个占位符，对应 12 列）
- `host_mounts = EXCLUDED.host_mounts,` 与 `updated_at = NOW()` 之间留下一行只含 tab 的空白残行

后果：调用 `POST /v1/admin/hosts` 时 PostgreSQL 报 `INSERT has more expressions than target columns`，控制面返回 500 `{"error":"create host failed"}`。

## 修改位置

文件：`internal/store/repository/queries.go`

| 位置 | 修改前 | 修改后 |
|------|--------|--------|
| L382 | `VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)` | `VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)` |
| L393–L394 | `host_mounts = EXCLUDED.host_mounts,` + 空白残行 + `updated_at = NOW()` | `host_mounts = EXCLUDED.host_mounts,` + `updated_at = NOW()`（紧邻） |

## 修复前后片段对比

修复前：

```sql
INSERT INTO hosts (user_id, status, short_id, template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (user_id, slot_key)
DO UPDATE SET
    ...
    host_mounts = EXCLUDED.host_mounts,
    <-- 这里残留一行只含 tab 的空白行 -->
    updated_at = NOW()
```

修复后：

```sql
INSERT INTO hosts (user_id, status, short_id, template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (user_id, slot_key)
DO UPDATE SET
    ...
    host_mounts = EXCLUDED.host_mounts,
    updated_at = NOW()
```

未改动：列名、列顺序、Go 参数顺序、Scan 顺序、`RETURNING` 子句、错误处理。

## 验证结果

| 项 | 命令 | 实际输出 |
|----|------|----------|
| 占位符自检 | `grep -n 'VALUES (\$1' internal/store/repository/queries.go` | L382 行尾为 `$12)`，无 `$13` |
| 残留空白行自检 | `grep -A1 'host_mounts = EXCLUDED.host_mounts,' ...` | 下一行直接是 `updated_at = NOW()` |
| 静态检查 | `go vet ./internal/store/...` | 退出码 0（输出仅 `VET_OK`） |
| 编译 | `go build ./internal/store/...` | 退出码 0（输出仅 `BUILD_OK`） |
| 单元测试 | `go test ./internal/store/repository/... -count=1` | `ok  github.com/zanel1u/cloud-cli-proxy/internal/store/repository  0.600s` |

## 提交

- `04636fd` — `fix(260513-gii): align UpsertHost VALUES placeholder count with column list`
  - `internal/store/repository/queries.go`（+1 / -2）

## 关联上下文

- 引入缺陷的提交：`74c1502` "彻底移除端口映射特性"
- 受影响接口：`POST /v1/admin/hosts`
- 验证渠道：本地 Postgres 启动后直接调用创建主机接口，应不再返回 500（运行时验证留给手动回归）

## Deviations from Plan

无 — 按计划精确完成两处修改，未触发任何 Rule 1/2/3 自动修复，也无 Rule 4 架构决策。

## Self-Check: PASSED

- 文件存在：`internal/store/repository/queries.go` — FOUND
- 提交存在：`04636fd` — FOUND（`git log --oneline --all` 已确认）
- SUMMARY 文件：本文件路径 `.planning/quick/260513-gii-upserthost-sql/260513-gii-SUMMARY.md`
