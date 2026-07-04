//go:build e2e && linux

// helpers_test.go v4.0 (Phase 55): 单容器架构下删除 gatewayInspectName，
// worker 容器即 user 容器（sing-box 内置）。

package killswitch_stress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	e2e "github.com/claudedock/claudedock/tests/e2e"
	"github.com/claudedock/claudedock/tests/e2e/harness"
)

// workerInspectName 反推 worker/user 容器名。
// v4.0 (Phase 55): 单容器架构下 worker = user 容器（sing-box 内置），
// gatewayInspectName 已删除。
func workerInspectName(_ context.Context, g *e2e.GoldenPath) (string, error) {
	if g == nil || g.Host == nil {
		return "", errors.New("container name: host handle nil")
	}
	if name := strings.TrimSpace(g.Host.ContainerName); name != "" {
		return name, nil
	}
	if g.Host.ID != "" {
		return "claudedock-" + g.Host.ID, nil
	}
	return "", errors.New("container name: host.ID empty (scenario step 7 未实现)")
}

// dockerExecHandle 是 ContainerHandle 接口的最小实现。
type dockerExecHandle struct {
	name string
}

func (h *dockerExecHandle) Logs(ctx context.Context) (io.ReadCloser, error) {
	return nil, errors.New("dockerExecHandle: Logs not implemented")
}

func (h *dockerExecHandle) Exec(ctx context.Context, argv []string) (int, io.Reader, error) {
	full := append([]string{"exec", h.name}, argv...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	runErr := cmd.Run()
	if runErr == nil {
		return 0, &stdout, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode(), &stdout, nil
	}
	return -1, &stdout, fmt.Errorf("docker exec %s: %w", h.name, runErr)
}

func newWorkerExecHandle(containerName string) harness.ContainerHandle {
	return &dockerExecHandle{name: containerName}
}

var _ = newWorkerExecHandle
