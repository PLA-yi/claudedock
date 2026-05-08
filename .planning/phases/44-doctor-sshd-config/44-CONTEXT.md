# Phase 44: doctor sshd_config 主动验证 - Context

**Gathered:** 2026-05-08
**Status:** Ready for planning (--auto 模式，推荐默认值)

<domain>
## Phase Boundary

doctor remote-ssh 维度主动检查 sshd_config 中端口转发相关指令（`AllowTcpForwarding`、`AllowStreamLocalForwarding`、`GatewayPorts`），消除 Phase 43 验证中发现的集成缺口：镜像已正确配置，但 doctor 没有主动检查，用户修改配置后无法自动发现。

不涉及新维度，不涉及新功能，仅在现有 doctor 检查体系中补充 sshd_config 解析检查。

</domain>

<decisions>
## Implementation Decisions

### D-01: 检查范围 — 覆盖全部三个转发相关指令
- 同时检查 `AllowTcpForwarding`、`AllowStreamLocalForwarding`、`GatewayPorts`
- 与镜像 `deploy/docker/managed-user/sshd_config` 中的 3 条显式配置对齐
- 一次检查消除整个端口转发配置面的隐患
- 基准值：`AllowTcpForwarding=yes`、`AllowStreamLocalForwarding=yes`、`GatewayPorts=no`

### D-02: 严重级别 — Warn
- `AllowTcpForwarding` 缺失或为 `no` → Warn（使用 `SSH_SSHD_FORWARDING_DISABLED` 错误码）
- `AllowStreamLocalForwarding` 缺失或为 `no` → Warn（使用 `SSH_SSHD_STREAM_FORWARDING_DISABLED` 错误码）
- `GatewayPorts` 不为 `no` → Warn（使用 `SSH_SSHD_GATEWAY_PORTS_OPEN` 错误码）
- 与现有 `SSH_SSHD_KEEPALIVE_DRIFT` 严重级别一致

### D-03: 归属维度 — ssh.go（非 remote_ssh.go）
- sshd_config 解析不依赖 VS Code，属于通用 SSH 健康检查
- 与现有的 `checkSSHDKeepaliveDrift`（ssh.go:29）同类：都是解析 `sshd -T` 输出
- remote_ssh.go 聚焦 VS Code 特有问题（进程/端口/磁盘/forwarding socket）

### D-04: 实现方式 — parseSSHDForwarding 函数提取 + table-driven 测试
- 从 `sshd -T` 输出中提取转发相关指令的解析逻辑抽成独立可测函数 `parseSSHDForwarding(output string) (tcpForwarding, streamForwarding string, gatewayPorts string)`
- 复用 `parseSSHDKeepalive` 的函数提取模式（ssh.go:50）
- 新增 `checkSSHDForwarding` check 函数，模式与 `checkSSHDKeepaliveDrift` 一致
- 测试用 table-driven 覆盖：正常配置 / 指令缺失 / 指令值为 no / 指令值异常

### D-05: 注册位置 — ssh 维度的 runChecks 函数中
- 在 `ssh.go` 的维度 check 列表中追加，紧跟 keepalive drift 检查之后
- `RunDoctor()` 中无需改动（维度注册已有）

### Claude's Discretion
- `sshd -T` 的具体 grep 命令可在实现时微调
- 错误码的 NextAction 描述文字格式

### Folded Todos
无

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Doctor 检查基础设施
- `internal/cloudclaude/doctor/ssh.go` — 现有 sshd_config 解析模式（checkSSHDKeepaliveDrift + parseSSHDKeepalive）
- `internal/cloudclaude/doctor/check.go` — Check 结构体、构造函数（newPass/newWarn/newFail/newSkip）、runWithTimeout
- `internal/cloudclaude/doctor/doctor.go` — RunDoctor 入口、维度注册模式

### 错误码
- `internal/cloudclaude/errcodes/codes.go` — SSH_* 错误码声明模式
- `internal/cloudclaude/errcodes/explanations.go` — registerExplanation 长说明注册模式

### 镜像基线
- `deploy/docker/managed-user/sshd_config` — 端口转发配置基准（AllowTcpForwarding yes / AllowStreamLocalForwarding yes / GatewayPorts no）
- `deploy/docker/managed-user/Dockerfile` — sshd_config COPY 到容器的构建步骤

### 前序上下文
- `.planning/phases/41-doctor/41-CONTEXT.md` — Phase 41 doctor 扩展决策（remote_ssh.go 维度）
- `.planning/phases/43-vscode-portforwarding-e2e/43-CONTEXT.md` — Phase 43 端口转发 E2E 验证
- `.planning/ROADMAP.md` § Phase 44 — 成功标准定义

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `parseSSHDKeepalive(output string)` (ssh.go:50) — `sshd -T` 输出解析模式，新 `parseSSHDForwarding` 直接复用同一模式
- `checkSSHDKeepaliveDrift` (ssh.go:29) — remote runner + sshd -T 的完整调用链，新 check 复制同一模式
- `SSH_SSHD_KEEPALIVE_DRIFT` (errcodes) — Warn 级别 sshd 配置偏差的错误码模式，新码按同一结构注册

### Established Patterns
- sshd 配置检查：`runner.RunScript("check_name", "sshd -T 2>/dev/null | grep ...")` → parse → compare against baseline → newWarn if drift
- 错误码注册：`MustRegister("SSH_*", "描述", SeverityWarn)` + `registerExplanation()` ≥200 字中文长说明
- 测试模式：`parseXxx` 函数单独测试 + check 函数用 mock runner 测试

### Integration Points
- `internal/cloudclaude/doctor/ssh.go` — 新增 `checkSSHDForwarding` 和 `parseSSHDForwarding`
- `internal/cloudclaude/doctor/ssh_test.go` — 新增 table-driven 测试用例
- `internal/cloudclaude/errcodes/codes.go` — 新增 3 个 SSH_* 错误码常量
- `internal/cloudclaude/errcodes/explanations.go` — 新增 3 条 registerExplanation 长说明

</code_context>

<specifics>
## Specific Ideas

- 复用 `ssh.go` 中已有的 `checkSSHDKeepaliveDrift` 作为模板，新 check 函数保持相同结构
- `parseSSHDForwarding` 返回三个值（tcpForwarding, streamForwarding, gatewayPorts），每个值为原始字符串
- 基准值硬编码为常量（与 keepalive drift 的 `expectedInterval=15, expectedCountMax=8` 模式一致）
- 测试 fixture 直接使用真实 `sshd -T` 输出片段

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 44-doctor-sshd-config*
*Context gathered: 2026-05-08 (--auto 模式)*
