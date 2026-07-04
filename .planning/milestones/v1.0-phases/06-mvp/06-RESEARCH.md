# Phase 6: 加固与 MVP 就绪 - Research

**Researched:** 2026-03-27
**Domain:** 端到端测试、部署文档、体验打磨与上线前加固
**Confidence:** HIGH

## Summary

Phase 6 是纯加固阶段，不新增功能，所有产品特性已在 Phase 1–5 完成。研究重点在三个维度：(1) 用 Go 标准测试设施 + build tag 分层的冒烟测试策略；(2) 面向有运维经验技术人员的可执行部署手册；(3) 前后端体验打磨和安全/稳定性上线检查清单。

代码基线已具备 9 个测试文件、完整的 stub/mock 依赖注入模式、systemd 服务单元、host-preflight 脚本和 bootstrap 脚本。Phase 6 在这些基础上扩展覆盖范围，不需要引入新框架或重大架构变更。

**Primary recommendation:** 沿用现有 Go `testing` + `httptest` + stub 模式扩展 API 测试；集成测试层使用 `testcontainers-go` 提供真实 PostgreSQL；shell 脚本测试使用 BATS；部署文档与自动化脚本并行输出。

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** 测试分两层：需要真实 Docker daemon 的集成测试层（Go test + build tag `integration`），以及无需 Docker 的快速单元/API 层（Go httptest + mock）。
- **D-02:** 集成测试优先覆盖核心关键路径：bootstrap 认证 → 任务创建与状态流转 → 网络校验（出口 IP / DNS / 泄漏阻断）→ SSH 就绪门槛 → 后台管理 API CRUD。
- **D-03:** bootstrap 脚本层面使用 shell 测试脚本验证端到端流程（curl → 认证 → 轮询 → SSH handoff），确保脚本与 API 的错误码契约不漂移。
- **D-04:** 后台管理 API 测试使用 Go httptest 覆盖认证、用户 CRUD、出口 IP CRUD、绑定管理和主机生命周期操作的正常路径与异常路径。
- **D-05:** 到期与对账定时器的测试通过注入可控时间源和 mock 依赖进行验证，不依赖真实等待。
- **D-06:** 目标读者为有 Linux 运维经验的技术人员，文档不需要从零解释 Docker 或 PostgreSQL 安装。
- **D-07:** 文档形式为 Markdown 文档与自动化部署脚本并行：手册提供理解与排障参考，脚本提供可执行的部署路径。
- **D-08:** 覆盖场景包括首次部署（环境准备 → 构建 → 配置 → 启动 → 验证）、日常运维（用户管理 → 主机运维 → 证书/密钥轮换 → 备份恢复）和常见故障排查。灾难恢复作为附录简要说明。
- **D-09:** 部署手册应覆盖：宿主机依赖检查（沿用 host-preflight.sh）、WireGuard 配置、PostgreSQL 初始化、控制面与 host-agent 的 systemd 部署、受管镜像构建、防火墙规则和 bootstrap 入口配置。
- **D-10:** 终端侧：审查 bootstrap 脚本所有失败路径，确保每个错误码都有清晰的中文提示和下一步建议（重试命令或联系管理员）；补齐网络超时、handoff 失败等边缘场景的提示。
- **D-11:** 后台侧：补齐表单校验反馈（必填字段、格式校验）、操作中 loading 状态指示器、列表空状态展示，确保管理员操作流程顺畅无死角。
- **D-12:** 运维侧：统一控制面和 host-agent 的结构化日志格式（slog JSON），完善 `/healthz` 端点使其包含数据库连接和 agent 可达性检查，为生产部署提供基础监控入口。
- **D-13:** 安全：审查所有 API 端点的权限边界（admin API 需 JWT、bootstrap API 需认证、公开端点仅限 healthz 和 script）；确认密码、JWT secret、WireGuard 私钥等敏感字段不在 API 响应或日志中泄露。
- **D-14:** 稳定性：确认控制面和 host-agent 支持优雅关闭（SIGTERM → 等待在途任务 → 关闭连接池）；确认 PostgreSQL 连接池有合理配置（max connections、idle timeout）；确认容器清理不遗留 orphan 资源。
- **D-15:** 可运维性：确认日志级别可通过环境变量调整；健康检查端点覆盖关键依赖；备份策略（PostgreSQL dump 周期）在文档中明确说明。

### Claude's Discretion
- 具体测试框架内的 test fixture 组织与 helper 函数设计。
- 文档的具体章节编排和格式细节。
- 日志字段的具体命名约定与采样策略。
- 健康检查端点的具体超时参数和降级行为。
- 前端体验打磨的具体交互动效与组件细节。

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ACCS-01 | 用户可以执行一条简短的 `curl` 启动命令，并在终端中看到用户名密码输入提示。 | 冒烟测试验证 bootstrap 脚本端到端流程；体验打磨确保错误路径有清晰提示。 |
| ACCS-03 | 当云主机已就绪且用户有权限时，系统可以直接把用户接入一个可用的 SSH 会话。 | 集成测试覆盖 bootstrap→任务→SSH handoff 完整链路；shell 测试验证脚本与 API 的错误码契约。 |
| NET-05 | 在主机被标记为可接入前，系统会验证出口 IP 和 DNS 路径都符合预期。 | 集成测试覆盖三重校验流程；冒烟测试确保网络校验失败时产生正确事件。 |
| ADMN-03 | 管理员可以在管理系统中查看用户、容器、出口 IP 绑定、生命周期和到期状态。 | Admin API 测试覆盖所有 CRUD 端点正常/异常路径；前端体验打磨覆盖空状态、loading、表单校验。 |
| ADMN-04 | 管理员操作和启动结果会被记录为运维事件，便于排障和支持。 | 事件 API 测试验证事件记录和查询；部署文档中说明日志和事件查看方式。 |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go `testing` + `httptest` | Go 1.25.7 标准库 | 单元/API 层测试 | 项目现有 9 个测试文件均使用此方案，无需引入外部测试框架 |
| `testcontainers-go` | v0.37.x | 集成测试提供真实 PostgreSQL | Go 生态最成熟的测试容器方案，直接复用 pgx 驱动 |
| `testcontainers-go/modules/postgres` | 与核心版本匹配 | PostgreSQL 容器初始化和迁移 | 提供 `WithInitScripts` 等便捷配置 |
| BATS (bats-core) | 1.x | Bootstrap 脚本端到端测试 | Bash 测试的事实标准，TAP 兼容输出 |
| Go `log/slog` JSONHandler | Go 1.25.7 标准库 | 生产结构化日志 | 已使用 TextHandler，切换 JSONHandler 无需额外依赖 |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `bats-assert` | 2.x | BATS 断言辅助库 | 编写脚本测试断言时使用 |
| `bats-support` | 0.3.x | BATS 测试基础辅助 | 与 bats-assert 配合使用 |
| react-hook-form + zod | 已在 package.json | 前端表单校验 | 扩展现有表单的校验规则和错误提示 |
| sonner | ^2.0.0 已在 package.json | Toast 通知 | 操作反馈和 loading 状态提示 |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| testcontainers-go | dockertest | testcontainers 社区更活跃，PostgreSQL module 更完善 |
| BATS | ShellSpec / shunit2 | BATS 是最广泛使用的 bash 测试框架，社区资源最多 |
| 纯 stub 测试 | 完全集成测试 | 两层分离兼顾速度和真实性 |

**Installation:**
```bash
# Go 集成测试依赖
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres

# BATS 安装（宿主机开发环境）
npm install --save-dev bats bats-assert bats-support
# 或通过包管理器: brew install bats-core
```

## Architecture Patterns

### 推荐测试目录结构
```
tests/
├── integration/           # 需要 Docker 的集成测试
│   ├── bootstrap_test.go  # bootstrap 全链路 (build tag: integration)
│   ├── admin_api_test.go  # Admin API CRUD (build tag: integration)
│   └── helpers_test.go    # 共享 TestMain、容器管理
├── smoke/                 # Shell 脚本测试
│   ├── bootstrap.bats     # bootstrap.sh 端到端验证
│   └── test_helper/       # BATS helper 加载
│       └── common.bash
internal/
├── controlplane/http/
│   ├── *_test.go          # 现有 API 层测试（无需 Docker）
│   └── admin_*_test.go    # 新增 Admin API 测试
├── controlplane/scheduler/
│   ├── expiry_test.go     # 到期扫描器测试（mock 时间源）
│   └── reconciler_test.go # 对账器测试（mock inspector）
```

### Pattern 1: Build Tag 分层测试
**What:** 使用 `//go:build integration` 将需要真实 Docker/PostgreSQL 的测试与快速单元测试分离。
**When to use:** 凡是需要启动外部服务（PostgreSQL、Docker daemon）的测试都打上 `integration` tag。
**Example:**
```go
//go:build integration

package integration

import (
    "context"
    "testing"

    "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestBootstrapFullFlow(t *testing.T) {
    ctx := context.Background()
    pgContainer, err := postgres.Run(ctx, "postgres:18.3",
        postgres.WithDatabase("claudedock_test"),
        postgres.WithUsername("test"),
        postgres.WithPassword("test"),
        postgres.WithInitScripts("../../internal/store/migrations/0001_initial.sql",
            "../../internal/store/migrations/0002_egress_tunnel.sql",
            "../../internal/store/migrations/0003_expiry_audit.sql"),
        postgres.BasicWaitStrategies(),
    )
    if err != nil {
        t.Fatal(err)
    }
    defer pgContainer.Terminate(ctx)

    connStr, _ := pgContainer.ConnectionString(ctx, "sslmode=disable")
    // 使用 connStr 初始化 app.New 或直接连接 repository
}
```

**Run commands:**
```bash
# 快速测试（无需 Docker，CI 常规执行）
go test ./...

# 集成测试（需要 Docker daemon）
go test -tags integration ./tests/integration/

# 全量
go test -tags integration ./...
```

### Pattern 2: Admin API 测试 — Stub + httptest
**What:** 沿用现有 bootstrap 测试的 stub 模式为每个 Admin handler 编写测试。
**When to use:** 测试 Admin API 的请求解析、权限检查、业务逻辑和响应格式。
**Example:**
```go
package http

func TestAdminUsersHandler_Create(t *testing.T) {
    tests := []struct {
        name       string
        body       map[string]string
        wantStatus int
    }{
        {
            name:       "valid create returns 201",
            body:       map[string]string{"username": "newuser", "password": "securepass123"},
            wantStatus: nethttp.StatusCreated,
        },
        {
            name:       "missing username returns 400",
            body:       map[string]string{"password": "securepass123"},
            wantStatus: nethttp.StatusBadRequest,
        },
    }
    // ... 套用现有 bootstrap_auth_test.go 的模式
}
```

### Pattern 3: 定时器测试 — 注入可控时间源
**What:** ExpiryScanner 和 Reconciler 的测试通过 mock store 接口验证逻辑，不依赖真实计时。
**When to use:** 测试 scheduler 包中的 expiry 和 reconcile 逻辑。
**Example:**
```go
package scheduler

type mockExpiryStore struct {
    expiredUsers []repository.User
    updatedIDs   []string
    stoppedHosts []string
}

func (m *mockExpiryStore) ListExpiredActiveUsers(_ context.Context) ([]repository.User, error) {
    return m.expiredUsers, nil
}

func TestExpiryScanner_Scan(t *testing.T) {
    store := &mockExpiryStore{
        expiredUsers: []repository.User{{ID: "u1", Username: "expired-user"}},
    }
    scanner := NewExpiryScanner(slog.Default(), store, &mockQueuer{})
    err := scanner.Scan(context.Background())
    if err != nil {
        t.Fatal(err)
    }
    if len(store.updatedIDs) != 1 || store.updatedIDs[0] != "u1" {
        t.Errorf("expected user u1 to be marked expired")
    }
}
```

### Pattern 4: slog JSON 日志切换
**What:** 根据环境变量在 TextHandler（开发）和 JSONHandler（生产）之间切换。
**When to use:** 控制面和 host-agent 的日志初始化。
**Example:**
```go
func newLogger() *slog.Logger {
    level := slog.LevelInfo
    if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
        var l slog.Level
        if err := l.UnmarshalText([]byte(lvl)); err == nil {
            level = l
        }
    }
    opts := &slog.HandlerOptions{Level: level}

    if os.Getenv("LOG_FORMAT") == "json" {
        return slog.New(slog.NewJSONHandler(os.Stdout, opts))
    }
    return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
```

### Pattern 5: 增强型健康检查端点
**What:** `/healthz` 扩展为包含 DB 连通性和 agent 可达性检查，返回分组状态。
**When to use:** 生产部署中作为 systemd、负载均衡或监控系统的探针目标。
**Example:**
```go
mux.HandleFunc("GET /healthz", func(w nethttp.ResponseWriter, r *nethttp.Request) {
    checks := map[string]string{}
    status := nethttp.StatusOK

    if err := deps.Health.Health(r.Context()); err != nil {
        checks["database"] = err.Error()
        status = nethttp.StatusServiceUnavailable
    } else {
        checks["database"] = "ok"
    }

    if deps.AgentHealth != nil {
        if err := deps.AgentHealth.Ping(r.Context()); err != nil {
            checks["agent"] = err.Error()
            status = nethttp.StatusServiceUnavailable
        } else {
            checks["agent"] = "ok"
        }
    }

    writeJSON(w, status, map[string]any{
        "status": map[bool]string{true: "ok", false: "degraded"}[status == nethttp.StatusOK],
        "checks": checks,
    })
})
```

### Anti-Patterns to Avoid
- **在 integration 测试中 mock 数据库**：集成测试的意义就是跑真实 DB，用 mock 等于退回到单元层。
- **在快速测试中连真实 DB**：会拖慢 `go test ./...` 并要求所有开发者运行 Docker，用 build tag 隔离。
- **把 healthz 用于功能探测**：healthz 只检查基础设施可达性，不应包含业务逻辑校验。
- **在 bootstrap 脚本里做复杂错误处理**：脚本只负责展示服务端返回的消息，错误分类和消息生成在服务端完成。

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| 测试用 PostgreSQL 容器管理 | 自己写 `docker run` + 端口等待逻辑 | `testcontainers-go/modules/postgres` | 自动端口映射、健康等待、清理 |
| Bash 脚本测试框架 | 自写 test runner | BATS (bats-core) | TAP 兼容、有成熟断言库、CI 友好 |
| 结构化日志 | 自定义 JSON 序列化 | `slog.NewJSONHandler` | 标准库已内置，线程安全 |
| 表单校验反馈 | 自写校验逻辑 | react-hook-form + zod（已安装） | 声明式 schema、自动错误绑定 |
| Toast 通知 | 自建通知系统 | sonner（已安装） | 已在项目中使用，API 简洁 |

**Key insight:** Phase 6 的核心原则是"最大限度复用已有设施"，不引入新框架或重建已有能力。

## Common Pitfalls

### Pitfall 1: 集成测试与单元测试混在一起导致 CI 变慢
**What goes wrong:** 没有 build tag 分层，每次 `go test ./...` 都尝试启动 Docker 容器，开发者本地和 CI 变慢 10-30 秒。
**Why it happens:** 忘记在集成测试文件顶部加 `//go:build integration`。
**How to avoid:** 集成测试统一放在 `tests/integration/` 目录，所有文件都带 `//go:build integration` tag。CI 分两个 step：快速测试和集成测试。
**Warning signs:** `go test ./...` 输出中出现 Docker 相关日志。

### Pitfall 2: testcontainers 端口冲突
**What goes wrong:** 多个测试同时启动 PostgreSQL 容器，端口冲突或资源耗尽。
**Why it happens:** 每个 TestXxx 函数独立启动容器。
**How to avoid:** 使用 `TestMain` 统一启动一个 PostgreSQL 容器，通过 `CREATE DATABASE` 为每个测试创建独立数据库，或使用事务回滚隔离。
**Warning signs:** 测试间歇性失败、端口被占用错误。

### Pitfall 3: BATS 测试中的 mock 服务器管理
**What goes wrong:** BATS 测试结束时 mock HTTP 服务没有清理，端口被占用。
**Why it happens:** 没有在 `teardown` 中杀掉后台进程。
**How to avoid:** 在 `setup()` 中启动 mock 服务并记录 PID，在 `teardown()` 中 `kill $PID`。
**Warning signs:** 第二次运行测试时端口冲突。

### Pitfall 4: 健康检查超时导致 systemd 重启循环
**What goes wrong:** `/healthz` 端点查询数据库超时，systemd 认为服务不健康反复重启。
**Why it happens:** 健康检查查询没有设置独立的超时，而 DB 连接池耗尽或慢查询时阻塞。
**How to avoid:** 健康检查使用 `context.WithTimeout` 设置独立的短超时（如 3 秒），DB 查询使用 `db.Ping()` 而非复杂查询。
**Warning signs:** systemd journal 中频繁出现 start/stop 循环。

### Pitfall 5: 日志级别切换需要重启服务
**What goes wrong:** 排障时需要 DEBUG 日志但不想重启生产服务。
**Why it happens:** 日志级别在启动时固定。
**How to avoid:** 使用 `slog.LevelVar` 替代固定级别，可以运行时动态调整（通过 HTTP 端点或信号）。不过对 v1 来说启动时环境变量配置已经足够，运行时动态调整是 v2 增强。
**Warning signs:** 排障时需要反复重启服务。

### Pitfall 6: 部署文档与实际脚本不同步
**What goes wrong:** 文档描述的步骤与实际脚本行为不一致，运维人员跟着文档操作失败。
**Why it happens:** 文档和脚本由不同任务维护，修改脚本后忘记更新文档。
**How to avoid:** 部署文档直接引用脚本路径和命令，不重复描述脚本内部逻辑；冒烟测试覆盖部署脚本的关键步骤。
**Warning signs:** 文档中的命令与脚本实际内容不一致。

### Pitfall 7: 前端空状态和 loading 体验不一致
**What goes wrong:** 有些页面有 loading skeleton，有些直接空白；有些空状态有引导，有些只显示空表格。
**Why it happens:** 各页面独立开发，没有统一的状态处理模式。
**How to avoid:** 审查所有列表页面（用户、主机、出口 IP、事件、任务）确保一致的 loading 骨架屏和空状态展示。现有 hosts/index.tsx 和 users/index.tsx 已有 skeleton 和空状态，以此为模版扩展到其他页面。
**Warning signs:** 页面加载时闪烁或突然出现内容。

## Code Examples

### BATS 测试 bootstrap 脚本契约
```bash
#!/usr/bin/env bats
# tests/smoke/bootstrap.bats

setup() {
    load 'test_helper/common'
    MOCK_PORT=$(get_free_port)
    BOOTSTRAP_API="http://127.0.0.1:${MOCK_PORT}"
    export BOOTSTRAP_API
}

teardown() {
    kill_mock_server
}

@test "auth_invalid error code produces exit code 10" {
    start_mock_server "$MOCK_PORT" 401 '{"error_code":"auth_invalid","message":"用户名或密码错误"}'
    run bash -c "echo -e 'user1\nwrong' | bash deploy/bootstrap/cloud-bootstrap.sh"
    [ "$status" -eq 10 ]
    [[ "$output" == *"用户名或密码错误"* ]]
}

@test "account_expired produces exit code 12" {
    start_mock_server "$MOCK_PORT" 403 '{"error_code":"account_expired","message":"账号已过期，请联系管理员续期"}'
    run bash -c "echo -e 'user2\npass' | bash deploy/bootstrap/cloud-bootstrap.sh"
    [ "$status" -eq 12 ]
}

@test "network timeout produces exit code 2" {
    # 不启动 mock server，curl 连接被拒
    run bash -c "echo -e 'user1\npass' | bash deploy/bootstrap/cloud-bootstrap.sh"
    [ "$status" -eq 2 ]
    [[ "$output" == *"连接控制面失败"* ]]
}
```

### Admin API 测试 — JWT 认证边界
```go
package http

func TestAdminAuth_InvalidToken(t *testing.T) {
    adminCfg := &repository.AdminConfig{
        Username:  "admin",
        Password:  "adminpass",
        JWTSecret: []byte("test-secret"),
    }
    router := NewRouter(Dependencies{
        Logger: slog.Default(),
        Admin:  adminCfg,
        // ... 其他 stub 依赖
    })

    req := httptest.NewRequest(nethttp.MethodGet, "/v1/admin/dashboard/stats", nil)
    req.Header.Set("Authorization", "Bearer invalid-token")
    rec := httptest.NewRecorder()
    router.ServeHTTP(rec, req)

    if rec.Code != nethttp.StatusUnauthorized {
        t.Errorf("status = %d, want 401", rec.Code)
    }
}

func TestAdminAuth_NoToken(t *testing.T) {
    adminCfg := &repository.AdminConfig{
        Username:  "admin",
        Password:  "adminpass",
        JWTSecret: []byte("test-secret"),
    }
    router := NewRouter(Dependencies{
        Logger: slog.Default(),
        Admin:  adminCfg,
    })

    req := httptest.NewRequest(nethttp.MethodGet, "/v1/admin/users", nil)
    rec := httptest.NewRecorder()
    router.ServeHTTP(rec, req)

    if rec.Code != nethttp.StatusUnauthorized {
        t.Errorf("status = %d, want 401", rec.Code)
    }
}
```

### 优雅关闭验证测试
```go
func TestApp_GracefulShutdown(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    app := setupTestApp(t) // 使用 testcontainers PostgreSQL

    errCh := make(chan error, 1)
    go func() {
        errCh <- app.Run(ctx)
    }()

    // 等待服务就绪
    waitForHealthy(t, app.cfg.Addr)

    // 触发关闭
    cancel()

    select {
    case err := <-errCh:
        if !errors.Is(err, context.Canceled) {
            t.Errorf("unexpected error: %v", err)
        }
    case <-time.After(15 * time.Second):
        t.Fatal("shutdown did not complete within 15 seconds")
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `log` 包 | `log/slog` 标准库 | Go 1.21 (2023) | 项目已使用 slog，切换 JSONHandler 即可 |
| 自建测试容器脚本 | testcontainers-go | 持续演进 | PostgreSQL module 简化集成测试 |
| 手工 curl 验证 | BATS 自动化测试 | 长期稳定 | 可重复、CI 友好 |

## Open Questions

1. **控制面 main.go 缺失**
   - What we know: `cmd/host-agent/main.go` 存在，但没有 `cmd/control-plane/main.go`。`app.go` 提供了完整的 `App.Run()` 方法，但没有独立的 main 入口。systemd 和 compose 都引用了 `control-plane` 二进制文件。
   - What's unclear: 控制面二进制是否通过其他方式构建（例如直接 `go build` 指向 app 包），还是 main.go 确实需要创建。
   - Recommendation: Phase 6 部署文档中需要补齐控制面的构建方式。如果 main.go 缺失，作为 06-02（部署文档）的前置任务补齐。

2. **控制面 Dockerfile 缺失**
   - What we know: `deploy/compose/control-plane.dev.yml` 引用 `deploy/docker/control-plane/Dockerfile`，但该路径下没有文件。
   - What's unclear: 是否曾计划在此阶段创建。
   - Recommendation: 作为 06-02 部署文档任务的一部分补齐。

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | 后端测试和构建 | ✓ | 1.25.7 (go.mod) | — |
| Docker Engine | 集成测试和容器管理 | 需要目标宿主机 | — | 集成测试需要，无 fallback |
| PostgreSQL | 数据持久化和集成测试 | 通过 testcontainers 按需启动 | 18.3 (compose) | — |
| Node.js | 前端构建 | ✓ | 需确认 | — |
| BATS | Shell 脚本测试 | 需安装 | — | 可通过 npm 安装 |

**Missing dependencies with no fallback:**
- 集成测试必须有 Docker daemon，CI 环境需确保可用。

**Missing dependencies with fallback:**
- BATS 可通过 `npm install --save-dev bats` 或 `brew install bats-core` 安装。

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` 标准库 (1.25.7) + BATS (1.x) |
| Config file | 无需配置文件（Go 标准工具链） |
| Quick run command | `go test ./internal/...` |
| Full suite command | `go test -tags integration ./... && bats tests/smoke/` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ACCS-01 | curl 启动命令端到端可用 | smoke (BATS) | `bats tests/smoke/bootstrap.bats` | ❌ Wave 0 |
| ACCS-03 | SSH handoff 完整链路 | integration | `go test -tags integration ./tests/integration/ -run TestBootstrapHandoff` | ❌ Wave 0 |
| NET-05 | 出口 IP 和 DNS 校验通过后才可接入 | unit + integration | `go test ./internal/network/... && go test -tags integration ./tests/integration/ -run TestNetworkVerification` | ✅ (unit) / ❌ (integration) |
| ADMN-03 | Admin API CRUD 正常/异常路径 | unit | `go test ./internal/controlplane/http/ -run TestAdmin` | ❌ Wave 0 |
| ADMN-04 | 事件记录和查询 | unit | `go test ./internal/controlplane/http/ -run TestAdminEvents` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/...` (快速单元测试)
- **Per wave merge:** `go test -tags integration ./...` (含集成测试)
- **Phase gate:** 全量测试通过 + BATS 脚本测试通过

### Wave 0 Gaps
- [ ] `tests/integration/helpers_test.go` — TestMain + testcontainers PostgreSQL 初始化
- [ ] `tests/integration/bootstrap_test.go` — bootstrap 全链路集成测试 (ACCS-01, ACCS-03)
- [ ] `tests/integration/admin_api_test.go` — Admin API CRUD 集成测试 (ADMN-03)
- [ ] `internal/controlplane/http/admin_users_test.go` — Admin Users handler 单元测试
- [ ] `internal/controlplane/http/admin_egress_ips_test.go` — Admin EgressIPs handler 单元测试
- [ ] `internal/controlplane/http/admin_hosts_test.go` — Admin Hosts handler 单元测试
- [ ] `internal/controlplane/http/admin_events_test.go` — Admin Events handler 单元测试
- [ ] `internal/controlplane/scheduler/expiry_test.go` — 到期扫描器单元测试 (D-05)
- [ ] `internal/controlplane/scheduler/reconciler_test.go` — 对账器单元测试 (D-05)
- [ ] `tests/smoke/bootstrap.bats` — bootstrap 脚本错误码契约测试 (D-03)
- [ ] BATS 安装：`npm install --save-dev bats bats-assert bats-support`

## 部署文档结构建议

基于 D-06 到 D-09 的锁定决策，部署文档推荐以下结构：

```
docs/
├── deployment-guide.md      # 首次部署指南（检查清单式）
├── operations-manual.md     # 日常运维手册
├── recovery-runbook.md      # 故障排查与恢复
└── pre-launch-checklist.md  # 上线前检查清单
deploy/
├── scripts/
│   ├── host-preflight.sh    # 现有宿主机依赖检查（扩展）
│   ├── deploy.sh            # 新增：自动化部署脚本
│   └── backup.sh            # 新增：数据库备份脚本
```

**文档锚点（已存在可直接引用的文件）：**
- `deploy/scripts/host-preflight.sh` — 宿主机依赖检查
- `deploy/systemd/claudedock-control-plane.service` — 控制面 systemd 单元
- `deploy/systemd/claudedock-host-agent.service` — host-agent systemd 单元
- `deploy/docker/managed-user/build-managed-image.sh` — 受管镜像构建
- `deploy/compose/control-plane.dev.yml` — 开发环境参考（生产需独立说明差异）

## 上线前安全检查要点

基于 D-13 到 D-15，以下为安全审查的具体检查项：

### API 权限边界（基于 router.go 审查）
| 端点 | 要求 | 当前状态 |
|------|------|----------|
| `GET /healthz` | 公开 | ✅ 无认证 |
| `GET /v1/bootstrap/script` | 公开 | ✅ 无认证 |
| `POST /v1/bootstrap/sessions` | 用户认证（用户名+密码） | ✅ bcrypt 校验 |
| `GET /v1/bootstrap/tasks/{id}` | 无认证（通过 taskID 隐式鉴权） | ⚠️ 需确认 taskID 是否足够随机 |
| `GET /v1/bootstrap/tasks/{id}/handoff` | 无认证 | ⚠️ 同上 |
| `POST /v1/admin/login` | 管理员凭证 | ✅ hmac.Equal 常量时间比对 |
| `GET/POST/PATCH/DELETE /v1/admin/*` | JWT | ✅ AdminAuthMiddleware |

### 敏感字段泄露检查项
- [ ] `EgressIP` 的 `WgPresharedKey` 字段是否在 Admin API 响应中暴露
- [ ] `AdminConfig.JWTSecret` 是否在任何响应中泄露
- [ ] 用户 `PasswordHash` 字段是否在 API 响应中暴露
- [ ] 日志中是否打印了密码、token 或 WireGuard 私钥

### 稳定性检查项（基于 app.go 审查）
- [x] 控制面优雅关闭：`ctx.Done() → server.Shutdown(10s timeout) → schedCancel() → schedDone`
- [x] host-agent 优雅关闭：`ctx.Done() → server.Shutdown()`
- [ ] PostgreSQL 连接池配置：当前使用 `pgxpool.New` 默认配置，需确认生产适用性
- [ ] 容器清理：reconciler 发现已停止容器后更新 DB 状态，但不清理容器资源

## Sources

### Primary (HIGH confidence)
- 项目代码库直接审查 — app.go、router.go、bootstrap_errors.go、所有 *_test.go 文件、deploy/ 目录
- Go 官方文档 `log/slog` — JSONHandler 和 LevelVar 配置
- testcontainers-go 官方文档 — PostgreSQL module 用法和配置

### Secondary (MEDIUM confidence)
- BATS-core 文档 — bash 测试框架用法和断言库

### Tertiary (LOW confidence)
- 无

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — 全部沿用现有项目设施和 Go 标准库
- Architecture: HIGH — 测试分层和目录结构基于现有 9 个测试文件的已证实模式
- Pitfalls: HIGH — 基于项目实际代码审查识别的具体风险点

**Research date:** 2026-03-27
**Valid until:** 2026-04-27（稳定领域，30 天有效）
