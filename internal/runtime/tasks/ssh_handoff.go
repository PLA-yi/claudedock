package tasks

import "github.com/claudedock/claudedock/internal/network"

// BuildSSHHandoffMetadata produces the machine-readable metadata
// that downstream consumers (handoff API, bootstrap script) need
// to automatically establish an SSH session to a container.
func BuildSSHHandoffMetadata(hostID, defaultUser string) map[string]any {
	access := network.DeriveManagementSSHAccess(hostID)
	return map[string]any{
		"ssh_host": access.Host,
		"ssh_port": access.Port,
		"ssh_user": defaultUser,
		"host_id":  hostID,
	}
}
