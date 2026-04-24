package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
)

func TestEmbeddedDispatcher_PanicRecovered(t *testing.T) {
	// 注入 testPanicTrigger，让 worker.Execute panic
	origTrigger := TestPanicTrigger
	TestPanicTrigger = func(action agentapi.HostAction) bool {
		return action == agentapi.ActionCreateHost
	}
	t.Cleanup(func() { TestPanicTrigger = origTrigger })

	repo := &fakeWorkerRepo{}
	worker := NewWorker(repo, nil)
	dispatcher := NewEmbeddedDispatcher(worker)

	req := minimalCreateHostRequest("dispatch-panic-host")
	response, err := dispatcher.Dispatch(context.Background(), req)

	if err != nil {
		t.Errorf("Dispatch error should be nil (panic recovered), got: %v", err)
	}
	if response.Update.Status != taskStateFailed {
		t.Errorf("Update.Status = %q, want %q", response.Update.Status, taskStateFailed)
	}
	if response.Update.ErrorCode != "panic_recovered" {
		t.Errorf("Update.ErrorCode = %q, want %q", response.Update.ErrorCode, "panic_recovered")
	}
	if !strings.Contains(response.Update.ErrorMessage, "test panic") {
		t.Errorf("ErrorMessage must contain 'test panic', got: %q", response.Update.ErrorMessage)
	}
	if response.Update.TaskID != req.TaskID {
		t.Errorf("TaskID = %q, want %q", response.Update.TaskID, req.TaskID)
	}
}

func TestEmbeddedDispatcher_NormalPath_Unchanged(t *testing.T) {
	repo := &fakeWorkerRepo{}
	worker := NewWorker(repo, nil)
	dispatcher := NewEmbeddedDispatcher(worker)

	// 使用一个不存在的 action，走 default 分支返回 error
	req := agentapi.HostActionRequest{
		TaskID:   "t-normal",
		HostID:   "h-normal",
		Action:   agentapi.HostAction("nonexistent_action"),
		ImageName: "img:test",
	}

	response, err := dispatcher.Dispatch(context.Background(), req)

	if err != nil {
		t.Errorf("Dispatch error should be nil for normal error path, got: %v", err)
	}
	if response.Update.Status != taskStateFailed {
		t.Errorf("Update.Status = %q, want %q", response.Update.Status, taskStateFailed)
	}
	if response.Update.ErrorCode != "host_action_failed" {
		t.Errorf("Update.ErrorCode = %q, want %q", response.Update.ErrorCode, "host_action_failed")
	}
}

func TestEmbeddedDispatcher_RunHostAction_PanicRecovered(t *testing.T) {
	// RunHostAction 是 Dispatch 的适配器，也应继承 panic recovery
	origTrigger := TestPanicTrigger
	TestPanicTrigger = func(action agentapi.HostAction) bool {
		return action == agentapi.ActionCreateHost
	}
	t.Cleanup(func() { TestPanicTrigger = origTrigger })

	repo := &fakeWorkerRepo{}
	worker := NewWorker(repo, nil)
	dispatcher := NewEmbeddedDispatcher(worker)

	req := minimalCreateHostRequest("runhostaction-panic")
	response, err := dispatcher.RunHostAction(context.Background(), req)

	if err != nil {
		t.Errorf("RunHostAction error should be nil, got: %v", err)
	}
	if response.Update.Status != taskStateFailed {
		t.Errorf("Update.Status = %q, want %q", response.Update.Status, taskStateFailed)
	}
	if response.Update.ErrorCode != "panic_recovered" {
		t.Errorf("Update.ErrorCode = %q, want %q", response.Update.ErrorCode, "panic_recovered")
	}
}
