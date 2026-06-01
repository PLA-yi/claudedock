# Phase 59: Admin 前端嵌入 - Context

**Gathered:** 2026-06-01
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped — implementation path clear)

<domain>
## Phase Boundary

将 admin 前端构建产物嵌入 Go 二进制，由 control-plane 直接提供静态文件服务，去掉独立 nginx/admin 容器。

**In scope:**
- `//go:embed web/admin/dist/*` 嵌入 Go 二进制
- SPA fallback handler — 非 API 路径先匹配静态文件，未命中返回 index.html
- router.go 注册静态文件 handler，API 路由 `/v1/*` 优先级高于静态文件
- vite.config.ts 代理 target 改为 `127.0.0.1:8080`

**Not in scope:** Admin 前端功能变更、E2E 测试、managed-user 容器。

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion

以下决策由 Claude 自行确定：

### 嵌入策略
- **D-01:** 使用 `embed.FS` 子目录嵌入 `web/admin/dist/*`，Go 1.26 原生支持。
- **D-02:** 通过 `fs.Sub(fs, "web/admin/dist")` 剥离路径前缀。

### 静态文件服务
- **D-03:** SPA fallback: 使用包装的 `http.FileSystem`，文件存在则返回，不存在则返回 `index.html`。
- **D-04:** 路由注册在 router.go 中，API 路由先注册，静态文件 handler 用 `http.FileServer` 最后注册。

### 开发配置
- **D-05:** vite.config.ts proxy target 改为 `http://127.0.0.1:8080`。

</decisions>

<canonical_refs>
## Canonical References

- `.planning/REQUIREMENTS.md` — UI-01 ~ UI-04 需求定义
- `.planning/ROADMAP.md` — Phase 59 成功标准
- `.planning/codebase/COMPONENTS.md` — router.go 当前路由结构
- `web/admin/vite.config.ts` — 当前 Vite 配置
- `internal/controlplane/http/router.go` — 路由定义

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/controlplane/http/router.go:43` — 路由注册结构，`mux.Handle` 模式
- `web/admin/vite.config.ts` — Vite 配置，当前 proxy target 为 `127.0.0.1:8090`

### Integration Points
- router.go 中 API 路由（`/v1/*`, `/healthz` 等）先注册，静态文件最后注册
- `cmd/control-plane/main.go` — 入口，可能需要添加 embed 变量声明

</code_context>

<specifics>
## Specific Ideas

无特殊要求 — 标准 embed + FileServer 模式。

</specifics>

<deferred>
## Deferred Ideas

None

</deferred>

---

*Phase: 59-Admin 前端嵌入*
*Context gathered: 2026-06-01*
