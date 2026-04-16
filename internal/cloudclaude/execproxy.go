package cloudclaude

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type execRequest struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args"`
	Cwd  string   `json:"cwd"`
}

type execResponse struct {
	ExitCode int `json:"exit_code"`
}

// ExecProxy 监视本地 .cloud-claude/exec/ 目录中的命令请求，
// 在本地执行后将结果写回，供容器内 wrapper 脚本读取。
type ExecProxy struct {
	localDir string
	execDir  string
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewExecProxy(localDir string) *ExecProxy {
	return &ExecProxy{
		localDir: localDir,
		execDir:  filepath.Join(localDir, ".cloud-claude", "exec"),
		stopCh:   make(chan struct{}),
	}
}

func (p *ExecProxy) Start() error {
	if err := os.MkdirAll(p.execDir, 0755); err != nil {
		return fmt.Errorf("创建 exec 目录失败: %w", err)
	}

	p.wg.Add(1)
	go p.pollLoop()
	return nil
}

func (p *ExecProxy) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	_ = os.RemoveAll(filepath.Join(p.localDir, ".cloud-claude"))
}

func (p *ExecProxy) pollLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.scanRequests()
		}
	}
}

func (p *ExecProxy) scanRequests() {
	entries, err := os.ReadDir(p.execDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "req-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		uuid := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "req-"), ".json")
		p.handleRequest(uuid)
	}
}

func (p *ExecProxy) handleRequest(uuid string) {
	reqPath := filepath.Join(p.execDir, "req-"+uuid+".json")
	data, err := os.ReadFile(reqPath)
	if err != nil {
		return
	}

	var req execRequest
	if err := json.Unmarshal(data, &req); err != nil {
		p.writeResponse(uuid, nil, nil, 1)
		_ = os.Remove(reqPath)
		return
	}

	cwd := req.Cwd
	if cwd == "" {
		cwd = p.localDir
	}

	cmd := exec.Command(req.Cmd, req.Args...)
	cmd.Dir = cwd
	cmd.Env = os.Environ()

	stdout, err := cmd.Output()
	var stderr []byte
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			stderr = exitErr.Stderr
		} else {
			exitCode = 1
			stderr = []byte(err.Error())
		}
	}

	p.writeResponse(uuid, stdout, stderr, exitCode)
	_ = os.Remove(reqPath)
}

func (p *ExecProxy) writeResponse(uuid string, stdout, stderr []byte, exitCode int) {
	base := filepath.Join(p.execDir, "res-"+uuid)

	if len(stdout) > 0 {
		_ = os.WriteFile(base+".stdout", stdout, 0644)
	}
	if len(stderr) > 0 {
		_ = os.WriteFile(base+".stderr", stderr, 0644)
	}

	resp := execResponse{ExitCode: exitCode}
	data, _ := json.Marshal(resp)
	_ = os.WriteFile(base+".json", data, 0644)
}
