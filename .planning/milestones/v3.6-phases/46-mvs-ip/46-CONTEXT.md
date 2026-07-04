# Phase 46: MVS 黄金路径与出口 IP 验证 - Context

**Gathered:** 2026-05-14
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous, 4/4 grey areas accepted)

<domain>
## Phase Boundary

把"首次 bootstrap → 进入 SSH → 出网经由绑定的出口 IP → DNS 走 tun → 直连外网被拒绝 → CLI 错误码契约稳定"这条用户主路径用 e2e 跑通，作为 MVS（Minimum Viable Suite）第一组真实可信用例。

**本 phase 交付 5 条 MVS 不变量的真实 e2e 用例**（每条对应一个 plan）：

- MVS-01：bootstrap 黄金路径（curl → 认证 → 容器启动 → SSH banner）
- MVS-02：出口 IP 匹配验证（三源轮询 + 多数派裁决）
- MVS-03：DNS 强制走 tun 或被防火墙拒绝
- MVS-04：默认拒绝矩阵（多 IP × 多端口）
- MVS-05：CLI 错误码契约（auth_invalid=10 / account_disabled=11 / account_expired=12 / host_not_found=13）

**不在本 phase 范围**：

- 治理与心跳（属 Phase 47）
- Kill-switch 与压力测试（属 Phase 48 / 50）
- 防泄漏对抗（属 Phase 49）
- verify.go 代码层加固（属 Phase 51）
- 完整 artifact 采集脚本（属 Phase 52）

**macOS 本地执行约束**：所有 `tests/e2e/` 用例统一 `//go:build e2e && linux` 隔离，本地 darwin 不强制跑断言；CI hosted ubuntu-24.04 runner 兜底。

</domain>

<decisions>
## Implementation Decisions

### Area 1: 用例编排与生产路径忠实度 (MVS-01)

- **bootstrap 认证路径**：真实 `claudedock` binary（`exec.CommandContext` 子进程）+ 真实控制面 HTTP，跟生产路径一条路，禁止任何 mock 认证旁路。
- **bootstrap 等待信号**：通过控制面 `events` 表查询 `host.ready` 事件 + SSH banner pump 双重确认（任一失败即用例 fail）。
- **macOS 本地行为**：整个 `tests/e2e/` 套件统一 `//go:build e2e && linux` build tag，darwin 上 `go test ./...` 不会触达；CI Linux runner 才真正跑断言。
- **bootstrap 用例命名**：`tests/e2e/bootstrap_test.go` + `TestBootstrap_GoldenPath`。

### Area 2: 出口 IP / DNS / 默认拒绝矩阵 (MVS-02 / MVS-03 / MVS-04)

- **出口 IP 多源轮询**：固定 3 源 `https://ip.me` / `https://ifconfig.io` / `https://ipinfo.io/ip`，并行拉取，**多数派裁决**（≥2 一致即 PASS）；若某源全部超时，**不直接 fail**，按现有"投票"语义裁决，外网抖动不误报。
- **DNS 断言风格**："tun 接管 ✅ 返回 A 记录" **OR** "firewall ❌ 拒绝/timeout" 二选一即 PASS（Go 内 OR 逻辑），失败时 artifact dump `nft list ruleset` counter 命中数。
- **默认拒绝矩阵**：固定 4 个 target — `1.1.1.1:80` / `8.8.8.8:443` / `9.9.9.9:443` / `169.254.169.254:80`，每个 connect timeout 3s；**任一连通即 fail**。
- **失败时 nft counter dump**：默认拒绝 / DNS / leak 用例失败时通过 Phase 45 已就绪的 artifact hook 自动 dump `nft list ruleset` 全量 counters，开发者无需手动复现。

### Area 3: CLI 错误码契约 (MVS-05)

- **错误码表来源**：在写 plan 前 grep `claudedock` 现有源码取当前 exit-code 常量定义，作为锁定表（避免再造）；若发现源码与 ROADMAP 描述不一致，以源码为准并在 SUMMARY 中记录差异。
- **各错误触发方式**：
  - `auth_invalid=10`：错密码触发，用真 binary + 真控制面。
  - `account_disabled=11`：DB 预置 `users.disabled=true` 的 fixture user。
  - `account_expired=12`：DB 预置 `users.expires_at < now` 的 fixture user。
  - `host_not_found=13`：传 invalid host 字段（不存在的 host slug）。
- **断言粒度**：exit code 数字 + stderr 文案 `Contains` 关键字（保留中英文兼容，避免锁完整文案造成脆性）。
- **用例组织**：单独 `tests/e2e/cli_error_codes_test.go`，table-driven，复用 Phase 45 Scenario builder 起一次 fixture。

### Area 4: 跨 Phase 复用与验证策略

- **scenario 复用**：新建 `tests/e2e/helpers.go`，抽象 `StartGoldenPath(t *testing.T) *GoldenPath`，封装"控制面 + host-agent + Postgres + sing-box gateway + 1 user + 1 host"标配，Phase 47/48/49/50 直接 import 复用。
- **helper 单测**：`tests/e2e/helpers_test.go` 对纯函数（vote 多数派裁决 / 错误码映射 / matrix 拼装）写单测，**不带 e2e build tag**，darwin 也跑，作为本 phase macOS 本地通过基线。
- **VERIFICATION 策略**：本 phase VERIFICATION.md 把"Go 代码编译 + 纯函数 unit 测试通过"判 PASS，"Linux 真机 e2e 断言"列为 deferred-to-CI 的人工核查项（非阻塞 ship，CI runner 跑通即闭环）。
- **Plan 切分粒度**：严格 5 plan 对应 5 条 MVS 不变量（46-01..05），每 plan 独立 commit，不合并。

### Claude's Discretion

以下细节实现层由 Claude 自行决定：

- `GoldenPath` 结构体的内部字段命名（建议暴露 `Scenario`、`User`、`Host`、`ControlPlaneURL`、`CLIBinary` 等访问器）
- DNS 测试具体用哪个公共域名（建议 `example.com` / `cloudflare.com`，HTTPS 协议无关）
- 默认拒绝矩阵的并发探测策略（串行 vs goroutine 并发，建议并发以加快用例 + 单一 ctx 取消）
- 三源轮询的 HTTP client 超时（建议 5s per source，总 timeout 15s）
- CLI 错误码 fixture 的 SQL 种子语句（建议放 `tests/e2e/fixtures/error-codes.sql`）
- macOS 上 `go test -tags=e2e ./tests/e2e/...` 的兜底行为（建议套件入口 `t.Skip("requires linux")`，但 `helpers_test.go` 始终参与编译）

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- **Phase 45 已交付**：`tests/e2e/harness/{suite.go, waitfor.go, scenario/, artifacts.go, dump.go}` —— 直接复用 Scenario builder 与 waitFor 4 个变体。
- **`internal/runtime/ContainerProxyProvider`**：v3.5 已拆 `PrepareGateway` + `PrepareHost`，e2e 中的 sing-box gateway + worker 启动直接复用真实 provider。
- **`cmd/claudedock/`**：CLI 主入口，错误码常量定义所在；需 grep 取当前值。
- **`internal/agentapi`**：host-agent ↔ 控制面 SDK，e2e 可 import 复用。
- **`internal/eventlog`**（如存在）：`events` 表读写 SDK，bootstrap 等待 `host.ready` 时用。
- **`scripts/uat-bypass-fixture-up.sh`**（v3.5）：fixture 起停脚本范式，Phase 46 helpers.go 内部启动序列参考其拓扑。

### Established Patterns

- **build tag 隔离**：Phase 45 已建立 `//go:build e2e` 模式，本 phase 升级为 `//go:build e2e && linux`。
- **结构化日志**：`log/slog` key-value 风格，helpers.go 内部输出沿用。
- **错误包装**：`fmt.Errorf("...: %w", err)` 链式包装上下文。
- **中文沟通**：所有面向用户的 stderr / artifact README / SUMMARY 默认中文。
- **failure-only artifact dump**：Phase 45 的 `DumpHook` 已就绪，本 phase 用例失败时调用即可。

### Integration Points

- **新增文件**：
  - `tests/e2e/helpers.go`（GoldenPath 抽象，linux+e2e tag）
  - `tests/e2e/helpers_test.go`（纯函数单测，无 build tag）
  - `tests/e2e/bootstrap_test.go`（MVS-01，linux+e2e tag）
  - `tests/e2e/egress_ip_test.go`（MVS-02，linux+e2e tag）
  - `tests/e2e/dns_test.go`（MVS-03，linux+e2e tag）
  - `tests/e2e/default_deny_test.go`（MVS-04，linux+e2e tag）
  - `tests/e2e/cli_error_codes_test.go`（MVS-05，linux+e2e tag）
  - `tests/e2e/fixtures/error-codes.sql`（CLI 错误码 fixture SQL 种子）
- **go.mod**：不预期新增依赖（沿用 Phase 45 已落地的 testcontainers-go / testify）。
- **CI 触发**：Phase 45 `e2e.yml` 的 `paths` 守护已覆盖 `tests/e2e/**`，本 phase 提交即自动跑。
- **隐私守护**：fixture SQL / helpers 输出严禁出现真实邮箱 / 个人路径；用 `test@example.com` / 相对路径。

</code_context>

<specifics>
## Specific Ideas

- **GoldenPath 访问器示例**：
  ```go
  type GoldenPath struct {
      Scenario       *harness.Scenario
      User           harness.User
      Host           harness.Host
      ControlPlaneURL string
      CLIBinary      string // 编译产物绝对路径
  }
  ```
- **三源轮询 vote 函数签名**：`Vote(results []string) (winner string, ok bool, dissent []string)`；多数派为空（全超时）时 `ok=false`，由用例决定是 skip 还是 fail。
- **默认拒绝矩阵示例**：
  ```go
  var DefaultDenyMatrix = []struct{ Host string; Port int }{
      {"1.1.1.1", 80}, {"8.8.8.8", 443}, {"9.9.9.9", 443}, {"169.254.169.254", 80},
  }
  ```
- **CLI 错误码表占位**：grep 时关注 `ExitCodeAuthInvalid / ExitCodeAccountDisabled / ExitCodeAccountExpired / ExitCodeHostNotFound` 或类似命名。

</specifics>

<deferred>
## Deferred Ideas

- **真实 Linux runner 跑通签字**：本 phase 不在 darwin 上强证，列 deferred-to-CI；CI runner 跑通即闭环。
- **Bootstrap 性能基线**：bootstrap 总耗时上限不在本 phase 锁定（属性能优化候选）。
- **多 host 并发场景**：本 phase 单 host 单 user 跑通即可，多 host 并发列后续。
- **DNS 反向缓存测试**：仅覆盖 forward query，反向 PTR 不在 MVS-03 范围。
- **CLI 错误码国际化**：当前 stderr 中英文混合断言 `Contains`，多语言切换列 v3.7+。
- **完整 artifact 收集脚本**：本 phase 沿用 Phase 45 占位 hook，Phase 52 (OBS-01..03) 再补完整 5 子目录采集逻辑。

</deferred>
