---
phase: 45-ci
plan: 03
subsystem: tests/e2e/harness
tags: [waitfor, dump-hook, polling, no-bare-sleep]
provides:
  - waitfor-helper-with-4-variants
  - dump-hook-interface
  - noop-dump-hook
requires:
  - tests/e2e/harness/suite.go (Plan 01 BaseSuite)
affects:
  - tests/e2e/harness/dump.go
  - tests/e2e/harness/waitfor.go
  - tests/e2e/harness/waitfor_test.go
tech-stack:
  added: []
  patterns:
    - "functional options（WaitOpt 模式）+ time.NewTimer + time.NewTicker 三路 select 轮询"
    - "DumpHook 接口隔离 Plan 03 与 Plan 04，Plan 03 用 NoopDumpHook 让单测无 docker 跑通"
    - "ContainerHandle 接口适配 testcontainers.Container 与未来 Scenario 句柄"
key-files:
  created:
    - tests/e2e/harness/dump.go
    - tests/e2e/harness/waitfor.go
    - tests/e2e/harness/waitfor_test.go
  modified: []
decisions:
  - "harness 包内部用 stdlib testing（不 import testify）：让 helper 库自身的 unit test 不依赖 testify suite，便于未来作为独立工具被引用"
  - "WaitForPort 单次 dial 1s 超时；WaitForHTTP 单次 req 2s 超时；让总超时与 pollInterval 决定真实重试节奏"
  - "TestWaitFor_CtxCancelImmediateReturn 用提前 cancel + entry predicate 失败的方式触发 ctx.Done() 分支，避免 goroutine + time.Sleep（守住 Plan 05 lint-no-bare-sleep 约定）"
  - "WithPollInterval(0) 防御性回退到 DefaultWaitPollInterval，避免 ticker panic 或忙等"
  - "WithDumpHook(nil) 防御性回退到 NoopDumpHook，避免 NPE"
  - "ContainerHandle.Exec 简化签名（[]string → int, io.Reader, error），让 Plan 02 Scenario 句柄包装 closure 即可适配"
metrics:
  duration: 约 30 分钟
  tasks_completed: 3/3
  files_modified: 3
  commits: 1（与本 SUMMARY + Plan 02 合并提交）
  completed_at: 2026-05-14
requirements_satisfied:
  - E2E-05
---

# Phase 45 Plan 03: waitFor helper Summary

## One-liner

实现 `tests/e2e/harness/waitfor.go` —— 一个无第三方依赖、严格用 `time.NewTimer` + `time.NewTicker` 三路 select 的轮询 helper，含 4 个语义化变体（`WaitForLog` / `WaitForPort` / `WaitForHTTP` / `WaitForExec`），默认 30s/500ms，超时即调 `DumpHook.OnWaitForTimeout` 钩子；同时落地 `dump.go`（DumpHook 接口 + NoopDumpHook 占位，Plan 04 接真实实现），以及 `waitfor_test.go` 8 个 unit test 全部 PASS（无 docker 依赖，1.092s 跑完）。

## 实际产出

| 文件 | 性质 | 关键内容 |
|------|------|----------|
| `tests/e2e/harness/dump.go` | 新建 | `DumpHook` interface + `NoopDumpHook` 默认实现 |
| `tests/e2e/harness/waitfor.go` | 新建 | `WaitFor` 核心 + 4 个 helper（Log/Port/HTTP/Exec）+ 3 个 WaitOpt（Timeout/PollInterval/DumpHook）+ `ContainerHandle` 适配接口 + `DefaultWaitTimeout=30s` / `DefaultWaitPollInterval=500ms` 常量 |
| `tests/e2e/harness/waitfor_test.go` | 新建 | 8 个 `TestWaitFor_*` 单测，覆盖立即成功 / 超时 / 自定义参数 / dump hook 调用 / hook 错误合并 / ctx cancel / 防御性 PollInterval=0 |

## 验证结果

| 验证 | 命令 | 结果 |
|------|------|------|
| 8 个 unit test 全 PASS | `go test -tags=e2e ./tests/e2e/harness/ -count=1 -run TestWaitFor -v -timeout=30s` | 8/8 PASS in 1.092s ✓ |
| 编译通过 | `go build -tags=e2e ./tests/e2e/harness/...` | exit 0 ✓ |
| 零 `time.Sleep` | `grep -rnE '^\s*time\.Sleep\(' tests/e2e/` | 无命中 ✓（Plan 05 lint-no-bare-sleep 守护就绪） |
| 默认路径不受影响 | `go build ./...` | exit 0 ✓ |

### 8 个 unit test 跑通明细

```
--- PASS: TestWaitFor_SucceedsImmediately            (0.00s)  入口立即成功
--- PASS: TestWaitFor_PredicateFailsUntilTimeout     (0.20s)  超时 + 错误文本断言
--- PASS: TestWaitFor_WithTimeoutOverridesDefault    (0.05s)  WithTimeout(50ms) 覆盖默认 30s
--- PASS: TestWaitFor_WithPollIntervalOverridesDefault(0.10s) WithPollInterval(10ms) 让 predicate 至少调 5 次
--- PASS: TestWaitFor_DumpHookCalledOnTimeout        (0.05s)  hook 被调用 + 错误文案不含 hook err
--- PASS: TestWaitFor_DumpHookErrorMerged            (0.05s)  hook 返回 err → 错误同时含 last err 与 dump hook err
--- PASS: TestWaitFor_CtxCancelImmediateReturn       (0.00s)  ctx cancel 后 < 1s 返回 errors.Is(context.Canceled)
--- PASS: TestWaitFor_PollIntervalZeroFallsBackToDefault(0.10s) 防御性回退，不卡死
```

## 给后续 plan 的接口契约

### 给 Plan 02（Scenario）
- Scenario.Start 中的所有轮询点（pg ready / cp health / sing-box healthy）建议**统一**改用 `harness.WaitFor`，弃用 testcontainers 的 `wait.*`，便于：
  1. 失败时统一 dump hook 接入
  2. 错误格式统一（`waitfor name=...: timed out after ...; last err: ...`）
  3. Plan 05 的 lint-no-bare-sleep 守护对所有轮询点生效
- Scenario 的 GatewayHandle / HostHandle 实现 `harness.ContainerHandle` 接口（提供 `Logs(ctx)` 与 `Exec(ctx, cmd)`），即可被 `WaitForLog` / `WaitForExec` 直接消费

### 给 Plan 04（artifact dump）
- `DumpHook` interface 已就位，Plan 04 实现 `artifactDumper` 后通过 `WithDumpHook(realDumper)` 注入到所有 Scenario.Start 内部的 WaitFor 调用位点
- `BaseSuite.TearDownTest()` 当前为空 hook，Plan 04 在此追加 `if s.T().Failed() { dump.Collect(...) }`
- 设计契约：`OnWaitForTimeout` 必须幂等 + best-effort + 短超时（建议 ≤ 30s 内部 ctx），避免 dump 阶段卡死整个 suite

### 给 Plan 05（lint-no-bare-sleep 守护）
- 本 plan 产出文件（dump.go / waitfor.go / waitfor_test.go）`grep -nE '^\s*time\.Sleep\('` 命中数 = 0
- Plan 05 的守护脚本可直接对 `tests/e2e/**.go` 起强约束，本 plan 产物作为零基线参考

## 决策回顾

1. **不引入 testify**：harness 包内部 unit test 用 stdlib `testing` + 自写 `failIf` helper（4 行）。理由：
   - 让 helper 库自身可被理论上独立抽出（未来可能拆 `pkg/testharness`）
   - 与 e2e 用例侧（用 testify/suite）形成"工具用 stdlib，业务用 testify"的清晰分层
2. **WaitForPort dial 单次 1s + WaitForHTTP req 单次 2s**：
   - dial 失败通常很快（connection refused 立即返回）；1s 上限防止极端环境下 SYN retry 把 pollInterval 节奏带乱
   - http req 因为可能有真实业务逻辑（如 control-plane / migrations），2s 给一定空间
3. **`TestWaitFor_CtxCancelImmediateReturn` 不用 goroutine + sleep**：
   - 原始 PLAN 模板用 `go func() { time.Sleep(20*ms); cancel() }()` 模拟延迟取消
   - 改为提前 `cancel()` + 让 entry predicate 返回 error → for-select 第一次迭代必然命中 `<-ctx.Done()`
   - 既守住 Plan 05 的 lint-no-bare-sleep 约定，又简化测试逻辑
4. **`WithPollInterval(0)` 防御性回退**：用户传 0 不应导致 panic 或死循环；统一回退到默认值，并在文档中标注

## 风险与遗留

- **WaitForLog 用 io.ReadAll + strings.Contains**：每次轮询读完整日志缓冲，对长跑容器（日志超过几 MB）有 O(N²) 风险。当前 e2e 用例规模可接受；后续 phase 若发现性能问题可改为 stream + ring buffer
- **没有 WaitForLogStream（带 since 时间戳）**：testcontainers.Container.Logs 在不同版本对 since 支持不一致，本 plan 不接入；后续 plan 真有需求再补
- **ContainerHandle.Exec 简化签名忽略了 ProcessOption**：`testcontainers.Container.Exec` 真实签名带可变 option（如 user / workdir / env）；当前简化为 `[]string`，Scenario 句柄包装 closure 时丢这些细节是有意识的取舍

## 完成度

- ✅ 8/8 truths 全部成立
- ✅ 3/3 task 完成
- ✅ 8 个 unit test 全 PASS
- ✅ 零 `time.Sleep`
- ✅ 给 Plan 02 / 04 / 05 的接口契约全部明确
- ✅ E2E-05 需求 satisfied
