package main

import (
	"fmt"
	"os/exec"

	"github.com/claudedock/claudedock/internal/claudedock/errcodes"
)

// requireGitRepo 在 cwd 执行 git rev-parse --show-toplevel：
// 退出码 0 视为 git 仓库（含 worktree、submodule、detached HEAD）；
// 非 0（含 git 二进制不可用）按 D-03 统一按「非 git 仓库」处理，返回包含
// MOUNT_REQUIRE_GIT_REPO 两段格式化消息的 error。
func requireGitRepo(cwd string) error {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", errcodes.Format(errcodes.MOUNT_REQUIRE_GIT_REPO, cwd))
	}
	return nil
}
