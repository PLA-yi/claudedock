package claudedock

import (
	"fmt"
	"testing"
	"time"
)

// Test_ParseExpiresAt 表驱动覆盖 D-22 三态 + JSON 解析容错（9 子用例）。
//
// 注：`secondsNotMilliseconds` 用例验证 v3.0 故意行为 — claude code 输出毫秒，
// 不做容错；如果调用方把秒级时间戳塞进来会被解读为「远过去」→ Expired。
func Test_ParseExpiresAt(t *testing.T) {
	now := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		raw       string
		wantState OAuthState
	}{
		{"valid", fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, now.Add(time.Hour).UnixMilli()), OAuthValid},
		{"expiringSoon3min", fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, now.Add(3*time.Minute).UnixMilli()), OAuthExpiringSoon},
		{"atBoundary5min", fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, now.Add(5*time.Minute).UnixMilli()), OAuthValid},
		{"expired", fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, now.Add(-time.Second).UnixMilli()), OAuthExpired},
		{"emptyInput", "", OAuthNotFound},
		{"malformedJSON", "{not json", OAuthNotFound},
		{"missingField", `{"foo":"bar"}`, OAuthNotFound},
		{"nestedMissing", `{"claudeAiOauth":{"accessToken":"x"}}`, OAuthNotFound},
		{"secondsNotMilliseconds", `{"claudeAiOauth":{"expiresAt":1700000000}}`, OAuthExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := parseExpiresAt(tc.raw, now)
			if s == nil {
				t.Fatalf("parseExpiresAt 返回 nil")
			}
			if s.State != tc.wantState {
				t.Errorf("State = %d, want %d", s.State, tc.wantState)
			}
		})
	}
}

// Test_ParseExpiresAt_ExpiringSoonMinutes 验证 ExpiringSoon 时 MinutesToExpire 字段填充。
func Test_ParseExpiresAt_ExpiringSoonMinutes(t *testing.T) {
	now := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	raw := fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, now.Add(3*time.Minute).UnixMilli())
	s := parseExpiresAt(raw, now)
	if s.State != OAuthExpiringSoon {
		t.Fatalf("State = %d, want OAuthExpiringSoon", s.State)
	}
	if s.MinutesToExpire < 1 || s.MinutesToExpire > 5 {
		t.Errorf("MinutesToExpire = %d, want 1..5", s.MinutesToExpire)
	}
	if s.ExpiresAt.IsZero() {
		t.Error("ExpiresAt 不应为 zero")
	}
}
