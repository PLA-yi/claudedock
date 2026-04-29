---
phase: 17-image-entrypoint-baseline
plan: 02
subsystem: infra
tags: [docker, claude-code, install-fallback, retry-logic, cdn-403]

# Dependency graph
requires:
  - phase: 17-image-entrypoint-baseline (plan 01)
    provides: "基础 Dockerfile 和 entrypoint 编排脚本"
provides:
  - "Claude Code 安装步骤带 3 次重试 + GitHub release binary 回退"
  - "docker build 在 CDN 返回 HTTP 403 时仍可成功产出完整镜像"
affects: [18-network-isolation]

# Tech tracking
tech-stack:
  added: []
  patterns: ["curl -o + bash 分离替代 curl | bash 管道，避免 pipefail 兼容问题"]

key-files:
  created: []
  modified:
    - claude-shell/docker/Dockerfile

key-decisions:
  - "用 curl -o + bash 分离替代 curl | bash 管道，避免 /bin/sh (dash) 不支持 pipefail 的兼容性问题"
  - "回退路径从 GitHub API 获取 latest 版本号，而非硬编码版本"

patterns-established:
  - "Dockerfile 网络安装容错模式：重试循环 + binary 直接下载回退"

requirements-completed: [INFRA-01]

# Metrics
duration: 9min
completed: 2026-04-29
---

# Phase 17 Plan 02: Claude Code 安装容错 Summary

**Dockerfile Claude Code 安装步骤增加 3 次重试 + GitHub release binary 回退，解决 CDN 返回 HTTP 403 导致 docker build 失败的问题**

## Performance

- **Duration:** 9 min
- **Started:** 2026-04-29T05:41:47Z
- **Completed:** 2026-04-29T05:50:14Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishes

- Claude Code 安装步骤从单次 `curl install.sh | bash` 升级为带重试和回退的安装逻辑
- 官方 install.sh 最多重试 3 次（间隔 2s/4s/6s），全部失败后自动从 GitHub releases 下载 binary
- docker build 在 CDN 返回 HTTP 403 时成功完成，产出包含 Claude Code v2.1.123 的完整镜像
- 镜像内 `claude --version`、`sing-box version`、`DISABLE_AUTOUPDATER=1` 全部验证通过

## Task Commits

Each task was committed atomically:

1. **Task 1: 修复 Dockerfile Claude Code 安装步骤** - `aa84399` (fix)

## Files Created/Modified

- `claude-shell/docker/Dockerfile` - 将 Claude Code 安装步骤从单次 curl 管道替换为带重试和 GitHub release 回退的安装逻辑

## Decisions Made

- **curl -o + bash 分离替代 curl | bash 管道**：Docker RUN 默认使用 `/bin/sh -c`（Ubuntu 上是 dash），dash 不支持 `set -o pipefail`。通过先 `curl -o /tmp/claude-install.sh` 再 `bash /tmp/claude-install.sh` 分离两步，使 `set -e` 能正确捕获 curl 失败，无需依赖 pipefail
- **回退路径使用 GitHub API 获取 latest 版本号**：不硬编码版本号，通过 `https://api.github.com/repos/anthropics/claude-code/releases/latest` 动态获取

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] 修复 curl | bash 管道在 dash 下无法正确检测失败的问题**
- **Found during:** Task 1 (首次构建验证)
- **Issue:** 计划中的 `curl | bash` 管道在 `/bin/sh`（dash）下即使 curl 返回 403，bash 仍以 0 退出（空 stdin），导致 `installed=true` 但实际未安装
- **Fix:** 将 `curl | bash` 改为 `curl -o /tmp/claude-install.sh && bash /tmp/claude-install.sh`，分离下载和执行步骤，使 `set -e` 能正确捕获 curl 失败
- **Files modified:** `claude-shell/docker/Dockerfile`
- **Verification:** docker build 成功，重试逻辑正确触发 3 次后回退到 GitHub release
- **Committed in:** aa84399 (Task 1 commit)

**2. [Rule 1 - Bug] 修复 `claude --version` 验证命令在 ENV PATH 设置前找不到二进制**
- **Found during:** Task 1 (首次构建验证)
- **Issue:** 安装步骤的 `claude --version` 在 `ENV PATH` 设置之前执行，`/bin/sh` 无法在默认 PATH 中找到 `~/.local/bin/claude`
- **Fix:** 将 `claude --version` 改为 `"${HOME}/.local/bin/claude" --version` 使用绝对路径
- **Files modified:** `claude-shell/docker/Dockerfile`
- **Verification:** docker build 成功，输出 `2.1.123 (Claude Code)`
- **Committed in:** aa84399 (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (2 bug fixes)
**Impact on plan:** Both fixes necessary for correctness. No scope creep.

## Issues Encountered

- CDN 返回 HTTP 403 是已知问题（17-VERIFICATION.md 记录），本 plan 的核心目标就是解决此问题。回退到 GitHub release 路径验证通过。

## User Setup Required

None - no external service configuration required.

## Known Stubs

None - all plan goals achieved.

## Next Phase Readiness

- Phase 18 网络隔离实现可基于当前镜像继续开发
- Claude Code 安装已稳定（官方脚本 + GitHub release 双路径保障）

---
*Phase: 17-image-entrypoint-baseline*
*Completed: 2026-04-29*

## Self-Check: PASSED

- [x] claude-shell/docker/Dockerfile exists and modified
- [x] .planning/phases/17-image-entrypoint-baseline/17-02-SUMMARY.md exists
- [x] Commit aa84399 exists in git history
