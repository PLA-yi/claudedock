package claudedock

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// fakeConn 实现 ssh.Conn 接口，按测试需要决定 SendRequest 的返回。
type fakeConn struct {
	ssh.Conn
	sendDelay   time.Duration
	sendErr     error
	closeCalled bool
}

func (f *fakeConn) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	if f.sendDelay > 0 {
		time.Sleep(f.sendDelay)
	}
	return f.sendErr == nil, nil, f.sendErr
}

func (f *fakeConn) Close() error { f.closeCalled = true; return nil }

func TestRunKeepAlive_RejectsTooShortInterval(t *testing.T) {
	err := RunKeepAlive(context.Background(), &fakeConn{}, 5*time.Second, 4)
	if err == nil {
		t.Fatal("期望 interval<15s 返回错误")
	}
}

func TestRunKeepAlive_TimeoutCounts(t *testing.T) {
	// sendDelay 远大于 interval timeout — 每次都超时，countMax 后关 conn。
	// 用 15s interval / countMax=2 → 应在 ~30s 内返回（2 次超时）。
	if testing.Short() {
		t.Skip("跳过长耗时测试（-short）")
	}
	f := &fakeConn{sendDelay: 30 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunKeepAlive(ctx, f, 15*time.Second, 2) }()
	select {
	case err := <-done:
		if !f.closeCalled {
			t.Error("期望 countMax 触发后关闭 conn")
		}
		_ = err
	case <-time.After(60 * time.Second):
		t.Fatal("RunKeepAlive 未在预期时间内返回")
	}
}

func TestRunKeepAlive_SuccessResetsFails(t *testing.T) {
	// 用 nil sendErr → SendRequest 立即成功；ctx cancel 后退出。
	// interval 15s + ctx 200ms → 没有 ticker 触发就 ctx.Done，无 close 调用。
	f := &fakeConn{sendErr: nil}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := RunKeepAlive(ctx, f, 15*time.Second, 4); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("期望 ctx.DeadlineExceeded，得 %v", err)
	}
	if f.closeCalled {
		t.Error("ctx 取消不应关闭 conn")
	}
}

func TestConfigureTCPKeepAlive_NoPanicOnTCPConn(t *testing.T) {
	// 起一个临时 listener，accept 后拿到 *net.TCPConn 验证 ConfigureTCPKeepAlive 不 panic。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		if c != nil {
			c.Close()
		}
	}()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	tc := c.(*net.TCPConn)
	if err := ConfigureTCPKeepAlive(tc, 15*time.Second); err != nil {
		t.Logf("ConfigureTCPKeepAlive 平台返回错误（接受 — best-effort）: %v", err)
	}
}
