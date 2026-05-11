//go:build !linux

package network

import (
	"context"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
)

// On non-Linux (macOS Docker Desktop), port mapping is handled by Docker's -p
// flag and vpnkit. These stubs are no-ops.

func ensurePortMapChain(_ context.Context) error { return nil }

func setupPortForwarding(_ context.Context, _, _, _ string, _ []agentapi.PortMapping) error {
	return nil
}

func teardownPortForwarding(_ context.Context, _ string) {}
