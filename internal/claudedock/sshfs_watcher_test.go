package claudedock

import (
	"bytes"
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestWatcher 构造一个不绑定 ssh.Client 的 watcher，便于单测。
func newTestWatcher(check func() bool, onDisconnect func() error, buf *bytes.Buffer) *SSHFSWatcher {
	w := &SSHFSWatcher{
		coldPath:     "/workspace-cold",
		interval:     10 * time.Millisecond,
		failureLimit: 3,
		logger:       buf,
		onDisconnect: onDisconnect,
		check:        check,
	}
	return w
}

func Test_SSHFSWatcher_OnDisconnectCalledAfter3Failures(t *testing.T) {
	var disconnects atomic.Int32
	var calls atomic.Int32
	check := func() bool {
		calls.Add(1)
		return false
	}
	onDisc := func() error {
		disconnects.Add(1)
		return nil
	}

	buf := &bytes.Buffer{}
	w := newTestWatcher(check, onDisc, buf)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	if got := disconnects.Load(); got != 1 {
		t.Errorf("disconnect 调用次数 = %d, want 1", got)
	}
	if got := calls.Load(); got < 3 {
		t.Errorf("check 调用次数 = %d, want >= 3", got)
	}
	if !strings.Contains(buf.String(), "MOUNT_SSHFS_DISCONNECTED") {
		t.Errorf("stderr 缺少 MOUNT_SSHFS_DISCONNECTED 行: %q", buf.String())
	}
}

func Test_SSHFSWatcher_RecoverResetsCounter(t *testing.T) {
	var disconnects atomic.Int32

	var step atomic.Int32
	// 循环模式：fail, fail, ok, fail, fail, ok, ...
	// 每 3 次中至少一次成功 → counter 永远 ≤ 2，不触发 disconnect。
	check := func() bool {
		s := step.Add(1)
		return (s % 3) == 0
	}

	buf := &bytes.Buffer{}
	w := newTestWatcher(check, func() error {
		disconnects.Add(1)
		return nil
	}, buf)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	if got := disconnects.Load(); got != 0 {
		t.Errorf("disconnect 调用次数 = %d, want 0（成功后 counter 应该重置）", got)
	}
}

func Test_SSHFSWatcher_CtxCancelStops(t *testing.T) {
	check := func() bool { return true }
	buf := &bytes.Buffer{}
	w := newTestWatcher(check, nil, buf)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("ctx cancel 后 Run 未在 200ms 内退出")
	}
}
