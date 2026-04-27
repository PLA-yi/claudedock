package network

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractProxyServer_ValidIP(t *testing.T) {
	cfg := json.RawMessage(`{"type":"socks","server":"192.168.1.100","server_port":1080}`)
	ip, port, err := extractProxyServer(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "192.168.1.100" {
		t.Errorf("ip = %q, want %q", ip, "192.168.1.100")
	}
	if port != 1080 {
		t.Errorf("port = %d, want %d", port, 1080)
	}
}

func TestExtractProxyServer_InvalidJSON(t *testing.T) {
	cfg := json.RawMessage(`{invalid`)
	_, _, err := extractProxyServer(cfg)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestExtractProxyServer_MissingServer(t *testing.T) {
	cfg := json.RawMessage(`{"type":"socks","server":"","server_port":1080}`)
	_, _, err := extractProxyServer(cfg)
	if err == nil {
		t.Fatal("expected error for missing server")
	}
}

func TestExtractProxyServer_MissingPort(t *testing.T) {
	cfg := json.RawMessage(`{"type":"socks","server":"192.168.1.100","server_port":0}`)
	_, _, err := extractProxyServer(cfg)
	if err == nil {
		t.Fatal("expected error for missing port")
	}
}

func TestExtractProxyServer_BothMissing(t *testing.T) {
	cfg := json.RawMessage(`{"type":"socks"}`)
	_, _, err := extractProxyServer(cfg)
	if err == nil {
		t.Fatal("expected error for missing server and port")
	}
}

func TestExtractProxyServer_DomainName(t *testing.T) {
	// Domain name resolution requires network access and is skipped in short mode.
	if testing.Short() {
		t.Skip("skipping domain resolution test in short mode (requires network)")
	}
	cfg := json.RawMessage(`{"type":"socks","server":"example.com","server_port":443}`)
	ip, port, err := extractProxyServer(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip == "" {
		t.Error("expected non-empty IP from domain resolution")
	}
	if port != 443 {
		t.Errorf("port = %d, want %d", port, 443)
	}
}

func TestExtractProxyServer_NonExistentDomain(t *testing.T) {
	// DNS resolution for non-existent domain - should fail gracefully.
	if testing.Short() {
		t.Skip("skipping domain resolution test in short mode (requires network)")
	}
	cfg := json.RawMessage(`{"type":"socks","server":"this-domain-definitely-does-not-exist.invalid","server_port":1080}`)
	_, _, err := extractProxyServer(cfg)
	if err == nil {
		t.Fatal("expected error for non-existent domain")
	}
	if !strings.Contains(err.Error(), "resolve") {
		t.Errorf("expected resolve-related error, got: %v", err)
	}
}
