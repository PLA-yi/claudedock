package claudedock

import (
	"fmt"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/claudedock/claudedock/internal/claudedock/errcodes"
)

// mountSSHFS 在 SSH 连接上开启 sshfs session 并启动嵌入式 SFTP server，
// 将 localDir 映射到容器内 remotePath（sshfs / cold 兜底唯一实现）。
//
// 与 v2.0 mountWorkspace 的唯一差异：sshfs 命令追加四个抗抖参数
// reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10
// 实现 PITFALLS C3 防御。
func mountSSHFS(conn *ssh.Client, localDir, remotePath string) (cleanup func(), err error) {
	cleanupStaleFUSE(conn, remotePath)

	mkdirCmd := fmt.Sprintf(
		"sudo mkdir -p %s && sudo chown $(id -u):$(id -g) %s",
		shellQuote(remotePath), shellQuote(remotePath),
	)
	if err := sshRun(conn, mkdirCmd); err != nil {
		return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, "创建远端挂载目录失败"), err)
	}

	sshfsSession, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, "创建 sshfs session 失败"), err)
	}

	stdin, err := sshfsSession.StdinPipe()
	if err != nil {
		sshfsSession.Close()
		return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, "获取 sshfs stdin pipe 失败"), err)
	}

	stdout, err := sshfsSession.StdoutPipe()
	if err != nil {
		stdin.Close()
		sshfsSession.Close()
		return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, "获取 sshfs stdout pipe 失败"), err)
	}

	sshfsCmd := fmt.Sprintf(
		"sshfs : %s -o passive,reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10,cache=yes,kernel_cache,auto_cache,cache_timeout=300 -f",
		shellQuote(remotePath),
	)
	if err := sshfsSession.Start(sshfsCmd); err != nil {
		stdin.Close()
		sshfsSession.Close()
		return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, "启动 sshfs 失败"), err)
	}

	rwc := &channelRWC{Reader: stdout, WriteCloser: stdin}

	server, err := sftp.NewServer(rwc, sftp.WithServerWorkingDirectory(localDir))
	if err != nil {
		stdin.Close()
		sshfsSession.Close()
		return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, "创建 SFTP server 失败"), err)
	}

	sftpDone := make(chan error, 1)
	go func() {
		sftpDone <- server.Serve()
	}()

	checkCmd := fmt.Sprintf("mountpoint -q %s", shellQuote(remotePath))
	check := func() error {
		sess, err := conn.NewSession()
		if err != nil {
			return err
		}
		defer sess.Close()
		return sess.Run(checkCmd)
	}

	if err := waitForMount(remotePath, check, 200*time.Millisecond, 10*time.Second); err != nil {
		sshfsSession.Close()
		<-sftpDone
		server.Close()
		fusermountCleanup(conn, remotePath)
		return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, "等待挂载就绪失败"), err)
	}

	cleanup = func() {
		sshfsSession.Close()
		<-sftpDone
		server.Close()
		fusermountCleanup(conn, remotePath)
		rmdirChain(conn, remotePath)
	}
	return cleanup, nil
}

