---
phase: quick-260504-dtd
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/controlplane/http/admin_egress_ip_probe.go
  - docker-compose.yml
  - docker-compose.build.yaml
autonomous: true
requirements:
  - QUICK-260504-DTD-01
  - QUICK-260504-DTD-02

must_haves:
  truths:
    - "在 macOS 宿主机直跑控制面（go run）时，触发出口 IP 探针不再报 `No such container: <hostname>`"
    - "在 Linux 容器内跑控制面时，探针仍然通过 `--network=container:<cpID>` 复用控制面 namespace（行为与今日一致）"
    - "执行 `docker compose pull` 默认会把 sing-box-gateway 镜像拉到本地，不再被 build-only profile 屏蔽"
    - "执行 `docker compose up -d` 时 sing-box-gateway 服务一次性退出 0 不进入 restart 循环"
    - "`docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build` 仍能源码构建 sing-box-gateway 镜像"
  artifacts:
    - path: "internal/controlplane/http/admin_egress_ip_probe.go"
      provides: "探针网络模式根据控制面运行环境自动决策（容器内 vs 宿主机直跑）"
      contains: "docker inspect"
    - path: "docker-compose.yml"
      provides: "sing-box-gateway 默认参与 pull、up 时一次性退出"
      contains: "command:"
    - path: "docker-compose.build.yaml"
      provides: "源码构建路径保持可用，build-only profile 与构建配置不变"
      contains: "build-only"
  key_links:
    - from: "internal/controlplane/http/admin_egress_ip_probe.go::startSingBoxDocker"
      to: "docker run 子进程"
      via: "resolveProbeNetworking helper 返回的网络参数切片拼入 exec.CommandContext args"
      pattern: "resolveProbeNetworking"
    - from: "docker-compose.yml::sing-box-gateway"
      to: "ghcr.io/.../sing-box-gateway:latest 镜像"
      via: "默认服务 + pull_policy: always + command: [\"true\"]"
      pattern: "sing-box-gateway"
---

<objective>
修复两个相互关联的体验缺陷：

1. **macOS 宿主机直跑控制面时探针报 `No such container`**：`startSingBoxDocker` 默认假设 `os.Hostname()` 返回的就是 Docker 容器名，但在 macOS 上 `go run` 时 hostname 是宿主机主机名，docker 找不到容器导致 IP 测试整体不可用。
2. **`docker compose pull` 默认不会拉 sing-box-gateway**：当前 `profiles: [build-only]` 把它从默认上下文里排除，新部署机器拉镜像时容易漏拉。

Purpose: 让本地开发者（macOS）可以在不进容器的情况下完成出口 IP 测试，同时让生产部署的 `docker compose pull && up` 默认就把 sing-box-gateway 镜像准备好。

Output:
- `internal/controlplane/http/admin_egress_ip_probe.go` 新增 `resolveProbeNetworking` helper，在 in-container 与 host-binary 两种运行模式下自动选择正确的 docker 网络参数
- `docker-compose.yml` 与 `docker-compose.build.yaml` 协同更新，让默认 pull/up 路径与源码构建路径都保持可用
</objective>

<execution_context>
@/workspace/Desktop/cloud-cli-proxy/.claude/get-shit-done/workflows/execute-plan.md
@/workspace/Desktop/cloud-cli-proxy/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@internal/controlplane/http/admin_egress_ip_probe.go
@docker-compose.yml
@docker-compose.build.yaml

<interfaces>
<!-- 关键现有契约（来自 admin_egress_ip_probe.go），执行者无需再次扫码 -->

来自 internal/controlplane/http/admin_egress_ip_probe.go:

```go
// 入口：协议为 vmess/vless/shadowsocks/trojan 时被 getProxyDialer 调用
func startLocalSingBox(ctx context.Context, proxyConfig json.RawMessage) (port int, cleanup func(), err error)

// 当前要修改的核心函数
func startSingBoxDocker(ctx context.Context, proxyConfig json.RawMessage, port int) (int, func(), error)

// 兼容口径：listen 已经是 0.0.0.0，host-binary 模式下宿主机 -p 映射可直接命中
func buildSingBoxConfig(proxyConfig json.RawMessage, listenAddr string, listenPort int) ([]byte, error)
// 现有调用：buildSingBoxConfig(proxyConfig, "0.0.0.0", port)  ← 不需要改

// 镜像引用
import "github.com/zanel1u/cloud-cli-proxy/internal/network"
network.GatewayImage()  // 返回 ghcr.io/.../sing-box-gateway:latest
```

来自 docker-compose.build.yaml（已存在的「pull-stub 服务」参考样本，managed-user-image 模式）：

```yaml
managed-user-image:
  build: { context: ., dockerfile: deploy/docker/managed-user/Dockerfile }
  image: ghcr.io/zanel1u/cloud-cli-proxy/managed-user:latest
  pull_policy: never
  command: ["true"]
  profiles:
    - build-only
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: 探针容器网络模式自适应（in-container / host-binary）</name>
  <files>internal/controlplane/http/admin_egress_ip_probe.go</files>
  <behavior>
    新增 helper `resolveProbeNetworking(ctx context.Context, port int) ([]string, error)`：
    - 行为 A（控制面在容器内）：当 `os.Hostname()` 返回非空，且 `docker inspect <hostname>`（带 2s 超时的 `exec.CommandContext`）退出码 0 且 stdout 非空时，返回 `["--network", "container:" + hostname]`。
    - 行为 B（控制面在宿主机直跑）：当 hostname 为空，或 `docker inspect` 失败/超时/输出为空时，返回 `["--network", "bridge", "-p", "127.0.0.1:<port>:<port>"]`（使用 fmt.Sprintf 把 port 拼进去）。
    - 错误处理：helper 自身不返回 error（fallback 到 host-binary 即可），签名简化为 `func resolveProbeNetworking(ctx context.Context, port int) []string`。这样上层 `startSingBoxDocker` 不需要新增 error 分支。

    helper 的最小测试覆盖（写 RED 测试时聚焦决策逻辑）：
    - Test 1：hostname 为空 → 返回 host-binary 参数切片，且包含 `127.0.0.1:<port>:<port>`
    - Test 2：注入一个永远失败的 docker inspect 命令（通过包级 `var dockerInspectRunner = ...` 钩子注入，类似项目里 Phase 29.1 的 `execInContainer` 包级 var 提升模式）→ 返回 host-binary 参数切片
    - Test 3：注入一个返回 0 + 非空 stdout 的 docker inspect 钩子 → 返回 `["--network", "container:fake-id"]`

    实现要点：
    - 在 admin_egress_ip_probe.go 引入包级 `var dockerInspectRunner = func(ctx context.Context, name string) (stdout []byte, err error) { ... 真实 exec.CommandContext ... }`，便于单测 swap，不破坏生产路径
    - 真实实现里 `exec.CommandContext(ctx2, "docker", "inspect", "--format", "{{.Id}}", name)`，ctx2 由 `context.WithTimeout(ctx, 2*time.Second)` 派生（记得 cancel）
    - 完成后修改 `startSingBoxDocker`：把原先 L276-279 的 `networkArg := "host"; if cpID, _ := os.Hostname(); cpID != "" { networkArg = "container:" + cpID }` 整段替换为 `netArgs := resolveProbeNetworking(ctx, port)`
    - 拼接 docker run 命令时由原先的固定 `"--network", networkArg` 改为通过 `append` 把 `netArgs...` 嵌入参数列表（保留原先 `-d --name containerName -v ... GatewayImage()` 顺序）
    - 不要触碰 `startSingBoxNative`、`buildSingBoxConfig`、`startLocalSingBox`、`TestProxy`、`CleanupOrphanProbes`，也不修改 listenAddr=0.0.0.0 的现状

    Rule（依据项目约束）：
    - 不写绝对路径；所有路径相对仓库根
    - 中文注释；错误消息保持中文（与 L313 现有 `"sing-box 启动超时（端口 %d）"` 风格一致）
  </behavior>
  <action>
    1. 打开 `internal/controlplane/http/admin_egress_ip_probe.go`
    2. 在文件靠后位置（建议放在 `startSingBoxDocker` 之前）新增包级 `dockerInspectRunner` var 与 `resolveProbeNetworking` helper，签名与行为参见 <behavior>
    3. 修改 `startSingBoxDocker`：删除 L276-279 旧 networkArg 逻辑，替换为 `netArgs := resolveProbeNetworking(ctx, port)`；改写 `exec.CommandContext` 参数构造为 `args := []string{"run", "-d", "--name", containerName}; args = append(args, netArgs...); args = append(args, "-v", tmpFile.Name()+":/etc/sing-box/config.json:ro", network.GatewayImage()); cmd := exec.CommandContext(ctx, "docker", args...)`
    4. 在同目录新增（或追加到既有测试文件，如果项目里已有 `admin_egress_ip_probe_test.go`/`admin_egress_ips_test.go`）轻量单测覆盖 helper 的三个分支；测试通过包级 var swap 注入假 inspect runner，不依赖真实 docker
    5. 不要触碰 startSingBoxNative / buildSingBoxConfig / TestProxy / CleanupOrphanProbes
    6. 验证 macOS 直跑路径：审阅 diff 确认 hostname=Vision-2.local 时走 host-binary 分支，不会再生成 `--network=container:Vision-2.local`
  </action>
  <verify>
    <automated>go vet ./internal/controlplane/http/... && go build ./... && go test ./internal/controlplane/http/... -run 'ResolveProbeNetworking|ProbeNetworking' -count=1</automated>
  </verify>
  <done>
    - go vet / go build 通过
    - 新增 helper 覆盖 in-container / host-binary / hostname 为空 三种决策路径，单测全 PASS
    - 旧的 `os.Hostname()` 直接拼 `container:` 的逻辑彻底消失（grep 确认 admin_egress_ip_probe.go 中不再出现裸 `"container:" + cpID` 字面量构造）
    - startSingBoxDocker 主流程其余逻辑（temp 配置文件、cleanup、deadline 轮询、docker logs 拉取）零改动
  </done>
</task>

<task type="auto">
  <name>Task 2: docker-compose 默认拉取 sing-box-gateway 且 up 不进入 restart 循环</name>
  <files>docker-compose.yml, docker-compose.build.yaml</files>
  <action>
    1. 编辑 `docker-compose.yml` 中的 `sing-box-gateway` 块：
       - 移除 `profiles: [build-only]`
       - 追加 `restart: "no"` 与 `command: ["true"]`
       - 保留 `image:` 与 `pull_policy: always`
       - 最终块形如：
         ```yaml
         sing-box-gateway:
           image: ghcr.io/zanel1u/cloud-cli-proxy/sing-box-gateway:latest
           pull_policy: always
           restart: "no"
           command: ["true"]
         ```
    2. 编辑 `docker-compose.build.yaml` 中的 `sing-box-gateway` 块，与 `managed-user-image` 已有的「pull-stub 服务」模式对齐：
       - 在现有 build/image/pull_policy: never 基础上追加 `command: ["true"]` 与 `profiles: [build-only]`
       - 这样源码构建路径（`docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build`）依旧只构建不上线
       - 最终块形如：
         ```yaml
         sing-box-gateway:
           build:
             context: .
             dockerfile: deploy/docker/sing-box-gateway/Dockerfile
           image: ghcr.io/zanel1u/cloud-cli-proxy/sing-box-gateway:latest
           pull_policy: never
           command: ["true"]
           profiles:
             - build-only
         ```
    3. 不要改动其它服务（postgres / control-plane / admin / managed-user-image）

    背景与不变量（用于 self-check）：
    - 默认 `docker compose pull` 会拉取所有未声明 profile 的服务 → sing-box-gateway 必须从 build-only 中移出
    - 默认 `docker compose up -d` 会启动所有未声明 profile 的服务 → sing-box-gateway 必须用 `command: ["true"]` 一次性退出 0，并用 `restart: "no"` 阻止默认 restart policy 让它无限重启
    - 源码构建仍走 `--profile build-only build` 路径 → build.yaml 必须保留 build/profile 配置；`command: ["true"]` 在 build.yaml 也加上以便保持两文件 merge 后语义一致
    - 不要把 `restart: "no"` 写成 `restart: no`（不带引号会被 YAML 解析为 false bool，docker compose 会拒绝）
  </action>
  <verify>
    <automated>docker compose config --services 2>/dev/null | sort | tr '\n' ',' && echo "---BUILD---" && docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only config --services 2>/dev/null | sort | tr '\n' ','</automated>
  </verify>
  <done>
    - 第一条 `docker compose config --services` 输出包含 `sing-box-gateway`（且包含 admin/control-plane/postgres，不含 managed-user-image）
    - 第二条 build-only 合并配置输出包含 `managed-user-image` 与 `sing-box-gateway`（profile 激活后两者均出现）
    - `docker compose config` 不报 YAML/schema 错误（即命令退出码 0）
    - `docker-compose.yml` diff 仅作用于 `sing-box-gateway` 块；`docker-compose.build.yaml` diff 仅作用于 `sing-box-gateway` 块
  </done>
</task>

</tasks>

<verification>
整体回归检查：

1. `go vet ./...` 与 `go build ./...` 通过（兜底 Task 1 没有破坏其它包编译）
2. `docker compose config -q` 通过（兜底 Task 2 没有引入语法/类型错）
3. `git diff --stat` 仅命中三个目标文件 + 可能的新增/修改测试文件，无意外文件被改
4. grep 兜底确认 `admin_egress_ip_probe.go` 内不再有 `os.Hostname()` 拼 `"container:" + ...` 的旧模式（用 `grep -n 'container:' internal/controlplane/http/admin_egress_ip_probe.go`，命中行数应 ≤ 1 且仅出现在 helper 内部）

UAT（标记为人工验证，不阻塞 plan 完成）：
- macOS 直跑：`go run ./cmd/control-plane`，控制台访问后台触发 IP 测试 → 不再报 `No such container: <hostname>`，能拿到 partial/passed/failed 结果（取决于代理是否真的能用）
- 单宿主机部署：执行 `docker compose pull --policy always` 后 `docker images | grep sing-box-gateway` 命中 latest tag；`docker compose up -d` 后 `docker ps -a --filter name=sing-box-gateway` 应显示 Exited (0)，不在 restarting/running 状态
- 源码构建：`docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build sing-box-gateway` 仍可成功
</verification>

<success_criteria>
- macOS 上 `go run ./cmd/control-plane` 后调用出口 IP 测试不再返回 `No such container` 错误，host-binary 分支被命中
- Linux 容器内 hostname 等于容器 ID 时仍走 in-container 分支，行为与修复前一致
- `docker compose pull` 默认能把 sing-box-gateway 镜像拉到本地
- `docker compose up -d` 启动时 sing-box-gateway 退出 0 一次后保持 stopped，不进入 restart 循环
- 源码构建路径未受影响：build-only profile 仍能 build sing-box-gateway 镜像
- 所有自动化命令（go vet、go build、go test、docker compose config）退出码 0
</success_criteria>

<output>
完成后，创建 `.planning/quick/260504-dtd-ip-macos-no-such-container-docker-compos/260504-dtd-SUMMARY.md`，需要包含：
- 探针网络模式决策的最终代码位置与函数签名
- compose 文件 diff 摘要（哪几行 added/removed）
- 自动化命令实际输出片段（go vet / go build / docker compose config）
- 待用户人工 UAT 的三条验证步骤（macOS 直跑、生产部署 pull+up、源码构建）
- 是否触发新增测试文件（若有，列文件名 + 测试函数清单）
</output>
