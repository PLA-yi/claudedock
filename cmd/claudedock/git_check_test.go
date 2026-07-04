package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestRequireGitRepo_InGitRepo 用当前 workspace（claudedock 本身是 git 仓库）
// 验证 git 仓库目录通过检查。
func TestRequireGitRepo_InGitRepo(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd 失败: %v", err)
	}
	if err := requireGitRepo(cwd); err != nil {
		t.Errorf("git 仓库目录应 pass，got error: %v", err)
	}
}

// TestRequireGitRepo_NotAGitRepo 用临时空目录验证非 git 仓库会返回带 MOUNT_REQUIRE_GIT_REPO
// 的格式化错误。
func TestRequireGitRepo_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	err := requireGitRepo(dir)
	if err == nil {
		t.Fatal("非 git 仓库目录应返回 error，got nil")
	}
	if !strings.Contains(err.Error(), "MOUNT_REQUIRE_GIT_REPO") {
		t.Errorf("error 消息应含 MOUNT_REQUIRE_GIT_REPO，got: %v", err)
	}
}

// TestRequireGitRepo_GitUnavailableHandled 通过把 PATH 截断为空，模拟 git 不可用。
// D-03：git 二进制不可用与「非 git 仓库」走同一错误码路径。
func TestRequireGitRepo_GitUnavailableHandled(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("当前环境无 git，本用例已天然进入「不可用」分支，跳过显式 PATH 截断。")
	}
	t.Setenv("PATH", "")

	dir := t.TempDir()
	err := requireGitRepo(dir)
	if err == nil {
		t.Fatal("git 不可用时应返回 error（按非 git 仓库处理，D-03）")
	}
	if !strings.Contains(err.Error(), "MOUNT_REQUIRE_GIT_REPO") {
		t.Errorf("error 消息应含 MOUNT_REQUIRE_GIT_REPO，got: %v", err)
	}
}
