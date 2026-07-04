// cmd/claudedock/doctor.go — Phase 34 Plan 02 Task 2.11
//
// claudedock doctor [domain] 子命令：五维度自检。
// 与 claudedock ssh doctor（v2.0 quick task 入口）双入口共存（CONTEXT D-04）。
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/claudedock/claudedock/internal/claudedock/doctor"
)

const (
	doctorExitOK   = 0
	doctorExitWarn = 1
	doctorExitFail = 2
)

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor [domain]",
		Short: "claudedock 六维度自检（network/auth/ssh/mount/disk/remote-ssh）",
		Long: "运行 claudedock doctor 检测当前环境健康度，每项输出 [符号] + 中文原因 + 建议 + 错误码。\n" +
			"支持 --fix 自动修复（Plan 03 落实）、--json 脚本消费、--verbose 详细日志、--yes 跳过交互确认。\n" +
			"维度：network / auth / ssh / mount / disk / remote-ssh / all。\n" +
			"退出码：0 全部通过 / 1 含 warn 无 fail / 2 含 fail（与 brew doctor 对齐）。",
		Args:          cobra.MaximumNArgs(1),
		ValidArgs:     []string{"network", "auth", "ssh", "mount", "disk", "remote-ssh", "all"},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runDoctor,
	}
	cmd.Flags().Bool("fix", false, "自动修复检测到的问题（Plan 03 落实）")
	cmd.Flags().Bool("verbose", false, "展开探测细节 + 放宽 timeout 到 30s")
	cmd.Flags().Bool("json", false, "输出 JSON 供脚本消费")
	cmd.Flags().Bool("yes", false, "跳过交互式 y/N 确认（CI 友好）")
	return cmd
}

func runDoctor(cmd *cobra.Command, args []string) error {
	domain := "all"
	if len(args) == 1 {
		domain = args[0]
	}

	fix, _ := cmd.Flags().GetBool("fix")
	verbose, _ := cmd.Flags().GetBool("verbose")
	jsonOut, _ := cmd.Flags().GetBool("json")
	yes, _ := cmd.Flags().GetBool("yes")

	opts := doctor.Options{
		Domain:       domain,
		Fix:          fix,
		Verbose:      verbose,
		JSON:         jsonOut,
		NoColor:      os.Getenv("NO_COLOR") != "",
		Yes:          yes,
		CheckTimeout: 0, // doctor 包自选默认
	}

	ctx, cancel := contextWithDoctorTimeout(cmd.Context(), verbose)
	defer cancel()

	report, err := doctor.RunDoctor(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误: "+err.Error())
		os.Exit(exitInternalError)
		return nil
	}

	// Plan 03：--fix 后跑 FixerRegistry，结果写回 Check.FixApplied/FixFailed
	// （Status 不降级 / CONTEXT D-16；退出码仍按 Summary.Fail/Warn 计）
	if fix && anyFixerRegistered() {
		var totalApplied, totalFailed int
		report.Checks = doctor.ApplyFixes(ctx, opts, report.Checks)
		for _, c := range report.Checks {
			totalApplied += len(c.FixApplied)
			totalFailed += len(c.FixFailed)
		}
		if totalApplied+totalFailed > 0 && !jsonOut {
			fmt.Fprintf(os.Stdout, "[fix] %d 项已修复 / %d 项修复失败\n\n", totalApplied, totalFailed)
		}
	}

	if jsonOut {
		raw, err := doctor.RenderJSON(report)
		if err != nil {
			fmt.Fprintln(os.Stderr, "JSON 序列化失败: "+err.Error())
			os.Exit(exitInternalError)
			return nil
		}
		fmt.Println(string(raw))
	} else {
		fmt.Print(doctor.RenderText(report, opts.NoColor))
	}

	// 退出码按 Summary 计（CONTEXT D-16：修复成功的 fail 不降级为 0）
	switch {
	case report.Summary.Fail > 0:
		os.Exit(doctorExitFail)
	case report.Summary.Warn > 0:
		os.Exit(doctorExitWarn)
	default:
		os.Exit(doctorExitOK)
	}
	return nil
}

// contextWithDoctorTimeout 顶层 timeout：verbose 2min，默认 60s（足够跑完 17 项 + ensureRemote）。
func contextWithDoctorTimeout(parent context.Context, verbose bool) (context.Context, context.CancelFunc) {
	if verbose {
		return context.WithTimeout(parent, 2*time.Minute)
	}
	return context.WithTimeout(parent, 60*time.Second)
}

// anyFixerRegistered 检查 doctor.FixerRegistry 是否已注册任何 Fixer（Plan 03 完成后恒 true）。
func anyFixerRegistered() bool {
	return len(doctor.FixerRegistry) > 0
}
