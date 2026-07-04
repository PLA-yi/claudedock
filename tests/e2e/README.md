# ClaudeDock — e2e 测试套件

本目录承载 ClaudeDock v3.6 的端到端测试体系（Phase 45-52）。

## 跑测试

**本地（darwin / linux）**：

```bash
go test -tags=e2e ./tests/e2e/... -count=1 -v -timeout=15m
```

darwin 上多数真实容器用例会通过 build tag (`//go:build e2e && linux`) 跳过；harness 单测全部都跑。

**CI**：`.github/workflows/e2e.yml` 自动触发（PR paths 过滤 + push to main）。
双 job：
- `lint` 永远跑（bash 语法 + lint-no-bare-sleep + go vet）。
- `e2e` 在 lint 通过后跑（hosted ubuntu-24.04 runner，timeout 15min）。

## 写测试

典型骨架见 `.planning/phases/45-ci/45-VERIFICATION.md` §「用例侧典型骨架」。
核心 API 入口：

- `harness.BaseSuite` — testify/suite 基类，含 Ctx / Logger / ProjectRoot。
- `harness.New(t).WithControlPlane().WithSingBoxGateway(...).WithHost(...).WithUser(...)` — Scenario builder。
- `harness.WaitFor[Log|Port|HTTP|Exec]` — 替代裸 `time.Sleep`，禁用直接 sleep。
- `harness.NewArtifactDumper(scenario, "")` — 失败 artifact 归档（Phase 45 落地，Phase 52 OBS-01..03 接入完整采集脚本）。

## 排障 / Artifacts (Phase 52 OBS-01..03)

**自动**：CI e2e job 失败时自动跑 `bash tests/e2e/harness/collect-artifacts.sh`，
归档结果通过 `actions/upload-artifact@v4` 在 PR 评论中给出下载链接（同仓 PR，
fork PR 因 GITHUB_TOKEN 权限降级会 403，仅 artifact 上传仍有）。

**手动**：本地复现失败时可手动跑：

```bash
bash tests/e2e/harness/collect-artifacts.sh ./out scenario_xyz
ls -la ./out/scenario_xyz/
```

### 输出树（5 子目录 + metadata.txt）

| 子目录 | 内容 | 典型用途 |
|--------|------|----------|
| `logs/` | `docker logs --tail 500 --timestamps` 每容器 1 份 | 看容器进程错误 |
| `network/` | `nft list ruleset` / `ip route` / `ip netns list` / `ss -tln` | 看防火墙 / 路由 / 监听端口 |
| `docker/` | `docker ps -a` / `inspect-<name>.json` | 看容器生命周期 / 网络挂载 |
| `postgres/` | `pg_dump --schema-only` + 3 个 key 表 SELECT | 看 DB 状态（不含敏感列） |
| `system/` | `uname` / `free` / `df` / `dmesg tail` / `wait-timeout.txt` | 看宿主机系统状态 / WaitFor 超时备忘 |

每个子目录内都有自己的 `README.md`（模板在 `harness/artifacts/<sub>/README.md`），含具体排障指引。

### metadata.txt 字段（7 个）

```
timestamp=<RFC3339 UTC>
scenario=<scenario-id>
hostname=<host>
kernel=<uname -srm>
git_sha=<short SHA>
runner=<$GITHUB_JOB or "local">
script_version=v1
```

### 在 CI artifact zip 内会看到的目录结构

```
out/e2e-artifacts/
├── TestKillSwitch_SingboxCrash_GoldenPath/          # 单用例级（ArtifactDumper.Collect 写）
│   └── 20260514T144023Z/
│       ├── metadata.txt
│       ├── logs/
│       ├── network/
│       ├── docker/
│       ├── postgres/
│       └── system/
└── ci-e2e-1/                                         # CI runner 全局快照（脚本 if: failure() 写）
    ├── metadata.txt
    ├── logs/
    └── ...
```

### darwin 上跑

darwin 本地缺 `nft` / `ip` / `pg_dump` / `dmesg`，5 子目录大半会写 `_skipped.txt` / 错误占位，这是预期。真实采集结果请在 Linux runner 上跑或本地起 Docker Desktop + testcontainer。

### 调试模式

```bash
COLLECT_DEBUG=1 bash tests/e2e/harness/collect-artifacts.sh ./out scenario_xyz
```

打开 `set -x`，逐行打印脚本执行轨迹。

## 子目录约定

- `harness/` — 共享基础设施（BaseSuite / Scenario / WaitFor / ArtifactDumper / collect-artifacts.sh + 5 README 模板）
- `leak/` — Phase 49 防泄漏对抗用例
- `killswitch_stress/` — Phase 50 kill-switch 压力测试
- `fixtures/` / `testdata/` — JSON / SQL 等静态资源

## 守护

- `bash scripts/lint-no-bare-sleep.sh` — 禁止裸 `time.Sleep`（所有等待走 `harness.WaitFor`）
- `go vet -tags=e2e ./tests/e2e/...` — 静态检查
- `go test -tags=e2e ./tests/e2e/harness/ -count=1` — harness 单测（含 Phase 45 ArtifactDumper 6 个 + Phase 52 OBS-01..03 8 个）
- `go test ./tests/e2e/harness/ -run Collect -count=1` — Phase 52 collect-artifacts.sh 单测（**无 build tag**，darwin 也跑）

## 隐私守护

- 任何 `tests/e2e/` 内的文件、脚本、注释、文档**禁止**写绝对路径（`/Users/...`、`/home/<user>/...`）。
- 任何 fixture / 测试数据**禁止**用真实凭据；统一用 `secret-placeholder-pw` / `test@example.com` / `your-secret-here` 等占位。
- `collect-artifacts.sh` 的 `postgres/` 采集**只**导出非敏感列（`id / username / status / type / created_at`），永远不写 `password_hash` / `entry_password` / `admin_token`。
- 详细规则见仓库根 `CONVENTIONS.md` §Privacy & Security。
