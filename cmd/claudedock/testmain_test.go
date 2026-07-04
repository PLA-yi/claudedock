package main

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain Phase 51 QUAL-08：接入 goleak.VerifyTestMain 拦截 goroutine 泄漏。
//
// IgnoreList 来源（首跑实测）：
//   - internal/broadcast.(*Hub).cleanupLoop：包级 init 启动的 SSE 清理 goroutine，
//     生命周期与进程绑定，由设计决定不退出（属于已知合法常驻 goroutine）。
//
// 其它已知第三方常驻 goroutine（pgx pool / sing-box 等）若后续被本测试集合触达，
// 同样在此追加 IgnoreTopFunction。
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/claudedock/claudedock/internal/broadcast.(*Hub).cleanupLoop"),
	)
}
