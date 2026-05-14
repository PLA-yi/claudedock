---
phase: 51-qual-harden
plan: 51-05
status: completed
completed_at: 2026-05-14
gap_closure:
  - phase-49-gap-2
---

# 51-05 SUMMARY — nft 全规则加 counter + 169.254/16 显式 drop

## 落地清单

- `internal/network/firewall_helpers.go`：
  - 全部 build/add 系列 helper 在 `Verdict` 之前插入 `&expr.Counter{}`：
    `addOifAcceptRule` / `addIifAcceptRule` / `addOifCtEstablishedRule` /
    `addIifCtEstablishedRule` / `addIifTCPDportAcceptRule` /
    `buildOifSkuidIPPortAcceptExprs` / `buildOifNameAcceptExprs` /
    `buildOifNamedSetMatchAcceptExprs` / `buildOifUDPDportDropExprs`。
  - 新增 `buildIPDaddrCIDRDropExprs(cidr) ([]expr.Any, error)` 纯函数 +
    `addIPDaddrCIDRDropRule(conn, table, chain, cidr, comment)` wrapper。
    `comment` 写入 `Rule.UserData` 便于 `nft list ruleset` 输出识别。
  - 仅支持 IPv4 CIDR（IPv6 / garbage 输入返回 error）。
- `internal/network/worker_firewall_linux.go`：
  - `addIifSrcIPAcceptRule` / `addOifDstIPAcceptRule` /
    `addOifProtoDstPortAcceptRule` 三个 inline rule 也插入 Counter。
  - **新增**：在 `applyWorkerIPv4Rules` OUTPUT 链中，**lo / ESTABLISHED 之后、
    gwIP / DNS / 白名单 accept 之前**调
    `addIPDaddrCIDRDropRule(... "169.254.0.0/16", "linklocal-drop")`，闭
    Phase 49 GAP-2 显式 link-local drop 契约。
- `internal/network/firewall_helpers_test.go`：新增 3 单测
  - `TestBuildIPDaddrCIDRDropExprs_LinkLocal`（断言 daddr payload + /16 mask +
    network cmp + Counter + Verdict Drop）
  - `TestBuildIPDaddrCIDRDropExprs_RejectsIPv6`
  - `TestBuildIPDaddrCIDRDropExprs_RejectsGarbage`

## 验证

- `go build ./...` + `go vet ./...` PASS。
- `GOOS=linux go build ./...` + `GOOS=linux go vet ./...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./internal/network/... -count=1` PASS（含新增 3 单测）。
- `go test ./... -count=1` 全绿。
- 既有 `findExpr`-based helpers 单测零修改通过（添加 Counter 位置在 Verdict
  之前，类型断言不依赖 index）。
- 既有 worker_firewall_linux_test `TestApplyWorkerFirewallRules_CustomSSHPort` /
  `TestApplyThenCleanupThenApply` 期望 input 链 5 条规则不变（本 plan 只动 OUTPUT
  链，+1 个 link-local drop 规则）。

## 与 Phase 49 GAP-2 闭环关系

- Phase 49 LEAK-07 fixture `nft_ruleset_with_link_local_drop.txt` 是「修复后预期」，
  本 plan 落地后实际 `nft list ruleset` 输出含
  `ip daddr 169.254.0.0/16 counter drop comment "linklocal-drop"` 行，与
  fixture 行式 grep 一致。Linux runner 上 LEAK-07 用例预期由 fail → pass
  （darwin t.Skip 不验，由 deferred-to-CI 把关）。
- LEAK-05 IMDS 用例本来就 PASS（行为上靠链末 sbfw-drop 兜住），本 plan 落地后
  改由更显式的 linklocal-drop 命中，counter 命中数 ≥1，不会回归。

## 偏差

- 无。
