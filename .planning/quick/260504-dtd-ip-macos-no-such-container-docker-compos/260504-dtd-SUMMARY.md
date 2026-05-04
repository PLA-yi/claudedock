---
phase: quick-260504-dtd
plan: "01"
subsystem: control-plane / docker-compose
tags: [macos, egress-ip, probe, docker-compose, sing-box-gateway]
dependency_graph:
  requires: []
  provides: [QUICK-260504-DTD-01, QUICK-260504-DTD-02]
  affects: [internal/controlplane/http/admin_egress_ip_probe.go, docker-compose.yml, docker-compose.build.yaml]
tech_stack:
  added: []
  patterns: [包级 var 钩子注入（hostnameProvider / dockerInspectRunner）, TDD RED-GREEN]
key_files:
  created:
    - internal/controlplane/http/admin_egress_ip_probe_test.go
  modified:
    - internal/controlplane/http/admin_egress_ip_probe.go
    - docker-compose.yml
    - docker-compose.build.yaml
decisions: []
metrics:
  duration_min: 2
  completed_date: "2026-05-04"
---

# Phase quick-260504-dtd Plan 01: macOS 直跑探针修复 + sing-box-gateway 默认拉取

## 一句话总结

探针容器网络模式根据控制面运行环境自动决策（容器内复用 namespace / 宿主机直跑走 bridge+端口映射），同时 sing-box-gateway 镜像默认参与 `docker compose pull` 且 `up` 时不进入 restart 循环。

## 变更摘要

### Task 1: 探针容器网络模式自适应（in-container / host-binary）

**文件:** `internal/controlplane/http/admin_egress_ip_probe.go`

新增包级钩子与 helper：

- `var hostnameProvider = os.Hostname` — 抽象 hostname 获取，便于单测注入
- `var dockerInspectRunner = func(ctx, name) ([]byte, error) { ... }` — 真实实现使用 `docker inspect --format "{{.Id}}"` + 2s 超时
- `func resolveProbeNetworking(ctx context.Context, port int) []string` — 决策逻辑：
  - hostname 非空且 docker inspect 成功返回非空 ID → `[]string{"--network", "container:" + hostname}`
  - 其余情况 → `[]string{"--network", "bridge", "-p", "127.0.0.1:<port>:<port>"}`

`startSingBoxDocker` 中旧逻辑（L276-279 `networkArg := "host"` + `container:` 拼接）被替换为 `netArgs := resolveProbeNetworking(ctx, port)`，并通过 `append(args, netArgs...)` 嵌入 docker run 参数列表。

**测试文件:** `internal/controlplane/http/admin_egress_ip_probe_test.go`（新建，117 行）

| 测试函数 | 覆盖场景 |
|----------|----------|
| `TestResolveProbeNetworking_HostnameEmpty` | hostname 为空 → host-binary 分支 |
| `TestResolveProbeNetworking_InspectFails` | docker inspect 报错 → host-binary 分支 |
| `TestResolveProbeNetworking_InspectEmptyOutput` | docker inspect stdout 为空 → host-binary 分支 |
| `TestResolveProbeNetworking_InspectSucceeds` | docker inspect 成功 → in-container 分支 |

### Task 2: docker-compose 默认拉取 sing-box-gateway 且 up 不进入 restart 循环

**docker-compose.yml**（sing-box-gateway 块）：

```diff
   sing-box-gateway:
     image: ghcr.io/zanel1u/cloud-cli-proxy/sing-box-gateway:latest
     pull_policy: always
-    profiles:
-      - build-only
+    restart: "no"
+    command: ["true"]
```

- 移除 `profiles: [build-only]` → 默认 `docker compose pull` 会拉取该镜像
- 追加 `restart: "no"` + `command: ["true"]` → `up` 时一次性退出 0，不进入 restart 循环

**docker-compose.build.yaml**（sing-box-gateway 块）：

```diff
   sing-box-gateway:
     build:
       context: .
       dockerfile: deploy/docker/sing-box-gateway/Dockerfile
     image: ghcr.io/zanel1u/cloud-cli-proxy/sing-box-gateway:latest
     pull_policy: never
+    command: ["true"]
+    profiles:
+      - build-only
```

- 追加 `command: ["true"]` + `profiles: [build-only]`，与 `managed-user-image` 的 pull-stub 模式对齐
- 源码构建路径 `docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build` 继续可用

## 自动化命令输出

```
$ go vet ./...
exit=0

$ go build ./...
exit=0

$ go test ./internal/controlplane/http/... -run 'ResolveProbeNetworking|ProbeNetworking' -count=1
=== RUN   TestResolveProbeNetworking_HostnameEmpty
--- PASS: TestResolveProbeNetworking_HostnameEmpty (0.00s)
=== RUN   TestResolveProbeNetworking_InspectFails
--- PASS: TestResolveProbeNetworking_InspectFails (0.00s)
=== RUN   TestResolveProbeNetworking_InspectEmptyOutput
--- PASS: TestResolveProbeNetworking_InspectEmptyOutput (0.00s)
=== RUN   TestResolveProbeNetworking_InspectSucceeds
--- PASS: TestResolveProbeNetworking_InspectSucceeds (0.00s)
PASS
ok      github.com/zanel1u/cloud-cli-proxy/internal/controlplane/http   0.011s

$ docker compose config --services | sort
admin
control-plane
postgres
sing-box-gateway

$ docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only config --services | sort
admin
control-plane
managed-user-image
postgres
sing-box-gateway

$ docker compose config -q
exit=0
```

## 人工 UAT（待用户验证）

1. **macOS 直跑：** `go run ./cmd/control-plane`，打开后台触发「出口 IP 测试」→ 不应再报 `No such container: <hostname>`，应能拿到 partial/passed/failed 结果（取决于代理是否可用）。
2. **单宿主机部署：** 执行 `docker compose pull --policy always` 后 `docker images | grep sing-box-gateway` 应命中 latest tag；执行 `docker compose up -d` 后 `docker ps -a --filter name=sing-box-gateway` 应显示 `Exited (0)`，不在 restarting/running 状态。
3. **源码构建：** `docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build sing-box-gateway` 仍可成功构建镜像。

## Deviations from Plan

无 — plan 执行完全按预期，无偏差。

## Commits

| # | Type | Message | Hash |
|---|------|---------|------|
| 1 | test | test(quick-260504-dtd-01): add failing tests for resolveProbeNetworking | de904f7 |
| 2 | feat | feat(quick-260504-dtd-01): add resolveProbeNetworking helper for host-binary mode | 07c4a36 |
| 3 | feat | feat(quick-260504-dtd-02): default-pull sing-box-gateway and avoid restart loop | 97d59a6 |

## Self-Check: PASSED

- [x] `internal/controlplane/http/admin_egress_ip_probe_test.go` 存在
- [x] `resolveProbeNetworking` helper 在 `admin_egress_ip_probe.go` 中定义
- [x] 旧 `container:` + hostname 裸拼模式已移除（grep 命中 ≤1 且仅在 helper 内部）
- [x] `docker-compose.yml` sing-box-gateway 块含 `restart: "no"` 与 `command: ["true"]`
- [x] `docker-compose.build.yaml` sing-box-gateway 块含 `command: ["true"]` 与 `profiles: [build-only]`
- [x] 全部 commit 存在于 git 历史（de904f7 / 07c4a36 / 97d59a6）
