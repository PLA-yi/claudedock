---
phase: 49-leak-defense
plan: 06
title: LEAK-06 raw socket 拒绝 (SUMMARY)
status: implemented_with_gap
leak: LEAK-06
build_tag: "e2e && linux"
created: 2026-05-14
gap: phase-51-qual-06
---

# Phase 49 Plan 06 SUMMARY: LEAK-06 raw socket 拒绝

## 实际实现

- **探测方法**：`*GoldenPath.TryRawSocket(ctx)`，bash 脚本：
  ```
  if command -v python3; then
    python3 -c 'socket.socket(AF_INET, SOCK_RAW, IPPROTO_ICMP); print("RAW_OK")'
  elif command -v python; then ...
  else exec 3<>/dev/raw/icmp ...
  fi
  ```
- **解析关键字**：stdout 含 `RAW_OK` → `Blocked=false`（实锤 leak）；
  stderr 含 `PermissionError` / `Operation not permitted` / `Permission denied` →
  `Blocked=true`。
- **裁决**：`!res.Blocked` → **t.Errorf**（不阻塞其它 LEAK 用例）。

## 与 Plan 偏差

无。

## 实际命令 / 工具

- `python3 -c '...'`（python3-minimal）；缺失走 `exec 3<>/dev/raw/icmp` bash 兜底。

## 单测覆盖（darwin）

复用 shared `ClassifyLeakProbe`；fixture：`python_raw_socket_perm.txt` / `python_raw_socket_ok.txt`。

## Phase 51 GAP（必须修）

**预期 fail**：grep `internal/runtime/tasks/worker.go:217-218` 当前显式
`--cap-add NET_ADMIN --cap-add SYS_ADMIN`，且未显式 `--cap-drop NET_RAW`，
docker 默认 capability 集合**包含** `cap_net_raw`，因此 worker 内 SOCK_RAW
预期能成功创建。

**Phase 51 QUAL-06 修复方案**：
1. `internal/runtime/tasks/worker.go::buildCreateArgs` 改 docker create args：
   - 删 `--cap-add SYS_ADMIN`（worker 不需要）。
   - 把 `--cap-add NET_ADMIN` 改为运行时按需 setcap（仅 sing-box 启动短窗口需要）。
   - 显式追加 `--cap-drop NET_RAW`（即便 docker 默认带 NET_RAW，显式 drop 仍能去掉）。
2. 同步更新 LEAK-08 的 capability 校验。

修复后本 plan 用例预期转 PASS（`Blocked=true, Reason=raw_socket_denied`）。
