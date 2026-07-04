package http

import (
	"log/slog"
	nethttp "net/http"
	"strconv"
	"time"

	"github.com/claudedock/claudedock/internal/store/repository"
)

type AdminEventsHandler struct {
	logger *slog.Logger
	store  AdminEventStore
}

func NewAdminEventsHandler(logger *slog.Logger, store AdminEventStore) *AdminEventsHandler {
	return &AdminEventsHandler{logger: logger, store: store}
}

func (h *AdminEventsHandler) List() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		q := r.URL.Query()

		params := repository.ListEventsParams{
			EventType: q.Get("type"),
			UserID:    q.Get("user_id"),
			HostID:    q.Get("host_id"),
		}

		if v := q.Get("since"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "since must be RFC3339 format"})
				return
			}
			params.Since = t
		}

		if v := q.Get("until"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "until must be RFC3339 format"})
				return
			}
			params.Until = t
		}

		limit := 50
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
				return
			}
			if n > 200 {
				n = 200
			}
			limit = n
		}
		params.Limit = limit

		if v := q.Get("offset"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "offset must be a non-negative integer"})
				return
			}
			params.Offset = n
		}

		result, err := h.store.ListEvents(r.Context(), params)
		if err != nil {
			h.logger.Error("list events failed", "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "list events failed"})
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{
			"events": result.Events,
			"total":  result.Total,
			"limit":  limit,
			"offset": params.Offset,
		})
	})
}
