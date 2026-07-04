# Phase 41: Doctor 扩展与收尾 - Research

**Researched:** 2026-05-08
**Status:** COMPLETE

## 1. Existing Doctor Architecture

### Dimension Model
`RunDoctor()` in `doctor.go` 串行执行 7 个维度块：network → auth → ssh → mount → disk。每个维度通过 `want("name")` 闭包过滤。

### Remote Check Pattern
- `RemoteRunner` 接口（`remote_runner.go`）封装远端命令执行
- `SSHRemoteRunner` 基于 `golang.org/x/crypto/ssh.Client`
- `ensureRemote()` lazy 建立 SSH 连接，失败时所有远端 check 自动降级为 Skip
- 远端 check 用 `runner.RunScript("check_name", "shell commands")` 执行

### Check Constructors
- `newPass(domain, name, msg)` → StatusPass
- `newWarn(domain, name, code, args...)` → StatusWarn + 从 errcodes 拉 Message/NextAction
- `newFail(domain, name, code, args...)` → StatusFail
- `newSkip(domain, name, reason)` → StatusSkip

### Timeout Wrapper
`runWithTimeout(ctx, domain, name, timeout, fn)` 统一包装单 check 超时（默认 5s / verbose 30s）。

## 2. Error Code Registry Pattern

### Constants (`errcodes/codes.go`)
```go
const (
    SSH_VSCODE_SERVER_NOT_RUNNING Code = "SSH_VSCODE_SERVER_NOT_RUNNING"
)
```
命名规范：`^[A-Z]+_[A-Z]+_[A-Z0-9]+$`

### Registration (`errcodes/ssh.go` 等域文件 init())
```go
func init() {
    MustRegister(Entry{
        Code:       SSH_VSCODE_SERVER_NOT_RUNNING,
        Severity:   SeverityInfo,
        Message:    "...",
        NextAction: "...",
    })
}
```

### Long Explanations (`errcodes/explanations.go`)
- `registerExplanation(code, text)` 在 init() 中调用
- 五段模板：触发场景 / 根本原因 / 复现方式 / 修复路径 / 关联文档
- `ExplainExempt` map 豁免 informational 级别码

## 3. Reusable Assets for Phase 41

| Asset | Location | Usage |
|-------|----------|-------|
| `RemoteRunner` 接口 | `doctor/remote_runner.go` | remote-ssh 维度直接复用 |
| `runWithTimeout()` | `doctor/check.go:35` | 新 check 统一使用 |
| `newPass/newWarn/newFail/newSkip` | `doctor/check.go:67-91` | 新 check 函数直接调用 |
| `parseDuHumanToMB()` | `doctor/disk.go:76` | .vscode-server 磁盘占用解析 |
| `errcodes.MustRegister()` | `errcodes/codes.go:61` | 新错误码注册 |
| `registerExplanation()` | `errcodes/explanations.go:29` | 新错误码长说明 |

## 4. Integration Points

### `RunDoctor()` (`doctor.go:85`)
- disk 维度之后、Summary 聚合之前插入 remote-ssh 维度块
- 远端 check 复用已有 `ensureRemote()` → `remoteRunner`

### Cobra 命令 (`cmd/claudedock/doctor.go:34`)
- `ValidArgs` 需要加入 `"remote-ssh"`
- `Short` 描述文本需要更新（五维度 → 六维度）

### 错误码文件
- `errcodes/codes.go`：新增 6 个 Code 常量
- `errcodes/explanations.go`：新增 4 条 `registerExplanation()`（2 个 Info 码进 ExplainExempt）

## 5. New Error Codes

| Code | Severity | Domain | 说明 |
|------|----------|--------|------|
| `SSH_VSCODE_SERVER_NOT_RUNNING` | Info | remote-ssh | VS Code Server 进程不存在 |
| `SSH_VSCODE_PORT_NOT_LISTENING` | Warn | remote-ssh | 进程存在但端口未监听 |
| `SSH_FORWARDING_SOCKET_MISSING` | Info | remote-ssh | forwarding socket 不存在 |
| `SSH_FORWARDING_BLOCKED` | Warn | remote-ssh | 防火墙拦截 forwarding |
| `DISK_VSCODE_SERVER_WARN` | Warn | disk (remote-ssh) | ~/.vscode-server/ ≥ 500MB |
| `SSH_VSCODE_SERVER_BLOAT` | Error | disk (remote-ssh) | ~/.vscode-server/ ≥ 2GB |

注意：CONTEXT.md 建议 DISK_VSCODE_SERVER_BLOAT 用 DISK_* 前缀，但查看现有模式，磁盘占用是独立维度。建议统一用 SSH_VSCODE_SERVER_BLOAT（保持 remote-ssh 维度内一致性），或用 DISK_VSCODE_SERVER_BLOAT（遵循磁盘类前缀）。实现时按 CONTEXT 决策走 DISK_* 前缀。

## 6. Check Design

### `vscode_server_process`
- 远端执行 `pgrep -f vscode-server`
- 进程不存在 → Skip（用户可能未使用 VS Code）
- 进程存在 → Pass

### `vscode_server_port`
- 依赖 vscode_server_process 结果
- 远端执行 `ss -tlnp | grep vscode-server`
- 端口未监听 → Warn
- 端口正常 → Pass

### `vscode_server_disk`
- 远端执行 `du -sh ~/.vscode-server/ 2>/dev/null`
- 目录不存在 → Skip
- 复用 `parseDuHumanToMB()` 解析
- ≥ 2GB → Fail + 清理建议（完整清理路径）
- ≥ 500MB → Warn + 清理建议（轻量/中量清理路径）
- < 500MB → Pass

### `forwarding_socket`
- 远端执行 `ss -xp | grep forwarding`
- socket 不存在 → Skip
- socket 存在 → Pass

### `forwarding_blocked`
- 依赖 forwarding_socket 存在
- 远端执行 `iptables -L OUTPUT -n 2>/dev/null | grep -c DROP`
- 有 DROP 规则 → Warn
- 无 DROP 规则 → Pass

## 7. File Plan

| Action | File | Description |
|--------|------|-------------|
| Create | `internal/claudedock/doctor/remote_ssh.go` | remote-ssh 维度 5 个 check 函数 |
| Create | `internal/claudedock/doctor/remote_ssh_test.go` | 单元测试（fakeRunner mock） |
| Modify | `internal/claudedock/doctor/doctor.go` | RunDoctor() 插入 remote-ssh 维度块 |
| Create | `internal/claudedock/errcodes/remote_ssh.go` | SSH_* 新错误码注册 |
| Modify | `internal/claudedock/errcodes/codes.go` | 新增 6 个 Code 常量 |
| Modify | `internal/claudedock/errcodes/explanations.go` | 新增 4 条长说明 + 2 条 ExplainExempt |
| Modify | `cmd/claudedock/doctor.go` | ValidArgs 加入 remote-ssh，更新描述 |

## 8. Gotchas & Risks

1. **进程检测误判**：`pgrep -f vscode-server` 可能匹配到 grep 自身或非 VS Code 进程。建议用 `pgrep -f 'vscode-server.*--connection-token'` 更精确，或用 `pgrep -x node` + `ls /proc/$pid/cmdline` 交叉验证。
2. **端口检测多实例**：VS Code Server 可能在多个端口监听（语言服务器、调试器）。`ss -tlnp | grep vscode-server` 返回多行时取第一个即可。
3. **du 耗时**：`~/.vscode-server/` 可能很大导致 du 超时。runWithTimeout 5s 默认超时已覆盖。
4. **iptables 权限**：容器内 iptables 可能需要 root 权限。managed-user 容器默认 root，但非 root 场景需 fallback。
5. **ExplainExempt 一致性测试**：`TestExplainExemptOnlyInformational` 需要验证新增的 Info 码被正确加入 ExplainExempt。

---

*Phase: 41-doctor*
*Research completed: 2026-05-08*
