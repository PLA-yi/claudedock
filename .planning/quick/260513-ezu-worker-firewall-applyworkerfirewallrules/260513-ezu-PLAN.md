---
phase: quick-260513-ezu
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/network/worker_firewall_linux_test.go
autonomous: true
requirements:
  - QUICK-FIX-CI-WORKER-FIREWALL-ARGS
must_haves:
  truths:
    - "go build ./... 在 linux 平台下不再报 too many arguments 错误"
    - "go vet ./internal/network/... 通过"
    - "ApplyWorkerFirewallRules 在测试文件中的所有调用参数数量为 4，与函数签名一致"
  artifacts:
    - path: "internal/network/worker_firewall_linux_test.go"
      provides: "修正后的测试调用，去除第 204、371 行多余的 nil"
      contains: "ApplyWorkerFirewallRules"
  key_links:
    - from: "internal/network/worker_firewall_linux_test.go"
      to: "internal/network/worker_firewall_linux.go"
      via: "ApplyWorkerFirewallRules 函数调用签名"
      pattern: "ApplyWorkerFirewallRules\\([^,]+,[^,]+,[^,]+,[^,]+\\)"
---

<objective>
修复 CI 构建失败：删除 internal/network/worker_firewall_linux_test.go 中第 204 行和第 371 行调用 ApplyWorkerFirewallRules 时多传的 nil 参数，使其与函数 4 参数签名一致。

Purpose: 解除 linux 平台下的编译阻塞，恢复 CI 通过。
Output: 修正后的测试文件，go build / go vet 在 linux 下通过。
</objective>

<execution_context>
@.claude/get-shit-done/workflows/execute-plan.md
@.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@./CLAUDE.md
@internal/network/worker_firewall_linux.go
@internal/network/worker_firewall_linux_test.go

<interfaces>
<!-- 关键签名（来自 internal/network/worker_firewall_linux.go:25），执行者直接对照即可，无需再次探索代码。 -->

From internal/network/worker_firewall_linux.go:
```go
func ApplyWorkerFirewallRules(
    containerNS netns.NsHandle,
    gwIP, bridgeGW net.IP,
    sshPort uint16,
) error
```

当前错误调用位置：
- internal/network/worker_firewall_linux_test.go:204
  `ApplyWorkerFirewallRules(invalidNS, gwIP, bridgeGW, 22, nil)`
- internal/network/worker_firewall_linux_test.go:371
  `ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, customSSHPort, nil)`

正确调用形式（4 参数）：
- `ApplyWorkerFirewallRules(invalidNS, gwIP, bridgeGW, 22)`
- `ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, customSSHPort)`
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: 删除 worker firewall 测试中多余的 nil 参数</name>
  <files>internal/network/worker_firewall_linux_test.go</files>
  <action>
对 internal/network/worker_firewall_linux_test.go 做最小化修复：

1. 第 204 行：
   - 现状：`err := ApplyWorkerFirewallRules(invalidNS, gwIP, bridgeGW, 22, nil)`
   - 改为：`err := ApplyWorkerFirewallRules(invalidNS, gwIP, bridgeGW, 22)`

2. 第 371 行：
   - 现状：`err := ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, customSSHPort, nil)`
   - 改为：`err := ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, customSSHPort)`

仅删除末尾的 `, nil`，不要改动其他代码、注释、空白或测试逻辑。

不要修改 ApplyWorkerFirewallRules 函数签名本身（功能签名是 4 参数版本，CI 错误中的 want 行已确认这是正确签名）。
不要新增任何参数、不要新增字段、不要补 mock 对象。
  </action>
  <verify>
    <automated>cd /Users/zaneliu/Projects/open-source/claudedock-main &amp;&amp; grep -n "ApplyWorkerFirewallRules(" internal/network/worker_firewall_linux_test.go | grep -v "^[[:space:]]*//" | awk -F'ApplyWorkerFirewallRules\\(' '{print $2}' | awk -F')' '{print $1}' | awk -F',' '{print NF}' | sort -u | grep -qx 4</automated>
  </verify>
  <done>
测试文件中所有对 ApplyWorkerFirewallRules 的调用参数数量均为 4；在 linux 构建环境下 `go build ./...` 与 `go vet ./internal/network/...` 不再报 "too many arguments in call to ApplyWorkerFirewallRules" 错误。
  </done>
</task>

</tasks>

<verification>
- 测试文件中无 5 参数调用：`grep -n "ApplyWorkerFirewallRules(" internal/network/worker_firewall_linux_test.go` 列出的每一行均与 4 参数签名匹配。
- 在 linux 平台或 CI 上运行 `go build ./...`、`go vet ./internal/network/...` 均通过。
- 仅有 2 行被修改（第 204 与 第 371 行），diff 极小。
</verification>

<success_criteria>
- internal/network/worker_firewall_linux_test.go 中第 204、371 行不再包含末尾的 `, nil` 参数。
- 没有其他无关改动（无新增导入、无函数签名修改、无注释 / 空白改动）。
- linux 平台下编译与 vet 通过，原 CI 报错信息消除。
</success_criteria>

<output>
完成后创建 `.planning/quick/260513-ezu-worker-firewall-applyworkerfirewallrules/260513-ezu-SUMMARY.md`，记录：
- 修改的文件与具体行号
- 修复前后的调用形式对比
- 本地或 CI 验证结果（go build / go vet）
</output>
