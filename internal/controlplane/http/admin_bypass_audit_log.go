package http

import (
	"context"
	"log/slog"
	nethttp "net/http"
	"strconv"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// AdminBypassAuditLogStore 是 Plan 46-02 Task 4 的最小存储接口。
// Repository 已实现 ListBypassAuditLogByTarget；handler 在内存里做
// before/limit cursor 过滤（当前体量小，按 created_at DESC 排序后即可截断）。
type AdminBypassAuditLogStore interface {
	ListBypassAuditLogByTarget(ctx context.Context, targetKind, targetID string) ([]repository.BypassAuditLog, error)
}

// 默认/最大 limit 常量便于测试断言。
const (
	defaultAuditLogLimit = 20
	maxAuditLogLimit     = 200
)

// AdminBypassAuditLogHandler 暴露 GET /v1/admin/hosts/{hostID}/bypass/audit-log。
type AdminBypassAuditLogHandler struct {
	logger *slog.Logger
	store  AdminBypassAuditLogStore
}

// NewAdminBypassAuditLogHandler 是构造函数；router/app.go wire 时传入。
func NewAdminBypassAuditLogHandler(logger *slog.Logger, store AdminBypassAuditLogStore) *AdminBypassAuditLogHandler {
	return &AdminBypassAuditLogHandler{logger: logger, store: store}
}

// auditLogListResponse 是 ListByHost 的响应体；next_before 在不足 limit 时为空串。
type auditLogListResponse struct {
	AuditLog   []repository.BypassAuditLog `json:"audit_log"`
	NextBefore string                      `json:"next_before"`
}

// ListByHost 处理 GET /v1/admin/hosts/{hostID}/bypass/audit-log。
//
//   - target_kind 默认为 "host"，可由 query param 覆盖（前端切到 preset/rule/binding 视图时复用）。
//   - target_id 默认为 path hostID，可由 query param 覆盖。
//   - limit 默认 20，最大 200（超出 clamp）。
//   - before 为 RFC3339 cursor，仅返回 created_at < before 的行；Repository 已按
//     created_at DESC 排序，handler 在内存里截断。
//   - next_before 取最后一行的 created_at（RFC3339Nano）；不足 limit 时返回空串表示没有下一页。
func (h *AdminBypassAuditLogHandler) ListByHost() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		hostID := r.PathValue("hostID")
		if hostID == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "hostID is required"})
			return
		}

		q := r.URL.Query()

		// target_kind / target_id 默认锁定到 host scope，但允许 query 覆盖。
		targetKind := q.Get("target_kind")
		if targetKind == "" {
			targetKind = "host"
		}
		targetID := q.Get("target_id")
		if targetID == "" {
			targetID = hostID
		}

		// limit 解析 + clamp。
		limit := defaultAuditLogLimit
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
				return
			}
			if n > maxAuditLogLimit {
				n = maxAuditLogLimit
			}
			limit = n
		}

		// before cursor 解析（RFC3339）。空串表示从最新开始。
		var beforeTS time.Time
		if v := q.Get("before"); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				// 退化尝试 RFC3339（去掉 Nano）。
				t2, err2 := time.Parse(time.RFC3339, v)
				if err2 != nil {
					writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "before must be RFC3339 format"})
					return
				}
				t = t2
			}
			beforeTS = t
		}

		rows, err := h.store.ListBypassAuditLogByTarget(r.Context(), targetKind, targetID)
		if err != nil {
			h.logger.Error("list bypass audit log failed", "error", err, "host_id", hostID)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "list audit log failed"})
			return
		}

		// 应用 before cursor 过滤（Repository 已 created_at DESC，handler 仅做截断）。
		filtered := rows
		if !beforeTS.IsZero() {
			filtered = filtered[:0]
			for _, it := range rows {
				if it.CreatedAt.Before(beforeTS) {
					filtered = append(filtered, it)
				}
			}
		}

		// 按 limit 截断。
		nextBefore := ""
		if len(filtered) > limit {
			filtered = filtered[:limit]
		}
		// next_before：截断后正好达到 limit 时取最后一行 created_at 作为下一页 cursor；
		// 不足 limit 表示已抵末页。
		if len(filtered) == limit && limit > 0 {
			nextBefore = filtered[len(filtered)-1].CreatedAt.UTC().Format(time.RFC3339Nano)
		}

		// 确保 audit_log 字段始终为非 nil 切片，避免 JSON 出现 null。
		if filtered == nil {
			filtered = []repository.BypassAuditLog{}
		}

		writeJSON(w, nethttp.StatusOK, auditLogListResponse{
			AuditLog:   filtered,
			NextBefore: nextBefore,
		})
	})
}
