//go:build e2e && linux

// helpers_test.go 在 killswitch_stress 包内复用 e2e 主包公开字段反推容器名
// 的兼容入口。
//
// e2e.GoldenPath.workerDockerName / gatewayDockerName 是包私有方法；
// killswitch_stress 子包通过 g.Host.ContainerName / Host.ID 与
// g.Gateway.ContainerID / Gateway.HostID 公开字段反推容器名（与
// helpers_linux.go::{workerDockerName,gatewayDockerName} 同语义）。

package killswitch_stress

import (
	"context"
	"errors"
	"strings"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"
)

// workerInspectName 反推 worker 容器名。
//
// 优先级：
//   - g.Host.ContainerName 非空 → 直接用。
//   - g.Host.ID 非空 → "cloudproxy-" + ID（与
//     internal/network/container_proxy_provider.go workerContainerName 一致）。
//   - 都为空 → 返回 err，调用方 t.Skip。
func workerInspectName(_ context.Context, g *e2e.GoldenPath) (string, error) {
	if g == nil || g.Host == nil {
		return "", errors.New("worker container name: host handle nil")
	}
	if name := strings.TrimSpace(g.Host.ContainerName); name != "" {
		return name, nil
	}
	if g.Host.ID != "" {
		return "cloudproxy-" + g.Host.ID, nil
	}
	return "", errors.New("worker container name: host.ID empty (scenario step 7 未实现)")
}

// gatewayInspectName 反推 gateway 容器名。
//
// 优先级：
//   - g.Gateway.ContainerID 非空 → 直接用。
//   - g.Gateway.HostID 非空 → "cloudproxy-gw-" + HostID（与
//     internal/network/container_proxy_provider.go gatewayContainerName 一致）。
//   - g.Host.ID 兜底 → "cloudproxy-gw-" + Host.ID。
//   - 都为空 → 返回 err，调用方 t.Skip。
func gatewayInspectName(_ context.Context, g *e2e.GoldenPath) (string, error) {
	if g == nil || g.Gateway == nil {
		return "", errors.New("gateway container name: gateway handle nil")
	}
	if id := strings.TrimSpace(g.Gateway.ContainerID); id != "" {
		return id, nil
	}
	if g.Gateway.HostID != "" {
		return "cloudproxy-gw-" + g.Gateway.HostID, nil
	}
	if g.Host != nil && g.Host.ID != "" {
		return "cloudproxy-gw-" + g.Host.ID, nil
	}
	return "", errors.New("gateway container name: ContainerID + HostID 均空 (scenario step 4..6 未实现)")
}
