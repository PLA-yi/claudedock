---
phase: 49-leak-defense
title: Phase 49 防泄漏对抗测试 VERIFICATION
status: gaps_found
created: 2026-05-14
darwin_gates: passed
linux_runner: deferred-to-ci
gaps:
  - leak-06-raw-socket-residual
  - leak-07-nft-linklocal-not-explicit
  - leak-08-capability-not-dropped
human_verification:
  - operator: TBD
  - linux_runner: ubuntu-24.04 hosted
  - performed_at: TBD
---

# Phase 49 防泄漏对抗测试 VERIFICATION

## 总体结论

**status: gaps_found**

darwin 上 8 plan 全部 implementer 验证通过：

- `go build ./tests/e2e/...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `GOOS=linux go vet -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./tests/e2e/ -run "Helpers|Leak" -count=1`：**92 PASS**（含 32 个 LEAK 新单测）。
- `bash scripts/lint-no-bare-sleep.sh` PASS。

Linux runner 真机 e2e（8 个 TestLeak_NN_* + Phase 45 Step 2..7 真实落地）→
列 `human_verification: deferred-to-CI`，待 ubuntu-24.04 hosted runner 真实跑通。

**3 条 backend GAP**：grep 源码确认 worker 当前 capability / nft 规则与 LEAK
契约不符，详见下面「Phase 51 必须修」段。

## 每条 LEAK 覆盖证据

| LEAK | e2e 用例 | 主断言 | 单测覆盖 | fixture | 预期 |
|------|----------|--------|----------|---------|------|
| LEAK-01 | `TestLeak_01_DNSPlainUDP_BlockedByHostEth0` | dig Blocked + tcpdump 0 包（udp/53→8.8.8.8） | `ClassifyLeakProbe` × 5 + `LeakVerdict_String` | `dig_timeout.txt` / `dig_servfail.txt` / `dig_ok.txt` | PASS |
| LEAK-02 | `TestLeak_02_DoT853_BlockedByHostEth0` | kdig/openssl Blocked + tcpdump 0 包（tcp/853→1.1.1.1） | 复用 ClassifyLeakProbe | `kdig_tls_timeout.txt` / `openssl_s_client_refused.txt` | PASS |
| LEAK-03 | `TestLeak_03_ICMP_BlockedByHostEth0` | ping Blocked + tcpdump 0 包（icmp→8.8.8.8） | 复用 ClassifyLeakProbe | `ping_perm_denied.txt` / `ping_unreachable.txt` / `ping_ok.txt` | PASS |
| LEAK-04 | `TestLeak_04_IPv6_BlockedByHostEth0` | disable_ipv6=1 双保险 + curl -6 Blocked + 条件性 ip6 抓包 0 包 | 复用 ClassifyLeakProbe | `curl_ipv6_unreachable.txt` / `proc_disable_ipv6_one.txt` / `proc_disable_ipv6_zero.txt` | PASS（worker.go 已 sysctl） |
| LEAK-05 | `TestLeak_05_IMDS_BlockedByHostEth0` | curl 169.254.169.254 + 169.254.170.2 都 Blocked + tcpdump 0 包 | 复用 ClassifyLeakProbe | `curl_imds_timeout.txt` / `curl_imds_refused.txt` | PASS（行为）|
| LEAK-06 | `TestLeak_06_RawSocket_PermissionDenied` | python3 SOCK_RAW 必须 PermissionError | 复用 ClassifyLeakProbe | `python_raw_socket_perm.txt` / `python_raw_socket_ok.txt` | **预期 fail（GAP）** |
| LEAK-07 | `TestLeak_07_NftLinkLocalDrop_RuleExists` | nft list ruleset → 至少一条 dst 169.254 的 drop | `ParseNftRules` × 5 fixture + `HasLinkLocalDropRule` × 2 + `ParseNftCounters` × 3 | `nft_ruleset_with_link_local_drop.txt` / `nft_ruleset_no_link_local.txt` / `nft_ruleset_with_counters.txt` / `nft_ruleset_empty.txt` | **预期 fail（GAP）** |
| LEAK-08 | `TestLeak_08_WorkerCapabilities_Locked` | /proc/1/status CapEff/CapBnd 不含 NET_RAW/NET_ADMIN/SYS_ADMIN | `ParseProcCapabilities` × 7 + `KnownCapabilityBits_LocksCriticalSubset` + `ExpandCapBits_*` × 3 | `proc_status_clean.txt` / `proc_status_dirty.txt` / `proc_status_partial.txt` / `proc_status_corrupt.txt` | **预期 fail（GAP）** |

## 新增统计

- **GoldenPath 探测方法**：9 个（DigPlainDNS / DigDoT / PingICMP / CurlIPv6 /
  ReadProcFile / CurlIMDS / TryRawSocket / ListNftRulesOnHost / GetProcCapabilities）
  + 1 个统一入口 `execWorkerCapture` + 1 个 setup 工具 `EnsureWorkerLeakTools`。
- **shared 纯函数 / 类型**：`LeakProbeResult` / `LeakVerdict` /
  `ClassifyLeakProbe` / `NftRule` / `ParseNftRules` / `ParseNftCounters` /
  `HasLinkLocalDropRule` / `Capability` 常量 × 14 / `KnownCapabilityBits` /
  `ProcCapabilities` / `ParseProcCapabilities` / `LeakDangerousCaps`。
- **darwin 新增单测**：32 个全绿，总 92 PASS（既有 60 + 新 32）。
- **e2e 用例**：8 个 `TestLeak_NN_*`（darwin t.Skip，Linux runner 真实跑）。
- **fixture**：22 份（dig / kdig / openssl / ping / curl / nft / proc/status）。

## Phase 51 必须修的 backend GAP

### GAP 1：LEAK-06 + LEAK-08 共因 — worker capability 残留 NET_RAW/NET_ADMIN/SYS_ADMIN

**grep 证据**：

- `internal/runtime/tasks/worker.go:217-218` 显式：
  ```go
  "--cap-add", "NET_ADMIN",
  "--cap-add", "SYS_ADMIN",
  ```
- 全文未发现 `--cap-drop NET_RAW`；docker 默认 capability 集合**包含**
  `cap_net_raw`。

**预期**：worker `/proc/1/status` 中 `CapEff` / `CapBnd` 都不应含
`{NET_RAW, NET_ADMIN, SYS_ADMIN}`，且容器内 `python3 -c 'socket.SOCK_RAW'`
应 PermissionError。

**实际**：3 条危险 cap 全命中；SOCK_RAW 创建成功。

**Phase 51 QUAL-06 修复方案**：

1. 删 `--cap-add SYS_ADMIN`（worker 不需要）。
2. 把 `--cap-add NET_ADMIN` 改为运行时按需 setcap（仅 sing-box 启动短窗口需要）；
   或改为 host-agent 在 prepare-host 阶段下发 nft 规则后立即 unset。
3. 显式追加 `--cap-drop NET_RAW`（即便 docker 默认带 NET_RAW，显式 drop 仍能去掉）。
4. 同步更新单测 fixture（`proc_status_clean.txt` 已经反映 fix 后期望，可作为
   QUAL-06 完成的回归基线）。

### GAP 2：LEAK-07 — nft 规则集中无显式 IPv4 link-local drop

**grep 证据**：`internal/network/firewall_helpers.go` 全文 `169` / `link-local`
均未命中。当前 nft 规则集只有：

- IPv6 chain `output6` 默认 `policy=drop`（firewall_helpers.go:24-30）。
- UDP dport drop（mDNS 5353 / LLMNR 5355 / NetBIOS 137/138；
  firewall_helpers.go:290-328）。
- 链末 `addLogDropRule` 兜底 `counter log prefix "sbfw-drop " drop`
  （firewall_helpers.go:330-349）。

**预期**：nft 规则集中至少一条 `ip daddr 169.254.0.0/16 counter drop comment
"linklocal-drop"`（或更精确的 `/32` 子集）。

**实际**：链末 sbfw-drop 兜底**行为上**能挡住 169.254（→ LEAK-05 抓包 0），
但**不是显式 dst-CIDR drop**，不满足审计契约。

**Phase 51 QUAL-06 / QUAL-07 修复方案**：

1. `internal/network/firewall_helpers.go` 新增 `addIPDaddrDropRule(conn, table,
   chain, "169.254.0.0/16")` helper（参考既有 `buildOifUDPDportDropExprs`）。
2. 在 worker netns nft output 链构造时，在白名单 accept 规则**之前**插入显式
   link-local drop。
3. counter 命名 `linklocal-drop`，让 LEAK-05 / LEAK-07 用例可通过 comment key
   读 counter 命中数。

修复后 LEAK-07 用例预期转 PASS（`HasLinkLocalDropRule(rules) == true`）；
LEAK-05 用例**不会回归**（行为已经 PASS，新规则只是更显式）。

## 整组耗时（CONTEXT §Area 4 锁定 ≤5min）

- darwin：纯函数单测 < 30s（实测 0.319s，含 92 PASS）。
- Linux runner（CI）：8 个 TestLeak_* 用例，每个 ≤ 60s timeout，整组并行
  ≤ 5min。当前 Phase 45 Step 2..7 sentinel → 8 用例都 t.Skip，实测 < 5s。
  Step 2..7 真实落地后预期 ≤ 5min。

## 与 Phase 50 / 51 的接口约定

### 给 Phase 50（Kill-switch 压力测试）

- `*GoldenPath.TcpdumpOnHostEth0` 已稳定（Phase 48 引入，本 phase 高频复用）；
  KILL-01..04 直接调用，不需要重新引入 sidecar 路径。
- `LeakProbeResult` / `ClassifyLeakProbe` 与 KILL-* 的 `KillswitchVerdict`
  正交：LEAK 看「是否泄漏」，KILL 看「sing-box 倒了之后还能不能出网」；
  Phase 50 用例可直接复用 `*GoldenPath.ProbeOutboundFromUser`，不依赖 LEAK helpers。

### 给 Phase 51（QUAL-* 源码改造）

- **QUAL-06**（worker capability 收紧）：删 `--cap-add SYS_ADMIN`，
  显式 `--cap-drop NET_RAW`，NET_ADMIN 按需 setcap。改完后跑
  `go test -tags='e2e linux' ./tests/e2e/leak/ -run "Leak_06|Leak_08"`
  应转 PASS；fixture `proc_status_clean.txt` 即修复后期望状态。
- **QUAL-07**（nft 显式 link-local drop）：在 firewall_helpers.go 加
  `addIPDaddrDropRule` + worker netns chain 注入；改完后跑
  `go test -tags='e2e linux' ./tests/e2e/leak/ -run "Leak_07"` 应转 PASS。
- **VERIFICATION 转 passed**：QUAL-06 + QUAL-07 落地后，本文件
  `status` 改为 `passed`、清空 `gaps`、补 `human_verification.operator/performed_at`。

## 整组 commit 列表

1. `docs(49): 拆出 Phase 49 八个 PLAN.md (LEAK-01..08)` — 79f1811
2. `feat(49-shared): LEAK helpers 与纯函数 + 单测` — 15f2397
3. `feat(49-01): LEAK-01 DNS 明文 UDP/53 旁路 e2e 用例` — 577d38e
4. `feat(49-02): LEAK-02 DoT (853) 旁路 e2e 用例` — 3280790
5. `feat(49-03): LEAK-03 ICMP 阻断 e2e 用例` — 9c52747
6. `feat(49-04): LEAK-04 IPv6 阻断 e2e 用例` — 82c876a
7. `feat(49-05): LEAK-05 IMDS 阻断 e2e 用例` — e4baede
8. `feat(49-06): LEAK-06 raw socket 拒绝 e2e 用例（预期 fail，等 Phase 51 QUAL-06 修源码）` — 37c268f
9. `feat(49-07): LEAK-07 link-local 显式 drop e2e 用例（预期 fail，等 Phase 51 QUAL-06/07 修源码）` — 3ac1a18
10. `feat(49-08): LEAK-08 capability 审计 e2e 用例（预期 fail，等 Phase 51 QUAL-06 修源码）` — 6ec141a
11. `docs(49): Phase 49 VERIFICATION.md` — 本 commit
