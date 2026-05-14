---
phase: 49-leak-defense
plan: 07
title: LEAK-07 link-local 显式 drop (SUMMARY)
status: implemented_with_gap
leak: LEAK-07
build_tag: "e2e && linux"
created: 2026-05-14
gap: phase-51-qual-06-or-07
---

# Phase 49 Plan 07 SUMMARY: LEAK-07 link-local 显式 drop

## 实际实现

- **oracle 命令**：`*GoldenPath.ListNftRulesOnHost(ctx)` —— 通过
  `docker run --rm --network host --cap-add NET_ADMIN --cap-add SYS_ADMIN
  nicolaka/netshoot nft list ruleset`，host-native 路径备选
  （`E2E_ALLOW_HOST_TCPDUMP=1` + euid==0）。
- **解析**：`ParseNftRules` 把 nft 文本展平为 `[]NftRule{Table, Chain, Action,
  Dst, Proto, Port, Comment, RawLine}`；`HasLinkLocalDropRule` 扫一组规则是否
  存在 Action="drop" 且 Dst 前缀 `169.254`。
- **裁决**：`HasLinkLocalDropRule(rules) == false` → **t.Errorf**（不阻塞其它用例），
  额外 t.Logf 出前 10 条 drop 规则便于审计。

## 与 Plan 偏差

无；`ParseNftRules` 解析覆盖 4 fixture（with_link_local_drop / no_link_local /
with_counters / empty）+ 2 额外断言（accept rule not missed / table-chain context propagated）。

## 实际命令 / 工具

- `docker run --rm --network host --cap-add NET_ADMIN --cap-add SYS_ADMIN
  nicolaka/netshoot:v0.13 nft list ruleset`。
- `nft list ruleset`（host-native 路径）。

## 单测覆盖（darwin）

`ParseNftRules` × 5 fixture + `HasLinkLocalDropRule` × 2 + `ParseNftCounters` × 3。
fixture：`testdata/leak/nft_ruleset_with_link_local_drop.txt` /
`nft_ruleset_no_link_local.txt` / `nft_ruleset_with_counters.txt` / `nft_ruleset_empty.txt`。

## Phase 51 GAP（必须修）

**预期 fail**：grep `internal/network/firewall_helpers.go` 未发现任何针对 IPv4
destination `169.254.0.0/16` 的显式 drop 规则。当前 nft 规则集只有：

- IPv6 chain `output6` 默认 `policy=drop`（firewall_helpers.go:24-30）。
- UDP dport drop（mDNS 5353 / LLMNR 5355 / NetBIOS 137/138；
  firewall_helpers.go:290-328）。
- 链末 `addLogDropRule` 兜底 `counter log prefix "sbfw-drop " drop`
  （firewall_helpers.go:330-349）。

链末兜底虽然能 *行为上* 让 169.254 流量被丢（→ LEAK-05 抓包仍 0 包），但**不是
显式 dst-CIDR 规则**，不满足审计契约。

**Phase 51 QUAL-06 / QUAL-07 修复方案**：
1. `internal/network/firewall_helpers.go` 新增 `addIPDaddrDropRule(conn, table,
   chain, "169.254.0.0/16")` helper。
2. 在 worker netns nft output 链构造时，在白名单 accept 规则**之前**插入：
   ```nft
   ip daddr 169.254.0.0/16 counter drop comment "linklocal-drop"
   ```
3. 同步更新 `internal/network/worker_firewall_linux.go` 调用点。

修复后本 plan 用例预期转 PASS。
