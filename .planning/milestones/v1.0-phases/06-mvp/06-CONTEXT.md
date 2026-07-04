# Phase 6: 加固与 MVP 就绪 - Context

**Gathered:** 2026-03-27
**Status:** Ready for planning

<domain>
## Phase Boundary

本阶段将产品从"功能完成"推进到"可交付给真实用户"。具体包括三个维度：为核心启动、网络、到期和后台流程增加端到端冒烟测试；编写面向运维的单宿主机部署手册和恢复清单；打磨终端与后台体验细节，确保常见失败场景都有清晰反馈。本阶段不新增功能特性，不做多宿主机部署、计费或 Web Terminal。

</domain>

<decisions>
## Implementation Decisions

### 冒烟测试策略
- **D-01:** 测试分两层：需要真实 Docker daemon 的集成测试层（Go test + build tag `integration`），以及无需 Docker 的快速单元/API 层（Go httptest + mock）。
- **D-02:** 集成测试优先覆盖核心关键路径：bootstrap 认证 → 任务创建与状态流转 → 网络校验（出口 IP / DNS / 泄漏阻断）→ SSH 就绪门槛 → 后台管理 API CRUD。
- **D-03:** bootstrap 脚本层面使用 shell 测试脚本验证端到端流程（curl → 认证 → 轮询 → SSH handoff），确保脚本与 API 的错误码契约不漂移。
- **D-04:** 后台管理 API 测试使用 Go httptest 覆盖认证、用户 CRUD、出口 IP CRUD、绑定管理和主机生命周期操作的正常路径与异常路径。
- **D-05:** 到期与对账定时器的测试通过注入可控时间源和 mock 依赖进行验证，不依赖真实等待。

### 部署文档范围
- **D-06:** 目标读者为有 Linux 运维经验的技术人员，文档不需要从零解释 Docker 或 PostgreSQL 安装。
- **D-07:** 文档形式为 Markdown 文档与自动化部署脚本并行：手册提供理解与排障参考，脚本提供可执行的部署路径。
- **D-08:** 覆盖场景包括首次部署（环境准备 → 构建 → 配置 → 启动 → 验证）、日常运维（用户管理 → 主机运维 → 证书/密钥轮换 → 备份恢复）和常见故障排查。灾难恢复作为附录简要说明。
- **D-09:** 部署手册应覆盖：宿主机依赖检查（沿用 host-preflight.sh）、WireGuard 配置、PostgreSQL 初始化、控制面与 host-agent 的 systemd 部署、受管镜像构建、防火墙规则和 bootstrap 入口配置。

### 体验打磨方向
- **D-10:** 终端侧：审查 bootstrap 脚本所有失败路径，确保每个错误码都有清晰的中文提示和下一步建议（重试命令或联系管理员）；补齐网络超时、handoff 失败等边缘场景的提示。
- **D-11:** 后台侧：补齐表单校验反馈（必填字段、格式校验）、操作中 loading 状态指示器、列表空状态展示，确保管理员操作流程顺畅无死角。
- **D-12:** 运维侧：统一控制面和 host-agent 的结构化日志格式（slog JSON），完善 `/healthz` 端点使其包含数据库连接和 agent 可达性检查，为生产部署提供基础监控入口。

### 上线前检查清单
- **D-13:** 安全：审查所有 API 端点的权限边界（admin API 需 JWT、bootstrap API 需认证、公开端点仅限 healthz 和 script）；确认密码、JWT secret、WireGuard 私钥等敏感字段不在 API 响应或日志中泄露。
- **D-14:** 稳定性：确认控制面和 host-agent 支持优雅关闭（SIGTERM → 等待在途任务 → 关闭连接池）；确认 PostgreSQL 连接池有合理配置（max connections、idle timeout）；确认容器清理不遗留 orphan 资源。
- **D-15:** 可运维性：确认日志级别可通过环境变量调整；健康检查端点覆盖关键依赖；备份策略（PostgreSQL dump 周期）在文档中明确说明。

### Claude's Discretion
- 具体测试框架内的 test fixture 组织与 helper 函数设计。
- 文档的具体章节编排和格式细节。
- 日志字段的具体命名约定与采样策略。
- 健康检查端点的具体超时参数和降级行为。
- 前端体验打磨的具体交互动效与组件细节。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 阶段范围与验收标准
- `.planning/ROADMAP.md` — Phase 6 的目标、成功标准与 06-01/06-02/06-03 计划拆分。
- `.planning/REQUIREMENTS.md` — `ACCS-01`、`ACCS-03`、`NET-05`、`ADMN-03`、`ADMN-04` 的正式定义，以及 v1 全量需求追踪矩阵。
- `.planning/PROJECT.md` — 产品核心价值、约束与"优雅、好用、运维清晰"的产品优先级。

### 前置阶段锁定决策
- `.planning/phases/01-foundation-control-plane-host-agent/01-CONTEXT.md` — 控制面/host-agent 特权边界、异步任务模型、受管镜像、失败策略。
- `.planning/phases/02-tunnel-egress-enforcement/02-CONTEXT.md` — 出口 IP 绑定语义、三重校验门槛、DNS 受控路径、默认拒绝策略。
- `.planning/phases/03-ssh/03-CONTEXT.md` — Bootstrap 认证流程、SSH 就绪门槛、错误分类与退出码。
- `.planning/phases/04-admin-ui/04-CONTEXT.md` — JWT 管理认证、后台 UI 布局、用户/IP/绑定/主机管理。
- `.planning/phases/05-expiry-audit-cleanup/05-CONTEXT.md` — 到期定时器、事件类型约定、对账策略、事件日志展示。

### 部署与运维基础设施
- `deploy/scripts/host-preflight.sh` — 现有宿主机依赖检查脚本，Phase 6 需扩展或文档化。
- `deploy/bootstrap/cloud-bootstrap.sh` — 终端启动脚本，是体验打磨的关键对象。
- `deploy/compose/control-plane.dev.yml` — 开发环境 Docker Compose 配置，生产部署文档需独立于此。
- `deploy/systemd/claudedock-control-plane.service` — 控制面 systemd 服务单元文件。
- `deploy/systemd/claudedock-host-agent.service` — host-agent systemd 服务单元文件。
- `deploy/docker/managed-user/build-managed-image.sh` — 受管用户镜像构建脚本。

### 核心代码锚点
- `internal/controlplane/app/app.go` — 控制面启动入口，含调度器注册与优雅关闭逻辑。
- `internal/controlplane/http/bootstrap_errors.go` — 错误码与终端消息映射，冒烟测试和体验打磨的核心对象。
- `internal/controlplane/http/router.go` — API 路由注册与权限边界。
- `internal/agent/server.go` — host-agent 服务端，需确认健康检查与日志格式。

### 现有测试基线
- `internal/controlplane/http/bootstrap_auth_test.go` — Bootstrap 认证测试。
- `internal/controlplane/http/bootstrap_status_test.go` — 启动状态查询测试。
- `internal/controlplane/http/bootstrap_handoff_test.go` — SSH handoff 测试。
- `internal/controlplane/http/bootstrap_script_test.go` — Bootstrap 脚本下发测试。
- `internal/runtime/tasks/ssh_ready_test.go` — SSH 就绪检查测试。
- `internal/runtime/tasks/ssh_handoff_test.go` — SSH 交接测试。
- `internal/network/verify_test.go` — 网络校验测试。
- `internal/network/validate_test.go` — 网络验证测试。
- `internal/network/errors_test.go` — 网络错误类型测试。

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **9 个测试文件**已覆盖 bootstrap 认证/状态/handoff/脚本、SSH 就绪/交接和网络校验/验证/错误，可作为冒烟测试的扩展基础。
- **host-preflight.sh**：已检查 docker/ip/nft/wireguard/nsenter/curl，可直接嵌入部署手册或扩展为生产前检查。
- **cloud-bootstrap.sh**：终端全流程已完整（认证 → 轮询 → handoff → exec ssh），体验打磨在此基础上增补边缘场景。
- **systemd 服务文件**：控制面和 host-agent 的 service 单元已存在，部署文档可以引用并补充配置说明。
- **Docker Compose dev**：开发联调环境已可用，生产部署文档需说明从 dev 到 prod 的差异。
- **BootstrapErrorEntries**：错误码到中文消息和退出码的映射已作为唯一来源，冒烟测试可验证 API 与脚本的契约一致性。

### Established Patterns
- Go 控制面使用 `slog.TextHandler`（`app.go`），生产部署可统一切换为 JSON 格式。
- 测试文件使用标准 Go `testing` 包 + `httptest.NewServer`，无外部测试框架依赖。
- API 路由通过 `NewRouter` 集中注册，权限边界通过 `AdminConfig` nil 检查和 JWT middleware 实现。
- 优雅关闭已实现（ctx 取消 → server.Shutdown → 调度器停止），可验证其行为。

### Integration Points
- 冒烟测试需要能独立启动控制面 + PostgreSQL（可复用 dev compose 或 testcontainers）。
- 部署文档需串联 host-preflight → 镜像构建 → DB 初始化 → systemd 启动 → bootstrap 验证的完整链路。
- 健康检查扩展需在 `router.go` 现有 `/healthz` 端点基础上增加 DB 和 agent 连通性检查。

</code_context>

<specifics>
## Specific Ideas

- 冒烟测试的核心价值是"防止回归"：功能已全部完成（Phase 1-5），Phase 6 通过自动化测试为发布建立信心基线。
- 部署文档不是"理论架构说明"，而是可执行的检查清单：运维人员跟着做就能完成部署。
- 体验打磨遵循"所有失败都要有出路"原则：用户看到错误后必须知道下一步该做什么。
- 上线检查遵循"没有隐藏的人工步骤"原则：MVP 成功标准要求部署和运维不依赖开发者口头传授。

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 06-mvp*
*Context gathered: 2026-03-27*
