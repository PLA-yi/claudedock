# 部署指南

推荐使用 Docker Compose 一键部署，无需手动安装 PostgreSQL、Go 或编译源码。

## 1. 安装 Docker

### Linux

```bash
curl -fsSL https://get.docker.com | sh
```

安装后确保当前用户在 `docker` 组：

```bash
sudo usermod -aG docker $USER
# 退出重新登录生效
```

### macOS

安装 [Docker Desktop](https://www.docker.com/products/docker-desktop/)，或通过 Homebrew：

```bash
brew install --cask docker
```

### Windows

安装 [Docker Desktop](https://www.docker.com/products/docker-desktop/)，启用 WSL 2 后端。

## 2. 启动

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy

bash deploy/scripts/setup-env.sh
docker compose pull
docker compose up -d
```

`setup-env.sh` 交互式生成所有密码和密钥。支持：

- **内置 PostgreSQL（推荐）**：自动创建，Docker Compose 统一管理，零配置
- **外部 PostgreSQL**：填入已有数据库的连接信息

启动后：

- 管理后台：`http://YOUR_HOST:3000`
- API：`http://YOUR_HOST:8080`

验证：

```bash
curl http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

## 中国大陆用户

### 方案一：使用 docker-compose.cn.yml（推荐）

项目提供了 `docker-compose.cn.yml`，已将 compose 镜像替换为 `ghcr.1ms.run`（毫秒镜像），同时设置了 `CONTAINER_REGISTRY` 环境变量，确保运行时动态拉取的镜像（`managed-user`、`sing-box` 探针）也走同一镜像源。直接通过覆盖文件启动：

```bash
docker compose -f docker-compose.yml -f docker-compose.cn.yml pull
docker compose -f docker-compose.yml -f docker-compose.cn.yml up -d
```

systemd 裸机部署的用户，在 `/etc/cloud-cli-proxy/env` 中加上：

```bash
CONTAINER_REGISTRY=ghcr.1ms.run
```

重启控制面后生效。所有 `docker pull` 操作都会自动走镜像源。

源码构建（兜底）：

```bash
docker compose -f docker-compose.yml -f docker-compose.cn.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.cn.yml -f docker-compose.build.yaml up -d --force-recreate
```

### 方案二：走本地代理拉取

本机已开 TUN 模式或全局代理，镜像走代理出站，直接用默认 compose 文件即可：

```bash
docker compose pull
docker compose up -d
```

## 环境变量

使用 `setup-env.sh` 生成后通常不需要手动修改。完整列表见 [配置参考](./configuration)。

## 源码构建

预构建镜像不可用时才需要本地构建：

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

## 宿主机直接部署

对于需要裸机 systemd 部署的场景，提供自动化脚本：

```bash
sudo bash deploy/scripts/deploy.sh
```

会自动完成：创建系统用户 → 构建二进制和镜像 → 生成配置 → 安装 systemd 服务 → 启动。详细步骤见仓库内的 `deploy/scripts/deploy.sh`。
