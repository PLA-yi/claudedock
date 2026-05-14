---
phase: 50-killswitch-stress
plan: 01
title: SIGKILL gateway timing 严格化 (KILL-01)
status: implemented
created: 2026-05-14
---

# Phase 50 Plan 01 SUMMARY — KILL-01 SIGKILL gateway timing 严格化

## 落地范围

- `tests/e2e/killswitch_stress/suite_test.go`：包级入口 `StartStressGolden` + `EnsureDumper`。
- `tests/e2e/killswitch_stress/helpers_test.go`：`workerInspectName` / `gatewayInspectName`。
- `tests/e2e/killswitch_stress/killswitch_01_sigkill_timing_test.go`：`TestKillSwitch_01_SigkillTiming` 主用例。
- 共享 helper / contract / classify 由 `feat(50-shared)` 一笔合入（PLAN 中明确）。

## 关键决策

- **timing 阈值 3000ms**：`KillswitchStressContract["KILL-01"].MaxDisconnectMs=3000`，与 Phase 48 `KillswitchTimingContract.ProbeMaxLatency` 一致；wall-clock 包含 `docker exec` overhead（约 200-400ms）。
- **BPF 不变**：`src host <workerIP> and not dst host <gatewayIP>`，与 Phase 48 MVS-09 完全对齐。
- **与 MVS-09 并存**：不动 Phase 48 用例文件，KILL-01 独立用例放 `tests/e2e/killswitch_stress/` 子目录。
- **gateway 句柄缺失 → t.Skip 不 fail**：与 Phase 46/47/48 范式一致。
- **tcpdump sidecar 不可用 → t.Skip + deferred-to-CI**：避免 darwin / 非特权 runner 假阴。

## darwin 闸

- `go build ./tests/e2e/...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./tests/e2e/ -run "Helpers|Killswitch" -count=1` PASS（含 50-shared 新增 27 个单测）。
- `bash scripts/lint-no-bare-sleep.sh` PASS（用例内无任何 `time.Sleep`）。

## Linux runner 真机验收（deferred-to-CI）

VERIFICATION.md 列 human_verification。
