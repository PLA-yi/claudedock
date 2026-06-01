//go:build e2e && linux

// egress_ip_binding_test.go 是 Phase 47 Plan 02 / MVS-07 的 e2e 主用例：
//
//   - 创建 host A（GoldenPath 默认拓扑提供）+ host B（直接 DB INSERT，跳过 admin
//     API 流程上的 entry credential 生成）；
//   - 创建一个 egress_ip X 并经 POST /v1/admin/bindings 绑给 A，确认成功（201）；
//   - 再次 POST /v1/admin/bindings 把 X 绑给 B；
//   - 断言：4xx + error message 含 "already bound" 子串 + A 的原绑定不被破坏。
//
// 当前源码（admin_bindings.go::Bind + host_egress_bindings 表 UNIQUE 约束仅
// 覆盖 (host_id, egress_ip_id) 复合键）**不**满足 MVS-07 双绑互斥语义；本
// 测试写「期望行为」，在 Linux runner 上跑红，作为 backend 缺口的真实证据。
// 详见 47-02-SUMMARY.md ROADMAP 偏差节。

package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestEgressIPBinding_DoubleBindExcluded 验证 MVS-07「同一 egress IP 第二次绑定被拒绝」。
func TestEgressIPBinding_DoubleBindExcluded(t *testing.T) {
	g := StartGoldenPath(t)
	if g == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if g.Host == nil || g.Host.ID == "" {
		t.Skipf("golden path host A not yet populated (scenario step 7 未实现)")
		return
	}

	cp := g.Scenario.ControlPlane()
	if cp == nil || cp.DBURL == "" {
		t.Skipf("control plane DBURL empty; scenario step 1 未跑通")
		return
	}

	hostAID := g.Host.ID

	// 1. 准备：DB 直接 INSERT 一个 stopped host B + 一个 egress_ip X。
	//    用 pgx 直查避免依赖完整 admin POST hosts 路径（鉴权 / 异步任务）。
	hostBID, egressIPID, err := setupEgressBindingFixture(ctx, cp.DBURL, g.User.ID)
	if err != nil {
		t.Fatalf("setup fixture: %v", err)
	}

	// 2. 第一次绑定：A ← X，期望 201。
	resp1, err := g.PostBindEgressIP(ctx, hostAID, egressIPID)
	if err != nil {
		t.Fatalf("first bind: %v", err)
	}
	if resp1.Status != 201 {
		t.Fatalf("first bind: status=%d body=%s (want 201)", resp1.Status, string(resp1.RawBody))
	}

	// 3. 第二次绑定：B ← X，期望被拒绝。
	resp2, err := g.PostBindEgressIP(ctx, hostBID, egressIPID)
	if err != nil {
		t.Fatalf("second bind: %v", err)
	}

	// 断言 1：status 在 4xx 范围（理想 409，但 backend 当前可能返回 500 或
	//        甚至 201 —— 后者表示业务约束完全缺失）。
	switch {
	case resp2.Status == EgressIPDoubleBindContract.WantStatus:
		t.Logf("PASS: status=%d matches ideal contract", resp2.Status)
	case resp2.Status >= 400 && resp2.Status < 500:
		t.Logf("PARTIAL PASS: status=%d is 4xx but not exactly %d; record as backend hardening item",
			resp2.Status, EgressIPDoubleBindContract.WantStatus)
	case resp2.Status >= 500:
		t.Errorf("BACKEND GAP: status=%d (5xx) on double-bind; "+
			"admin_bindings.go::Bind 缺 pre-check，应改成 409 + already bound 错误消息; "+
			"body=%s", resp2.Status, string(resp2.RawBody))
	default:
		t.Fatalf("BACKEND GAP: status=%d (2xx) on double-bind: 双绑互斥完全缺失, body=%s",
			resp2.Status, string(resp2.RawBody))
	}

	// 断言 2：error message 含 already bound / egress / bound 任一子串（容忍中英文）。
	if resp2.Status >= 400 {
		msg := strings.ToLower(resp2.ErrorMessage)
		switch {
		case strings.Contains(msg, "already bound"),
			strings.Contains(msg, "已绑定"),
			strings.Contains(msg, "egress"):
			t.Logf("PASS: error message contains expected substring: %q", resp2.ErrorMessage)
		default:
			t.Errorf("error message lacks egress/bound hint: %q (raw=%s)",
				resp2.ErrorMessage, string(resp2.RawBody))
		}
	}

	// 断言 3：A 原绑定仍存在（不允许 backend 在错误处理中把 A 也回滚）。
	exists, err := g.QueryBindingExists(ctx, hostAID, egressIPID)
	if err != nil {
		t.Fatalf("query A binding: %v", err)
	}
	if !exists {
		t.Fatalf("BACKEND GAP: hostA(%s) ← egress(%s) 原绑定被错误回滚",
			hostAID, egressIPID)
	}

	// 断言 4：B 没有意外被写入绑定（不允许 backend 在拒绝路径里依然落 DB 行）。
	bExists, err := g.QueryBindingExists(ctx, hostBID, egressIPID)
	if err != nil {
		t.Fatalf("query B binding: %v", err)
	}
	if bExists && resp2.Status >= 400 {
		t.Errorf("BACKEND GAP: hostB(%s) ← egress(%s) 在拒绝响应下仍落了 binding 行",
			hostBID, egressIPID)
	}
}

// setupEgressBindingFixture 直接 INSERT 一个 stopped host B + 一个 egress_ip。
//
// 不走 admin API：admin POST /v1/admin/hosts 会触发异步 ensure-image / create
// container 任务，e2e fixture 流程引入额外不确定性；本测试只关心 binding handler
// 行为，故 DB 直插一行 status='stopped' 的 host 即可。
func setupEgressBindingFixture(ctx context.Context, dbURL, userID string) (hostBID, egressIPID string, err error) {
	conn, err := sql.Open("sqlite", dbURL)
	if err != nil {
		return "", "", fmt.Errorf("connect db: %w", err)
	}
	defer conn.Close()

	// host B：复用 hosts 表 schema（参见 0001_initial.sql）；只填必需字段。
	if err := conn.QueryRowContext(ctx, `
		INSERT INTO hosts (user_id, status)
		VALUES (?1, 'stopped')
		RETURNING id
	`, userID).Scan(&hostBID); err != nil {
		return "", "", fmt.Errorf("insert host B: %w", err)
	}

	// egress_ip X：label / ip_address 都用 e2e- 前缀避免与既有 fixture 冲突。
	if err := conn.QueryRowContext(ctx, `
		INSERT INTO egress_ips (label, ip_address, provider, status)
		VALUES (?1, ?2, 'manual', 'available')
		RETURNING id
	`, "e2e-mvs07-"+hostBID[:8], "192.0.2.47").Scan(&egressIPID); err != nil {
		return "", "", fmt.Errorf("insert egress_ip: %w", err)
	}
	return hostBID, egressIPID, nil
}
