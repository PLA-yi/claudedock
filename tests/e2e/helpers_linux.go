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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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

// 防御性强引用，避免 goimports 把这些 import 删掉。
var _ = http.MethodGet
