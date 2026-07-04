package http

import (
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/claudedock/claudedock/internal/store/repository"
)

func TestBootstrapHandoffHandler(t *testing.T) {
	hostID := "host-1"

	tests := []struct {
		name          string
		taskID        string
		task          repository.Task
		taskErr       error
		events        []repository.Event
		wantStatus    int
		wantReady     bool
		wantErrorCode string
		wantHost      bool
	}{
		{
			name:   "succeeded task with ssh.handoff.ready returns handoff data",
			taskID: "task-1",
			task: repository.Task{
				ID:     "task-1",
				HostID: &hostID,
				Kind:   "start_host",
				Status: repository.TaskStatusSucceeded,
			},
			events: []repository.Event{
				{Type: "ssh.ready", CreatedAt: time.Now().Add(-2 * time.Second)},
				{
					Type:      "ssh.handoff.ready",
					CreatedAt: time.Now().Add(-1 * time.Second),
					Metadata: map[string]any{
						"ssh_host": "10.99.1.2",
						"ssh_port": float64(22),
						"ssh_user": "root",
						"host_id":  "host-1",
					},
				},
			},
			wantStatus: nethttp.StatusOK,
			wantReady:  true,
			wantHost:   true,
		},
		{
			name:   "succeeded task without ssh.handoff.ready returns not ready",
			taskID: "task-2",
			task: repository.Task{
				ID:     "task-2",
				HostID: &hostID,
				Kind:   "start_host",
				Status: repository.TaskStatusSucceeded,
			},
			events:        []repository.Event{},
			wantStatus:    nethttp.StatusServiceUnavailable,
			wantErrorCode: "ssh_not_ready",
		},
		{
			name:   "running task returns not ready",
			taskID: "task-3",
			task: repository.Task{
				ID:     "task-3",
				HostID: &hostID,
				Kind:   "start_host",
				Status: repository.TaskStatusRunning,
			},
			events:        []repository.Event{},
			wantStatus:    nethttp.StatusServiceUnavailable,
			wantErrorCode: "ssh_not_ready",
		},
		{
			name:   "failed task returns task error code",
			taskID: "task-4",
			task: repository.Task{
				ID:               "task-4",
				HostID:           &hostID,
				Kind:             "start_host",
				Status:           repository.TaskStatusFailed,
				ErrorCode:        "ssh_not_ready",
				LastErrorSummary: "SSH 端口在超时时间内未就绪",
			},
			events:        []repository.Event{},
			wantStatus:    nethttp.StatusServiceUnavailable,
			wantErrorCode: "ssh_not_ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewBootstrapHandoffHandler(BootstrapHandoffDependencies{
				Logger: slog.Default(),
				Tasks:  &stubTaskGetter{task: tt.task, err: tt.taskErr},
				Events: &stubEventLister{events: tt.events, err: nil},
			})

			req := httptest.NewRequest(nethttp.MethodGet, "/v1/bootstrap/tasks/"+tt.taskID+"/handoff", nil)
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

			if tt.wantReady {
				if got, _ := resp["ready"].(bool); !got {
					t.Errorf("ready = %v, want true", got)
				}
				if got, _ := resp["host"].(string); got == "" {
					t.Error("expected non-empty host in response")
				}
				if got, _ := resp["port"].(float64); got != 22 {
					t.Errorf("port = %v, want 22", got)
				}
				if got, _ := resp["user"].(string); got == "" {
					t.Error("expected non-empty user in response")
				}
			}

			if tt.wantErrorCode != "" {
				if got, _ := resp["error_code"].(string); got != tt.wantErrorCode {
					t.Errorf("error_code = %q, want %q", got, tt.wantErrorCode)
				}
			}
		})
	}
}

func TestBootstrapErrorMapping(t *testing.T) {
	tests := []struct {
		errorCode    string
		wantMessage  bool
		wantExitCode bool
	}{
		{errorCode: "auth_invalid"},
		{errorCode: "account_disabled"},
		{errorCode: "account_expired"},
		{errorCode: "egress_binding_missing"},
		{errorCode: "start_failed"},
		{errorCode: "ssh_not_ready"},
	}

	for _, tt := range tests {
		t.Run("maps "+tt.errorCode, func(t *testing.T) {
			entry, ok := BootstrapErrorEntries[tt.errorCode]
			if !ok {
				t.Fatalf("missing error mapping for %q", tt.errorCode)
			}
			if entry.Message == "" {
				t.Errorf("empty message for %q", tt.errorCode)
			}
			if entry.ExitCode <= 0 {
				t.Errorf("exit code for %q should be positive, got %d", tt.errorCode, entry.ExitCode)
			}
		})
	}
}

