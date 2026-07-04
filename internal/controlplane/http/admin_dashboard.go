package http

import (
	"context"
	"log/slog"
	nethttp "net/http"

	"github.com/claudedock/claudedock/internal/store/repository"
)

type DashboardStatsGetter interface {
	GetDashboardStats(context.Context) (repository.DashboardStats, error)
}

type DashboardHandler struct {
	logger *slog.Logger
	stats  DashboardStatsGetter
}

func NewDashboardHandler(logger *slog.Logger, stats DashboardStatsGetter) *DashboardHandler {
	return &DashboardHandler{
		logger: logger,
		stats:  stats,
	}
}

func (h *DashboardHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	stats, err := h.stats.GetDashboardStats(r.Context())
	if err != nil {
		h.logger.Error("get dashboard stats failed", "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "failed to get dashboard stats"})
		return
	}

	writeJSON(w, nethttp.StatusOK, stats)
}
