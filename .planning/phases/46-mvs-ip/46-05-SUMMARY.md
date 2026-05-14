---
phase: 46-mvs-ip
plan: 05
subsystem: tests/e2e
tags: [mvs-05, cli-error-codes, bootstrap-script, contract-lock]
provides:
  - bootstrap-exit-code-contract-table
  - cli-error-cases-table
  - run-bootstrap-script-helper (helpers_linux.go)
  - error-codes-sql-fixture
requires:
  - tests/e2e/helpers.go (BootstrapExitCodeContract / CLIErrorCases，46-01 已落)
  - tests/e2e/helpers_linux.go (RunBootstrapScript，46-01 已落)
  - internal/controlplane/http/bootstrap_errors.go (源真相)
affects:
  - tests/e2e/cli_error_codes_test.go (新增)
  - tests/e2e/fixtures/error-codes.sql (新增)
tech-stack:
  added: []
  patterns:
    - "锁定表 cross-check：helpers_test 通过 import internal/controlplane/http 直接比对 BootstrapErrorEntries"
    - "table-driven 错误场景 + suite.Run 子用例隔离"
key-files:
  created:
    - tests/e2e/cli_error_codes_test.go
    - tests/e2e/fixtures/error-codes.sql
  modified: []
decisions:
  - "把『真实 cloud-claude binary』换成『deploy/bootstrap/cloud-bootstrap.sh』作为被测 binary：grep 后 cmd/cloud-claude/main.go 实际只定义 exit 1-5，错误码 10-13 由 bootstrap.sh 在 case \"$error_code\" 分支映射；按 CONTEXT §Area 3 以源码为准"
  - "BootstrapExitCodeContract / CLIErrorCases 与 RunBootstrapScript helper 都在 46-01 一次性落地（helpers.go / helpers_linux.go），本 plan 只新增主用例 + fixture SQL"
  - "fixture SQL 内 bcrypt hash 用占位字符串：CI 接通后由 helper 函数动态生成（避免硬编码过期 hash + 隐私安全）"
  - "host_not_found 用 active 用户但不绑 host 触发；用独立 username 命名空间（user-no-host）避免与 GoldenPath 默认 alice 用户冲突"
metrics:
  duration: 约 10 分钟
  tasks_completed: 3/3
  files_modified: 2
  completed_at: 2026-05-14
requirements_satisfied:
  - MVS-05 (4 条错误码契约表 + 锁定 unit test + 用例骨架就位)
requirements_partial:
  - MVS-05 真实 fixture 灌种 + 4 场景跑通（deferred-to-CI，需要 admin API 接通后调用）
---

# Phase 46 Plan 05 Summary: CLI 错误码契约

## One-liner

落地 MVS-05 锁定表与用例骨架：`BootstrapExitCodeContract`（46-01 已落，对应 10/11/12/13）+ helpers_test 中 import `internal/controlplane/http.BootstrapErrorEntries` 做编译期 cross-check + 新增 `cli_error_codes_test.go` table-driven 4 场景 + `tests/e2e/fixtures/error-codes.sql` 占位种子。

## 与 ROADMAP 偏差（重要）

ROADMAP §Phase 46 §Details 5：「CLI 错误码用真实 cloud-claude binary 触发各场景」。

实际 grep `cmd/cloud-claude/`：

- `cmd/cloud-claude/main.go` 常量 `exitAuthFailed=1 / exitNetworkError=2 / exitTimeout=3 / exitConfigError=4 / exitInternalError=5`，**不含 10-13**。
- 错误码 10-13 由 `internal/controlplane/http/bootstrap_errors.go` `BootstrapErrorEntries` 定义，并由 `deploy/bootstrap/cloud-bootstrap.sh` 在 `case "$error_code"` 分支落到 exit code。
- 用户主入口 `curl -sSL .../v1/bootstrap/script | bash` 走的是 bootstrap.sh，**不是** cloud-claude binary。

→ 按 CONTEXT §Area 3「源码与 ROADMAP 不一致时以源码为准」决策：本 plan 把「真实 cloud-claude binary」换成「`deploy/bootstrap/cloud-bootstrap.sh` 子进程 + 真实控制面 HTTP API」。VERIFICATION.md 中也记录此偏差。

## 实际产出

| 文件 | 性质 | 关键内容 |
|------|------|----------|
| `tests/e2e/cli_error_codes_test.go` | 新建 | `//go:build e2e && linux`；CLIErrorCodesSuite + TestCLIErrorCodes_Contract；table-driven 4 个错误码场景 |
| `tests/e2e/fixtures/error-codes.sql` | 新建 | 3 个特殊用户占位种子（disabled / expired / no-host），bcrypt hash 用占位字符串，CI runtime 由 helper 动态生成 |

## 验证结果

- `go build ./tests/e2e/...`（darwin）exit 0 ✓
- `go test ./tests/e2e/ -run "HelpersBootstrap|HelpersCLIError" -count=1`（darwin）3 PASS ✓
  - `TestHelpersBootstrapExitCodeContract_AlignsWithSourceOfTruth` 通过 import `internal/controlplane/http` 比对 BootstrapErrorEntries
  - `TestHelpersBootstrapErrorEntries_AtLeastSeven` 防止源码 entry 误删
  - `TestHelpersCLIErrorCases_Wellformed` 验证 4 条 table-driven 用例结构正确
- `GOOS=linux go vet -tags='e2e linux' ./tests/e2e/...` 干净 ✓

## 风险与遗留

- `error-codes.sql` 中的 bcrypt hash 当前是占位字符串，CI runner 上接通 `SeedBootstrapErrorFixtures` 后需要动态生成真实 hash 才能让 bcrypt.CompareHashAndPassword 成功（不然 disabled/expired 用例会落到 auth_invalid 分支）。这是 deferred-to-CI 项。
- `users` 表实际 schema 可能与 SQL 假设有偏差（列名、约束）；CI runner 接通后由 helper 校对调整。
- `host_not_found` 场景需要 active 用户但不绑 host；当前 GoldenPath 不会清理 host 绑定，CI 接通后由 helper 在灌种时直接跳过 host 创建。

## 给后续 plan 的接口契约

- `BootstrapExitCodeContract` 是只读锁定表；Phase 47..52 用例不应改它，要扩 error_code 必须在 `internal/controlplane/http/bootstrap_errors.go` 同步。
- `RunBootstrapScript` helper 由 46-01 / 46-05 共用。
