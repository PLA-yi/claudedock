package agentapi

import (
	"encoding/json"
	"testing"
)

func TestHostAction_Constants(t *testing.T) {
	actions := map[HostAction]string{
		ActionCreateHost:   "create_host",
		ActionStartHost:    "start_host",
		ActionStopHost:     "stop_host",
		ActionRebuildHost:  "rebuild_host",
		ActionPrepareHost:  "prepare_host",
		ActionVolumeRemove: "volume_remove",
	}
	for action, want := range actions {
		if string(action) != want {
			t.Errorf("HostAction = %q, want %q", action, want)
		}
	}
}

func TestHostActionRequest_JSONMarshaling(t *testing.T) {
	req := HostActionRequest{
		TaskID:        "task-1",
		HostID:        "host-1",
		Action:        ActionCreateHost,
		ImageName:     "ghcr.io/test/managed-user:v1",
		DefaultUser:   "workspace",
		ContainerName: "user-alice",
		MemoryLimitMB: 4096,
		CPULimit:      2.0,
		Username:      "alice",
		EntryPassword: "temp",
		SSHKeys: []SSHKeyEntry{
			{Purpose: "auth", Label: "default", PublicKey: "ssh-rsa AAA...", KeyType: "rsa"},
		},
	}

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal = %v", err)
	}

	var decoded HostActionRequest
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal = %v", err)
	}

	if decoded.TaskID != req.TaskID {
		t.Errorf("TaskID = %q, want %q", decoded.TaskID, req.TaskID)
	}
	if decoded.Action != req.Action {
		t.Errorf("Action = %q, want %q", decoded.Action, req.Action)
	}
	if len(decoded.SSHKeys) != 1 {
		t.Errorf("SSHKeys length = %d, want 1", len(decoded.SSHKeys))
	}
}

func TestHostActionRequest_OmitEmpty(t *testing.T) {
	req := HostActionRequest{
		TaskID: "t1",
		Action: ActionStartHost,
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal = %v", err)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	// omitempty fields should be absent
	for _, field := range []string{"memory_limit_mb", "cpu_limit", "username", "ssh_public_key"} {
		if _, ok := m[field]; ok {
			t.Errorf("%q should be omitted when empty", field)
		}
	}
}

func TestTaskStatusUpdate_JSONMarshaling(t *testing.T) {
	update := TaskStatusUpdate{
		TaskID:    "t1",
		Status:    "failed",
		ErrorCode: "ERR_TIMEOUT",
	}
	b, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("json.Marshal = %v", err)
	}
	var decoded TaskStatusUpdate
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal = %v", err)
	}
	if decoded.ErrorMessage != "" {
		t.Error("empty ErrorMessage should not appear")
	}
}

func TestHostActionResponse_JSONMarshaling(t *testing.T) {
	resp := HostActionResponse{
		TaskID:        "t1",
		Action:        ActionStopHost,
		ContainerName: "user-alice",
		Update: TaskStatusUpdate{
			TaskID: "t1",
			Status: "succeeded",
		},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal = %v", err)
	}
	if !json.Valid(b) {
		t.Error("invalid JSON")
	}
}

func TestContainerStatusResponse_Values(t *testing.T) {
	r := ContainerStatusResponse{Name: "ctr-1", Exists: true, Running: true}
	if !r.Exists {
		t.Error("Exists should be true")
	}
	if !r.Running {
		t.Error("Running should be true")
	}

	r2 := ContainerStatusResponse{Name: "ctr-2"}
	if r2.Exists {
		t.Error("zero-value Exists should be false")
	}
	if r2.Running {
		t.Error("zero-value Running should be false")
	}
}

func TestSSHKeyEntry_OmitEmptyPrivateKey(t *testing.T) {
	k := SSHKeyEntry{Purpose: "auth", PublicKey: "pk", KeyType: "ed25519"}
	b, _ := json.Marshal(k)
	var m map[string]any
	json.Unmarshal(b, &m)
	if _, ok := m["private_key"]; ok {
		t.Error("private_key should be omitted when empty")
	}
}

func TestVolumeMount_JSONMarshaling(t *testing.T) {
	vm := VolumeMount{
		Name:   "vol-1",
		Target: "/workspace",
		Labels: map[string]string{"env": "prod"},
	}
	b, err := json.Marshal(vm)
	if err != nil {
		t.Fatalf("json.Marshal = %v", err)
	}
	var decoded VolumeMount
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal = %v", err)
	}
	if decoded.Name != "vol-1" {
		t.Errorf("Name = %q", decoded.Name)
	}
	// ReadOnly is false, should be omitted
	var m map[string]any
	json.Unmarshal(b, &m)
	if _, ok := m["read_only"]; ok {
		t.Error("read_only should be omitted when false")
	}
}

func TestClaudeAccountID_OmitEmpty(t *testing.T) {
	req := HostActionRequest{
		TaskID: "t1",
		Action: ActionCreateHost,
		// ClaudeAccountID intentionally empty
	}
	b, _ := json.Marshal(req)
	var m map[string]any
	json.Unmarshal(b, &m)
	if _, ok := m["claude_account_id"]; ok {
		t.Error("claude_account_id should be omitted when empty")
	}
}
