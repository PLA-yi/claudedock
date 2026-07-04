package claudedock

import (
	"strings"
	"testing"
	"time"
)

func TestRenderDisconnectStatus_Thresholds(t *testing.T) {
	cases := []struct {
		name         string
		d            time.Duration
		noColor      bool
		wantContains string
	}{
		{"under_1.5s_empty", 500 * time.Millisecond, false, ""},
		{"1.5s_to_8s_gray", 3 * time.Second, false, "\x1b[90m"},
		{"8s_to_30s_yellow", 10 * time.Second, false, "\x1b[33m"},
		{"over_30s_red", 45 * time.Second, false, "\x1b[31m"},
		{"no_color_30s_text", 45 * time.Second, true, "网络已断"},
		{"no_color_30s_check", 45 * time.Second, true, "✗"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderDisconnectStatus(tc.d, tc.noColor)
			if tc.wantContains == "" {
				if got != "" {
					t.Errorf("期望空字符串，得 %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantContains) {
				t.Errorf("renderDisconnectStatus(%v, %v) = %q, want contain %q", tc.d, tc.noColor, got, tc.wantContains)
			}
			if tc.noColor && strings.Contains(got, "\x1b[") {
				t.Errorf("noColor=true 不应含 ANSI escape，得 %q", got)
			}
		})
	}
}

func TestReconnector_TriggerDropsExtras(t *testing.T) {
	r := NewReconnector(SSHConfig{}, nil, nil, nil, true)
	for i := 0; i < 100; i++ {
		r.Trigger()
	}
	if len(r.triggerCh) != 1 {
		t.Errorf("期望 triggerCh 长度 = 1，得 %d", len(r.triggerCh))
	}
}

func TestReconnector_ExceededFastRetryBudget_60sWindow(t *testing.T) {
	r := NewReconnector(SSHConfig{}, nil, nil, nil, true)
	for i := 0; i < 5; i++ {
		r.recordFastRetry()
		if r.exceededFastRetryBudget() {
			t.Fatalf("第 %d 次不应触发兜底", i+1)
		}
	}
	r.recordFastRetry()
	if !r.exceededFastRetryBudget() {
		t.Fatal("第 6 次应触发 fastRetry 兜底")
	}
}

func TestReconnector_FastRetryWindowResetsAfter60s(t *testing.T) {
	r := NewReconnector(SSHConfig{}, nil, nil, nil, true)
	r.recordFastRetry()
	r.fastRetryWindow = time.Now().Add(-65 * time.Second)
	r.recordFastRetry()
	if r.fastRetryCount != 1 {
		t.Errorf("窗口重置后期望 fastRetryCount=1，得 %d", r.fastRetryCount)
	}
}

func TestBackoffSeq(t *testing.T) {
	expected := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		30 * time.Second,
	}
	if len(backoffSeq) != len(expected) {
		t.Fatalf("期望 backoffSeq 长度 %d，得 %d", len(expected), len(backoffSeq))
	}
	for i := range expected {
		if backoffSeq[i] != expected[i] {
			t.Errorf("backoffSeq[%d] = %v, want %v", i, backoffSeq[i], expected[i])
		}
	}
}

func TestReconnector_State_Initial(t *testing.T) {
	r := NewReconnector(SSHConfig{}, nil, nil, nil, true)
	if r.State() != StateConnected {
		t.Errorf("期望初始 State == StateConnected，得 %v", r.State())
	}
	if r.StateAddr() == nil {
		t.Error("StateAddr 不应返回 nil")
	}
}

func TestFormatGiveUpMessage_TwoElements(t *testing.T) {
	got := FormatGiveUpMessage(5, 60*time.Second)
	if !strings.Contains(got, "重连失败") {
		t.Errorf("FormatGiveUpMessage 应包含中文原因 '重连失败'，得 %q", got)
	}
	if !strings.Contains(got, "请检查网络后重新运行 claudedock") {
		t.Errorf("FormatGiveUpMessage 应包含中文下一步建议，得 %q", got)
	}
	if !strings.Contains(got, "[NET_RECONNECT_GAVE_UP]") {
		t.Errorf("FormatGiveUpMessage 应带 [NET_RECONNECT_GAVE_UP] 标签，得 %q", got)
	}
}
