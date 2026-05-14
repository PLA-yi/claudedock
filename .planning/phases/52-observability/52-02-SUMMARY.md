---
phase: 52-observability
plan: 02
title: 5 子目录 README + metadata.txt 完整契约 (OBS-02)
status: shipped
requirement: OBS-02
created: 2026-05-14
---

# Phase 52 Plan 02 — SUMMARY

## 实际落地

### 新增文件（5 份模板 README）

- `tests/e2e/harness/artifacts/logs/README.md`
- `tests/e2e/harness/artifacts/network/README.md`
- `tests/e2e/harness/artifacts/docker/README.md`
- `tests/e2e/harness/artifacts/postgres/README.md`
- `tests/e2e/harness/artifacts/system/README.md`

每份 ≈ 30-60 行中文，结构统一：

1. 「里面是什么」段（含文件清单）
2. 「采集命令」段（shell 行）
3. 「典型排障场景」段（3-5 条具体场景，引用具体 phase 关联用例）
4. 「darwin 上为空怎么办」段
5. 隐私守护 / 与既有 phase 关联（按需）

### 修改文件

- `tests/e2e/harness/collect-artifacts_test.go`：新增 1 个单测 `TestCollectArtifacts_CopiesReadmes`，断言 5 子目录下 README.md 都存在且含 `Phase 52` + `排障` 关键字。

### 未改

- `tests/e2e/harness/collect-artifacts.sh`：Plan 01 已预埋 `copy_readmes()` 函数 + `SCRIPT_VERSION="v1"` + `metadata.txt` 7 字段，本 plan 仅创建模板，函数立即激活。
- `tests/e2e/harness/{dump.go, artifacts.go, suite.go}`、`.github/workflows/e2e.yml`：Plan 03 才动。

## metadata.txt 完整契约（Plan 01 已锁，Plan 02 仅文档化）

每次脚本执行后 `$ROOT/metadata.txt` 含 7 行：

```
timestamp=2026-05-14T14:32:32Z       # date -u +%Y-%m-%dT%H:%M:%SZ
scenario=<scenario-id>               # 命令行第 2 参数，默认 "default"
hostname=<hostname>                  # 兜底 "unknown"
kernel=<uname -srm>                  # 兜底 "unknown"
git_sha=<short SHA>                  # git rev-parse 失败 → "unknown"
runner=<$GITHUB_JOB or "local">      # CI 上是 job name，本地是 "local"
script_version=v1                    # SCRIPT_VERSION 常量
```

未来字段扩展（如 `e2e_test_name`、`runner_arch`）需将 `SCRIPT_VERSION` 升到 `v2` 并同步更新 5 份 README 与 VERIFICATION。

## 5 份 README 各自的「关联 phase」

| 子目录 | 关联 phase | 典型用例 |
|--------|------------|----------|
| `logs/` | Phase 45/46/47 | 容器启动失败、SSH 拒连、host-agent 心跳 |
| `network/` | Phase 49 LEAK-* / Phase 51 QUAL-05 / Phase 48 kill-switch | nft drop 规则命中、tun0 优先级、出口 IP |
| `docker/` | Phase 50 KILL-04 / Phase 49 LEAK-08 / Phase 51 QUAL-06 | 网络断开判定、capability 漂移、挂载点 |
| `postgres/` | Phase 47 MVS-06/08 / Phase 51 D-47-3 | user.expired / 心跳 events / 双绑互斥 |
| `system/` | Phase 45 ArtifactDumper / Phase 47 Health | dmesg / WaitFor 超时备忘 |

## darwin 本地验证

```
$ bash tests/e2e/harness/collect-artifacts.sh ./out smoke
$ ls ./out/smoke/*/README.md
  ./out/smoke/docker/README.md
  ./out/smoke/logs/README.md
  ./out/smoke/network/README.md
  ./out/smoke/postgres/README.md
  ./out/smoke/system/README.md                                ✓
$ head -1 ./out/smoke/logs/README.md
  # logs/ —— 容器日志（Phase 52 OBS-01..03 收集）             ✓
$ go test ./tests/e2e/harness/ -run "Collect" -count=1 -v   → 7/7 PASS ✓
$ grep -l '/Users/\|/home/zaneliu\|@gmail' tests/e2e/harness/artifacts/*/README.md
  （0 命中）                                                  ✓
$ go build ./tests/e2e/...                                  → exit 0 ✓
$ GOOS=linux go build -tags='e2e linux' ./tests/e2e/...     → exit 0 ✓
$ bash scripts/lint-no-bare-sleep.sh                        → [ok] ✓
```

## 与 PLAN 偏差

- PLAN 草案中 5 份 README 各 ≤ 60 行，实际落地是 30-60 行（logs/docker/system 偏短，network/postgres 偏长，因为 network/postgres 关联 phase 更多）。范围内，未漂移。
- PLAN 草案中 metadata.txt 字段「runner」实现侧用 `${GITHUB_JOB:-local}`，未引入 `RUNNER_NAME` / `RUNNER_OS` 字段（GitHub Actions 自带这些环境变量，但 v1 锁 `GITHUB_JOB` 已足够；v2 扩展时可加）。
- PLAN 草案中提到「脚本中加 `SCRIPT_VERSION` 常量」与「`copy_readmes()` 单函数」—— Plan 01 都已预埋实现，Plan 02 无需再改脚本。
- 新增的 `TestCollectArtifacts_CopiesReadmes` 单测命中 darwin 上 README 复制完整流程，连同 Plan 01 的 6 个共 7 个单测全部 PASS。

## 隐私守护

- 5 份 README 内容 grep 检查：
  ```
  $ grep -E '/Users/[a-z]|/home/[a-z]|@gmail\.com|@qq\.com|@outlook\.com' tests/e2e/harness/artifacts/*/README.md
  （0 命中）
  ```
- README 内提到的所有占位凭据 / 邮箱都用 `your-secret-here` / `test@example.com` / `secret-placeholder-pw` 等通用形式。
- Plan 01 单测 `TestCollectArtifacts_NoAbsoluteUserPathsInScript` 守护脚本本体；README 隐私靠人工 + CONVENTIONS.md 守护（CI 未来如需扫描，再统一加 lint）。

## 给 Plan 03 的接口约定

- README 模板路径锁死 `tests/e2e/harness/artifacts/<sub>/README.md`，Plan 03 切换 DumpHook 到调脚本时**会覆盖** Phase 45 ArtifactDumper.Collect 写入的占位 README（这是预期，Plan 02 模板更详尽）。
- Phase 45 单测 `TestArtifactDumper_CollectCreates5Subdirs` 中断言「README 含 'Phase 52' 字样」，Plan 02 模板顶部都有 `Phase 52 OBS-01..03 收集` 字样，兼容。
- Plan 03 改 `runCollectScript` 时**无需**改 Plan 01/02 任何文件，直接 `bash collect-artifacts.sh` 即可拿到完整 5 README + metadata.txt。
