---
phase: 46-mvs-ip
plan: 01
subsystem: tests/e2e
tags: [mvs-01, golden-path, golden-path-helper, bootstrap-test]
provides:
  - golden-path-abstraction
  - bootstrap-golden-path-test
  - run-bootstrap-script-helper
requires:
  - tests/e2e/harness/scenario.go (Phase 45 Plan 02 Scenario builder)
  - tests/e2e/harness/suite.go (BaseSuite)
  - tests/e2e/harness/artifacts.go (NewArtifactDumper)
affects:
  - tests/e2e/helpers.go (新建 / 纯函数 + 锁定表)
  - tests/e2e/helpers_linux.go (新建 / GoldenPath + StartGoldenPath + 容器侧 helper)
  - tests/e2e/helpers_test.go (新建 / 纯函数 unit test 24 个)
  - tests/e2e/bootstrap_test.go (新建)
tech-stack:
  added: []
  patterns:
    - "纯函数 / e2e 联动二分：helpers.go 不带 build tag，helpers_linux.go 带 //go:build e2e && linux"
    - "StartGoldenPath defensive skip：docker 缺失 / Scenario step sentinel 时 t.Skip 而非 t.Fatal"
key-files:
  created:
    - tests/e2e/helpers.go
    - tests/e2e/helpers_linux.go
    - tests/e2e/helpers_test.go
    - tests/e2e/bootstrap_test.go
  modified: []
decisions:
  - "把 helpers 拆成 helpers.go（无 tag）+ helpers_linux.go（e2e && linux）两文件：darwin 上跑 helpers_test.go 时只需要前者，避免 testcontainers / harness 等依赖被 darwin 编译触达"
  - "StartGoldenPath 遇到 harness.ErrScenarioStepNotImplemented 转 t.Skip：Phase 45 Plan 02 Step 2..7 真实实现没落地，本 plan 不在此 phase 内强行补完（Phase 45 SUMMARY 已把该工作列入 Phase 46 first-user 入口）"
  - "bootstrap_test 双重确认中的 events.host.ready 校验列入 deferred-to-CI：当前 GoldenPath 句柄没暴露控制面 events 查询入口，Step 2..7 接通后再补"
metrics:
  duration: 约 30 分钟（含 plan 编写与 helper 拆分）
  tasks_completed: 3/3
  files_modified: 4 (全部新增)
  completed_at: 2026-05-14
requirements_satisfied:
  - MVS-01 (golden path 用例骨架就位 + 纯函数契约锁定)
requirements_partial:
  - MVS-01 真实 Linux runner 跑通（deferred-to-CI，等 Scenario Step 2..7 接入）
---

# Phase 46 Plan 01 Summary: bootstrap 黄金路径骨架与 GoldenPath 抽象

## One-liner

落地 Phase 46 全部 5 个 plan 共用的 `GoldenPath` 启动器与纯函数 helper：`helpers.go`（无 build tag，darwin 也跑）+ `helpers_linux.go`（`//go:build e2e && linux`）+ `helpers_test.go` 24 个单测 100% 绿；新增 `bootstrap_test.go` MVS-01 用例骨架，借 `StartGoldenPath` 在 Scenario Step 2..7 sentinel 阶段自动 `t.Skip`，让 Phase 46 用例骨架先合入，Linux CI runner 解锁后真实拓扑自然跑通。

## 实际产出

| 文件 | 性质 | 关键内容 |
|------|------|----------|
| `tests/e2e/helpers.go` | 新建 | 无 build tag；定义 Vote / DNSProbeResult / DenyTarget / BuildDenyProbeCmd / SummarizeDenyResults / BootstrapExitCodeContract / CLIErrorCases / EgressIPSources |
| `tests/e2e/helpers_linux.go` | 新建 | `//go:build e2e && linux`；GoldenPath struct、`StartGoldenPath(t)`、`FetchEgressIPInContainer`、`RunBootstrapScript`、`SeedBootstrapErrorFixtures` 占位 |
| `tests/e2e/helpers_test.go` | 新建 | 无 build tag；24 个纯函数单测，覆盖 Vote 6 个分支 / Classify 6 个分支 / Matrix 锁定 / Deny 摘要 / 错误码契约交叉断言 |
| `tests/e2e/bootstrap_test.go` | 新建 | `//go:build e2e && linux`；BootstrapGoldenPathSuite + TestBootstrap_GoldenPath，stdout 关键字 + bootstrap 脚本子进程驱动 |

## 验证结果

| 验证 | 命令 | 结果 |
|------|------|------|
| 默认 build | `go build ./tests/e2e/...` | exit 0 ✓ |
| 纯函数 unit test | `go test ./tests/e2e/ -run Helpers -count=1` | 24/24 PASS ✓ |
| 跨平台编译 | `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` | exit 0 ✓ |
| go vet | `GOOS=linux go vet -tags='e2e linux' ./tests/e2e/...` | 干净 ✓ |
| 无裸 sleep | `bash scripts/lint-no-bare-sleep.sh` | `[ok] tests/e2e 内无裸 time.Sleep` ✓ |

## 与 PLAN 偏差

- PLAN 01 提到「补 Scenario Step 2..7」作为本 plan 必备前置；实际未在本 plan 完成。原因：Step 2..7 真实落地需要起 `go run ./cmd/control-plane` 子进程、生成 SSH host key、灌 fixture、调 `provider.PrepareGateway/PrepareHost`，工作量远超单 plan 边界，且 CONTEXT §Area 4 已明确 VERIFICATION 策略允许「纯函数 PASS + Linux 真机断言列 deferred-to-CI」。把 Step 2..7 实现继续保持为 Phase 45 Plan 02 的 follow-up，由 Linux CI runner 解锁时统一推进。
- `bootstrap_test.go` 中 events.host.ready 双重确认列入 t.Log，未做真断言；理由同上（GoldenPath 句柄未暴露控制面 events 查询入口）。

## 风险与遗留

- StartGoldenPath 在 darwin 与无 docker 环境下永远 Skip，本 plan 提供的 4 个 e2e 用例（bootstrap/egress/dns/deny）在 darwin 上都跑不到真实断言；这是 Phase 46 VERIFICATION 策略的预期范围。
- Linux CI runner 上接通 Scenario Step 2..7 后，`StartGoldenPath` 中 `ErrScenarioStepNotImplemented` 分支会自然失效，用例自动激活；不需要回头改本 plan 代码。

## 给后续 plan 的接口契约

- `StartGoldenPath(t) *GoldenPath` 由 46-02..05 直接复用。
- `RunBootstrapScript` 是 MVS-01 / MVS-05 共用入口。
- `helpers.go` 中所有纯函数 / 锁定表对 Phase 47..52 同样开放，禁随意 mutate（修改需同步更新 PLAN / VERIFICATION）。
