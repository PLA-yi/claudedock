---
phase: 53-entrypoint
plan: "03"
subsystem: tests
tags: [smoke, fixture, sing-box, bash, shellcheck, phase-53]

requires:
  - phase: 53-entrypoint
    plan: "01"
    provides: managed-user 镜像 v4.0 基线（sing-box 1.13.3 + singbox uid=9000 + nftables/libcap2-bin）
  - phase: 53-entrypoint
    plan: "02"
    provides: entrypoint v4.0 fail-closed 启动序列 + default-deny.nft ruleset
provides:
  - tests/phase53/smoke.sh 本地一键烟测脚本（T-53-1..6 六条断言）
  - tests/phase53/fixtures/test-singbox-config.json 最小 sing-box 1.13.x 测试 config（direct outbound）
  - tests/phase53/README.md 使用说明 + 与 53-01/02 验收映射
  - Makefile phase53-smoke target（bash 入口 alias）
affects: [54, 55, 56]

tech-stack:
  added:
    - tests/phase53 烟测目录约定（独立于 tests/smoke BATS / tests/e2e Go）
  patterns:
    - 本地一键 docker run 自测：sing-box config fixture 走 bind-mount，--device /dev/net/tun + --cap-add NET_ADMIN
    - tun 接管路由的双 oracle 断言：ip route show table all 默认 dev=tun0 + curl 外网通
    - sing-box 死容器死时序断言：date +%s 起止 + docker inspect Running 轮询，≤3s 通过

key-files:
  created:
    - tests/phase53/smoke.sh
    - tests/phase53/fixtures/test-singbox-config.json
    - tests/phase53/README.md
    - .planning/phases/53-entrypoint/53-03-SUMMARY.md
  modified:
    - Makefile

key-decisions:
  - "smoke.sh 不进 CI（CI 走 Phase 55 e2e，testcontainers-go + testify/suite），只是开发期手测一键"
  - "fixture 用 direct outbound 而非真上游代理：仅证明 sing-box 起得来 + tun 接管 + curl 走 tun，出口 IP 强约束留 Phase 55 真上游 outbound 验证"
  - "T-53-3 加双 oracle（默认路由 dev + curl reachability）防 direct outbound 出口 IP = host eth0 IP 的歧义"
  - "Makefile target 加但不强制（make e2e Phase 56 才统一），仅作 alias 便于日常开发"

patterns-established:
  - "Phase 53 烟测目录约定 tests/phase53/：脚本 + fixtures/ 子目录 + README"
  - "shellcheck 通过 docker run --rm koalaman/shellcheck:stable 跑（macOS 本机无 shellcheck），cleanup 函数 trap 调用加 SC2329 disable"

requirements-completed: []

duration: ~12 min
completed: 2026-05-16
---

# Phase 53 Plan 03: 容器内手测 smoke 脚本 + fixture Summary

**Phase 53 自测层落地：tests/phase53/smoke.sh 本地一键烟测脚本 + 最小 sing-box 1.13.x fixture + README 使用说明，覆盖 T-53-1..6 六条断言。Makefile 加 phase53-smoke target alias。不接 v3.6 e2e harness（Phase 55 才完整接入）。**

## Performance

- **Duration:** ~12 min（含 V1 fixture sing-box check + V2 shellcheck 二次校验）
- **Started:** 2026-05-16T02:41:00Z
- **Completed:** 2026-05-16T02:53:00Z
- **Tasks:** 4 (T1..T4)
- **Files created:** 3（smoke.sh / test-singbox-config.json / README.md）+ 1 SUMMARY
- **Files modified:** 1（Makefile）

## Accomplishments

- **T1 — `tests/phase53/fixtures/test-singbox-config.json`**：36 行 sing-box 1.13.x 最小 config。结构与 v3.6 gateway 输出一致：
  - `log` info level + timestamp
  - `dns` 单 udp upstream `1.1.1.1`，final = upstream-dns
  - `inbounds` 单 tun 接口 `tun0`（172.19.0.1/30，mtu 1500，auto_route + strict_route + system stack + endpoint_independent_nat）
  - `outbounds` 单 `direct` outbound（**重要**：fixture 不接真上游代理，仅证明 sing-box 起得来 + tun 接管；出口 IP = host eth0 IP，由 T-53-3 双 oracle 断言路由确实经 tun0）
  - `route` 单条 sniff rule + final = direct-out + auto_detect_interface

- **T2 — `tests/phase53/smoke.sh`**：127 行 bash 脚本，`set -euo pipefail`。结构：
  - 准备：`mktemp -d` 写本临时 config + chmod 600 + `sudo chown root:root` (tolerant：mac/desktop 失败 `|| true`)
  - 启动：`docker run -d --device /dev/net/tun --cap-drop ALL --cap-add NET_ADMIN -v <config> --restart=on-failure:3` + sleep 6 等启动序列
  - **T-53-1**: `ip link show tun0` + `ps -o uid= -p $(pidof sing-box)` 拼断言 uid=9000
  - **T-53-2**: `docker exec test -f /etc/sing-box/config.json` 必须失败
  - **T-53-3**: `ip route show table all | grep ^default | awk {print $5}` 默认 dev=tun0 + `curl -fsS --max-time 10 https://api.ipify.org` 通
  - **T-53-4**: `getpcaps $$` 不含 `cap_net_admin/sys_admin/net_raw` + `sudo -n true` 必须失败
  - **T-53-5**: `kill -9 $(pidof sing-box)` 后 `docker inspect Running` 轮询，5s 内退出且 elapsed ≤3s
  - **T-53-6**: `RestartCount >= 1` + Running=true + restart 后 sing-box uid 仍 9000
  - cleanup 函数 trap EXIT 强制 `docker rm -f` + tmp 清理

- **T3 — `tests/phase53/README.md`**：55 行使用说明。包括：
  - `docker build` + `bash tests/phase53/smoke.sh` 标准流程，及 `make phase53-smoke` alias
  - 明确"不是 e2e"立场（Phase 55 才接 v3.6 harness）
  - T-53-1..6 与 53-01/53-02 验收 PLAN 映射表
  - fixture direct outbound 的语义说明 + T-53-3 双 oracle 解释
  - 平台限制（macOS desktop / arm vs linux/amd64 时序）说明，跑不通就 deferred-to-Phase-55-CI

- **T4 — Makefile `phase53-smoke` target**：在 `test-smoke` 之后加一行
  ```makefile
  phase53-smoke: ## Run Phase 53 image smoke tests (requires managed-user:v4-dev)
      bash tests/phase53/smoke.sh
  ```
  `make -n phase53-smoke` 输出 `bash tests/phase53/smoke.sh` ✅。

## Task Commits

按 plan 要求**单 atomic test commit + 1 SUMMARY commit**：

| Commit | Hash | 范围 |
|---|---|---|
| 1. `test(53-03): Phase 53 image smoke tests (T-53-1..6) + fixture` | `77794fe` | T1（fixture json）+ T2（smoke.sh）+ T3（README）+ T4（Makefile target） |

之后还有 1 个 SUMMARY commit（本文件）。

## Files Created/Modified

- `tests/phase53/smoke.sh` — 新建，127 行 bash 脚本，chmod +x。
- `tests/phase53/fixtures/test-singbox-config.json` — 新建，36 行 JSON。
- `tests/phase53/README.md` — 新建，55 行 markdown。
- `Makefile` — `test-smoke` target 之后追加 3 行（target + tab+command）。
- `.planning/phases/53-entrypoint/53-03-SUMMARY.md` — 本文件。

## Decisions Made

| ID | 内容 | 偏离 plan? |
|---|---|---|
| D-53-03-D1 | smoke.sh 加 `# shellcheck disable=SC2329` 注释，避免 cleanup 函数 trap 调用被 shellcheck 误报"never invoked" | ⚠️ 微调（plan 原文未含此注释，但 shellcheck stable 默认严格度下会报 SC2329 info；加注释让"shellcheck 0 警告"成立）|

无其他偏离。Plan 给的代码块逐字落地（fixture / smoke / README / Makefile target）。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - shellcheck 兼容] cleanup 函数加 SC2329 disable 注释**

- **Found during:** V2 shellcheck 校验
- **Issue:** `koalaman/shellcheck:stable` 在 SC2329（since 2024）下把 trap 调用的函数视为"never invoked"（info 级），导致默认 severity 下 shellcheck 退出码非 0 —— 不满足 plan V2 "0 error" 验收。
- **Fix:** 在 `cleanup() {` 上方加一行 `# shellcheck disable=SC2329  # invoked via trap above`。
- **Files modified:** `tests/phase53/smoke.sh`
- **Commit:** `77794fe`
- **Rationale:** Rule 1 微调，属语义上"假阳性"压制；保留注释解释原因，便于后续维护者理解。

无其他自动修复。Plan 原文 smoke.sh 在 macOS bash 5.x + linux bash 5.x 下 `bash -n` 均通过，无需结构调整。

## Verification Results

| ID | 项 | 期望 | 实际 | 状态 |
|---|---|---|---|---|
| V0  | `bash -n smoke.sh` | 0 退出 | `BASH-SYNTAX-OK` | ✅ |
| V1  | `sing-box check -c test-singbox-config.json` | `Configuration OK` | 通过（用 `ghcr.io/sagernet/sing-box:v1.13.3` 镜像跑，与 `managed-user:v4-dev` 同 sing-box 版本，0 退出码） | ✅ |
| V2  | `shellcheck smoke.sh` | 0 error | `koalaman/shellcheck:stable`，加 SC2329 disable 注释后 0 issue（info/warning/error 全 0） | ✅ |
| V3  | smoke.sh 本地端到端跑通 T-53-1..6 | 6 条全绿 | **Deferred-to-Phase-55-CI** | ⏸ |
| Makefile | `make -n phase53-smoke` | 输出 `bash tests/phase53/smoke.sh` | 输出一致 | ✅ |

**说明：**

1. **V1 镜像替换：** Plan 原文 `docker run --rm -v ... managed-user:v4-dev sing-box check ...`，但 `managed-user:v4-dev` 在本机不可用（依赖 Plan 53-01 V1 deferred-D-53-PRE-1 修复后的完整 build）。实测改用 `ghcr.io/sagernet/sing-box:v1.13.3`（Plan 53-01 通过 `ARG SINGBOX_VERSION=1.13.3` 决定的版本，与 v4-dev 镜像内 sing-box binary 同一来源同一版本），属等价 fixture 校验。CI 上 v4-dev 解锁后 plan 原命令亦能通过。
2. **V2 0 issue 通过：** 见 Deviations #1，加 SC2329 disable 后 shellcheck stable 默认 severity 下 0 输出 0 退出码。
3. **V3 Deferred-to-Phase-55-CI：** 本机 docker desktop 跑 V3 端到端被两个上游 deferred 项阻塞 ——
   - **D-53-PRE-1（Plan 53-01 deferred-items.md）** Claude Code 安装段远端 404/403 → `docker build -t managed-user:v4-dev` 在第 19 步退出 22；本地无 v4-dev 镜像可跑。
   - **D-53-02 V3..V7（Plan 53-02 SUMMARY）** 等价 deferred 项，同因依赖 v4-dev 镜像。
   - **平台兼容性风险**：即便 Claude Code 安装段修复，macOS docker desktop 上 `--cap-add NET_ADMIN` + `/dev/net/tun` 在 sing-box tun stack 下的时序（T-53-5 ≤3s）与 `--restart=on-failure` 触发行为，与 linux 原生 daemon 存在差异（CONTEXT.md 已知 Risk）。
   - 本 plan acceptance 的 V3 由 Phase 55 CI 在 ubuntu-latest runner 跑（GitHub Actions），届时 6 条 T-53-* 全绿才算最终交付。
4. **smoke.sh 单文件 < 200 行 + bash 4.0+ 兼容性：** 127 行 < 200 ✅；脚本未用 bash 5.x 专属语法（`((...))` / `case` / `local` / `printf` 都是 bash 3.2+），bash 4.0+ 兼容 ✅。

## Deferred Issues

- **V3 端到端 Deferred-to-Phase-55-CI：** 见 Verification Results 说明 #3。Phase 55 CI 上跑 6 条 T-53-* 全绿才算 Phase 53 最终交付，之前"上线就绪"在 PR 描述里需显式标注此项 unverified。
- **fixture 真上游代理验证 Deferred-to-Phase-55**：本 fixture 用 `direct` outbound 仅证 tun 链路通，**不验证出口 IP 是上游代理 IP**。Phase 55 e2e 需补一个真 outbound（vmess/shadowsocks 任一）的 fixture 跑出口 IP 强约束。
- **`chattr +i` overlay2 兼容性 spike**（继承 Plan 53-02 deferred）：smoke.sh 不直接验证此项，留给 Phase 55 e2e 加一条独立断言。

## Issues Encountered

- **shellcheck SC2329 假阳性**（见 Deviation #1，Rule 1 微调修复）。
- **本地 V3 阻塞**：D-53-PRE-1 + Plan 53-02 V3..V7 同属 deferred，本机无 v4-dev 镜像。归 deferred-to-CI，不在 53-03 修复责任范围。

## Threat Flags

无新增 threat surface。本 Plan 仅新增本地测试脚本与 fixture，不改动镜像 / entrypoint / 控制面。

**反向影响**：smoke.sh 帮助每次镜像构建前本地复现 NET-04（workspace 空 cap）、SEC-03（无 sudo）、EP-04（fail-closed）等已实现的安全约束，是降低回归风险的工具。

## User Setup Required

无。本 Plan 落地的 3 个文件 + Makefile 1 行不需要任何 host 配置。运行 `make phase53-smoke` 需先在本机有 `managed-user:v4-dev` 镜像（依赖 Phase 53-01 build 解锁，见 deferred-items.md D-53-PRE-1）。

## Next Phase Readiness

- ✅ Phase 53 三 plan 全部交付（53-01 Dockerfile + 53-02 entrypoint + 53-03 烟测 + fixture），Phase 53 自身在静态/语法/单元层面 acceptance 满足。
- ⏸ 上线 Phase 53 前依赖：（1）D-53-PRE-1 修复或在 CI 干净环境完整 build；（2）Phase 55 CI 跑通 T-53-1..6 全绿。
- ✅ Phase 54（控制面集成）可基于本 plan 的 smoke.sh 作为开发期 dogfooding 手段，控制面创建容器后跑一次 smoke.sh 即能本地验证 sing-box 启动序列回归。

## TDD Gate Compliance

Plan 53-03 frontmatter 无 `type: tdd`，单 task 也没标 `tdd="true"`，不适用 RED/GREEN/REFACTOR 强制门。本 plan 本身就是测试落地（test commit），承担 tests-as-spec 角色而非 TDD 实现循环。

## Self-Check: PASSED

- `[ -f tests/phase53/smoke.sh ]` ✅（127 行，chmod +x）
- `[ -f tests/phase53/fixtures/test-singbox-config.json ]` ✅（36 行 JSON，python json.load 通过）
- `[ -f tests/phase53/README.md ]` ✅（55 行）
- `[ -f .planning/phases/53-entrypoint/53-03-SUMMARY.md ]` ✅（本文件）
- `git log --oneline | grep 77794fe` 命中 ✅（atomic test commit 落地）
- `bash -n tests/phase53/smoke.sh` 0 退出 ✅
- `docker run --rm koalaman/shellcheck:stable smoke.sh` 0 退出 0 issue ✅
- `docker run --rm ghcr.io/sagernet/sing-box:v1.13.3 check -c <fixture>` 0 退出 ✅
- `make -n phase53-smoke` 输出 `bash tests/phase53/smoke.sh` ✅

---
*Phase: 53-entrypoint*
*Completed: 2026-05-16*
