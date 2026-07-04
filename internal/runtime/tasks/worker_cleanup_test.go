package tasks

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"testing"

	"github.com/claudedock/claudedock/internal/network"
	"github.com/claudedock/claudedock/internal/store/repository"
)

// fakeCleanupProvider 记录每个 Provider 方法的调用次数，
// 用于断言 createHost 失败路径是否真的触发了 CleanupHost。
type fakeCleanupProvider struct {
	mu sync.Mutex

	prepareGatewayCalls int
	prepareHostCalls    int
	cleanupHostCalls    int

	prepareGatewayErr error
	prepareHostErr    error
}

func (p *fakeCleanupProvider) PrepareGateway(_ context.Context, _ network.HostNetworkSpec) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prepareGatewayCalls++
	return p.prepareGatewayErr
}

func (p *fakeCleanupProvider) PrepareHost(_ context.Context, _ network.HostNetworkSpec) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prepareHostCalls++
	return p.prepareHostErr
}

func (p *fakeCleanupProvider) CleanupHost(_ context.Context, _ network.HostNetworkSpec) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleanupHostCalls++
	return nil
}

// fakeEgressRepo 在 fakeWorkerRepo 基础上让 GetEgressIPByHost 返回带 ProxyConfig
// 的真实记录，触发 buildEgressConfig 走 proxy 分支并让 PrepareGateway 路径被进入。
type fakeEgressRepo struct {
	fakeWorkerRepo
}

func (r *fakeEgressRepo) GetEgressIPByHost(_ context.Context, _ string) (repository.EgressIP, error) {
	return repository.EgressIP{
		ID:          "eip-1",
		IPAddress:   "1.2.3.4",
		ProxyConfig: json.RawMessage(`{"type":"http","server":"203.0.113.10","server_port":8080}`),
	}, nil
}

// TestWorker_CreateHost_CleanupOnFailure 验证 Phase 45 CR-02：
// PrepareGateway 成功后 buildCreateArgs 失败（这里通过 EntryPassword="" 触发），
// createHost 必须经由 defer 调用一次 CleanupHost 把 gateway 容器 + DATA_DIR
// 残留文件清干净。
//
// 旧实现只在 Execute 失败路径 `docker stop`，从不调 CleanupHost，资源持续泄漏。
func TestWorker_CreateHost_CleanupOnFailure(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker unavailable on this runner")
	}
	repo := &fakeEgressRepo{}
	provider := &fakeCleanupProvider{}
	w := NewWorker(repo, provider)

	req := minimalCreateHostRequest("cleanup-test-host")
	req.EntryPassword = "" // buildCreateArgs 看到空密码会返回错误，触发 CR-02 失败路径

	err := w.createHost(context.Background(), req)
	if err == nil {
		t.Fatal("expected createHost to fail with empty EntryPassword")
	}

	if provider.prepareGatewayCalls != 1 {
		t.Errorf("PrepareGateway 应被调用 1 次, got %d", provider.prepareGatewayCalls)
	}
	if provider.cleanupHostCalls != 1 {
		t.Errorf("CR-02 修复要求 createHost 失败路径必须调用 CleanupHost 1 次, got %d", provider.cleanupHostCalls)
	}
}

// TestWorker_CreateHost_NoCleanupBeforeGatewayPrepared 反向断言：
// PrepareGateway 之前的早期失败（egress 校验失败）不应触发 CleanupHost，
// 避免误清生产 gateway。fakeWorkerRepo 默认 GetEgressIPByHost 返回空记录，
// ValidateEgressBinding 立即失败。
func TestWorker_CreateHost_NoCleanupBeforeGatewayPrepared(t *testing.T) {
	repo := &fakeWorkerRepo{}
	provider := &fakeCleanupProvider{}
	w := NewWorker(repo, provider)

	req := minimalCreateHostRequest("early-fail-host")

	err := w.createHost(context.Background(), req)
	if err == nil {
		t.Fatal("expected createHost to fail before PrepareGateway")
	}

	if provider.prepareGatewayCalls != 0 {
		t.Errorf("PrepareGateway 不应被调用（egress 校验先失败）, got %d", provider.prepareGatewayCalls)
	}
	if provider.cleanupHostCalls != 0 {
		t.Errorf("PrepareGateway 未执行时 CleanupHost 也不应被调用, got %d", provider.cleanupHostCalls)
	}
}
