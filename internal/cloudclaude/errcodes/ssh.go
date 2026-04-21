package errcodes

// SSH_* 错误码注册（Phase 34 D-21）。known_hosts 冲突 + sshd 基线漂移。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       SSH_KNOWN_HOSTS_CONFLICT,
		Severity:   SeverityWarn,
		Message:    "~/.ssh/known_hosts 中 %s 的 fingerprint 与本次握手不一致",
		NextAction: "运行 cloud-claude doctor ssh --fix 自动 ssh-keygen -R",
	})

	MustRegister(Entry{
		Code:       SSH_SSHD_KEEPALIVE_DRIFT,
		Severity:   SeverityWarn,
		Message:    "远端 sshd ClientAlive 配置 (%s) 与基线 (15/8) 不一致",
		NextAction: "重建容器以恢复基线（参考 deploy/docker/managed-user/sshd_config）",
	})
}
