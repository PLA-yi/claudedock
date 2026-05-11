//go:build !linux

package network

import (
	"context"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
)

func applyWorkerFirewall(_ context.Context, _, _, _ string, _ []agentapi.PortMapping) error {
	return nil
}

func verifyWorkerNetwork(_ context.Context, _ string, _ EgressConfig) (VerifyResult, error) {
	return VerifyResult{}, nil
}

func cleanupWorkerFirewall(_ context.Context, _ string) {}
