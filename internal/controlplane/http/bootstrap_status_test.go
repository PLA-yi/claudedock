package http

import (
	"context"
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/claudedock/claudedock/internal/store/repository"
)

type stubTaskGetter struct {
	task repository.Task
	err  error
}

func (s *stubTaskGetter) GetTaskByID(_ context.Context, _ string) (repository.Task, error) {
	return s.task, s.err
}

type stubEventLister struct {
	events []repository.Event
	err    error
}

func (s *stubEventLister) ListEventsByTaskID(_ context.Context, _ string, _ int) ([]repository.Event, error) {
	return s.events, s.err
}

func TestBootstrapStatusHandler(t *testing.T) {
	hostID := "host-1"

	tests := []struct {
		name           string
		taskID         string
		task           repository.Task
		taskErr        error
		events         []repository.Event
		wantStatus     int
		wantStageCode  string
		wantStageText  string
		wantTaskStatus string
		wantRetryable  bool
	}{
		{
			name:   "pending task maps to host_starting stage",
			taskID: "task-1",
			task: repository.Task{
				ID:     "task-1",
				HostID: &hostID,
				Kind:   "start_host",
				Status: repository.TaskStatusPending,
			},
			events:         nil,
			wantStatus:     nethttp.StatusOK,
			wantStageCode:  "host_starting",
			wantStageText:  "主机启动中",
			wantTaskStatus: "pending",
		},
		{
			name:   "running task without ssh.ready maps to host_starting",
			taskID: "task-2",
			task: repository.Task{
				ID:     "task-2",
				HostID: &hostID,
				Kind:   "start_host",
				Status: repository.TaskStatusRunning,
			},
			events:         nil,
			wantStatus:     nethttp.StatusOK,
			wantStageCode:  "host_starting",
			wantStageText:  "主机启动中",
			wantTaskStatus: "running",
		},
		{
			name:   "running task with runtime.validating event maps to runtime_validating",
			taskID: "task-3",
			task: repository.Task{
				ID:     "task-3",
				HostID: &hostID,
				Kind:   "start_host",
				Status: repository.TaskStatusRunning,
			},
			events: []repository.Event{
				{Type: "net.ready", CreatedAt: time.Now().Add(-2 * time.Second)},
				{Type: "runtime.validating", CreatedAt: time.Now().Add(-1 * time.Second)},
			},
			wantStatus:     nethttp.StatusOK,
			wantStageCode:  "runtime_validating",
			wantStageText:  "运行时校验中",
			wantTaskStatus: "running",
		},
		{
			name:   "succeeded task with ssh.ready event maps to ssh_ready",
			taskID: "task-4",
			task: repository.Task{
				ID:     "task-4",
				HostID: &hostID,
				Kind:   "start_host",
				Status: repository.TaskStatusSucceeded,
			},
			events: []repository.Event{
				{Type: "net.ready", CreatedAt: time.Now().Add(-3 * time.Second)},
				{Type: "runtime.validating", CreatedAt: time.Now().Add(-2 * time.Second)},
				{Type: "ssh.ready", CreatedAt: time.Now().Add(-1 * time.Second)},
			},
			wantStatus:     nethttp.StatusOK,
			wantStageCode:  "ssh_ready",
			wantStageText:  "SSH 就绪",
			wantTaskStatus: "succeeded",
		},
		{
			name:   "failed task returns error_code and retryable false",
			taskID: "task-5",
			task: repository.Task{
				ID:               "task-5",
				HostID:           &hostID,
				Kind:             "start_host",
				Status:           repository.TaskStatusFailed,
				ErrorCode:        "ssh_not_ready",
				LastErrorSummary: "ssh not ready on container claudedock-host-1 after 30s",
			},
			events:         nil,
			wantStatus:     nethttp.StatusOK,
			wantStageCode:  "host_starting",
			wantStageText:  "主机启动中",
			wantTaskStatus: "failed",
			wantRetryable:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewBootstrapStatusHandler(BootstrapStatusDependencies{
				Logger: slog.Default(),
				Tasks:  &stubTaskGetter{task: tt.task, err: tt.taskErr},
				Events: &stubEventLister{events: tt.events, err: nil},
			})

			req := httptest.NewRequest(nethttp.MethodGet, "/v1/bootstrap/tasks/"+tt.taskID, nil)
			req.SetPathValue("taskID", tt.taskID)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			var resp map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			if got, _ := resp["task_status"].(string); got != tt.wantTaskStatus {
				t.Errorf("task_status = %q, want %q", got, tt.wantTaskStatus)
			}
			if got, _ := resp["stage_code"].(string); got != tt.wantStageCode {
				t.Errorf("stage_code = %q, want %q", got, tt.wantStageCode)
			}
			if got, _ := resp["stage_text"].(string); got != tt.wantStageText {
				t.Errorf("stage_text = %q, want %q", got, tt.wantStageText)
			}

			if tt.wantTaskStatus == "failed" {
				if got, _ := resp["error_code"].(string); got == "" {
					t.Error("expected non-empty error_code for failed task")
				}
				retryable, ok := resp["retryable"].(bool)
				if !ok {
					t.Error("expected retryable field in response")
				} else if retryable != tt.wantRetryable {
					t.Errorf("retryable = %v, want %v", retryable, tt.wantRetryable)
				}
			}
		})
	}
}
