package cloudclaude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestAuthResponse_V3Fields_RoundTrip 覆盖 Phase 30 Wave 2 D-03：
// v3 服务器响应扩展 image_version / supports_mergerfs / claude_account_id，
// 客户端结构体必须能完整读取。
func TestAuthResponse_V3Fields_RoundTrip(t *testing.T) {
	payload := `{
		"status": "ready",
		"ssh_user": "u", "ssh_pass": "p", "ssh_host": "h", "ssh_port": 2222,
		"image_version": "v3.0.0",
		"supports_mergerfs": true,
		"claude_account_id": "claude-acct-42"
	}`
	var resp AuthResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal v3 payload: %v", err)
	}
	if resp.ImageVersion != "v3.0.0" {
		t.Errorf("ImageVersion = %q, want v3.0.0", resp.ImageVersion)
	}
	if !resp.SupportsMergerfs {
		t.Errorf("SupportsMergerfs = false, want true")
	}
	if resp.ClaudeAccountID != "claude-acct-42" {
		t.Errorf("ClaudeAccountID = %q, want claude-acct-42", resp.ClaudeAccountID)
	}
}

// TestAuthResponse_MissingV3Fields_DefaultZero 覆盖 D-08：非 v3 服务器不返回扩展字段时，
// 客户端字段默认零值即可，既有 SSH 校验路径不受影响。
func TestAuthResponse_MissingV3Fields_DefaultZero(t *testing.T) {
	payload := `{"status":"ready","ssh_user":"u","ssh_pass":"p","ssh_host":"h","ssh_port":2222}`
	var resp AuthResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal v2 payload: %v", err)
	}
	if resp.ImageVersion != "" || resp.SupportsMergerfs || resp.ClaudeAccountID != "" {
		t.Errorf("v2 payload must leave v3 fields zero-valued, got: %+v", resp)
	}
	if resp.SSHHost != "h" || resp.SSHPort != 2222 || resp.SSHUser != "u" || resp.SSHPass != "p" {
		t.Errorf("v2 SSH fields must survive decode, got: %+v", resp)
	}
}

// TestAuthResponse_MarshalOmitempty 锁死 v3 扩展字段的 omitempty 行为：
// 仅在有意义时出现在 JSON 里，避免把"没拿到"误为"明确的 false/空串"。
func TestAuthResponse_MarshalOmitempty(t *testing.T) {
	resp := AuthResponse{
		Status: "ready", SSHUser: "u", SSHPass: "p", SSHHost: "h", SSHPort: 2222,
	}
	buf, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"image_version"`, `"supports_mergerfs"`, `"claude_account_id"`} {
		if strings.Contains(string(buf), key) {
			t.Errorf("empty value must be omitted for %s, got: %s", key, buf)
		}
	}
}

// TestAuthenticate_V3Gateway_PreservesExtensions 覆盖端到端：
// Authenticate() 针对 v3 gateway 返回的 JSON 必须把扩展字段原样透传，且不破坏既有 SSH 四元组校验（D-08）。
func TestAuthenticate_V3Gateway_PreservesExtensions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status":"ready","ssh_user":"u","ssh_pass":"p","ssh_host":"h","ssh_port":2222,
			"image_version":"v3.0.0","supports_mergerfs":true,
			"claude_account_id":"claude-acct-42"
		}`))
	}))
	defer srv.Close()

	client := NewEntryClient(srv.URL)
	resp, err := client.Authenticate(context.Background(), "short", "pwd")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if resp.Status != "ready" {
		t.Fatalf("status = %q", resp.Status)
	}
	if resp.ImageVersion != "v3.0.0" || !resp.SupportsMergerfs || resp.ClaudeAccountID != "claude-acct-42" {
		t.Errorf("extensions lost after round-trip: %+v", resp)
	}
}

// TestAuthResponse_StatusString 确认字符串形态的 status 字段仍然正常工作。
func TestAuthResponse_StatusString(t *testing.T) {
	payload := `{"status":"ready","ssh_user":"u","ssh_pass":"p","ssh_host":"h","ssh_port":2222}`
	var resp AuthResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal string status: %v", err)
	}
	if resp.Status.String() != "ready" {
		t.Errorf("Status = %q, want ready", resp.Status.String())
	}
}

// TestAuthResponse_StatusNumber 覆盖数字 status 解析场景：
// gateway 返回 {"status":1} 时，Status 应解析为 "1" 不报错。
func TestAuthResponse_StatusNumber(t *testing.T) {
	payload := `{"status":1,"ssh_user":"u","ssh_pass":"p","ssh_host":"h","ssh_port":2222}`
	var resp AuthResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal number status: %v", err)
	}
	if resp.Status.String() != "1" {
		t.Errorf("Status = %q, want 1", resp.Status.String())
	}
}

// TestAuthResponse_StatusNumberZero 覆盖数字 0 边界。
func TestAuthResponse_StatusNumberZero(t *testing.T) {
	payload := `{"status":0}`
	var resp AuthResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal number zero status: %v", err)
	}
	if resp.Status.String() != "0" {
		t.Errorf("Status = %q, want 0", resp.Status.String())
	}
}

// TestAuthResponse_StatusMarshal 确认序列化时 Status 输出为 JSON 字符串。
func TestAuthResponse_StatusMarshal(t *testing.T) {
	resp := AuthResponse{Status: Status("ready"), SSHUser: "u"}
	buf, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(buf), `"status":"ready"`) {
		t.Errorf("marshal must emit status as string, got: %s", buf)
	}
}

// TestAuthResponse_StatusComparison 确认 == 比较语法保持可用。
func TestAuthResponse_StatusComparison(t *testing.T) {
	resp := AuthResponse{Status: Status("ready")}
	if resp.Status != "ready" {
		t.Errorf("Status != \"ready\" with direct comparison")
	}
	if resp.Status.String() != "ready" {
		t.Errorf("Status.String() != \"ready\"")
	}
}

// TestAuthenticate_NumberStatus 覆盖端到端：gateway 返回数字 status 时，
// Authenticate() 不报错且能正确读取 SSH 字段。
func TestAuthenticate_NumberStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"ssh_user":"u","ssh_pass":"p","ssh_host":"h","ssh_port":2222}`))
	}))
	defer srv.Close()

	client := NewEntryClient(srv.URL)
	resp, err := client.Authenticate(context.Background(), "short", "pwd")
	if err != nil {
		t.Fatalf("Authenticate with number status: %v", err)
	}
	if resp.Status.String() != "1" {
		t.Errorf("Status = %q, want 1", resp.Status.String())
	}
	if resp.SSHUser != "u" || resp.SSHPass != "p" || resp.SSHHost != "h" || resp.SSHPort != 2222 {
		t.Errorf("SSH fields lost: %+v", resp)
	}
}

// TestAuthenticateAndWait_NumberStatusPolling 验证数字 status 轮询逻辑：
// 第一次返回 status:0（未就绪），第二次返回 status:1（就绪，带 SSH 四元组）。
// 注意：AuthenticateAndWait 中判断 resp.Status == "ready"，数字 "1" 不等于 "ready"，
// 因此本测试模拟的是一个"数字形态的非 ready 状态"轮询到"字符串形态的 ready"。
// 若未来 gateway 用数字 1 表示 ready，应在文档中说明映射关系。
func TestAuthenticateAndWait_NumberStatusPolling(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if callCount == 1 {
			_, _ = w.Write([]byte(`{"status":0,"message":"容器启动中"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ready","ssh_user":"u","ssh_pass":"p","ssh_host":"h","ssh_port":2222}`))
	}))
	defer srv.Close()

	client := NewEntryClient(srv.URL)
	client.pollInterval = 50 * time.Millisecond
	client.pollTimeout = 2 * time.Second

	resp, err := client.AuthenticateAndWait(context.Background(), "short", "pwd", nil)
	if err != nil {
		t.Fatalf("AuthenticateAndWait: %v", err)
	}
	if resp.Status.String() != "ready" {
		t.Errorf("final Status = %q, want ready", resp.Status.String())
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 poll iterations, got %d", callCount)
	}
}

// TestAuthenticate_V2Gateway_NoExtensionsRequired 锁死向后兼容：
// 旧 gateway 只返回 v2 字段时 Authenticate() 依然成功，扩展字段保持零值（D-08）。
func TestAuthenticate_V2Gateway_NoExtensionsRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready","ssh_user":"u","ssh_pass":"p","ssh_host":"h","ssh_port":2222}`))
	}))
	defer srv.Close()

	client := NewEntryClient(srv.URL)
	resp, err := client.Authenticate(context.Background(), "short", "pwd")
	if err != nil {
		t.Fatalf("Authenticate on v2 gateway: %v", err)
	}
	if resp.Status != "ready" || resp.SSHHost != "h" || resp.SSHPort != 2222 {
		t.Fatalf("v2 ready payload lost: %+v", resp)
	}
	if resp.ImageVersion != "" || resp.SupportsMergerfs || resp.ClaudeAccountID != "" {
		t.Errorf("v3 fields must be zero-valued on v2 gateway, got: %+v", resp)
	}
}
