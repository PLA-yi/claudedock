package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
)

// newSyncCmd 构造 cloud-claude sync 父命令树。
//
// Phase 31 Plan 03 仅注册 sync conflicts 子命令（CONTEXT D-28 — 最小可行版本）；
// sync resolve / sync resume 等 wrapper 由 v3.1 落地（OOS-A2 不暴露 5 种冲突模式）。
func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sync",
		Short:         "Mutagen 同步管理（v3.0 三层文件映射）",
		Long:          "查看本地 Mutagen 客户端管理的 cloud-claude 同步会话与冲突文件清单。\n注：当前仅实现 sync conflicts 子命令；sync resolve / sync resume 留 v3.1。",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	conflictsCmd := &cobra.Command{
		Use:           "conflicts",
		Short:         "查看当前 Mutagen 同步会话的冲突文件清单",
		Long:          "调用本地 Mutagen 客户端 sync list --long 渲染所有 cloud-claude 创建的 sync session 的冲突文件（path / alpha / beta / mtime）。",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runSyncConflicts,
	}
	cmd.AddCommand(conflictsCmd)
	return cmd
}

// runSyncConflicts 执行 ~/.cloud-claude/bin/mutagen sync list --long 并把结果直接打到
// stdout / stderr。本地命令出错时返回中文错误，由 cobra runE 错误处理流程 stderr 打印。
//
// 行为约束：
//   - 不发起 SSH 连接（本子命令只查本地 mutagen daemon 状态，不需要联网）
//   - ExtractMutagenBinary 幂等保证 ~/.cloud-claude/bin/mutagen 存在
//   - MUTAGEN_DATA_DIRECTORY 环境变量隔离与主流程一致（CONTEXT D-05）
func runSyncConflicts(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("无法获取用户主目录: %w", err)
	}

	binPath := filepath.Join(home, ".cloud-claude", "bin", "mutagen")
	if err := cloudclaude.ExtractMutagenBinary(binPath); err != nil {
		return fmt.Errorf("无法准备 Mutagen 二进制: %w", err)
	}

	env := append(os.Environ(),
		"MUTAGEN_DATA_DIRECTORY="+filepath.Join(home, ".cloud-claude", "mutagen"),
	)

	// sync list --long 输出含每个 session 的冲突详细信息（path / alpha / beta / mtime）
	c := exec.Command(binPath, "sync", "list", "--long")
	c.Env = env
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("查询 Mutagen 冲突清单失败: %w", err)
	}
	return nil
}
