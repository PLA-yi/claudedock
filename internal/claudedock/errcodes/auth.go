package errcodes

// AUTH_* 错误码注册（Phase 34 D-21）。本地 config / Entry token / OAuth refresh。
// 同文件附带 NET_EGRESS_IP_DRIFT（doctor network.egress_ip_visible 命中 → 与 auth/gateway 语义同组）。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       AUTH_CONFIG_MISSING,
		Severity:   SeverityFatal,
		Message:    "~/.claudedock/config.yaml 不存在或解析失败: %s",
		NextAction: "运行 claudedock init 重新配置网关与凭证",
	})

	MustRegister(Entry{
		Code:       AUTH_GATEWAY_UNREACHABLE,
		Severity:   SeverityError,
		Message:    "网关 %s 不可达: %s",
		NextAction: "检查网络与 gateway 配置，或运行 claudedock init",
	})

	MustRegister(Entry{
		Code:       AUTH_TOKEN_EXPIRED,
		Severity:   SeverityWarn,
		Message:    "Entry API token 已过期或 401: %s",
		NextAction: "运行 claudedock doctor auth --fix 自动刷新",
	})

	MustRegister(Entry{
		Code:       AUTH_OAUTH_REFRESH_FAILED,
		Severity:   SeverityError,
		Message:    "Claude OAuth 刷新失败: %s",
		NextAction: "在容器内运行 claudedock exec claude login 重新登录",
	})

	MustRegister(Entry{
		Code:       NET_EGRESS_IP_DRIFT,
		Severity:   SeverityWarn,
		Message:    "容器出口 IP (%s) 与 Entry API 期望值 (%s) 不一致",
		NextAction: "检查代理出口配置，或运行 claudedock doctor network",
	})
}
