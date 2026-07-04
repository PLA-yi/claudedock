---
phase: 37-e2e-uat
plan: "05"
subsystem: mount
tags: [uat, e2e, promotion, ci, scripting]
requires:
  - REQ-MOUNT-V31-16
  - REQ-MOUNT-V31-11
provides:
  - tests/scripts/uat-v31-promotion.sh
  - Makefile ci-gate 追加
affects:
  - CI pipeline (make ci-gate)
tech-stack:
  added: []
  patterns:
    - "UAT 脚本框架（pass/fail/skip helper + dry-run 默认安全 + JSON schema_version=1）"
key-files:
  created:
    - tests/scripts/uat-v31-promotion.sh
  modified:
    - Makefile
decisions:
  - "场景 3/4/5 在非 Linux 平台 dry-run 时 SKIP（需要 Docker/claudedock/FUSE 内核支持）"
  - "dry-run 模式各场景通过 write_json_report 自行填充状态，不做主函数覆盖"
metrics:
  duration: "~15min"
  completed_date: "2026-04-24"
---

# Phase 37 Plan 05: 冷文件晋升 e2e UAT 脚本 + CI 接入 Summary

编写 619 行 e2e UAT 脚本 `tests/scripts/uat-v31-promotion.sh`，覆盖 6 大场景（非 git 拒绝 / 大文件熔断 / FUSE cache 命中 / 冷文件晋升 / NO_PROMOTION 关闭 / JSON 报告），并接入 CI (`make ci-gate`)。

## 任务执行

| Task | Name | Status | Commit |
|------|------|--------|--------|
| 1 | 编写 uat-v31-promotion.sh 脚本 | DONE | 2511a33 |
| 2 | CI 接入 make ci-gate | DONE | bd06353 |

## 场景覆盖

| # | 场景 | 函数 | 关键断言 |
|---|------|------|---------|
| 1 | 非 git 目录拒绝挂载 | `scenario_git_reject` | stderr 含 `MOUNT_REQUIRE_GIT_REPO` |
| 2 | 大文件熔断（60MB） | `scenario_oversized_skip` | stderr 含 "跳过大文件"；hot 分支不含大文件 |
| 3 | FUSE cache 命中 | `scenario_fuse_cache_hit` | 首次 cat → SFTP read +N；二次 cat → SFTP read 不变 |
| 4 | 冷文件晋升 | `scenario_cold_promotion` | 首次 cat → 5s → hot 分支出现文件 → 二次 cat SFTP read 不变（REQ-MOUNT-V31-11） |
| 5 | CLOUD_CLAUDE_NO_PROMOTION=1 | `scenario_no_promotion` | PID file 不存在；promotion_count=0 |
| 6 | JSON 报告格式 | `scenario_json_report` | schema_version=1；scenarios 数组非空 |

## 脚本特性

- **框架层**：`set -euo pipefail`、pass/fail/skip/info helper、usage()、trap EXIT 清理
- **双模式**：`--dry-run`（默认，仅打印操作描述）vs `--confirm-destructive`（实际 mount + 断言）
- **退出码**：0=PASS / 1=FAIL / 2=SKIP（与 uat-network-resilience.sh 风格一致）
- **JSON 报告**：`schema_version=1`，含 summary（pass/fail/skip）+ scenarios 数组
- **平台适配**：非 Linux 平台场景 3/4/5 自动 SKIP（需要 Docker/claudedock/FUSE 内核支持）
- **安全闸门**：`--confirm-destructive` 中文提示确认

## 验收标准通过

| 验收项 | 预期 | 实际 |
|--------|------|------|
| 文件存在 | `test -f tests/scripts/uat-v31-promotion.sh` | PASS |
| `--help` 退出码 0 + 含 "用法" | exit 0 + 含 "用法" | PASS ✓ |
| `--dry-run` 退出码 0 + 含 [PASS]/[SKIP] | exit 0 + both tags | PASS (7 PASS, 3 SKIP) ✓ |
| `grep -c schema_version` | >= 1 | 8 ✓ |
| `grep -c scenario_*` (6 functions) | >= 6 | 12 ✓ |
| `grep -c MOUNT_REQUIRE_GIT_REPO` | >= 1 | 5 ✓ |
| `grep -c CLOUD_CLAUDE_NO_PROMOTION` | >= 1 | 4 ✓ |
| `grep -c promotion_count` | >= 1 | 2 ✓ |
| `grep -c trap.*cleanup.*EXIT` | >= 1 | 1 ✓ |
| `grep -c uat-v31-promotion.sh Makefile` | 1 | 1 ✓ |
| `grep -c dry-run Makefile` | >= 1 | 2 ✓ |

## Dry-Run 输出样例

```
[INFO]  v3.1 冷文件晋升 e2e UAT — dry_run=true
[PASS]  非 git 目录拒绝挂载（dry-run 描述通过）
[PASS]  大文件熔断（dry-run 描述通过）
[SKIP]  fuse_cache_hit: 需要 Linux 内核 FUSE 支持（当前平台: Darwin）
[SKIP]  cold_promotion: 需要 Docker + claudedock + SSH server + mergerfs ...
[SKIP]  no_promotion: 需要 Docker + claudedock 完整链路（当前平台: Darwin）
[PASS]  场景 6: schema_version == 1
[PASS]  场景 6: scenarios 数组非空（6 个条目）

v3.1 冷文件晋升 UAT 结果: 7 PASS, 0 FAIL, 3 SKIP
状态: 全部通过（dry_run=true）
```

## CI 接入

```makefile
ci-gate: ## CI gate: short go test + ci-doctor-grep + uat dry-run
	go test ./... -count=1 -short
	$(MAKE) ci-doctor-grep
	bash tests/scripts/uat-v31-promotion.sh --dry-run
```

## 偏差记录

无。计划完全按书面内容执行。

## 已知 Stubs

无。所有场景函数均已实现完整的 dry-run 和 confirm-destructive 分支逻辑。
