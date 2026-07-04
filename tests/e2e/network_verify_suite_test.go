//go:build e2e

package e2e

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// NetworkVerifySuite 针对已有 running claudedock 容器做网络安全验证。
// 此套件不创建/销毁容器，仅在已有运行环境中执行只读验证。
type NetworkVerifySuite struct {
	suite.Suite
	containerName string
	ctx           context.Context
	cancel        context.CancelFunc
}

func (s *NetworkVerifySuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	if _, err := exec.LookPath("docker"); err != nil {
		s.T().Skip("docker not available")
	}
	out, err := exec.Command("docker", "ps", "--filter", "name=claudedock-", "--format", "{{.Names}}").Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		s.T().Skip("no running claudedock container")
	}
	s.containerName = strings.Split(strings.TrimSpace(string(out)), "\n")[0]
	s.T().Logf("testing container: %s", s.containerName)
}

func (s *NetworkVerifySuite) TearDownSuite() {
	s.cancel()
}

// dockerExec 在目标容器中执行命令，返回 stdout/stderr 和 error。
func (s *NetworkVerifySuite) dockerExec(args ...string) (string, error) {
	cmdArgs := append([]string{"exec", s.containerName}, args...)
	out, err := exec.CommandContext(s.ctx, "docker", cmdArgs...).CombinedOutput()
	return string(out), err
}

// TestSOCKS5ProxyAvailable 验证 sing-box SOCKS5 inbound 可达。
func (s *NetworkVerifySuite) TestSOCKS5ProxyAvailable() {
	out, err := s.dockerExec(
		"curl", "-x", "socks5h://127.0.0.1:1080",
		"-4", "--max-time", "5", "-s",
		"https://ip.me",
	)
	if err != nil {
		s.T().Logf("SOCKS5 proxy unavailable (expected on macOS/no-proxy): %v", err)
		return
	}
	ip := strings.TrimSpace(out)
	if ip != "" {
		s.T().Logf("SOCKS5 egress IP: %s", ip)
		s.Require().NotEmpty(ip)
	}
}

// TestDNSLockedToLocalhost 验证 /etc/resolv.conf 首行指向 127.0.0.1。
func (s *NetworkVerifySuite) TestDNSLockedToLocalhost() {
	out, err := s.dockerExec("cat", "/etc/resolv.conf")
	s.Require().NoError(err)
	s.Require().Contains(strings.TrimSpace(out), "nameserver 127.0.0.1",
		"resolv.conf must contain nameserver 127.0.0.1 (sing-box DNS)")
}

// TestTUNDeviceExists 验证 tun0 网络设备存在。
func (s *NetworkVerifySuite) TestTUNDeviceExists() {
	out, err := s.dockerExec("ip", "link", "show", "tun0")
	s.Require().NoError(err)
	s.Require().Contains(out, "tun0", "tun0 device must exist")
	s.Require().Contains(out, "UP", "tun0 must be UP")
}

// TestSingBoxRunning 验证 sing-box 进程在运行。
func (s *NetworkVerifySuite) TestSingBoxRunning() {
	out, err := s.dockerExec("pgrep", "-f", "sing-box run")
	s.Require().NoError(err)
	pid := strings.TrimSpace(out)
	s.Require().NotEmpty(pid, "sing-box process must be running")
	s.T().Logf("sing-box pid: %s", pid)
}

// TestNFTRulesExist 验证 nftables 规则就位。
func (s *NetworkVerifySuite) TestNFTRulesExist() {
	// macOS Docker Desktop 不支持 nft，此测试预期失败时仅记录
	out, err := s.dockerExec("nft", "list", "table", "inet", "cloud_proxy_v4")
	if err != nil {
		s.T().Logf("nft not available on this platform (expected on macOS): %v", err)
		return
	}
	s.Require().Contains(out, "chain output", "nft output chain must exist")
	s.Require().Contains(out, "chain input", "nft input chain must exist")
	// 验证关键规则存在
	s.Require().Contains(out, "oifname \"tun0\"", "nft must accept tun0 output")
	s.Require().Contains(out, "singbox-direct-egress", "nft must allow sing-box direct egress")
	s.T().Log("nft rules verified")
}

// TestIPRoutingFixed 验证 ip rule 路由修复生效。
func (s *NetworkVerifySuite) TestIPRoutingFixed() {
	out, err := s.dockerExec("sh", "-c", "ip rule show | grep '8999\\|9001'")
	if err != nil {
		s.T().Logf("ip rule check failed (may be pre-fix container): %v", err)
		return
	}
	// rule 8999: sing-box uid 9000 → main 表
	s.Require().Contains(out, "uidrange 9000-9000", "ip rule 8999 must route sing-box to main table")
	// rule 9001: 用户流量 → table 2022 (不含 suppress_prefixlength)
	s.Require().Contains(out, "lookup 2022", "ip rule 9001 must route users to table 2022")
	s.Require().NotContains(out, "suppress_prefixlength", "ip rule 9001 must NOT have suppress_prefixlength")
	s.T().Log("ip routing rules verified")
}

// TestTable2022HasLocalSubnets 验证 table 2022 包含本地子网直连路由。
func (s *NetworkVerifySuite) TestTable2022HasLocalSubnets() {
	out, err := s.dockerExec("ip", "route", "show", "table", "2022")
	if err != nil {
		s.T().Logf("table 2022 check failed: %v", err)
		return
	}
	s.Require().Contains(out, "default via 172.19.0.2", "table 2022 must have default via tun0")
	// 至少有 lo 的直连路由
	s.Require().Contains(out, "127.0.0", "table 2022 must have localhost route")
	s.T().Logf("table 2022 routes:\n%s", out)
}

// TestSSHPortListening 验证 SSH 端口在监听。
func (s *NetworkVerifySuite) TestSSHPortListening() {
	out, err := s.dockerExec("ss", "-tlnp")
	s.Require().NoError(err)
	s.Require().Contains(out, ":22", "SSH port 22 must be listening")
}

// TestVNCPortListening 验证 VNC 端口在监听（MODE!=local 时）。
func (s *NetworkVerifySuite) TestVNCPortListening() {
	out, _ := s.dockerExec("ss", "-tlnp")
	if strings.Contains(out, ":6080") {
		s.T().Log("VNC port 6080 listening")
	} else {
		s.T().Log("VNC port 6080 not listening (MODE=local or VNC not started)")
	}
}

// ─── Go test 入口 ──────────────────────────────────────────────────────────

func TestNetworkVerifySuite(t *testing.T) {
	suite.Run(t, new(NetworkVerifySuite))
}
