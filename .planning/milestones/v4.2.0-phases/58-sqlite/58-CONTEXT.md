# Phase 58: SQLite 数据库层迁移 - Context

**Gathered:** 2026-06-01
**Status:** Ready for planning

<domain>
## Phase Boundary

将整个持久化层从 PostgreSQL 切换至 SQLite，包括：
- 移除 pgx/v5 依赖，引入 modernc.org/sqlite
- 重写 migrator 支持 SQLite 语法 + embed.FS
- 23 个迁移文件从 PG 语法改写为 SQLite 语法
- Repository 层 140+ 处查询从 pgxpool.Pool 改为 database/sql 标准库
- App 初始化改为 sql.Open("sqlite", ...) + PRAGMA 配置

不涉及：数据迁移工具、E2E 测试适配、managed-user 容器变更。

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion

用户明确表示所有实现决策由 Claude 自行决定。以下为锁定的实现方案：

### 迁移文件策略
- **D-01:** 保留原编号（0001-0023），从 PG 语法改写为 SQLite 语法。保持迁移历史一致性，便于追溯。
- **D-02:** 使用 embed.FS 嵌入迁移文件，migrator 按序号排序执行。

### 类型映射策略
- **D-03:** UUID → TEXT，Go 侧使用 `github.com/google/uuid` 生成，存储为字符串。
- **D-04:** TIMESTAMPTZ → TEXT，存储 ISO 8601 格式字符串，Go 侧 `time.Time` 自动序列化。
- **D-05:** JSONB → TEXT，存储 JSON 字符串，Go 侧 `encoding/json` 序列化/反序列化。
- **D-06:** BOOLEAN → INTEGER（0/1），SQLite 标准做法。
- **D-07:** SERIAL/BIGSERIAL → INTEGER PRIMARY KEY AUTOINCREMENT。

### 并发写入策略
- **D-08:** WAL 模式 + busy_timeout=5000 已足够。通过 `database/sql` 的 `SetMaxOpenConns(1)` 确保单写入者，避免 SQLITE_BUSY 错误。读连接可并发。
- **D-09:** 连接配置：`sql.Open("sqlite", dsn)` + `PRAGMA journal_mode=WAL` + `PRAGMA foreign_keys=ON` + `PRAGMA busy_timeout=5000` 在连接建立时执行。

### Repository 重构幅度
- **D-10:** 纯替换 — 保持 Repository 结构体和方法签名不变，仅将内部实现从 pgxpool.Pool 改为 *sql.DB。最小化变更范围，降低回归风险。
- **D-11:** pgx 特有的 `QueryRow().Scan()` 模式改为 `db.QueryRowContext().Scan()`，`Query()` 改为 `db.QueryContext()`，`Exec()` 改为 `db.ExecContext()`。

### 驱动选择
- **D-12:** 使用 `modernc.org/sqlite`（纯 Go 实现，无 CGO 依赖）。不使用 `github.com/mattn/go-sqlite3`（需要 CGO，增加构建复杂度）。

### UUID 处理
- **D-13:** 现有 PG 的 `id::text` 类型转换不再需要，SQLite TEXT 列直接存储 UUID 字符串。INSERT 时由 Go 侧 `uuid.New().String()` 生成。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 需求与路线图
- `.planning/REQUIREMENTS.md` — DB-01 ~ DB-05 需求定义和验收标准
- `.planning/ROADMAP.md` — Phase 58 成功标准（5 项）

### 现有代码（需改写的文件）
- `internal/store/repository/queries.go` — Repository 层核心，140+ 处 pgx 调用
- `internal/store/repository/models.go` — 数据模型定义
- `internal/store/migrator/migrator.go` — 迁移执行器
- `internal/store/migrations/*.sql` — 23 个 PG 迁移文件
- `internal/controlplane/app/app.go` — App 初始化（pgxpool 创建、迁移执行）
- `cmd/control-plane/main.go` — 控制面入口
- `cmd/host-agent/main.go` — 宿主机代理入口（也连接 PG）

### 依赖参考
- `.planning/codebase/DEPENDENCIES.md` — 当前 pgx/v5 依赖详情
- `.planning/codebase/COMPONENTS.md` — store 层组件结构

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/store/repository/models.go` — 数据模型结构体可直接复用，无需修改
- `internal/store/migrations/` — 迁移文件逻辑可参考，语法需改写

### Established Patterns
- Repository 使用 `*pgxpool.Pool` 作为数据库连接，通过 `New(db *pgxpool.Pool)` 注入
- 迁移文件按序号命名（0001-0023），migrator 按序执行
- 所有查询使用 `context.Context` 作为第一个参数
- 错误处理使用 `fmt.Errorf("operation: %w", err)` 包装模式

### Integration Points
- `app.go` 创建 pgxpool 并注入 Repository → 改为 sql.Open + 注入 *sql.DB
- `host-agent/main.go` 也创建 pgxpool → 同样改为 sql.Open
- `migrator.go` 从嵌入的 FS 读取 SQL 文件执行 → 需适配 SQLite 语法
- Scheduler（expiry, reconcile）通过 Repository 查询 → 无需修改（签名不变）

</code_context>

<specifics>
## Specific Ideas

无特殊要求 — 采用标准 SQLite 最佳实践。

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 58-SQLite 数据库层迁移*
*Context gathered: 2026-06-01*
