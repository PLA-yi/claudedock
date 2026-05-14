---
phase: 50-killswitch-stress
plan: 02
title: ip link set tun0 down → 容器 curl 失败 (KILL-02)
status: implemented
created: 2026-05-14
---

# Phase 50 Plan 02 SUMMARY — KILL-02 tun0 down

## 落地范围

- `tests/e2e/killswitch_stress/killswitch_02_tun_down_test.go`：`TestKillSwitch_02_TunDevDown` 主用例。
- 共享 helper `(*GoldenPath).SetTunDevDown` / `SetTunDevUp` 由 `feat(50-shared)` 一笔合入。

## 关键决策

- **设备名锁 `tun0`**：grep 源码确认 sing-box auto_route 在 gateway 容器内创建 tun0；worker netns 内 nft 用的 `sb-tun0` 接口名是不同语义，与本 plan 无关。
- **cleanup 用 t.Cleanup（不是 defer）**：保证注册顺序在 Scenario.Stop 之前；best-effort 失败 t.Logf。
- **复用 KILL-01 BPF / contract**：3000ms timing + 5s 抓包窗口 + ClassifyStressResult。
- **同 ≤ 3000ms 断网契约**：tun0 down 后 sing-box auto_route 失效 → worker netns nft 默认拒绝接管 → curl 立即 ENETUNREACH。

## darwin 闸

- `go build ./tests/e2e/...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./tests/e2e/ -run "Helpers|Killswitch" -count=1` PASS。
- `bash scripts/lint-no-bare-sleep.sh` PASS。

## Linux runner 真机验收（deferred-to-CI）

VERIFICATION.md 列 human_verification。
