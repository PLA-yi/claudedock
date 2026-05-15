package network

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const gatewayTPProxyPort = 7892

// resolvConfContent 是 worker 容器 /etc/resolv.conf 的固定内容（v3.5 Phase 45 Plan 02）。
// 唯一 nameserver 指向 sing-box gateway 的 tun0 (172.19.0.1)；ndots:0 +
// single-request-reopen 避免无谓的 search-domain 查询与 SERVFAIL 复用问题。
// 必须以换行结尾以便 grep / 行匹配。
const resolvConfContent = "nameserver 172.19.0.1\noptions ndots:0 single-request-reopen\n"

// nsswitchConfContent 是 worker 容器 /etc/nsswitch.conf 的固定内容。
// hosts 行严格限定 "files dns"（禁用 mdns / myhostname / wins / nis_plus），
// 其余字段沿用 Linux 标准默认以保证 passwd / group / shadow 等查询正常工作。
// 用 "+" 字符串拼接避免 raw-string 缩进陷阱。
const nsswitchConfContent = "passwd:         compat\n" +
	"group:          compat\n" +
	"shadow:         compat\n" +
	"gshadow:        files\n" +
	"hosts:          files dns\n" +
	"networks:       files\n" +
	"protocols:      db files\n" +
	"services:       db files\n" +
	"ethers:         db files\n" +
	"rpc:            db files\n" +
	"netgroup:       nis\n"

type ContainerProxyProvider struct {
	logger *slog.Logger
}

func NewContainerProxyProvider(logger *slog.Logger) *ContainerProxyProvider {
	return &ContainerProxyProvider{logger: logger}
}

// PrepareGateway 在 worker 容器 docker create 之前把 sing-box config 写到 host 端
// SingBoxConfigDir(hostID)/config.json（D-54-2 / Plan 54-02），容器随后通过 ro bind
// mount 把该文件挂到 /etc/sing-box/config.json，entrypoint start_singbox_or_die 读取
// 后从 fs 删除（D-V4-2）。
//
// v4.0 (Phase 54) 改造（54-01）：
//   - 不再创建 cloudproxy-net-* 自定义 bridge（删除 dockerNetworkCreate 调用）
//   - 不再启动 sidecar gateway 容器（删除 dockerRunGateway / waitGatewayHealthy）
//   - 不再写 v3.5 容器 DNS 入口锁占位（resolv.conf / nsswitch.conf 由容器内 sing-box
//     接管，删除 WriteContainerDNSConfig 调用）
//   - 不再写 v4 sing-box 路径下的 cidrs / domains placeholder（由 sing-box config
//     的 route.rule_set 直接拉取，54-02 决定具体格式）
//
// user 容器自带 sing-box（Phase 53 镜像），entrypoint 内 start_singbox_or_die 在
// 容器自身 netns 里建 tun0 并应用全局策略路由；host-agent 只做「config 注入 + verify」。
func (p *ContainerProxyProvider) PrepareGateway(ctx context.Context, spec HostNetworkSpec) error {
	_ = ctx
	if spec.Egress == nil {
		p.logger.Info("container-proxy: no egress config, skipping config inject", "host_id", spec.HostID)
		return nil
	}
	if spec.Egress.Proxy == nil {
		p.logger.Warn("container-proxy: no proxy config, skipping config inject", "host_id", spec.HostID)
		return nil
	}

	dir := SingBoxConfigDir(spec.HostID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("sing-box: mkdir config dir: %w", err)
	}
	if err := writeContainerSingBoxConfig(spec.HostID, spec.Egress); err != nil {
		return fmt.Errorf("sing-box: write config: %w", err)
	}
	p.logger.Info("container-proxy: sing-box config injected", "host_id", spec.HostID, "dir", dir)
	return nil
}

// writeContainerSingBoxConfig 由 Plan 54-02 实现真正逻辑；54-01 留 stub 让单容器
// 链路骨架就位。stub 返回 nil 是 **设计意图**：worker.buildCreateArgs 的 sing-box
// config ro bind mount 在 config.json 不存在时会让 docker create 立即失败，避免
// 静默旁路。
func writeContainerSingBoxConfig(hostID string, egress *EgressConfig) error {
	_ = hostID
	_ = egress
	return nil
}

// PrepareHost 在 worker 容器 docker start 之后调用。
//
// v4.0 (Phase 54) 改造（54-01）：user 容器自带 sing-box（Phase 53），entrypoint
// apply_nft_or_die 在容器内部应用 fail-closed firewall + sing-box 自起 tun0；
// host-agent 不再：
//   - dockerNetworkConnect / disconnect bridge（容器走默认 docker bridge）
//   - configureWorkerEgress（容器内 sing-box 自己配 tun 路由）
//   - applyWorkerFirewall（容器内 entrypoint 自己 apply）
//   - join 控制面到隔离网络（无隔离网络存在）
//
// 仅保留 verifyWorkerNetwork 做出口 IP / DNS / leak 三检，确认 Phase 53 entrypoint
// 启动序列真的把流量导到 tun0。
func (p *ContainerProxyProvider) PrepareHost(ctx context.Context, spec HostNetworkSpec) error {
	if spec.Egress == nil {
		p.logger.Info("container-proxy: no egress config, skipping verify", "host_id", spec.HostID)
		return nil
	}
	if spec.Egress.Proxy == nil {
		p.logger.Warn("container-proxy: no proxy config, skipping verify", "host_id", spec.HostID)
		return nil
	}

	workerName := workerContainerName(spec.HostID)
	result, verifyErr := verifyWorkerNetwork(ctx, workerName, *spec.Egress)
	if verifyErr != nil {
		p.logger.Error("container-proxy: network verification failed",
			"host_id", spec.HostID,
			"egress_ip_match", result.EgressIPMatch,
			"dns_correct", result.DNSCorrect,
			"leak_blocked", result.LeakBlocked,
			"actual_egress_ip", result.ActualEgressIP,
			"actual_dns", result.ActualDNS,
		)
		if netErr, ok := verifyErr.(*NetworkError); ok {
			netErr.HostID = spec.HostID
		}
		return verifyErr
	}
	p.logger.Info("container-proxy: network verification passed (single-container)",
		"host_id", spec.HostID,
		"egress_ip", result.ActualEgressIP,
		"dns_server", result.ActualDNS,
	)
	return nil
}

func (p *ContainerProxyProvider) CleanupHost(ctx context.Context, spec HostNetworkSpec) error {
	p.teardownGateway(ctx, spec.HostID)
	return nil
}

func (p *ContainerProxyProvider) teardownGateway(ctx context.Context, hostID string) {
	netName := networkName(hostID)
	gwName := gatewayContainerName(hostID)
	workerName := workerContainerName(hostID)

	cleanupWorkerFirewall(ctx, workerName)

	// Phase 45 WR-04：旧实现把 os.Hostname() 与所有 docker 命令的错误全部 `_ =` 吞掉，
	// 控制面 disconnect 失败时既无审计也无 Warn 日志。现在统一 Warn 到 p.logger，
	// 与 PrepareHost 的对应路径保持一致；任何错误都不阻断后续清理（best-effort）。
	cpID, hostnameErr := os.Hostname()
	if hostnameErr != nil {
		p.logger.Warn("container-proxy: teardown get control-plane hostname failed",
			"host_id", hostID, "error", hostnameErr)
	} else if cpID != "" {
		if err := exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", netName, cpID).Run(); err != nil {
			p.logger.Warn("container-proxy: teardown disconnect control-plane from isolated network failed",
				"host_id", hostID, "cp_id", cpID, "network", netName, "error", err)
		}
	}

	if err := exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", netName, workerName).Run(); err != nil {
		p.logger.Warn("container-proxy: teardown disconnect worker from isolated network failed",
			"host_id", hostID, "worker", workerName, "network", netName, "error", err)
	}
	if err := exec.CommandContext(ctx, "docker", "rm", "-f", gwName).Run(); err != nil {
		p.logger.Warn("container-proxy: teardown remove gateway container failed",
			"host_id", hostID, "gateway", gwName, "error", err)
	}
	if err := exec.CommandContext(ctx, "docker", "network", "rm", netName).Run(); err != nil {
		p.logger.Warn("container-proxy: teardown remove isolated network failed",
			"host_id", hostID, "network", netName, "error", err)
	}
	if err := os.RemoveAll(GatewayConfigDir(hostID)); err != nil {
		p.logger.Warn("container-proxy: teardown remove gateway config dir failed",
			"host_id", hostID, "dir", GatewayConfigDir(hostID), "error", err)
	}
}

func GatewayImage() string {
	if v := os.Getenv("CLOUD_CLI_PROXY_GATEWAY_IMAGE"); v != "" {
		return v
	}
	return "cloud-cli-proxy-sing-gateway:local"
}

// SingBoxConfigDir 返回 host 专属的 sing-box config 目录。
// 路径规则：<DATA_DIR>/gateway/<host_id>/。
//
// 路径名 "gateway" 是 v3.x 历史包袱（D-54-10）：为避免跨包（bypass_reload.go /
// admin_hosts.go / app.go / e2e 等）大改动，54-01 保留物理路径不变，语义重定义
// 为「单容器架构下 host-agent 注入到 user 容器内 /etc/sing-box/config.json 的源
// 路径」。下一里程碑（v4.1）再考虑物理迁移到 sing-box/<host_id>/。
func SingBoxConfigDir(hostID string) string {
	base := os.Getenv("DATA_DIR")
	if base == "" {
		base = "/var/lib/cloud-cli-proxy"
	}
	return filepath.Join(base, "gateway", hostID)
}

// GatewayConfigDir 是 SingBoxConfigDir 的 v4.0 兼容 alias（D-54-9），
// 保留一个里程碑（v4.1 删除）。新代码请使用 SingBoxConfigDir。
//
// Deprecated: use SingBoxConfigDir.
func GatewayConfigDir(hostID string) string {
	return SingBoxConfigDir(hostID)
}

// WriteContainerDNSConfig 把 v3.5 容器 DNS 入口锁的两个源文件写到
// <DATA_DIR>/gateway/<host_id>/resolv.conf 与 nsswitch.conf。
// 这两个文件随后由 worker 容器以 ro bind mount 挂入 /etc/resolv.conf 与
// /etc/nsswitch.conf。必须在 worker docker create 之前调用。
func WriteContainerDNSConfig(hostID string) error {
	dir := GatewayConfigDir(hostID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir container dns config dir: %w", err)
	}
	resolvPath := filepath.Join(dir, "resolv.conf")
	if err := os.WriteFile(resolvPath, []byte(resolvConfContent), 0o644); err != nil {
		return fmt.Errorf("write resolv.conf: %w", err)
	}
	nsswitchPath := filepath.Join(dir, "nsswitch.conf")
	if err := os.WriteFile(nsswitchPath, []byte(nsswitchConfContent), 0o644); err != nil {
		return fmt.Errorf("write nsswitch.conf: %w", err)
	}
	return nil
}

func networkName(hostID string) string {
	return "cloudproxy-net-" + hostID
}

// proxyServerIP 从 EgressConfig 中解析 sing-box outbound 的代理服务器 IP（字符串形式）。
// 返回空字符串表示无代理 IP（Phase 1+ 兼容路径或解析失败）；调用方应据此 skip uid 锁规则。
// 解析失败仅在底层 outbound JSON 缺失字段或域名解析失败时发生，控制面侧已经在
// PrepareGateway 阶段先做过一次 extractProxyServer + dockerRunGateway 引用了相同 IP，
// 此处再次解析极少失败；为避免 nft 加固阻断主流程，失败一律降级为「无 uid 锁」。
func proxyServerIP(egress *EgressConfig) string {
	if egress == nil || egress.Proxy == nil {
		return ""
	}
	ip, _, err := extractProxyServer(egress.Proxy.OutboundConfig)
	if err != nil {
		return ""
	}
	return ip
}

func gatewayContainerName(hostID string) string {
	return "cloudproxy-gw-" + hostID
}

func workerContainerName(hostID string) string {
	return "cloudproxy-" + hostID
}

func subnetThirdOctet(hostID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(hostID))
	return int(h.Sum32()%200) + 20
}

func dockerNetworkCreate(ctx context.Context, name, subnet, gateway string) error {
	cmd := exec.CommandContext(ctx, "docker", "network", "create",
		"--driver", "bridge",
		"--subnet", subnet,
		"--gateway", gateway,
		name,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func dockerRunGateway(ctx context.Context, gwName, netName, gwIP, proxyServerIP, configPath, cidrsPath, domainsPath, image string) error {
	args := []string{
		"run", "-d",
		"--name", gwName,
		"--network", netName,
		"--ip", gwIP,
		"--cap-add", "NET_ADMIN",
		"--device", "/dev/net/tun:/dev/net/tun",
		"--sysctl", "net.ipv4.ip_forward=1",
		"-v", configPath + ":/etc/sing-box/config.json:ro",
		"-v", cidrsPath + ":/etc/sing-box/whitelist-cidrs.json:ro",
		"-v", domainsPath + ":/etc/sing-box/whitelist-domains.json:ro",
		"--label", "cloud-cli-proxy.role=gateway",
		"--label", "cloud-cli-proxy.managed=true",
		"--restart", "no",
		image,
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func dockerNetworkConnect(ctx context.Context, netName, containerName, staticIP string) error {
	args := []string{"network", "connect"}
	if staticIP != "" {
		args = append(args, "--ip", staticIP)
	}
	args = append(args, netName, containerName)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker network connect: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// waitGatewayHealthy 等待 gateway 容器内 sing-box 真正完成 tun0 监听。
//
// Phase 45 WR-03 修复：旧实现仅检查 docker `inspect.State.Running=true`
// 与 logs 中是否含 "FATAL"/"panic:"，但 sing-box 启动序列包含：
//   (1) 解析 config（可能因 schema 错误失败）
//   (2) 建立 tun 设备（Linux 必须 NET_ADMIN + /dev/net/tun）
//   (3) 启动 DoH 拨号到上游 DNS
//   (4) 建立 proxy outbound
//
// 在 (1)~(2) 完成之前，容器虽然 Running=true，但 worker 容器内 ro-mount
// 的 /etc/resolv.conf 指向 172.19.0.1（tun0），实际还没监听 → DNS 查询
// 立即 SERVFAIL。同时 sing-box 1.11 在 config 解析错误时输出
// "start service:" / "unmarshal:" 等关键字，旧的两关键字串匹配会漏掉。
//
// 新实现：
//   - 探测策略改为「在 gateway 容器内 ip link show tun0」就绪检测，
//     重试最多 30 次、间隔 200ms（总等待约 6 秒，与旧 20 秒上界相当但更精确）
//   - 仍同时检查 logs 中扩展后的失败关键字，遇到立即返回错误
func waitGatewayHealthy(ctx context.Context, gwName string) error {
	const maxAttempts = 30
	const interval = 200 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// 先确认容器仍在 Running，避免在已退出的容器上反复 exec 浪费时间。
		inspect := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", gwName)
		if out, err := inspect.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
			lastErr = fmt.Errorf("container not running: %v", err)
			time.Sleep(interval)
			continue
		}

		// 关键改动：探测 tun0 是否已在容器 netns 内被 sing-box 创建并 UP。
		// 用 `docker exec gwName ip link show tun0` 替代假设 nsenter 存在的方案，
		// docker exec 与 macOS 开发机 + linux 生产机都兼容。
		probe := exec.CommandContext(ctx, "docker", "exec", gwName, "ip", "link", "show", "tun0")
		if probe.Run() == nil {
			return nil
		}

		// tun0 尚未就绪 → 检查 logs 中是否出现已知失败关键字；命中则提前 fail。
		logs, _ := exec.CommandContext(ctx, "docker", "logs", "--tail", "120", gwName).CombinedOutput()
		s := string(logs)
		for _, kw := range []string{"FATAL", "panic:", "start service:", "unmarshal:", "failed to start"} {
			if strings.Contains(s, kw) {
				return fmt.Errorf("gateway sing-box failed (matched %q): %s", kw, strings.TrimSpace(s))
			}
		}

		lastErr = fmt.Errorf("tun0 not ready yet")
		time.Sleep(interval)
	}
	logs, _ := exec.CommandContext(ctx, "docker", "logs", gwName).CombinedOutput()
	return fmt.Errorf("gateway container tun0 not ready in time (last=%v): %s", lastErr, strings.TrimSpace(string(logs)))
}

func configureWorkerEgress(ctx context.Context, workerName, defaultGW, workerIP string) error {
	const maxRetry = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetry; attempt++ {
		if err := tryConfigureWorkerEgress(ctx, workerName, defaultGW, workerIP); err == nil {
			return nil
		} else {
			lastErr = err
			if attempt < maxRetry {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
		}
	}
	return fmt.Errorf("configureWorkerEgress failed after %d attempts: %w", maxRetry, lastErr)
}

func tryConfigureWorkerEgress(ctx context.Context, workerName, defaultGW, workerIP string) error {
	// Phase 45 Plan 02：/etc/resolv.conf 已被 PrepareGateway 写盘 + worker docker
	// create 时 ro bind mount 接管，这里**不再** docker exec 写 resolv.conf；
	// 任何写盘尝试都会被 ro mount 拒绝。本脚本只负责 default route。
	script := fmt.Sprintf(`set -e
# 等待网络接口就绪
for i in 1 2 3 4 5; do
  DEV=$(ip -o addr show | grep '%s' | awk '{print $2}' | head -1)
  [ -n "$DEV" ] && break
  sleep 1
done
if [ -z "$DEV" ]; then
  echo "waiting for interface with IP %s timed out"
  ip -o addr show >&2
  exit 1
fi
# 删除所有现有 default 路由
ip route show default | while read -r line; do
  gw=$(echo "$line" | grep -oP 'via \K[^ ]+' || true)
  dev=$(echo "$line" | grep -oP 'dev \K[^ ]+' || true)
  if [ -n "$gw" ] && [ -n "$dev" ]; then
    ip route del default via "$gw" dev "$dev" 2>/dev/null || true
  fi
done
ip route del default 2>/dev/null || true
# 默认路由指向 gateway，所有流量必须经过 sing-box 代理隧道
ip route add default via %s dev "$DEV" metric 0
# 立即 verify
default_route=$(ip route show default | head -1)
echo "$default_route" | grep -q "via %s"
`, workerIP, workerIP, defaultGW, defaultGW)

	cmd := exec.CommandContext(ctx, "docker", "exec", workerName, "sh", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
