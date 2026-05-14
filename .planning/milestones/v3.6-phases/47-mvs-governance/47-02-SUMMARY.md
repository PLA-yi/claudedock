---
phase: 47-mvs-governance
plan: 02
title: 出口 IP 双绑互斥 (MVS-07)
status: shipped
mvs: MVS-07
created: 2026-05-14
---

# Phase 47 Plan 02 — SUMMARY

## 实际落地

### 新增 / 修改文件

- `tests/e2e/helpers.go`（无 build tag）：
  - 新增 `BindEgressIPResponse` 结构 + `ParseBindEgressIPResponse(status, body) (BindEgressIPResponse, error)` 纯函数。
  - 新增 `EgressIPDoubleBindContract` 锁定表：`WantStatus=409, WantErrSubstring="already bound"`。
- `tests/e2e/helpers_test.go`（无 build tag）：
  - 新增 5 个纯函数单测：`TestHelpersEgressIPDoubleBindContract_Locked`、`TestHelpersParseBindEgressIPResponse_{Success2xx,ConflictWithError,EmptyBody,NonJSONBody}`。
- `tests/e2e/helpers_linux.go`（`e2e && linux`）：
  - 新增 `(*GoldenPath).AdminLogin(ctx) (token string, err error)`：POST `/v1/auth/login`，token 来自 `E2E_ADMIN_USERNAME / E2E_ADMIN_PASSWORD` 环境变量（默认 `admin/admin-pw`）。
  - 新增 `(*GoldenPath).PostBindEgressIP(ctx, hostID, egressIPID) (BindEgressIPResponse, error)`：POST `/v1/admin/bindings`，Bearer header。
  - 新增 `(*GoldenPath).QueryBindingExists(ctx, hostID, egressIPID) (bool, error)`：直查 `host_egress_bindings` 表。
- `tests/e2e/egress_ip_binding_test.go`（`e2e && linux`，新文件）：
  - `TestEgressIPBinding_DoubleBindExcluded` 主用例。
  - 内部 helper `setupEgressBindingFixture`：直接 INSERT 一行 stopped host B + 一行 egress_ip X（跳过 admin POST hosts 的异步 ensure-image / create-container 任务路径）。
  - 4 段断言：status 4xx / error 含子串 / A 原绑定不破坏 / B 不被意外写入。

### 关键设计

- **断言策略「分级降级」**：理想情况 status=409。如果 backend 返回 4xx 但不是 409，记 `t.Logf` 部分通过；如果 5xx，记 `t.Errorf` backend gap；如果 2xx（完全无约束），记 `t.Fatalf` backend 缺失。这是「写期望，记录现状」的策略，让 Linux runner 上的测试结果能同时反映「现在到哪儿了」和「应该到哪儿」。
- **直查 DB 验证 A 原绑定不破坏**：不走 admin API（schema 演进风险高），直查 `host_egress_bindings` 表。
- **fixture 直 INSERT**：admin POST hosts 走 `runtimeService.QueueHostAction` → 异步 ensure-image 任务 → 不可控 fixture 时间窗。本 plan 只测 bindings handler，直接 INSERT 一行 stopped host 是合理的测试 hack。

## 与 PLAN 偏差

- PLAN 草案曾建议「调 admin API 再创建 host B」，落地改为「直接 DB INSERT」。原因如上：避免 ensure-image 异步任务污染 fixture。
- PLAN 草案的 `AdminLogin` 复用 Phase 46 既有通路；落地时发现 Phase 46 没有把 admin login 提到 GoldenPath 方法层，本 plan 首次落地该方法（供 Phase 47..52 共享）。
- 断言策略由「PASS / FAIL 二值」改为「PASS / PARTIAL PASS / BACKEND GAP / BACKEND FAIL 四级」，更准确反映当前 backend 状态。

## ROADMAP / CONTEXT 偏差

| ROADMAP/CONTEXT 草案 | 源码真相 | 处置 |
|---------------------|----------|------|
| 「同一出口 IP 第二次绑定必须 4xx + 稳定错误码」（ROADMAP §Phase 47 §Details 2、CONTEXT §Area 2） | `host_egress_bindings` 表仅 `UNIQUE (host_id, egress_ip_id)` 复合键（`0001_initial.sql:39`），**无 `egress_ip_id` 单列 UNIQUE**；`admin_bindings.go::Bind` 也没有 pre-check。当前路径上「同一 IP 绑给 B」很可能 INSERT 成功（行级合法）；最坏情况返回 2xx。 | 测试写「期望行为」（status=409 + already bound），断言策略分级；Linux runner 跑出 backend gap 时记录到 SUMMARY 与 VERIFICATION，要求 Phase 51 QUAL-04 修源码。 |
| 「稳定 error code 常量（如 ErrCodeEgressIPAlreadyBound）」（CONTEXT §Area 2、§Specifics） | `admin_bindings.go` 所有错误用 `{"error":"自由文本"}`，**无任何枚举码常量** | `BindEgressIPResponse.ErrorMessage` 取自由文本子串；SUMMARY 提议 Phase 51 引入稳定 code（命名建议 `egress_ip_already_bound`，类似 `internal/controlplane/http/bootstrap_errors.go` 风格）。 |
| 「不 DB 直接插冲突行（绕过 API 层会漏掉应用层校验）」（CONTEXT §Area 2） | 本 plan 主断言路径**严格**走 admin API `POST /v1/admin/bindings`；fixture 阶段的 host B / egress_ip X 直插 DB 是数据准备而非互斥校验绕过。 | 满足 CONTEXT 决策。 |

## 建议 backend 修源码（Phase 51 QUAL-04 跟进）

```go
// internal/controlplane/http/admin_bindings.go::Bind 增加 pre-check
existingHostID, err := h.store.GetHostByEgressIPID(r.Context(), req.EgressIPID)
if err == nil && existingHostID != "" {
    writeJSON(w, nethttp.StatusConflict, map[string]string{
        "error":      "egress IP already bound to host " + existingHostID,
        "error_code": "egress_ip_already_bound",  // 新增枚举常量
    })
    return
}
```

并补 `0020_egress_ip_unique_binding.sql` migration 在 DB 层加 `UNIQUE (egress_ip_id) WHERE deleted_at IS NULL`（如有软删字段）作为最后防线。

## Linux 真机验证项（deferred-to-CI）

- `TestEgressIPBinding_DoubleBindExcluded` 在 Scenario.Start Step 2..7 全部真实实现下跑通。
- 关键依赖：Phase 46 Plan 01 Step 3 admin login fixture 通路（admin user 写入 + JWT secret 生效）。
- 预期 Linux runner 结果（基于当前 backend 状态）：测试**会失败**，作为 backend gap 的真实证据。这是 CONTEXT §Area 2 的预期路径（「列为 Linux runner deferred + 在 SUMMARY 标记需修源码」）。

## darwin 本地验证

- `go build ./tests/e2e/...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./tests/e2e/ -run "Helpers" -count=1` PASS（5 个新增纯函数单测 + 5 个 Plan 01 单测）。
- `bash scripts/lint-no-bare-sleep.sh` PASS。

## 给 Phase 48..52 的接口约定

- `(*GoldenPath).AdminLogin(ctx) (token, err)` —— 任何需要 admin Bearer header 的用例直接复用。
- `(*GoldenPath).PostBindEgressIP / QueryBindingExists` —— Phase 48..50 网络治理用例可直接调用。
- `BindEgressIPResponse` + `ParseBindEgressIPResponse` —— 所有 admin POST 类 4xx error message 解析的标准模板（其它资源可仿造，命名一致即可）。
- `EgressIPDoubleBindContract` —— Phase 51 修源码时把 `WantStatus / WantErrSubstring` 作为对齐目标。
- `setupEgressBindingFixture` 模式 —— 后续 plan 若要测某个 handler 而不想触发完整 admin POST 流程的副作用，可仿照本 helper 直接 INSERT fixture。
