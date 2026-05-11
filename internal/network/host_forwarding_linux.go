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
// Architecture:
//   - Worker's default gateway points to the host's bridge IP (10.99.X.1).
//   - All worker outbound traffic reaches the host first.
//   - Host policy-routes worker subnet traffic to the gateway (10.99.X.2) -> sing-box tunnel.
//   - Port-mapped inbound traffic is DNAT'd to worker, then SNAT'd so the worker replies
//     directly to the host (same subnet) instead of routing through the gateway where
//     sing-box auto_route would hijack the reply into the proxy tunnel.
func setupPortForwarding(ctx context.Context, hostID, bridgeGW, gwIP string, ports []agentapi.PortMapping) error {
	third := subnetThirdOctet(hostID)
	workerIP := fmt.Sprintf("10.99.%d.3", third)
	subnet := fmt.Sprintf("10.99.%d.0/24", third)

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

		// DNAT: external:hostPort -> worker:containerPort
		dnatArgs := []string{"-t", "nat", "-A", "CLOUDPROXY-PORTMAP",
			"-p", proto, "--dport", hp,
			"-j", "DNAT", "--to-destination", workerIP + ":" + cp}
		if out, err := exec.CommandContext(ctx, "iptables", dnatArgs...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables DNAT %d->%s:%d: %w (%s)", pm.HostPort, workerIP, pm.ContainerPort, err, strings.TrimSpace(string(out)))
		}

		// SNAT: change source IP to bridgeGW so worker replies directly to host
		// instead of routing through gateway where sing-box would hijack it.
		snatArgs := []string{"-t", "nat", "-A", "POSTROUTING",
			"-p", proto,
			"-d", workerIP, "--dport", cp,
			"-m", "comment", "--comment", "cloudproxy-snat-" + hostID,
			"-j", "SNAT", "--to-source", bridgeGW}
		if out, err := exec.CommandContext(ctx, "iptables", snatArgs...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables SNAT %s:%d: %w (%s)", workerIP, pm.ContainerPort, err, strings.TrimSpace(string(out)))
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

	// --- FORWARD chain: allow all worker subnet traffic ---
	// Worker traffic (DNS, HTTP, etc.) reaches the host first because the
	// default gateway is the host bridge IP. The host policy-routes it to
	// the gateway for sing-box tunneling.
	allFwdArgs := []string{"-A", "CLOUDPROXY-PORTMAP",
		"-s", "10.99.0.0/16",
		"-j", "ACCEPT"}
	if out, err := exec.CommandContext(ctx, "iptables", allFwdArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("iptables worker subnet forward: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// --- Policy routing: worker subnet -> gateway -> sing-box tunnel ---
	_ = exec.CommandContext(ctx, "ip", "rule", "add", "from", subnet, "lookup", "100").Run()
	_ = exec.CommandContext(ctx, "ip", "route", "add", "default", "via", gwIP, "table", "100").Run()

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
		return fmt.Errorf("hook %s/%s->%s: %w (%s)", table, parent, child, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// teardownPortForwarding removes the CLOUDPROXY-PORTMAP chain from both
// PREROUTING (nat) and FORWARD (filter), then flushes and deletes the chain.
// Also cleans up SNAT rules and policy routes for the given host.
func teardownPortForwarding(ctx context.Context, hostID string) {
	third := subnetThirdOctet(hostID)
	gwIP := fmt.Sprintf("10.99.%d.2", third)
	subnet := fmt.Sprintf("10.99.%d.0/24", third)

	// Remove SNAT rules by matching the comment
	deleteNatRulesByComment(ctx, "cloudproxy-snat-"+hostID)

	// Remove hooks
	exec.CommandContext(ctx, "iptables", "-t", "nat", "-D", "PREROUTING", "-j", "CLOUDPROXY-PORTMAP").Run()
	exec.CommandContext(ctx, "iptables", "-D", "FORWARD", "-j", "CLOUDPROXY-PORTMAP").Run()

	// Flush and delete nat chain
	exec.CommandContext(ctx, "iptables", "-t", "nat", "-F", "CLOUDPROXY-PORTMAP").Run()
	exec.CommandContext(ctx, "iptables", "-t", "nat", "-X", "CLOUDPROXY-PORTMAP").Run()

	// Flush and delete filter chain
	exec.CommandContext(ctx, "iptables", "-F", "CLOUDPROXY-PORTMAP").Run()
	exec.CommandContext(ctx, "iptables", "-X", "CLOUDPROXY-PORTMAP").Run()

	// Clean up policy routes
	exec.CommandContext(ctx, "ip", "rule", "del", "from", subnet, "lookup", "100").Run()
	exec.CommandContext(ctx, "ip", "route", "del", "default", "via", gwIP, "table", "100").Run()
}

// deleteNatRulesByComment removes all rules in the POSTROUTING chain whose
// comment contains the given substring. It iterates from the end to avoid
// line-number shifts.
func deleteNatRulesByComment(ctx context.Context, comment string) {
	out, _ := exec.CommandContext(ctx, "iptables", "-t", "nat", "-L", "POSTROUTING", "--line-numbers", "-n").CombinedOutput()
	lines := strings.Split(string(out), "\n")
	// Iterate backwards so line numbers don't shift
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], comment) {
			fields := strings.Fields(lines[i])
			if len(fields) > 0 {
				exec.CommandContext(ctx, "iptables", "-t", "nat", "-D", "POSTROUTING", fields[0]).Run()
			}
		}
	}
}
