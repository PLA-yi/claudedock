---
phase: 46-mvs-ip
plan: 04
subsystem: tests/e2e
tags: [mvs-04, default-deny, matrix]
provides:
  - default-deny-matrix-locked-constant
  - build-deny-probe-cmd-pure-function
  - summarize-deny-results-pure-function
  - default-deny-test-skeleton
requires:
  - tests/e2e/helpers.go (DenyTarget / DefaultDenyMatrix / BuildDenyProbeCmd /
    SummarizeDenyResults，46-01 已落)
affects:
  - tests/e2e/default_deny_test.go (新增)
tech-stack:
  added: []
  patterns:
    - "errgroup 风格并发：sync.WaitGroup + 互斥 map 收集 exit code"
    - "GNU timeout(1) 包裹 bash /dev/tcp，nft drop 长尾不拖垮用例"
key-files:
  created:
    - tests/e2e/default_deny_test.go
  modified: []
decisions:
  - "矩阵 / 命令拼装 / 结果摘要 3 个纯函数在 46-01 一次性落地；本 plan 只新增主用例"
  - "并发探测用 sync.WaitGroup 而非 errgroup：本 plan 没引入 golang.org/x/sync 依赖，WaitGroup + 共享 ctx 取消足够"
  - "Exec 错误回退到 exit code 124：避免把 Exec 错误本身误判为 leak；与 GNU timeout 默认超时退出码一致，t.Log 中可识别"
metrics:
  duration: 约 10 分钟
  tasks_completed: 3/3
  files_modified: 1
  completed_at: 2026-05-14
requirements_satisfied:
  - MVS-04 (默认拒绝矩阵纯函数 + 锁定常量 + 用例骨架就位)
requirements_partial:
  - MVS-04 Linux 真机 4 target 真实跑通（deferred-to-CI）
---

# Phase 46 Plan 04 Summary: 默认拒绝矩阵

## One-liner

新增 `tests/e2e/default_deny_test.go` 主用例，复用 46-01 已锁定的 `DefaultDenyMatrix`、`BuildDenyProbeCmd(target, 3)`、`SummarizeDenyResults` 三件套：并发对 4 target 跑 `timeout 3 bash -c 'echo >/dev/tcp/HOST/PORT'`，任一 exit 0 即 leak fail。

## 实际产出

| 文件 | 性质 | 关键内容 |
|------|------|----------|
| `tests/e2e/default_deny_test.go` | 新建 | `//go:build e2e && linux`；DefaultDenySuite + TestDefaultDeny_Matrix；4 target × 3s timeout × 并发 |

## 验证结果

- `go build ./tests/e2e/...`（darwin）exit 0 ✓
- `go test ./tests/e2e/ -run "HelpersDefaultDeny|HelpersBuildDeny|HelpersSummarizeDeny" -count=1`（darwin）6 PASS ✓
- `GOOS=linux go vet -tags='e2e linux' ./tests/e2e/...` 干净 ✓

## 与 PLAN 偏差

- 4 target 与 CONTEXT 锁定值完全一致（`1.1.1.1:80` / `8.8.8.8:443` / `9.9.9.9:443` / `169.254.169.254:80`）。
- 并发库选 `sync.WaitGroup` 而非 PLAN 04 备注的 `errgroup`，避免引入 `golang.org/x/sync` 直接依赖。
- nft counters 与 deny-matrix.txt 写盘列入 deferred-to-OBS-02。

## 风险与遗留

- `169.254.169.254:80` 在某些云上是真实 IMDS，若 CI runner 上能联通会触发 false leak；当前判断「违反默认拒绝即 fail」的语义是正确的，runner 配置层面是否需要绕开 IMDS 由 Phase 50 压力测试时回看。
- `bash /dev/tcp` 依赖 worker 镜像带 bash；alpine 默认无 bash，需镜像里装 bash 或换用 `nc -z -w 3 host port` 备选。本 plan 选 bash 单方案；CI 上不通时由 SUMMARY 记录 → 改 `BuildDenyProbeCmd` 提供 `nc` 备选。

## 给后续 plan 的接口契约

- `DefaultDenyMatrix` 暴露给 Phase 48 (Kill-switch) / Phase 49 (防泄漏) 直接复用；修改条目需先通知下游。
- `BuildDenyProbeCmd` / `SummarizeDenyResults` 是公开纯函数。
