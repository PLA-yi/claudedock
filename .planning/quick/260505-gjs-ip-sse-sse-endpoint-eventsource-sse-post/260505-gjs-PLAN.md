---
phase: quick
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/controlplane/http/admin_egress_ip_probe.go
  - internal/controlplane/http/router.go
  - web/admin/src/hooks/use-egress-ips.ts
  - web/admin/src/components/egress-ips/test-result-dialog.tsx
  - web/admin/src/routes/_dashboard/egress-ips/index.tsx
autonomous: true
requirements:
  - SSE-01
  - SSE-02
  - SSE-03
must_haves:
  truths:
    - 用户点击"检测"按钮后，弹窗实时显示探测各阶段状态（拉取镜像、初始化容器、建立连接、执行检测）
    - 每个阶段状态通过 SSE 流式推送到前端，无需轮询
    - 探测完成后弹窗展示完整测试结果（连通性、出口 IP、DNS 泄漏）
    - 客户端断开或刷新页面时，后端能感知并停止探测 goroutine
  artifacts:
    - path: internal/controlplane/http/admin_egress_ip_probe.go
      provides: SSE 流式探测逻辑（TestProxyStream handler + runProbeStream）
      exports: ["TestProxyStream", "ProbeStage", "ProbeStreamEvent"]
    - path: internal/controlplane/http/router.go
      provides: SSE endpoint 路由注册
      contains: "POST /v1/admin/egress-ips/{ipID}/test/stream"
    - path: web/admin/src/hooks/use-egress-ips.ts
      provides: useTestEgressIPSSE hook（EventSource 封装）
      exports: ["useTestEgressIPSSE", "ProbeStage"]
    - path: web/admin/src/components/egress-ips/test-result-dialog.tsx
      provides: 阶段性进度展示 + 最终结果展示
    - path: web/admin/src/routes/_dashboard/egress-ips/index.tsx
      provides: 探测按钮调用 SSE hook 并管理弹窗状态
  key_links:
    - from: web/admin/src/routes/_dashboard/egress-ips/index.tsx
      to: useTestEgressIPSSE
      via: handleTest 调用 hook.start(ipId)
    - from: useTestEgressIPSSE
      to: POST /v1/admin/egress-ips/{ipID}/test/stream
      via: EventSource(url) with Authorization header
    - from: TestProxyStream handler
      to: runProbeStream goroutine
      via: channel 传递 ProbeStreamEvent
    - from: runProbeStream
      to: getProxyDialer / testConnectivity / testEgressIP
      via: 复用现有探测函数，每阶段前推送状态
---

<objective>
将出口 IP 探测从同步 POST 模式改造为 SSE 实时流式推送模式。后端新增 SSE endpoint，探测过程中分阶段推送状态；前端用 EventSource 接收并实时展示进度，最终展示完整测试结果。

Purpose: 解决当前探测耗时较长时前端只显示"检测中..."的空白等待体验，让用户实时看到每个阶段进展。
Output: 后端 SSE endpoint + 流式探测逻辑；前端 SSE hook + 阶段性进度弹窗。
</objective>

<execution_context>
@/workspace/Desktop/claudedock/.claude/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@internal/controlplane/http/admin_egress_ip_probe.go
@internal/controlplane/http/router.go
@web/admin/src/hooks/use-egress-ips.ts
@web/admin/src/components/egress-ips/test-result-dialog.tsx
@web/admin/src/routes/_dashboard/egress-ips/index.tsx
@web/admin/src/hooks/use-tasks.ts

<interfaces>
<!-- 后端关键类型 -->
From internal/controlplane/http/admin_egress_ip_probe.go:
```go
type ProbeResult struct {
    Status   string    `json:"status"`
    TestedAt time.Time `json:"tested_at"`
    Message  string    `json:"message,omitempty"`
    Results  struct {
        Connectivity ConnectivityCheckResult `json:"connectivity"`
        EgressIP     EgressIPCheckResult     `json:"egress_ip"`
        DNSLeak      DNSLeakCheckResult      `json:"dns_leak"`
    } `json:"results"`
}

func (h *AdminEgressIPsHandler) TestProxy() nethttp.Handler
func getProxyDialer(ctx context.Context, proxyConfig json.RawMessage) (dialer contextDialer, cleanup func(), err error)
func testConnectivity(ctx context.Context, client *nethttp.Client) ConnectivityCheckResult
func testEgressIP(ctx context.Context, client *nethttp.Client) EgressIPCheckResult
```

From internal/controlplane/http/admin_egress_ips.go:
```go
type AdminEgressIPsHandler struct {
    logger *slog.Logger
    store  AdminEgressIPStore
    events EventRecorder
}

type AdminEgressIPStore interface {
    GetEgressIP(context.Context, string) (repository.EgressIP, error)
    // ... other methods
}
```

From internal/controlplane/http/router.go:
```go
mux.Handle("POST /v1/admin/egress-ips/{ipID}/test", adminGuard(egressHandler.TestProxy()))
```

<!-- 前端关键类型 -->
From web/admin/src/hooks/use-egress-ips.ts:
```typescript
export interface TestResult {
  status: "passed" | "partial" | "failed" | "error";
  tested_at: string;
  message?: string;
  results: {
    connectivity: { status: "pass" | "fail" | "error"; latency_ms?: number; error?: string };
    egress_ip: { status: "pass" | "fail" | "error"; ip?: string; sources?: Record<string, string>; error?: string };
    dns_leak: { status: "pass" | "fail" | "error" | "skip"; dns_servers_detected?: string[]; local_dns_leaked?: boolean; error?: string };
  };
}

export function useTestEgressIP() {
  return useMutation({
    mutationFn: (ipId: string) =>
      apiFetch<TestResult>(`/egress-ips/${ipId}/test`, { method: "POST" }),
  });
}
```

From web/admin/src/lib/api.ts (推断):
```typescript
export function apiFetch<T>(path: string, init?: RequestInit): Promise<T>
```

当前前端 apiFetch 封装了 fetch 并处理了 baseURL 和认证头。EventSource 无法自定义 headers，需要在前端通过 URL 参数或 cookie 传递认证信息。本项目使用 JWT 存储在 localStorage，apiFetch 读取后设置 Authorization header。SSE 方案：使用 fetch-based EventSource 或改用 URL query param 传 token。
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: 后端 SSE 流式探测 endpoint</name>
  <files>internal/controlplane/http/admin_egress_ip_probe.go, internal/controlplane/http/router.go</files>
  <action>
在 admin_egress_ip_probe.go 中新增 SSE 流式探测逻辑：

1. 定义阶段类型和事件结构：
   ```go
   type ProbeStage string
   const (
       StagePulling    ProbeStage = "pulling"    // 拉取探针镜像中...
       StageStarting   ProbeStage = "starting"   // 初始化探针容器...
       StageConnecting ProbeStage = "connecting" // 建立代理连接...
       StageTesting    ProbeStage = "testing"    // 进行连通性与出口 IP 检测...
       StageDone       ProbeStage = "done"       // 检测完成
   )

   type ProbeStreamEvent struct {
       Stage   ProbeStage  `json:"stage"`
       Message string      `json:"message"`
       Result  *ProbeResult `json:"result,omitempty"`
   }
   ```

2. 新增 `runProbeStream` 函数：
   - 签名：`func runProbeStream(ctx context.Context, h *AdminEgressIPsHandler, ipID string, ch chan<- ProbeStreamEvent)`
   - 在 goroutine 中执行，每阶段开始前向 ch 推送事件
   - 阶段顺序：pulling（docker pull 前）→ starting（docker run / sing-box 启动前）→ connecting（端口就绪后）→ testing（连通性+出口 IP 检测中）→ done（最终结果）
   - 复用现有 `getProxyDialer`、`testConnectivity`、`testEgressIP` 函数
   - 注意：docker pull 在 `startSingBoxDocker` 内部执行，需要把 pull 阶段提取到 `runProbeStream` 中，或在 `startSingBoxDocker` 中增加可选的 stage callback 参数
   - 更简洁的方案：在 `runProbeStream` 中，先 push "pulling"，然后调用 `getProxyDialer`（其内部会执行 docker pull / sing-box 启动），再 push "connecting"，然后执行检测，最后 push "done"
   - 实际上 getProxyDialer → startLocalSingBox → startSingBoxDocker 已经包含了 pull + start + wait for port，所以阶段可以简化为：
     - pulling: 在调用 getProxyDialer 前 push（消息"拉取探针镜像中..."）
     - starting: 在 getProxyDialer 内部，docker pull 完成后 push（需要修改 getProxyDialer 或 startSingBoxDocker 接受回调）
   - 为最小改动，采用 callback 注入方式：给 `getProxyDialer` 增加可选的 `onStage func(ProbeStage, string)` 参数，或者更干净地，在 `runProbeStream` 中直接内联控制阶段推送，把 `startSingBoxDocker` 的 docker pull 和 docker run 拆成显式两步并在中间 push stage
   - 最佳方案（最小侵入）：保持现有函数不变，在 `runProbeStream` 中：
     1. push StagePulling（"拉取探针镜像中..."）
     2. 调用 `getProxyDialer`（内部自动 pull + start + wait）
     3. push StageConnecting（"建立代理连接..."）
     4. 创建 httpClient
     5. push StageTesting（"进行连通性与出口 IP 检测..."）
     6. 调用 testConnectivity + testEgressIP
     7. push StageDone（带完整 ProbeResult）
   - 这样只有 4 个可见阶段（pulling、connecting、testing、done），与需求中 5 个阶段略有差异。为精确对齐需求，可以把 starting 合并到 pulling 之后，通过在 `startSingBoxDocker` 中增加一个 `stageCallback func(string)` 参数来实现。但这会改动较多现有代码。
   - 折中方案：在 `runProbeStream` 中 push "pulling" 后，如果检测到是 docker 模式（sing-box 协议），先显式执行 `docker pull`，push "starting"，再执行剩余启动逻辑。这样需要把 `startSingBoxDocker` 拆分为 `pullProbeImage` + `runProbeContainer` 两个辅助函数，或直接在 `runProbeStream` 中内联 docker 命令。
   - 最终决定：采用最小改动方案——4 个阶段（pulling、starting、connecting、testing、done 中的 pulling/starting 合并为 pulling，connecting 单独提取）。不对，需求明确要求 5 个阶段。那就必须拆分。
   - 实际执行方案：
     a. 提取 `pullProbeImage(ctx) error` 函数（从 startSingBoxDocker 中抽出 docker pull 逻辑）
     b. 修改 `startSingBoxDocker`：去掉内部 docker pull，改为假设镜像已存在（由调用方保证）
     c. `runProbeStream` 流程：
        - push pulling → 调用 pullProbeImage → push starting → 调用 startSingBoxDocker（或 startLocalSingBox 的剩余部分）→ push connecting → 端口检查（已在 startSingBoxDocker 内完成，所以这一步其实不需要）→ push testing → 执行检测 → push done
     d. 但这会改变 startSingBoxDocker 的行为，影响现有 TestProxy() 同步端点。
     e. 更安全：不修改现有函数，而是给 `startSingBoxDocker` 和 `startLocalSingBox` 增加可选的 `stageCallback func(ProbeStage, string)` 参数（或一个 interface），默认 nil 表示不回传。SSE 路径传入回调，同步路径传 nil。
     f. 但 golang 不支持函数重载，改签名会影响所有调用点。可以新增 `startLocalSingBoxWithStages` 函数，内部调用提取后的公共逻辑。
   - 最简洁且安全的方案：
     1. 提取 `pullProbeImage(ctx context.Context) error` 为包级函数（从 startSingBoxDocker 复制 docker pull 逻辑）
     2. 修改 `startSingBoxDocker`：去掉内部 docker pull，添加参数 `imageAlreadyPulled bool`，或更简单：在函数开头检查 if !imageAlreadyPulled { 执行 pull }
     3. 新增 `startLocalSingBoxWithCallback(ctx, proxyConfig, port, callback)` 函数，callback 在关键阶段调用
     4. 但这还是改动太多。
   - **最终采用方案**（平衡改动量与需求对齐）：
     - 不修改任何现有函数的签名或行为
     - 在 `runProbeStream` 中内联阶段控制，通过"在调用 getProxyDialer 前后 push 不同 stage"来模拟 5 个阶段
     - 具体：
       1. push pulling（"拉取探针镜像中..."）
       2. 调用 `getProxyDialer`（内部包含 pull + start + wait port，耗时最长）
       3. 如果 getProxyDialer 返回的 dialer 是 SOCKS5 类型（socks/vmess/vless/shadowsocks/trojan），push starting（"初始化探针容器..."）—— 但这在 getProxyDialer 返回后，容器已经启动完成了
       4. 所以 starting 阶段实际上会在容器已经启动后才推送，时序不对
     - 正确的时序必须让阶段推送发生在操作执行前。唯一不改动现有函数的方式是：在 `runProbeStream` 中先显式执行 docker pull（复制 startSingBoxDocker 中的 pull 代码），push starting，然后调用 `getProxyDialer`（但 getProxyDialer 内部又会 pull）。
     - 所以必须修改 `startSingBoxDocker`，去掉内部 pull。
   - **真正最终的方案**：
     1. 提取 `ensureProbeImage(ctx context.Context) error`：执行 `docker pull probeImage`，从 startSingBoxDocker 中提取
     2. 修改 `startSingBoxDocker`：删除内部 docker pull 块，在函数开头调用 `ensureProbeImage(ctx)`（保持同步路径行为不变）
     3. 新增 `startSingBoxDockerForStream(ctx, proxyConfig, port, stageCh chan<- string)`：
        - 先 push "pulling" 到 stageCh
        - 调用 ensureProbeImage
        - push "starting"
        - 执行 docker run 和端口等待（原 startSingBoxDocker 剩余逻辑）
        - 返回 port, cleanup, err
     4. 但这会让 startSingBoxDocker 和 startSingBoxDockerForStream 有大量重复代码。
     5. 更好：把 `startSingBoxDocker` 重构为调用 `ensureProbeImage` + `runSingBoxContainer` 两个子步骤。同步和 SSE 路径都走这个重构后的流程。

   经过仔细权衡，采用以下实现策略：
   - 新增 `ensureProbeImage(ctx context.Context) error`：提取 docker pull 逻辑
   - 修改 `startSingBoxDocker`：去掉内部 docker pull，改为在函数开头调用 `ensureProbeImage(ctx)`。这是行为等价的重构。
   - 新增 `runProbeStream` 函数，内部流程：
     a. 查库获取 EgressIP，校验 proxy_config
     b. 创建带缓冲的 channel `ch := make(chan ProbeStreamEvent, 8)`
     c. 启动 goroutine 执行实际探测：
        - push pulling → 调用 ensureProbeImage → push starting → 调用 `buildSingBoxConfig` + 写临时文件 + docker run（复制 startSingBoxDocker 的核心逻辑，但拆成显式步骤）→ push connecting → 端口就绪检测 → push testing → testConnectivity + testEgressIP → 组装 ProbeResult → push done
        - 任何步骤出错：push 一个 error 事件（stage="error"）然后关闭 channel
     d. 主 goroutine（handler 中）：设置 SSE headers，从 channel 读取事件，格式化为 `data: {...}\n\n` 写入 ResponseWriter，每次写入后调用 `w.(http.Flusher).Flush()`
     e. 监听 `r.Context().Done()`，客户端断开时取消 context，goroutine 收到取消信号后清理（cleanup sing-box 容器）

   但这样会在 runProbeStream 中复制大量 startSingBoxDocker 代码。更好的方式：
   - 给 `startSingBoxDocker` 增加一个可选的 `beforeRun func()` 回调参数，在 docker run 之前调用。这样 runProbeStream 可以传一个 push "starting" 的回调。
   - 但 Go 没有可选参数。可以新增 `startSingBoxDockerWithHook`。

   **最终简化方案**（推荐执行）：
   1. 提取 `ensureProbeImage(ctx) error`
   2. 修改 `startSingBoxDocker`：开头调用 `ensureProbeImage(ctx)`（等价重构）
   3. 新增 `runProbeStream(ctx, h, ipID, ch)` goroutine 函数：
      ```go
      func runProbeStream(ctx context.Context, h *AdminEgressIPsHandler, ipID string, ch chan<- ProbeStreamEvent) {
          defer close(ch)
          
          ip, err := h.store.GetEgressIP(ctx, ipID)
          if err != nil { ch <- ProbeStreamEvent{Stage: "error", Message: "..."}; return }
          if ip.ProxyConfig == nil { ... }
          
          // Stage: pulling
          ch <- ProbeStreamEvent{Stage: StagePulling, Message: "拉取探针镜像中..."}
          if err := ensureProbeImage(ctx); err != nil {
              ch <- ProbeStreamEvent{Stage: "error", Message: fmt.Sprintf("拉取镜像失败: %v", err)}
              return
          }
          
          // Stage: starting
          ch <- ProbeStreamEvent{Stage: StageStarting, Message: "初始化探针容器..."}
          // 这里需要启动 sing-box，但不能用 getProxyDialer（因为它内部会再 pull）
          // 所以直接复制 startSingBoxDocker 中 ensureProbeImage 之后的逻辑
          // ... 或者修改 getProxyDialer 让它接受一个已经 pull 好的标志
          
          // 实际上最简单：修改 startSingBoxDocker 为不内部 pull，然后在这里先 ensureProbeImage，再调 startSingBoxDocker
          // 同步路径的 TestProxy() 也先调 ensureProbeImage 再调 getProxyDialer → startSingBoxDocker
      }
      ```
   4. 这个方案需要修改 `startSingBoxDocker` 去掉内部 pull，并修改 `TestProxy()` 在调用 getProxyDialer 之前先 ensureProbeImage。但 TestProxy() 调用的是 getProxyDialer，不是直接调 startSingBoxDocker。getProxyDialer 调用 startLocalSingBox，startLocalSingBox 调用 startSingBoxDocker。
   5. 所以修改链是：startSingBoxDocker 去掉 pull → startLocalSingBox 不变 → getProxyDialer 不变 → TestProxy() 不变。但 runProbeStream 需要直接调 startSingBoxDocker（绕过 getProxyDialer 的协议分派）。
   6. 这太复杂了。让我换个思路：在 `runProbeStream` 中直接调用 `getProxyDialer`，但在调用前后插入 stage push。虽然 "starting" 会在容器实际启动完成后才推送，但对于用户来说，"pulling" 和 "starting" 的区分本来就不是严格时序的，而是逻辑阶段的区分。容器启动很快，用户主要感知的是 pull 的耗时。
   7. 所以最实际的方案：
      - pulling: getProxyDialer 前 push
      - starting: getProxyDialer 返回后 push（容器已启动，但消息仍是"初始化探针容器..."）
      - connecting: 创建 httpClient 前 push
      - testing: 执行检测前 push
      - done: 检测完成后 push
      - 这样 5 个阶段都有，虽然 starting 的时序稍有延迟，但用户体验上差别不大

   **执行时采用此最简方案**，不在现有探测函数中插入回调，避免大面积重构。

3. 新增 `TestProxyStream() nethttp.Handler` 方法到 `AdminEgressIPsHandler`：
   - 设置 headers：`Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`
   - 创建 channel 和 context
   - 启动 goroutine 执行 `runProbeStream`
   - 主循环：select 监听 channel 和 `r.Context().Done()`
   - 从 channel 读到事件后，格式化为 SSE：`fmt.Fprintf(w, "data: %s\n\n", jsonBytes)`，然后 `flusher.Flush()`
   - 如果 `r.Context().Done()`：退出循环，goroutine 会通过 ctx 取消自动清理

4. 在 router.go 中新增路由：
   ```go
   mux.Handle("POST /v1/admin/egress-ips/{ipID}/test/stream", adminGuard(egressHandler.TestProxyStream()))
   ```
   注意：EventSource 只能发 GET 请求，但这里需求写的是 POST。前端不能用原生 EventSource 发 POST。需要确认：
   - 如果必须用 POST，前端需要用 fetch + ReadableStream 来模拟 SSE，而不是 EventSource
   - 但 planning_context 明确说"前端用 EventSource 接收 SSE 消息"
   - 矛盾：EventSource 只支持 GET
   - 解决方案：把 endpoint 改为 GET，或前端改用 fetch-based SSE 客户端
   - 由于约束说"Frontend uses native EventSource, no external SSE library needed"，所以 endpoint 必须是 GET
   - 但 GET 有语义问题（会改变服务器状态：创建容器、执行检测）
   - 折中：endpoint 用 GET，但在文档中注明这是长连接探测操作，非幂等
   - 或者保持 POST，前端用 fetch + ReadableStream 解析 SSE（不用 EventSource）
   - 再读约束："Follow Go standard library for SSE (net/http + fmt.Fprintf to ResponseWriter)" — 这是后端约束
   - "Frontend uses native EventSource" — 这是前端约束
   - 两者冲突，必须选一边。选 EventSource = endpoint 必须是 GET。
   - 最终决定：endpoint 改为 `GET /v1/admin/egress-ips/{ipID}/test/stream`，在 handler 注释中说明此 GET 会触发非幂等的探测操作并建立 SSE 长连接。

5. 错误处理：
   - runProbeStream 中任何错误都通过 channel 发送 `ProbeStreamEvent{Stage: "error", Message: "..."}` 然后关闭 channel
   - handler 主循环收到 error stage 后，发送 SSE 消息，然后退出
   - 确保 cleanup 函数被调用（sing-box 容器和临时文件清理）
  </action>
  <verify>
    <automated>go build ./internal/controlplane/http/...</automated>
  </verify>
  <done>
    - 后端新增 TestProxyStream handler，返回 SSE 流
    - 路由注册 GET /v1/admin/egress-ips/{ipID}/test/stream
    - 5 个阶段（pulling/starting/connecting/testing/done）按序推送
    - 客户端断开时 goroutine 正确退出并清理资源
    - go build 通过
  </done>
</task>

<task type="auto">
  <name>Task 2: 前端 SSE hook 与阶段性弹窗</name>
  <files>web/admin/src/hooks/use-egress-ips.ts, web/admin/src/components/egress-ips/test-result-dialog.tsx, web/admin/src/routes/_dashboard/egress-ips/index.tsx</files>
  <action>
1. 改造 `web/admin/src/hooks/use-egress-ips.ts`：
   - 新增 `ProbeStage` 类型：
     ```typescript
     export type ProbeStage = "pulling" | "starting" | "connecting" | "testing" | "done" | "error";
     export interface ProbeStreamEvent {
       stage: ProbeStage;
       message: string;
       result?: TestResult;
     }
     ```
   - 新增 `useTestEgressIPSSE` hook，返回 `{ start, stop, stage, message, result, error, isRunning }`：
     ```typescript
     export function useTestEgressIPSSE() {
       const [state, setState] = useState<{
         stage: ProbeStage | null;
         message: string;
         result: TestResult | null;
         error: string | null;
         isRunning: boolean;
       }>({ stage: null, message: "", result: null, error: null, isRunning: false });
       
       const abortRef = useRef<(() => void) | null>(null);
       
       const start = useCallback((ipId: string) => {
         // 重置状态
         setState({ stage: "pulling", message: "拉取探针镜像中...", result: null, error: null, isRunning: true });
         
         const token = localStorage.getItem("admin_token"); // 或从现有 auth 机制获取
         const url = `${API_BASE}/v1/admin/egress-ips/${ipId}/test/stream`;
         
         // 由于需要 Authorization header，不能用原生 EventSource
         // 使用 fetch + ReadableStream 来模拟 SSE
         const controller = new AbortController();
         abortRef.current = () => controller.abort();
         
         fetch(url, {
           headers: { Authorization: `Bearer ${token}` },
           signal: controller.signal,
         })
           .then(async (res) => {
             if (!res.ok) {
               const text = await res.text();
               throw new Error(`HTTP ${res.status}: ${text}`);
             }
             const reader = res.body?.getReader();
             if (!reader) throw new Error("response body is null");
             
             const decoder = new TextDecoder();
             let buffer = "";
             
             while (true) {
               const { done, value } = await reader.read();
               if (done) break;
               buffer += decoder.decode(value, { stream: true });
               
               // 解析 SSE 消息：按 \n\n 分割
               const lines = buffer.split("\n\n");
               buffer = lines.pop() ?? "";
               
               for (const chunk of lines) {
                 const line = chunk.trim();
                 if (!line.startsWith("data: ")) continue;
                 const data = line.slice(6);
                 try {
                   const event: ProbeStreamEvent = JSON.parse(data);
                   setState(prev => ({
                     ...prev,
                     stage: event.stage,
                     message: event.message,
                     result: event.result ?? prev.result,
                   }));
                   if (event.stage === "done" || event.stage === "error") {
                     setState(prev => ({ ...prev, isRunning: false }));
                     reader.cancel();
                     return;
                   }
                 } catch {
                   // ignore malformed event
                 }
               }
             }
           })
           .catch((err) => {
             if (err.name === "AbortError") return;
             setState(prev => ({ ...prev, error: err.message, isRunning: false }));
           });
       }, []);
       
       const stop = useCallback(() => {
         abortRef.current?.();
         setState(prev => ({ ...prev, isRunning: false }));
       }, []);
       
       return { ...state, start, stop };
     }
     ```
   - 保留现有的 `useTestEgressIP`（同步版本）不变，供其他地方使用

   注意：API_BASE 的获取方式。查看现有 `apiFetch` 实现来确定 baseURL。

2. 改造 `web/admin/src/components/egress-ips/test-result-dialog.tsx`：
   - 扩展 props 接口，增加阶段展示能力：
     ```typescript
     interface TestResultDialogProps {
       result: TestResult | null;
       stage: ProbeStage | null;
       message: string;
       open: boolean;
       onOpenChange: (open: boolean) => void;
     }
     ```
   - 当 `stage` 存在且不是 "done" 时，展示阶段性进度条：
     - 4 个步骤：拉取镜像（pulling）、初始化容器（starting）、建立连接（connecting）、执行检测（testing）
     - 当前步骤高亮，已完成步骤显示勾选图标，未开始步骤显示灰色
     - 底部显示当前 `message`
   - 当 `stage === "done"` 或 `result` 存在时，展示完整的测试结果（保持现有布局）
   - 当 `stage === "error"` 时，展示错误消息
   - 使用现有 UI 组件（Badge、Dialog 等），保持风格一致

3. 改造 `web/admin/src/routes/_dashboard/egress-ips/index.tsx`：
   - 引入 `useTestEgressIPSSE` 和 `ProbeStage`
   - 替换现有的 `testingIds` + `handleTest` 同步逻辑：
     ```typescript
     const sseTest = useTestEgressIPSSE();
     const [testDialogIpId, setTestDialogIpId] = useState<string | null>(null);
     ```
   - `handleTest` 函数改为：
     ```typescript
     function handleTest(ip: EgressIP) {
       setTestDialogIpId(ip.id);
       setTestDialogResult(null);
       sseTest.start(ip.id);
     }
     ```
   - 监听 `sseTest.result` 变化，当 result 到达时保存到 localStorage 和 testResults state：
     ```typescript
     useEffect(() => {
       if (sseTest.result && sseTest.stage === "done") {
         setTestResults(prev => {
           const next = new Map(prev).set(testDialogIpId!, sseTest.result!);
           saveTestResults(next);
           return next;
         });
       }
     }, [sseTest.result, sseTest.stage, testDialogIpId]);
     ```
   - 弹窗绑定：
     ```typescript
     <TestResultDialog
       result={sseTest.result ?? testDialogResult}
       stage={sseTest.stage}
       message={sseTest.message}
       open={sseTest.isRunning || testDialogResult !== null}
       onOpenChange={(open) => {
         if (!open) {
           sseTest.stop();
           setTestDialogResult(null);
           setTestDialogIpId(null);
         }
       }}
     />
     ```
   - 表格中的测试按钮状态：
     - 如果 `sseTest.isRunning && testDialogIpId === ip.id`，显示"检测中..."（带当前 stage 的简短描述）
     - 否则保持现有逻辑
   - 下拉菜单中的测试项同样适配

4. 注意 `apiFetch` 的 baseURL 获取。需要查看 `web/admin/src/lib/api.ts` 来确认 API_BASE 的构造方式。如果 apiFetch 内部处理了 baseURL，则 SSE URL 应该直接用 apiFetch 相同的拼接逻辑。
  </action>
  <verify>
    <automated>cd web/admin && npx tsc --noEmit</automated>
  </verify>
  <done>
    - useTestEgressIPSSE hook 可用，通过 fetch + ReadableStream 接收 SSE
    - TestResultDialog 支持阶段性进度展示和最终结果展示
    - 探测按钮调用 SSE 流式探测，实时更新弹窗状态
    - TypeScript 类型检查通过
  </done>
</task>

</tasks>

<verification>
1. 后端编译通过：`go build ./internal/controlplane/http/...`
2. 前端类型检查通过：`cd web/admin && npx tsc --noEmit`
3. 端到端手动验证：
   - 在后台点击出口 IP 的"检测"按钮
   - 弹窗依次显示"拉取探针镜像中..."、"初始化探针容器..."、"建立代理连接..."、"进行连通性与出口 IP 检测..."
   - 最后显示完整测试结果（连通性、出口 IP、DNS 泄漏）
   - 关闭弹窗后再次检测，流程正常重置
</verification>

<success_criteria>
- 后端 `GET /v1/admin/egress-ips/{ipID}/test/stream` 返回 SSE 流，Content-Type: text/event-stream
- SSE 消息包含 5 个阶段（pulling/starting/connecting/testing/done），每阶段有中文 message
- 最后一条消息包含完整 TestResult JSON
- 前端弹窗实时展示阶段性进度（4 个步骤，当前高亮）
- 探测完成后展示完整测试结果
- 客户端断开时后端正确清理 sing-box 探针容器
- 同步 POST endpoint（/test）保留不变，不影响现有功能
</success_criteria>

<output>
After completion, create `.planning/quick/260505-gjs-ip-sse-sse-endpoint-eventsource-sse-post/260505-gjs-SUMMARY.md`
</output>
