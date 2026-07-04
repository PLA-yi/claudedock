//go:build e2e && linux

// leak_05_imds_test.go 是 Phase 49 LEAK-05 的 e2e 主用例：
//
//   - 容器内 `curl --max-time 3 http://169.254.169.254/latest/meta-data/` 必须失败。
//   - 容器内 `curl --max-time 3 http://169.254.170.2/v2/credentials/x` 必须失败。
//   - host eth0 抓包 BPF：`dst net 169.254.0.0/16 and src host <workerIP>` 必须 0 包。

package leak

import (
	"context"
	"testing"
	"time"

	e2e "github.com/claudedock/claudedock/tests/e2e"
)

func TestLeak_05_IMDS_BlockedByHostEth0(t *testing.T) {
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

	bpf := "dst net 169.254.0.0/16 and src host " + workerIP

	type tcpdumpResult struct {
		packets int
		err     error
	}
	dumpCh := make(chan tcpdumpResult, 1)
	go func() {
		packets, dErr := g.TcpdumpOnHostEth0(ctx, bpf, 5, 6*time.Second)
		dumpCh <- tcpdumpResult{packets: packets, err: dErr}
	}()

	imdsURLs := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://169.254.170.2/v2/credentials/x",
	}
	for _, url := range imdsURLs {
		res, err := g.CurlIMDS(ctx, url)
		if err != nil {
			t.Fatalf("curl IMDS %s: %v", url, err)
		}
		verdict := e2e.ClassifyLeakProbe(res, true)
		t.Logf("LEAK-05 url=%s verdict=%s blocked=%v reason=%q",
			url, verdict, res.Blocked, res.Reason)
		if !res.Blocked {
			t.Fatalf("LEAK-05 IMDS %s 返回 200（Reason=%q），实锤泄漏", url, res.Reason)
		}
	}

	var tdRes tcpdumpResult
	select {
	case tdRes = <-dumpCh:
	case <-ctx.Done():
		t.Fatalf("tcpdump goroutine did not finish before ctx deadline: %v", ctx.Err())
	}

	if tdRes.err != nil {
		t.Logf("tcpdump sidecar err: %v", tdRes.err)
		t.Skipf("host eth0 tcpdump oracle unavailable; deferred-to-CI")
		return
	}
	if tdRes.packets > 0 {
		t.Fatalf("LEAK-05 host eth0 抓到 %d 个 169.254.0.0/16 包，IMDS 直连泄漏 (worker=%s)",
			tdRes.packets, workerIP)
	}
}
