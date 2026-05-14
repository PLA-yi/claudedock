---
phase: 50-killswitch-stress
plan: 04
title: docker network disconnect → worker 不 fallback host bridge (KILL-04)
status: implemented
created: 2026-05-14
---

# Phase 50 Plan 04 SUMMARY — KILL-04 docker network disconnect

## 落地范围

- `tests/e2e/killswitch_stress/killswitch_04_disconnect_test.go`：`TestKillSwitch_04_NetworkDisconnect`。
- 共享 helper `(*GoldenPath).DisconnectGatewayFromBridge` / `ReconnectGatewayToBridge`、`PickGatewayBridgeNetwork` 纯函数由 `feat(50-shared)` 一笔合入。

## 关键决策

- **真相校准**：grep 源码 `internal/network/container_proxy_provider.go:323` 确认 gateway 实际接的是专属自定义 bridge `cloudproxy-net-<HostID>`，**不是** docker 默认 `bridge`；KILL-04 disconnect 的就是这个网络。worker 同时挂着 default bridge + cloudproxy-net-* 两网（`internal/runtime/tasks/worker.go:215 --network bridge` + 之后 `dockerNetworkConnect`）。
- **PickGatewayBridgeNetwork 纯函数**：从 `docker inspect` 输出 `<name>=<ip>;` 字面量中按 `cloudproxy-net-` 前缀优先挑出；兜底取首个非 default `bridge` 的网络；都没有 → 返回空（backend GAP）。
- **backend GAP 流转 Phase 51 模式**：DisconnectGatewayFromBridge 返回 `has no cloudproxy-net / custom bridge network` → t.Skipf + reason 说明 backend 当前未实现专属 bridge（与 Phase 49 LEAK-06/07/08 流转 QUAL-06/07 模式一致）。
- **cleanup ReconnectGatewayToBridge**：best-effort + 保存原 IP；失败仅 t.Logf。
- **timing 同 KILL-01/02**：3000ms 断网阈值 + 5s 抓包窗口。

## darwin 闸

- `go build ./tests/e2e/...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./tests/e2e/ -run "Helpers|Killswitch" -count=1` PASS（含 `PickGatewayBridgeNetwork` × 5）。
- `bash scripts/lint-no-bare-sleep.sh` PASS。

## Linux runner 真机验收（deferred-to-CI）

VERIFICATION.md 列 human_verification。
