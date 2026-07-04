package doctor

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// RemoteRunner 抽象远端命令执行，Plan 02 单测注入 fakeRunner（CONTEXT D-20 / RESEARCH §9 第 1 条）。
// name 仅用于 --verbose 日志标签；script 是完整 shell 片段（executor 必须保证 shellescape 已处理）。
type RemoteRunner interface {
	RunScript(name, script string) (stdout, stderr string, err error)
}

// sshRemoteRunner 是生产实现，基于 golang.org/x/crypto/ssh。
// 与 ssh_doctor.go:runSSHSession 的 stdout/stderr 收集模式逐字符对齐。
type sshRemoteRunner struct {
	conn *ssh.Client
}

// NewSSHRemoteRunner 构造生产 runner。conn 由 RunDoctor 在第一次需要远端的 check 前 lazy 建立。
func NewSSHRemoteRunner(conn *ssh.Client) RemoteRunner {
	return &sshRemoteRunner{conn: conn}
}

func (r *sshRemoteRunner) RunScript(name, script string) (string, string, error) {
	if r.conn == nil {
		return "", "", fmt.Errorf("remote_runner: conn is nil (name=%s)", name)
	}
	sess, err := r.conn.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("创建 SSH 会话失败 (%s): %w", name, err)
	}
	defer sess.Close()
	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr
	if err := sess.Run(script); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), stderr.String(), fmt.Errorf("%w (%s)", err, msg)
	}
	return stdout.String(), stderr.String(), nil
}
