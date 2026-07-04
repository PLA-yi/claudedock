package errcodes

// SSH_* 错误码注册（Phase 34 D-21）。known_hosts 冲突 + sshd 基线漂移。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       SSH_KNOWN_HOSTS_CONFLICT,
		Severity:   SeverityWarn,
		Message:    "~/.ssh/known_hosts 中 %s 的 fingerprint 与本次握手不一致",
		NextAction: "运行 claudedock doctor ssh --fix 自动 ssh-keygen -R",
	})

	MustRegister(Entry{
		Code:       SSH_SSHD_KEEPALIVE_DRIFT,
		Severity:   SeverityWarn,
		Message:    "远端 sshd ClientAlive 配置 (%s) 与基线 (15/8) 不一致",
		NextAction: "重建容器以恢复基线（参考 deploy/docker/managed-user/sshd_config）",
	})

	MustRegister(Entry{
		Code:       SSH_SSHD_FORWARDING_DISABLED,
		Severity:   SeverityWarn,
		Message:    "远端 sshd AllowTcpForwarding 未开启（当前值: %s），端口转发功能不可用",
		NextAction: "检查 deploy/docker/managed-user/sshd_config 并重建容器恢复基线",
	})

	MustRegister(Entry{
		Code:       SSH_SSHD_STREAM_FORWARDING_DISABLED,
		Severity:   SeverityWarn,
		Message:    "远端 sshd AllowStreamLocalForwarding 未开启（当前值: %s），Unix socket 转发不可用",
		NextAction: "检查 deploy/docker/managed-user/sshd_config 并重建容器恢复基线",
	})

	MustRegister(Entry{
		Code:       SSH_SSHD_GATEWAY_PORTS_OPEN,
		Severity:   SeverityWarn,
		Message:    "远端 sshd GatewayPorts 非 no（当前值: %s），可能导致外部暴露",
		NextAction: "将 GatewayPorts 改为 no 并重启 sshd，或重建容器",
	})
}
