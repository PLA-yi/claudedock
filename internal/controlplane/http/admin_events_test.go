package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/claudedock/claudedock/internal/store/repository"
)

type stubEventStore struct {
	result    repository.ListEventsResult
	err       error
	lastQuery repository.ListEventsParams
}

func (s *stubEventStore) ListEvents(_ context.Context, params repository.ListEventsParams) (repository.ListEventsResult, error) {
	s.lastQuery = params
	return s.result, s.err
}

func TestAdminEventsHandler(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	sampleResult := repository.ListEventsResult{
		Events: []repository.Event{
			{ID: "e1", Level: "info", Type: "admin.user.created", Message: "test", CreatedAt: now},
		},
		Total: 1,
	}

	tests := []struct {
		name       string
		query      string
		store      *stubEventStore
		wantStatus int
		wantField  string
		checkQuery func(t *testing.T, q repository.ListEventsParams)
	}{
		{
			name:       "List events 200",
			query:      "",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 200,
			wantField:  "events",
		},
		{
			name:       "List events store error 500",
			query:      "",
			store:      &stubEventStore{err: fmt.Errorf("db down")},
			wantStatus: 500,
		},
		{
			name:       "List with type filter",
			query:      "?type=admin.user.created",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 200,
			checkQuery: func(t *testing.T, q repository.ListEventsParams) {
				t.Helper()
				if q.EventType != "admin.user.created" {
					t.Errorf("EventType = %q, want %q", q.EventType, "admin.user.created")
				}
			},
		},
		{
			name:       "List with user_id filter",
			query:      "?user_id=u1",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 200,
			checkQuery: func(t *testing.T, q repository.ListEventsParams) {
				t.Helper()
				if q.UserID != "u1" {
					t.Errorf("UserID = %q, want %q", q.UserID, "u1")
				}
			},
		},
		{
			name:       "List with host_id filter",
			query:      "?host_id=h1",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 200,
			checkQuery: func(t *testing.T, q repository.ListEventsParams) {
				t.Helper()
				if q.HostID != "h1" {
					t.Errorf("HostID = %q, want %q", q.HostID, "h1")
				}
			},
		},
		{
			name:       "List with valid since",
			query:      "?since=2026-01-01T00:00:00Z",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 200,
			checkQuery: func(t *testing.T, q repository.ListEventsParams) {
				t.Helper()
				if q.Since.IsZero() {
					t.Error("Since should be set")
				}
			},
		},
		{
			name:       "List with invalid since 400",
			query:      "?since=not-a-date",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 400,
		},
		{
			name:       "List with valid until",
			query:      "?until=2026-12-31T23:59:59Z",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 200,
			checkQuery: func(t *testing.T, q repository.ListEventsParams) {
				t.Helper()
				if q.Until.IsZero() {
					t.Error("Until should be set")
				}
			},
		},
		{
			name:       "List with invalid until 400",
			query:      "?until=bad-time",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 400,
		},
		{
			name:       "List with limit 100",
			query:      "?limit=100",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 200,
			checkQuery: func(t *testing.T, q repository.ListEventsParams) {
				t.Helper()
				if q.Limit != 100 {
					t.Errorf("Limit = %d, want 100", q.Limit)
				}
			},
		},
		{
			name:       "List with limit exceeding 200 capped",
			query:      "?limit=500",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 200,
			checkQuery: func(t *testing.T, q repository.ListEventsParams) {
				t.Helper()
				if q.Limit != 200 {
					t.Errorf("Limit = %d, want 200 (capped)", q.Limit)
				}
			},
		},
		{
			name:       "List with negative limit 400",
			query:      "?limit=-1",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 400,
		},
		{
			name:       "List with non-integer limit 400",
			query:      "?limit=abc",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 400,
		},
		{
			name:       "List with offset",
			query:      "?offset=10",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 200,
			checkQuery: func(t *testing.T, q repository.ListEventsParams) {
				t.Helper()
				if q.Offset != 10 {
					t.Errorf("Offset = %d, want 10", q.Offset)
				}
			},
		},
		{
			name:       "List with negative offset 400",
			query:      "?offset=-5",
			store:      &stubEventStore{result: sampleResult},
			wantStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := adminTestRouter(t, Dependencies{
				Logger:      slog.Default(),
				AdminEvents: tt.store,
			})
			srv := httptest.NewServer(router)
			defer srv.Close()

			req, _ := nethttp.NewRequest("GET", srv.URL+"/v1/admin/events"+tt.query, nil)
			req.Header.Set("Authorization", "Bearer "+validAdminToken(t))

			resp, err := nethttp.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				var respBody map[string]any
				json.NewDecoder(resp.Body).Decode(&respBody)
				t.Errorf("status = %d, want %d; body = %v", resp.StatusCode, tt.wantStatus, respBody)
				return
			}

			if tt.wantField != "" {
				var respBody map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if _, ok := respBody[tt.wantField]; !ok {
					t.Errorf("response missing field %q: %v", tt.wantField, respBody)
				}
			}

			if tt.checkQuery != nil {
				tt.checkQuery(t, tt.store.lastQuery)
			}
		})
	}
}
