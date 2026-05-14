---
phase: 52-observability
plan: 03
title: DumpHook 切换到脚本 + CI workflow upload-artifact 完整化 (OBS-03)
status: shipped
requirement: OBS-03
created: 2026-05-14
---

# Phase 52 Plan 03 — SUMMARY

## 实际落地

### 修改文件

- `tests/e2e/harness/artifacts.go`：
  - `Collect()` 签名从 `Collect(_ context.Context, ...)` 改为 `Collect(ctx context.Context, ...)`（公开签名不变，仅参数名启用）。
  - mkdir 5 子目录之前先调 `runCollectScript(ctx, baseDir/sanitized, timestamp)` 子进程；失败 `logger.Warn` 吞掉（best-effort）。
  - 新增私有方法 `(*ArtifactDumper).runCollectScript(ctx, outDir, scenarioID)`：`runtime.Caller(0)` 定位脚本，`context.WithTimeout(ctx, 30s)` 兜底超时，`cmd.Stdout/Stderr = os.Stderr` 把脚本输出导到测试 stderr 便于 CI log 检索。
  - 新增 imports：`os/exec`、`runtime`。

- `tests/e2e/harness/dump.go`：尾部追加 OBS-03 注释挂点（公开接口字符级零漂移）。

- `tests/e2e/harness/collect-artifacts.sh`：`copy_readmes()` 改成 `[[ -f $template && ! -f $target ]] && cp`（no-clobber），保证 Phase 45 `TestArtifactDumper_CollectIsIdempotent` 的 mtime 检查仍通过。

- `tests/e2e/harness/artifacts_test.go`：新增 `TestArtifactDumper_CollectInvokesScript` —— 断言 Collect 调脚本后产生 `metadata.txt`（含 `script_version=v1`），且 5 子目录 README 都被 Plan 02 模板覆盖（含「典型排障场景」关键字）。

- `.github/workflows/e2e.yml`：
  - `e2e` job 失败分支新增 `Collect e2e artifacts on failure (script)` 步骤，跑 `bash collect-artifacts.sh ./out/e2e-artifacts ci-<job>-<attempt>` 收 CI runner 全局快照。
  - Upload artifact name 加 `-${{ github.run_attempt }}` 后缀（避免重跑覆盖）。
  - PR 评论 body 升级为 Phase 52 完整版（5 子目录详尽内容 + 两类目录区分 + 排障 README 指引）。

### 新增文件

- `tests/e2e/README.md`：e2e 套件总入口文档，含跑测 / 写测 / **排障** / 子目录约定 / 守护 / 隐私守护 6 节，约 110 行。

### 未改

- `tests/e2e/harness/{suite.go, scenario.go, waitfor.go}` 公开 API 零漂移。
- Phase 47-50 既有 e2e 用例 `_test.go` 文件零修改（grep 验证）。
- `go.mod` 主体依赖未动。

## darwin 本地验证

```
$ go build ./tests/e2e/...                              → exit 0 ✓
$ GOOS=linux go build -tags='e2e linux' ./tests/e2e/... → exit 0 ✓
$ go vet -tags=e2e ./tests/e2e/...                      → exit 0 ✓
$ bash scripts/lint-no-bare-sleep.sh                    → [ok] ✓
$ go test ./tests/e2e/harness/ -count=1                  → 7/7 PASS（无 build tag）✓
$ go test -tags=e2e ./tests/e2e/harness/ -count=1        → 23/23 PASS ✓
  含 Phase 45 ArtifactDumper 5 + BaseSuite 1 + WaitFor 8 + Phase 52 OBS-01..02 6 + Phase 52 OBS-03 1 = 21
  + Phase 45 已有 6 个 Artifact 测试中 1 个 IsIdempotent + 1 个 5Subdirs + 1 个 OnWaitForTimeoutWritesNoteFile 总计落地 23 个
$ go test -tags=e2e ./tests/e2e/... -count=1 -timeout=60s → all PASS（含 Helpers 单测全绿）✓
```

关键单测验证：

- `TestArtifactDumper_CollectIsIdempotent` 仍 PASS（`copy_readmes` no-clobber 修正生效）。
- `TestArtifactDumper_CollectCreates5Subdirs` 仍 PASS（Phase 45 占位 README 在脚本调用前未写入，因为我们改了顺序：脚本先 cp 详尽 README；如脚本 cp 不到（template 缺失），Go 兜底写占位 README；脚本 cp 成功 → Go for 循环跳过 → 测试断言「Phase 52」字样仍命中，因为 Plan 02 模板顶部含 `Phase 52 OBS-01..03`）。
- `TestArtifactDumper_CollectInvokesScript`（新）PASS：metadata.txt 含 `script_version=v1`，5 子目录 README 含「典型排障场景」（Plan 02 模板独有字符）。

### Phase 47-50 用例零破坏验证

- `grep -rn "wait-timeout\|CollectArtifacts\|ArtifactDumper" tests/e2e/ --include='*.go'` 显示只在 `harness/` 包内有引用，Phase 47-50 用例**未**直接调 `ArtifactDumper.Collect`；它们通过 `BaseSuite.TearDownTest` 与 `harness.WaitFor + WithDumpHook` 间接触发，公开签名零漂移。
- `go test -tags=e2e ./tests/e2e/ -run "TestHelpers" -count=1` PASS（Phase 46/48/49/50 共享纯函数 helpers 全部回归绿）。

## 与 PLAN 偏差

- PLAN §Steps Step 1 草案中要求「先 Go 写占位 README，再调脚本」；实现侧颠倒了顺序（先调脚本，后 Go 兜底），原因：让 Plan 02 详尽 README 直接落到目标位置，避免脚本 cp 与 Go 占位冲突。同时脚本 `copy_readmes` 加 no-clobber，所以即使 Go 先写也安全；但调换顺序更符合「Plan 02 模板优先」语义。
- PLAN §Steps Step 4 草案中提到把 `DATABASE_URL` 透传给 collect 步骤；实现侧**未**在 e2e.yml 步骤 env 中显式透传 —— `Run e2e suite` 内部 testcontainer 起的 PG URL 是动态的，hosted runner 上没有简单办法把它 export 到下个 step。Plan 01 `collect_postgres` 检测到 URL 未设时优雅跳过（写 `_skipped.txt` 占位），符合 deferred-to-后续 milestone 决策。
- PLAN §Steps Step 5 草案中 `tests/e2e/README.md` 写得偏简（≤ 50 行），实际落地约 110 行 —— 因为这是 v3.6 最后一份用户面文档，把跑测 / 写测 / 排障 / 子目录 / 守护 / 隐私守护 6 节都写实在了，给后续 milestone 开发者一份完整 onboarding。

## 关键决策（CONTEXT「Claude's Discretion」节落地）

- **脚本调用顺序**：`mkdir root → 调脚本（mkdir 5 子目录 + cp Plan 02 README + 真实采集）→ Go for 循环兜底（README 已存在则跳过）`。这个顺序保证 Plan 02 模板优先且与 Phase 45 idempotent 测试兼容。
- **30s 超时**：`context.WithTimeout(ctx, 30*time.Second)` 派生新 ctx，外层 cancel 时脚本快速失败 + Warn 不阻塞 testing 退出。CONTEXT §Specifics 锁定 ≤30s。
- **CI artifact 命名**：`e2e-artifacts-${{ github.run_id }}-${{ github.run_attempt }}` 加 attempt 后缀，避免重跑覆盖。
- **PR 评论**：升级为 Phase 52 完整版本，明确告知排障 README 在每子目录都有。
- **fork PR 评论 403** 已知问题：保持 Phase 45 现状，未引入 `pull_request_target`（后续 milestone 处理）。

## 给 Phase 52 audit / milestone cleanup 的接口约定

- `ArtifactDumper.Collect / OnWaitForTimeout` 公开签名锁死，Phase 47-50 既有用例零破坏。
- `collect-artifacts.sh` v1 接口（`<output-dir> [scenario-id]`）锁死；扩展字段升 `SCRIPT_VERSION="v2"` 即可。
- 5 份 README 模板路径锁死 `tests/e2e/harness/artifacts/<sub>/README.md`，模板**仅模板**（不在运行时输出树内），与 Phase 52 输出根 `./out/e2e-artifacts/` 不冲突。
- e2e.yml 双 if: failure() 流程（脚本采集 → upload → PR 评论）稳定，后续 CI tweak 在该模式下追加即可。

## Linux 真机验证项（`human_verification`）

deferred-to-CI：

- ubuntu-24.04 hosted runner 上故意 fail 一个 e2e 用例，验证：
  1. `if: failure()` 跑 `collect-artifacts.sh` 成功 exit 0。
  2. artifact zip 内 `ci-e2e-<run_attempt>/{logs,network,docker,postgres,system}/` 5 子目录都有真实输出：
     - `network/nft-ruleset.txt` 非空（hosted runner 有 nft）
     - `docker/ps.txt` 含真实容器列表
     - `system/dmesg-tail.txt` 含真实内核日志
  3. 同时 `<TestName>/<timestamp>/` 单用例目录也归档。
  4. PR 评论自动出现（同仓 PR）。

签字位置：`.planning/phases/52-observability/52-VERIFICATION.md` §human_verification。
