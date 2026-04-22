# Phase 35 Benchmarks 产物目录

> 适用版本：v3.0 起；对应阶段 Phase 35-e2e
> 关联需求：BASE-01（rg/ls -R ≤ 本地 1.5×）、BASE-02（首连 ≤ 8s）

本目录用于存放 `scripts/perf-benchmark.sh` 与 `scripts/cold-start-benchmark.sh` 产出的基准测试报告。
所有报告均以 JSON + Markdown 双格式落盘，便于：
1. **机器消费**：CI 用 `jq` 抽取关键比值做回归断言
2. **人工审阅**：Markdown 表格在 PR review 与运维手册中可直接引用
3. **历史对比**：跨 release 把 P50/P99 摆在一起趋势化

---

## 1. 文件命名约定

| 用途 | 模板 | 产生脚本 |
|------|------|----------|
| BASE-01 性能基准 JSON | `bench-YYYYMMDD-HHMMSS.json` | `scripts/perf-benchmark.sh` |
| BASE-01 性能基准 Markdown | `bench-YYYYMMDD-HHMMSS.md` | `scripts/perf-benchmark.sh` |
| BASE-02 首连基准 JSON | `cold-start-YYYYMMDD-HHMMSS.json` | `scripts/cold-start-benchmark.sh` |
| BASE-02 首连基准 Markdown | `cold-start-YYYYMMDD-HHMMSS.md` | `scripts/cold-start-benchmark.sh` |

时间戳使用 UTC 本地化的 `date +%Y%m%d-%H%M%S`，命名前缀固定，便于 `ls bench-*.json` 通配。

---

## 2. JSON Schema

### 2.1 BASE-01 (`bench-*.json`)

直接复用 hyperfine `--export-json` 输出，再追加项目自定义元数据：

```json
{
  "schema_version": 1,
  "kind": "perf-benchmark",
  "timestamp": "2026-04-22T12:34:56Z",
  "host": { "hostname": "...", "uname": "...", "cpu_count": 8 },
  "results": [
    {
      "command": "local-rg",
      "mean": 0.0234, "stddev": 0.0012, "median": 0.0231,
      "user": 0.0210, "system": 0.0020,
      "min": 0.0215, "max": 0.0267,
      "times": [0.0215, 0.0221, 0.0229]
    }
  ],
  "ratios": {
    "mergerfs_rg_p50_over_local": 1.32,
    "mergerfs_ls_p50_over_local": 1.41,
    "mergerfs_rg_p99_over_local": 1.78
  }
}
```

P50 / P99 由 `jq` 从 `.results[].times[]` 排序后按索引抽取（详见 `scripts/perf-benchmark.sh` 的 jq 表达式）。

### 2.2 BASE-02 (`cold-start-*.json`)

```json
{
  "schema_version": 1,
  "kind": "cold-start-benchmark",
  "timestamp": "2026-04-22T12:34:56Z",
  "config": { "attempts": 5, "threshold_ms": 8000, "min_pass": 4 },
  "attempts": [
    { "idx": 1, "duration_ms": 7812, "stderr_progress_matches": true, "outcome": "pass" }
  ],
  "summary": {
    "total": 5, "pass": 4, "fail": 1,
    "threshold_ms": 8000, "progress_matches_all": true
  }
}
```

`stderr_progress_matches` 表示该次 attempt 的 stderr 同时命中三段式中文进度（REQ-F1-B 锁定）。

---

## 3. 历史对比示例

```bash
# 提取所有历史 BASE-01 mean，按时间排序
jq -r '.results[] | select(.command=="local-rg") | .mean' \
  $(ls -t .planning/phases/35-e2e/benchmarks/bench-*.json) \
  | paste -s -d, -

# 比较最近两次 mergerfs P50 ratio
jq -r '.ratios.mergerfs_rg_p50_over_local' \
  $(ls -t .planning/phases/35-e2e/benchmarks/bench-*.json | head -2)

# BASE-02 历次首连 pass 率
for f in .planning/phases/35-e2e/benchmarks/cold-start-*.json; do
  jq -r '"\(.timestamp): \(.summary.pass)/\(.summary.total)"' "$f"
done
```

---

## 4. 真机执行签字

所有真机（macOS APFS / Ubuntu 25.04 真机）跑出来的报告，**Markdown 头部需要追加一段执行机器签字**，例如：

```markdown
> 执行机器: M3 MacBook Pro / macOS 15.4 / 8C8G / @zaneliu / 2026-04-22 14:00 CST
> 验收 PR: https://github.com/zanel1u/cloud-cli-proxy/pull/XXX
```

CI 跑出来的报告则以 GitHub Actions 的 `${{ github.run_id }}` 与 SHA 替代。

---

## 5. 参考

- `scripts/gen-bench-tree.sh` — synthetic 10k 文件树生成器
- `scripts/perf-benchmark.sh` — BASE-01 三档基准（local / mergerfs / sshfs-only）
- `scripts/cold-start-benchmark.sh` — BASE-02 首连 ≤ 8s + 三段式进度断言
- `.planning/phases/35-e2e/35-01-perf-benchmarks-PLAN.md` — Plan 01 设计原文
- `.planning/phases/35-e2e/35-CONTEXT.md` — 用户决策（10k / 80-15-5 / 1.5×）
