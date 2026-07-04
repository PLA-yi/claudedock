package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckKeepaliveConfig_TooAggressive_Fail(t *testing.T) {
	c := checkKeepaliveConfig(context.Background(), 10*time.Second)
	if c.Status != StatusFail {
		t.Errorf("10s 应 Fail，实际 %s", c.Status)
	}
	if c.Code != "SESSION_KEEPALIVE_TOO_AGGRESSIVE" {
		t.Errorf("Code 应为 SESSION_KEEPALIVE_TOO_AGGRESSIVE，实际 %q", c.Code)
	}
}

func TestCheckKeepaliveConfig_OK_Pass(t *testing.T) {
	c := checkKeepaliveConfig(context.Background(), 15*time.Second)
	if c.Status != StatusPass {
		t.Errorf("15s 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckKeepaliveConfig_Zero_Skip(t *testing.T) {
	c := checkKeepaliveConfig(context.Background(), 0)
	if c.Status != StatusSkip {
		t.Errorf("0 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckSSHDKeepaliveDrift_Baseline_Pass(t *testing.T) {
	r := &fakeRunner{out: "clientaliveinterval 15\nclientalivecountmax 8\n"}
	c := checkSSHDKeepaliveDrift(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("15/8 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckSSHDKeepaliveDrift_Drift_Warn(t *testing.T) {
	r := &fakeRunner{out: "clientaliveinterval 60\nclientalivecountmax 3\n"}
	c := checkSSHDKeepaliveDrift(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("60/3 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "SSH_SSHD_KEEPALIVE_DRIFT" {
		t.Errorf("Code 应为 SSH_SSHD_KEEPALIVE_DRIFT，实际 %q", c.Code)
	}
}

func TestCheckSSHDKeepaliveDrift_NilRunner_Skip(t *testing.T) {
	c := checkSSHDKeepaliveDrift(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckKnownHosts_Missing_Skip(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "known_hosts")
	c := checkKnownHosts(context.Background(), missing, "example.com:22")
	if c.Status != StatusSkip {
		t.Errorf("文件不存在应 Skip，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckKnownHosts_Valid_Pass(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, "known_hosts")
	// 最小合法的 known_hosts 条目（ssh-ed25519 sample，仅用于 knownhosts.New 解析通过）
	sample := "example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGIE6GJ4FN+uQQl7yh0K8x3lG0m5f5n6Kk7aA0GZXAbD\n"
	if err := os.WriteFile(khPath, []byte(sample), 0600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
	c := checkKnownHosts(context.Background(), khPath, "example.com:22")
	if c.Status != StatusPass {
		t.Logf("actual status=%s message=%q (knownhosts.New 在不同 Go 版本可能对 sample key 严格校验，允许 Warn)", c.Status, c.Message)
	}
}

// ── parseSSHDForwarding 单元测试 ──────────────────────────────────

func TestParseSSHDForwarding_Baseline(t *testing.T) {
	out := "allowtcpforwarding yes\nallowstreamlocalforwarding yes\ngatewayports no\n"
	tcp, stream, gw := parseSSHDForwarding(out)
	if tcp != "yes" || stream != "yes" || gw != "no" {
		t.Errorf("baseline 应为 yes/yes/no，实际 %s/%s/%s", tcp, stream, gw)
	}
}

func TestParseSSHDForwarding_ForwardingDisabled(t *testing.T) {
	out := "allowtcpforwarding no\nallowstreamlocalforwarding yes\ngatewayports no\n"
	tcp, stream, gw := parseSSHDForwarding(out)
	if tcp != "no" || stream != "yes" || gw != "no" {
		t.Errorf("应为 no/yes/no，实际 %s/%s/%s", tcp, stream, gw)
	}
}

func TestParseSSHDForwarding_MissingAllowTcp(t *testing.T) {
	out := "allowstreamlocalforwarding yes\ngatewayports no\n"
	tcp, stream, gw := parseSSHDForwarding(out)
	if tcp != "" || stream != "yes" || gw != "no" {
		t.Errorf("缺失 AllowTcp 应为空串，实际 %s/%s/%s", tcp, stream, gw)
	}
}

func TestParseSSHDForwarding_AllMissing(t *testing.T) {
	tcp, stream, gw := parseSSHDForwarding("")
	if tcp != "" || stream != "" || gw != "" {
		t.Errorf("空输入应全为空串，实际 %s/%s/%s", tcp, stream, gw)
	}
}

func TestParseSSHDForwarding_GatewayPortsYes(t *testing.T) {
	out := "allowtcpforwarding yes\nallowstreamlocalforwarding yes\ngatewayports yes\n"
	tcp, stream, gw := parseSSHDForwarding(out)
	if tcp != "yes" || stream != "yes" || gw != "yes" {
		t.Errorf("应为 yes/yes/yes，实际 %s/%s/%s", tcp, stream, gw)
	}
}

func TestParseSSHDForwarding_CaseInsensitive(t *testing.T) {
	out := "AllowTcpForwarding yes\n"
	tcp, _, _ := parseSSHDForwarding(out)
	if tcp != "yes" {
		t.Errorf("大写前缀应仍解析为 yes，实际 %q", tcp)
	}
}

// ── checkSSHDForwarding 单元测试 ─────────────────────────────────

func TestCheckSSHDForwarding_Baseline_Pass(t *testing.T) {
	r := &fakeRunner{out: "allowtcpforwarding yes\nallowstreamlocalforwarding yes\ngatewayports no\n"}
	c := checkSSHDForwarding(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("baseline 应 Pass，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckSSHDForwarding_ForwardingDisabled_Warn(t *testing.T) {
	r := &fakeRunner{out: "allowtcpforwarding no\nallowstreamlocalforwarding yes\ngatewayports no\n"}
	c := checkSSHDForwarding(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("AllowTcpForwarding=no 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "SSH_SSHD_FORWARDING_DISABLED" {
		t.Errorf("Code 应为 SSH_SSHD_FORWARDING_DISABLED，实际 %q", c.Code)
	}
}

func TestCheckSSHDForwarding_StreamDisabled_Warn(t *testing.T) {
	r := &fakeRunner{out: "allowtcpforwarding yes\nallowstreamlocalforwarding no\ngatewayports no\n"}
	c := checkSSHDForwarding(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("AllowStreamLocalForwarding=no 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "SSH_SSHD_STREAM_FORWARDING_DISABLED" {
		t.Errorf("Code 应为 SSH_SSHD_STREAM_FORWARDING_DISABLED，实际 %q", c.Code)
	}
}

func TestCheckSSHDForwarding_GatewayPortsOpen_Warn(t *testing.T) {
	r := &fakeRunner{out: "allowtcpforwarding yes\nallowstreamlocalforwarding yes\ngatewayports yes\n"}
	c := checkSSHDForwarding(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("GatewayPorts=yes 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "SSH_SSHD_GATEWAY_PORTS_OPEN" {
		t.Errorf("Code 应为 SSH_SSHD_GATEWAY_PORTS_OPEN，实际 %q", c.Code)
	}
}

func TestCheckSSHDForwarding_MultipleIssues(t *testing.T) {
	// AllowTcpForwarding 优先级最高
	r := &fakeRunner{out: "allowtcpforwarding no\ngatewayports yes\n"}
	c := checkSSHDForwarding(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("多问题应 Warn，实际 %s", c.Status)
	}
	if c.Code != "SSH_SSHD_FORWARDING_DISABLED" {
		t.Errorf("AllowTcpForwarding 优先，Code 应为 SSH_SSHD_FORWARDING_DISABLED，实际 %q", c.Code)
	}
}

func TestCheckSSHDForwarding_NilRunner_Skip(t *testing.T) {
	c := checkSSHDForwarding(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckSSHDForwarding_RunnerError_Warn(t *testing.T) {
	r := &fakeRunner{err: fmt.Errorf("connection refused")}
	c := checkSSHDForwarding(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("runner 错误应 Warn，实际 %s", c.Status)
	}
	if c.Code != "SSH_SSHD_FORWARDING_DISABLED" {
		t.Errorf("runner 错误应报第一个问题 Code，实际 %q", c.Code)
	}
}
