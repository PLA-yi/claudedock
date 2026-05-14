---
phase: 49-leak-defense
plan: 05
title: LEAK-05 IMDS 阻断 (SUMMARY)
status: implemented
leak: LEAK-05
build_tag: "e2e && linux"
created: 2026-05-14
---

# Phase 49 Plan 05 SUMMARY: LEAK-05 IMDS 阻断

## 实际实现

- **探测方法**：`*GoldenPath.CurlIMDS(ctx, url)` 串行打两条 IMDS：
  - `http://169.254.169.254/latest/meta-data/`（AWS / GCP / Azure 共享入口）。
  - `http://169.254.170.2/v2/credentials/x`（ECS task IAM 角色）。
- **解析关键字**：`http_code == "200"` → `Blocked=false`（实锤泄漏）；
  `Connection refused` / `timed out` / `unreachable` → `Blocked=true`；
  http 4xx/5xx → `imds_http_<code>`（被代理/网关返回错而非真实 IMDS）。
- **独立 oracle**：`TcpdumpOnHostEth0` BPF `dst net 169.254.0.0/16 and src host <workerIP>`，
  count=5，window=6s（覆盖两次 curl 串行的总时长）。
- **裁决**：任一 curl 未阻断 / 抓包 > 0 → t.Fatalf。

## 与 Plan 偏差

- 计划中提到「nft counter 校验作为附加 oracle」未落地。原因：counter 校验属
  Plan 07 职责（LEAK-07 nft 规则形态）；本 Plan 严格只看「curl 失败 + host eth0 0 包」
  即可锁定 IMDS 不可达。

## 实际命令 / 工具

- `curl -sS -o /dev/null -w '%{http_code}' --max-time 3 <url>`。
- tcpdump sidecar 同 Plan 01。

## 单测覆盖（darwin）

复用 shared `ClassifyLeakProbe`；fixture：`curl_imds_timeout.txt` / `curl_imds_refused.txt`。

## Phase 51 GAP

间接依赖 LEAK-07 的 nft 规则形态。即使没有显式 `169.254.0.0/16` drop，
worker netns nft 链末 sbfw-drop 兜底也会让抓包 0；本 plan 行为预期 PASS。
但若 Phase 51 落地后 nft 规则更严格（QUAL-06 / QUAL-07），LEAK-05 仍稳。
