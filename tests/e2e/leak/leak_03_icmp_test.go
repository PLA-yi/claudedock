//go:build e2e && linux

// leak_03_icmp_test.go 是 Phase 49 LEAK-03 的 e2e 主用例：
//
//   - 容器内 `ping -c 1 -W 3 8.8.8.8` 必须**非 0** 退出（Blocked=true）。
//   - host eth0 抓包 BPF：`icmp and dst host 8.8.8.8 and src host <workerIP>`
//     必须 0 包。

package leak

import (
	"context"
	"testing"
	"time"

	e2e "github.com/claudedock/claudedock/tests/e2e"
)

func TestLeak_03_ICMP_BlockedByHostEth0(t *testing.T) {
	g, skip := StartLeakGolden(t)
	if skip {
		return
	}
	EnsureLeakWorkerTools(t, g)
	EnsureDumper(t, g)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	workerName, err := workerInspectName(ctx, g)
	if err != nil {
		t.Skipf("worker container name unavailable: %v", err)
		return
	}
	workerIP, err := g.InspectContainerIPv4(ctx, workerName, "")
	if err != nil {
		t.Skipf("worker container ipv4 not available: %v", err)
		return
	}

	bpf := "icmp and dst host 8.8.8.8 and src host " + workerIP

	type tcpdumpResult struct {
		packets int
		err     error
	}
	dumpCh := make(chan tcpdumpResult, 1)
	go func() {
		packets, dErr := g.TcpdumpOnHostEth0(ctx, bpf, 5, 5*time.Second)
		dumpCh <- tcpdumpResult{packets: packets, err: dErr}
	}()

	pingRes, err := g.PingICMP(ctx, "8.8.8.8")
	if err != nil {
		t.Fatalf("ping icmp: %v", err)
	}

	var tdRes tcpdumpResult
	select {
	case tdRes = <-dumpCh:
	case <-ctx.Done():
		t.Fatalf("tcpdump goroutine did not finish before ctx deadline: %v", ctx.Err())
	}

	if tdRes.err != nil {
		t.Logf("tcpdump sidecar reported err: %v", tdRes.err)
		t.Skipf("host eth0 tcpdump oracle unavailable; deferred-to-CI")
		return
	}

	verdict := e2e.ClassifyLeakProbe(pingRes, true)
	t.Logf("LEAK-03 verdict=%s blocked=%v reason=%q exit=%d packets=%d worker=%s",
		verdict, pingRes.Blocked, pingRes.Reason, pingRes.ExitCode, tdRes.packets, workerIP)

	if tdRes.packets > 0 {
		t.Fatalf("LEAK-03 host eth0 抓到 %d 个 ICMP→8.8.8.8 包，ping 直连泄漏",
			tdRes.packets)
	}
	if !pingRes.Blocked {
		t.Fatalf("LEAK-03 ping 未被阻断（Reason=%q）", pingRes.Reason)
	}
}
