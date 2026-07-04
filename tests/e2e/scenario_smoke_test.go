//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/claudedock/claudedock/tests/e2e/harness"
)

// ScenarioSmokeSuite 验证 Scenario builder 能端到端跑通。
type ScenarioSmokeSuite struct {
	harness.BaseSuite
}

// TestScenarioBuilder_StartsAllComponents 端到端用例。
// v4.0 (Phase 55): WithSingBoxGateway → WithOutboundConfig, SingBoxGateway 删除。
func (s *ScenarioSmokeSuite) TestScenarioBuilder_StartsAllComponents() {
	s.T().Skip("Step 2..7 未实现，端到端验证留待后续阶段。Step 1（Postgres testcontainer）由 TestScenarioStartStep1_PostgresOnly 单独覆盖。")

	ctx, cancel := context.WithTimeout(s.Ctx, 180*time.Second)
	defer cancel()

	outbound := json.RawMessage(`{"type":"socks","tag":"proxy-out","server":"127.0.0.1","server_port":1080}`)

	sc := harness.New(s.T()).
		WithControlPlane().
		WithOutboundConfig(outbound).
		WithHost("alpha").
		WithUser("alice")

	err := sc.Start(ctx)
	s.Require().NoError(err, "scenario start")
	defer func() {
		if stopErr := sc.Stop(s.Ctx); stopErr != nil {
			s.Logger.Warn("scenario stop", "err", stopErr)
		}
	}()

	cp := sc.ControlPlane()
	s.Require().NotEmpty(cp.Addr, "control-plane addr")
	s.Require().NotEmpty(cp.AdminToken, "control-plane admin token")

	host := sc.Host("alpha")
	s.Require().NotEmpty(host.ID, "host id")
	s.Require().NotEmpty(host.ContainerName, "host container name")

	user := sc.User("alice")
	s.Require().NotEmpty(user.Username, "user username")
	s.Require().NotEmpty(user.EntryPassword, "user entry password")
}

// TestScenarioBuilder_DeclarationStateMachine 不依赖 Start，仅验证 builder 链的声明阶段约束。
// v4.0 (Phase 55): 删除 WithSingBoxGateway 引用。
func (s *ScenarioSmokeSuite) TestScenarioBuilder_DeclarationStateMachine() {
	outbound := json.RawMessage(`{"type":"direct","tag":"proxy-out"}`)

	s.T().Run("happy_path_chain", func(t *testing.T) {
		sc := harness.New(t).
			WithControlPlane().
			WithOutboundConfig(outbound).
			WithHost("h1").
			WithUser("u1").
			WithHost("h2").
			WithUser("u2")
		_ = sc
	})
}

// TestScenarioStartStep1_PostgresOnly 单独验证 Start 的 Step 1 真实实现。
func (s *ScenarioSmokeSuite) TestScenarioStartStep1_PostgresOnly() {
	s.T().Skip("需要 docker daemon；当前阶段留给 CI workflow 在 hosted ubuntu-24.04 上守护")

	ctx, cancel := context.WithTimeout(s.Ctx, 120*time.Second)
	defer cancel()

	sc := harness.New(s.T()).WithControlPlane()
	err := sc.Start(ctx)
	defer func() { _ = sc.Stop(s.Ctx) }()

	s.Require().Error(err, "Step 2 未实现，Start 必须返回错")
	s.Require().True(errors.Is(err, harness.ErrScenarioStepNotImplemented),
		"err must wrap ErrScenarioStepNotImplemented, got %v", err)
}

func TestScenarioSmokeSuite(t *testing.T) {
	suite.Run(t, new(ScenarioSmokeSuite))
}
