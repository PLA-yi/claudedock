//go:build e2e && linux

// expiry_test.go 是 Phase 47 Plan 01 / MVS-06 的 e2e 主用例：
//
//   - 给 GoldenPath 的 alice 用户写一个过去的 expires_at；
//   - 等 ExpiryScanner（EXPIRY_SCAN_INTERVAL=1s）触发；
//   - 断言 alice 名下的运行 host 被 stop（admin API status=stopped）；
//   - 断言 events 表里出现 type=host.stop.expired，metadata 含 reason=user expired。
//
// darwin 上不参与编译；本文件依赖的 GoldenPath / SimulateExpiry / Scenario
// 真实拓扑均要求 Linux + docker + 完整 Step 2..7 实现。

package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/claudedock/claudedock/tests/e2e/harness"
)

// TestExpiry_AutoStop_GoldenPath 验证 MVS-06「到期容器自动停止 + 审计事件入库」。
//
// 流程：
//  1. StartGoldenPath（含 EXPIRY_SCAN_INTERVAL=1s 覆写）
//  2. SimulateExpiry(alice.UserID, waitForTick=true) → user.expired 事件出现
//  3. WaitFor 30s 内 admin API GET /v1/admin/hosts/{X} 返回 status="stopped"
//  4. WaitFor 30s 内 events 表出现 host.stop.expired (host_id=X)
//
// 总 timeout 90s（30s scanner + 30s 容器停止 + 30s 事件落盘缓冲）。
func TestExpiry_AutoStop_GoldenPath(t *testing.T) {
	g := StartGoldenPath(t)
	if g == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if g.User == nil || g.User.ID == "" {
		t.Skipf("golden path user not yet populated (scenario step 3 未实现)")
		return
	}
	if g.Host == nil || g.Host.ID == "" {
		t.Skipf("golden path host not yet populated (scenario step 7 未实现)")
		return
	}

	// 1. 触发到期 + 等 scanner tick
	if err := g.SimulateExpiry(ctx, g.User.ID, true); err != nil {
		t.Fatalf("SimulateExpiry: %v", err)
	}

	// 2. host 状态进入 stopped
	if err := waitHostStatus(ctx, g, g.Host.ID, "stopped", 30*time.Second); err != nil {
		t.Fatalf("wait host stopped: %v", err)
	}

	// 3. 审计事件 host.stop.expired 入库
	if err := waitExpiryEvent(ctx, g, g.Host.ID, 30*time.Second); err != nil {
		t.Fatalf("wait expiry event: %v", err)
	}
}

// adminHostDetailBody 复用 admin_hosts.go::Get 的响应结构（最小子集）。
type adminHostDetailBody struct {
	Host struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"host"`
}

func waitHostStatus(ctx context.Context, g *GoldenPath, hostID, expected string, timeout time.Duration) error {
	token, err := g.AdminLogin(ctx)
	if err != nil {
		return fmt.Errorf("admin login: %w", err)
	}
	url := strings.TrimRight(g.ControlPlaneURL, "/") + "/v1/admin/hosts/" + hostID
	client := disableKeepAliveClient(2 * time.Second)

	return harness.WaitFor(ctx, fmt.Sprintf("host_status=%s", expected),
		func(ctx context.Context) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return fmt.Errorf("build req: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("do: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("status %d body=%s", resp.StatusCode, string(body))
			}
			// admin Get 返回的是 adminHostDetailResponse（嵌入 HostDetail），
			// 这里只关心顶层 host.status；为了对结构演进有韧性，先尝试两种 schema。
			var parsed adminHostDetailBody
			if err := json.Unmarshal(body, &parsed); err == nil && parsed.Host.Status != "" {
				if parsed.Host.Status != expected {
					return fmt.Errorf("host status=%s want=%s", parsed.Host.Status, expected)
				}
				return nil
			}
			// Fallback：HostDetail 内嵌字段在顶层（部分 schema 版本如此）。
			var flat map[string]any
			if err := json.Unmarshal(body, &flat); err != nil {
				return fmt.Errorf("decode body: %w", err)
			}
			status, _ := flat["status"].(string)
			if status != expected {
				return fmt.Errorf("host status=%s want=%s (body=%s)", status, expected, string(body))
			}
			return nil
		},
		harness.WithTimeout(timeout),
		harness.WithPollInterval(500*time.Millisecond),
	)
}

// waitExpiryEvent 等 events 表里出现 host.stop.expired (host_id=X) 行，metadata 含 reason=user expired。
//
// 直接连 Postgres 而不是经 admin API（admin API 有分页 / 过滤参数演进风险）。
func waitExpiryEvent(ctx context.Context, g *GoldenPath, hostID string, timeout time.Duration) error {
	cp := g.Scenario.ControlPlane()
	if cp == nil || cp.DBURL == "" {
		return fmt.Errorf("control plane DBURL empty")
	}
	conn, err := sql.Open("sqlite", cp.DBURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer conn.Close()

	return harness.WaitFor(ctx, fmt.Sprintf("%s:%s", ExpiryEventType, hostID),
		func(ctx context.Context) error {
			var hits int
			row := conn.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM events
				 WHERE type = ?
				   AND host_id = ?
				   AND json_extract(metadata, '$.reason') = ?`,
				ExpiryEventType, hostID, "user expired",
			)
			if err := row.Scan(&hits); err != nil {
				return fmt.Errorf("scan: %w", err)
			}
			if hits == 0 {
				return fmt.Errorf("%s event not yet recorded for host=%s", ExpiryEventType, hostID)
			}
			return nil
		},
		harness.WithTimeout(timeout),
		harness.WithPollInterval(500*time.Millisecond),
	)
}
