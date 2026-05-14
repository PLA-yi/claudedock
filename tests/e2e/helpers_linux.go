//go:build e2e && linux

// helpers_linux.go 收纳 Phase 46 中依赖 docker / linux netns / testcontainers
// 的 e2e 入口与容器侧 helper。darwin 上不参与编译（保护本地 `go build ./...`
// 与 `go test ./tests/e2e/ -run Helpers` 的清洁度）。
//
// 关键约定：
//   - 任一前置缺失（无 docker / Scenario.Start 仍是 Step 2..7 sentinel）→ t.Skip。
//   - 这里只放「需要 GoldenPath 句柄 / Container Exec / 控制面 admin API」的
//     函数；其它纯函数（Vote / Classify / Summarize）放 helpers.go 共享。

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"

	"github.com/zanel1u/cloud-cli-proxy/tests/e2e/harness"
)

// GoldenPath 封装 Phase 46 MVS 所需的完整 e2e 拓扑：
// 控制面 + host-agent + Postgres + sing-box gateway + 1 user + 1 host。
//
// 字段在 StartGoldenPath 成功返回后才填充；用例代码不应自行构造 GoldenPath。
type GoldenPath struct {
	Scenario        *harness.Scenario
	Gateway         *harness.GatewayHandle
	Host            *harness.HostHandle
	User            *harness.UserHandle
	ControlPlaneURL string

	// BootstrapScript 指向 deploy/bootstrap/cloud-bootstrap.sh 的项目相对路径，
	// e2e 用例通过 exec.CommandContext("bash", g.BootstrapScript) 起子进程。
	BootstrapScript string
}

// StartGoldenPath 启动并返回 GoldenPath 句柄。
//
// 行为约定：
//   - 任一前置缺失（无 docker daemon / Scenario.Start 命中 Phase 45 Plan 02
//     Step 2..7 sentinel 错误）→ t.Skip 并返回 nil。
//   - 用例代码必须先判 `if g == nil { return }` 再访问 GoldenPath 字段，
//     避免对 nil 解引用。
//   - Cleanup 通过 t.Cleanup(func(){ scenario.Stop }) 注册，调用者无需手动 Stop。
//
// 失败 fast path：除了 Skip 之外的硬错（控制面真的启动不起来、PrepareGateway
// 真的报错）通过 t.Fatalf 上抛，让 CI 上的失败立刻冒泡。
func StartGoldenPath(t *testing.T) *GoldenPath {
	t.Helper()

	if _, err := testcontainers.NewDockerProvider(); err != nil {
		t.Skipf("docker provider unavailable, skipping golden path: %v", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)

	outbound := json.RawMessage(`{"type":"direct","tag":"proxy-out"}`)
	sc := harness.New(t).
		WithControlPlane().
		WithSingBoxGateway("primary", outbound).
		WithHost("alpha").
		WithUser("alice")

	if err := sc.Start(ctx); err != nil {
		// Phase 45 Plan 02 当前 Step 2..7 仍是 sentinel。
		// 把它转 Skip，让 Phase 46 用例骨架先合入；真实拓扑由 CI runner 在
		// Scenario.Start Step 2..7 实现完成后自然解锁。
		if errors.Is(err, harness.ErrScenarioStepNotImplemented) {
			t.Skipf("scenario step 2..7 not yet implemented (Phase 45 follow-up); deferred to Linux CI: %v", err)
			return nil
		}
		t.Fatalf("StartGoldenPath: scenario start: %v", err)
		return nil
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer stopCancel()
		_ = sc.Stop(stopCtx)
	})

	cp := sc.ControlPlane()
	gw := sc.SingBoxGateway("primary")
	host := sc.Host("alpha")
	user := sc.User("alice")

	return &GoldenPath{
		Scenario:        sc,
		Gateway:         gw,
		Host:            host,
		User:            user,
		ControlPlaneURL: cp.Addr,
		BootstrapScript: projectRelativePath("deploy/bootstrap/cloud-bootstrap.sh"),
	}
}

// projectRelativePath 返回项目根 + 相对路径；禁绝对路径硬编码。
func projectRelativePath(rel string) string {
	_, file, _, _ := runtime.Caller(0) // tests/e2e/helpers_linux.go
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, rel)
}

// ipv4Re 抽出回显文本中的第一个 IPv4 字面量；空字符串表示未抓到。
var ipv4Re = regexp.MustCompile(`\b(\d{1,3}\.){3}\d{1,3}\b`)

// FetchEgressIPInContainer 并行调容器内的 curl 拉 EgressIPSources() 的 3 源，
// 返回结果切片（顺序对齐 EgressIPSources()）。某源 timeout / 非 200 → 对应
// 位置空字符串。
//
// 单源 5s 超时；总 ctx 由调用方决定（推荐 15s）。
func FetchEgressIPInContainer(ctx context.Context, c harness.ContainerHandle) []string {
	sources := EgressIPSources()
	results := make([]string, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			cmd := []string{"curl", "-fsS", "--max-time", "5", url}
			code, reader, err := c.Exec(ctx, cmd)
			if err != nil || code != 0 || reader == nil {
				return
			}
			body, err := io.ReadAll(io.LimitReader(reader, 1024))
			if err != nil || len(body) == 0 {
				return
			}
			results[idx] = ipv4Re.FindString(string(body))
		}(i, src)
	}
	wg.Wait()
	return results
}

// RunBootstrapScript 起子进程跑 deploy/bootstrap/cloud-bootstrap.sh，喂 stdin，
// 返回 exitCode + stdout + stderr。MVS-05 用例与 MVS-01 用例共用。
//
// 行为约定：
//   - 通过 *exec.ExitError 解包 exit code；进程正常退出 → exit 0。
//   - exec.CommandContext 启动失败（如脚本不存在）→ exitCode=-1, err 非 nil。
//   - 调用方应通过 context 控制总超时；本函数自身不设硬超时。
func RunBootstrapScript(
	ctx context.Context,
	scriptPath string,
	env []string,
	stdin string,
) (exitCode int, stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = strings.NewReader(stdin)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if runErr == nil {
		return 0, stdout, stderr, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode(), stdout, stderr, nil
	}
	return -1, stdout, stderr, fmt.Errorf("run bootstrap script: %w", runErr)
}

// ControlPlaneHealthURL 拼接 GoldenPath.ControlPlaneURL 的 /healthz 入口。
// 用例可直接拿来喂 harness.WaitForHTTP。
func (g *GoldenPath) ControlPlaneHealthURL() string {
	if g == nil {
		return ""
	}
	base := strings.TrimRight(g.ControlPlaneURL, "/")
	return base + "/healthz"
}

// SeedBootstrapErrorFixtures 把 tests/e2e/fixtures/error-codes.sql 灌进控制面
// Postgres，让 MVS-05 用例的 disabled / expired / no-host 用户预先存在。
//
// 实现策略：通过控制面 admin API 创建 user（避免直接连 Postgres，保持 e2e 走
// 真实生产路径）。如果 admin API 不支持 disabled/expired 状态字段，则 fallback
// 到直接 SQL 注入；fallback 实现由 Phase 46 Plan 05 落地时补全。
//
// 当前阶段：返回 nil 占位（实际灌种放在 cli_error_codes_test.go 内联，配合
// admin API 真实路径）。
func SeedBootstrapErrorFixtures(_ context.Context, _ *GoldenPath) error {
	// TODO(46-05): 通过 admin API + 直接 SQL 双路径灌种 disabled/expired/no-host 用户。
	// 当前阶段不阻塞 build，CI runner 接通 Step 2..7 后再补全。
	return nil
}

// ─── Phase 47 Plan 01 / MVS-06：到期治理 ────────────────────────────

// SimulateExpiry 把 user 的 expires_at 拉到过去 1 秒，等价于该 user 立刻到期。
//
// 行为：
//   - 直接连 Scenario.ControlPlane().DBURL，UPDATE users.expires_at = NOW() - 1s。
//   - 不调 ExpiryScanner.Scan()；通过生产路径上的 EXPIRY_SCAN_INTERVAL=1s 让真实
//     scanner 在下一 tick 触发，避免绕过 scheduler 包裹层。
//   - waitForTick=true：调 harness.WaitFor 30s 内轮询 events 表中是否出现
//     type='user.expired' AND user_id=$1，等到出现为止。
//   - waitForTick=false：UPDATE 返回即返回，由调用方自己等。
//
// 注意：本函数依赖 DBURL 字段；GoldenPath / scenario.startPostgres 必须先完成
// Step 1。Step 2..7 仍 sentinel 的当下，本函数仅供 Linux runner 在 Step 完整
// 落地后使用；darwin 上整个测试经 t.Skip 跳过。
func (p *GoldenPath) SimulateExpiry(ctx context.Context, userID string, waitForTick bool) error {
	if p == nil || p.Scenario == nil {
		return errors.New("simulate expiry: golden path not initialized")
	}
	cp := p.Scenario.ControlPlane()
	if cp == nil || cp.DBURL == "" {
		return errors.New("simulate expiry: control plane DBURL empty")
	}

	conn, err := pgx.Connect(ctx, cp.DBURL)
	if err != nil {
		return fmt.Errorf("simulate expiry: connect db: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	tag, err := conn.Exec(ctx,
		`UPDATE users SET expires_at = NOW() - INTERVAL '1 second' WHERE id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("simulate expiry: update users: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("simulate expiry: user %q not found", userID)
	}

	if !waitForTick {
		return nil
	}

	return harness.WaitFor(ctx, fmt.Sprintf("user.expired:%s", userID),
		func(ctx context.Context) error {
			var hits int
			row := conn.QueryRow(ctx,
				`SELECT COUNT(*) FROM events WHERE type = $1 AND user_id = $2`,
				UserExpiredEventType, userID,
			)
			if err := row.Scan(&hits); err != nil {
				return fmt.Errorf("scan events count: %w", err)
			}
			if hits == 0 {
				return fmt.Errorf("user.expired event not yet recorded for %s", userID)
			}
			return nil
		},
		harness.WithTimeout(30*time.Second),
		harness.WithPollInterval(500*time.Millisecond),
	)
}

// ─── Phase 47 Plan 02 / MVS-07：出口 IP 双绑互斥 ─────────────────────

// adminLoginRequest / adminLoginResponse 对应 POST /v1/auth/login 当前 schema。
type adminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type adminLoginResponse struct {
	Token string `json:"token"`
}

// AdminLogin 通过 POST /v1/auth/login 拿一个 admin JWT。
//
// 复用 Phase 46 admin fixture 入路；用户名 / 密码当前从环境变量
// E2E_ADMIN_USERNAME / E2E_ADMIN_PASSWORD 取（默认 admin / admin-pw，与
// Phase 46 Plan 01 §Step 2 scenario.go TODO 注释中描述的一致）。
//
// 缺 token 字段 / 非 200 → 返回错误。
func (p *GoldenPath) AdminLogin(ctx context.Context) (string, error) {
	if p == nil || p.ControlPlaneURL == "" {
		return "", errors.New("admin login: control plane URL empty")
	}
	username := strings.TrimSpace(getEnvOrDefault("E2E_ADMIN_USERNAME", "admin"))
	password := getEnvOrDefault("E2E_ADMIN_PASSWORD", "admin-pw")

	payload, _ := json.Marshal(adminLoginRequest{Username: username, Password: password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(p.ControlPlaneURL, "/")+"/v1/auth/login",
		bytes.NewReader(payload),
	)
	if err != nil {
		return "", fmt.Errorf("admin login: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := disableKeepAliveClient(5 * time.Second).Do(req)
	if err != nil {
		return "", fmt.Errorf("admin login: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("admin login: status %d body=%s", resp.StatusCode, string(body))
	}
	var parsed adminLoginResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("admin login: decode body: %w", err)
	}
	if parsed.Token == "" {
		return "", errors.New("admin login: empty token in response")
	}
	return parsed.Token, nil
}

// bindEgressIPRequest 对应 POST /v1/admin/bindings 的请求 schema（admin_bindings.go::Bind）。
type bindEgressIPRequest struct {
	HostID     string `json:"host_id"`
	EgressIPID string `json:"egress_ip_id"`
}

// PostBindEgressIP 调 POST /v1/admin/bindings 绑一个 egress IP 到一个 host。
//
// 返回 BindEgressIPResponse，包含 status code、error message（若有）、raw body。
// 401 / 403 等鉴权错由调用方自行判断；本函数不区分。
func (p *GoldenPath) PostBindEgressIP(ctx context.Context, hostID, egressIPID string) (BindEgressIPResponse, error) {
	if p == nil || p.ControlPlaneURL == "" {
		return BindEgressIPResponse{}, errors.New("bind egress: control plane URL empty")
	}
	token, err := p.AdminLogin(ctx)
	if err != nil {
		return BindEgressIPResponse{}, fmt.Errorf("bind egress: admin login: %w", err)
	}
	payload, _ := json.Marshal(bindEgressIPRequest{HostID: hostID, EgressIPID: egressIPID})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(p.ControlPlaneURL, "/")+"/v1/admin/bindings",
		bytes.NewReader(payload),
	)
	if err != nil {
		return BindEgressIPResponse{}, fmt.Errorf("bind egress: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := disableKeepAliveClient(10 * time.Second).Do(req)
	if err != nil {
		return BindEgressIPResponse{}, fmt.Errorf("bind egress: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16384))
	out, err := ParseBindEgressIPResponse(resp.StatusCode, body)
	if err != nil {
		return BindEgressIPResponse{}, fmt.Errorf("bind egress: parse: %w", err)
	}
	return out, nil
}

// QueryBindingExists 直接连 DB 查 (host_id, egress_ip_id) 绑定行是否存在。
//
// 用例在「断言 A 原绑定不被破坏」时使用。通过 admin API GET /v1/admin/hosts/{X}
// 也能查，但 schema 经多次演进；直查 host_egress_bindings 表更稳。
func (p *GoldenPath) QueryBindingExists(ctx context.Context, hostID, egressIPID string) (bool, error) {
	if p == nil || p.Scenario == nil {
		return false, errors.New("query binding: golden path not initialized")
	}
	cp := p.Scenario.ControlPlane()
	if cp == nil || cp.DBURL == "" {
		return false, errors.New("query binding: control plane DBURL empty")
	}
	conn, err := pgx.Connect(ctx, cp.DBURL)
	if err != nil {
		return false, fmt.Errorf("query binding: connect db: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var hits int
	row := conn.QueryRow(ctx,
		`SELECT COUNT(*) FROM host_egress_bindings WHERE host_id = $1 AND egress_ip_id = $2`,
		hostID, egressIPID,
	)
	if err := row.Scan(&hits); err != nil {
		return false, fmt.Errorf("query binding: scan: %w", err)
	}
	return hits > 0, nil
}

// ─── Phase 47 Plan 03 / MVS-08：host-agent 心跳与恢复 ────────────────

// KillHostAgent 在 host-agent 所在容器内执行 `pkill -9 -f host-agent`，
// 杀进程但不杀容器，让容器内 supervisor（dumb-init/systemd/supervisord）拉起。
//
// 行为约定：
//   - GoldenPath 当前没有 host-agent 容器句柄字段；本函数通过 host-agent 容器名
//     约定（沿用 v1 单宿主机 deploy 风格的 `host-agent` 容器）调 docker exec。
//   - 容器名通过 E2E_HOST_AGENT_CONTAINER 环境变量覆盖；默认 `host-agent`。
//   - embedded 模式下没有独立 host-agent 容器；调用方应先用
//     IsEmbeddedHostAgent() 判断，embedded 则 t.Skip 本用例。
//
// 不用 docker kill 整容器：CONTEXT §Area 3 决策——契约是「进程级恢复」，杀容器
// 会绕过被测路径。
func (p *GoldenPath) KillHostAgent(ctx context.Context) error {
	if p == nil {
		return errors.New("kill host-agent: golden path not initialized")
	}
	containerName := getEnvOrDefault("E2E_HOST_AGENT_CONTAINER", "host-agent")
	cmd := exec.CommandContext(ctx, "docker", "exec", containerName,
		"sh", "-c", "pkill -9 -f host-agent")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kill host-agent: docker exec %s: %w (stderr=%s)",
			containerName, err, stderr.String())
	}
	return nil
}

// IsEmbeddedHostAgent 返回当前控制面是否以 embedded 模式运行 host-agent。
//
// 通过环境变量 HOST_AGENT_MODE 推断（与 cmd/control-plane/main.go ENV 流相同）。
// embedded 模式下杀 host-agent = 杀控制面，MVS-08 用例应 t.Skip。
func IsEmbeddedHostAgent() bool {
	return getEnvOrDefault("HOST_AGENT_MODE", "") == "embedded"
}

// WaitHostHealthStatus 反复轮询 /healthz，直到 agent 字段等于期望状态或 timeout。
//
// expected 通常是 HostHealthHealthy / HostHealthUnhealthy。
// 单次请求 2s 超时，DisableKeepAlives=true（避免连接复用造成的假阳）。
func (p *GoldenPath) WaitHostHealthStatus(ctx context.Context, expected HostHealthStatus, timeout time.Duration) error {
	if p == nil || p.ControlPlaneURL == "" {
		return errors.New("wait health: control plane URL empty")
	}
	healthURL := strings.TrimRight(p.ControlPlaneURL, "/") + "/healthz"
	client := disableKeepAliveClient(2 * time.Second)

	name := fmt.Sprintf("agent_status=%s", expected)
	return harness.WaitFor(ctx, name, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return fmt.Errorf("build req: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("do: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		_, agent, perr := ParseControlPlaneHealth(body)
		if perr != nil {
			return fmt.Errorf("parse: %w (body=%s)", perr, string(body))
		}
		if agent != expected {
			return fmt.Errorf("agent=%s want=%s (body=%s)", agent, expected, string(body))
		}
		return nil
	},
		harness.WithTimeout(timeout),
		harness.WithPollInterval(500*time.Millisecond),
	)
}

// ─── 内部 helpers ──────────────────────────────────────────────────────

// disableKeepAliveClient 返回一个禁 keep-alive 的 http.Client，避免长连接造成
// /healthz 等高频轮询时的连接复用假阳。
func disableKeepAliveClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
}

// getEnvOrDefault 与 cmd/control-plane/main.go::envOrDefault 同语义；
// 本包内独立实现，避免反向 import cmd 包。
func getEnvOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// 防御性强引用，避免 goimports 把这些 import 删掉。
var _ = http.MethodGet
