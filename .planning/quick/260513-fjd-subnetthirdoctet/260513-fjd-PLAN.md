---
phase: 260513-fjd-subnetthirdoctet
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/network/container_proxy_provider_test.go
autonomous: true
requirements:
  - QUICK-260513-fjd
must_haves:
  truths:
    - "TestSubnetThirdOctet_CollisionResistance 在确定性输入下稳定通过（实际碰撞数 22 ≤ 阈值 40）。"
    - "测试注释准确描述生日悖论的期望碰撞数（约 25），不再使用误导性的 'very few collisions' 表述。"
    - "subnetThirdOctet 实现保持不变，仅修改测试断言阈值与说明。"
  artifacts:
    - path: "internal/network/container_proxy_provider_test.go"
      provides: "调整后的碰撞抗性测试（阈值 40 + 准确注释）"
      contains: "Allow up to 40"
  key_links:
    - from: "internal/network/container_proxy_provider_test.go:108"
      to: "internal/network/container_proxy_provider.go:203 subnetThirdOctet"
      via: "调用 subnetThirdOctet 计算 100 个确定性输入的碰撞分布"
      pattern: "subnetThirdOctet\\("
---

<objective>
修复 `TestSubnetThirdOctet_CollisionResistance` 在 CI 上必失败的问题。

当前测试阈值为 `collisions > 10`，但 100 样本投到 200 桶里根据生日悖论期望碰撞数 E ≈ 24.75，FNV-1a 对硬编码的 100 个确定性输入实际产生 22 次碰撞（每次执行都一样）。由于输入是确定性的，CI 每次都会失败。

Purpose: 让阈值与生日悖论数学期望对齐，避免 CI 误报；同时修正注释中误导性的 "very few collisions" 描述。
Output: 修改后的 `internal/network/container_proxy_provider_test.go`，测试可稳定通过。
</objective>

<execution_context>
@.claude/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@CLAUDE.md
@.planning/STATE.md
@internal/network/container_proxy_provider_test.go
@internal/network/container_proxy_provider.go

<interfaces>
<!-- 关键函数签名（无需探索代码库） -->

From internal/network/container_proxy_provider.go (第 203-207 行)：
```go
func subnetThirdOctet(hostID string) int {
    h := fnv.New32a()
    _, _ = h.Write([]byte(hostID))
    return int(h.Sum32()%200) + 20
}
```

测试输入构造（test 文件第 91 行）：
```go
hostID := "host-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
```
该输入完全确定性，所以碰撞数（22）在每次运行中固定。

生日悖论期望：E[collisions] = k(k-1)/(2n) = 100*99/(2*200) ≈ 24.75。
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: 调整 TestSubnetThirdOctet_CollisionResistance 阈值与注释</name>
  <files>internal/network/container_proxy_provider_test.go</files>
  <action>
仅修改 `internal/network/container_proxy_provider_test.go` 中第 106-108 行的 3 处文本，其它内容（包括 subnetThirdOctet 实现、其它测试用例、build tag）保持不变。

修改前（第 106-108 行）：
```go
	// 100 samples into 200 possible values: expect very few collisions
	// Allow up to 10 collisions as a generous threshold
	if collisions > 10 {
```

修改后：
```go
	// 100 samples into 200 buckets: birthday paradox expects E[collisions] = 100*99/(2*200) ≈ 25
	// Allow up to 40 to accommodate birthday paradox variance (deterministic FNV-1a inputs produce 22)
	if collisions > 40 {
```

要求：
- 使用 Edit 工具，oldString 必须包含 3 行原文以确保唯一匹配。
- 不修改 `subnetThirdOctet` 实现（保留 `% 200 + 20` 区间）。
- 不修改其它测试函数。
- 不动文件顶部的 `//go:build linux` build tag。
  </action>
  <verify>
    <automated>cd /Users/zaneliu/Projects/open-source/cloud-cli-proxy-main && grep -n "collisions > 40" internal/network/container_proxy_provider_test.go && grep -n "birthday paradox" internal/network/container_proxy_provider_test.go && ! grep -n "collisions > 10" internal/network/container_proxy_provider_test.go && ! grep -n "very few collisions" internal/network/container_proxy_provider_test.go</automated>
  </verify>
  <done>
- 测试文件中阈值已改为 `> 40`。
- 注释已替换为生日悖论的准确描述（包含 "birthday paradox" 关键词）。
- 旧的 `> 10` 和 `very few collisions` 文本已完全移除。
- 文件其余内容（包括 subnetThirdOctet 实现）未变更。
  </done>
</task>

</tasks>

<verification>
最终检查（Linux 环境下可执行；本地 darwin 由于 `//go:build linux` 不会编译该文件，跳过执行测试由 CI 兜底）：

```bash
# 1. 语法/编译检查（即使 darwin 也能通过 build tag 检测）
cd /Users/zaneliu/Projects/open-source/cloud-cli-proxy-main && go vet ./internal/network/... 2>&1 || true

# 2. 关键修改点确认
grep -n "collisions > 40\|birthday paradox" internal/network/container_proxy_provider_test.go

# 3. 确认旧文本已清除
! grep -n "collisions > 10\|very few collisions" internal/network/container_proxy_provider_test.go

# 4. 确认 subnetThirdOctet 未被改动
grep -A 4 "^func subnetThirdOctet" internal/network/container_proxy_provider.go
```

预期：grep 命中新文本、未命中旧文本；`subnetThirdOctet` 实现保持 `% 200 + 20`。
</verification>

<success_criteria>
- `internal/network/container_proxy_provider_test.go` 中：
  - 阈值由 `collisions > 10` 改为 `collisions > 40`。
  - 注释由 "100 samples into 200 possible values: expect very few collisions / Allow up to 10 collisions as a generous threshold" 改为基于生日悖论的准确描述（含 "birthday paradox" 与 "≈ 25" 期望值说明，以及 "Allow up to 40" 余量说明）。
- `subnetThirdOctet` 实现（`container_proxy_provider.go` 第 203 行）保持不变。
- 在 Linux CI 环境下 `go test ./internal/network/ -run TestSubnetThirdOctet_CollisionResistance -v` 通过（确定性输入产生 22 次碰撞，远低于阈值 40）。
- 不引入任何其它代码或文档改动。
</success_criteria>

<output>
完成后在 `.planning/quick/260513-fjd-subnetthirdoctet/` 下创建 `260513-fjd-SUMMARY.md`，使用中文撰写，记录：
- 修改前/后阈值与注释对比
- 生日悖论的数学依据（E = k(k-1)/(2n)）
- 确定性输入下实际碰撞数（22）与新阈值（40）之间的安全余量（18）
- 未触碰 `subnetThirdOctet` 实现的说明
</output>
