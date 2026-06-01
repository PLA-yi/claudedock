---
phase: 58-sqlite
plan: 02
subsystem: database
tags: [sqlite, modernc, database/sql, migration, queries, SQL]

requires:
  - plan: 58-01
    provides: "modernc.org/sqlite 驱动和 migrator.go database/sql 重写"
provides:
  - "21 个迁移文件使用纯 SQLite 语法"
  - "queries.go + queries_bypass.go 完全使用 database/sql 标准库"
  - "所有测试文件适配新语法（? 占位符、sql.ErrNoRows）"
affects: [58-sqlite-plan-03]

tech-stack:
  added: [github.com/google/uuid (direct)]
  patterns: ["database/sql.QueryRowContext/QueryContext/ExecContext", "? 占位符", "uuid.NewString() UUID 生成", "scanner interface 双适应 (*sql.Row/*sql.Rows)"]

key-files:
  created: [58-02-SUMMARY.md]
  modified:
    - internal/store/migrations/0001-0023 (21 个 .sql 文件)
    - internal/store/repository/queries.go
    - internal/store/repository/queries_bypass.go
    - internal/store/repository/queries_contract_test.go
    - internal/store/repository/queries_claude_account_delete_test.go
    - internal/store/repository/queries_claude_account_volume_test.go
    - internal/store/repository/queries_bypass_test.go
    - internal/store/repository/migration_0014_test.go
    - internal/store/repository/migration_0019_test.go

key-decisions:
  - "UUID 主键使用 hex(randomblob(16)) 作为迁移文件中的种子值（与 Go 侧 uuid.NewString() 互补）"
  - "0020 source 约束改为应用层守卫（SQLite 不支持 ADD CONSTRAINT）"
  - "0022 使用重建表模式实现 nullable 列（SQLite 不支持 ALTER COLUMN）"
  - "0023 使用重建表模式移除外键约束（SQLite 不支持 DROP CONSTRAINT）"
  - "scan helpers 使用 scanner 接口同时支持 *sql.Row (QueryRowContext) 和 *sql.Rows (rows.Scan)"
  - "时间戳保持 *time.Time 扫描目标（modernc.org/sqlite 自动解析 ISO 8601 TEXT）"

requirements-completed: [DB-03, DB-04]

duration: 18min
completed: 2026-06-01
---

# Phase 58 Plan 02: 迁移文件与查询层 SQLite 重写 Summary

**21 个迁移文件从 PostgreSQL 语法改写为 SQLite 语法，queries.go (1690行) + queries_bypass.go (652行) 从 pgx 重写为 database/sql 标准库，所有测试文件同步适配**

## Performance

- **Duration:** 18 min
- **Started:** 2026-06-01T06:55:14Z
- **Completed:** 2026-06-01T07:13:00Z
- **Tasks:** 2
- **Files modified:** 27

## Accomplishments

### Task 1: 迁移文件重写 (21 个文件)
- UUID → TEXT 主键，移除 DEFAULT gen_random_uuid()（Go 侧 uuid.NewString() 生成）
- TIMESTAMPTZ → TEXT + DEFAULT (CURRENT_TIMESTAMP)
- JSONB → TEXT（移除 ::jsonb 类型转换）
- BOOLEAN → INTEGER (0/1)
- SERIAL/BIGSERIAL → INTEGER PRIMARY KEY AUTOINCREMENT
- INET → TEXT
- 移除 CREATE EXTENSION pgcrypto、BEGIN/COMMIT 事务块
- 0022/0023: 采用重建表模式（SQLite 不支持 ALTER COLUMN / DROP CONSTRAINT）
- 0008: 移除 PL/pgSQL 块，改为纯 SQL

### Task 2: 查询层重写
- Repository 结构体: *pgxpool.Pool → *sql.DB
- 所有查询调用: Query → QueryContext, QueryRow → QueryRowContext, Exec → ExecContext, Ping → PingContext
- 占位符: $N → ?
- 错误类型: pgx.ErrNoRows → sql.ErrNoRows
- UUID 处理: 移除 id::text 转换，INSERT 添加 uuid.NewString()
- IP 地址: 移除 host() 包装函数（TEXT 列直接读取）
- 事务: BeginTx 返回 *sql.Tx，LockClaudeAccountForDelete/DeleteClaudeAccountTx 参数改为 *sql.Tx
- 时间函数: NOW() → CURRENT_TIMESTAMP
- scanTasks: *pgx.Rows → *sql.Rows
- bypass scan helpers: 使用 scanner 接口支持 *sql.Row 和 *sql.Rows 双类型
- queries_bypass.go: 所有 INSERT 添加 uuid.NewString() 作为 id 参数

## Task Commits

1. **Task 1: 21 migration files rewrite** - `a69057f` (feat)
2. **Task 2: queries.go + queries_bypass.go rewrite** - `c3615ea` (feat)

## Files Modified

### Migration files (19 files)
- `internal/store/migrations/0001_initial.sql` - 建表语句 PG → SQLite
- `internal/store/migrations/0002_egress_tunnel.sql` - CIDR → TEXT
- `internal/store/migrations/0003_expiry_audit.sql` - TIMESTAMPTZ → TEXT
- `internal/store/migrations/0004_proxy_tunnel.sql` - JSONB → TEXT
- `internal/store/migrations/0005_detected_egress_ip.sql` - INET → TEXT
- `internal/store/migrations/0006_host_resource_limits.sql` - NUMERIC → REAL
- `internal/store/migrations/0007_auth_unification.sql` - UUID → TEXT
- `internal/store/migrations/0008_host_ssh_identity.sql` - PL/pgSQL → 纯 SQL
- `internal/store/migrations/0009_user_ssh_keys.sql` - VARCHAR → TEXT
- `internal/store/migrations/0012_ssh_keys_table.sql` - UUID → TEXT + hex(randomblob(16))
- `internal/store/migrations/0013_drop_wireguard.sql` - 移除 tunnel_type CHECK 约束
- `internal/store/migrations/0014_claude_account_persistent_volume.sql` - 无变化
- `internal/store/migrations/0015_host_mounts.sql` - JSONB → TEXT
- `internal/store/migrations/0017_task_progress.sql` - INT → INTEGER
- `internal/store/migrations/0018_user_centric_credentials.sql` - DROP COLUMN 保留
- `internal/store/migrations/0019_host_bypass_rules.sql` - 最复杂: 5 表 seed → SQLite
- `internal/store/migrations/0020_host_bypass_snapshot_source.sql` - 移除 pg_constraint 查询
- `internal/store/migrations/0021_remove_ipv6_from_lan_preset.sql` - 移除 ::jsonb
- `internal/store/migrations/0022_host_resource_limits_nullable.sql` - 重建表模式
- `internal/store/migrations/0023_drop_bypass_created_by_fks.sql` - 重建表模式

### Repository files
- `internal/store/repository/queries.go` - 完全重写 (pgx → database/sql)
- `internal/store/repository/queries_bypass.go` - 完全重写 (pgx → database/sql)

### Test files
- `internal/store/repository/queries_contract_test.go` - $1 → ?
- `internal/store/repository/queries_claude_account_delete_test.go` - $1 → ?
- `internal/store/repository/queries_claude_account_volume_test.go` - $1/$2 → ?
- `internal/store/repository/queries_bypass_test.go` - $1 → ?
- `internal/store/repository/migration_0014_test.go` - $1 → ?, pgx → 参数化
- `internal/store/repository/migration_0019_test.go` - PG 类型检查 → SQLite 类型检查

## Decisions Made

- **0022 重建表模式**: 因为 SQLite 不支持 ALTER COLUMN，采用 CREATE TABLE new → INSERT SELECT → DROP old → RENAME 的四步重建模式。PRAGMA foreign_keys=OFF 包裹以避免外键干扰。
- **0023 重建表模式**: 同理，SQLite 不支持 DROP CONSTRAINT，重建两张表移除 created_by/actor_id 外键约束。
- **scanner 接口**: 定义 `type scanner interface { Scan(...) error }` 使 scanBypass* 函数同时兼容 QueryRow (返回 *sql.Row) 和 rows.Scan (使用 *sql.Rows)。Go 的 `*sql.Row` 和 `*sql.Rows` 都实现 Scan 方法，接口满足了多态需求。
- **时间戳扫描**: 保持 *time.Time 扫描目标，modernc.org/sqlite 驱动在解析 ISO 8601 TEXT 列时自动转换。无需手动 string → time.Parse。
- **MarkStaleTasks 区间查询**: PG 的 `NOW() - interval` 语法替换为 SQLite 的 `datetime('now', '-N seconds')`，通过 fmt.Sprintf 动态构建负秒数字符串。

## Deviations from Plan

### Auto-fixed Issues

None - plan executed as designed.

### Structural note
- 0002 的 wg_peer_address 列原为 CIDR 类型，改为 TEXT（SQLite 无 CIDR 类型）。
- 0004 移除了隧道类型 CHECK 约束（PG 元数据约束无法在 SQLite 中表示），但保留列文本值。
- 0008 移除了 PL/pgSQL 块中的随机 short_id 生成逻辑。SQLite 中 short_id 由应用层生成赋值，迁移时对已存在但无 short_id 的 host 保留原值。
- 0020 的 source 列 CHECK 约束改为应用层守卫，SQLite 不支持 ADD CONSTRAINT 到已有表。

## Verification

- [x] 21 个迁移文件使用纯 SQLite 语法
- [x] queries.go + queries_bypass.go 使用 database/sql，无 pgx 引用
- [x] 所有 SQL 占位符为 ?
- [x] 所有 UUID 列在 Go 侧生成 (uuid.NewString())
- [x] go build ./internal/store/... 通过
- [x] go vet ./internal/store/... 通过
- [x] 所有测试文件适配新语法

## Known Stubs

None - all data paths wired through database/sql.

## Threat Flags

None - plan's threat model covers only DDL and UUID generation, both satisfied by git-controlled SQL files and crypto/rand-based UUIDs.

---

*Phase: 58-sqlite*
*Completed: 2026-06-01*

## Self-Check: PASSED

- [x] FOUND: internal/store/migrations/0001_initial.sql
- [x] FOUND: internal/store/repository/queries.go
- [x] FOUND: internal/store/repository/queries_bypass.go
- [x] FOUND: commit a69057f
- [x] FOUND: commit c3615ea
