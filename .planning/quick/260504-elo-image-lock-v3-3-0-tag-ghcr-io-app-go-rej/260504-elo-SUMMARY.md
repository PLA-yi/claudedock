# Quick Task 260504-elo Summary

**Date:** 2026-05-04
**Commits:** d1cbdf9, 5596353

## Task 1: image.lock tag 改为 latest

**File:** [deploy/docker/managed-user/image.lock](deploy/docker/managed-user/image.lock)

ghcr.io 上不存在 `v3.3.0` tag，启动时 `docker pull` 报 `manifest unknown`。
将 `image_name` 和 `local_dev_image_name` 都从 `v3.3.0` 改为 `latest`。

```diff
-image_name: ghcr.io/zanel1u/cloud-cli-proxy/managed-user:v3.3.0
-local_dev_image_name: ghcr.io/zanel1u/cloud-cli-proxy/managed-user:v3.3.0
+image_name: ghcr.io/zanel1u/cloud-cli-proxy/managed-user:latest
+local_dev_image_name: ghcr.io/zanel1u/cloud-cli-proxy/managed-user:latest
```

`image_version: v3.3.0` 保持不动（语义版本号，与镜像 tag 独立）。

## Task 2: rejoinHostNetworks 加容器存在性探测

**File:** [internal/controlplane/app/app.go](internal/controlplane/app/app.go)

`rejoinHostNetworks()` 假设 `os.Hostname()` 等于 docker 容器名。在 macOS 宿主机直跑时 hostname 是 `Vision-2.local`，不是容器名，逐网络连接都报 `No such container` WARN。

修复：在 hostname 检测后、list networks 前，插入 `docker inspect --format {{.Id}} <hostname>`（2s 超时）探测。如果 hostname 不是真实容器，直接静默 return。

```go
// 探测控制面是否跑在 docker 容器内（hostname = 容器名）。
// 非容器环境（如 macOS 宿主机直跑）直接跳过，避免 "No such container" 误报。
inspectCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
if out, err := exec.CommandContext(inspectCtx, "docker", "inspect", "--format", "{{.Id}}", cpID).Output(); err != nil || len(strings.TrimSpace(string(out))) == 0 {
    cancel()
    return
}
cancel()
```

## 验证结果

- `go vet ./internal/controlplane/app/...` PASS
- `go build ./...` PASS
- `docker compose config --services` 未受影响（仅改 image.lock 字符串）

## UAT（人工验证）

1. `make dev` 启动时不再出现 `image cache refresh failed: manifest unknown`
2. `make dev` 启动时不再出现 `rejoin-networks: connect failed: No such container: Vision-2.local`
