package network

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type mockValidator struct {
	egressIP  EgressIPRecord
	egressErr error
}

func (m *mockValidator) GetEgressIPByHost(_ context.Context, _ string) (EgressIPRecord, error) {
	return m.egressIP, m.egressErr
}

func TestValidateEgressBinding_MissingBinding(t *testing.T) {
	v := &mockValidator{
		egressErr: errors.New("no rows"),
	}

	_, err := ValidateEgressBinding(context.Background(), v, "host-1")
	if err == nil {
		t.Fatal("expected error for missing binding")
	}

	var netErr *NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected *NetworkError, got %T", err)
	}
	if netErr.Type != ErrBindingMissing {
		t.Errorf("expected ErrBindingMissing, got %s", netErr.Type)
	}
}

func TestValidateEgressBinding_ProxySuccess(t *testing.T) {
	proxyConfig := json.RawMessage(`{"type":"socks","server":"proxy.example.com","server_port":1080,"dns_server":"10.0.0.1"}`)

	v := &mockValidator{
		egressIP: EgressIPRecord{
			ID:          "eip-proxy-1",
			IPAddress:   "5.6.7.8",
			TunnelType:  TunnelTypeProxy,
			ProxyConfig: proxyConfig,
		},
	}

	cfg, err := ValidateEgressBinding(context.Background(), v, "host-proxy-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.TunnelType != TunnelTypeProxy {
		t.Errorf("TunnelType: got %q, want %q", cfg.TunnelType, TunnelTypeProxy)
	}
	if cfg.EgressIPID != "eip-proxy-1" {
		t.Errorf("EgressIPID: got %q, want %q", cfg.EgressIPID, "eip-proxy-1")
	}
	if cfg.ExpectedIP != "5.6.7.8" {
		t.Errorf("ExpectedIP: got %q, want %q", cfg.ExpectedIP, "5.6.7.8")
	}
	if cfg.Proxy == nil {
		t.Fatal("Proxy should not be nil for proxy type")
	}
	if cfg.Proxy.OutboundConfig == nil {
		t.Error("Proxy.OutboundConfig should not be nil")
	}
	if cfg.Proxy.DNSServer != "10.0.0.1" {
		t.Errorf("Proxy.DNSServer: got %q, want %q", cfg.Proxy.DNSServer, "10.0.0.1")
	}
}

func TestValidateEgressBinding_ProxyMissingConfig(t *testing.T) {
	v := &mockValidator{
		egressIP: EgressIPRecord{
			ID:          "eip-proxy-2",
			IPAddress:   "5.6.7.8",
			TunnelType:  TunnelTypeProxy,
			ProxyConfig: nil,
		},
	}

	_, err := ValidateEgressBinding(context.Background(), v, "host-proxy-2")
	if err == nil {
		t.Fatal("expected error for proxy type with nil proxy_config")
	}

	var netErr *NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected *NetworkError, got %T", err)
	}
	if netErr.Type != ErrTunnelSetupFailed {
		t.Errorf("expected ErrTunnelSetupFailed, got %s", netErr.Type)
	}
}

func TestTruncateID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		n    int
		want string
	}{
		{name: "shorter than n", id: "abc", n: 10, want: "abc"},
		{name: "equal to n", id: "abcdefghij", n: 10, want: "abcdefghij"},
		{name: "longer than n", id: "abcdefghijklmno", n: 10, want: "abcdefghij"},
		{name: "empty string", id: "", n: 5, want: ""},
		{name: "n is zero", id: "hello", n: 0, want: ""},
		{name: "single char truncate", id: "hello", n: 1, want: "h"},
		{name: "large n", id: "short", n: 100, want: "short"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateID(tt.id, tt.n)
			if got != tt.want {
				t.Errorf("truncateID(%q, %d) = %q, want %q", tt.id, tt.n, got, tt.want)
			}
		})
	}
}

func TestValidateProxyBinding_DNSServerVariants(t *testing.T) {
	tests := []struct {
		name          string
		proxyConfig   string
		wantDNSServer string
	}{
		{
			name:          "dns_server present",
			proxyConfig:   `{"type":"socks","server":"1.2.3.4","server_port":1080,"dns_server":"10.0.0.1"}`,
			wantDNSServer: "10.0.0.1",
		},
		{
			name:          "dns_server missing",
			proxyConfig:   `{"type":"socks","server":"1.2.3.4","server_port":1080}`,
			wantDNSServer: "",
		},
		{
			name:          "dns_server empty string",
			proxyConfig:   `{"type":"socks","server":"1.2.3.4","server_port":1080,"dns_server":""}`,
			wantDNSServer: "",
		},
		{
			name:          "dns_server is number (type mismatch)",
			proxyConfig:   `{"type":"socks","server":"1.2.3.4","server_port":1080,"dns_server":53}`,
			wantDNSServer: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := EgressIPRecord{
				ID:          "eip-1",
				IPAddress:   "5.6.7.8",
				TunnelType:  TunnelTypeProxy,
				ProxyConfig: json.RawMessage(tt.proxyConfig),
			}

			cfg, err := validateProxyBinding(record, "host-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Proxy.DNSServer != tt.wantDNSServer {
				t.Errorf("DNSServer = %q, want %q", cfg.Proxy.DNSServer, tt.wantDNSServer)
			}
		})
	}
}

func TestValidateEgressBinding_ProxyInvalidJSON(t *testing.T) {
	v := &mockValidator{
		egressIP: EgressIPRecord{
			ID:          "eip-proxy-3",
			IPAddress:   "5.6.7.8",
			TunnelType:  TunnelTypeProxy,
			ProxyConfig: json.RawMessage(`{invalid json`),
		},
	}

	_, err := ValidateEgressBinding(context.Background(), v, "host-proxy-3")
	if err == nil {
		t.Fatal("expected error for invalid proxy_config JSON")
	}

	var netErr *NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected *NetworkError, got %T", err)
	}
	if netErr.Type != ErrTunnelSetupFailed {
		t.Errorf("expected ErrTunnelSetupFailed, got %s", netErr.Type)
	}
}
