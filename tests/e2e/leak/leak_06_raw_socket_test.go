//go:build e2e && linux

// leak_06_raw_socket_test.go 是 Phase 49 LEAK-06 的 e2e 主用例：
//
//   - 容器内 `python3 -c 'socket.socket(SOCK_RAW)'` 必须以 PermissionError 失败。
//
// 当前 worker.go 未显式 `--cap-drop NET_RAW`，docker 默认 capability 包含
// CAP_NET_RAW，故本用例**预期 fail**。fail 即明确指向 Phase 51 QUAL-06 修源码。
// 用 t.Errorf（非 Fatalf）让其它 LEAK 用例继续跑。

package leak

import (
	"context"
	"testing"
	"time"

	e2e "github.com/claudedock/claudedock/tests/e2e"
)

func TestLeak_06_RawSocket_PermissionDenied(t *testing.T) {
	g, skip := StartLeakGolden(t)
	if skip {
		return
	}
	EnsureLeakWorkerTools(t, g)
	EnsureDumper(t, g)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := g.TryRawSocket(ctx)
	if err != nil {
		t.Fatalf("try raw socket: %v", err)
	}
	verdict := e2e.ClassifyLeakProbe(res, true)
	t.Logf("LEAK-06 verdict=%s blocked=%v reason=%q exit=%d stderr_tail=%q",
		verdict, res.Blocked, res.Reason, res.ExitCode,
		tail(res.RawStderr, 200))

	if !res.Blocked {
		// 预期 fail：worker.go 当前未 --cap-drop NET_RAW。
		// 用 t.Errorf 不阻塞其它 LEAK 用例；VERIFICATION 标 backend GAP（Phase 51 QUAL-06）。
		t.Errorf(
			"LEAK-06 raw socket 创建成功（Reason=%q），证明 worker 仍带 CAP_NET_RAW。"+
				"backend GAP：internal/runtime/tasks/worker.go:217-218 仅 --cap-add NET_ADMIN/SYS_ADMIN，"+
				"未显式 --cap-drop NET_RAW；docker 默认 capability 集合含 NET_RAW。"+
				"修复方案见 Phase 51 QUAL-06。",
			res.Reason)
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
