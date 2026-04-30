package cloudclaude

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/ssh"
)

// SessionDashboard 是会话信息面板的数据结构。
// 在文件挂载成功后、Claude Code PTY 接管终端前展示。
type SessionDashboard struct {
	Username        string
	ClaudeAccountID string
	ExitIP          string
	ImageVersion    string
	MountMode       string
	SessionName     string
	ConflictCount   int
	OversizedCount  int
	PromotionCount  int
	DowngradeChain  []string
	Warnings        []string
}

// CollectDashboard 通过已有 SSH 连接收集会话信息并组装面板数据。
// 所有 SSH 查询均为 best-effort，失败不 panic。
func CollectDashboard(
	conn *ssh.Client,
	mountCfg MountConfig,
	snap *LastSessionSnapshot,
) *SessionDashboard {
	d := &SessionDashboard{
		Username:        mountCfg.Username,
		ClaudeAccountID: mountCfg.ClaudeAccountID,
		ImageVersion:    mountCfg.ImageVersion,
	}

	// 出口 IP 查询
	if conn != nil {
		d.ExitIP = queryExitIP(conn)
	}
	if d.ExitIP == "" {
		d.ExitIP = "unavailable"
	}

	// 从 snapshot 读取挂载与同步状态
	if snap != nil {
		d.MountMode = snap.ActualMode
		if d.MountMode == "" {
			d.MountMode = snap.IntendedMode
		}
		d.ConflictCount = snap.ConflictCount
		d.OversizedCount = len(snap.OversizedFiles)
		d.PromotionCount = snap.PromotionCount
		for _, step := range snap.DowngradeChain {
			msg := fmt.Sprintf("%s → %s (%s)", step.From, step.To, step.ReasonMessage)
			d.DowngradeChain = append(d.DowngradeChain, msg)
		}
		if snap.TmuxSession != "" {
			d.SessionName = snap.TmuxSession
		}
	}
	if d.MountMode == "" {
		d.MountMode = mountCfg.Mode.String()
	}
	if d.SessionName == "" {
		d.SessionName = buildTmuxSessionName(mountCfg.ClaudeAccountID, mountCfg.Cwd)
	}

	// 动态警告
	if d.ConflictCount > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("⚠ %d 个文件同步冲突", d.ConflictCount))
	}
	if d.OversizedCount > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("⚠ %d 个大文件被跳过", d.OversizedCount))
	}
	if len(d.DowngradeChain) > 0 {
		d.Warnings = append(d.Warnings, "⚠ 挂载发生降级")
	}
	if mountCfg.IsSecondaryClient {
		d.Warnings = append(d.Warnings, "⚠ 当前为 secondary 客户端（热同步只读）")
	}

	return d
}

// queryExitIP 在 SSH 连接上执行 curl 查询容器出口公网 IP。
// 命令自带 5 秒超时，失败返回空串。
func queryExitIP(conn *ssh.Client) string {
	sess, err := conn.NewSession()
	if err != nil {
		return ""
	}
	defer sess.Close()

	cmd := "curl -s --max-time 5 https://api.ipify.org 2>/dev/null || echo timeout"
	out, err := sess.Output(cmd)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Print 渲染会话信息面板到 w。
func (d *SessionDashboard) Print(w io.Writer, noColor bool) {
	enabled := false
	if fh, ok := w.(fdHolder); ok {
		enabled = ColorEnabled(noColor, fh)
	}

	title := " Cloud Claude 会话信息面板 "
	if enabled {
		title = Colorize(title, AnsiCyan, enabled)
	}

	fmt.Fprintln(w, "╭──────────────────────────────────────────╮")
	fmt.Fprintf(w, "│%s│\n", centerPad(title, 42))
	fmt.Fprintln(w, "╰──────────────────────────────────────────╯")

	accountLabel := d.Username
	if d.ClaudeAccountID != "" {
		accountLabel += fmt.Sprintf(" (%s)", d.ClaudeAccountID)
	} else {
		accountLabel += " (未绑定 Claude 账号)"
	}

	ipValue := d.ExitIP
	if enabled && ipValue != "" && ipValue != "timeout" && ipValue != "unavailable" {
		ipValue = Colorize(ipValue, AnsiGreen, enabled)
	}

	modeValue := d.MountMode
	if enabled && len(d.DowngradeChain) > 0 {
		modeValue = Colorize(modeValue, AnsiYellow, enabled)
	}

	printRow(w, "出口 IP", ipValue)
	printRow(w, "使用账号", accountLabel)
	if d.ImageVersion != "" {
		printRow(w, "镜像版本", d.ImageVersion)
	}
	printRow(w, "挂载模式", modeValue)
	printRow(w, "会话名", d.SessionName)
	printRow(w, "同步冲突", fmt.Sprintf("%d", d.ConflictCount))
	printRow(w, "跳过大文件", fmt.Sprintf("%d", d.OversizedCount))
	printRow(w, "冷文件晋升", fmt.Sprintf("%d", d.PromotionCount))

	if len(d.DowngradeChain) > 0 {
		fmt.Fprintln(w)
		for _, dc := range d.DowngradeChain {
			line := "  → " + dc
			if enabled {
				line = Colorize(line, AnsiYellow, enabled)
			}
			fmt.Fprintln(w, line)
		}
	}

	if len(d.Warnings) > 0 {
		fmt.Fprintln(w)
		for _, warn := range d.Warnings {
			line := "  " + warn
			if enabled {
				line = Colorize(line, AnsiYellow, enabled)
			}
			fmt.Fprintln(w, line)
		}
	}

	fmt.Fprintln(w)
}

func printRow(w io.Writer, label, value string) {
	fmt.Fprintf(w, "  %-12s %s\n", label, value)
}

// centerPad 把 s 居中填充到总宽度 width（含 s 本身）。
// 若 s 长度已大于等于 width，则原样返回。
func centerPad(s string, width int) string {
	n := len(s)
	if n >= width {
		return s
	}
	left := (width - n) / 2
	right := width - n - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}
