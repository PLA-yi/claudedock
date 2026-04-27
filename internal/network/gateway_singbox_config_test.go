package network

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildGatewaySingBoxConfig(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildGatewaySingBoxConfig(outbound, "10.0.0.1", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]any
	if err := json.Unmarshal(cfg, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Check top-level keys
	for _, key := range []string{"log", "dns", "inbounds", "outbounds", "route"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}

	// Verify route has final = "proxy-out"
	route, _ := parsed["route"].(map[string]any)
	if final, _ := route["final"].(string); final != "proxy-out" {
		t.Errorf("route.final = %q, want %q", final, "proxy-out")
	}

	// Verify outbounds contains two entries (proxy-out + direct)
	outbounds, _ := parsed["outbounds"].([]any)
	if outbounds == nil {
		t.Fatal("outbounds is not an array")
	}
	if len(outbounds) != 2 {
		t.Errorf("outbounds has %d entries, want 2", len(outbounds))
	}

	// Verify DNS servers point to expected DNS
	dns, _ := parsed["dns"].(map[string]any)
	servers, _ := dns["servers"].([]any)
	if servers == nil || len(servers) != 1 {
		t.Fatal("expected exactly 1 DNS server")
	}
	srv := servers[0].(map[string]any)
	if srv["server"] != "10.0.0.1" {
		t.Errorf("dns server = %q, want %q", srv["server"], "10.0.0.1")
	}
}

func TestBuildGatewaySingBoxConfig_DefaultDNS(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildGatewaySingBoxConfig(outbound, "", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(cfg, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	dns, _ := parsed["dns"].(map[string]any)
	servers, _ := dns["servers"].([]any)
	srv := servers[0].(map[string]any)
	if srv["server"] != "1.1.1.1" {
		t.Errorf("dns server = %q, want %q", srv["server"], "1.1.1.1")
	}
}

func TestBuildGatewayProxyOutbound(t *testing.T) {
	userConfig := json.RawMessage(`{"type":"socks","server":"example.com","server_port":1080,"dns_server":"10.0.0.1","bind_interface":"eth0"}`)
	out, err := buildGatewayProxyOutbound(userConfig, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// dns_server should be removed
	if _, ok := parsed["dns_server"]; ok {
		t.Error("dns_server should have been removed")
	}
	// bind_interface should be removed
	if _, ok := parsed["bind_interface"]; ok {
		t.Error("bind_interface should have been removed")
	}
	// tag should be set to proxy-out
	if tag, _ := parsed["tag"].(string); tag != "proxy-out" {
		t.Errorf("tag = %q, want %q", tag, "proxy-out")
	}
	// server should be replaced with resolved IP
	if server, _ := parsed["server"].(string); server != "1.2.3.4" {
		t.Errorf("server = %q, want %q", server, "1.2.3.4")
	}
}

func TestBuildGatewayProxyOutbound_RealityWithUTLS(t *testing.T) {
	userConfig := json.RawMessage(`{
		"type":"vless",
		"server":"example.com",
		"server_port":443,
		"tls":{
			"enabled":true,
			"reality":{
				"enabled":true,
				"public_key":"test-key"
			}
		}
	}`)

	out, err := buildGatewayProxyOutbound(userConfig, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Check that utls was added to tls config
	tls, ok := parsed["tls"].(map[string]any)
	if !ok {
		t.Fatal("tls config missing")
	}
	utls, ok := tls["utls"].(map[string]any)
	if !ok {
		t.Fatal("utls should have been added for reality")
	}
	if enabled, _ := utls["enabled"].(bool); !enabled {
		t.Error("utls.enabled should be true")
	}
	if fp, _ := utls["fingerprint"].(string); fp != "chrome" {
		t.Errorf("utls.fingerprint = %q, want %q", fp, "chrome")
	}
}

func TestBuildGatewayProxyOutbound_RealityWithExistingUTLS(t *testing.T) {
	// When utls is already configured, it should not be overwritten
	userConfig := json.RawMessage(`{
		"type":"vless",
		"server":"example.com",
		"server_port":443,
		"tls":{
			"enabled":true,
			"reality":{
				"enabled":true,
				"public_key":"test-key"
			},
			"utls":{
				"enabled":true,
				"fingerprint":"firefox"
			}
		}
	}`)

	out, err := buildGatewayProxyOutbound(userConfig, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	tls, _ := parsed["tls"].(map[string]any)
	utls, _ := tls["utls"].(map[string]any)
	if fp, _ := utls["fingerprint"].(string); fp != "firefox" {
		t.Errorf("utls.fingerprint should remain 'firefox', got %q", fp)
	}
}

func TestBuildGatewayProxyOutbound_NoTLS(t *testing.T) {
	// Config without TLS should not cause issues
	userConfig := json.RawMessage(`{"type":"socks","server":"example.com","server_port":1080}`)
	out, err := buildGatewayProxyOutbound(userConfig, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if tag, _ := parsed["tag"].(string); tag != "proxy-out" {
		t.Errorf("tag = %q, want %q", tag, "proxy-out")
	}
}

func TestBuildGatewaySingBoxConfig_ProxyServerIPInRoute(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildGatewaySingBoxConfig(outbound, "10.0.0.1", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The route rules should include a direct bypass for the proxy server IP
	cfgStr := string(cfg)
	if !strings.Contains(cfgStr, "1.2.3.4/32") {
		t.Error("route rules should contain proxy server IP /32 bypass")
	}
}
