//go:build linux

package claudedock

import (
	"net"
	"syscall"
)

// tcpUserTimeout = TCP_USER_TIMEOUT；syscall pkg 不导出此常量。
// RFC 793 + Linux 2.6.37+ tcp(7) 定义为 18。
const tcpUserTimeout = 18

func configurePlatformSpecific(tcpConn *net.TCPConn) error {
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return err
	}
	var sockErr error
	if cErr := rawConn.Control(func(fd uintptr) {
		// 30000ms：与 ROADMAP §Phase 32 SC2 30s 抖动验收对齐；
		// 客户端先于服务端 (8×15s=120s) 宣告"断"。
		sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpUserTimeout, 30000)
	}); cErr != nil {
		return cErr
	}
	return sockErr
}
