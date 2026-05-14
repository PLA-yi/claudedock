---
phase: 51-qual-harden
plan: 51-06
title: QUAL-06 worker cap-drop NET_RAW + 删 SYS_ADMIN
status: completed
completed: 2026-05-14
gap_closure:
  - phase-49-gap-1
---

# 51-06 QUAL-06 — worker `--cap-drop NET_RAW` + 删 SYS_ADMIN — SUMMARY

## 变更

- `internal/runtime/tasks/worker.go::buildCreateArgs`：
  - **保留** `--cap-add NET_ADMIN`：sing-box 在 worker netns 内创建 tun0 设备必须依赖 `CAP_NET_ADMIN`（sing-box 进程通过 `nsenter -n` 进入 worker netns 执行，运行时 setcap 无法事先生效）。
  - **删除** `--cap-add SYS_ADMIN`：grep 业务路径不依赖；fuse 已用 `--device /dev/fuse + --security-opt apparmor=unconfined`，fusermount setuid root 已足够。
  - **显式追加** `--cap-drop NET_RAW`：docker 默认 capability 集合含 `CAP_NET_RAW`，必须显式 drop 才能去掉；移除后容器内 `socket(AF_INET, SOCK_RAW, ...)` 立即 `PermissionDenied`，闭 Phase 49 LEAK-06 攻击面。
  - 顶部插入 doc comment 说明上述三条决策。
- `internal/runtime/tasks/worker_caps_test.go`（新文件）：`TestBuildCreateArgs_CapabilitiesLocked` 断言 args 切片三条契约：含 `--cap-add NET_ADMIN`、不含 `--cap-add SYS_ADMIN`、含 `--cap-drop NET_RAW`。

## 闸

- `go build ./...` PASS。
- `GOOS=linux go build ./...` + `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go vet ./...` PASS。
- `go test ./internal/runtime/tasks/... -run "Capabilities|BuildCreateArgs"`：5 个 PASS。
- `go test ./... -count=1`：19 包全 PASS。

## 偏差与 fixture 影响

- **NET_ADMIN 保留**：CONTEXT §Area 4 已明确允许「保留 NET_ADMIN，仅删 NET_RAW + SYS_ADMIN」折中。理由：sing-box 在 worker netns 内创建 tun0 设备需要 `CAP_NET_ADMIN`，无法运行时 setcap 替代。
- **Phase 49 LEAK-08 fixture `proc_status_clean.txt`**：原 fixture 期望 `CapEff/CapBnd` 同时**不含 NET_ADMIN/NET_RAW/SYS_ADMIN`。本 plan 落地后实际 `CapEff` 仍含 NET_ADMIN（保留），与 fixture 期望不一致。
  - **处置**：fixture 修订属 Phase 49 范围；按 CONTEXT §Area 4 决策保留 NET_ADMIN 是已批权衡，Phase 49 fixture 应放宽为「不含 NET_RAW/SYS_ADMIN」即可，NET_ADMIN 保留不再视为违规。
  - 本 plan 不修改 `tests/e2e/` 文件（CONTEXT §禁止条款）；后续 Phase 49 二次工作或 Phase 51 VERIFICATION 章节统一记录。
- **LEAK-06 SOCK_RAW**：NET_RAW 已显式 drop，darwin 上无法验证，Linux runner 真机 e2e 预期由 fail → PASS。

## 风险闭环

- 不交叉 51-01..05 / 51-09。
- 既有 `TestBuildCreateArgs_VolumesMount / EmptyVolumes_NoExtraArgs / InvalidVolumeMount / EmptyEntryPassword_ReturnsError` 全 PASS（args 切片顺序变化不破坏既有断言）。
