---
phase: 51-qual-harden
plan: 51-09
title: 双绑 API pre-check + error code
status: completed
completed: 2026-05-14
gap_closure:
  - phase-47-gap-d-47-3
---

# 51-09 — 双绑互斥 API pre-check + 稳定 error code — SUMMARY

## 变更

- `internal/store/repository/queries.go`：新增 `GetBindingHostIDByEgressIP(ctx, egressIPID) (string, error)`，SQL `SELECT host_id::text FROM host_egress_bindings WHERE egress_ip_id=$1 LIMIT 1`；row 不存在透传 `pgx.ErrNoRows`。
- `internal/controlplane/http/admin_bindings.go`：
  - `AdminBindingStore` interface 加 `GetBindingHostIDByEgressIP` 方法。
  - 新增 `const ErrCodeEgressIPAlreadyBound = "egress_ip_already_bound"`。
  - `Bind()` 处理函数在 host running 闸通过后、`BindEgressIPToHost` 之前插入双绑互斥 pre-check：
    - lookup err 非 `pgx.ErrNoRows` → 500 `"check existing binding failed"`。
    - lookup ok 且 `existingHostID != req.HostID` → **409 Conflict**，响应体含：
      - `error`: 中文 + 英文双语 `"出口 IP 已绑定到其它宿主机 (egress IP already bound to another host)"`
      - `error_code`: `"egress_ip_already_bound"`
      - `host_id`: 实际占用的 host
      - `egress_ip_id`: 回显请求体
    - 同 host 重新绑定同 IP：跳过 pre-check，走原 `BindEgressIPToHost` 路径，由表 `UNIQUE (host_id, egress_ip_id)` 复合键兜底重复 row（保持幂等）。
- `internal/controlplane/http/admin_bindings_test.go`：
  - `stubBindingStore` 加 `existingEgressHostID` / `existingEgressErr` 字段 + `GetBindingHostIDByEgressIP` stub 实现（默认返回 `pgx.ErrNoRows` → pre-check 透明跳过，既有 8 个 case 零回归）。
  - 新增 2 个 table-driven case：「双绑互斥 409」「同 host 重新绑定 201 幂等」。
  - 新增 `TestAdminBindings_DoubleBind_ErrorCode`：专门断言 409 响应同时携带 `error_code = "egress_ip_already_bound"`、中文「已绑定」+ 英文 `"already bound"` 双子串、`host_id`、`egress_ip_id` 四字段。

## 闸

- `go build ./...` PASS。
- `GOOS=linux go build ./...` + `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go vet ./...` PASS。
- `go test ./internal/controlplane/http/... -run "AdminBinding"`：11 个 PASS（含 2 case + 1 dedicated test）。
- `go test ./... -count=1`：19 包全 PASS。

## Phase 47 用例自动闭环

- `tests/e2e/helpers.go::EgressIPDoubleBindContract{WantStatus:409, WantErrSubstring:"already bound"}` 锁定的契约：本 plan 响应 status=409 + error 含 `"already bound"` 英文子串 → 命中。
- `tests/e2e/helpers.go::ParseBindEgressIPResponse(status, body)` 解析路径：响应体 `error` / `error_code` 双字段满足 helpers 既有 `ErrorMessage` / `ErrorCode` 解析；Phase 47 `TestHelpersParseBindEgressIPResponse_ConflictWithError` 单测预期由 PARTIAL → PASS（已为「错误码字段实际填充」）。
- `tests/e2e/egress_ip_binding_test.go::TestEgressIPBinding_DoubleBindExcluded`：Linux runner 真机预期由 BACKEND GAP 分支 → PASS 分支（fixture 锁定 status=409 + ErrorMessage 含 `"already bound"`）。

## 偏差

- response body 同时用中文 + 英文双语，便于 admin 前端中文显示 + e2e 英文断言。CONTEXT §Area 6 写「中文 message」，本 plan 落地为「中文 + 英文双语」更严谨地兼容 Phase 47 `WantErrSubstring:"already bound"` 锁定契约，没有破坏中文沟通约定（中文文本在前）。

## 风险闭环

- 不交叉 51-01..08；仅改 admin_bindings.go / queries.go / admin_bindings_test.go。
- 既有 stub 默认行为返回 `pgx.ErrNoRows` → pre-check 透明跳过，所有 8 个既有 `TestAdminBindingsHandler` case 零回归。
- 并发 race（同时两个 admin 请求同 egress IP 绑不同 host）不在本 plan 范围（CONTEXT §Deferred 锁定）；当前 pre-check 是 best-effort，最终一致性靠表 UNIQUE 复合键兜底。
