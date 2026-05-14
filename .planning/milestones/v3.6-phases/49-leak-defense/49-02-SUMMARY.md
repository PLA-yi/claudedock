---
phase: 49-leak-defense
plan: 02
title: LEAK-02 DoT (853) 旁路检测 (SUMMARY)
status: implemented
leak: LEAK-02
build_tag: "e2e && linux"
created: 2026-05-14
---

# Phase 49 Plan 02 SUMMARY: LEAK-02 DoT (853) 旁路检测

## 实际实现

- **探测方法**：`*GoldenPath.DigDoT(ctx, "1.1.1.1", "example.com")`，
  容器内 `bash -c 'command -v kdig >/dev/null && kdig +tls +time=3 @1.1.1.1 example.com
  || (timeout 5 openssl s_client -connect 1.1.1.1:853 -brief </dev/null)'`。
- **解析关键字**：`Verify return code: 0` / IPv4 字面量 → `Blocked=false`；
  `Connection refused` / `timeout` / `handshake failed` → `Blocked=true`。
- **独立 oracle**：`TcpdumpOnHostEth0` BPF `tcp port 853 and dst host 1.1.1.1 and src host <workerIP>`。
- **裁决**：`tdRes.packets > 0 || !dotRes.Blocked` → t.Fatalf。

## 与 Plan 偏差

无；按 Plan 走 kdig 优先 + openssl s_client 兜底。

## 实际命令 / 工具

- 优先 `kdig +tls +time=3 @1.1.1.1 example.com`（knot-dnsutils 提供）。
- Fallback `openssl s_client -connect 1.1.1.1:853 -brief </dev/null`。
- worker 缺工具 → `EnsureWorkerLeakTools` 调 `apt-get install -y knot-dnsutils openssl`。
- tcpdump：复用 Phase 48 sidecar 路径。

## 单测覆盖（darwin）

复用 shared `ClassifyLeakProbe` × 5；fixture：`testdata/leak/kdig_tls_timeout.txt` /
`openssl_s_client_refused.txt`。

## Phase 51 GAP

无（本 plan 期望 PASS）。
