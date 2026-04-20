package tasks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/network"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

const (
	defaultHostRoot       = "/var/lib/cloud-cli-proxy/hosts/"
	defaultWorkspaceMount = "/workspace"
	taskStatePending      = "pending"
	taskStateRunning      = "running"
	taskStateSucceeded    = "succeeded"
	taskStateFailed       = "failed"
	taskStateCanceled     = "canceled"
)

const (
	sshManagedBeginMarker = "# >>> cloud-cli-proxy managed keys (do not edit) >>>"
	sshManagedEndMarker   = "# <<< cloud-cli-proxy managed keys <<<"
)

type WorkerRepo interface {
	UpdateTaskStatus(context.Context, string, string, string, string, string) (repository.Task, error)
	UpdateHostStatus(ctx context.Context, hostID string, status string) error
	GetEgressIPByHost(ctx context.Context, hostID string) (repository.EgressIP, error)
	RecordEvent(ctx context.Context, params repository.RecordEventParams) (repository.Event, error)
}

type Worker struct {
	repo     WorkerRepo
	provider network.Provider
}

func NewWorker(repo WorkerRepo, provider network.Provider) *Worker {
	return &Worker{repo: repo, provider: provider}
}

func (w *Worker) Execute(ctx context.Context, request agentapi.HostActionRequest) agentapi.TaskStatusUpdate {
	var err error
	switch request.Action {
	case agentapi.ActionCreateHost:
		err = w.createHost(ctx, request)
	case agentapi.ActionStartHost:
		err = w.startHost(ctx, request)
	case agentapi.ActionStopHost:
		err = w.stopHost(ctx, request)
	case agentapi.ActionRebuildHost:
		err = w.rebuildHost(ctx, request)
	case agentapi.ActionPrepareHost:
		err = w.validateAndPrepare(ctx, request.HostID)
	default:
		err = fmt.Errorf("unsupported host action: %s", request.Action)
	}

	if err != nil {
		errorCode := "host_action_failed"
		var sshErr *SSHNotReadyError
		var netErr *network.NetworkError
		if errors.As(err, &sshErr) {
			errorCode = "ssh_not_ready"
		} else if errors.As(err, &netErr) {
			switch netErr.Type {
			case network.ErrBindingMissing:
				errorCode = "egress_binding_missing"
			case network.ErrEgressIPMismatch:
				errorCode = "egress_ip_mismatch"
			case network.ErrDNSLeak:
				errorCode = "dns_leak"
			case network.ErrLeakNotBlocked:
				errorCode = "leak_not_blocked"
			case network.ErrEgressUnreachable:
				errorCode = "egress_unreachable"
			case network.ErrTunnelSetupFailed:
				errorCode = "tunnel_setup_failed"
			default:
				errorCode = "network_error"
			}
		}
		_ = w.repo.UpdateHostStatus(ctx, request.HostID, "failed")
		return agentapi.TaskStatusUpdate{
			TaskID:           request.TaskID,
			Status:           taskStateFailed,
			ErrorCode:        errorCode,
			ErrorMessage:     err.Error(),
			LastErrorSummary: summarizeError(err),
		}
	}

	hostStatus := actionToHostStatus(request.Action)
	_ = w.repo.UpdateHostStatus(ctx, request.HostID, hostStatus)

	return agentapi.TaskStatusUpdate{
		TaskID: request.TaskID,
		Status: taskStateSucceeded,
	}
}

func (w *Worker) UpdateTaskStatus(ctx context.Context, update agentapi.TaskStatusUpdate) error {
	_, err := w.repo.UpdateTaskStatus(
		ctx,
		update.TaskID,
		update.Status,
		update.ErrorCode,
		update.ErrorMessage,
		update.LastErrorSummary,
	)
	return err
}

func actionToHostStatus(action agentapi.HostAction) string {
	switch action {
	case agentapi.ActionCreateHost:
		return "running"
	case agentapi.ActionStartHost:
		return "running"
	case agentapi.ActionStopHost:
		return "stopped"
	case agentapi.ActionRebuildHost:
		return "running"
	default:
		return "stopped"
	}
}

func (w *Worker) buildCreateArgs(request agentapi.HostActionRequest, containerName, hostname string) ([]string, error) {
	homeDir := firstNonEmpty(request.HomeDir, hostHomeDir(request.HostID))

	args := []string{
		"create",
		"--name", containerName,
		"--network", "bridge",
		"--cap-add", "NET_ADMIN",
		"--cap-add", "SYS_ADMIN",
		"--device", "/dev/fuse",
		"--security-opt", "apparmor=unconfined",
		"--label", "cloud-cli-proxy.managed=true",
		"--label", fmt.Sprintf("cloud-cli-proxy.host_id=%s", request.HostID),
		"--hostname", hostname,
		"--shm-size", "1g",
		"--sysctl", "net.ipv6.conf.all.disable_ipv6=1",
	}

	if request.MemoryLimitMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", request.MemoryLimitMB))
	}
	if request.CPULimit > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.1f", request.CPULimit))
	}

	if request.EntryPassword == "" {
		return nil, fmt.Errorf("host %s entry_password is empty; refusing to build create args", request.HostID)
	}

	linuxUser := firstNonEmpty(request.DefaultUser, "workspace")
	args = append(args,
		"-e", "TZ="+firstNonEmpty(request.Timezone, "America/Los_Angeles"),
		"-e", "LANG=en_US.UTF-8",
		"-e", "LANGUAGE=en_US:en",
		"-e", "LC_ALL=en_US.UTF-8",
		"-e", "CONTAINER_USER="+linuxUser,
		"-e", "CONTAINER_SSH_PASSWORD="+request.EntryPassword,
		"-v", fmt.Sprintf("%s:%s", homeDir, firstNonEmpty(request.HomeMount, defaultWorkspaceMount)),
	)

	for _, vm := range request.Volumes {
		if vm.Name == "" || vm.Target == "" {
			return nil, fmt.Errorf("invalid volume mount: name=%q target=%q", vm.Name, vm.Target)
		}
		opts := fmt.Sprintf("type=volume,src=%s,dst=%s", vm.Name, vm.Target)
		if vm.ReadOnly {
			opts += ",readonly"
		}
		args = append(args, "--mount", opts)
	}

	for key, value := range request.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
	}

	args = append(args, request.ImageName)
	return args, nil
}

func (w *Worker) createHost(ctx context.Context, request agentapi.HostActionRequest) error {
	homeDir := firstNonEmpty(request.HomeDir, hostHomeDir(request.HostID))
	containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return fmt.Errorf("prepare host home dir %s: %w", homeDir, err)
	}

	w.pullImage(ctx, request.ImageName)

	if exists, err := w.containerExists(ctx, containerName); err != nil {
		return err
	} else if exists {
		if err := w.runDocker(ctx, "rm", "-f", containerName); err != nil {
			return err
		}
	}

	hostname := request.Hostname
	if hostname == "" {
		hostname = containerName
	}

	args, err := w.buildCreateArgs(request, containerName, hostname)
	if err != nil {
		return err
	}

	if err := w.runDocker(ctx, args...); err != nil {
		return err
	}

	if err := w.runDocker(ctx, "start", containerName); err != nil {
		return fmt.Errorf("start container after create: %w", err)
	}

	egressCfg, err := w.buildEgressConfig(ctx, request.HostID)
	if err != nil {
		return err
	}
	if egressCfg != nil {
		spec := network.HostNetworkSpec{
			HostID: request.HostID,
			Egress: egressCfg,
		}
		if err := w.provider.PrepareHost(ctx, spec); err != nil {
			w.recordNetworkError(ctx, request.HostID, err)
			return fmt.Errorf("prepare host network after create: %w", err)
		}
	}

	if err := w.waitForSSH(ctx, request, containerName); err != nil {
		return err
	}

	return nil
}

func (w *Worker) startHost(ctx context.Context, request agentapi.HostActionRequest) error {
	if err := w.validateAndPrepare(ctx, request.HostID); err != nil {
		return err
	}

	containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
	if exists, err := w.containerExists(ctx, containerName); err != nil {
		return err
	} else if !exists {
		if err := w.createHost(ctx, request); err != nil {
			return err
		}
	}

	if err := w.runDocker(ctx, "start", containerName); err != nil {
		return err
	}

	egressCfg, err := w.buildEgressConfig(ctx, request.HostID)
	if err != nil {
		return err
	}

	if egressCfg != nil {
		spec := network.HostNetworkSpec{
			HostID: request.HostID,
			Egress: egressCfg,
		}
		if err := w.provider.PrepareHost(ctx, spec); err != nil {
			w.recordNetworkError(ctx, request.HostID, err)
			return fmt.Errorf("prepare host network: %w", err)
		}
	}

	w.repo.RecordEvent(ctx, repository.RecordEventParams{
		HostID:   &request.HostID,
		Level:    "info",
		Type:     "net.ready",
		Message:  "host started",
		Metadata: map[string]any{"host_id": request.HostID},
	})

	if err := w.waitForSSH(ctx, request, containerName); err != nil {
		return err
	}

	return nil
}

func (w *Worker) stopHost(ctx context.Context, request agentapi.HostActionRequest) error {
	containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
	if exists, err := w.containerExists(ctx, containerName); err != nil {
		return err
	} else if !exists {
		return nil
	}

	if err := w.runDocker(ctx, "stop", containerName); err != nil {
		return err
	}

	if err := w.provider.CleanupHost(ctx, network.HostNetworkSpec{HostID: request.HostID}); err != nil {
		return fmt.Errorf("cleanup host network after stop: %w", err)
	}

	return nil
}

func (w *Worker) rebuildHost(ctx context.Context, request agentapi.HostActionRequest) error {
	if err := w.validateAndPrepare(ctx, request.HostID); err != nil {
		return err
	}

	containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
	if err := w.stopHost(ctx, request); err != nil {
		return err
	}
	if exists, err := w.containerExists(ctx, containerName); err != nil {
		return err
	} else if exists {
		if err := w.runDocker(ctx, "rm", "-f", containerName); err != nil {
			return err
		}
	}

	if request.RebuildMode == "wipe-/workspace" {
		if err := os.RemoveAll(firstNonEmpty(request.HomeDir, hostHomeDir(request.HostID))); err != nil {
			return fmt.Errorf("factory reset host home: %w", err)
		}
	}

	if err := w.provider.CleanupHost(ctx, network.HostNetworkSpec{HostID: request.HostID}); err != nil {
		return fmt.Errorf("cleanup host network: %w", err)
	}

	if err := w.createHost(ctx, request); err != nil {
		return err
	}

	w.repo.RecordEvent(ctx, repository.RecordEventParams{
		HostID:   &request.HostID,
		Level:    "info",
		Type:     "net.ready",
		Message:  "host rebuilt",
		Metadata: map[string]any{"host_id": request.HostID},
	})

	if err := w.waitForSSH(ctx, request, containerName); err != nil {
		return err
	}

	return nil
}

func (w *Worker) validateAndPrepare(ctx context.Context, hostID string) error {
	validator := &repoValidator{repo: w.repo}
	_, err := network.ValidateEgressBinding(ctx, validator, hostID)
	if err != nil {
		w.recordNetworkError(ctx, hostID, err)
		return err
	}
	return nil
}

func (w *Worker) buildEgressConfig(ctx context.Context, hostID string) (*network.EgressConfig, error) {
	validator := &repoValidator{repo: w.repo}
	cfg, err := network.ValidateEgressBinding(ctx, validator, hostID)
	if err != nil {
		w.recordNetworkError(ctx, hostID, err)
		return nil, err
	}
	return &cfg, nil
}

func (w *Worker) waitForSSH(ctx context.Context, request agentapi.HostActionRequest, containerName string) error {
	w.repo.RecordEvent(ctx, repository.RecordEventParams{
		TaskID:   &request.TaskID,
		HostID:   &request.HostID,
		Level:    "info",
		Type:     "runtime.validating",
		Message:  "validating SSH readiness",
		Metadata: map[string]any{"host_id": request.HostID, "container": containerName},
	})

	if err := WaitForSSHReady(ctx, containerName, DefaultSSHReadyConfig); err != nil {
		w.repo.RecordEvent(ctx, repository.RecordEventParams{
			TaskID:   &request.TaskID,
			HostID:   &request.HostID,
			Level:    "error",
			Type:     "ssh.failed",
			Message:  err.Error(),
			Metadata: map[string]any{"host_id": request.HostID, "container": containerName},
		})
		return err
	}

	w.syncContainerCredentials(ctx, request, containerName)
	w.injectSSHKeys(ctx, request, containerName)

	w.repo.RecordEvent(ctx, repository.RecordEventParams{
		TaskID:   &request.TaskID,
		HostID:   &request.HostID,
		Level:    "info",
		Type:     "ssh.ready",
		Message:  "SSH port is ready",
		Metadata: map[string]any{"host_id": request.HostID, "container": containerName},
	})

	handoffMeta := BuildSSHHandoffMetadata(request.HostID, "root")
	w.repo.RecordEvent(ctx, repository.RecordEventParams{
		TaskID:   &request.TaskID,
		HostID:   &request.HostID,
		Level:    "info",
		Type:     "ssh.handoff.ready",
		Message:  "SSH handoff metadata ready",
		Metadata: handoffMeta,
	})

	return nil
}

// syncContainerCredentials forces the container's Linux password to match
// the request, regardless of what the entrypoint set via environment variables.
func (w *Worker) syncContainerCredentials(ctx context.Context, request agentapi.HostActionRequest, containerName string) {
	user := firstNonEmpty(request.DefaultUser, "workspace")
	if request.EntryPassword == "" {
		w.repo.RecordEvent(ctx, repository.RecordEventParams{
			HostID:  &request.HostID,
			Level:   "error",
			Type:    "runtime.entry_password_missing",
			Message: "host entry_password is empty; refusing to sync container credentials",
			Metadata: map[string]any{
				"host_id":   request.HostID,
				"container": containerName,
				"source":    "sync",
			},
		})
		return
	}
	pass := request.EntryPassword

	if out, err := execInContainer(ctx, containerName, "chpasswd", fmt.Sprintf("%s:%s\n", user, pass)); err != nil {
		w.repo.RecordEvent(ctx, repository.RecordEventParams{
			HostID:  &request.HostID,
			Level:   "warn",
			Type:    "runtime.password_sync_failed",
			Message: fmt.Sprintf("docker exec chpasswd failed: %s", strings.TrimSpace(string(out))),
		})
	}
}

func (w *Worker) injectSSHKeys(ctx context.Context, request agentapi.HostActionRequest, containerName string) {
	if len(request.SSHKeys) == 0 {
		if request.SSHPublicKey == "" && request.SSHPrivateKey == "" {
			return
		}
		w.injectSSHKeysLegacy(ctx, request, containerName)
		return
	}

	// 必须与容器内 CONTAINER_USER / entrypoint 一致；不能用平台用户名（request.Username）。
	user := firstNonEmpty(request.DefaultUser, "workspace")
	sshDir := "/workspace/.ssh"

	var managed []string
	if proxyPubKey := loadProxyPublicKey(); proxyPubKey != "" {
		managed = append(managed, proxyPubKey)
	}
	for _, key := range request.SSHKeys {
		if key.Purpose == "inbound" && key.PublicKey != "" {
			managed = append(managed, strings.TrimSpace(key.PublicKey))
		}
	}

	authorizedPath := sshDir + "/authorized_keys"
	existing, _ := containerReadFile(ctx, containerName, authorizedPath)
	merged := mergeAuthorizedKeys(existing, managed)

	switch {
	case merged == "":
		// managed 与 existing 均为空，彻底跳过，不创建文件
	case merged == existing:
		// 内容未变，保持幂等；仍尝试修正属主/权限（容错失败）
		w.fixFileOwnership(ctx, request, containerName, user, authorizedPath, "600")
	default:
		writeScript := fmt.Sprintf(
			"mkdir -p %s && cat > %s && chmod 600 %s && chown %s:%s %s",
			sshDir, authorizedPath, authorizedPath, user, user, authorizedPath,
		)
		if out, err := execInContainer(ctx, containerName, writeScript, merged); err != nil {
			w.repo.RecordEvent(ctx, repository.RecordEventParams{
				HostID:  &request.HostID,
				Level:   "warn",
				Type:    "runtime.ssh_authorized_keys_failed",
				Message: fmt.Sprintf("inject authorized_keys failed: %s", strings.TrimSpace(string(out))),
			})
		}
	}

	outboundIdx := 0
	for _, key := range request.SSHKeys {
		if key.Purpose != "outbound" {
			continue
		}

		var keyFile, pubFile string
		if outboundIdx == 0 {
			if key.KeyType == "rsa" || strings.Contains(key.PublicKey, "ssh-rsa") {
				keyFile = sshDir + "/id_rsa"
				pubFile = sshDir + "/id_rsa.pub"
			} else {
				keyFile = sshDir + "/id_ed25519"
				pubFile = sshDir + "/id_ed25519.pub"
			}
		} else {
			safeName := key.Label
			if safeName == "" {
				safeName = fmt.Sprintf("id_%d", outboundIdx)
			}
			keyFile = sshDir + "/" + safeName
			pubFile = sshDir + "/" + safeName + ".pub"
		}

		if key.PrivateKey != "" {
			if containerFileNonEmpty(ctx, containerName, keyFile) {
				w.repo.RecordEvent(ctx, repository.RecordEventParams{
					HostID:   &request.HostID,
					Level:    "info",
					Type:     "runtime.ssh_key_skipped_existing",
					Message:  "outbound private key file already present, skip overwrite",
					Metadata: map[string]any{"host_id": request.HostID, "file": keyFile},
				})
				w.fixFileOwnership(ctx, request, containerName, user, keyFile, "600")
			} else {
				script := fmt.Sprintf(
					"mkdir -p %s && cat > %s && chmod 600 %s && chown %s:%s %s",
					sshDir, keyFile, keyFile, user, user, keyFile,
				)
				if out, err := execInContainer(ctx, containerName, script, key.PrivateKey); err != nil {
					w.repo.RecordEvent(ctx, repository.RecordEventParams{
						HostID:  &request.HostID,
						Level:   "warn",
						Type:    "runtime.ssh_key_inject_failed",
						Message: fmt.Sprintf("inject outbound private key failed: %s", strings.TrimSpace(string(out))),
					})
				}
			}
		}

		if key.PublicKey != "" {
			if containerFileNonEmpty(ctx, containerName, pubFile) {
				w.repo.RecordEvent(ctx, repository.RecordEventParams{
					HostID:   &request.HostID,
					Level:    "info",
					Type:     "runtime.ssh_key_skipped_existing",
					Message:  "outbound public key file already present, skip overwrite",
					Metadata: map[string]any{"host_id": request.HostID, "file": pubFile},
				})
				w.fixFileOwnership(ctx, request, containerName, user, pubFile, "644")
			} else {
				script := fmt.Sprintf(
					"mkdir -p %s && cat > %s && chmod 644 %s && chown %s:%s %s",
					sshDir, pubFile, pubFile, user, user, pubFile,
				)
				if out, err := execInContainer(ctx, containerName, script, key.PublicKey); err != nil {
					w.repo.RecordEvent(ctx, repository.RecordEventParams{
						HostID:  &request.HostID,
						Level:   "warn",
						Type:    "runtime.ssh_key_inject_failed",
						Message: fmt.Sprintf("inject outbound public key failed: %s", strings.TrimSpace(string(out))),
					})
				}
			}
		}

		outboundIdx++
	}
}

func (w *Worker) injectSSHKeysLegacy(ctx context.Context, request agentapi.HostActionRequest, containerName string) {
	user := firstNonEmpty(request.DefaultUser, "workspace")
	sshDir := "/workspace/.ssh"

	if request.SSHPrivateKey != "" {
		keyFile := sshDir + "/id_ed25519"
		if strings.Contains(request.SSHPublicKey, "ssh-rsa") {
			keyFile = sshDir + "/id_rsa"
		}

		if containerFileNonEmpty(ctx, containerName, keyFile) {
			w.repo.RecordEvent(ctx, repository.RecordEventParams{
				HostID:   &request.HostID,
				Level:    "info",
				Type:     "runtime.ssh_key_skipped_existing",
				Message:  "legacy private key file already present, skip overwrite",
				Metadata: map[string]any{"host_id": request.HostID, "file": keyFile},
			})
			w.fixFileOwnership(ctx, request, containerName, user, keyFile, "600")
		} else {
			script := fmt.Sprintf(
				"mkdir -p %s && cat > %s && chmod 600 %s && chown %s:%s %s",
				sshDir, keyFile, keyFile, user, user, keyFile,
			)
			if out, err := execInContainer(ctx, containerName, script, request.SSHPrivateKey); err != nil {
				w.repo.RecordEvent(ctx, repository.RecordEventParams{
					HostID:  &request.HostID,
					Level:   "warn",
					Type:    "runtime.ssh_key_inject_failed",
					Message: fmt.Sprintf("inject private key failed: %s", strings.TrimSpace(string(out))),
				})
			}
		}
	}

	if request.SSHPublicKey != "" {
		pubKeyFile := sshDir + "/id_ed25519.pub"
		if strings.Contains(request.SSHPublicKey, "ssh-rsa") {
			pubKeyFile = sshDir + "/id_rsa.pub"
		}

		if containerFileNonEmpty(ctx, containerName, pubKeyFile) {
			w.repo.RecordEvent(ctx, repository.RecordEventParams{
				HostID:   &request.HostID,
				Level:    "info",
				Type:     "runtime.ssh_key_skipped_existing",
				Message:  "legacy public key file already present, skip overwrite",
				Metadata: map[string]any{"host_id": request.HostID, "file": pubKeyFile},
			})
			w.fixFileOwnership(ctx, request, containerName, user, pubKeyFile, "644")
		} else {
			script := fmt.Sprintf(
				"mkdir -p %s && cat > %s && chmod 644 %s && chown %s:%s %s",
				sshDir, pubKeyFile, pubKeyFile, user, user, pubKeyFile,
			)
			if out, err := execInContainer(ctx, containerName, script, request.SSHPublicKey); err != nil {
				w.repo.RecordEvent(ctx, repository.RecordEventParams{
					HostID:  &request.HostID,
					Level:   "warn",
					Type:    "runtime.ssh_key_inject_failed",
					Message: fmt.Sprintf("inject public key failed: %s", strings.TrimSpace(string(out))),
				})
			}
		}
	}
}

func (w *Worker) recordNetworkError(ctx context.Context, hostID string, err error) {
	var netErr *network.NetworkError
	if errors.As(err, &netErr) {
		_, _ = w.repo.RecordEvent(ctx, repository.RecordEventParams{
			HostID:   &hostID,
			Level:    "error",
			Type:     netErr.EventType(),
			Message:  netErr.Error(),
			Metadata: netErr.EventMetadata(),
		})
	}
}

type repoValidator struct {
	repo WorkerRepo
}

func (rv *repoValidator) GetEgressIPByHost(ctx context.Context, hostID string) (network.EgressIPRecord, error) {
	eip, err := rv.repo.GetEgressIPByHost(ctx, hostID)
	if err != nil {
		return network.EgressIPRecord{}, err
	}
	return network.EgressIPRecord{
		ID:          eip.ID,
		IPAddress:   eip.IPAddress,
		TunnelType:  network.TunnelTypeProxy,
		ProxyConfig: eip.ProxyConfig,
	}, nil
}

// execInContainer 在目标容器中以 `docker exec -i <container> bash -c <script>` 执行，
// 支持可选 stdin。暴露为 package-level 变量以便单元测试注入 fake。
var execInContainer = func(ctx context.Context, container, script, stdin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", container, "bash", "-c", script)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return cmd.CombinedOutput()
}

// containerFileNonEmpty 判断容器内文件是否存在且非空。任何异常一律返回 false，
// 调用方据此走"需要写入"分支，保持原有自举路径可用。
// path 通过 stdin 传入，避免脚本字符串层面做 shell 拼接。
func containerFileNonEmpty(ctx context.Context, container, path string) bool {
	script := `P=$(cat) && [ -s "$P" ] && echo y || echo n`
	out, err := execInContainer(ctx, container, script, path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "y"
}

// containerReadFile 读取容器内文件；不存在或读失败都返回 ("", false)，
// 存在则返回完整原始内容与 true。path 通过 stdin 传入。
func containerReadFile(ctx context.Context, container, path string) (string, bool) {
	script := `P=$(cat) && [ -f "$P" ] && cat "$P" || exit 42`
	out, err := execInContainer(ctx, container, script, path)
	if err != nil {
		return "", false
	}
	return string(out), true
}

// mergeAuthorizedKeys 在已有 authorized_keys 内容与控制面权威条目之间做"marker 块合并"：
//   - existing 为空或不可读：managed 空 → 返回空串（调用方应跳过写入，避免创建空文件）；
//     managed 非空 → 只返回 marker 块包裹的 managed 内容。
//   - existing 非空且包含 marker 对：把 marker 对中间（含 marker）替换为新 marker 块；
//     managed 为空时整段删除，保留 marker 之外的其他行。
//   - existing 非空但没有 marker 对：managed 非空 → 末尾追加新 marker 块；managed 空 → 返回 existing 原样。
//
// 结果保证以 `\n` 结尾（除非整体为空字符串）；不做整体 TrimSpace，以免破坏用户自加行。
func mergeAuthorizedKeys(existing string, managed []string) string {
	if existing == "" {
		if len(managed) == 0 {
			return ""
		}
		return sshManagedBeginMarker + "\n" + strings.Join(managed, "\n") + "\n" + sshManagedEndMarker + "\n"
	}

	lines := strings.Split(existing, "\n")
	beginIdx, endIdx := -1, -1
	for i, line := range lines {
		if beginIdx == -1 && line == sshManagedBeginMarker {
			beginIdx = i
			continue
		}
		if beginIdx != -1 && line == sshManagedEndMarker {
			endIdx = i
			break
		}
	}

	var managedBlock []string
	if len(managed) > 0 {
		managedBlock = append(managedBlock, sshManagedBeginMarker)
		managedBlock = append(managedBlock, managed...)
		managedBlock = append(managedBlock, sshManagedEndMarker)
	}

	var result []string
	if beginIdx != -1 && endIdx != -1 {
		result = append(result, lines[:beginIdx]...)
		result = append(result, managedBlock...)
		result = append(result, lines[endIdx+1:]...)
	} else {
		if len(managed) == 0 {
			return existing
		}
		result = append(result, lines...)
		for len(result) > 0 && result[len(result)-1] == "" {
			result = result[:len(result)-1]
		}
		result = append(result, managedBlock...)
	}

	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}

	if len(result) == 0 {
		return ""
	}
	return strings.Join(result, "\n") + "\n"
}

// fixFileOwnership 只修属主/权限，不重写内容；chown/chmod 失败仅记录 warn 事件，不阻断后续流程。
func (w *Worker) fixFileOwnership(ctx context.Context, request agentapi.HostActionRequest, containerName, user, path, mode string) {
	script := fmt.Sprintf("chown %s:%s %s && chmod %s %s", user, user, path, mode, path)
	if out, err := execInContainer(ctx, containerName, script, ""); err != nil {
		w.repo.RecordEvent(ctx, repository.RecordEventParams{
			HostID:   &request.HostID,
			Level:    "warn",
			Type:     "runtime.ssh_key_chown_failed",
			Message:  fmt.Sprintf("chown %s failed: %s", path, strings.TrimSpace(string(out))),
			Metadata: map[string]any{"host_id": request.HostID, "file": path},
		})
	}
}

func loadProxyPublicKey() string {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/var/lib/cloud-cli-proxy"
	}
	pubKeyPath := dataDir + "/ssh_host_ed25519_key.pub"
	data, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (w *Worker) pullImage(ctx context.Context, imageName string) {
	cmd := exec.CommandContext(ctx, "docker", "pull", imageName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("docker pull failed, will use local image if available",
			"image", imageName, "error", err, "output", strings.TrimSpace(string(output)))
		return
	}
	slog.Info("pulled latest image", "image", imageName)
}

func (w *Worker) containerExists(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "container", "inspect", name)
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() != 0 {
			return false, nil
		}
		return false, fmt.Errorf("inspect docker container %s: %w", name, err)
	}

	return true, nil
}

func (w *Worker) runDocker(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}

	return nil
}

func hostHomeDir(hostID string) string {
	return fmt.Sprintf("%s%s/home", defaultHostRoot, hostID)
}

func containerNameForHost(hostID string) string {
	return fmt.Sprintf("cloudproxy-%s", hostID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func summarizeError(err error) string {
	message := err.Error()
	if len(message) > 160 {
		return message[:160]
	}

	return message
}
