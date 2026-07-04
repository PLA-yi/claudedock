package errcodes

// SYSTEM_* 错误码注册（Phase 34 D-21）。
// 覆盖 OS / kernel / FUSE / DNS / timeout 类；由 doctor network + mount 维度命中。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       SYSTEM_APPARMOR_FUSERMOUNT3_MISSING,
		Severity:   SeverityError,
		Message:    "AppArmor 缺 fusermount3 override（%s）",
		NextAction: "按 host-preflight.sh 写入 capability dac_override 行",
	})

	MustRegister(Entry{
		Code:       SYSTEM_FUSE_RESIDUAL_MOUNT,
		Severity:   SeverityWarn,
		Message:    "发现 %d 个残留 FUSE 挂载: %s",
		NextAction: "运行 claudedock doctor mount --fix 自动解挂",
	})

	MustRegister(Entry{
		Code:       SYSTEM_DNS_RESOLVE_FAILED,
		Severity:   SeverityError,
		Message:    "DNS 解析 %s 失败: %s",
		NextAction: "运行 claudedock doctor network --fix 刷新 DNS 缓存",
	})

	MustRegister(Entry{
		Code:       SYSTEM_CHECK_TIMEOUT,
		Severity:   SeverityWarn,
		Message:    "检查 %s 超时（>%s）",
		NextAction: "加 --verbose 放宽到 30s，或检查远端容器状态",
	})
}
