package doctor

import (
	"context"
	"fmt"
	"time"

	"github.com/claudedock/claudedock/internal/claudedock/errcodes"
)

// Check 是单项检查结果（RESEARCH §5.1 JSON tag 严格对齐）。
type Check struct {
	Domain     string         `json:"domain"`
	Name       string         `json:"name"`
	Status     Status         `json:"status"`
	Code       errcodes.Code  `json:"code,omitempty"`
	Message    string         `json:"message,omitempty"`
	NextAction string         `json:"next_action,omitempty"` // CI grep gate 关键字段
	Details    map[string]any `json:"details,omitempty"`
	FixApplied []string       `json:"fix_applied,omitempty"` // Plan 03 填充
	FixFailed  []string       `json:"fix_failed,omitempty"`  // Plan 03 填充
	DurationMS int64          `json:"duration_ms"`
}

// Checker 是单项检查的统一接口；Plan 02 每个维度的 check function 可直接实现也可走 helper。
// Run 负责检测本身；Fix（Plan 03 落）幂等修复。
type Checker interface {
	Run(ctx context.Context, runner RemoteRunner) Check
	Fix(ctx context.Context, opts Options) Check
}

// runWithTimeout 包装单 check 的 context timeout（CONTEXT D-08）：
//   - timeout 命中 → Status=StatusFail + Code=SYSTEM_CHECK_TIMEOUT + 中文 Message
//   - 正常返回 → 保留 fn 返回的 Check
func runWithTimeout(ctx context.Context, domain, name string, timeout time.Duration, fn func(context.Context) Check) Check {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	done := make(chan Check, 1)
	go func() {
		done <- fn(ctx2)
	}()
	select {
	case c := <-done:
		if c.DurationMS == 0 {
			c.DurationMS = time.Since(start).Milliseconds()
		}
		return c
	case <-ctx2.Done():
		return Check{
			Domain:     domain,
			Name:       name,
			Status:     StatusFail,
			Code:       errcodes.SYSTEM_CHECK_TIMEOUT,
			Message:    "检查超时（" + timeout.String() + "）",
			NextAction: "加 --verbose 放宽到 30s，或检查远端容器状态",
			DurationMS: time.Since(start).Milliseconds(),
		}
	}
}

// newPass / newWarn / newFail / newSkip 是 check 函数用的 constructor helpers（减少 verbosity）。
// Plan 02 各维度文件直接调用；Code 可空（Pass/Skip 允许不带 Code）。
func newPass(domain, name, msg string) Check {
	return Check{Domain: domain, Name: name, Status: StatusPass, Message: msg}
}

func newWarn(domain, name string, code errcodes.Code, args ...any) Check {
	entry, _ := errcodes.Lookup(code)
	msg := entry.Message
	if len(args) > 0 {
		msg = fmt.Sprintf(entry.Message, args...)
	}
	return Check{Domain: domain, Name: name, Status: StatusWarn, Code: code, Message: msg, NextAction: entry.NextAction}
}

func newFail(domain, name string, code errcodes.Code, args ...any) Check {
	entry, _ := errcodes.Lookup(code)
	msg := entry.Message
	if len(args) > 0 {
		msg = fmt.Sprintf(entry.Message, args...)
	}
	return Check{Domain: domain, Name: name, Status: StatusFail, Code: code, Message: msg, NextAction: entry.NextAction}
}

func newSkip(domain, name, reason string) Check {
	return Check{Domain: domain, Name: name, Status: StatusSkip, Message: reason}
}
