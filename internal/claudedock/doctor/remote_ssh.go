package doctor

// Phase 41 Plan 01 Task 4: remote-ssh 维度实现。
//
// 5 项远端检查，覆盖 VS Code Remote-SSH 场景的诊断：
//   - vscode_server_process: VS Code Server 进程存在性
//   - vscode_server_port: VS Code Server 端口监听
//   - vscode_server_disk: ~/.vscode-server/ 磁盘占用
//   - forwarding_socket: SSH forwarding socket 存在性
//   - forwarding_blocked: 防火墙是否拦截 forwarding
//
// 远端检查走 RemoteRunner 接口，保持 lazy connect 模式不变。

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/claudedock/claudedock/internal/claudedock/errcodes"
)

const (
	vscodeServerDiskWarnMB = 500
	vscodeServerDiskBloatMB = 2048
)

// checkVSCodeServerProcess 检测远端容器内 VS Code Server 进程是否存在。
// 进程不存在 → Skip（用户可能未使用 VS Code Remote-SSH）。
func checkVSCodeServerProcess(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("remote-ssh", "vscode_server_process", "未能连接远端容器，跳过")
	}
	stdout, _, err := runner.RunScript("vscode_server_process",
		"pgrep -f vscode-server >/dev/null 2>&1 && echo running || echo stopped")
	if err != nil {
		return newSkip("remote-ssh", "vscode_server_process", "进程检测命令执行失败: "+err.Error())
	}
	if strings.TrimSpace(stdout) == "stopped" {
		return newSkip("remote-ssh", "vscode_server_process", "VS Code Server 进程未运行（用户可能未使用 VS Code Remote-SSH）")
	}
	return newPass("remote-ssh", "vscode_server_process", "VS Code Server 进程运行中")
}

// checkVSCodeServerPort 检测 VS Code Server 是否在监听端口。
// 仅在进程存在时有意义；进程不存在时由 checkVSCodeServerProcess 返回 Skip。
func checkVSCodeServerPort(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("remote-ssh", "vscode_server_port", "未能连接远端容器，跳过")
	}
	// 先确认进程存在
	stdout, _, err := runner.RunScript("vscode_server_port",
		"pgrep -f vscode-server >/dev/null 2>&1 && ss -tlnp 2>/dev/null | grep -q vscode-server && echo listening || echo not_listening")
	if err != nil {
		return newSkip("remote-ssh", "vscode_server_port", "端口检测命令执行失败: "+err.Error())
	}
	if strings.TrimSpace(stdout) == "not_listening" {
		return newWarn("remote-ssh", "vscode_server_port", errcodes.SSH_VSCODE_PORT_NOT_LISTENING)
	}
	return newPass("remote-ssh", "vscode_server_port", "VS Code Server 端口正常监听")
}

// checkVSCodeServerDisk 检测远端 ~/.vscode-server/ 磁盘占用。
// 目录不存在 → Skip；≥ 2GB → Fail；≥ 500MB → Warn；< 500MB → Pass。
// 复用 disk.go 中已有的 parseDuHumanToMB() 解析 du 输出。
func checkVSCodeServerDisk(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("remote-ssh", "vscode_server_disk", "未能连接远端容器，跳过")
	}
	stdout, _, err := runner.RunScript("vscode_server_disk",
		"du -sh ~/.vscode-server/ 2>/dev/null || echo NOT_FOUND")
	if err != nil {
		return newSkip("remote-ssh", "vscode_server_disk", "磁盘检测命令执行失败: "+err.Error())
	}
	out := strings.TrimSpace(stdout)
	if out == "NOT_FOUND" || strings.Contains(out, "No such file") {
		return newSkip("remote-ssh", "vscode_server_disk", "~/.vscode-server/ 目录不存在，跳过")
	}
	// du -sh 输出格式："1.2G\t/home/user/.vscode-server/"
	parts := strings.Fields(out)
	if len(parts) == 0 {
		return newSkip("remote-ssh", "vscode_server_disk", "无法解析 du 输出: "+out)
	}
	mb := parseDuHumanToMB(parts[0])
	switch {
	case mb >= vscodeServerDiskBloatMB:
		c := newFail("remote-ssh", "vscode_server_disk", errcodes.DISK_VSCODE_SERVER_BLOAT, mb)
		c.Details = map[string]any{
			"size_mb": mb,
			"cleanup_light":  "rm -rf ~/.vscode-server/extensions-cache/ （仅缓存）",
			"cleanup_medium": "rm -rf ~/.vscode-server/*/extensions/*/ （所有扩展，保留配置）",
			"cleanup_full":   "rm -rf ~/.vscode-server/ （完全清理，VS Code 重连时会重建）",
		}
		return c
	case mb >= vscodeServerDiskWarnMB:
		c := newWarn("remote-ssh", "vscode_server_disk", errcodes.DISK_VSCODE_SERVER_WARN, mb)
		c.Details = map[string]any{
			"size_mb": mb,
			"cleanup_light":  "rm -rf ~/.vscode-server/extensions-cache/ （仅缓存）",
			"cleanup_medium": "rm -rf ~/.vscode-server/*/extensions/*/ （所有扩展，保留配置）",
		}
		return c
	default:
		return newPass("remote-ssh", "vscode_server_disk", fmt.Sprintf("~/.vscode-server/ 占用 %dMB", mb))
	}
}

// checkForwardingSocket 检测远端 SSH forwarding socket 是否存在。
// socket 不存在 → Skip（可能未建立 VS Code forwarding）。
func checkForwardingSocket(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("remote-ssh", "forwarding_socket", "未能连接远端容器，跳过")
	}
	stdout, _, err := runner.RunScript("forwarding_socket",
		"ss -xp 2>/dev/null | grep -q forwarding && echo found || echo not_found")
	if err != nil {
		return newSkip("remote-ssh", "forwarding_socket", "socket 检测命令执行失败: "+err.Error())
	}
	if strings.TrimSpace(stdout) == "not_found" {
		return newSkip("remote-ssh", "forwarding_socket", "SSH forwarding socket 不存在（可能未建立 VS Code forwarding）")
	}
	return newPass("remote-ssh", "forwarding_socket", "SSH forwarding socket 存在")
}

// checkForwardingBlocked 检测远端防火墙是否拦截 forwarding 流量。
// 仅在 forwarding socket 存在时有意义。
func checkForwardingBlocked(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("remote-ssh", "forwarding_blocked", "未能连接远端容器，跳过")
	}
	// 先检查 forwarding socket 是否存在
	sockOut, _, err := runner.RunScript("forwarding_blocked_check",
		"ss -xp 2>/dev/null | grep -q forwarding && echo found || echo not_found")
	if err != nil {
		return newSkip("remote-ssh", "forwarding_blocked", "socket 检测命令执行失败: "+err.Error())
	}
	if strings.TrimSpace(sockOut) == "not_found" {
		return newSkip("remote-ssh", "forwarding_blocked", "forwarding socket 不存在，跳过防火墙检测")
	}
	// 检测 OUTPUT 链 DROP 规则计数
	stdout, _, err := runner.RunScript("forwarding_blocked",
		"iptables -L OUTPUT -n 2>/dev/null | grep -c DROP || echo 0")
	if err != nil {
		return newSkip("remote-ssh", "forwarding_blocked", "iptables 检测命令执行失败: "+err.Error())
	}
	count, _ := strconv.Atoi(strings.TrimSpace(stdout))
	if count > 0 {
		c := newWarn("remote-ssh", "forwarding_blocked", errcodes.SSH_FORWARDING_BLOCKED)
		c.Details = map[string]any{"drop_rules": count}
		return c
	}
	return newPass("remote-ssh", "forwarding_blocked", "OUTPUT 链无 DROP 规则，forwarding 流量正常")
}
