package errcodes

// STATE_* 错误码注册（Phase 34 D-21 + D-27）。
// STATE_VOLUME_IN_USE_001 字面量与 Phase 33 admin_claude_accounts.go 硬编码保持一致（D-27 兼容已部署 frontend）。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       STATE_LAST_SESSION_MISSING,
		Severity:   SeverityInfo,
		Message:    "未找到上次会话快照（%s）",
		NextAction: "首次运行 claudedock 后再 doctor 即可看到",
	})

	MustRegister(Entry{
		Code:       STATE_VOLUME_IN_USE_001,
		Severity:   SeverityError,
		Message:    "持久化 volume %s 仍被容器持有，DELETE 拒绝",
		NextAction: "先停止容器：claudedock admin hosts stop <id>",
	})

	MustRegister(Entry{
		Code:       STATE_CONTAINER_NOT_RUNNING,
		Severity:   SeverityWarn,
		Message:    "主机 %s 状态为 %s，远端 doctor 检查跳过",
		NextAction: "运行 claudedock admin hosts start <id> 启动容器",
	})
}
