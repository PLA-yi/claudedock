# Requirements: Cloud CLI Proxy

**Milestone:** v4.2.0 容器合并 · SQLite 迁移 · 配置统一
**Core Value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP

## v4.2.0 Requirements

本次里程碑需求：将部署从 5 服务精简至 2 服务，数据库从 PostgreSQL 切换至 SQLite，admin 前端与 sing-box 探针均内嵌至 control-plane 单一二进制。

### DB — 数据库层迁移（PostgreSQL → SQLite）

- [ ] **DB-01**: 依赖切换 — 移除 pgx/v5，引入 modernc.org/sqlite，go mod tidy 后编译通过
- [ ] **DB-02**: 迁移系统重写 — migrator 支持 SQLite 语法 + embed.FS 嵌入迁移文件
- [ ] **DB-03**: 24 个迁移文件改写为 SQLite 语法（TEXT 替代 UUID/TIMESTAMPTZ/JSONB，Go 侧生成 UUID）
- [ ] **DB-04**: Repository 层重写 — pgxpool.Pool → *sql.DB，140+ 处 Query/QueryRow 调用改为标准库
- [ ] **DB-05**: App 初始化重写 — sql.Open("sqlite", ...) + PRAGMA WAL/foreign_keys/busy_timeout

### UI — Admin 前端嵌入

- [ ] **UI-01**: 使用 //go:embed 嵌入 web/admin/dist/* 到 Go 二进制
- [ ] **UI-02**: 实现 SPA fallback — 非 API 路径先匹配静态文件，未命中返回 index.html
- [ ] **UI-03**: router.go 注册静态文件 handler，API 路由优先级高于静态文件
- [ ] **UI-04**: vite.config.ts 代理 target 从 127.0.0.1:8090 改为 127.0.0.1:8080

### PRB — Sing-box 探针内嵌

- [ ] **PRB-01**: control-plane Dockerfile 运行阶段下载 sing-box v1.13.3 二进制到 /usr/local/bin/
- [ ] **PRB-02**: 探针优先级调整 — startLocalSingBox 优先使用宿主机二进制，不存在才回退 Docker

### DEP — 部署精简与配置统一

- [ ] **DEP-01**: docker-compose.yml 移除 postgres、admin、sing-box 三个服务及 cloudproxy-postgres volume
- [ ] **DEP-02**: control-plane 服务改为 DATABASE_URL=file:/data/cloud-cli-proxy.db + ./data:/data volume
- [ ] **DEP-03**: .env / .env.example 移除 POSTGRES_* 变量，DATABASE_URL 改为 SQLite 路径，CONTROL_PLANE_ADDR 统一为 :8080
- [ ] **DEP-04**: Makefile 移除 db/db-stop/db-reset 目标，dev 目标不再检测 PostgreSQL
- [ ] **DEP-05**: deploy/scripts/setup-env.sh 和 deploy/scripts/deploy.sh 移除 PostgreSQL 交互和检查

### DOC — 文档同步

- [ ] **DOC-01**: README.md / README.en.md 更新架构图、环境变量表、访问地址
- [ ] **DOC-02**: docs/zh/guide/ 和 docs/en/guide/ 更新 architecture、deployment、quickstart、configuration
- [ ] **DOC-03**: docs/zh/reference/faq.md 和 docs/en/reference/faq.md 更新 PG 相关排障条目

### TEST — 测试适配

- [ ] **TEST-01**: internal/store/repository/*_test.go 从 testcontainers PG 切换到内存 SQLite
- [ ] **TEST-02**: internal/controlplane/app/app_test.go 更新配置常量，确认编译和测试通过

## v2 Requirements

None for this milestone.

## Out of Scope

| Feature | Reason |
|---------|--------|
| managed-user 容器变更 | 用户容器架构不受影响 |
| E2E 测试体系适配 | 本次仅保证单元测试通过，e2e 后续处理 |
| 多宿主机支持 | v1 仅单宿主机 |
| 数据迁移工具（PG → SQLite） | 新部署直接用 SQLite，不处理旧数据 |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| DB-01 | 58 | Pending |
| DB-02 | 58 | Pending |
| DB-03 | 58 | Pending |
| DB-04 | 58 | Pending |
| DB-05 | 58 | Pending |
| UI-01 | 59 | Pending |
| UI-02 | 59 | Pending |
| UI-03 | 59 | Pending |
| UI-04 | 59 | Pending |
| PRB-01 | 60 | Pending |
| PRB-02 | 60 | Pending |
| DEP-01 | 61 | Pending |
| DEP-02 | 61 | Pending |
| DEP-03 | 61 | Pending |
| DEP-04 | 61 | Pending |
| DEP-05 | 61 | Pending |
| DOC-01 | 62 | Pending |
| DOC-02 | 62 | Pending |
| DOC-03 | 62 | Pending |
| TEST-01 | 62 | Pending |
| TEST-02 | 62 | Pending |

**Coverage:**

- v4.2.0 requirements: 21 total
- Mapped to phases: 21
- Unmapped: 0 ✓

---
*Requirements defined: 2026-06-01*
*Last updated: 2026-06-01 after initial definition*
