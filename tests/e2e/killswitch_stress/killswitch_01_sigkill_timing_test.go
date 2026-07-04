//go:build e2e && linux

// killswitch_01_sigkill_timing_test.go v4.0 (Phase 55):
// KILL-01: `docker exec <user> kill -9 $(pidof sing-box)` + timing ≤ 3000ms

package killswitch_stress

import (
	"context"
	"testing"
	"time"

	e2e "github.com/claudedock/claudedock/tests/e2e"
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
		t.Skipf("baseline probe unavailable: %v", err)
		return
	}
	if baselineExit != 0 {
		t.Skipf("baseline egress not working (exit=%d)", baselineExit)
		return
	}

	containerName, err := workerInspectName(ctx, g)
	if err != nil {
		t.Skipf("container name unavailable: %v", err)
		return
	}

	workerIP, err := g.InspectContainerIPv4(ctx, containerName, "")
	if err != nil {
		t.Skipf("container ipv4 not available: %v", err)
		return
	}

	// v4.0: 单容器，BPF 简化为 src host workerIP
	bpf := "src host " + workerIP

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
	if err := g.KillSingBox(ctx); err != nil {
		t.Fatalf("kill sing-box: %v", err)
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
		t.Fatalf("tcpdump goroutine did not finish: %v", ctx.Err())
	}

	if td.err != nil {
		t.Skipf("host eth0 tcpdump oracle unavailable; deferred-to-CI: %v", td.err)
		return
	}

	verdict, reason := e2e.ClassifyStressResult("KILL-01", e2e.StressEvidence{
		ProbeExitCode: probeExit,
		LeakedPackets: td.packets,
		ElapsedMs:     elapsedMs,
	})
	t.Logf("KILL-01 v4.0 verdict=%s reason=%q elapsed=%dms probeExit=%d packets=%d container=%s",
		verdict, reason, elapsedMs, probeExit, td.packets, workerIP)
	if verdict != e2e.StressVerdictPass {
		t.Fatalf("KILL-01 fail: verdict=%s reason=%s elapsed=%dms probeExit=%d packets=%d",
			verdict, reason, elapsedMs, probeExit, td.packets)
	}
}
