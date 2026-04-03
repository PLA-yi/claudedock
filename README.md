<div align="center">

<img src="docs/public/logo.svg" width="88" height="88" alt="Cloud CLI Proxy" />

# Cloud CLI Proxy

**一条命令，一台云主机，所有流量走指定出口。**

为 Claude Code 和开发团队提供开箱即用的隔离云主机环境，预装 AI 编程工具，全流量强制走指定出口 IP，零泄漏。

[![CI](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/ci.yml)
[![Images](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/build-images.yml/badge.svg)](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/build-images.yml)
[![Release](https://img.shields.io/github/v/release/ZaneL1u/cloud-cli-proxy)](https://github.com/ZaneL1u/cloud-cli-proxy/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.en.md) | [Documentation](https://zanel1u.github.io/cloud-cli-proxy/)

**Go · React · PostgreSQL · Docker · WireGuard**

</div>

---

## 功能特性

- **一条命令接入** — `curl | bash` 自动认证、创建容器、SSH 接入，用户无需任何配置
- **Claude Code 开箱即用** — 容器预装 Claude Code，进入即可使用，所有 API 请求自动走指定出口
- **全流量强制出口** — WireGuard + Linux netns / sing-box tun 双通道，nftables 默认拒绝策略，杜绝 DNS / WebRTC 泄漏
- **多协议支持** — 出口 IP 支持 WireGuard 和 5 种代理协议（SOCKS5 / VMess / Shadowsocks / Trojan / HTTP）
- **每用户隔离** — 独立 Docker 容器，预装 KasmVNC 远程桌面 + Chromium 浏览器
- **管理后台** — React SPA 仪表盘，用户、主机、出口 IP、事件日志一站式管理
- **用户自助面板** — 用户可查看主机状态、重建主机、访问 VNC 桌面
- **到期自动治理** — 过期自动停机、禁止登录
- **多架构 CI/CD** — GitHub Actions 自动构建 `linux/amd64` + `linux/arm64` 镜像

---

## 功能预览

### 仪表板

可快速查看活跃用户、运行主机、可用出口 IP 和最近事件。

![仪表板总览](imgs/1.png)

### 主机管理列表

集中查看每台主机状态、所属用户、绑定出口 IP、最近任务与操作入口。

![主机管理列表](imgs/2.png)

### 主机详情与接入方式

主机详情页可直接复制 `curl` 入口、SSH 命令和 VNC 登录入口。

![生命周期与网络操作](https://cdn.zaneliu.me/2026/04/4.png)

### 生命周期与网络操作

支持一站式完成出口 IP 绑定、重建、停机、密码轮换、VNC 打开等日常运维动作。

![主机详情与接入方式](https://cdn.zaneliu.me/2026/04/5.png)

### 浏览器远程桌面（KasmVNC）

无需本地安装 GUI，直接在浏览器中进入云主机桌面环境进行操作。

![浏览器远程桌面](imgs/3.png)

---

## 部署

### Docker Compose

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy

bash deploy/scripts/setup-env.sh

# 推荐：优先使用预构建镜像（latest）
docker compose pull --policy always
docker compose up -d

curl http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

`setup-env.sh` 交互式生成所有密码和密钥，支持内置 Docker PostgreSQL（零配置）或外部数据库。

启动后管理后台在 `http://YOUR_HOST:3000`，API 在 `:8080`。

本地源码构建（可选，作为预构建不可用时的兜底）：

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DATABASE_URL` | PostgreSQL 连接字符串（必填） | — |
| `ADMIN_USERNAME` | 管理员用户名 | `admin` |
| `ADMIN_PASSWORD` | 管理员密码（必填） | — |
| `ADMIN_JWT_SECRET` | JWT 签名密钥（必填） | — |
| `ADMIN_PORT` | 管理后台端口 | `3000` |
| `SSH_PROXY_PORT` | SSH 代理端口 | `2222` |
| `LOG_FORMAT` | 日志格式 `json` / `text` | `json` |
| `LOG_LEVEL` | 日志级别 | `info` |

---

## 使用

### 管理员设置

登录管理后台，依次完成：

1. **添加出口 IP** — 支持 WireGuard 配置或代理协议，可一键测试连通性
2. **创建用户** — 设置用户名、密码、到期时间
3. **创建主机** — 为用户创建容器并绑定出口 IP
4. **分发接入命令** — 在主机详情页复制 `curl` 命令发给用户

### 用户接入

用户在终端执行管理员提供的命令即可：

```bash
curl -sSf http://YOUR_HOST/entry/abc123 | bash
# 输入密码 → 等待启动 → 自动 SSH 进入云主机
```

### Claude Code

进入云主机后 Claude Code 已预装，直接使用：

```bash
claude
```

所有 Claude API 请求自动通过指定出口 IP 路由，无需额外配置代理。

### KasmVNC 远程桌面

容器内置 KasmVNC + Chromium，可通过管理后台或用户面板直接访问浏览器桌面环境。

---

## 架构

```
用户 ──curl──> Control Plane (:8080) ──Docker──> 用户容器 (SSH + Claude Code + VNC)
                    │                                  │
               PostgreSQL                        WireGuard / sing-box 隧道
                    │                                  │
              Admin SPA (:3000)                   指定出口 IP
                    │
              SSH Proxy (:2222)
```

| 组件 | 说明 |
|------|------|
| **Control Plane** | Go API，认证、用户管理、任务编排、SSH 代理 |
| **Host Agent** | 特权代理，管理 Docker 容器、网络命名空间和隧道 |
| **用户容器** | Ubuntu 24.04，预装 OpenSSH + Claude Code + KasmVNC + Chromium |
| **PostgreSQL** | 持久化用户、主机、出口 IP、任务和事件 |
| **Admin SPA** | React 19 + TypeScript + Vite + Tailwind CSS |

---

## 开发

### 从 clone 到本机启动（推荐流程）

#### 1. 准备依赖

- Git
- Go `1.25.7+`
- Node.js `20+`（建议启用 `corepack`）
- pnpm `10+`
- Docker Engine + Docker Compose v2
- GNU Make

#### 2. 克隆仓库

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy
```

#### 3. 初始化开发环境

```bash
make setup
```

`make setup` 会安装前端依赖，并在本地不存在 `.env` 时自动从 `.env.example` 复制一份。

#### 4. 启动数据库

```bash
make db
```

默认会拉起本地 PostgreSQL（端口 `5433`）。

#### 5. 启动后端 + 前端热重载

```bash
make dev
```

启动后可访问：

- Admin 前端：`http://localhost:5173`
- Control Plane API：`http://127.0.0.1:8090`

#### 6. 验证与测试

```bash
curl http://127.0.0.1:8090/healthz
make test
```

### 常用开发命令

```bash
make dev-api   # 仅启动后端
make dev-web   # 仅启动前端
make db-stop   # 停止本地 PostgreSQL
make db-reset  # 重建本地数据库
make help      # 查看所有命令
```

更多命令见 `make help`。

---

## 发布与 Changelog

推送 `v*` 标签会自动触发 `Release` 工作流，完成三件事：

- 先执行 CI 门禁（Go tests + Admin 前端构建）
- 创建 GitHub Release
- 触发多架构镜像发布（`semver` + `latest`）
- 按 monorepo 分组生成发布说明并回写 [CHANGELOG.md](CHANGELOG.md)

当前 changelog 默认按路径分组为：

- Backend（Go / API，`cmd` + `internal`）
- Frontend（`web/admin`）
- Runtime & Deployment（`deploy`、compose、workflow）
- Docs（`docs` + README）

手动发版示例：

```bash
make release VERSION=1.5.0
```

---

## 文档

完整文档见 [GitHub Pages](https://zanel1u.github.io/cloud-cli-proxy/)：

- [快速开始](https://zanel1u.github.io/cloud-cli-proxy/zh/guide/quickstart) — 部署和首次使用
- [部署指南](https://zanel1u.github.io/cloud-cli-proxy/zh/guide/deployment) — systemd 原生部署
- [配置参考](https://zanel1u.github.io/cloud-cli-proxy/zh/guide/configuration) — 环境变量和 WireGuard 配置
- [架构说明](https://zanel1u.github.io/cloud-cli-proxy/zh/guide/architecture) — 系统设计和项目结构
- [API 参考](https://zanel1u.github.io/cloud-cli-proxy/zh/reference/api) — 完整 Admin API
- [故障排查](https://zanel1u.github.io/cloud-cli-proxy/zh/reference/faq) — 常见问题和灾难恢复

---

## 许可证

[MIT](LICENSE)
