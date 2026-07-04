---
phase: quick-260504-elo
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - deploy/docker/managed-user/image.lock
  - internal/controlplane/app/app.go
---

<objective>
修复 `make dev` 启动时两个 WARN 级报错，它们导致 log 噪音且 image cache 刷新失败会影响后续创建主机。

1. **managed-user 镜像 tag 不存在**：image.lock 配置 `v3.3.0`，但 ghcr.io 上没有该 tag，启动时 `docker pull` 报 `manifest unknown`。
2. **rejoin-networks macOS 误报**：app.go `rejoinHostNetworks()` 假设 `os.Hostname()` 等于容器名，macOS 直跑时 hostname 是宿主机名（如 Vision-2.local），逐网络连接都报 `No such container`。
</objective>

<tasks>

<task type="auto">
  <name>Task 1: image.lock tag 改为 latest</name>
  <files>deploy/docker/managed-user/image.lock</files>
  <action>
    将 image_name 和 local_dev_image_name 都从 `v3.3.0` 改为 `latest`：
    ```
    image_name: ghcr.io/claudedock/claudedock/managed-user:latest
    local_dev_image_name: ghcr.io/claudedock/claudedock/managed-user:latest
    ```
  </action>
  <verify>
    <automated>grep 'managed-user:latest' deploy/docker/managed-user/image.lock | wc -l | grep -q '2'</automated>
  </verify>
  <done>
    - image.lock 中 image_name 和 local_dev_image_name 均指向 latest
    - image_version 字段保持 v3.3.0（语义版本号，与镜像 tag 独立）
  </done>
</task>

<task type="auto">
  <name>Task 2: rejoinHostNetworks 加容器存在性探测</name>
  <files>internal/controlplane/app/app.go</files>
  <action>
    在 `rejoinHostNetworks()` hostname 检测之后、list networks 之前，插入 docker inspect 探测：
    - 用 `docker inspect --format {{.Id}} <hostname>`（2s 超时）验证 hostname 是否对应真实容器
    - 失败或输出为空 → 控制面不在容器内，直接 return，不执行后续网络连接
    - 成功 → 继续原有逻辑，行为与修复前一致
    无需新增 import，复用已有的 `os/exec`、`context`、`time`、`strings`。
  </action>
  <verify>
    <automated>go vet ./internal/controlplane/app/... && go build ./...</automated>
  </verify>
  <done>
    - go vet / go build 通过
    - rejoinHostNetworks 在非容器环境下静默返回，不再逐个网络报 WARN
    - Linux 容器内环境行为不变（docker inspect 成功，继续原有网络重连逻辑）
  </done>
</task>

</tasks>

<success_criteria>
- `make dev` 启动时不再出现 `image cache refresh failed: manifest unknown`
- `make dev` 启动时不再出现 `rejoin-networks: connect failed: No such container: Vision-2.local`
- go vet ./... && go build ./... 通过
</success_criteria>
