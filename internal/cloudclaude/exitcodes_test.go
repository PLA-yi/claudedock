package cloudclaude

import "testing"

// allExitCodes 返回所有导出常量的 (name, value) map。
// 添加新常量时同步更新此 helper（Go 没有常量反射）。
func allExitCodes() map[string]int {
	return map[string]int{
		"ExitOK":               ExitOK,
		"ExitAuthFailed":       ExitAuthFailed,
		"ExitNetworkError":     ExitNetworkError,
		"ExitTimeout":          ExitTimeout,
		"ExitConfigError":      ExitConfigError,
		"ExitInternalError":    ExitInternalError,
		"ExitOAuthNotFound":    ExitOAuthNotFound,
		"ExitOAuthExpired":     ExitOAuthExpired,
		"ExitMountForceFailed": ExitMountForceFailed,
	}
}

func Test_ExitCodes_Unique(t *testing.T) {
	seen := map[int]string{}
	for name, val := range allExitCodes() {
		if existing, ok := seen[val]; ok {
			t.Fatalf("duplicate exit code value %d: %s and %s", val, existing, name)
		}
		seen[val] = name
	}
}

func Test_ExitCodes_PosixLimit(t *testing.T) {
	for name, val := range allExitCodes() {
		if val < 0 || val > 125 {
			t.Errorf("%s = %d, must be in [0, 125] (POSIX shell limit)", name, val)
		}
	}
}

func Test_ExitCodes_V2Compat(t *testing.T) {
	cases := map[string]int{
		"ExitOK":            0,
		"ExitAuthFailed":    1,
		"ExitNetworkError":  2,
		"ExitTimeout":       3,
		"ExitConfigError":   4,
		"ExitInternalError": 5,
	}
	got := allExitCodes()
	for name, want := range cases {
		if got[name] != want {
			t.Errorf("%s = %d, want %d (v2.0 cmd/cloud-claude/main.go compat)",
				name, got[name], want)
		}
	}
}

func Test_ExitCodes_NewCodesNotConflictV2(t *testing.T) {
	v2 := map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true, 5: true}
	newCodes := map[string]int{
		"ExitOAuthNotFound":    ExitOAuthNotFound,
		"ExitOAuthExpired":     ExitOAuthExpired,
		"ExitMountForceFailed": ExitMountForceFailed,
	}
	for name, val := range newCodes {
		if v2[val] {
			t.Errorf("%s = %d collides with v2.0 0-5 range", name, val)
		}
	}
}

func Test_ExitCodes_NamesPresent(t *testing.T) {
	if ExitOAuthNotFound == 0 || ExitOAuthExpired == 0 || ExitMountForceFailed == 0 {
		t.Fatal("new exit codes must be non-zero")
	}
}
