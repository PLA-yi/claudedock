package http

import (
	"log/slog"
	nethttp "net/http"
)

const listTasksRoute = "GET /v1/tasks"

type TasksHandlerDependencies struct {
	Logger *slog.Logger
	Tasks  TaskLister
}

type tasksHandler struct {
	logger *slog.Logger
	tasks  TaskLister
}

func NewTasksHandler(deps TasksHandlerDependencies) nethttp.Handler {
	return tasksHandler{
		logger: deps.Logger,
		tasks:  deps.Tasks,
	}
}

func (h tasksHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	if h.tasks == nil {
		if h.logger != nil {
			h.logger.Info("tasks handler requested without repository", "route", listTasksRoute)
		}
		writeJSON(w, nethttp.StatusServiceUnavailable, map[string]string{"error": "tasks repository unavailable"})
		return
	}

	items, err := h.tasks.ListTasksWithLastErrorSummary(r.Context())
	if err != nil {
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "list tasks failed"})
		return
	}

	response := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{
			"task_id":            item.ID,
			"host_id":            item.HostID,
			"kind":               item.Kind,
			"status":             item.Status,
			"requested_by":       item.RequestedBy,
			"error_code":         item.ErrorCode,
			"error_message":      item.ErrorMessage,
			"last_error_summary": item.LastErrorSummary,
			"progress_percent":   item.ProgressPercent,
			"progress_message":   item.ProgressMessage,
			"created_at":         item.CreatedAt,
			"updated_at":         item.UpdatedAt,
		}
		response = append(response, entry)
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{"tasks": response})
}
