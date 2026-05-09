# 快速开始

## Docker Compose 部署（推荐）

### 前置要求

- Linux 宿主机（Ubuntu 22.04+ / Debian 12+）
- Docker Engine 28+，Docker Compose v2
- 至少一个出口 IP（代理服务器）

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
docker compose pull
docker compose up -d

# 外部 PostgreSQL（跳过内置数据库）
docker compose pull control-plane admin
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

出口 IP 使用 sing-box tun 全隧道，`tunnel_type` 为 `proxy`，在 `proxy_config` 中配置上游代理（sing-box outbound）。

支持 6 种协议 — SOCKS5、VMess、VLESS、Shadowsocks、Trojan、HTTP。

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

主机创建并绑定出口 IP、任务显示容器**已就绪**后，在管理后台「主机详情」页可复制接入信息。

**方式 A：浏览器 / 一条命令 SSH（传统）**

把下面命令发给用户（将 `YOUR_HOST` 换成对外网关域名或 IP，将 `SHORT_ID` 换成主机详情里的**主机短 ID**）：

```bash
curl -sSf http://YOUR_HOST/entry/SHORT_ID | bash
```

也可以使用 bootstrap 方式（需要用户输入用户名）：

```bash
curl -sSf http://YOUR_HOST:8080/v1/bootstrap/script | bash
```

**方式 B：cloud-claude（推荐，本地跑 Claude Code）**

除上述 `curl` 外，请把下面三项一并发给用户（与后台显示一致即可）：

| 信息 | 说明 |
|------|------|
| **网关地址** | 控制面对外的 HTTPS 地址，例如 `https://gw.example.com`（与浏览器打开管理后台同源；一般**不是**开发时的 `:3000` 前端端口） |
| **Short ID** | 主机详情页的**主机短 ID**。若用户在配置里填**用户短 ID**，则连到该用户的主主机 |
| **密码** | 该用户在后台的登录密码 |

用户在本机安装 `cloud-claude`、执行一次 `init` 填入上表三项后，在**希望开发的工程目录**下运行 `cloud-claude` 即可；当前目录会通过 sshfs 挂到容器内**同名路径**，Claude Code 里看到的路径与本地一致。默认会把 `git` 代理到本机执行（可用 `proxy_commands` 配置）。

## 用户使用方式

> 可以直接把这一段发给你的用户。

### 方式一：cloud-claude 本地 CLI（推荐）

#### 安装

**Homebrew（macOS / Linux，推荐）：**

```bash
brew tap ZaneL1u/tap
brew install cloud-claude
```

**一行脚本（任意平台）：**

```bash
curl -fsSL https://raw.githubusercontent.com/ZaneL1u/cloud-cli-proxy/main/scripts/install.sh | bash
```

也可从 [Releases](https://github.com/ZaneL1u/cloud-cli-proxy/releases) 下载对应平台的压缩包，或自行 `go build ./cmd/cloud-claude`。

#### 首次配置（只需一次）

```bash
cloud-claude init
```

按提示输入：**网关地址**、**Short ID**（主机或用户短 ID）、**密码** → 写入 `~/.cloud-claude/config.yaml`。

或使用 flag / 环境变量：

```bash
cloud-claude init --gateway https://gw.example.com --short-id abc123 --password your-password

export CLOUD_CLAUDE_GATEWAY=https://gw.example.com
export CLOUD_CLAUDE_SHORT_ID=abc123
export CLOUD_CLAUDE_PASSWORD=your-password
cloud-claude init
```

#### 连接并进入 Claude Code

```bash
cd ~/你的项目根目录    # 希望映射进容器的目录

alias claude=cloud-claude   # 可选，与本地 claude 习惯一致

cloud-claude                # 或 claude
cloud-claude -p "帮我重构这个函数"
```

**会话管理：** 默认 attach 同一账号的已有 tmux 会话，断线不丢失工作区：

```bash
cloud-claude                  # 默认：attach 已有会话（多端共享）
cloud-claude --new-session    # 强制新建独立会话
cloud-claude --take-over      # 接管主会话并踢掉其他客户端

cloud-claude sessions                  # 列出当前 tmux 会话
cloud-claude sessions --attach 0       # 接管指定会话
```

**映射模式：** 默认 Auto 模式自动选择最优挂载策略，也可手动指定：

```bash
cloud-claude --mount-mode=auto         # 默认：优先 HotSync，失败降级 SSHFS
cloud-claude --mount-mode=full         # HotSync + SSHFS 双轨（完整功能）
cloud-claude --mount-mode=sshfs-only   # 纯 SSHFS（兼容性优先）
```

**自检与排障：**

```bash
cloud-claude doctor                    # 五维度全面自检（network / auth / ssh / mount / disk）
cloud-claude doctor mount --fix        # 仅检查挂载维度，并自动修复常见故障
cloud-claude explain MOUNT_SSHFS_DISCONNECTED   # 查询错误码详细说明与修复建议
cloud-claude env check                 # 检查远端容器时区、语言、出口 IP、FUSE 等
```

**环境变量：**

- `CLOUD_CLAUDE_NO_PROMOTION=1` — 禁用冷文件读触发晋升（Linux 默认启用，macOS 自动跳过）
- 在 `~/.cloud-claude/config.yaml` 中设置 `proxy_commands`（字符串列表），指定在**本机**执行的命令名；默认仅 `git`；设为空数组 `[]` 可关闭代理。
- `hot_sync_max_file_mb` — 单文件熔断阈值（默认 50MB），超过此大小的文件走 cold 路径。

运行时 `cloud-claude` 会：1）向网关认证；2）等待容器就绪；3）sshfs 将当前目录挂到容器内**同名路径**；4）在远端启动 Claude Code。终端大小、信号、退出码会透传；网络抖动 30s 内自动重连，输入缓冲不丢失。

**错误提示：**

| 退出码 | 含义 | 排查方向 |
|--------|------|---------|
| 1 | 认证失败 | 检查 Short ID 和密码 |
| 2 | 网络错误 | 检查网关地址是否可达 |
| 3 | 超时 | 容器启动超时，联系管理员 |
| 4 | 配置错误 | 运行 `cloud-claude init` 重新配置 |

### 方式二：curl + SSH 接入

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

### 使用 Claude Code（SSH 方式）

进入云主机后，直接运行：

```bash
claude
```

所有 Claude API 请求自动通过管理员指定的出口 IP 路由，无需任何额外代理配置。

### 断线重连

如果 SSH 连接中断，重新执行同一条 `curl` 命令即可恢复。容器不会因为断开连接而停止。

### 重建主机

如果需要重置环境，可以在管理后台中点击"重建"按钮。重建会重新创建容器，但 home 目录数据会保留。

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

- Admin 前端：`http://localhost:2568`
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
