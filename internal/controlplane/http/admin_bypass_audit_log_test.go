package http

import (
	"context"
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// stubBypassAuditLogStore 是 ListBypassAuditLogByTarget 的最小 fake。
// rows 按 created_at DESC 预排序（Repository 实际行为）。
type stubBypassAuditLogStore struct {
	rows []repository.BypassAuditLog
	err  error
}

func (s *stubBypassAuditLogStore) ListBypassAuditLogByTarget(_ context.Context, _, _ string) ([]repository.BypassAuditLog, error) {
	if s.err != nil {
		return nil, s.err
	}
	// 返回副本，handler 不应修改 store 内部切片。
	cp := make([]repository.BypassAuditLog, len(s.rows))
	copy(cp, s.rows)
	return cp, nil
}

func newAuditHandler(rows []repository.BypassAuditLog) *AdminBypassAuditLogHandler {
	return NewAdminBypassAuditLogHandler(
		slog.Default(),
		&stubBypassAuditLogStore{rows: rows},
	)
}

// doAuditRequest 用 mux 注入 hostID 路径参数。
func doAuditRequest(t *testing.T, h *AdminBypassAuditLogHandler, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := nethttp.NewServeMux()
	mux.Handle("GET /v1/admin/hosts/{hostID}/bypass/audit-log", h.ListByHost())
	req := httptest.NewRequest(nethttp.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// genAuditRows 生成 n 条按 created_at DESC 排列的 audit row（每条间隔 1 秒）。
func genAuditRows(n int, base time.Time) []repository.BypassAuditLog {
	out := make([]repository.BypassAuditLog, n)
	for i := 0; i < n; i++ {
		out[i] = repository.BypassAuditLog{
			ID:         "log-" + strconv.Itoa(i),
			Action:     "apply",
			TargetKind: "host",
			CreatedAt:  base.Add(-time.Duration(i) * time.Second),
		}
	}
	return out
}

func TestAdminBypassAuditLogHandler(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("default limit=20 returns up to 20 rows, next_before set when full", func(t *testing.T) {
		rows := genAuditRows(50, now)
		h := newAuditHandler(rows)
		rec := doAuditRequest(t, h, "/v1/admin/hosts/h1/bypass/audit-log")
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp auditLogListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.AuditLog) != defaultAuditLogLimit {
			t.Errorf("expected %d rows, got %d", defaultAuditLogLimit, len(resp.AuditLog))
		}
		if resp.NextBefore == "" {
			t.Error("expected next_before to be set when limit fully consumed")
		}
	})

	t.Run("before filter returns only rows older than cursor", func(t *testing.T) {
		// 5 行：每行间隔 1 秒，row[0]=now, row[1]=now-1s, ..., row[4]=now-4s。
		rows := genAuditRows(5, now)
		h := newAuditHandler(rows)
		// cursor = now-2s，严格过滤 created_at < cursor，仅保留 row[3]、row[4]（共 2 条）。
		cursor := now.Add(-2 * time.Second).Format(time.RFC3339Nano)
		rec := doAuditRequest(t, h, "/v1/admin/hosts/h1/bypass/audit-log?before="+cursor)
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp auditLogListResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if len(resp.AuditLog) != 2 {
			t.Errorf("expected 2 rows after before-filter, got %d", len(resp.AuditLog))
		}
		// 不足 default limit=20，next_before 应为空串。
		if resp.NextBefore != "" {
			t.Errorf("expected empty next_before when result < limit, got %q", resp.NextBefore)
		}
	})

	t.Run("limit > 200 clamps to 200", func(t *testing.T) {
		// 准备 250 行，请求 limit=500，应只返回 200。
		rows := genAuditRows(250, now)
		h := newAuditHandler(rows)
		rec := doAuditRequest(t, h, "/v1/admin/hosts/h1/bypass/audit-log?limit=500")
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp auditLogListResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if len(resp.AuditLog) != maxAuditLogLimit {
			t.Errorf("expected clamp to %d, got %d", maxAuditLogLimit, len(resp.AuditLog))
		}
		if resp.NextBefore == "" {
			t.Error("expected next_before set when full clamp")
		}
	})

	t.Run("empty result returns audit_log=[] and empty next_before", func(t *testing.T) {
		h := newAuditHandler(nil)
		rec := doAuditRequest(t, h, "/v1/admin/hosts/h1/bypass/audit-log")
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		// 直接做字符串断言保证 audit_log 字段是 [] 而非 null。
		body := rec.Body.String()
		if !contains(body, `"audit_log":[]`) {
			t.Errorf("expected audit_log:[] in body, got: %s", body)
		}
		if !contains(body, `"next_before":""`) {
			t.Errorf("expected empty next_before in body, got: %s", body)
		}
	})

	t.Run("invalid limit returns 400", func(t *testing.T) {
		h := newAuditHandler(nil)
		rec := doAuditRequest(t, h, "/v1/admin/hosts/h1/bypass/audit-log?limit=abc")
		if rec.Code != nethttp.StatusBadRequest {
			t.Errorf("expected 400 for invalid limit, got %d", rec.Code)
		}
	})

	t.Run("invalid before format returns 400", func(t *testing.T) {
		h := newAuditHandler(nil)
		rec := doAuditRequest(t, h, "/v1/admin/hosts/h1/bypass/audit-log?before=not-a-date")
		if rec.Code != nethttp.StatusBadRequest {
			t.Errorf("expected 400 for invalid before, got %d", rec.Code)
		}
	})

	t.Run("limit < default but rows >= limit sets next_before", func(t *testing.T) {
		rows := genAuditRows(5, now)
		h := newAuditHandler(rows)
		rec := doAuditRequest(t, h, "/v1/admin/hosts/h1/bypass/audit-log?limit=3")
		if rec.Code != nethttp.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp auditLogListResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if len(resp.AuditLog) != 3 {
			t.Errorf("expected 3 rows, got %d", len(resp.AuditLog))
		}
		if resp.NextBefore == "" {
			t.Error("expected next_before set when len == limit")
		}
	})
}

// contains 是 strings.Contains 的极小替代（避免给测试再引一次 strings）。
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
