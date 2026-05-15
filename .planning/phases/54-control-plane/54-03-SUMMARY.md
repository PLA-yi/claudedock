---
phase: 54-control-plane
plan: "03"
subsystem: api
tags: [admin-api, double-binding, contract-test, regression-guard, v4.0, ctrl-04]

# Dependency graph
requires:
  - phase: 51-rate-limit
    provides: 51-09 双绑互斥 pre-check（ErrCodeEgressIPAlreadyBound + 409 + 中英双语 message + host_id/egress_ip_id 字段回显）
  - phase: 54-control-plane
    provides: Plan 54-01 单容器化重构（container_proxy_provider 519 → 155 行，admin_bindings.go 业务代码 0 diff）
provides:
  - Test_Phase54_DoubleBindingContract_PreservedAfterSingleContainerRefactor 单元测试（CTRL-04 不变式锁）
  - 5 项契约保护断言（HTTP 409 / error_code / 中英双语 message / host_id 占用者回显 / egress_ip_id 请求回显）
  - admin_bindings.go 业务代码 0 diff 的 grep 守护（V2 三条 grep 命令）
affects: [54-04, 55, 56, v4.1]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "契约保护测试模式：以 Phase 显式命名 Test_PhaseXX_*Contract_PreservedAfter* 充当不变式锁，与原始落地用例并存（D-54-11）"
    - "字面量子串硬断言：中文「已绑定」+ 英文「already bound」双子串同时校验，文案变更必须双语同步"

key-files:
  created: []
  modified:
    - internal/controlplane/http/admin_bindings_test.go (+96 行，仅末尾追加 Test_Phase54_DoubleBindingContract_PreservedAfterSingleContainerRefactor)

key-decisions:
  - "D-54-11 落地：新增 Phase 54 显式命名契约保护测试，与 v3.6 TestAdminBindings_DoubleBind_ErrorCode 并存（设计意图，合并需走 Phase 56 / v4.1 显式 audit）"
  - "admin_bindings.go 业务代码严格 0 diff：本 plan 仅追加测试，业务文件不动（CTRL-04 核心要求）"
  - "测试断言绑定 ErrCodeEgressIPAlreadyBound 常量 + 字面量「egress_ip_already_bound」双重校验：常量改名或值改都会失败"

patterns-established:
  - "Phase 命名契约保护测试：测试名以 Test_PhaseXX_* 开头让 reviewer 在合并/删除时被名字提示「这是契约保护，不要删」"
  - "5 个契约项分散断言：每个 t.Errorf 带 [contract-N {field}] 前缀，失败时立即定位被破坏的具体契约"

requirements-completed: [CTRL-04]

# Metrics
duration: ~10min
completed: 2026-05-16
---

# Phase 54 Plan 03: 双绑互斥契约保持 + 单元测试加固 Summary

**新增 Test_Phase54_DoubleBindingContract_PreservedAfterSingleContainerRefactor（96 行）作为单容器化重构的不变式锁；admin_bindings.go 业务代码 0 diff，5 项 v3.6 51-09 双绑互斥契约（HTTP 409 / ErrCodeEgressIPAlreadyBound / 中英双语 message / host_id 占用者回显 / egress_ip_id 请求回显）锁定**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-05-16T04:49:00Z
- **Completed:** 2026-05-16T04:55:00Z
- **Tasks:** 3（T1 测试追加 + T2 grep 验证 + T3 SUMMARY 契约 ID 记录）
- **Files modified:** 1（仅 admin_bindings_test.go 末尾追加）

## Accomplishments

- 在 `internal/controlplane/http/admin_bindings_test.go` 末尾追加 96 行新测试，与既有 `TestAdminBindings_DoubleBind_ErrorCode` 并存
- 5 项契约逐项断言：
  1. HTTP status = 409
  2. JSON `error_code` = `ErrCodeEgressIPAlreadyBound` 常量 + 字面量值 `"egress_ip_already_bound"` 双重校验
  3. JSON `error` 同时含中文子串「已绑定」+ 英文子串「already bound」（v3.6 51-09 双语契约 + Phase 47 `ParseBindEgressIPResponse` 兼容）
  4. JSON `host_id` = 既有占用者 host id（不是请求者）
  5. JSON `egress_ip_id` = 请求体回显
- admin_bindings.go 业务代码严格 0 diff（V6 git diff --stat 空）
- V2 三条 grep 守护：admin_bindings.go 不依赖 internal/network、不引用 docker/gateway/cloudproxy-net/bridge、AdminBindingStore 接口 5 方法签名稳定
- V5 字面量值 `ErrCodeEgressIPAlreadyBound = "egress_ip_already_bound"` 未改

## Task Commits

按 PLAN Commit Plan 单一 atomic commit + 1 SUMMARY commit：

1. **T1 + T2 + T3: 追加 Phase 54 双绑互斥契约保护测试（含 V2 grep 守护 + 契约 ID 注释）** — `c7f3461` (test)

**Plan metadata:** _本 commit_ (docs: 54-03-SUMMARY)

## Files Created/Modified

- `internal/controlplane/http/admin_bindings_test.go` — 末尾 +96 行新增 `Test_Phase54_DoubleBindingContract_PreservedAfterSingleContainerRefactor`；既有 stub / table-driven 测试 / `TestAdminBindings_DoubleBind_ErrorCode` 0 diff

## Decisions Made

- **新测试与 v3.6 既有用例并存**：D-54-11 显式选择不复用既有 `TestAdminBindings_DoubleBind_ErrorCode`，原因是 v3.6 用例可能被未来重构合并/拆分；新增独立用例 + Phase 54 显式命名（`Test_Phase54_*Contract_PreservedAfter*`）让 reviewer 在删除时被名字提示「这是契约保护，不要删」。两个用例都跑、断言重复，是 acceptable 的成本（每次 < 10ms）
- **常量 + 字面量双重校验**：契约 2 同时断言 `code != ErrCodeEgressIPAlreadyBound` 和 `ErrCodeEgressIPAlreadyBound != "egress_ip_already_bound"`，让常量改名或值变更都会触发明确错误
- **5 个契约项分散断言而非 t.Fatalf 短路**：契约 1（status）失败用 `t.Fatalf` 提前终止（继续解 body 无意义）；契约 2-5 用 `t.Errorf` 累计失败，让单次跑能看到所有被破坏的契约项

## 契约保护

- **契约 ID:** CTRL-04-CONTRACT-1..5
- **测试用例:** `Test_Phase54_DoubleBindingContract_PreservedAfterSingleContainerRefactor` (`internal/controlplane/http/admin_bindings_test.go`)
- **契约源:** Phase 51 Plan 09（落地于 v3.6 milestone，参见 `.planning/milestones/v3.6-MILESTONE-AUDIT.md`）
- **不变式锁定方式:**
  - Go unit test 5 项断言（每项失败带 `[contract-N {field}]` 前缀方便定位）
  - `ErrCodeEgressIPAlreadyBound` 常量字面量值 grep 守护（V5）
  - admin_bindings.go 业务代码 0 diff（V6 git diff --stat 空）
- **并存设计意图（D-54-11）:** 与 v3.6 `TestAdminBindings_DoubleBind_ErrorCode` 一同存在，合并需走 Phase 56 / v4.1 显式 audit

## Deviations from Plan

None - plan executed exactly as written.

PLAN T1 / T2 / T3 全部按字面量 1:1 落地：
- T1 测试代码与 PLAN 行内代码块 96 行字面相同（仅 doc string 末尾补充「两份用例并存是设计意图（D-54-11）」一句强调，不改断言）
- T2 三条 grep 全部按 PLAN 字面量执行，结果符合期望（0 / 0 / 5）
- T3 SUMMARY 「契约保护」节按 PLAN 模板 4 项字段填写

业务代码 0 diff，本测试作为单容器化重构的不变式锁。

## Issues Encountered

None.

## Verification Results（V1-V6）

| 验证 | 期望 | 实际 | 状态 |
|---|---|---|---|
| V1 新增测试 PASS | `Test_Phase54_DoubleBindingContract_PreservedAfterSingleContainerRefactor` PASS | PASS（0.00s，5 项契约全断言通过） | ✅ |
| V2.1 admin_bindings.go 不 import internal/network | 0 命中 | 0 命中 | ✅ |
| V2.2 admin_bindings.go 不引用 docker/gateway/cloudproxy-net/bridge | 0 命中 | 0 命中 | ✅ |
| V2.3 AdminBindingStore 接口 5 方法签名 | 5 行命中 | 5 行命中（GetHost / BindEgressIPToHost / UnbindEgressIPFromHost / GetBindingHostID / GetBindingHostIDByEgressIP） | ✅ |
| V3 v3.6 既有 11+1 用例不破 | TestAdminBindingsHandler 全 10 个子用例 + TestAdminBindings_DoubleBind_ErrorCode PASS | 全 PASS | ✅ |
| V4 完整 controlplane test | `go test -short ./internal/controlplane/...` 全 PASS | 4 个 package 全 PASS（app / credgen / http / scheduler） | ✅ |
| V5 ErrCodeEgressIPAlreadyBound 字面量值未改 | `const ErrCodeEgressIPAlreadyBound = "egress_ip_already_bound"` 1 命中 | 1 命中（admin_bindings.go L29） | ✅ |
| V6 admin_bindings.go 业务代码 0 diff | `git diff --stat` 空 | 空 | ✅ |
| 全量 short 测试 | `go test -short ./...` 全 PASS | 全 PASS（17 个有测试的包绿，含 e2e/harness） | ✅ |
| go build | `go build ./...` 0 error | 0 error | ✅ |

## Known Stubs

无新增 stub。本 plan 仅新增测试用例，未改动业务代码、未引入新组件。

## Threat Flags

无新增网络 / 认证 / 文件访问 / schema 表面。本 plan 整体方向是**收紧契约监控**：
- 双绑互斥 pre-check 是 v3.6 51-09 落地的安全约束（防止 egress IP 被多 host 共用导致流量归因错误）
- 本测试作为不变式锁，让单容器化重构（54-01 / 54-02）即使大幅删行也无法静默破坏该约束
- 与 Phase 47 helpers `EgressIPDoubleBindContract` / `ParseBindEgressIPResponse` 一同构成多层契约守护

## Next Phase Readiness

- ✅ Phase 54 wave 1 全部完成（54-01 容器路径重构 + 54-03 双绑契约加固），可进入 wave 2
- ✅ Plan 54-04 (sing-box-gateway 镜像退役 + Makefile gateway-image target 删除): admin_bindings.go 与 internal/network/ 解耦已被 grep 守护，可安全继续清理
- ✅ Plan 54-02 (host-agent sing-box config 写盘): 与本 plan 0 文件冲突（test-only 改动），可并行/接力执行
- ⚠️ Phase 55 e2e 重构 (`harness.Scenario.WithSingBoxGateway` → 单容器): Out of Scope，但本 plan 守护的契约对 e2e 双绑用例同样有效

## Self-Check

- [x] `internal/controlplane/http/admin_bindings_test.go` 末尾包含 `Test_Phase54_DoubleBindingContract_PreservedAfterSingleContainerRefactor`（grep 确认）
- [x] commit `c7f3461` 存在（test(54-03)）
- [x] V1-V6 全部通过
- [x] admin_bindings.go 业务代码 0 diff（V6 git diff --stat 空）
- [x] ErrCodeEgressIPAlreadyBound = "egress_ip_already_bound" 字面量值未改（V5）
- [x] go build ./... + go test -short ./... 全绿

## Self-Check: PASSED

---
*Phase: 54-control-plane*
*Plan: 03*
*Completed: 2026-05-16*
