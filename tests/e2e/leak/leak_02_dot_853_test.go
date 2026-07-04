//go:build e2e && linux

// leak_02_dot_853_test.go 是 Phase 49 LEAK-02 的 e2e 主用例：
//
//   - 容器内 `kdig +tls @1.1.1.1 example.com`（缺 kdig 时 fallback openssl
//     s_client）必须 TLS 握手失败 / connect refused / timeout（Blocked=true）。
//   - host eth0 抓包 BPF：`tcp port 853 and dst host 1.1.1.1 and src host
//     <workerIP>` 必须 0 包。

package leak

import (
	"context"
	"testing"
	"time"

	e2e "github.com/claudedock/claudedock/tests/e2e"
)

func TestLeak_02_DoT853_BlockedByHostEth0(t *testing.T) {
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

	bpf := "tcp port 853 and dst host 1.1.1.1 and src host " + workerIP

	type tcpdumpResult struct {
		packets int
		err     error
	}
	dumpCh := make(chan tcpdumpResult, 1)
	go func() {
		packets, dErr := g.TcpdumpOnHostEth0(ctx, bpf, 5, 5*time.Second)
		dumpCh <- tcpdumpResult{packets: packets, err: dErr}
	}()

	dotRes, err := g.DigDoT(ctx, "1.1.1.1", "example.com")
	if err != nil {
		t.Fatalf("dig dot: %v", err)
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

	verdict := e2e.ClassifyLeakProbe(dotRes, true)
	t.Logf("LEAK-02 verdict=%s blocked=%v reason=%q exit=%d packets=%d worker=%s",
		verdict, dotRes.Blocked, dotRes.Reason, dotRes.ExitCode, tdRes.packets, workerIP)

	if tdRes.packets > 0 {
		t.Fatalf("LEAK-02 host eth0 抓到 %d 个 TCP/853→1.1.1.1 包，DoT 直连泄漏",
			tdRes.packets)
	}
	if !dotRes.Blocked {
		t.Fatalf("LEAK-02 DoT TLS 握手未被阻断（Reason=%q stderr 末尾=%q）",
			dotRes.Reason, lastChars(dotRes.RawStderr, 200))
	}
}

func lastChars(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
