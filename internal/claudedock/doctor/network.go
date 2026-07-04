package doctor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/claudedock/claudedock/internal/claudedock/errcodes"
)

// lookupHost / httpGet 是包级 var，便于 network_test.go 注入 mock。
var (
	lookupHost = net.LookupHost
	httpGet    = func(ctx context.Context, url string, timeout time.Duration) (int, error) {
		ctx2, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx2, "GET", url, nil)
		if err != nil {
			return 0, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		return resp.StatusCode, nil
	}
)

// checkDNSResolve 本地 DNS 解析（RESEARCH §3.1 表）。
func checkDNSResolve(ctx context.Context, host string) Check {
	if host == "" {
		return newSkip("network", "dns_resolve", "未配置网关，跳过；运行 claudedock init 配置后重试")
	}
	addrs, err := lookupHost(host)
	if err != nil {
		return newFail("network", "dns_resolve", errcodes.SYSTEM_DNS_RESOLVE_FAILED, host, err.Error())
	}
	if len(addrs) == 0 {
		return newFail("network", "dns_resolve", errcodes.SYSTEM_DNS_RESOLVE_FAILED, host, "returned empty addr list")
	}
	return Check{
		Domain: "network", Name: "dns_resolve", Status: StatusPass,
		Message: fmt.Sprintf("%s 解析到 %d 个地址", host, len(addrs)),
		Details: map[string]any{"addrs": addrs},
	}
}

// checkGatewayReachable 裸 HTTP GET /healthz（**不**复用 EntryClient，避免误报 token 过期成网络问题，
// RESEARCH §3.1 关键实现信号）。
func checkGatewayReachable(ctx context.Context, gateway string) Check {
	if gateway == "" {
		return newSkip("network", "gateway_reachable", "未配置网关，跳过；运行 claudedock init 配置后重试")
	}
	url := strings.TrimRight(gateway, "/") + "/healthz"
	status, err := httpGet(ctx, url, 2*time.Second)
	if err != nil {
		return newFail("network", "gateway_reachable", errcodes.AUTH_GATEWAY_UNREACHABLE, gateway, err.Error())
	}
	switch {
	case status == 200 || status == 204:
		return newPass("network", "gateway_reachable", fmt.Sprintf("%s 返回 %d", url, status))
	case status >= 500:
		return newWarn("network", "gateway_reachable", errcodes.AUTH_GATEWAY_UNREACHABLE, gateway, fmt.Sprintf("HTTP %d", status))
	default:
		return newFail("network", "gateway_reachable", errcodes.AUTH_GATEWAY_UNREACHABLE, gateway, fmt.Sprintf("HTTP %d", status))
	}
}

// checkEgressIPVisible 远端 curl ifconfig.io（RESEARCH §3.1 表）。
// runner 为 nil 时（未 init 或 SSH 未建立）→ StatusSkip（D-06 / D-20）。
func checkEgressIPVisible(ctx context.Context, runner RemoteRunner, expectedIP string) Check {
	if runner == nil {
		return newSkip("network", "egress_ip_visible", "未能连接远端容器，跳过")
	}
	stdout, _, err := runner.RunScript("egress_ip",
		"curl -sS --max-time 5 https://ifconfig.io 2>/dev/null || curl -sS --max-time 5 https://checkip.amazonaws.com")
	if err != nil {
		return newFail("network", "egress_ip_visible", errcodes.NET_EGRESS_IP_DRIFT, "unknown", "curl 失败: "+err.Error())
	}
	got := strings.TrimSpace(stdout)
	if expectedIP != "" && got != expectedIP {
		return newWarn("network", "egress_ip_visible", errcodes.NET_EGRESS_IP_DRIFT, got, expectedIP)
	}
	return Check{
		Domain: "network", Name: "egress_ip_visible", Status: StatusPass,
		Message: "容器出口 IP: " + got,
		Details: map[string]any{"egress_ip": got},
	}
}
