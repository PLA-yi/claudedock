package doctor

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/claudedock/claudedock/internal/claudedock"
	"github.com/claudedock/claudedock/internal/claudedock/errcodes"
)

// checkKeepaliveConfig 纯本地：MountConfig.KeepAliveInterval >= 15s（PITFALLS M11）。
func checkKeepaliveConfig(ctx context.Context, keepalive time.Duration) Check {
	if keepalive == 0 {
		return newSkip("ssh", "keepalive_config", "未设置 KeepAliveInterval，跳过（默认 15s）")
	}
	if keepalive < 15*time.Second {
		return newFail("ssh", "keepalive_config", errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE, keepalive.String())
	}
	return newPass("ssh", "keepalive_config", fmt.Sprintf("KeepAliveInterval=%s ≥ 15s", keepalive))
}

// checkSSHDKeepaliveDrift 远端 sshd -T（RESEARCH §3.3）。容器内 Phase 29 基线 15/8。
func checkSSHDKeepaliveDrift(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("ssh", "sshd_keepalive_drift", "未能连接远端容器，跳过")
	}
	stdout, _, err := runner.RunScript("sshd_keepalive",
		"sshd -T 2>/dev/null | grep -E '^(clientalive(interval|countmax))\\b' || true")
	if err != nil {
		return newWarn("ssh", "sshd_keepalive_drift", errcodes.SSH_SSHD_KEEPALIVE_DRIFT, "sshd -T 失败: "+err.Error())
	}
	interval, count := parseSSHDKeepalive(stdout)
	baseline := "clientaliveinterval=15 clientalivecountmax=8"
	if interval != 15 || count != 8 {
		got := fmt.Sprintf("clientaliveinterval=%d clientalivecountmax=%d", interval, count)
		c := newWarn("ssh", "sshd_keepalive_drift", errcodes.SSH_SSHD_KEEPALIVE_DRIFT, got)
		c.Details = map[string]any{"interval": interval, "count": count, "baseline": "15/8"}
		return c
	}
	return newPass("ssh", "sshd_keepalive_drift", baseline)
}

// parseSSHDKeepalive 从 sshd -T 输出解析两个值；解析失败返回 0。
func parseSSHDKeepalive(out string) (interval, count int) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "clientaliveinterval "):
			interval, _ = strconv.Atoi(strings.TrimPrefix(line, "clientaliveinterval "))
		case strings.HasPrefix(line, "clientalivecountmax "):
			count, _ = strconv.Atoi(strings.TrimPrefix(line, "clientalivecountmax "))
		}
	}
	return
}

// parseSSHDForwarding 从 sshd -T 输出解析三个转发指令；缺失时返回空串。
func parseSSHDForwarding(out string) (tcpForwarding, streamForwarding, gatewayPorts string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		// 用 TrimPrefix 从原始行中提取值；大小写混合时回退到 lowercase 做前缀匹配
		trimVal := func(prefix, l, lo string) string {
			if v, ok := strings.CutPrefix(l, prefix); ok {
				return strings.TrimSpace(v)
			}
			if v, ok := strings.CutPrefix(lo, prefix); ok {
				return strings.TrimSpace(v)
			}
			return ""
		}
		switch {
		case strings.HasPrefix(lower, "allowtcpforwarding "):
			tcpForwarding = trimVal("allowtcpforwarding ", line, lower)
		case strings.HasPrefix(lower, "allowstreamlocalforwarding "):
			streamForwarding = trimVal("allowstreamlocalforwarding ", line, lower)
		case strings.HasPrefix(lower, "gatewayports "):
			gatewayPorts = trimVal("gatewayports ", line, lower)
		}
	}
	return
}

// checkSSHDForwarding 远端 sshd 转发指令检查。基线：AllowTcpForwarding=yes, AllowStreamLocalForwarding=yes, GatewayPorts=no。
func checkSSHDForwarding(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("ssh", "sshd_forwarding", "未能连接远端容器，跳过")
	}
	stdout, _, err := runner.RunScript("sshd_forwarding",
		"sshd -T 2>/dev/null | grep -E '^(allowtcpforwarding|allowstreamlocalforwarding|gatewayports)\\b' || true")
	if err != nil {
		// runner 错误时，假设第一个指令有问题（与基线不一致）
		return newWarn("ssh", "sshd_forwarding_disabled", errcodes.SSH_SSHD_FORWARDING_DISABLED, "sshd -T 失败: "+err.Error())
	}
	tcp, stream, gw := parseSSHDForwarding(stdout)

	if tcp != "yes" {
		actual := tcp
		if actual == "" {
			actual = "(缺失)"
		}
		return newWarn("ssh", "sshd_forwarding_disabled", errcodes.SSH_SSHD_FORWARDING_DISABLED, actual)
	}
	if stream != "yes" {
		actual := stream
		if actual == "" {
			actual = "(缺失)"
		}
		return newWarn("ssh", "sshd_stream_forwarding_disabled", errcodes.SSH_SSHD_STREAM_FORWARDING_DISABLED, actual)
	}
	if gw != "no" {
		actual := gw
		if actual == "" {
			actual = "(缺失)"
		}
		return newWarn("ssh", "sshd_gateway_ports_open", errcodes.SSH_SSHD_GATEWAY_PORTS_OPEN, actual)
	}
	return newPass("ssh", "sshd_forwarding", "AllowTcpForwarding=yes AllowStreamLocalForwarding=yes GatewayPorts=no")
}

// checkKnownHosts 本地读 ~/.ssh/known_hosts（RESEARCH §3.3 gotcha：HASHED hostname，
// 用 knownhosts.New HostKeyCallback）。本 plan 简化：
//   - ~/.ssh/known_hosts 不存在 → Skip
//   - 其中含 host 但解析失败 → Warn
//   - 含 host 且可 Load → Pass（精确 fingerprint 比对 v3.1 backlog）
func checkKnownHosts(ctx context.Context, khPath string, authHostPort string) Check {
	if khPath == "" || authHostPort == "" {
		return newSkip("ssh", "known_hosts", "未配置远端地址，跳过")
	}
	_, err := knownhosts.New(khPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "not exist") {
			return newSkip("ssh", "known_hosts", "~/.ssh/known_hosts 不存在，跳过（首次连接时自动创建）")
		}
		// Plan 03 Task 3.3：fix.go 依赖 Details["host_port"] 跑 ssh-keygen -R
		c := newWarn("ssh", "known_hosts", errcodes.SSH_KNOWN_HOSTS_CONFLICT, authHostPort)
		c.Details = map[string]any{"host_port": authHostPort}
		return c
	}
	return newPass("ssh", "known_hosts", fmt.Sprintf("known_hosts 已加载（含 %s 条目将由后续连接校验）", authHostPort))
}

// checkWorkspaceSSHKeys 复用 v2.0 RunSSHDoctor（CONTEXT D-04 双入口共存，doctor ssh 维度内部调同一函数）。
// 结果以 Details 形式返回；**本阶段无错误码**（D-19）。
func checkWorkspaceSSHKeys(ctx context.Context, sshCfg claudedock.SSHConfig) Check {
	res, err := claudedock.RunSSHDoctor(sshCfg, claudedock.SSHDoctorOptions{Fix: false})
	if err != nil {
		return Check{
			Domain: "ssh", Name: "workspace_ssh_keys", Status: StatusFail,
			Message:    "RunSSHDoctor 失败: " + err.Error(),
			NextAction: "运行 claudedock ssh doctor 查看详细报告",
		}
	}
	status := StatusPass
	problemCount := 0
	for _, f := range res.Files {
		if !f.OwnerOK || !f.ModeOK {
			problemCount++
		}
		if f.PEMEndsNL != nil && !*f.PEMEndsNL {
			problemCount++
		}
	}
	if problemCount > 0 {
		status = StatusWarn
	}
	return Check{
		Domain: "ssh", Name: "workspace_ssh_keys", Status: status,
		Message:    fmt.Sprintf("扫描 %d 个文件，%d 个问题项", len(res.Files), problemCount),
		NextAction: "运行 claudedock ssh doctor --fix 自动修复",
		Details:    map[string]any{"files": len(res.Files), "problems": problemCount},
	}
}

// makeKnownHostsHostPort 构造 known_hosts 字面量的 host:port（auth 成功后用）。
func makeKnownHostsHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}
