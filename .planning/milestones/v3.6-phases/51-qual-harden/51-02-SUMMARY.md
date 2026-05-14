---
phase: 51-qual-harden
plan: 51-02
title: QUAL-02 verifyLeakBlocked 多目标参数化
status: completed
completed: 2026-05-14
---

# 51-02 QUAL-02 — `verifyLeakBlocked` 多目标参数化 — SUMMARY

## 变更

- `internal/network/verify.go`：
  - 新增 `leakTarget{Host,Port}` 结构 + `String()` 返回 `host:port`。
  - 新增包级 `defaultLeakTargets`（4 target）：`1.1.1.1:80`、`8.8.8.8:443`、`9.9.9.9:443`、`169.254.169.254:80`，与 Phase 46 `tests/e2e/helpers.go::DefaultDenyMatrix` 对齐。
  - 新增 `verifyLeakBlockedMulti(ctx, prefix, targets, *VerifyResult)`：并发对每个 target 跑 `timeout 3 bash -c 'echo >/dev/tcp/HOST/PORT'`；任一连通即 `LeakBlocked=false`，按 targets 顺序写入第一个泄漏 target。
  - `verifyLeakBlocked` 旧签名保留，默认 `LeakTarget="1.1.1.1:80"` 兼容 `TestVerifyNetworkIntegrity_LeakTargetSet` 既有断言；内部委托给 `verifyLeakBlockedMulti(defaultLeakTargets)`。
- `internal/network/verify_test.go`：
  - 新增 `TestDefaultLeakTargets_LockedContract`：锁 4 target 顺序与值。
  - 新增 `TestVerifyLeakBlockedMulti_AllBlocked / OneLeaked / AllLeaked / EmptyTargets`。

## 闸

- `go build ./...` PASS。
- `GOOS=linux go build ./...` PASS。
- `go test ./internal/network/... -count=1`：13 个 leak / verify 用例全 PASS（含 5 新增）。
- `go test ./... -count=1`：19 包全 PASS。

## 偏差

- 生产代码 + 单测在历史 worktree 已写好，**实际随 51-01 commit `290e5b5` 一并落地**（commit message 仅署 51-01，未提及 51-02）。本 SUMMARY commit 仅追加文档记录，保留 11 commit 总数。
- CONTEXT §Area 1 写「全部连通的 fail 路径下用占位字符串 `all_blocked`」；落地为「保留默认 `1.1.1.1:80` + fail 路径覆盖为第一个泄漏 target」，更便于 e2e metadata 读 LeakTarget 时获得真实泄漏目标，且不破坏既有 `result.LeakTarget != ""` 断言。

## 风险闭环

- 既有 `TestVerifyNetworkIntegrity_LeakTargetSet` PASS（默认值 `1.1.1.1:80`）。
- `VerifyResult` 字段语义不变；`firstNetworkError` 优先级不动。
- 169.254.169.254 IMDS target 与 51-05 新增 `linklocal-drop` nft 规则同向，不冲突。
