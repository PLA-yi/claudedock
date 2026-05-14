---
phase: 51-qual-harden
plan: 51-03
title: QUAL-03 verifyDNS 遍历全部 nameserver
status: completed
completed: 2026-05-14
---

# 51-03 QUAL-03 — `verifyDNS` 遍历全部 nameserver 行 — SUMMARY

## 变更

- `internal/network/verify.go`：
  - 新增 `parseAllNameservers(rawContent string) []string`：按行扫描 trim 后以 `nameserver` 起首的行收集第二字段；忽略 `#` / `;` 注释、空行、无操作数行；空输入或无 nameserver 返回 `nil`。
  - `verifyDNS` 改用 `parseAllNameservers` 取出全部 nameserver，`result.ActualDNS` 写入逗号分隔串（单 ns 退化为原值）；`firstNS` 通过 `nameservers[0]` 取，`DNSCorrect` 判定路径不变（仍是 `rawContent != resolvConfContent` 严格比对 + 首行 nameserver 等于 expectedDNS 双保险）。
- `internal/network/verify_test.go`：新增 6 个单测：
  - `TestParseAllNameservers_Empty / SingleNS / MultipleNS / Comments / Garbage`：纯函数 5 case。
  - `TestVerifyDNS_ReportsAllNameservers`：fake nsenter 返回双 nameserver → `DNSCorrect=false` + `ActualDNS="172.19.0.1,8.8.8.8"`。

## 闸

- `go build ./...` PASS。
- `GOOS=linux go build ./...` + `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go vet ./...` PASS。
- `go test ./... -count=1`：19 包全 PASS（含 `internal/network` 6 新增 + 既有 `TestFirstNetworkError_DNSLeak{,_NilProxy}` 零回归）。

## 偏差

- `ActualDNS` 字段语义：单 ns 不变（与旧实现等价为单 IP 字符串）；多 ns 改为逗号分隔。既有 fake 桩与 e2e 用例只触发单 ns 场景，零回归。
- 无新增字段（CONTEXT 中提到的 `ActualDNSAll []string` 未引入），避免 `VerifyResult` 结构外露字段变化。

## 风险闭环

- `DNSCorrect` 判定逻辑保持「整 buffer 相等 + 首行 nameserver 等于 expectedDNS」，e2e 行为契约不变。
- 不交叉 51-01 / 51-02 / 51-05 / 51-06 / 51-09。
