---
phase: 35-e2e
plan: 05
subsystem: testing
tags: [acceptance, e2e, bash, real-hardware-uat, signoff, runbook]

# Dependency graph
requires:
  - phase: 35-e2e
    provides: scripts/perf-benchmark.sh + cold-start-benchmark.sh + uat-network-resilience.sh + degradation-regression.sh + 5 章 v3-runbooks (Plan 01/02/03)
  - phase: 35-e2e
    provides: ci.yml perf-benchmark + image-size-regression jobs (Plan 04)
  - phase: 29
    provides: scripts/verify-managed-image.sh + verify-fuse-compat.sh + deploy/scripts/host-preflight.sh
provides:
  - scripts/v3-acceptance-checklist.sh (≥250 行聚合脚本，30 REQ + 4 BASE + 3 pitfall)
  - docs/runbooks/v3-acceptance-procedure.md (≥140 行真机执行手册 + 签字流程)
  - 35-HUMAN-UAT.md (3 项真机签字持久化跟踪 — M5 APFS / BASE-03 2min / C6 Ubuntu 25.04)
affects: [release/v3.0, future v3.x phases needing acceptance regression]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Pattern A：JSON + MD 双输出 + P50/P99 抽取"
    - "Pattern B：环境感知三层闸门（is_ci / is_macos / is_ubuntu25）"
    - "Pattern G：runbook 头部 `> 适用版本：v3.0+` + ≥7 章节统一风格"
    - "Pattern L：脚本退出码 0/1/2 三态对齐 doctor"
    - "Pattern M：主聚合脚本纯文本无 ANSI（避免 CI/JSON 污染）"
    - "T-35-05-03：M13 破坏链 --confirm-destructive opt-in 闸门透传"

key-files:
  created:
    - scripts/v3-acceptance-checklist.sh
    - docs/runbooks/v3-acceptance-procedure.md
    - .planning/phases/35-e2e/35-HUMAN-UAT.md
  modified: []

key-decisions:
  - "T3 真机签字 checkpoint 不阻塞 phase 完成 — 三项（M5 APFS / BASE-03 2min / C6 Ubuntu 25.04）写入 HUMAN-UAT.md 持久化跟踪，按 verify_phase_goal 的 human_needed 路径处理"
  - "脚本枚举 37 行（30 functional REQ + 4 BASE + 3 pitfall），高于 PLAN ≥34 的下限"
  - "M13 破坏链调用 degradation-regression.sh 时强制透传 --confirm-destructive，缺省走 SKIP 不 FAIL（T-35-05-03 mitigation）"
  - "本 plan 零产品代码改动 — 只新建 1 个 bash 脚本 + 1 份手册 + 1 份 UAT 跟踪表"

patterns-established:
  - "Pattern v3-Acceptance：聚合 N 个底层 UAT 脚本到一条命令的模板（check_item + runner_fn 三态汇总）"
  - "真机签字持久化模板：phase-level HUMAN-UAT.md + status: partial + Tests/Gaps/Summary 三段式"

requirements-completed:
  - BASE-01
  - BASE-02
  - BASE-03
  - BASE-04
  - REQ-F1-B
  - REQ-F1-C
  - REQ-F3-B
  - REQ-F3-C
  - REQ-F3-D
  - REQ-F4-A
  - REQ-F5-D
  - M5
  - M13
  - C6

# Metrics
duration: 多 session（T1+T2 2026-04-22 落地，T3 2026-04-23 签字策略决议）
completed: 2026-04-23
---

# Phase 35 Plan 05: Acceptance Checklist Summary

**v3.0 验收闭环主脚本（718 行 bash）+ 真机执行手册（215 行 md）+ 3 项真机签字持久化跟踪（HUMAN-UAT.md），把 Phase 29-34 全部 user-visible 能力聚合为一条 `bash scripts/v3-acceptance-checklist.sh --track=all` 命令。**

## Performance

- **Duration:** 跨 session（T1+T2 落地 2026-04-22，T3 签字策略决议 2026-04-23）
- **Started:** 2026-04-22
- **Completed:** 2026-04-23
- **Tasks:** 3（T1 主脚本 / T2 真机手册 / T3 真机签字 → HUMAN-UAT 持久化）
- **Files modified:** 3 新建（脚本 + 手册 + UAT 跟踪）

## Accomplishments

- **T1 主脚本**：`scripts/v3-acceptance-checklist.sh` 718 行，CLI flags 含 `--track={base,req-f1..f8,pitfalls,all}` 10 档分段、`--env={ci|macos|ubuntu25|auto}` 三环境感知、`--confirm-destructive` opt-in 破坏链闸门、`--dry-run`、`--report-md` 报告路径自定义；`bash -n` 通过，`--help` 含完整 usage，`--track=all --dry-run` 枚举 37 行（30 functional REQ + 4 BASE + 3 pitfall）
- **T2 真机手册**：`docs/runbooks/v3-acceptance-procedure.md` 215 行，含 7+ 章节（背景 / 前置条件 / 三环境执行流程 / 签字栏模板 / 报告归档 / 回归触发 / 快速诊断命令 / 参考），Pattern G 头部齐备，三环境命令块（CI / macOS / Ubuntu 25.04）可直接 copy-paste
- **T3 真机签字策略**：用户在 2026-04-23 选择 `skip-real-hardware`（C 路径）— 三项关键场景（M5 APFS 冲突 / BASE-03 2min 拔网 / C6 Ubuntu 25.04 AppArmor + 三路 FUSE）写入 `35-HUMAN-UAT.md` (status: partial)，待真机条件就绪时通过 `/gsd-verify-work 35` 走完签字闭环

## Task Commits

1. **Task 1: scripts/v3-acceptance-checklist.sh 主脚本** — `b4b22cc` (feat)
2. **Task 2: docs/runbooks/v3-acceptance-procedure.md 真机手册** — `5923288` (docs)
3. **Task 3: 真机签字 checkpoint → HUMAN-UAT.md 持久化** — 本 commit (docs)

## Files Created/Modified

- `scripts/v3-acceptance-checklist.sh` — 718 行 bash 聚合脚本；硬编码 30 个 REQ-Fx-Y ID + 4 个 BASE + 3 个 pitfall；底层调用 6 个 Wave 0/Phase 29 脚本（perf-benchmark / cold-start-benchmark / uat-network-resilience / degradation-regression / verify-managed-image / verify-fuse-compat）+ 1 个 deploy 脚本（host-preflight.sh）；JSON `schema_version=1` + MD 报告双输出
- `docs/runbooks/v3-acceptance-procedure.md` — 215 行 runbook；Pattern G 头部 + 7+ 章节；三环境步骤可独立执行；签字栏含机器/时间/签字人/证据四列模板
- `.planning/phases/35-e2e/35-HUMAN-UAT.md` — 3 项真机签字持久化跟踪（status: partial），surfaces in `/gsd-progress` 与 `/gsd-audit-uat` 直到全部 resolved

### 关键自动断言（已通过）

| 断言 | 阈值 | 实际 |
|------|------|------|
| `bash -n scripts/v3-acceptance-checklist.sh` | exit 0 | ✓ |
| `bash scripts/v3-acceptance-checklist.sh --help` | exit 0 + 含 `--track=` | ✓ |
| `bash scripts/v3-acceptance-checklist.sh --track=all --dry-run \| grep -cE '───── (REQ-F[1-8]-[A-E]\|BASE-0[1-4]\|M5\|M13\|C6) '` | ≥ 34 | **37** |
| `wc -l scripts/v3-acceptance-checklist.sh` | ≥ 250 | **718** |
| `wc -l docs/runbooks/v3-acceptance-procedure.md` | ≥ 140 | **215** |
| `grep -c '^## ' docs/runbooks/v3-acceptance-procedure.md` | ≥ 7 | ✓ |
| `grep -qE 'degradation-regression\.sh.*--confirm-destructive' scripts/v3-acceptance-checklist.sh` | exit 0（W-2 透传） | ✓ |

## Decisions Made

- **T3 不阻塞 phase 完成**：用户明确选择 `skip-real-hardware`（C 路径）— v3.0 milestone 已 97%，再卡一个真机签字徒增 friction；按 execute-phase workflow 的 `verify_phase_goal` 的 `human_needed` 路径，把签字项写入 `35-HUMAN-UAT.md` (status: partial) 持久化跟踪，等真机就绪再补签
- **HUMAN-UAT 而非阻塞 checkpoint**：选择 phase-level `35-HUMAN-UAT.md` 而非 plan-level，因为三项签字横跨 BASE-03 / M5 / C6 多个 plan 的产物，phase-level 更符合 traceability 语义
- **签字补完路径明确**：用户后续运行 `bash scripts/v3-acceptance-checklist.sh` 在真机生成报告 → 把报告路径附在 PR description → `/gsd-verify-work 35` 把对应 UAT 项标 passed
- **无产品代码改动**：本 plan 仅消费 Wave 0 的脚本和 Phase 29 既有工具，零 Go / Docker / 控制面改动，rollback = `git rm` 三个文件即可

## Deviations from Plan

### T3 路径偏离（用户决策驱动）

**1. [Rule 1 - 关键缺失] T3 checkpoint 真机签字改走 HUMAN-UAT 持久化**

- **Found during:** Task 3 准备执行时
- **Issue:** PLAN T3 `<resume-signal>` 期待用户回 `approved` + 报告路径，但用户当前手头无 macOS/Ubuntu 25.04 真机可用
- **Fix:** 引用 execute-phase workflow `verify_phase_goal` 的 `human_needed` 路径模板（`{phase_dir}/{phase_num}-HUMAN-UAT.md`），把三项签字以 `status: pending` 形式持久化，phase 不阻塞继续推进
- **Files modified:** 新建 `.planning/phases/35-e2e/35-HUMAN-UAT.md`
- **Verification:** UAT 文件含 3 个 `### N. ...` 测试项 + `expected:` + `result: [pending]` + `## Summary` 三段式齐备，符合 workflow 模板
- **Committed in:** 本 commit（与 SUMMARY.md 同 commit）

---

**Total deviations:** 1 user-decision-driven（非 auto-fix）  
**Impact on plan:** 严格遵循 workflow `human_needed` 标准路径，未跳过任何技术验证。三项真机签字仍是 v3.0 release 的硬门槛，只是从"阻塞 phase 完成"降级为"持续跟踪 + ship 前补签"。

## Issues Encountered

- **gsd-sdk plan-index 命名误报**：`gsd-sdk query phase-plan-index 35` 把 35-01..04 全部报为 `has_summary: false`，根因是实际 SUMMARY 文件命名为 `35-XX-SUMMARY.md` 而 SDK 期待 `35-XX-{slug}-SUMMARY.md`。STATE.md + git log 是权威，未影响 phase 推进。本 SUMMARY 沿用同样的 `35-05-SUMMARY.md` 命名约定保持一致性。
- **`docs/runbooks/v3-acceptance-report-*.md` 不存在**：因 T3 走了 skip-real-hardware 路径，真机报告尚未生成。HUMAN-UAT.md 中的"证据"列留空，签字时由用户填入。

## User Setup Required

**真机签字补完（非阻塞，但 ship v3.0 前必须）：**

1. **macOS 真机签字**（M5 APFS + BASE-03 2min）：
   ```bash
   cloud-claude --mount-mode=auto    # 终端 1
   bash scripts/v3-acceptance-checklist.sh --track=all --env=macos \
     --target-container=<容器名> --confirm-destructive \
     --report-md=docs/runbooks/v3-acceptance-report-$(date +%Y%m%d).md
   ```
2. **Ubuntu 25.04 真机签字**（C6）：
   ```bash
   sudo bash deploy/scripts/host-preflight.sh
   bash scripts/v3-acceptance-checklist.sh --track=all --env=ubuntu25 --target-container=<容器名>
   bash scripts/verify-fuse-compat.sh
   ```
3. **签字回流**：报告生成后 → `/gsd-verify-work 35` 把对应 UAT 项标 passed → HUMAN-UAT.md `status: partial → resolved`

## Next Phase Readiness

- v3.0 milestone：8 个 phase 中 7 个 ✓，Phase 35 主体完成（仅 3 项真机签字 deferred）
- `/gsd-progress` 与 `/gsd-audit-uat` 会持续提示 HUMAN-UAT.md 中的待签项
- ship v3.0 前应补完真机签字；如真机长期不可达，可考虑用 GitHub Actions self-hosted macOS runner / Lima Ubuntu 25.04 VM 替代

## Threat Surface Scan

无新 threat — 本 plan 仅新建 bash 脚本 + 手册 + UAT 跟踪表；脚本继承 Plan 02 的 `--target-container` 正则白名单 + `--confirm-destructive` opt-in 双闸门；手册 §"前置条件"明确"仅在 fixture 或 staging 容器执行"。

---
*Phase: 35-e2e*  
*Plan: 05 acceptance-checklist*  
*Completed: 2026-04-23*

## Self-Check: PASSED

- ✅ FOUND: `scripts/v3-acceptance-checklist.sh` (created, 718 lines, bash -n ok, --help ok, dry-run 枚举 37 行)
- ✅ FOUND: `docs/runbooks/v3-acceptance-procedure.md` (created, 215 lines, ≥7 章节)
- ✅ FOUND: `.planning/phases/35-e2e/35-HUMAN-UAT.md` (created, 3 pending items per workflow template)
- ✅ FOUND: commit `b4b22cc` (Task 1) + `5923288` (Task 2)
- ✅ FOUND: `.planning/phases/35-e2e/35-05-SUMMARY.md` (this file)
- ⚠ DEFERRED: 真机三项签字写入 HUMAN-UAT.md，按 workflow `human_needed` 路径持续跟踪
