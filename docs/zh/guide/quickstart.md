# 快速开始

## Docker Compose 部署（推荐）

### 前置要求

- Linux 宿主机（Ubuntu 22.04+ / Debian 12+）
- Docker Engine 28+，Docker Compose v2
- 至少一个出口 IP（WireGuard 配置或代理服务器）

### 界面预览

> 下图均来自仓库 `imgs/`，帮助你在部署前先直观看到后台和用户侧体验。

#### 仪表板总览

![仪表板总览](/imgs/1.png)

#### 主机管理列表

![主机管理列表](/imgs/2.png)

#### 主机详情与连接入口

![主机详情与连接入口](/imgs/4.png)

#### 生命周期与网络操作

![生命周期与网络操作](/imgs/5.png)

#### 浏览器远程桌面（KasmVNC）

![浏览器远程桌面](/imgs/3.png)

### 第 1 步：克隆代码

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy
```

### 第 2 步：生成环境配置

运行初始化脚本，自动生成所有密码和密钥：

```bash
bash deploy/scripts/setup-env.sh
```

脚本会让你选择数据库方案：

- **内置 Docker PostgreSQL（推荐）**：自动生成数据库密码，Docker Compose 一起管理，零配置。
- **外部 PostgreSQL**：交互式填入外部数据库的地址、端口、用户名、密码，支持 SSL。

两种方案都会自动生成管理员密码（20 位）和 JWT 密钥（48 位）。

::: warning 重要
脚本执行完毕后会显示管理员密码，请立即保存，此处仅显示一次！
:::

### 第 3 步：启动服务

默认推荐：**优先使用预构建镜像**（`latest`），启动更快，且与 CI 发布保持一致。

```bash
# 内置 Docker PostgreSQL
docker compose pull --policy always
docker compose up -d

# 外部 PostgreSQL（跳过内置数据库）
docker compose pull --policy always control-plane admin
docker compose up -d control-plane admin
```

本地源码构建（可选）：

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

### 第 4 步：验证

```bash
curl http://127.0.0.1:8080/healthz
# {"status":"ok","checks":{"database":"ok","agent":"ok"}}
```

服务地址：
- **API**：`http://YOUR_HOST:8080`
- **管理后台**：`http://YOUR_HOST:3000`
- **SSH 代理**：`YOUR_HOST:2222`

## 给用户开机器

完整流程分 5 步：**登录 → 添加出口 IP → 创建用户 → 创建主机并绑定 → 发给用户连接命令**。

### 1. 获取管理员 Token

可以通过管理后台登录，也可以通过 API：

```bash
TOKEN=$(curl -s -X POST http://YOUR_HOST:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"你的管理员密码"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
```

### 2. 添加出口 IP

支持两种隧道类型：

**WireGuard 类型（全隧道 VPN）：**

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "hk-wg-01",
    "ip_address": "203.0.113.10",
    "tunnel_type": "wireguard",
    "provider": "manual",
    "wg_endpoint": "vpn-provider.example.com:51820",
    "wg_public_key": "对端公钥Base64",
    "wg_allowed_ips": "0.0.0.0/0",
    "wg_peer_address": "10.0.0.2/32"
  }'
```

**Proxy 类型（代理协议）：**

支持 5 种协议 — SOCKS5、VMess、Shadowsocks、Trojan、HTTP。

```bash
# Shadowsocks 示例
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "jp-ss-01",
    "ip_address": "198.51.100.5",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "shadowsocks",
      "server": "198.51.100.5",
      "server_port": 8388,
      "method": "aes-256-gcm",
      "password": "your-ss-password"
    }
  }'
```

```bash
# SOCKS5 示例
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "us-socks-01",
    "ip_address": "192.0.2.50",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "socks",
      "server": "192.0.2.50",
      "server_port": 1080,
      "username": "user",
      "password": "pass"
    }
  }'
```

**测试出口 IP 连通性：**

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID}/test \
  -H "Authorization: Bearer $TOKEN"
```

测试结果包括连通性、出口 IP 匹配和 DNS 泄漏三项检测。

### 3. 创建用户

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "zhangsan",
    "password": "给用户的初始密码",
    "expires_at": "2026-12-31T23:59:59Z"
  }'
```

### 4. 创建主机并绑定出口 IP

**创建主机：**

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "用户UUID"}'
```

**绑定出口 IP：**

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/bindings \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"host_id": "主机UUID", "egress_ip_id": "出口IP的UUID"}'
```

::: tip
一个主机至少需要绑定一个出口 IP 才能正常启动。
:::

### 5. 发给用户

主机创建后，在管理后台的主机详情页可以复制用户的接入命令。或者直接把以下命令发给用户（替换 `SHORT_ID` 为主机短 ID）：

```bash
curl -sSf http://YOUR_HOST/entry/SHORT_ID | bash
```

也可以使用 bootstrap 方式（需要用户输入用户名）：

```bash
curl -sSf http://YOUR_HOST:8080/v1/bootstrap/script | bash
```

## 用户使用方式

> 可以直接把这一段发给你的用户。

### 接入云主机

在终端执行管理员给你的命令：

```bash
curl -sSf http://YOUR_HOST/entry/abc123 | bash
```

输入密码后，几秒即可进入专属云主机。

### 预装工具

容器内已预装以下工具：

| 工具 | 说明 |
|------|------|
| **Claude Code** | AI 编程助手，直接在终端运行 `claude` 即可使用 |
| **KasmVNC + Chromium** | 浏览器远程桌面，通过管理后台或用户面板访问 |
| **Git** | 版本控制 |
| **tmux** | 终端复用，断线不丢失会话 |
| **zsh** | 增强 Shell 体验 |
| **Node.js** | JavaScript 运行时 |

### 使用 Claude Code

进入云主机后，直接运行：

```bash
claude
```

所有 Claude API 请求自动通过管理员指定的出口 IP 路由，无需任何额外代理配置。

### 断线重连

如果 SSH 连接中断，重新执行同一条 `curl` 命令即可恢复。容器不会因为断开连接而停止。

### 重建主机

如果需要重置环境，可以在用户面板中点击"重建"按钮。重建会重新创建容器，但 home 目录数据会保留。

## 在本机进行源码开发（从 clone 开始）

如果你想参与二次开发，推荐按下面流程在本机启动开发环境。

### 1. 准备依赖

- Git
- Go `1.25.7+`
- Node.js `20+`（建议启用 `corepack`）
- pnpm `10+`
- Docker Engine + Docker Compose v2
- GNU Make

### 2. 克隆仓库

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy
```

### 3. 初始化依赖与环境文件

```bash
make setup
```

该命令会安装前端依赖，并在缺失 `.env` 时从 `.env.example` 自动生成。

### 4. 启动本地数据库

```bash
make db
```

默认连接本地 PostgreSQL（`127.0.0.1:5433`）。

### 5. 启动开发模式

```bash
make dev
```

启动后可访问：

- Admin 前端：`http://localhost:5173`
- Control Plane API：`http://127.0.0.1:8090`

### 6. 基础验证与测试

```bash
curl http://127.0.0.1:8090/healthz
make test
```

### 常用命令

```bash
make dev-api   # 仅启动后端
make dev-web   # 仅启动前端
make db-stop   # 停止数据库
make db-reset  # 重建数据库
make help      # 查看全部命令
```

## 下一步

- [部署指南](./deployment) — systemd 原生部署
- [配置参考](./configuration) — 环境变量和网络配置
- [API 参考](../reference/api) — 完整的 Admin API 文档
- [故障排查](../reference/faq) — 常见问题和灾难恢复
