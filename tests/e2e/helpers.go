// Package e2e 中的 helpers.go 提供 Phase 46 MVS 的纯函数 helpers。
//
// 设计原则：
//   - 本文件不带 build tag，darwin 上的 `go build ./tests/e2e/...` /
//     `go test ./tests/e2e/ -run Helpers` 必须能直接编译并跑通对应单测。
//   - 任何依赖 docker / linux netns / testcontainers 的接口都放在
//     helpers_linux.go（带 `//go:build e2e && linux`）。
//   - 三个 MVS 核心纯函数（Vote / ClassifyDNSResult / SummarizeDenyResults）
//     与一份 CLI 错误码契约表全部锁在此处，供 helpers_test.go 与 CI runner
//     共同消费。
//
// 引入历史：Phase 46 Plan 01（v3.6 milestone）。
package e2e

import (
	"fmt"
	"strconv"
	"strings"
)

// ─── MVS-02 / 多数派裁决 ───────────────────────────────────────────────

// VoteResult 是 Vote 函数的返回值，便于 t.Logf 输出 + artifact dump。
type VoteResult struct {
	Winner  string
	OK      bool
	Dissent []string
}

// Vote 把多源回显结果合成一个多数派裁决（MVS-02）。
//
// 语义：
//   - results 中空字符串视为「弃权」（该源 timeout / dns fail / 非 200）。
//   - winner 为出现次数最多的非空字符串；ok=true 当且仅当 winner 计数 >= 2
//     （即「多数派达成」，CONTEXT §Area 2 决策）。
//   - dissent 为与 winner 不一致的非空回显（用于失败时 artifact dump）。
//   - 全部弃权 → winner="", ok=false, dissent=nil。
//
// 输入为 nil / 空切片 → 视为全部弃权。
func Vote(results []string) VoteResult {
	counts := map[string]int{}
	for _, r := range results {
		if r == "" {
			continue
		}
		counts[r]++
	}

	var winner string
	maxCount := 0
	for ip, c := range counts {
		if c > maxCount || (c == maxCount && ip < winner) {
			maxCount = c
			winner = ip
		}
	}

	ok := maxCount >= 2
	var dissent []string
	if ok {
		for _, r := range results {
			if r != "" && r != winner {
				dissent = append(dissent, r)
			}
		}
	} else {
		winner = ""
		for _, r := range results {
			if r != "" {
				dissent = append(dissent, r)
			}
		}
	}
	return VoteResult{Winner: winner, OK: ok, Dissent: dissent}
}

// egressIPSources 是 MVS-02 锁定的 3 个公网回显源。
// CONTEXT §Area 2 决策：固定 3 源避免每个用例自由发挥。
var egressIPSources = []string{
	"https://ip.me",
	"https://ifconfig.io",
	"https://ipinfo.io/ip",
}

// EgressIPSources 返回 MVS-02 锁定的 3 源副本（防止用例侧 mutate 影响其它 plan）。
func EgressIPSources() []string {
	cp := make([]string, len(egressIPSources))
	copy(cp, egressIPSources)
	return cp
}

// ─── MVS-03 / DNS 分类 ─────────────────────────────────────────────────

// DNSProbeResult 是 ClassifyDNSResult 的枚举返回值（MVS-03）。
//
// Tunneled：exit 0，认为 tun 接管并正常返回 A 记录或 HTTPS 握手成功。
// Denied：被防火墙拒绝（refused / timeout / unreachable / not permitted）。
// Leaked：明确证据走宿主机直连绕过 tun（暂留扩展点；本 phase 仅 Tunneled / Denied）。
// Unknown：分类不明，用例应 fail。
type DNSProbeResult int

const (
	DNSResultUnknown DNSProbeResult = iota
	DNSResultTunneled
	DNSResultDenied
	DNSResultLeaked
)

// String 让 DNSProbeResult 在 t.Log 输出可读。
func (r DNSProbeResult) String() string {
	switch r {
	case DNSResultTunneled:
		return "Tunneled"
	case DNSResultDenied:
		return "Denied"
	case DNSResultLeaked:
		return "Leaked"
	default:
		return "Unknown"
	}
}

// dnsDenyKeywords 是 stderr 出现即视为 Denied 的关键字集合（小写匹配）。
var dnsDenyKeywords = []string{
	"connection refused",
	"timed out",
	"timeout",
	"network is unreachable",
	"operation not permitted",
	"permanent failure in name resolution",
	"name or service not known",
	"no route to host",
}

// ClassifyDNSResult 把容器内 DNS probe（getent / dig / nslookup）的 exit code +
// stderr 文本映射到 DNSProbeResult 枚举（MVS-03）。
//
// 语义：
//   - exit 0 → Tunneled。
//   - exit != 0 且 stderr 含 dnsDenyKeywords 任一 → Denied。
//   - 其它 → Unknown，用例应 fail（避免悄悄假阳）。
//
// Leaked 不由本函数赋值；由调用方拿到「目标域名解析结果落到宿主机直连 IP」的
// 硬证据时另行 override（Phase 49 防泄漏对抗时会接管此分支）。
func ClassifyDNSResult(exitCode int, stderr string) DNSProbeResult {
	if exitCode == 0 {
		return DNSResultTunneled
	}
	lower := strings.ToLower(stderr)
	for _, kw := range dnsDenyKeywords {
		if strings.Contains(lower, kw) {
			return DNSResultDenied
		}
	}
	return DNSResultUnknown
}

// ─── MVS-04 / 默认拒绝矩阵 ─────────────────────────────────────────────

// DenyTarget 是默认拒绝矩阵中的 host + port 组合（MVS-04）。
type DenyTarget struct {
	Host string
	Port int
}

// String 让 DenyTarget 在 t.Log / artifact dump 中可读。
func (t DenyTarget) String() string { return fmt.Sprintf("%s:%d", t.Host, t.Port) }

// DefaultDenyMatrix 是 CONTEXT §Area 2 锁定的 4 个默认拒绝 target。
// 修改本切片需同步更新 PLAN / VERIFICATION，并通知 Phase 48 / 49 复用方。
var DefaultDenyMatrix = []DenyTarget{
	{Host: "1.1.1.1", Port: 80},
	{Host: "8.8.8.8", Port: 443},
	{Host: "9.9.9.9", Port: 443},
	{Host: "169.254.169.254", Port: 80},
}

// BuildDenyProbeCmd 返回容器内执行的「直连探测」shell 命令。
//
// 命令形如：timeout <N> bash -c 'echo >/dev/tcp/HOST/PORT'
// 用 timeout(1) 把 bash /dev/tcp 探测裹一层硬超时，避免 nft drop 触发的
// TCP 长尾 retransmit 拖垮整个用例。
//
// timeoutSec <= 0 时回退到默认 3s。
func BuildDenyProbeCmd(t DenyTarget, timeoutSec int) []string {
	if timeoutSec <= 0 {
		timeoutSec = 3
	}
	return []string{
		"timeout", strconv.Itoa(timeoutSec),
		"bash", "-c",
		fmt.Sprintf("echo >/dev/tcp/%s/%d", t.Host, t.Port),
	}
}

// SummarizeDenyResults 把矩阵每个 target 的 exit code 合成裁决（MVS-04）。
//
//   - exit 0 → 连通 → leak（违反"默认拒绝"约束）。
//   - exit != 0 → Denied（refused / unreachable / timeout 都计入）。
//   - allDenied=true 当且仅当所有 target 都被拒绝。
//   - leaks 列表按 DefaultDenyMatrix 顺序遍历输入 map（保证稳定输出）。
func SummarizeDenyResults(results map[DenyTarget]int) (allDenied bool, leaks []DenyTarget) {
	allDenied = true
	for _, t := range DefaultDenyMatrix {
		code, ok := results[t]
		if !ok {
			continue
		}
		if code == 0 {
			allDenied = false
			leaks = append(leaks, t)
		}
	}
	return allDenied, leaks
}

// ─── MVS-05 / CLI 错误码契约 ──────────────────────────────────────────

// CLIErrorCase 是 bootstrap 脚本错误码场景的 table-driven 测试条目。
type CLIErrorCase struct {
	Name               string
	Username           string
	Password           string
	WantExitCode       int
	WantStderrContains string
}

// BootstrapExitCodeContract 是 MVS-05 锁定的 4 个错误码契约表。
//
// 数值与 internal/controlplane/http/bootstrap_errors.go BootstrapErrorEntries
// 中的 ExitCode 字段一致；helpers_test.go 中通过 import 该包做交叉断言，
// 任一漂移立即编译期 / 测试期失败。
//
// 与 ROADMAP §Phase 46 §Details 5 描述的差异：ROADMAP 写「真实 cloud-claude
// binary」，但 grep `cmd/cloud-claude/main.go` 实际定义的常量是
// exitAuthFailed=1 / exitNetworkError=2 等，不含 10-13；错误码 10-13 由
// `deploy/bootstrap/cloud-bootstrap.sh` 在 `case "$error_code"` 分支映射，
// 走的是 curl bootstrap 入口而非 cloud-claude binary。本表以源码为准。
var BootstrapExitCodeContract = map[string]int{
	"auth_invalid":     10,
	"account_disabled": 11,
	"account_expired":  12,
	"host_not_found":   13,
}

// CLIErrorCases 是 MVS-05 的 4 个 table-driven 用例预设（用例代码可直接 range）。
// stderr 关键字与 bootstrap_errors.go 中 Message 字段保持一致的子串。
var CLIErrorCases = []CLIErrorCase{
	{
		Name:               "auth_invalid",
		Username:           "alice",
		Password:           "wrong-password",
		WantExitCode:       BootstrapExitCodeContract["auth_invalid"],
		WantStderrContains: "用户名或密码错误",
	},
	{
		Name:               "account_disabled",
		Username:           "disabled-user",
		Password:           "secret-placeholder-pw",
		WantExitCode:       BootstrapExitCodeContract["account_disabled"],
		WantStderrContains: "账号已被停用",
	},
	{
		Name:               "account_expired",
		Username:           "expired-user",
		Password:           "secret-placeholder-pw",
		WantExitCode:       BootstrapExitCodeContract["account_expired"],
		WantStderrContains: "账号已过期",
	},
	{
		Name:               "host_not_found",
		Username:           "user-no-host",
		Password:           "secret-placeholder-pw",
		WantExitCode:       BootstrapExitCodeContract["host_not_found"],
		WantStderrContains: "未找到可用主机",
	},
}
