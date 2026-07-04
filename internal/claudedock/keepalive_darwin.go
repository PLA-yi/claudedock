//go:build darwin

package claudedock

import (
	"net"
	"syscall"
)

// tcpKeepalive = Darwin <netinet/tcp.h> TCP_KEEPALIVE 0x10。
// Go stdlib SetKeepAlivePeriod 在 darwin 已经走该 sockopt（行为重复但保留 hook 方便日后加 TCP_KEEPCNT/INTVL）。
const tcpKeepalive = 0x10

func configurePlatformSpecific(tcpConn *net.TCPConn) error {
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return err
	}
	var sockErr error
	if cErr := rawConn.Control(func(fd uintptr) {
		sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpKeepalive, 15)
	}); cErr != nil {
		return cErr
	}
	return sockErr
}
