# Phase 56: CI paths 扩面 + Makefile 入口 - Context

**Gathered:** 2026-05-27
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure phase)

<domain>
## Phase Boundary

堵塞 v3.6 "偶尔改坏没拦住" 的 CI 触发盲区，新增 `make e2e` 一条命令本地入口。

- CI-01: `.github/workflows/e2e.yml` paths filter 扩面
- CI-02: `make e2e` 目标
- CI-03: `make e2e` 内 lint + vet

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase.
</decisions>

<code_context>
## Existing Code Insights

- `.github/workflows/e2e.yml` — 现有 CI workflow
- `Makefile` — 已清理 gateway-image（54-04）
- `go.mod` / `go.sum` — Go 依赖
</code_context>

<specifics>
## Specific Ideas

CI-01 paths: `internal/controlplane/http/**` / `internal/store/**` / `deploy/docker/**` / `Makefile` / `go.mod` / `go.sum`
make e2e: `go test -tags=e2e ./tests/e2e/... -count=1 -v -timeout=15m`
lint-no-bare-sleep: 检查 e2e 测试中没有裸 `time.Sleep` 或 `sleep`
</specifics>

<deferred>
## Deferred Ideas

None.
</deferred>
