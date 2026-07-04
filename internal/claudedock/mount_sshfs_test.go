package claudedock

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// countingFileReader 实现 sftp.FileReader（FileGet 接口），统计每个文件路径
// 被 Fileread 调用的次数。通过 atomic.Int64 + sync.Mutex 保证并发安全
// （FUSE 可能并发发起多个 read request）。
type countingFileReader struct {
	root  string
	mu    sync.Mutex
	reads map[string]*atomic.Int64
}

func newCountingFileReader(root string) *countingFileReader {
	return &countingFileReader{
		root:  root,
		reads: make(map[string]*atomic.Int64),
	}
}

func (c *countingFileReader) counter(path string) *atomic.Int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.reads[path]; !ok {
		c.reads[path] = &atomic.Int64{}
	}
	return c.reads[path]
}

// ReadCount 返回 path 已被 Fileread 调用的次数（线程安全）。
func (c *countingFileReader) ReadCount(path string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctr, ok := c.reads[path]; ok {
		return ctr.Load()
	}
	return 0
}

// Fileread 实现 sftp.FileReader 接口（FileGet），计数并返回真实文件 ReaderAt。
// 注意：sftp.Request.Filepath 总是以 "/" 开头（remote 视角的绝对路径），
// 与本地 root 拼接时使用 filepath.Join 会自动 normalize。
func (c *countingFileReader) Fileread(req *sftp.Request) (io.ReaderAt, error) {
	c.counter(req.Filepath).Add(1)
	return os.Open(filepath.Join(c.root, req.Filepath))
}

// noopFileWriter 实现 sftp.FileWriter（FilePut 接口），返回 OpUnsupported。
// 用于占位 sftp.Handlers.FilePut（不能为 nil）。
type noopFileWriter struct{}

func (n *noopFileWriter) Filewrite(req *sftp.Request) (io.WriterAt, error) {
	return nil, sftp.ErrSSHFxOpUnsupported
}

// mustSignerFromKey 生成临时 SSH host key（测试专用，2048-bit RSA）。
func mustSignerFromKey(t *testing.T) ssh.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("生成 RSA key 失败: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("生成 SSH signer 失败: %v", err)
	}
	return signer
}

// TestSSHFSCacheHitsKernelPageCache 验证：同会话同文件 ReadFile 2 次 → SFTP
// server-side Fileread 仅调用 1 次（FUSE page cache 命中第二次不回源）。
//
// 测试拓扑：
//   client(os.ReadFile) → mountpoint → sshfs（本机进程）→ 真实 ssh → fixture SSH server
//   → sftp.NewRequestServer(handlers={FileGet: counting,...}) → counter.Add(1)
//
// sshfs / fusermount(3) 任一未安装时自动 Skip（D-11 / D-20 / D-22）。
func TestSSHFSCacheHitsKernelPageCache(t *testing.T) {
	if _, err := exec.LookPath("sshfs"); err != nil {
		t.Skip("sshfs 未安装，跳过 cache counting 测试（D-11）")
	}
	if _, err := exec.LookPath("fusermount"); err != nil {
		if _, err2 := exec.LookPath("fusermount3"); err2 != nil {
			t.Skip("fusermount/fusermount3 未安装，跳过 FUSE 测试")
		}
	}

	fixtureDir := t.TempDir()
	fixturePath := filepath.Join(fixtureDir, "fixture.bin")
	const fixtureSz = int64(512 * 1024) // 512KB 足够触发真实 read（>1 sftp packet）
	fixFile, err := os.Create(fixturePath)
	if err != nil {
		t.Fatalf("创建 fixture 文件失败: %v", err)
	}
	if err := fixFile.Truncate(fixtureSz); err != nil {
		_ = fixFile.Close()
		t.Fatalf("Truncate 失败: %v", err)
	}
	_ = fixFile.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen 失败: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	port := listener.Addr().(*net.TCPAddr).Port
	counting := newCountingFileReader(fixtureDir)

	sshConfig := &ssh.ServerConfig{NoClientAuth: true}
	sshConfig.AddHostKey(mustSignerFromKey(t))

	// inMem 提供 FileCmd / FileList 的 no-op 底座（L5 规避：四字段不能为 nil）。
	inMem := sftp.InMemHandler()
	handlers := sftp.Handlers{
		FileGet:  counting,
		FilePut:  &noopFileWriter{},
		FileCmd:  inMem.FileCmd,
		FileList: inMem.FileList,
	}

	go func() {
		for {
			netConn, err := listener.Accept()
			if err != nil {
				return // listener closed
			}
			go func(c net.Conn) {
				srvConn, chans, reqs, err := ssh.NewServerConn(c, sshConfig)
				if err != nil {
					return
				}
				_ = srvConn
				go ssh.DiscardRequests(reqs)
				for newChan := range chans {
					if newChan.ChannelType() != "session" {
						_ = newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
						continue
					}
					ch, chReqs, err := newChan.Accept()
					if err != nil {
						return
					}
					go func(channel ssh.Channel, reqs <-chan *ssh.Request) {
						for req := range reqs {
							if req.Type == "subsystem" && len(req.Payload) > 4 {
								name := string(req.Payload[4:])
								if name == "sftp" {
									_ = req.Reply(true, nil)
									srv := sftp.NewRequestServer(channel, handlers)
									_ = srv.Serve()
									return
								}
							}
							if req.WantReply {
								_ = req.Reply(false, nil)
							}
						}
					}(ch, chReqs)
				}
			}(netConn)
		}
	}()

	mountDir := t.TempDir()
	// sshfs 完整参数含 cache 选项（与 mount_sshfs.go::mountSSHFS 字面量同步，D-10）。
	// StrictHostKeyChecking=no + UserKnownHostsFile=/dev/null：测试环境跳过 known_hosts。
	// password_stdin + 空密码：sshfs 不交互；fixture server NoClientAuth=true 实际放行。
	sshfsCmd := exec.Command("sshfs",
		fmt.Sprintf("testuser@127.0.0.1:%s", fixtureDir), mountDir,
		"-p", fmt.Sprintf("%d", port),
		"-o", "StrictHostKeyChecking=no,UserKnownHostsFile=/dev/null,password_stdin",
		"-o", "passive,reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10,cache=yes,kernel_cache,auto_cache,cache_timeout=300",
		"-f",
	)
	sshfsCmd.Stdin = nil // 空 stdin：password_stdin 读到 EOF 后用空密码尝试，配合 NoClientAuth 通过
	if err := sshfsCmd.Start(); err != nil {
		t.Fatalf("sshfs Start 失败: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("fusermount", "-u", mountDir).Run()
		_ = exec.Command("fusermount3", "-u", mountDir).Run()
		if sshfsCmd.Process != nil {
			_ = sshfsCmd.Process.Kill()
		}
	})

	mountedFile := filepath.Join(mountDir, "fixture.bin")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(mountedFile); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(mountedFile); err != nil {
		t.Fatalf("sshfs 挂载超时（5s），fixture.bin 不可见: %v", err)
	}

	data1, err := os.ReadFile(mountedFile)
	if err != nil {
		t.Fatalf("第一次 ReadFile 失败: %v", err)
	}
	if int64(len(data1)) != fixtureSz {
		t.Fatalf("第一次读取大小 %d != %d", len(data1), fixtureSz)
	}

	data2, err := os.ReadFile(mountedFile)
	if err != nil {
		t.Fatalf("第二次 ReadFile 失败: %v", err)
	}
	if int64(len(data2)) != fixtureSz {
		t.Fatalf("第二次读取大小 %d != %d", len(data2), fixtureSz)
	}

	readCount := counting.ReadCount("/fixture.bin")
	if readCount != 1 {
		t.Errorf("期望 SFTP server-side Fileread = 1（第二次由 FUSE page cache 接管），实际 = %d", readCount)
	}
}
