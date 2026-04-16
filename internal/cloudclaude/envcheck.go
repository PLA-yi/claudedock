package cloudclaude

import (
	"bytes"
	"fmt"
	"strings"
)

type EnvCheckResult struct {
	Hostname string
	User     string
	OS       string
	Kernel   string
	Timezone string
	Locale   string
	PublicIP string
	DNSIP    string
	Uptime   string
	Memory   string
	Disk     string
	Claude   string
	Fuse     string
	SSHFS    string
	Jq       string
	Git      string
	Sudo     string
}

// RunEnvCheck 通过 SSH 连接到容器并收集环境信息。
func RunEnvCheck(cfg SSHConfig) (*EnvCheckResult, error) {
	conn, err := sshConnect(cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	run := func(cmd string) string {
		sess, err := conn.NewSession()
		if err != nil {
			return "(session error)"
		}
		defer sess.Close()
		var buf bytes.Buffer
		sess.Stdout = &buf
		if err := sess.Run(cmd); err != nil {
			out := strings.TrimSpace(buf.String())
			if out != "" {
				return out
			}
			return "(unavailable)"
		}
		return strings.TrimSpace(buf.String())
	}

	r := &EnvCheckResult{
		Hostname: run("hostname"),
		User:     run("whoami"),
		OS:       run("cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d'\"' -f2"),
		Kernel:   run("uname -r"),
		Timezone: run("cat /etc/timezone 2>/dev/null || timedatectl show -p Timezone --value 2>/dev/null || echo unknown"),
		Locale:   run("grep '^LANG=' /etc/default/locale 2>/dev/null | cut -d= -f2 || locale 2>/dev/null | grep '^LANG=' | cut -d= -f2"),
		PublicIP: run("curl -s --max-time 5 https://api.ipify.org 2>/dev/null || curl -s --max-time 5 https://ifconfig.me 2>/dev/null || echo timeout"),
		DNSIP:    run("dig +short myip.opendns.com @resolver1.opendns.com 2>/dev/null || echo N/A"),
		Uptime:   run("uptime -p 2>/dev/null || uptime"),
		Memory:   run("free 2>/dev/null | awk '/Mem:/{pct=int($3/$2*100); printf \"%.1fG / %.1fG (%d%%)\", $3/1048576, $2/1048576, pct}'"),
		Disk:     run("df -h / 2>/dev/null | awk 'NR==2{printf \"%s / %s (%s)\", $3, $2, $5}'"),
		Claude:   run("claude --version 2>/dev/null || claude-real --version 2>/dev/null || echo not found"),
		Fuse:     run("test -c /dev/fuse && echo OK || echo MISSING"),
		SSHFS:    run("sshfs --version 2>&1 | head -1"),
		Jq:       run("jq --version 2>/dev/null || echo not found"),
		Git:      run("git --version 2>/dev/null || echo not found"),
		Sudo:     run("bash -c 'sudo -n true 2>/dev/null && echo OK || echo NO'"),
	}

	return r, nil
}

func (r *EnvCheckResult) Print() {
	fmt.Println("╭─────────────────────────────────────────╮")
	fmt.Println("│       Cloud Claude 环境检测报告         │")
	fmt.Println("╰─────────────────────────────────────────╯")
	fmt.Println()

	section("系统信息")
	row("主机名", r.Hostname)
	row("用户", r.User)
	row("系统", r.OS)
	row("内核", r.Kernel)
	row("运行时间", r.Uptime)
	fmt.Println()

	section("区域设置")
	row("时区", r.Timezone)
	row("语言", r.Locale)
	fmt.Println()

	section("网络")
	row("出口 IP (HTTP)", r.PublicIP)
	row("出口 IP (DNS)", r.DNSIP)
	ipMatch := r.PublicIP == r.DNSIP
	if r.DNSIP == "N/A" || r.DNSIP == "(unavailable)" {
		row("IP 一致性", "⚠ DNS 检测不可用，无法比对")
	} else if ipMatch {
		row("IP 一致性", "✓ HTTP 与 DNS 出口一致（无泄漏）")
	} else {
		row("IP 一致性", "✗ HTTP 与 DNS 出口不一致，可能存在泄漏！")
	}
	fmt.Println()

	section("工具链")
	row("Claude Code", r.Claude)
	row("Git", r.Git)
	row("jq", r.Jq)
	fmt.Println()

	section("FUSE / 挂载支持")
	row("/dev/fuse", r.Fuse)
	row("sshfs", r.SSHFS)
	row("sudo 免密", r.Sudo)
	fmt.Println()

	section("资源")
	row("内存", r.Memory)
	row("磁盘 (/)", r.Disk)
}

func section(title string) {
	fmt.Printf("  ── %s ──\n", title)
}

func row(label, value string) {
	fmt.Printf("  %-16s %s\n", label, value)
}

