package claudedock

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/claudedock/claudedock/internal/claudedock/errcodes"
)

// SSHFSWatcher 后台 goroutine：在 conn-A 上每 interval 检查 mountpoint，
// 连续 failureLimit 次失败后输出 MOUNT_SSHFS_DISCONNECTED 警告 + 调 onDisconnect（mergerfs branch 摘除）。
//
// 设计（CONTEXT D-27 + RESEARCH §2.3）：
//   - interval 默认 5s，failureLimit 默认 3（≥ 15s 抖动判定）
//   - 检测命令：timeout 2 mountpoint -q <coldPath>，exit 0 = mounted
//   - 触发 onDisconnect 后立即 return（不重入，避免无限循环）；恢复留给 Phase 34 doctor --fix
//   - ctx cancel 即停止；mount_strategy cleanup 时 cancel
//
// check 字段为函数注入，方便单测无需真实 ssh.Client；
// 默认指向 checkOnce（remote timeout 2 mountpoint）。
type SSHFSWatcher struct {
	conn         *ssh.Client
	coldPath     string
	interval     time.Duration
	failureLimit int
	logger       io.Writer
	onDisconnect func() error
	check        func() bool
}

// NewSSHFSWatcher 构造默认 5s / 3 次失败的 watcher，绑定真实 mountpoint check。
func NewSSHFSWatcher(conn *ssh.Client, coldPath string, logger io.Writer, onDisconnect func() error) *SSHFSWatcher {
	w := &SSHFSWatcher{
		conn:         conn,
		coldPath:     coldPath,
		interval:     5 * time.Second,
		failureLimit: 3,
		logger:       logger,
		onDisconnect: onDisconnect,
	}
	w.check = w.checkOnce
	return w
}

// Run 阻塞运行；调用方应放在 goroutine 里。
// 触发 onDisconnect 或 ctx.Done 后返回。
func (w *SSHFSWatcher) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if w.check() {
				failures = 0
				continue
			}
			failures++
			if failures >= w.failureLimit {
				if w.logger != nil {
					fmt.Fprintln(w.logger, errcodes.Format(errcodes.MOUNT_SSHFS_DISCONNECTED))
				}
				if w.onDisconnect != nil {
					_ = w.onDisconnect()
				}
				return
			}
		}
	}
}

// checkOnce 在 conn 上执行 timeout 2 mountpoint -q <coldPath>。
// exit 0 = 已挂载；conn 为 nil 视为未挂载（测试场景）。
func (w *SSHFSWatcher) checkOnce() bool {
	if w.conn == nil {
		return false
	}
	cmd := fmt.Sprintf("timeout 2 mountpoint -q %s", shellQuote(w.coldPath))
	return sshRun(w.conn, cmd) == nil
}
