//go:build !linux && !darwin

package cloudclaude

import (
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// configurePlatformSpecific 在 !linux && !darwin 平台为空操作。
// SetKeepAlive + SetKeepAlivePeriod 已经被公共 ConfigureTCPKeepAlive 调用过（stdlib 跨平台生效）。
// 平台特化优化跳过；输出 warning 但不阻塞（CONTEXT D-04 第 4 条 best-effort）。
func configurePlatformSpecific(tcpConn *net.TCPConn) error {
	fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.NET_TCP_KEEPALIVE_UNSUPPORTED, runtime.GOOS))
	_ = tcpConn
	return nil
}
