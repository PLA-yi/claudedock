---
phase: 49-leak-defense
plan: 03
title: LEAK-03 ICMP 阻断 (SUMMARY)
status: implemented
leak: LEAK-03
build_tag: "e2e && linux"
created: 2026-05-14
---

# Phase 49 Plan 03 SUMMARY: LEAK-03 ICMP 阻断

## 实际实现

- **探测方法**：`*GoldenPath.PingICMP(ctx, "8.8.8.8")`，容器内 `ping -c 1 -W 3 8.8.8.8`。
- **解析关键字**：`Operation not permitted` / `Permission denied` →
  `raw_socket_denied`（与 LEAK-06 共享语义）；`Network is unreachable` /
  `Destination Host Unreachable` → `route_unreachable`；`0 received` → `ping_no_reply`。
- **独立 oracle**：`TcpdumpOnHostEth0` BPF `icmp and dst host 8.8.8.8 and src host <workerIP>`。
- **裁决**：`tdRes.packets > 0 || !pingRes.Blocked` → t.Fatalf。

## 与 Plan 偏差

无。

## 实际命令 / 工具

- `ping -c 1 -W 3 8.8.8.8`（iputils-ping）；缺失 → `apt-get install -y iputils-ping`。
- tcpdump sidecar 同 Plan 01。

## 单测覆盖（darwin）

复用 shared `ClassifyLeakProbe`；fixture：`ping_perm_denied.txt` / `ping_unreachable.txt` / `ping_ok.txt`（覆盖 raw_socket_denied / route_unreachable / 通联三分支）。

## Phase 51 GAP

无（本 plan 期望 PASS）。如果 Linux runner 跑出 raw_socket_denied，那是 LEAK-06 / LEAK-08
的 backend GAP 透传到 ping 层；不归 LEAK-03 自己。
