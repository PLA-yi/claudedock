//go:build e2e && linux

// host_agent_recovery_test.go 是 Phase 47 Plan 03 / MVS-08 的 e2e 主用例：
//
//   - 入口：基线 healthy（/healthz checks.agent == "ok"）；
//   - `pkill -9 -f host-agent`（进程级，不杀容器）；
//   - 30s 内 /healthz checks.agent == "unreachable"（Unhealthy）；
//   - 60s 内 /healthz checks.agent 自动回到 "ok"（Healthy），全程无人工干预。
//
// ROADMAP / CONTEXT 草案曾写 GET /v1/admin/hosts/{X}/health，源码不存在该端点；
// 本测试以源码为准用 /healthz checks.agent。详见 47-03-SUMMARY.md。

package e2e

import (
	"context"
	"testing"
	"time"
)

// TestHostAgent_KillRecover_GoldenPath 验证 MVS-08「kill -9 host-agent 后自动恢复」。
func TestHostAgent_KillRecover_GoldenPath(t *testing.T) {
	if IsEmbeddedHostAgent() {
		t.Skip("HOST_AGENT_MODE=embedded：kill host-agent 等价于杀控制面，MVS-08 不适用")
		return
	}

	g := StartGoldenPath(t)
	if g == nil {
		return
	}

	// 总 timeout：30s 基线 healthy + 30s unhealthy + 60s recovery + 30s 缓冲。
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	// 1. 基线断言：/healthz checks.agent == "ok"（GoldenPath 启动后必须立刻就绪）。
	if err := g.WaitHostHealthStatus(ctx, HostHealthHealthy, 30*time.Second); err != nil {
		t.Fatalf("baseline health: %v", err)
	}

	// 2. 杀 host-agent 进程（不杀容器，让 supervisor 拉起）。
	if err := g.KillHostAgent(ctx); err != nil {
		t.Fatalf("kill host-agent: %v", err)
	}

	// 3. 30s 内进入 unhealthy。
	if err := g.WaitHostHealthStatus(ctx, HostHealthUnhealthy,
		HostHealthRecoveryContract.UnhealthyWithin); err != nil {
		t.Fatalf("wait unhealthy within %s: %v",
			HostHealthRecoveryContract.UnhealthyWithin, err)
	}

	// 4. 60s 内自动恢复 healthy（无主动 force resync 调用）。
	if err := g.WaitHostHealthStatus(ctx, HostHealthHealthy,
		HostHealthRecoveryContract.HealthyWithin); err != nil {
		t.Fatalf("wait recovered healthy within %s: %v (supervisor 可能未配置自动重启)",
			HostHealthRecoveryContract.HealthyWithin, err)
	}
}
