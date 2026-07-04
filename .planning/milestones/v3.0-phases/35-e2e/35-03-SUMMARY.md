---
phase: 35-e2e
plan: 03
subsystem: docs
tags: [runbooks, doctor, errcodes, apparmor, mutagen, mergerfs, persistent-volume]

requires:
  - phase: 29-v3-worker
    provides: image.lock 字面量 / host-preflight.sh::check_apparmor_fusermount3 / D-23 override 字面量
  - phase: 31-cli
    provides: errcodes Registry + 8 域文件 + 命名正则 + Severity 枚举
  - phase: 32-ssh-tmux
    provides: SESSION_* / NET_RECONNECT_* 错误码 + Reconnector + tmux 多端
  - phase: 33-claude-code-cli-admin-gc
    provides: claude-state-<id> volume 生命周期 + STATE_VOLUME_IN_USE_001
  - phase: 34-claudedock-doctor-v3
    provides: doctor 5 维度 18 项 + ExtendedExplanations 38 + ci-doctor-grep.sh
provides:
  - docs/runbooks/v3-upgrade-guide.md（控制面/镜像/CLI 三件套升级 + 7 项回滚触发）
  - docs/runbooks/v3-apparmor-deployment.md（D-23 字面量部署 + 三路并发 FUSE 烟测）
  - docs/runbooks/v3-doctor-troubleshoot.md（5 维度 18 项 cookbook + --fix 5 类 + M13 banner + M14 CI gate）
  - docs/runbooks/v3-persistent-volume-lifecycle.md（顶层导航 + Mutagen 数据卷 + mergerfs hot/cold）
  - docs/runbooks/v3-error-code-index.md（43 条 / 8 域 / 反向 diff 为空）
affects: [35-04-ci-gates, 35-05-acceptance-checklist, v3.1 开发者上手, v3.0 PR 评审]

tech-stack:
  added: []
  patterns:
    - "运维手册 Pattern G 头部模板（适用版本 + 关联需求 + ---）"
    - "错误码索引 5 列表格（Code/Severity/Message/NextAction/Extended）+ 反向 diff CI 断言"
    - "持久卷顶层 + 跳转链接整合规则（避免 v3-claude-state-volumes.md 内容复制）"

key-files:
  created:
    - "docs/runbooks/v3-upgrade-guide.md (237 行)"
    - "docs/runbooks/v3-apparmor-deployment.md (219 行)"
    - "docs/runbooks/v3-doctor-troubleshoot.md (340 行)"
    - "docs/runbooks/v3-persistent-volume-lifecycle.md (194 行)"
    - "docs/runbooks/v3-error-code-index.md (211 行)"
  modified: []

key-decisions:
  - "doctor 5 维度子节命名 ### 3.1-3.5 严格匹配 PLAN acceptance 的 grep 计数 = 5"
  - "错误码索引采用 5 列表格，Extended 列用 ✅ / — 区分长说明与豁免"
  - "持久卷手册自我设限：仅做导航 + 新内容（Mutagen / mergerfs hot-cold），claude-state OAuth 部分一律跳转"
  - "AppArmor override 文件改用 sudo install -d + sudo tee heredoc 写入（比裸 echo 更安全；与 host-preflight.sh fix 提示语义等价但操作更稳）"
  - "v3-error-code-index.md 表格使用 message/next_action 摘要而非全文，全文保留在代码注册表与 claudedock explain"

patterns-established:
  - "Pattern G 头部 5 章应用：5 份新手册全部 ≥ 5 个 ## 章节、含 适用版本：v3.0 起 头部、含 ### 快速诊断命令 小节"
  - "错误码索引反向 diff CI 断言：comm -23 注册表 vs 手册输出空 = 无漏项"
  - "运维手册到代码引用到函数级（::RunDoctor / ::createHost / ::check_apparmor_fusermount3）"

requirements-completed: [C6, M13, M5]

duration: 9min
completed: 2026-04-22
---

# Phase 35 Plan 03: v3.0 运维手册 5 章收口 Summary

**5 份 docs/runbooks/v3-*.md 落地 — 升级指南 + AppArmor 部署 + doctor 5 维度排障 + 持久卷顶层导航 + 43 条错误码索引（与 errcodes 注册表反向 diff 为空）**

## Performance

- **Duration:** ~9 min
- **Started:** 2026-04-22T11:00:08Z
- **Completed:** 2026-04-22T11:09:00Z（约值）
- **Tasks:** 3 / 3
- **Files modified:** 5（全部新建，0 修改）

## Accomplishments

- 5 章手册按 Pattern G 头部 + ≥ 5 个 ## 章节 + ### 快速诊断命令 小节统一风格落地
- AppArmor 手册锁定 D-23 三条字面量（`/etc/apparmor.d/local/fusermount3` + `capability dac_override,` + `apparmor_parser -r /etc/apparmor.d/fusermount3`），与 `deploy/scripts/host-preflight.sh::check_apparmor_fusermount3` L51-68 输出的 fix 提示一字不差对齐
- 升级指南覆盖控制面 / 镜像 / CLI 三件套 + 7 项回滚触发 + image.lock 字面量（`image_version: v3.0.0`）+ 0014 migration 锚点
- doctor 排障手册按代码 5 维度顺序（network → auth → ssh → mount → disk）建 `### 3.1-3.5` 五子节，每节列检查项 / 常见错误码 / 排障流程；含 `--fix` 5 类幂等修复表 / M13 降级第一屏 / M14 三段 CI gate 断言 / 3 个故障案例
- 持久卷手册自我设限只做顶层导航 + 新增 Mutagen 数据卷 / mergerfs hot-cold / REQ-F1-D 50MB 白名单内容，OAuth 持久化全部跳转 `v3-claude-state-volumes.md`（反向断言验证：未复制 §4 Audit 事件清单）
- 错误码索引按 8 域分组（AUTH×4 / DISK×3 / MOUNT×13 / NET×7 / SESSION×7 / SSH×2 / STATE×3 / SYSTEM×4 = 43 条），反向 diff `comm -23 registry vs 手册` 输出空，证明零漏项

## Task Commits

每个 task 原子 commit：

1. **Task 1：升级指南 + AppArmor 部署** — `88ea838` (docs)
2. **Task 2：doctor 排障手册 + 持久卷整合** — `baa0b7d` (docs)
3. **Task 3：错误码索引（43 条）** — `33de75a` (docs)

**Plan metadata commit:** （本 SUMMARY + STATE/ROADMAP 更新，最终一段提交）

## Files Created/Modified

- `docs/runbooks/v3-upgrade-guide.md`（237 行 / 9 个 ##）— v2.0 → v3.0 升级三件套 + 7 项回滚触发
- `docs/runbooks/v3-apparmor-deployment.md`（219 行 / 8 个 ##）— Ubuntu 25.04 AppArmor override 部署 + 三路 FUSE 烟测
- `docs/runbooks/v3-doctor-troubleshoot.md`（340 行 / 9 个 ##；`### 3.1-3.5` 5 子节）— doctor 5 维度 18 项排障 cookbook
- `docs/runbooks/v3-persistent-volume-lifecycle.md`（194 行 / 8 个 ##）— 持久卷顶层导航 + Mutagen 数据卷 + mergerfs hot-cold
- `docs/runbooks/v3-error-code-index.md`（211 行 / 7 个 ##；`### 3.1-3.8` 8 子节）— 43 条错误码索引

## 头部字段一致性验证结果

5 份手册全部满足：

```bash
for f in docs/runbooks/v3-upgrade-guide.md docs/runbooks/v3-apparmor-deployment.md \
         docs/runbooks/v3-doctor-troubleshoot.md docs/runbooks/v3-persistent-volume-lifecycle.md \
         docs/runbooks/v3-error-code-index.md; do
  grep -qF '适用版本：v3.0' "$f" && grep -qF '快速诊断命令' "$f" || echo "FAIL: $f"
done
# (无 FAIL 输出)
```

## 错误码索引交叉 diff 结果

```bash
comm -23 \
  <(grep -hE 'Code\s*=\s*"[A-Z]+_[A-Z]+' internal/claudedock/errcodes/*.go \
    | awk -F'"' '{print $2}' | sort -u) \
  <(grep -oE '[A-Z]+_[A-Z]+_[A-Z0-9_]+' docs/runbooks/v3-error-code-index.md \
    | grep -E '^(AUTH|DISK|MOUNT|NET|SESSION|SSH|STATE|SYSTEM)_' | sort -u)
# 输出空 — 注册表中 43 条 Code 全部在手册中出现
```

总数：43 条（≥ 42 Phase 34 锚点）。Severity 枚举命中 `INFO|WARN|ERROR|FATAL` 共 43 行。

## 运维手册到代码的引用映射表

| 运维手册 | 代码引用（函数级） |
|----------|-------------------|
| `v3-upgrade-guide.md` | `internal/runtime/tasks/worker.go::createHost` / `internal/claudedock/doctor/doctor.go::RunDoctor` / `deploy/docker/managed-user/entrypoint.sh::prepare_persistent_state` / `deploy/docker/managed-user/image.lock` / `scripts/install.sh` |
| `v3-apparmor-deployment.md` | `deploy/scripts/host-preflight.sh::check_apparmor_fusermount3` (L11-73, L51-68 字面量) / `scripts/verify-fuse-compat.sh` (L42-58 + 阶段 2-4) / `internal/claudedock/errcodes/system.go::SYSTEM_APPARMOR_FUSERMOUNT3_MISSING` |
| `v3-doctor-troubleshoot.md` | `internal/claudedock/doctor/doctor.go::RunDoctor` (L83-84 5 维度顺序) + `network.go/auth.go/ssh.go/mount.go/disk.go` + `fix.go::FixerRegistry/ApplyFixes` + `render.go` + `internal/claudedock/errcodes/codes.go::Format` + `scripts/ci-doctor-grep.sh` |
| `v3-persistent-volume-lifecycle.md` | `internal/runtime/tasks/worker.go::createHost/ensureDockerVolume` + `deploy/docker/managed-user/entrypoint.sh::prepare_persistent_state` + `internal/claudedock/errcodes/{mount,disk}.go` + 跳转 `docs/runbooks/v3-claude-state-volumes.md` |
| `v3-error-code-index.md` | `internal/claudedock/errcodes/codes.go::Registry/Format/MustRegister` + `explanations.go::ExtendedExplanations/ExplainExempt` + `{auth,disk,mount,net,session,ssh,state,system}.go` + `cmd/claudedock/explain.go` + `scripts/ci-doctor-grep.sh` |

## Decisions Made

- doctor 5 维度子节命名 `### 3.1-3.5` 严格匹配 PLAN acceptance 的 `grep -c '^### 3\.' = 5`
- 错误码索引采用 5 列表格 + 摘要文案；全文长说明保留在 `claudedock explain` 与 `explanations.go`，避免手册与代码漂移成本
- 持久卷手册严守 PATTERNS Pattern J，禁止复制 `v3-claude-state-volumes.md` 任何章节；自我定位为顶层 + 增量
- AppArmor 部署步骤改用 `sudo install -d + sudo tee <<'EOF'` 写入 override 文件（比裸 echo 更安全、防止 BOM/CRLF 污染），与 host-preflight.sh fix 提示语义等价
- 升级指南 ## 章节增到 9 个（PLAN 要求 ≥ 7），覆盖背景 / 前置 / 控制面 / 镜像 / CLI / 自检 / 回归与回滚 / 快速诊断 / 参考

## Deviations from Plan

None — plan executed exactly as written. PLAN acceptance 全部 grep 断言（行数 / 章节数 / 字面量 / 反向 diff）一次通过。

## Issues Encountered

None.

## Known Stubs

无。5 份手册均无 TODO/FIXME/placeholder 占位；所有命令可直接 copy-paste 执行。

## Threat Flags

无。本 plan 仅交付 markdown 文档，未引入新的网络端点 / 认证路径 / 文件访问模式 / schema 变更。手册中所有示例命令的 token / 密码 / UUID 均使用 `<ADMIN-JWT>` / `<container>` / `<account_id>` 占位（T-35-03-01 mitigation 落地）。

## User Setup Required

None — 无外部服务配置；手册即用即查。

## Next Phase Readiness

- Plan 04（CI gates）可直接引用本 plan 的错误码索引与 doctor 排障入口
- Plan 05（acceptance checklist）可在脚本中引用各手册路径作为 SC#9 证据
- v3.0 PR 评审：5 份手册可作为「运维侧知识资产收口」的直接交付物
- 无遗留 blocker

## Self-Check: PASSED

文件存在性：

```bash
for f in v3-upgrade-guide v3-apparmor-deployment v3-doctor-troubleshoot \
         v3-persistent-volume-lifecycle v3-error-code-index; do
  [ -f "docs/runbooks/${f}.md" ] && echo "FOUND: docs/runbooks/${f}.md"
done
# 5 行 FOUND
```

Commit 存在性：

```bash
for h in 88ea838 baa0b7d 33de75a; do
  git log --oneline | grep -q "^${h} " && echo "FOUND: ${h}" || echo "MISSING: ${h}"
done
# 3 行 FOUND
```

PLAN 全量 acceptance grep 集（含行数 / 章节数 / 头部 / 字面量 / 反向 diff）一次通过，未出现 FAIL 输出。

---

*Phase: 35-e2e*
*Plan: 03 — runbooks*
*Completed: 2026-04-22*
