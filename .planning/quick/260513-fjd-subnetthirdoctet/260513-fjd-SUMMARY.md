---
phase: 260513-fjd-subnetthirdoctet
plan: 01
subsystem: internal/network
tags: [test, bugfix, ci-stability]
requires: []
provides:
  - 修复后的 TestSubnetThirdOctet_CollisionResistance（阈值 40，注释准确）
affects:
  - internal/network/container_proxy_provider_test.go
tech_stack:
  added: []
  patterns:
    - 测试断言阈值需与统计学期望（生日悖论）对齐
key_files:
  created: []
  modified:
    - internal/network/container_proxy_provider_test.go
decisions:
  - 阈值 40 留出约 18 次余量，覆盖生日悖论方差（E≈25）
  - 仅修改测试断言与注释，不改动 subnetThirdOctet 实现
metrics:
  duration_minutes: 2
  completed_date: 2026-05-13
  tasks_completed: 1
  files_modified: 1
commits:
  - hash: 0def841
    message: "test(260513-fjd-01): 调整 SubnetThirdOctet 碰撞抗性阈值至 40"
---

# Quick Task 260513-fjd: SubnetThirdOctet 碰撞抗性测试阈值修复 Summary

## 一句话总结

把 `TestSubnetThirdOctet_CollisionResistance` 的失败阈值从 `> 10` 调整为 `> 40`，并把注释改为基于生日悖论的准确描述，消除 CI 上的确定性误报。

## 背景与问题

`internal/network/container_proxy_provider_test.go` 中的 `TestSubnetThirdOctet_CollisionResistance` 使用 FNV-1a 把 100 个**确定性**输入哈希到 200 个桶（`% 200 + 20`，对应 IP 第三段 20-219）。原断言为 `if collisions > 10`，并附注释 "expect very few collisions / Allow up to 10 as a generous threshold"。

问题在于：
- **统计学层面**：根据生日悖论，把 k 个样本均匀投到 n 个桶，期望碰撞数为 `E = k(k-1)/(2n) = 100 * 99 / (2 * 200) ≈ 24.75`。即使是理想均匀分布，期望就已远超 10。
- **实测层面**：FNV-1a 对硬编码的 100 个测试输入产生**固定** 22 次碰撞（输入确定 → 输出确定）。
- **结果**：阈值 10 在 CI 上**每次必失败**，而本地若未启用 `//go:build linux`（macOS）则不会编译该文件，导致问题被掩盖直到 CI 跑起来。

## 修改对比

### 修改前（`internal/network/container_proxy_provider_test.go` 第 106-108 行）

```go
// 100 samples into 200 possible values: expect very few collisions
// Allow up to 10 collisions as a generous threshold
if collisions > 10 {
```

### 修改后

```go
// 100 samples into 200 buckets: birthday paradox expects E[collisions] = 100*99/(2*200) ≈ 25
// Allow up to 40 to accommodate birthday paradox variance (deterministic FNV-1a inputs produce 22)
if collisions > 40 {
```

## 数学依据

生日悖论的碰撞数期望公式：

```
E[collisions] = k(k - 1) / (2n)
              = 100 * 99 / (2 * 200)
              = 9900 / 400
              ≈ 24.75
```

其中：
- `k = 100`：样本数（测试用例数）
- `n = 200`：桶数（`subnetThirdOctet` 返回 `% 200 + 20`，即 20-219 共 200 个值）

| 指标 | 数值 |
|------|------|
| 样本数 k | 100 |
| 桶数 n | 200 |
| 期望碰撞数 E | ≈ 25 |
| FNV-1a 实测碰撞数（确定性） | 22 |
| 新阈值 | 40 |
| 安全余量（40 − 22） | 18 |
| 余量相对期望比例 | 18 / 25 ≈ 72% |

40 这个阈值在期望值 25 之上留出约 72% 的余量，足以吸收即便未来微调输入构造方式后产生的方差波动，同时仍能在哈希函数严重退化（碰撞翻倍）时及时报警。

## 实现保持不变的部分

按计划要求，**未触碰** `subnetThirdOctet` 的实现。`internal/network/container_proxy_provider.go` 第 203-207 行保留：

```go
func subnetThirdOctet(hostID string) int {
    h := fnv.New32a()
    _, _ = h.Write([]byte(hostID))
    return int(h.Sum32()%200) + 20
}
```

理由：`% 200 + 20` 的区间约束属于业务契约（避开保留段），属于稳定接口；本次问题完全在测试断言侧。

## 验证

| 检查项 | 命令 | 结果 |
|--------|------|------|
| 新阈值生效 | `grep "collisions > " ...test.go` | 仅一行：`if collisions > 40 {` |
| 新注释生效 | `grep "birthday paradox" ...test.go` | 命中第 106 行 |
| 旧文本清除 | `grep "collisions > 10\|very few collisions" ...test.go` | 无匹配 |
| 编译通过 | `GOOS=linux go test ./internal/network/ -c -o /dev/null` | exit 0 |
| 实现未改动 | `grep -A4 subnetThirdOctet container_proxy_provider.go` | 保持 `% 200 + 20` |

CI 上完整执行 `go test ./internal/network/ -run TestSubnetThirdOctet -v` 时，22 次碰撞远低于阈值 40，测试稳定通过。

## 偏差

无。计划按字面意思执行，仅修改约定的 3 行文本，未触发任何 Rule 1-4 偏差。

## 提交

- `0def841` — test(260513-fjd-01): 调整 SubnetThirdOctet 碰撞抗性阈值至 40

## Self-Check: PASSED

- FOUND: internal/network/container_proxy_provider_test.go（包含 `collisions > 40`、`birthday paradox`，已移除 `collisions > 10`、`very few collisions`）
- FOUND: 0def841（`git log --oneline` 命中）
- FOUND: internal/network/container_proxy_provider.go（`subnetThirdOctet` 实现保持 `% 200 + 20`）
- COMPILED: `GOOS=linux go test ./internal/network/ -c -o /dev/null` exit 0
