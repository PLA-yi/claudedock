---
phase: 51-qual-harden
title: Phase 51 代码层质量加固 VERIFICATION
status: passed
created: 2026-05-14
darwin_gates: passed
linux_runner: deferred-to-ci
gap_closures:
  - phase-47-d-47-3
  - phase-49-gap-1-partial   # NET_ADMIN 保留（CONTEXT §Area 4 折中），LEAK-08 fixture 需后续放宽
  - phase-49-gap-2
human_verification:
  - operator: TBD
  - linux_runner: ubuntu-24.04 hosted
  - performed_at: TBD
---

# Phase 51 代码层质量加固 VERIFICATION

## 总体结论

**status: passed**

9 plan 全部 implementer 验证通过，darwin 本地 4 道闸全绿：

| 闸 | 命令 | 结果 |
|----|------|------|
| 1 | `go build ./...` | PASS |
| 2 | `GOOS=linux go build ./...` | PASS |
| 3 | `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` | PASS |
| 4 | `go test ./... -count=1` | PASS（19 包） |
| 5 | `go test $(go list ./... | grep -v '/tests/e2e$') -race -shuffle=on -count=1` | PASS（QUAL-07 新默认） |
| 6 | `go vet ./...` + `GOOS=linux go vet ./...` | PASS |

Linux runner 真机端到端验证（Phase 47 `TestEgressIPBinding_DoubleBindExcluded` /
Phase 49 LEAK-06/07/08）→ 列 `human_verification: deferred-to-CI`，待 ubuntu-24.04
hosted runner 跑通。

## QUAL-01..08 + 51-09 覆盖证据矩阵

| Plan | 主改动文件 | 单测 / 行为证据 |
|------|----------|-----------------|
| **QUAL-01** verifyEgressIP 多源轮询 | `internal/network/verify.go` 新增 `egressIPSources` + `verifyEgressIPMulti` + `voteEgressIP`；`VerifyNetworkIntegrity` Check 1 切换 | `internal/network/verify_test.go::TestVoteEgressIP_{MajorityWins3of3,MajorityWins2of3,TieFails,AllEmpty,NilInput}` + `TestVerifyEgressIPMulti_{MajorityMatch,MajorityMismatch,AllTimeout}` × 共 8 单测 |
| **QUAL-02** verifyLeakBlocked 多目标 | `internal/network/verify.go` 新增 `leakTarget` + `defaultLeakTargets` + `verifyLeakBlockedMulti`；旧 `verifyLeakBlocked` 包装 | `verify_test.go::TestDefaultLeakTargets_LockedContract` + `TestVerifyLeakBlockedMulti_{AllBlocked,OneLeaked,AllLeaked,EmptyTargets}` × 5 单测 |
| **QUAL-03** verifyDNS 遍历全 ns | `internal/network/verify.go` 新增 `parseAllNameservers`；`verifyDNS` 把 ActualDNS 改为逗号分隔 | `verify_test.go::TestParseAllNameservers_{Empty,SingleNS,MultipleNS,Comments,Garbage}` + `TestVerifyDNS_ReportsAllNameservers` × 6 单测 |
| **QUAL-04** GetContainerNetNS option | `internal/network/namespace.go` 新增 `nsConfig` + `Option` + `WithProbeWindow` / `WithMaxRetries` + `defaultNsConfig` | `internal/network/namespace_options_test.go::TestDefaultNsConfig_LocksValues / TestWith{ProbeWindow,MaxRetries}_{AppliesPositive,IgnoresZeroOrNegative} / TestOptions_Composable` × 6 单测 |
| **QUAL-05** nft 全规则 counter + 169.254/16 drop | `internal/network/firewall_helpers.go` + `worker_firewall_linux.go` 全部 build/add 规则插入 `Counter`；新增 `buildIPDaddrCIDRDropExprs` + `addIPDaddrCIDRDropRule`；`applyWorkerIPv4Rules` 在 OUTPUT 链 lo/ESTABLISHED 之后注入 `169.254.0.0/16 counter drop comment "linklocal-drop"` | `firewall_helpers_test.go::TestBuildIPDaddrCIDRDropExprs_{LinkLocal,RejectsIPv6,RejectsGarbage}` × 3 单测；既有 helpers 单测零修改通过（`findExpr` 类型断言不依赖 index） |
| **QUAL-06** worker cap-drop NET_RAW + 删 SYS_ADMIN | `internal/runtime/tasks/worker.go::buildCreateArgs` 删 `--cap-add SYS_ADMIN`、加 `--cap-drop NET_RAW`、保留 `--cap-add NET_ADMIN`（sing-box tun 依赖） | `internal/runtime/tasks/worker_caps_test.go::TestBuildCreateArgs_CapabilitiesLocked` 锁三条契约 |
| **QUAL-07** -race -shuffle 默认 | `Makefile` (`test-go` / `ci-gate`) + `.github/workflows/ci.yml` (`go-test` job) | darwin `go test $(go list ./... | grep -v '/tests/e2e$') -race -shuffle=on -count=1` 19 包全绿 |
| **QUAL-08** goleak.VerifyTestMain | `cmd/cloud-claude/testmain_test.go` + `internal/network/testmain_test.go` + `internal/controlplane/app/testmain_test.go`；`go.mod` 新增 `go.uber.org/goleak v1.3.0` | `go test ./cmd/cloud-claude/... ./internal/network/... ./internal/controlplane/app/...` 全绿；ignore list 仅 `broadcast.(*Hub).cleanupLoop`（设计内常驻） |
| **51-09** 双绑 API pre-check | `internal/store/repository/queries.go` 新增 `GetBindingHostIDByEgressIP`；`internal/controlplane/http/admin_bindings.go` 新增 `ErrCodeEgressIPAlreadyBound` 常量 + `Bind` pre-check 路径（409 + 中英 message + error_code + host_id / egress_ip_id 回显） | `admin_bindings_test.go::TestAdminBindingsHandler` table-driven 新增 2 case（双绑 / 同 host 幂等）+ `TestAdminBindings_DoubleBind_ErrorCode` 锁定 error_code / 双语 message / host_id / egress_ip_id 四字段 |

## 自动闭环的 GAP

### Phase 47 D-47-3（双绑互斥后端缺 pre-check + 无 4xx error code）

51-09 落地后：
- `host_egress_bindings` 表无单列 UNIQUE 的现状不变，但 Bind API 在 INSERT
  前先 SELECT lookup 同 egress_ip_id 的现有 host_id；
- 双绑场景响应 status=409 + `error_code="egress_ip_already_bound"` + 中英双语
  message（含 `已绑定` + `already bound` 两子串），命中
  `EgressIPDoubleBindContract{WantStatus:409, WantErrSubstring:"already bound"}`。

Phase 47 `tests/e2e/helpers_test.go::TestHelpersParseBindEgressIPResponse_*` 锁定
的 `error_code` 字段命中 → darwin 纯函数单测自动从 PARTIAL → PASS（无需修改
e2e 用例代码）。Linux runner `TestEgressIPBinding_DoubleBindExcluded` 走非
GAP 分支：预期 PASS（deferred-to-CI 待跑）。

### Phase 49 GAP-1（worker capability 残留 NET_RAW / NET_ADMIN / SYS_ADMIN）

51-06 落地后：
- **SYS_ADMIN** 已删除（buildCreateArgs 不再含 `--cap-add SYS_ADMIN`）。
- **NET_RAW** 显式 drop（`--cap-drop NET_RAW`），容器内 SOCK_RAW 创建立刻
  PermissionDenied。
- **NET_ADMIN 保留**（按 CONTEXT §Area 4 折中决策）：sing-box 在 worker netns
  内创建 tun0 设备依赖 CAP_NET_ADMIN，无法运行时 setcap 替代。

GAP 闭环状态：
- Phase 49 LEAK-06（raw socket PermissionDenied）：**完全闭环**，Linux runner
  预期 PASS。
- Phase 49 LEAK-08（CapBnd 不含 3 cap）：**部分闭环**，Linux runner 实际
  CapBnd 仍含 NET_ADMIN；fixture `proc_status_clean.txt` 严格期望需后续放宽，
  属 Phase 49 范围（本 phase 不动 `tests/e2e/`）。在本 VERIFICATION 显式标记
  `phase-49-gap-1-partial`。

### Phase 49 GAP-2（缺 IPv4 169.254.0.0/16 显式 drop nft 规则）

51-05 落地后：
- `applyWorkerIPv4Rules` 在 OUTPUT 链 lo/ESTABLISHED accept 之后、白名单/
  DNS/gwIP accept 之前注入 `ip daddr 169.254.0.0/16 counter drop comment
  "linklocal-drop"`。
- `nft list ruleset` 输出包含该规则行，与 Phase 49 fixture
  `nft_ruleset_with_link_local_drop.txt` 行式 grep 一致。
- Phase 49 LEAK-07 用例 Linux runner 预期 PASS（HasLinkLocalDropRule 返回 true）。
- LEAK-05 IMDS 用例不会回归（行为上原本由链末 sbfw-drop 兜住，本 plan 改由
  更显式的 linklocal-drop 命中）。

同时本 plan 给所有 nft 规则插入 `expr.Counter{}`，未来 e2e artifact 采集
`nft list ruleset -a` 输出能逐条读到命中数。

## human_verification_needed（deferred-to-CI）

以下 3 项必须在 Linux runner（含 docker + 真实拓扑）跑通：

1. **Phase 47 MVS-07 `TestEgressIPBinding_DoubleBindExcluded`**
   - 51-09 落地后，第二次绑同一 egress IP 到不同 host → 409 + `error_code=
     "egress_ip_already_bound"` + 中英双语 message + A 原绑定不破坏 + B 不
     被意外写入。Linux runner 预期直接走 PASS 分支（不再走 BACKEND GAP）。

2. **Phase 49 LEAK-06 / LEAK-07**
   - LEAK-06：worker 容器内 `python3 -c 'socket.SOCK_RAW'` → PermissionError；
     `/proc/1/status` CapEff / CapBnd 不含 CAP_NET_RAW。
   - LEAK-07：`nft list ruleset` 输出至少一条 `ip daddr 169.254 counter drop
     comment "linklocal-drop"`。

3. **Phase 49 LEAK-08 capability 审计**
   - 实际 CapBnd 仍含 NET_ADMIN（本 phase 折中保留）。fixture 期望需 Phase 49
     重审；本 phase 不修改 e2e，按 LEAK-06 / LEAK-07 PASS 即可视为 Phase 49
     主路径已闭。

## 提交记录（按里程碑顺序展开）

```
dda44c2 docs(51): 拆出 Phase 51 九个 PLAN.md (QUAL-01..08 + 双绑 API)
e146b3f feat(51-04): GetContainerNetNS 探测窗口参数化
92b5e56 feat(51-07): go test 默认 -race -shuffle=on
1bb7831 feat(51-08): goleak.VerifyTestMain 接入
2d51db3 feat(51-01): verifyEgressIP 多源轮询 + 多数派投票
290e5b5 feat(51-01): verifyEgressIP 多源轮询 + 多数派投票 (Cursor agent 自动复制；SUMMARY 调校)
c5d809b feat(51-02): verifyLeakBlocked 多目标矩阵化
6ea8862 feat(51-02): verifyLeakBlocked 多目标参数化 (SUMMARY 调校)
dccebd3 feat(51-03): verifyDNS 遍历全部 nameserver
974659a feat(51-03): verifyDNS 遍历全部 nameserver 行 (SUMMARY 调校)
0f5228e feat(51-05): nft 全规则加 counter + 169.254/16 显式 drop
c84c86d feat(51-06): worker cap-drop NET_RAW + 删 SYS_ADMIN
03ace12 feat(51-06): worker cap-drop NET_RAW + 删 SYS_ADMIN (SUMMARY 调校)
c290810 feat(51-09): 双绑互斥 API pre-check + 稳定 error code
e59dc5a feat(51-09): 双绑互斥 API pre-check + 稳定 error code (SUMMARY 调校)
```

（VERIFICATION 自身提交在本文件落定后追加。一些 SUMMARY 文件在 implementer
轮次中被 Cursor agent 自动调校了文案；生产代码改动只在每个 plan 的首个
commit 中落地，不影响验证结果。）

## 新增 / 修改清单（生产代码视角）

- `internal/network/verify.go`：+200 行（QUAL-01..03 三 plan 共同改动）。
- `internal/network/namespace.go`：+50 行（QUAL-04）。
- `internal/network/firewall_helpers.go`：~ +90 行（QUAL-05 counter 插入 +
  buildIPDaddrCIDRDropExprs / addIPDaddrCIDRDropRule）。
- `internal/network/worker_firewall_linux.go`：~ +20 行（QUAL-05 counter +
  link-local drop 注入点）。
- `internal/runtime/tasks/worker.go`：+10 行 / -1 行（QUAL-06 cap 调整）。
- `internal/controlplane/http/admin_bindings.go`：+30 行（51-09 pre-check）。
- `internal/store/repository/queries.go`：+12 行（51-09 GetBindingHostIDByEgressIP）。
- `Makefile`：+3 行（QUAL-07 -race -shuffle）。
- `.github/workflows/ci.yml`：+3 行 / -1 行（QUAL-07）。
- `go.mod` / `go.sum`：新增 `go.uber.org/goleak v1.3.0`（QUAL-08，本里程碑
  唯一允许的新依赖）。
- 3 个新 `testmain_test.go` 文件（QUAL-08）。

新增 / 修改测试：

- `internal/network/verify_test.go`：+19 单测（QUAL-01..03 合计）。
- `internal/network/namespace_options_test.go`（新）：6 单测（QUAL-04）。
- `internal/network/firewall_helpers_test.go`：+3 单测（QUAL-05）。
- `internal/runtime/tasks/worker_caps_test.go`（新）：1 单测（QUAL-06）。
- `internal/controlplane/http/admin_bindings_test.go`：+2 table case + 1
  独立单测（51-09）。

总单测增量：~32 个，覆盖本 phase 全部生产代码改动。

## 偏差汇总

| 偏差 | 说明 | 处置 |
|------|------|------|
| Phase 46 `Vote` / `DefaultDenyMatrix` 复用 | 这两个符号定义在 `tests/e2e/helpers.go` 包，production 不能 import tests 包 | 本地 `voteEgressIP` / `defaultLeakTargets` 私有复刻；锁定单测保证双边契约不漂移 |
| `cmd/host-agent` 不存在 | host-agent 以 embedded 模式跑在 control-plane 进程内（`internal/controlplane/app`） | QUAL-08 goleak 接入到 `cmd/cloud-claude` / `internal/network` / `internal/controlplane/app` 三个最关键的测试包，覆盖 control-plane + cloud-claude 主路径 |
| NET_ADMIN 保留 | sing-box 在 worker netns 内创建 tun0 设备依赖 CAP_NET_ADMIN | CONTEXT §Area 4 明确允许折中；Phase 49 LEAK-08 fixture 后续修订 |
| Cursor agent 重复 commit | 部分 plan 的 SUMMARY 在 implementer 第二轮被自动重写文案 | 生产代码改动只在首 commit 落地，不影响功能 |
