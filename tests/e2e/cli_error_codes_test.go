//go:build e2e && linux

// cli_error_codes_test.go 是 MVS-05 CLI 错误码契约 e2e 用例。
//
// 验证：bootstrap.sh 在 4 类错误场景下输出锁定的 exit code 与 stderr 关键字。
//
// 与 ROADMAP 偏差：ROADMAP 描述「真实 claudedock binary 触发各场景」，但
// cmd/claudedock/main.go 实际只定义 exit 1-5；错误码 10-13 由 bootstrap.sh
// 在 case "$error_code" 分支映射。CONTEXT §Area 3「以源码为准」原则下，本
// 用例以 bootstrap.sh 为被测 binary，详见 helpers.go BootstrapExitCodeContract
// 注释 + 46-VERIFICATION.md。

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/claudedock/claudedock/tests/e2e/harness"
)

type CLIErrorCodesSuite struct {
	harness.BaseSuite
	GP *GoldenPath
}

func (s *CLIErrorCodesSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.GP = StartGoldenPath(s.T())
	if s.GP == nil {
		return
	}
	s.SetArtifactDumper(harness.NewArtifactDumper(s.GP.Scenario, ""))
	if err := SeedBootstrapErrorFixtures(s.Ctx, s.GP); err != nil {
		s.T().Fatalf("seed error-codes fixtures: %v", err)
	}
}

// TestCLIErrorCodes_Contract table-driven 跑 4 个错误场景。
// 每个场景断言 exit code 与 stderr 关键字。
func (s *CLIErrorCodesSuite) TestCLIErrorCodes_Contract() {
	if s.GP == nil {
		s.T().Skip("golden path not started; deferred to Linux CI")
		return
	}

	for _, tc := range CLIErrorCases {
		s.Run(tc.Name, func() {
			ctx, cancel := context.WithTimeout(s.Ctx, 30*time.Second)
			defer cancel()

			env := []string{
				"BOOTSTRAP_API=" + s.GP.ControlPlaneURL,
				"POLL_INTERVAL=1",
				"POLL_TIMEOUT=5",
				"PATH=/usr/local/bin:/usr/bin:/bin",
			}
			stdin := tc.Username + "\n" + tc.Password + "\n"

			code, stdout, stderr, err := RunBootstrapScript(ctx, s.GP.BootstrapScript, env, stdin)
			if err != nil {
				s.T().Fatalf("run bootstrap script: %v", err)
			}
			combined := stdout + "\n" + stderr

			s.Require().Equalf(tc.WantExitCode, code,
				"case=%s exit code mismatch; stdout=%s stderr=%s",
				tc.Name, stdout, stderr)
			s.Require().Truef(strings.Contains(combined, tc.WantStderrContains),
				"case=%s expected substring %q not found; combined=%s",
				tc.Name, tc.WantStderrContains, combined)
		})
	}
}

func TestCLIErrorCodesSuite(t *testing.T) {
	suite.Run(t, new(CLIErrorCodesSuite))
}
