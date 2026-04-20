package cloudclaude

import (
	"context"
	"errors"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// RunKeepAlive 在 conn 上每 interval 发一次 SSH 全局 keepalive 请求；
// 连续 countMax 次失败（含单次 timeout）后 conn.Close() 让上层 reconnect 感知。
// interval 必须 >= 15s（启动期已校验；本函数仍做 defensive check）。
// 单次 SendRequest 必须包 timeout（goroutine + select <-time.After(interval)），
// 否则 dead network 上 SendRequest 永久阻塞、失败计数永远不增长（RESEARCH §1.2 [ASSUMED]）。
func RunKeepAlive(ctx context.Context, conn ssh.Conn, interval time.Duration, countMax int) error {
	if interval < 15*time.Second {
		return errors.New("keepalive interval 必须 >= 15s")
	}
	if countMax <= 0 {
		countMax = 4
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	fails := 0
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ok, err := sendKeepaliveWithTimeout(conn, interval)
			if err == nil && ok {
				fails = 0
				lastErr = nil
				continue
			}
			fails++
			lastErr = err
			if fails >= countMax {
				_ = conn.Close()
				return lastErr
			}
		}
	}
}

// sendKeepaliveWithTimeout 单次 SendRequest 的 timeout 包装。
// SendRequest 在 dead network 上会永久阻塞（RESEARCH §1.2），必须由调用方提供 timeout。
func sendKeepaliveWithTimeout(conn ssh.Conn, timeout time.Duration) (bool, error) {
	type result struct {
		ok  bool
		err error
	}
	ch := make(chan result, 1)
	go func() {
		_, _, err := conn.SendRequest("keepalive@openssh.com", true, nil)
		ch <- result{ok: err == nil, err: err}
	}()
	select {
	case <-time.After(timeout):
		return false, errors.New("keepalive timeout")
	case r := <-ch:
		return r.ok, r.err
	}
}

// ConfigureTCPKeepAlive 在 sshConnect 拨号成功后立即调用：
//  1. tcpConn.SetKeepAlive(true)
//  2. tcpConn.SetKeepAlivePeriod(period)（period=15s）
//  3. configurePlatformSpecific(tcpConn) — Linux 设 TCP_USER_TIMEOUT=30000ms / macOS 设 TCP_KEEPALIVE=15s / 其它 noop+warning
//
// 任一步骤失败仅返回 error，调用方 stderr 打 NET_TCP_KEEPALIVE_UNSUPPORTED 警告，
// 不阻塞连接建立（CONTEXT D-04 第 4 条 best-effort）。
func ConfigureTCPKeepAlive(tcpConn *net.TCPConn, period time.Duration) error {
	if err := tcpConn.SetKeepAlive(true); err != nil {
		return err
	}
	if err := tcpConn.SetKeepAlivePeriod(period); err != nil {
		return err
	}
	return configurePlatformSpecific(tcpConn)
}
