package claudedock

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintDashboardFull(t *testing.T) {
	var buf bytes.Buffer
	dash := &SessionDashboard{
		Username:        "alice",
		ClaudeAccountID: "claude_abc123",
		ExitIP:          "1.2.3.4",
		ImageVersion:    "v3.2.3",
		MountMode:       "full",
		SessionName:     "claude-abc123-deadbeef",
		ConflictCount:   0,
		OversizedCount:  0,
		PromotionCount:  12,
	}
	dash.Print(&buf, true)
	out := buf.String()

	mustContain(t, out, "ClaudeDock 会话信息面板")
	mustContain(t, out, "出口 IP")
	mustContain(t, out, "1.2.3.4")
	mustContain(t, out, "alice")
	mustContain(t, out, "claude_abc123")
	mustContain(t, out, "v3.2.3")
	mustContain(t, out, "full")
	mustContain(t, out, "claude-abc123-deadbeef")
	mustContain(t, out, "同步冲突")
	mustContain(t, out, "跳过大文件")
	mustContain(t, out, "冷文件晋升")
	mustContain(t, out, "12")
}

func TestPrintDashboardWithWarnings(t *testing.T) {
	var buf bytes.Buffer
	dash := &SessionDashboard{
		Username:       "bob",
		MountMode:      "hot-only",
		ConflictCount:  3,
		OversizedCount: 5,
		Warnings:       []string{"⚠ 3 个文件同步冲突", "⚠ 5 个大文件被跳过"},
	}
	dash.Print(&buf, true)
	out := buf.String()

	mustContain(t, out, "3 个文件同步冲突")
	mustContain(t, out, "5 个大文件被跳过")
}

func TestPrintDashboardWithDowngradeChain(t *testing.T) {
	var buf bytes.Buffer
	dash := &SessionDashboard{
		Username:       "carol",
		MountMode:      "sshfs-only",
		DowngradeChain: []string{"full → hot-only (mergerfs 不支持)"},
		Warnings:       []string{"⚠ 挂载发生降级"},
	}
	dash.Print(&buf, true)
	out := buf.String()

	mustContain(t, out, "full → hot-only")
	mustContain(t, out, "mergerfs 不支持")
	mustContain(t, out, "挂载发生降级")
}

func TestPrintDashboardNoColor(t *testing.T) {
	var buf bytes.Buffer
	dash := &SessionDashboard{ExitIP: "8.8.8.8"}
	dash.Print(&buf, true) // noColor=true
	out := buf.String()

	if strings.Contains(out, "\033[") {
		t.Errorf("noColor=true 时输出不应包含 ANSI 转义序列，got: %q", out)
	}
}

func TestPrintDashboardEmpty(t *testing.T) {
	var buf bytes.Buffer
	dash := &SessionDashboard{}
	dash.Print(&buf, true)
	out := buf.String()

	mustContain(t, out, "ClaudeDock 会话信息面板")
	mustContain(t, out, "出口 IP")
	mustContain(t, out, "使用账号")
	mustContain(t, out, "挂载模式")
}

func TestCenterPad(t *testing.T) {
	cases := []struct {
		s      string
		width  int
		expect int
	}{
		{"hello", 10, 10},
		{"hello world", 5, 11},
		{"", 5, 5},
		{"a", 3, 3},
	}
	for _, c := range cases {
		got := centerPad(c.s, c.width)
		if len(got) != c.expect {
			t.Errorf("centerPad(%q, %d) = %q (len=%d), want len=%d", c.s, c.width, got, len(got), c.expect)
		}
	}
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("输出应包含 %q，实际输出:\n%s", needle, haystack)
	}
}
