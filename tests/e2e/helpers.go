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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
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

// ─── MVS-06 / 到期容器自动停止 + 审计事件 ────────────────────────────

// ExpiryEventType 是 Phase 47 Plan 01 锁定的「过期触发主机停止」审计事件类型。
//
// 源码 internal/controlplane/scheduler/expiry.go::expireUser 中写入：
//
//	store.RecordEvent(ctx, repository.RecordEventParams{
//	    Type: "host.stop.expired",
//	    Metadata: map[string]any{"reason": "user expired", ...},
//	})
//
// 与 ROADMAP §Phase 47 §Details 1 与 47-CONTEXT.md §Area 1 草案的差异：
// 文档草案曾写「host.stopped」事件；以源码为准，本表锁定为 host.stop.expired。
// 任一漂移立即在 darwin 单测层失败，并需同步更新 PLAN/SUMMARY。
const ExpiryEventType = "host.stop.expired"

// UserExpiredEventType 是 ExpiryScanner 在标记 user.status='expired' 之后
// 写入的用户级别审计事件类型。用例可用作「scanner 已触发」的快速断言。
const UserExpiredEventType = "user.expired"

// expiryEventListResponse 对应 GET /v1/admin/events 的响应 body 顶层结构。
// 仅取 type 字段；其它字段（id / created_at / metadata）通过 RawMessage 透传。
type expiryEventListResponse struct {
	Events []struct {
		Type string `json:"type"`
	} `json:"events"`
}

// ParseEventTypes 抽取 admin events API 响应 body 中的事件 type 列表（保留顺序）。
//
// 行为：
//   - 输入为 GET /v1/admin/events 的 JSON body 切片。
//   - 解析失败 → 返回 nil + err。
//   - 缺 events 字段 / 空数组 → 返回 []string{} + nil。
//   - 解析成功 → 返回每行 type 的有序切片（不去重，保留 backend 排序）。
//
// 不依赖 metadata 内容；上层用例自行根据 type 子串匹配 + metadata 二次过滤。
func ParseEventTypes(body []byte) ([]string, error) {
	if len(body) == 0 {
		return []string{}, fmt.Errorf("parse event types: empty body")
	}
	var parsed expiryEventListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse event types: %w", err)
	}
	out := make([]string, 0, len(parsed.Events))
	for _, ev := range parsed.Events {
		out = append(out, ev.Type)
	}
	return out, nil
}

// ─── MVS-07 / 出口 IP 双绑互斥 ────────────────────────────────────────

// BindEgressIPResponse 是 POST /v1/admin/bindings 的解析后契约视图。
//
// ErrorMessage：当前源码 admin_bindings.go::Bind() 用 `{"error":"自由文本"}`
// 而非稳定 error code 枚举；本结构保留 raw message 子串，待 backend 引入
// 稳定 code（如 egress_ip_already_bound）后再扩展枚举字段（Phase 51 QUAL-04）。
type BindEgressIPResponse struct {
	Status       int
	ErrorMessage string
	RawBody      []byte
}

// bindErrorBody 对应 {"error":"..."}。
type bindErrorBody struct {
	Error string `json:"error"`
}

// ParseBindEgressIPResponse 把 admin bindings POST 的 status code + body 合成契约视图。
//
// 行为：
//   - body 非空且解析出 error 字段 → ErrorMessage 取该值。
//   - body 空 / 非 JSON / 缺 error 字段 → ErrorMessage="", err=nil（合法 2xx 路径）。
//   - 不消耗 body；RawBody 字段透传原 bytes 供上层 t.Logf。
func ParseBindEgressIPResponse(status int, body []byte) (BindEgressIPResponse, error) {
	out := BindEgressIPResponse{Status: status, RawBody: body}
	if len(body) == 0 {
		return out, nil
	}
	var parsed bindErrorBody
	if err := json.Unmarshal(body, &parsed); err == nil {
		out.ErrorMessage = parsed.Error
	}
	return out, nil
}

// EgressIPDoubleBindContract 锁定 MVS-07「期望」的拒绝行为。
//
// 当前源码 admin_bindings.go 在底层 BindEgressIPToHost 错误时返回 500，
// 因为 `host_egress_bindings` 仅 UNIQUE(host_id, egress_ip_id)，没有
// 「同一 egress_ip_id 只允许绑给一个 host」的硬约束。本契约定义「应有」
// 的行为；用例失败时把 backend 缺口落到 SUMMARY，建议 Phase 51 修源码。
var EgressIPDoubleBindContract = struct {
	WantStatus       int
	WantErrSubstring string
}{
	WantStatus:       409,
	WantErrSubstring: "already bound",
}

// ─── MVS-08 / host-agent 心跳与恢复 ───────────────────────────────────

// HostHealthStatus 是控制面 /healthz checks.agent 字段的枚举映射。
//
// 当前控制面没有 GET /v1/admin/hosts/{X}/health 端点（grep router.go 与
// admin_hosts.go），单宿主机 v1 部署用全局 /healthz 的 checks.agent 即可
// 表达 host-agent 进程级健康状态。多宿主机场景属未来 phase。
type HostHealthStatus int

const (
	HostHealthUnknown HostHealthStatus = iota
	HostHealthHealthy
	HostHealthUnhealthy
	HostHealthDegraded
)

// String 让 HostHealthStatus 在 t.Log / artifact dump 中可读。
func (s HostHealthStatus) String() string {
	switch s {
	case HostHealthHealthy:
		return "Healthy"
	case HostHealthUnhealthy:
		return "Unhealthy"
	case HostHealthDegraded:
		return "Degraded"
	default:
		return "Unknown"
	}
}

// controlPlaneHealthBody 对应 /healthz 响应顶层结构。
type controlPlaneHealthBody struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// ParseControlPlaneHealth 解析 GET /healthz 响应 body，返回 (overall, agent) 二元组。
//
// 映射（参见 internal/controlplane/http/router.go::/healthz handler）：
//
//	overall:
//	  "ok"       → HostHealthHealthy
//	  "warning"  → HostHealthUnhealthy（含 agent unreachable）
//	  "degraded" → HostHealthDegraded（含 database 故障）
//	  其它/缺失   → HostHealthUnknown
//
//	agent (取自 checks.agent 字段)：
//	  "ok"           → HostHealthHealthy
//	  "unreachable"  → HostHealthUnhealthy
//	  缺失（embedded 模式不暴露 agent 字段）→ HostHealthUnknown
//	  其它字面量      → HostHealthUnknown
//
// 解析失败 → 返回 (Unknown, Unknown, err)。
func ParseControlPlaneHealth(body []byte) (HostHealthStatus, HostHealthStatus, error) {
	if len(body) == 0 {
		return HostHealthUnknown, HostHealthUnknown, fmt.Errorf("parse health: empty body")
	}
	var parsed controlPlaneHealthBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return HostHealthUnknown, HostHealthUnknown, fmt.Errorf("parse health: %w", err)
	}
	overall := HostHealthUnknown
	switch parsed.Status {
	case "ok":
		overall = HostHealthHealthy
	case "warning":
		overall = HostHealthUnhealthy
	case "degraded":
		overall = HostHealthDegraded
	}
	agent := HostHealthUnknown
	if parsed.Checks != nil {
		switch parsed.Checks["agent"] {
		case "ok":
			agent = HostHealthHealthy
		case "unreachable":
			agent = HostHealthUnhealthy
		case "":
			agent = HostHealthUnknown
		}
	}
	return overall, agent, nil
}

// HostHealthRecoveryContract 锁定 MVS-08 时间窗。
//
// 任一阈值漂移 → 同步修 47-03-PLAN / 47-03-SUMMARY，避免静默放宽 SLA。
var HostHealthRecoveryContract = struct {
	UnhealthyWithin time.Duration
	HealthyWithin   time.Duration
}{
	UnhealthyWithin: 30 * time.Second,
	HealthyWithin:   60 * time.Second,
}

// ─── 既有锁定表（保留） ────────────────────────────────────────────────

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
