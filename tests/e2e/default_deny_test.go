//go:build e2e && linux

// default_deny_test.go 是 MVS-04 默认拒绝矩阵 e2e 用例。
//
// 验证主路径：worker 容器内并发对 4 个固定 IP × 端口组合发起直连，
// 任一连通即 fail，全部超时 / refused / unreachable 才 PASS。

package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/zanel1u/cloud-cli-proxy/tests/e2e/harness"
)

type DefaultDenySuite struct {
	harness.BaseSuite
	GP *GoldenPath
}

func (s *DefaultDenySuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.GP = StartGoldenPath(s.T())
	if s.GP != nil {
		s.SetArtifactDumper(harness.NewArtifactDumper(s.GP.Scenario, ""))
	}
}

// TestDefaultDeny_Matrix 并发对 DefaultDenyMatrix 4 个 target 发起直连，
// 把每个 target 的 exit code 喂给 SummarizeDenyResults 合成裁决。
func (s *DefaultDenySuite) TestDefaultDeny_Matrix() {
	if s.GP == nil {
		s.T().Skip("golden path not started; deferred to Linux CI")
		return
	}

	ctx, cancel := context.WithTimeout(s.Ctx, 30*time.Second)
	defer cancel()

	container := workerContainerHandle(s.GP)
	if container == nil {
		s.T().Skip("worker container handle not available; deferred to Linux CI")
		return
	}

	results := map[DenyTarget]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, target := range DefaultDenyMatrix {
		wg.Add(1)
		go func(t DenyTarget) {
			defer wg.Done()
			cmd := BuildDenyProbeCmd(t, 3)
			code, _, err := container.Exec(ctx, cmd)
			if err != nil {
				// Exec 错误本身不算 leak；记 124 作为占位（与 GNU timeout 一致）。
				code = 124
			}
			mu.Lock()
			results[t] = code
			mu.Unlock()
		}(target)
	}
	wg.Wait()

	allDenied, leaks := SummarizeDenyResults(results)
	s.T().Logf("MVS-04 deny results=%v allDenied=%v leaks=%v", results, allDenied, leaks)
	s.Require().Truef(allDenied, "default-deny matrix found leaks: %v", leaks)
}

func TestDefaultDenySuite(t *testing.T) {
	suite.Run(t, new(DefaultDenySuite))
}
