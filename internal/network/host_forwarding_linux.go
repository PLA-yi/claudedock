//go:build linux

package network

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
)

func ensureIPForwarding(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sysctl", "-w", "net.ipv4.ip_forward=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("enable ip forwarding: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureHostMasquerade(ctx context.Context) error {
	check := exec.CommandContext(ctx, "iptables", "-t", "nat", "-C", "POSTROUTING",
		"-s", "10.99.0.0/16", "-j", "MASQUERADE")
	if check.Run() == nil {
		return nil
	}
	add := exec.CommandContext(ctx, "iptables", "-t", "nat", "-A", "POSTROUTING",
		"-s", "10.99.0.0/16", "-j", "MASQUERADE")
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("add masquerade rule: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// setupPortForwarding creates host iptables rules for port mapping and worker routing.
//
// The worker's default gateway points to the host's bridge IP (10.99.X.1) instead
// of the gateway container, because the gateway's sing-box TUN auto_route would
// hijack forwarded reply packets and send them through the proxy tunnel.
//
// The host uses iptables + conntrack to route:
//   - ESTABLISHED/RELATED from worker subnet → MASQUERADE → direct out
//     (port-mapped replies + tunnel replies, conntrack reverses NAT for each)
//   - New connections from worker → gateway (10.99.X.2)
//     (tunnel traffic, processed by gateway's sing-box → proxy server)
//   - Port-mapped inbound → DNAT → worker IP
//
func setupPortForwarding(ctx context.Context, hostID string, ports []agentapi.PortMapping) error {
	third := subnetThirdOctet(hostID)
	workerIP := fmt.Sprintf("10.99.%d.3", third)
	gwIP := fmt.Sprintf("10.99.%d.2", third)

	// --- PREROUTING chain (nat): DNAT for port mapping ---
	for _, pm := range ports {
		if pm.HostPort <= 0 || pm.ContainerPort <= 0 {
			continue
		}

		proto := strings.ToLower(pm.Protocol)
		if proto == "" {
			proto = "tcp"
		}

		hp := strconv.Itoa(pm.HostPort)
		cp := strconv.Itoa(pm.ContainerPort)

		dnatArgs := []string{"-t", "nat", "-A", "CLOUDPROXY-PORTMAP",
			"-p", proto, "--dport", hp,
			"-j", "DNAT", "--to-destination", workerIP + ":" + cp}
		if out, err := exec.CommandContext(ctx, "iptables", dnatArgs...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables DNAT %d→%s:%d: %w (%s)", pm.HostPort, workerIP, pm.ContainerPort, err, strings.TrimSpace(string(out)))
		}

		// FORWARD ACCEPT for port-mapped inbound traffic
		fwdArgs := []string{"-A", "CLOUDPROXY-PORTMAP",
			"-p", proto, "--dport", cp,
			"-d", workerIP,
			"-j", "ACCEPT"}
		if out, err := exec.CommandContext(ctx, "iptables", fwdArgs...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables FORWARD %s:%d: %w (%s)", workerIP, pm.ContainerPort, err, strings.TrimSpace(string(out)))
		}
	}

	// --- FORWARD chain: reply routing + tunnel forwarding ---

	// ESTABLISHED/RELATED from 10.99.0.0/16 → MASQUERADE + ACCEPT
	// Covers: port-mapped replies (worker→external) + tunnel replies (gateway→worker)
	// conntrack reverses the NAT for return traffic in both cases.
	estArgs := []string{"-A", "CLOUDPROXY-PORTMAP",
		"-s", "10.99.0.0/16",
		"-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED",
		"-j", "ACCEPT"}
	if out, err := exec.CommandContext(ctx, "iptables", estArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("iptables ESTABLISHED rule: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// New connections from 10.99.0.0/16 to gateway IP → ACCEPT
	// Worker's tunnel traffic: worker → host → gateway → sing-box → proxy server
	gwFwdArgs := []string{"-A", "CLOUDPROXY-PORTMAP",
		"-s", "10.99.0.0/16",
		"-d", gwIP,
		"-j", "ACCEPT"}
	if out, err := exec.CommandContext(ctx, "iptables", gwFwdArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("iptables gateway forward: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// MASQUERADE in POSTROUTING for tunnel traffic (worker → gateway).
	// The existing ensureHostMasquerade rule (10.99.0.0/16 → MASQUERADE) covers this,
	// but we add a per-host rule here for clarity and to ensure correct ordering.
	masqArgs := []string{"-t", "nat", "-A", "CLOUDPROXY-PORTMAP",
		"-s", "10.99.0.0/16",
		"-j", "MASQUERADE"}
	if out, err := exec.CommandContext(ctx, "iptables", masqArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("iptables MASQUERADE: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// ensurePortMapChain creates the CLOUDPROXY-PORTMAP iptables chain and hooks
// it into PREROUTING (nat) and FORWARD (filter) if not already present.
func ensurePortMapChain(ctx context.Context) error {
	// Create chain (ignore "already exists" error)
	exec.CommandContext(ctx, "iptables", "-t", "nat", "-N", "CLOUDPROXY-PORTMAP").Run()
	exec.CommandContext(ctx, "iptables", "-N", "CLOUDPROXY-PORTMAP").Run()

	// Hook into PREROUTING (nat) if not already present
	if err := ensureChainHook(ctx, "nat", "PREROUTING", "CLOUDPROXY-PORTMAP"); err != nil {
		return err
	}
	// Hook into FORWARD (filter) if not already present
	if err := ensureChainHook(ctx, "filter", "FORWARD", "CLOUDPROXY-PORTMAP"); err != nil {
		return err
	}

	return nil
}

func ensureChainHook(ctx context.Context, table, parent, child string) error {
	baseArgs := []string{"-t", table, "-C", parent, "-j", child}
	if exec.CommandContext(ctx, "iptables", baseArgs...).Run() == nil {
		return nil
	}

	addArgs := []string{"-t", table, "-I", parent, "1", "-j", child}
	if out, err := exec.CommandContext(ctx, "iptables", addArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("hook %s/%s→%s: %w (%s)", table, parent, child, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// teardownPortForwarding removes the CLOUDPROXY-PORTMAP chain from both
// PREROUTING (nat) and FORWARD (filter), then flushes and deletes the chain.
func teardownPortForwarding(ctx context.Context) {
	// Remove hooks
	exec.CommandContext(ctx, "iptables", "-t", "nat", "-D", "PREROUTING", "-j", "CLOUDPROXY-PORTMAP").Run()
	exec.CommandContext(ctx, "iptables", "-D", "FORWARD", "-j", "CLOUDPROXY-PORTMAP").Run()

	// Flush and delete nat chain
	exec.CommandContext(ctx, "iptables", "-t", "nat", "-F", "CLOUDPROXY-PORTMAP").Run()
	exec.CommandContext(ctx, "iptables", "-t", "nat", "-X", "CLOUDPROXY-PORTMAP").Run()

	// Flush and delete filter chain
	exec.CommandContext(ctx, "iptables", "-F", "CLOUDPROXY-PORTMAP").Run()
	exec.CommandContext(ctx, "iptables", "-X", "CLOUDPROXY-PORTMAP").Run()
}
