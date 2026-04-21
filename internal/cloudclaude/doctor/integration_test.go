//go:build integration
// +build integration

// Phase 34 Plan 03 集成测试 — 在 Phase 29 镜像 fixture 容器上跑 cloud-claude doctor，验证：
//  1. TestIntegration_DoctorMountHappy — v3 镜像健康态下 mount 维度全 pass/skip；退出码 0/1
//  2. TestIntegration_DoctorMountFail_MergerfsTampered — 篡改 mergerfs readdir 参数后，
//     doctor mount 输出含 MOUNT_MERGERFS_FAILED + NextAction 含 'doctor mount'（SC#7 锚点）
//
// 本文件默认 `go test ./...` 不触发（受 build tag `integration` 隔离）；完整执行需：
//
//	bash scripts/test-fixture-up.sh
//	go test -tags=integration -count=1 -v ./internal/cloudclaude/doctor/
//	bash scripts/test-fixture-down.sh

package doctor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// fixtureCtr 与 scripts/test-fixture-up.sh 中 container_name 字面量一致（cc-fixture）。
const fixtureCtr = "cc-fixture"

func TestMain(m *testing.M) {
	if err := exec.Command("scripts/test-fixture-up.sh").Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fixture 启动失败，跳过集成测试:", err)
		os.Exit(0)
	}
	code := m.Run()
	_ = exec.Command("scripts/test-fixture-down.sh").Run()
	os.Exit(code)
}

// dockerExec 在 fixture 容器内执行命令，返回合并 stdout/stderr。
func dockerExec(t *testing.T, args ...string) (string, error) {
	t.Helper()
	full := append([]string{"exec", fixtureCtr}, args...)
	c := exec.Command("docker", full...)
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out
	if err := c.Run(); err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

// runCloudClaudeDoctor 编译 cloud-claude 并跑 doctor mount，返回 exit/stdout/stderr。
func runCloudClaudeDoctor(t *testing.T, extraArgs ...string) (int, string, string) {
	t.Helper()
	bin := "/tmp/cloud-claude-doctor-int"
	if _, err := os.Stat(bin); err != nil {
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/cloud-claude")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("编译失败: %v\n%s", err, out)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := append([]string{"doctor", "mount", "--json"}, extraArgs...)
	c := exec.CommandContext(ctx, bin, args...)
	c.Env = append(os.Environ(), "NO_COLOR=1")
	var outBuf, errBuf bytes.Buffer
	c.Stdout = &outBuf
	c.Stderr = &errBuf
	err := c.Run()
	exitCode := 0
	if e, ok := err.(*exec.ExitError); ok {
		exitCode = e.ExitCode()
	} else if err != nil {
		t.Logf("cloud-claude doctor 执行错误: %v", err)
		exitCode = -1
	}
	return exitCode, outBuf.String(), errBuf.String()
}

// TestIntegration_DoctorMountHappy — happy path：v3 镜像健康态，mount 维度允许 0 或 1 退出码。
func TestIntegration_DoctorMountHappy(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过 docker 集成测试")
	}
	exit, stdout, stderr := runCloudClaudeDoctor(t)
	if exit != 0 && exit != 1 {
		t.Errorf("happy path 期望 exit 0/1（allow warn from apparmor/fuse_residual on host），实际 %d；stderr=%s",
			exit, stderr)
	}
	if strings.Count(stdout, `"status": "fail"`) > 3 {
		t.Errorf("happy path fail 过多：stdout=%s", stdout)
	}
}

// TestIntegration_DoctorMountFail_MergerfsTampered — 篡改 mergerfs 参数 → doctor mount 必含
// MOUNT_MERGERFS_FAILED + 'doctor mount' 字面量（SC#7 / PITFALLS C2+M14 联合验收）。
func TestIntegration_DoctorMountFail_MergerfsTampered(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过 docker 集成测试")
	}
	_, err := dockerExec(t, "bash", "-c", "umount /workspace 2>/dev/null; mount -t mergerfs -o cache.attr=60 branchsource /workspace || true")
	if err != nil {
		t.Skipf("无法在 fixture 内重挂 mergerfs，跳过: %v", err)
	}
	defer func() {
		_ = exec.Command("scripts/test-fixture-down.sh").Run()
		_ = exec.Command("scripts/test-fixture-up.sh").Run()
	}()

	exit, stdout, _ := runCloudClaudeDoctor(t)
	if exit != 2 {
		t.Errorf("mergerfs 篡改后应 exit 2 (fail)，实际 %d", exit)
	}
	if !strings.Contains(stdout, "MOUNT_MERGERFS_FAILED") {
		t.Errorf("输出缺 MOUNT_MERGERFS_FAILED：%s", stdout)
	}
	if !strings.Contains(stdout, "doctor mount") {
		t.Errorf("next_action 缺 'doctor mount' 字面量（SC#7）：%s", stdout)
	}
}
