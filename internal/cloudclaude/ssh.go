package cloudclaude

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"al.essio.dev/pkg/shellescape"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type SSHConfig struct {
	Host     string
	Port     int
	User     string
	Password string
}

// ConnectAndRunClaude 建立 SSH 连接，将本地 cwd 通过 sshfs 映射到容器内同路径，
// 启动本地命令代理，然后在远端执行 claude。
// defer 顺序：stop exec proxy → cleanup mount → close conn（LIFO）。
func ConnectAndRunClaude(cfg SSHConfig, claudeArgs []string, cwd string, proxyCommands []string) (int, error) {
	conn, err := sshConnect(cfg)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	cleanupMount, err := mountWorkspace(conn, cwd, cwd)
	if err != nil {
		return 0, fmt.Errorf("目录映射失败: %w", err)
	}
	defer cleanupMount()

	var proxy *ExecProxy
	if len(proxyCommands) > 0 {
		proxy = NewExecProxy(cwd)
		if err := proxy.Start(); err != nil {
			return 0, fmt.Errorf("启动命令代理失败: %w", err)
		}
		defer proxy.Stop()

		if err := InstallWrappers(cwd, proxyCommands, cwd); err != nil {
			return 0, fmt.Errorf("安装命令代理脚本失败: %w", err)
		}
	}

	return runClaude(conn, claudeArgs, cwd, len(proxyCommands) > 0)
}

func sshConnect(cfg SSHConfig) (*ssh.Client, error) {
	clientCfg := &ssh.ClientConfig{
		User: cfg.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(cfg.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	tcpConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败（无法连接 %s）: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientCfg)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("SSH 握手失败: %w", err)
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
}

func runClaude(conn *ssh.Client, claudeArgs []string, remoteCwd string, hasProxy bool) (int, error) {
	session, err := conn.NewSession()
	if err != nil {
		return 0, fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer session.Close()

	fd := int(os.Stdin.Fd())
	isTTY := term.IsTerminal(fd)

	if isTTY {
		width, height := 80, 24
		if w, h, err := term.GetSize(fd); err == nil {
			width, height = w, h
		}

		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return 0, fmt.Errorf("设置终端 raw 模式失败: %w", err)
		}
		defer term.Restore(fd, oldState)

		modes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}

		if err := session.RequestPty("xterm-256color", height, width, modes); err != nil {
			return 0, fmt.Errorf("申请 PTY 失败: %w", err)
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)
		go func() {
			for range sigCh {
				if w, h, err := term.GetSize(fd); err == nil {
					_ = session.WindowChange(h, w)
				}
			}
		}()
		defer signal.Stop(sigCh)
	}

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	claudeCmd := shellescape.QuoteCommand(append([]string{"claude"}, claudeArgs...))
	var remoteCmd string
	if hasProxy {
		binDir := remoteCwd + "/.cloud-claude/bin"
		remoteCmd = fmt.Sprintf("export PATH=%s:$PATH && cd %s && %s",
			shellescape.Quote(binDir), shellescape.Quote(remoteCwd), claudeCmd)
	} else {
		remoteCmd = fmt.Sprintf("cd %s && %s", shellescape.Quote(remoteCwd), claudeCmd)
	}

	if err := session.Start(remoteCmd); err != nil {
		return 0, fmt.Errorf("启动远程 Claude Code 失败: %w", err)
	}

	if err := session.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), nil
		}
		if err == io.EOF {
			return 0, nil
		}
		return 0, fmt.Errorf("SSH 会话异常结束: %w", err)
	}

	return 0, nil
}
