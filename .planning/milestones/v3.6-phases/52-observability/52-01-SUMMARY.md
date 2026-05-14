---
phase: 52-observability
plan: 01
title: collect-artifacts.sh 一键采集脚本 (OBS-01)
status: shipped
requirement: OBS-01
created: 2026-05-14
---

# Phase 52 Plan 01 — SUMMARY

## 实际落地

### 新增文件

- `tests/e2e/harness/collect-artifacts.sh`（bash + POSIX 兼容，chmod +x）：
  - 接口：`bash collect-artifacts.sh <output-dir> [scenario-id]`，缺 output-dir 时 `${1:?...}` 报错退出，其它路径永远 exit 0。
  - 6 个采集函数 + 1 个 README 复制函数（Plan 02 激活）：`collect_metadata` / `collect_logs` / `collect_network` / `collect_docker` / `collect_postgres` / `collect_system` / `copy_readmes`。
  - 缺工具时全部走 `_skipped.txt` / `_empty.txt` 占位 + `|| true` 兜底，子命令失败不影响脚本整体退出码。
  - `SCRIPT_VERSION="v1"` 常量已锁，方便后续升级。
  - 隐私：路径全部走 `$OUT` / `$ROOT` 形参化；`pg_dump --schema-only`；用户表只 SELECT `id/username/status`，不导出 `password_hash` / `entry_password`。
  - 调试：`COLLECT_DEBUG=1 bash collect-artifacts.sh ...` 打开 `set -x`。

- `tests/e2e/harness/collect-artifacts_test.go`（**无 build tag**，darwin 也跑）：
  - 6 个单测：`Creates5SubdirsAndMetadata` / `ExitZeroWithoutDocker` / `ScenarioIdSubpath` / `DefaultScenarioWhenOmitted` / `FailsWithoutOutputDir` / `NoAbsoluteUserPathsInScript`。
  - 用 `runtime.Caller(0)` 定位脚本路径，不依赖 CWD。
  - 显式清空 `DATABASE_URL` / `PG_DUMP_URL` 环境变量，避免本机有真实 PG 时拖慢测试。
  - 隐私守护单测 `NoAbsoluteUserPathsInScript` 静态扫脚本源码，确保不含 `/Users/zaneliu`、`/home/zaneliu`、`@gmail.com` 等 5 类敏感字串。

### 不动 / 未引入

- `tests/e2e/harness/{dump.go, artifacts.go, suite.go, scenario.go}` 全部零修改（Plan 02/03 才动）。
- `.github/workflows/e2e.yml` 不动（Plan 03 才改）。
- `go.mod` 不动。

## darwin 本地验证

```
$ chmod +x tests/e2e/harness/collect-artifacts.sh
$ bash -n tests/e2e/harness/collect-artifacts.sh            → exit 0 ✓
$ bash tests/e2e/harness/collect-artifacts.sh ./out smoke   → exit 0 ✓
  ./out/smoke/{logs,network,docker,postgres,system} 5 子目录创建 ✓
  ./out/smoke/metadata.txt 含 timestamp/scenario/hostname/kernel/git_sha/runner/script_version ✓
  ./out/smoke/logs/_empty.txt（docker daemon 未起）
  ./out/smoke/network/nft-ruleset.txt = "nft not available（darwin / 非 root linux）"
  ./out/smoke/network/listen-tcp.txt  ≈ 130 KB（netstat -tln 输出）
  ./out/smoke/postgres/_skipped.txt = "PG_DUMP_URL / DATABASE_URL 未设置..."
  ./out/smoke/system/{uname,df,free,dmesg-tail}.txt 都有内容 ✓
$ go test ./tests/e2e/harness/ -run "Collect" -count=1 -v   → 6/6 PASS ✓
$ go build ./tests/e2e/...                                  → exit 0 ✓
$ GOOS=linux go build -tags='e2e linux' ./tests/e2e/...     → exit 0 ✓
$ bash scripts/lint-no-bare-sleep.sh                        → [ok] ✓
```

darwin 上 5 子目录采集到的内容：

| 子目录 | darwin 上实际产物 | 备注 |
|--------|------|------|
| `logs/` | `_empty.txt` 占位 | docker daemon 未起 |
| `network/` | `nft-ruleset.txt` 占位 + `ip-link.txt` 占位 + `listen-tcp.txt` 真实 netstat 输出 | nft / ip 不存在 |
| `docker/` | `ps.txt` / `network-ls.txt`（含 `Cannot connect to the Docker daemon` 错误） | docker CLI 在 daemon 没起 |
| `postgres/` | `_skipped.txt` 占位 | 无 DATABASE_URL |
| `system/` | `uname.txt` / `df.txt` / `free.txt`（vm_stat 输出）/ `dmesg-tail.txt`（"dmesg failed" 占位） | macOS 上 vm_stat 替换 free，dmesg 通常拒读 |

## 与 PLAN 偏差

- PLAN §Steps Step 1 草案中 `collect_logs` 用 `docker ps -a --format '{{.Names}}' | while read -r name`，实现侧改成 `names=$(docker ps -a ...); while ... <<< "$names"`，避免 `pipe to while` 在 bash 4 下创建子 shell 导致 `$ROOT` 等变量传递不畅；同时也加了 `docker ps -a` 输出为空的快速跳过分支（写 `_empty.txt`）。
- PLAN §Steps Step 1 草案的 `collect_postgres` 增加了 `psql` 是否可用的二次检查（缺 psql 时只写 schema.sql 不跑 SELECT），避免 ubuntu-24.04 上 `pg_dump` 存在但 `psql` 缺失的边缘环境。
- PLAN 未提：darwin 上 `df -h` 与 `free` 输出格式与 linux 不同，但保留这些命令并接受输出差异（CI runner 才是真实采集场景，darwin 上只验证脚本 exit 0 + 文件存在）。
- PLAN §Steps Step 1 中提到 `copy_readmes()` 在 Plan 02 落地后激活；本 plan 已**预留** `copy_readmes()` 函数（指向 `$SCRIPT_DIR/artifacts/$sub/README.md` 模板路径），但模板还未存在 → `cp` 走 `|| true` 静默跳过，单测断言不会失败。Plan 02 创建模板后立即激活，不需要再改脚本。

## 隐私守护

- 脚本本身（122 行）grep 检查：
  ```
  $ grep -E '/Users/[a-z]|/home/[a-z]|@gmail\.com|@qq\.com|@outlook\.com' tests/e2e/harness/collect-artifacts.sh
  （0 命中）
  ```
- 单测 `TestCollectArtifacts_NoAbsoluteUserPathsInScript` 自动守护，回归即 fail。
- 注意：脚本**运行时**采集到的 `metadata.txt` 会写本机 hostname（如 `Wanda.local`），这是采集功能的本职，不在「禁绝对路径」约束内。CI runner 上 hostname 通常是 `fv-azXXX-XXX` 之类 GitHub Actions 临时名。

## 给 Plan 02 / Plan 03 的接口约定

- **`copy_readmes()`** 已预埋：Plan 02 创建 `tests/e2e/harness/artifacts/<sub>/README.md` 5 份模板后立即生效，**无需**再改脚本。
- **`SCRIPT_VERSION` 常量** 已锁 `"v1"`；Plan 02 扩展 metadata.txt 字段时不需要改这个；Plan 03 / 后续 phase 若需打破 metadata.txt schema，升到 `"v2"` 即可。
- **Go 单测路径定位**：`runtime.Caller(0)` 锁脚本与 `_test.go` 同目录。Plan 03 `ArtifactDumper.runCollectScript` 也按相同方式定位脚本，无需引入路径配置。
- **环境变量约定**：
  - `PG_DUMP_URL` > `DATABASE_URL` > 跳过（Plan 03 CI workflow 透传 e2e testcontainer URL 时可选用 `PG_DUMP_URL`，与 `Run e2e suite` 步骤的真实 DB URL 解耦）。
  - `COLLECT_DEBUG=1` 打开 bash 调试。
  - `GITHUB_JOB` CI runner 自动设置，用作 metadata.txt `runner=` 字段。

## Linux 真机验证项（deferred-to-Plan 03 CI）

- ubuntu-24.04 hosted runner 上故意失败一个 e2e 用例 → Plan 03 e2e.yml 触发 `bash collect-artifacts.sh` → 检查：
  - `network/nft-ruleset.txt` 非空（hosted runner 有 nft）
  - `docker/ps.txt` 含真实容器列表
  - `system/dmesg-tail.txt` 含真实 100 行内核日志
  - `postgres/_skipped.txt` 或 `schema.sql`（取决于 e2e job 是否透传 DATABASE_URL）

签字：Plan 03 落地后在 VERIFICATION `human_verification` 中记录。
