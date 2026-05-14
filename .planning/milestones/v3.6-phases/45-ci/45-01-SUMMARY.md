---
phase: 45-ci
plan: 01
subsystem: tests/e2e
tags: [e2e-foundation, testcontainers, testify, build-tag-isolation]
provides:
  - e2e-build-tag-isolation
  - harness-base-suite
  - testcontainers-go-direct-dep
  - testify-direct-dep
  - smoke-postgres-pg-isready
requires:
  - go>=1.25
  - docker daemon（仅 e2e 实测时；编译验证不需要）
affects:
  - go.mod
  - go.sum
  - tests/e2e/doc.go
  - tests/e2e/harness/suite.go
  - tests/e2e/suite_test.go
tech-stack:
  added:
    - "github.com/testcontainers/testcontainers-go v0.42.0（直接依赖）"
    - "github.com/stretchr/testify v1.11.1（直接依赖）"
  patterns:
    - "//go:build e2e 构建标签把整个 tests/e2e/ 隔离在默认 go test ./... 路径之外"
    - "harness 子包封装 BaseSuite + 4 个生命周期 hook，子 suite 通过结构体嵌入复用"
key-files:
  created:
    - tests/e2e/doc.go
    - tests/e2e/harness/suite.go
    - tests/e2e/suite_test.go
  modified:
    - go.mod
    - go.sum
decisions:
  - "选用 testcontainers-go v0.42.0（latest 在 v0 段，与 PLAN 约束一致，未 pin v1+）"
  - "选用 testify v1.11.1（latest）"
  - "Go 版本不变（1.25.7）；testcontainers v0.42.0 未要求 Go ≥ 1.26"
  - "顺带升级的传递依赖：crypto v0.41 → v0.48、net v0.42 → v0.49、sync v0.16 → v0.19、text v0.28 → v0.34（属传递依赖自然演进，PLAN Task 1 第 5 步已允许）"
  - "BaseSuite 不导入 testing 包（PLAN action 中的 `var _ = func(_ *testing.T) {}` 哨兵未实现 —— must_haves.truths 不要求，避免无意义占位）"
  - "ProjectRoot 通过 runtime.Caller 反推（向上 3 级：harness → e2e → tests → root），不依赖 git 也不依赖 CWD"
metrics:
  duration: 约 25 分钟
  tasks_completed: 3/3
  files_modified: 5
  commits: 1（与本 SUMMARY 合并提交）
  completed_at: 2026-05-14
requirements_satisfied:
  - E2E-01
---

# Phase 45 Plan 01: e2e 测试基础设施骨架 Summary

## One-liner

为 v3.6 e2e 测试体系打下第一块地基：引入 `testcontainers-go v0.42.0` 与 `testify v1.11.1` 直接依赖，建立 `tests/e2e/` 顶层目录与 `tests/e2e/harness/` 子包，落一个能独立运行的最小烟雾用例（postgres:18 testcontainer + `pg_isready`）；统一 `//go:build e2e` 构建标签把 e2e 重依赖隔离在默认 `go test ./...` 路径之外，零拖慢现有 ci.yml::go-test。

## 实际产出

| 文件 | 性质 | 关键内容 |
|------|------|----------|
| `go.mod` | 修改 | 新增 2 条直接 require：testcontainers-go v0.42.0、testify v1.11.1 |
| `go.sum` | 修改 | 同步 testcontainers / testify 及其传递依赖（含 docker SDK、go-connections、otel sdk 等） |
| `tests/e2e/doc.go` | 新建 | 包根说明（中文）+ `//go:build e2e` |
| `tests/e2e/harness/suite.go` | 新建 | `BaseSuite` struct（嵌入 testify/suite.Suite + Ctx/Cancel/Logger/ProjectRoot 字段 + 4 个生命周期 hook）+ `projectRootFromCaller` 反推仓库根 |
| `tests/e2e/suite_test.go` | 新建 | `SmokeSuite` + `TestPostgresReady`（postgres:18 + `wait.ForLog` 双 occurrence 防中间重启假阳性 + Exec `pg_isready` 退出码 0）+ `TestE2ESmokeSuite` 入口 |

## 验证结果

| 验证 | 命令 | 结果 |
|------|------|------|
| 直接依赖落地 | `grep -E "testcontainers\|stretchr/testify" go.mod` | 两条均出现且 **不带 `// indirect`** ✓ |
| 默认构建未破坏 | `go build ./...` | exit 0 ✓ |
| e2e 构建通过 | `go build -tags=e2e ./tests/e2e/...` | exit 0 ✓ |
| e2e 编译验证 | `go test -tags=e2e -run NONE ./tests/e2e/... -count=1` | `ok ... [no tests to run]` ✓ |
| build tag 隔离 | `go test -count=1 ./tests/e2e/...`（无 tag） | `matched no packages` ✓（确认默认路径完全跳过 e2e） |
| go vet 默认 | `go vet ./...` | exit 0 ✓ |
| go vet e2e | `go vet -tags=e2e ./tests/e2e/...` | exit 0 ✓ |
| 现有测试不受影响 | `go test ./internal/network/... -count=1 -timeout=60s` | `ok ... 0.611s` ✓ |
| Go 版本未升级 | `grep "^go " go.mod` | `go 1.25.7`（不变）✓ |

**Docker 实测**：本机 OrbStack daemon 未启动，按 PLAN Task 3 manual 段约定，**真实 `TestPostgresReady` 跑通验证由 Plan 05 落地 CI workflow 后在 hosted ubuntu-24.04 上守护**；本 plan 仅完成编译验证。

## 给后续 plan 的接口契约

### 给 Plan 02（Scenario 抽象）
- `harness.BaseSuite` 已就位，含 `Ctx` / `Cancel` / `Logger` / `ProjectRoot` 与 4 个生命周期 hook
- Plan 02 可在 `SetupTest` 中注入 Scenario 启动逻辑；`TearDownTest` 已留作 Plan 04 dump 钩点
- Plan 02 子 suite 复用模板：

  ```go
  type ScenarioSmokeSuite struct {
      *harness.BaseSuite
      Scenario *harness.Scenario  // Plan 02 新增
  }
  func (s *ScenarioSmokeSuite) SetupSuite() {
      s.BaseSuite = &harness.BaseSuite{}
      s.BaseSuite.SetT(s.T())
      s.BaseSuite.SetupSuite()
  }
  ```

### 给 Plan 03（waitFor helper）
- helper 文件落 `tests/e2e/harness/waitfor.go`，包名同 `harness`
- 导入路径：`github.com/zanel1u/cloud-cli-proxy/tests/e2e/harness`（注意 module path 是 `zanel1u`，不是 `zaneliu`）
- 默认 `Logger` 已在 BaseSuite 提供，waitFor 可接受 `*slog.Logger` 参数复用

### 给 Plan 04（artifact dump hook）
- `BaseSuite.TearDownTest()` 当前为空 hook，Plan 04 在此追加：
  ```go
  func (s *BaseSuite) TearDownTest() {
      if s.T().Failed() {
          dump.Collect(s.Ctx, s.T().Name(), ...)
      }
  }
  ```

### 给 Plan 05（CI workflow）
- e2e 测试入口固定命令：`go test -tags=e2e ./tests/e2e/... -count=1 -v -timeout=15m`
- 不需要任何额外环境变量（postgres 容器内置；后续 plan 加的 fixture 同样自带）
- 默认 `go test ./...` 不会触发 e2e 路径（已通过 build tag 验证），ci.yml::go-test job 行为零回退
- `lint-no-bare-sleep.sh` 守护脚本扫描目标固定为 `tests/e2e/**.go`（本 plan 三份新文件均无 `time.Sleep`，符合零基线）

## 决策回顾

1. **build tag 隔离的代价**：testcontainers-go 间接拉入 docker SDK + otel SDK + go-connections 等约 30 个间接依赖；通过 `//go:build e2e` 标签隔离后，默认 `go build ./...` 完全不编译这条路径，对二进制体积零影响（实测 `go build ./cmd/control-plane` 二进制大小不变）。
2. **postgres:18 与 v3.5 fixture 对齐**：与 `scripts/uat-bypass-fixture-up.sh` 默认 PG_IMAGE 一致，CI 上可复用同一镜像缓存层，缩短 e2e 启动时间。
3. **`SetT` 必须在 `SetupSuite` 之前调用**：testify/suite 的子 suite 嵌入 BaseSuite 时，因为 `s.T()` 在 SetupSuite 内才可用，所以子 suite 必须先 `s.BaseSuite.SetT(s.T())` 再 `s.BaseSuite.SetupSuite()`，否则 BaseSuite 内 `s.Logger` / `s.Ctx` 创建时 `T` 是 nil。该模式已写入 SUMMARY 接口契约段，供 Plan 02 复制。

## 风险与遗留

- **本机未跑过 `TestPostgresReady` 真实路径**：依赖 Plan 05 CI 覆盖；如本地需要排障，启动 OrbStack 后跑 `go test -tags=e2e -run TestE2ESmokeSuite ./tests/e2e/... -v` 即可
- **未引入 goleak**：QUAL-08 在 ROADMAP 中归到 Phase 51；本 plan 不提前引入
- **未在 ci.yml 加 `go vet -tags=e2e` 守护**：Plan 05 的 e2e.yml lint job 会承担（`go vet -tags=e2e ./tests/e2e/...`），本 plan 不在 ci.yml 改动

## 完成度

- ✅ 5/5 truths 全部成立
- ✅ 3/3 task 完成
- ✅ 给 Plan 02 / 03 / 04 / 05 的接口契约全部明确
- ✅ E2E-01 需求 satisfied
