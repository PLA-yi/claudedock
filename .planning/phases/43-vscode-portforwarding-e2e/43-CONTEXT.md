# Phase 43: VS Code Remote-SSH 端口转发 E2E 补齐 - Context

**Gathered:** 2026-05-08
**Status:** Ready for planning

<domain>
## Phase Boundary

完成 VS Code Remote-SSH 端口转发 + egress 流量的端到端验证，生成标准 VERIFICATION.md。补齐 Phase 40 验证缺口、集成缺口（UAT 端口转发）和流程缺口（VS Code 端口转发 E2E）。

</domain>

<decisions>
## Implementation Decisions

### UAT 脚本补充
- UAT 脚本必须包含 direct-tcpip 端口转发验证场景
- UAT 脚本必须包含 tcpip-forward 端口转发验证场景
- 测试应覆盖本地和云端两种模式

### 端口转发验证
- 验证 VS Code 语言服务器端口（如 60000-60010）通过 direct-tcpip channel 正常工作
- 验证多个并发端口转发通道不互相干扰
- 验证端口转发到被禁止的目标（管理网段、Docker socket）被正确拒绝

### 出口 IP 强约束验证
- 验证通过端口转发访问外部服务时，出口 IP 必须是绑定的 egress IP
- 验证 VS Code Server 下载和扩展安装流量通过 sing-box 出站
- 验证无 egress 配置时，流量不泄漏

### Claude's Discretion
- 具体测试端口选择（只要不冲突即可）
- 测试超时设置
- 错误消息格式

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/sshproxy/forward.go`: 完整的 direct-tcpip 和 forwarded-tcpip 实现
- `internal/sshproxy/forward_test.go`: 现有的单元测试（payload 解析、安全验证、通道中继）
- `isForbiddenTarget()`: 安全目标验证逻辑（管理网段、Docker socket、metadata 端点）
- Phase 40 的手动测试流程和发现的问题修复

### Established Patterns
- SSH 通道通过 `ssh.NewChannel` 接口处理
- 全局请求（tcpip-forward/cancel-tcpip-forward）通过 `handleGlobalRequests` 转发
- 安全验证在通道建立前执行，使用 `ssh.Prohibited` 拒绝

### Integration Points
- VS Code Remote-SSH 通过 SSH Proxy 2222 端口连接
- 端口转发请求从客户端 → proxy → 容器
- sing-box tun 设备处理所有出站流量

</code_context>

<specifics>
## Specific Ideas

- UAT 脚本应模拟 VS Code 的典型端口转发行为（语言服务器、调试器）
- 验证应包括成功和失败场景（安全拒绝）
- 出口 IP 验证需要实际检查 curl ifconfig.me 或类似服务

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 43-vscode-portforwarding-e2e*
*Context gathered: 2026-05-08*
