package doctor

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	out, errs string
	err       error
}

func (f *fakeRunner) RunScript(name, script string) (string, string, error) {
	return f.out, f.errs, f.err
}

func TestCheckDNSResolve_Empty_Skip(t *testing.T) {
	c := checkDNSResolve(context.Background(), "")
	if c.Status != StatusSkip {
		t.Errorf("empty host 应 StatusSkip，实际 %s", c.Status)
	}
}

func TestCheckDNSResolve_Error_Fail(t *testing.T) {
	orig := lookupHost
	lookupHost = func(host string) ([]string, error) { return nil, fmt.Errorf("no such host") }
	t.Cleanup(func() { lookupHost = orig })
	c := checkDNSResolve(context.Background(), "nonexistent.example.com")
	if c.Status != StatusFail {
		t.Errorf("lookup 失败应 StatusFail，实际 %s", c.Status)
	}
	if c.Code != "SYSTEM_DNS_RESOLVE_FAILED" {
		t.Errorf("Code 应为 SYSTEM_DNS_RESOLVE_FAILED，实际 %q", c.Code)
	}
	if c.NextAction == "" {
		t.Error("NextAction 不能为空（M14 回归）")
	}
}

func TestCheckDNSResolve_Success_Pass(t *testing.T) {
	orig := lookupHost
	lookupHost = func(host string) ([]string, error) { return []string{"1.2.3.4"}, nil }
	t.Cleanup(func() { lookupHost = orig })
	c := checkDNSResolve(context.Background(), "gw.example.com")
	if c.Status != StatusPass {
		t.Errorf("lookup 成功应 StatusPass，实际 %s", c.Status)
	}
}

func TestCheckGatewayReachable_200_Pass(t *testing.T) {
	orig := httpGet
	httpGet = func(ctx context.Context, url string, timeout time.Duration) (int, error) { return 200, nil }
	t.Cleanup(func() { httpGet = orig })
	c := checkGatewayReachable(context.Background(), "https://gw.example.com")
	if c.Status != StatusPass {
		t.Errorf("200 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckGatewayReachable_503_Warn(t *testing.T) {
	orig := httpGet
	httpGet = func(ctx context.Context, url string, timeout time.Duration) (int, error) { return 503, nil }
	t.Cleanup(func() { httpGet = orig })
	c := checkGatewayReachable(context.Background(), "https://gw.example.com")
	if c.Status != StatusWarn {
		t.Errorf("503 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "AUTH_GATEWAY_UNREACHABLE" {
		t.Errorf("Code 应为 AUTH_GATEWAY_UNREACHABLE，实际 %q", c.Code)
	}
}

func TestCheckGatewayReachable_Connection_Fail(t *testing.T) {
	orig := httpGet
	httpGet = func(ctx context.Context, url string, timeout time.Duration) (int, error) {
		return 0, fmt.Errorf("connection refused")
	}
	t.Cleanup(func() { httpGet = orig })
	c := checkGatewayReachable(context.Background(), "https://gw.example.com")
	if c.Status != StatusFail {
		t.Errorf("connection refused 应 Fail，实际 %s", c.Status)
	}
}

func TestCheckEgressIPVisible_NilRunner_Skip(t *testing.T) {
	c := checkEgressIPVisible(context.Background(), nil, "1.2.3.4")
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckEgressIPVisible_Drift_Warn(t *testing.T) {
	r := &fakeRunner{out: "5.6.7.8\n"}
	c := checkEgressIPVisible(context.Background(), r, "1.2.3.4")
	if c.Status != StatusWarn {
		t.Errorf("IP 漂移应 Warn，实际 %s", c.Status)
	}
	if c.Code != "NET_EGRESS_IP_DRIFT" {
		t.Errorf("Code 应为 NET_EGRESS_IP_DRIFT，实际 %q", c.Code)
	}
	if !strings.Contains(c.NextAction, "doctor") {
		t.Errorf("NextAction 应含 'doctor'（M14 回归），实际 %q", c.NextAction)
	}
}

func TestCheckEgressIPVisible_Match_Pass(t *testing.T) {
	r := &fakeRunner{out: "1.2.3.4\n"}
	c := checkEgressIPVisible(context.Background(), r, "1.2.3.4")
	if c.Status != StatusPass {
		t.Errorf("IP 匹配应 Pass，实际 %s", c.Status)
	}
}
