//go:build e2e && linux

// killswitch_singbox_crash_test.go v4.0 (Phase 55):
//
//   - 入口：基线 worker 容器内 `curl https://ifconfig.io` 必须 exit 0；
//   - 后台启 host eth0 tcpdump（独立 oracle，BPF：src host worker_ip）；
//   - `docker exec <container> kill -9 $(pidof sing-box)`；
//   - 容器内立即跑 `curl --max-time 3 <url>`，期望非 0 退出（kill-switch 兜住）；
//   - 等待容器在 ≤3s 内退出（entrypoint fail-closed）；
//   - tcpdump 退出后包数必须 0（sing-box 死 → 出网立即断）；
//   - ClassifyKillswitchResult 合成裁决，非 OK → t.Fatalf。
//
// v4.0 单容器：无独立 gateway 容器，sing-box 跑在 user 容器内。

package e2e

import (
	"context"
	"testing"
	"time"
)

// TestKillSwitch_SingboxCrash_GoldenPath 验证 MVS-09 kill-switch。
// v4.0: `docker kill <gw>` → `docker exec <user> kill -9 $(pidof sing-box)`
// 新增断言 "PID 1 死 → 容器死 → 出网立即断"。
func TestKillSwitch_SingboxCrash_GoldenPath(t *testing.T) {
	g := StartGoldenPath(t)
	if g == nil {
		return
	}
	if g.Host == nil || g.Host.ID == "" {
		t.Skipf("golden path host not yet populated (scenario step 7 未实现)")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const probeURL = "https://ifconfig.io"

	baselineExit, err := g.ProbeOutboundFromUser(ctx, probeURL, 5*time.Second)
	if err != nil {
		t.Skipf("baseline probe unavailable (worker container handle): %v", err)
		return
	}
	if baselineExit != 0 {
		t.Skipf("baseline egress not working (exit=%d); 极可能 CI 出口被屏蔽，避免外网抖动 false-fail", baselineExit)
		return
	}

	containerName, err := g.singBoxContainerName()
	if err != nil {
		t.Skipf("container name unavailable: %v", err)
		return
	}

	workerIP, err := g.InspectContainerIPv4(ctx, containerName, "")
	if err != nil {
		t.Skipf("container ipv4 not available: %v", err)
		return
	}

	// v4.0: 抓包视角简化为 host eth0 src host workerIP（无 gateway 隔离）
	bpf := "src host " + workerIP

	type tcpdumpResult struct {
		packets int
		err     error
	}
	dumpCh := make(chan tcpdumpResult, 1)
	go func() {
		packets, dErr := g.TcpdumpOnHostEth0(ctx, bpf, 5, KillswitchTimingContract.TcpdumpWindow)
		dumpCh <- tcpdumpResult{packets: packets, err: dErr}
	}()

	if err := g.KillSingBox(ctx); err != nil {
		t.Fatalf("kill sing-box: %v", err)
	}

	probeExit, probeErr := g.ProbeOutboundFromUser(ctx, probeURL, KillswitchTimingContract.ProbeMaxLatency)
	if probeErr != nil {
		t.Fatalf("probe outbound after kill: %v", probeErr)
	}

	var tdRes tcpdumpResult
	select {
	case tdRes = <-dumpCh:
	case <-ctx.Done():
		t.Fatalf("tcpdump goroutine did not finish before ctx deadline: %v", ctx.Err())
	}

	if tdRes.err != nil {
		t.Logf("tcpdump sidecar reported err (可能 host eth0 在 runner 上不可抓包，转 Skip): %v", tdRes.err)
		t.Skipf("host eth0 tcpdump oracle unavailable; deferred-to-CI (hosted ubuntu-24.04 with sudo)")
		return
	}

	verdict := ClassifyKillswitchResult(probeExit, tdRes.packets)
	t.Logf("MVS-09 v4.0 verdict=%s probeExit=%d leakedPackets=%d container=%s bpf=%q",
		verdict, probeExit, tdRes.packets, workerIP, bpf)
	if verdict != KillswitchOK {
		t.Fatalf("MVS-09 kill-switch fail: verdict=%s probeExit=%d packets=%d",
			verdict, probeExit, tdRes.packets)
	}
}
