//go:build e2e && linux

// killswitch_01_sigkill_timing_test.go 是 Phase 50 Plan 01 / KILL-01 的主用例：
//
//   - 入口：基线 worker `curl https://ifconfig.io` 必须 exit 0；
//   - 后台启 host eth0 tcpdump（BPF：src worker and not dst gateway）；
//   - `docker kill --signal=KILL <gateway>` 并记 wall-clock；
//   - 容器内立即跑 `curl --max-time 3 <url>`，期望非 0 退出；
//   - tcpdump 退出后包数必须 0；
//   - ClassifyStressResult("KILL-01", ...) 合成裁决，
//     verdict != StressVerdictPass → t.Fatalf。
//
// 与 Phase 48 既有 killswitch_singbox_crash_test.go 互不替代：MVS-09 守护
// 「行为不变量」，KILL-01 在同一故障注入下额外锁「timing ≤ 3000ms」。

package killswitch_stress

import (
	"context"
	"testing"
	"time"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"
)

func TestKillSwitch_01_SigkillTiming(t *testing.T) {
	g, skip := StartStressGolden(t)
	if skip {
		return
	}
	EnsureDumper(t, g)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const probeURL = "https://ifconfig.io"

	baselineExit, err := g.ProbeOutboundFromUser(ctx, probeURL, 5*time.Second)
	if err != nil {
		t.Skipf("baseline probe unavailable (worker container handle): %v", err)
		return
	}
	if baselineExit != 0 {
		t.Skipf("baseline egress not working (exit=%d); 避免外网抖动 false-fail", baselineExit)
		return
	}

	workerName, err := workerInspectName(ctx, g)
	if err != nil {
		t.Skipf("worker container name unavailable: %v", err)
		return
	}
	gatewayName, err := gatewayInspectName(ctx, g)
	if err != nil {
		t.Skipf("gateway container name unavailable: %v", err)
		return
	}

	workerIP, err := g.InspectContainerIPv4(ctx, workerName, "")
	if err != nil {
		t.Skipf("worker container ipv4 not available: %v", err)
		return
	}
	gatewayIP := g.Gateway.GatewayIP
	if gatewayIP == "" {
		var ipErr error
		gatewayIP, ipErr = g.InspectContainerIPv4(ctx, gatewayName, "")
		if ipErr != nil {
			t.Skipf("gateway ipv4 not available: %v", ipErr)
			return
		}
	}

	bpf := "src host " + workerIP + " and not dst host " + gatewayIP

	type tcpdumpResult struct {
		packets int
		err     error
	}
	dumpCh := make(chan tcpdumpResult, 1)
	go func() {
		packets, dErr := g.TcpdumpOnHostEth0(ctx, bpf, 5, e2e.KillswitchTimingContract.TcpdumpWindow)
		dumpCh <- tcpdumpResult{packets: packets, err: dErr}
	}()

	contract := e2e.KillswitchStressContract["KILL-01"]
	start := time.Now()
	if err := g.KillGateway(ctx); err != nil {
		t.Fatalf("kill gateway: %v", err)
	}
	probeTimeout := time.Duration(contract.MaxDisconnectMs) * time.Millisecond
	probeExit, probeErr := g.ProbeOutboundFromUser(ctx, probeURL, probeTimeout)
	elapsedMs := int(time.Since(start).Milliseconds())
	if probeErr != nil {
		t.Fatalf("probe outbound after kill: %v", probeErr)
	}

	var td tcpdumpResult
	select {
	case td = <-dumpCh:
	case <-ctx.Done():
		t.Fatalf("tcpdump goroutine did not finish before ctx deadline: %v", ctx.Err())
	}

	if td.err != nil {
		t.Logf("tcpdump sidecar reported err (host eth0 抓包不可用): %v", td.err)
		t.Skipf("host eth0 tcpdump oracle unavailable; deferred-to-CI (hosted ubuntu-24.04 with sudo)")
		return
	}

	verdict, reason := e2e.ClassifyStressResult("KILL-01", e2e.StressEvidence{
		ProbeExitCode: probeExit,
		LeakedPackets: td.packets,
		ElapsedMs:     elapsedMs,
	})
	t.Logf("KILL-01 verdict=%s reason=%q elapsed=%dms probeExit=%d packets=%d worker=%s gateway=%s bpf=%q",
		verdict, reason, elapsedMs, probeExit, td.packets, workerIP, gatewayIP, bpf)
	if verdict != e2e.StressVerdictPass {
		t.Fatalf("KILL-01 fail: verdict=%s reason=%s elapsed=%dms probeExit=%d packets=%d",
			verdict, reason, elapsedMs, probeExit, td.packets)
	}
}
