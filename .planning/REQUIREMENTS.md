# Requirements: claude-shell

**Defined:** 2026-04-09
**Core Value:** 单一二进制替换 claude 命令，透明启动 Docker 容器运行 Claude Code，所有网络流量走代理出口，设备指纹完全伪装，用户和 Claude Code 均无感知。

## v1.3 Requirements

### 容器基础设施

- [ ] **INFRA-01**: 精简 Docker 镜像，通过官方安装脚本安装 Claude Code（Bun standalone），包含 sing-box 和基础开发工具
- [ ] **INFRA-02**: entrypoint 按正确顺序编排：网络配置 → 指纹伪造 → 反检测 → 启动 Claude Code
- [ ] **INFRA-03**: 容器内 Claude Code 自动更新被禁用（DISABLE_AUTOUPDATER）

### 网络隔离与代理

- [ ] **NET-01**: sing-box tun 模式接管容器内所有出站流量，外网流量走配置的代理出口
- [ ] **NET-02**: nftables 默认拒绝策略，仅允许 tun 和本地网络接口的流量
- [ ] **NET-03**: DNS 查询走代理（外网域名），防止 DNS 泄漏
- [ ] **NET-04**: 本地流量（127.0.0.1、10.0.0.0/8、172.16.0.0/12、192.168.0.0/16）通过 host-gateway 回连宿主机
- [ ] **NET-05**: 支持多种代理协议（SOCKS5、HTTP、VMess、Shadowsocks、Trojan）

### 设备指纹伪造

- [ ] **SPOOF-01**: /etc/machine-id 在 entrypoint 中写入基于配置派生的伪造值
- [ ] **SPOOF-02**: 容器 hostname 通过 Docker --hostname 设为配置的伪造值
- [ ] **SPOOF-03**: /proc/cpuinfo 和 /proc/meminfo 通过 docker run -v 注入伪造文件
- [ ] **SPOOF-04**: 反容器检测：删除 /.dockerenv、伪造 /proc/1/cgroup、清除 container 环境变量

### CLI 包装器

- [ ] **CLI-01**: Go 二进制作为 claude 命令，无子命令时透传所有参数给容器内 Claude Code
- [ ] **CLI-02**: docker run 启动容器，bind mount 当前目录到 /workspace，TTY 透传 + 信号转发
- [ ] **CLI-03**: init 子命令生成 ~/.claude-shell/config.yaml 配置文件
- [ ] **CLI-04**: verify 子命令在容器内运行检测脚本，验证出口 IP、DNS、指纹和容器标记
- [ ] **CLI-05**: 自动检测 Docker 可用性，镜像不存在时自动拉取
- [ ] **CLI-06**: YAML 配置支持代理设置、指纹参数和网络选项

### 构建与交付

- [ ] **BUILD-01**: garble 混淆构建，交付单一 Go 二进制
- [ ] **BUILD-02**: 项目位于 claude-shell/ 子目录，独立 go.mod，与 cloud-cli-proxy 零依赖

## v2 Requirements

### 进程级注入（无 Docker 高级模式）

- **DYLIB-01**: macOS DYLD_INSERT_LIBRARIES + codesign 重签注入
- **DYLIB-02**: Linux LD_PRELOAD 注入
- **DYLIB-03**: connect$NOCANCEL 等 Bun 特有 syscall 变体 hook
- **DYLIB-04**: PATH 劫持脚本（hostname / uname / ioreg 等）

### 跨平台

- **PLAT-01**: Windows DLL 注入或 IAT hook 支持
- **PLAT-02**: 多架构 Docker 镜像（amd64 + arm64）

## Out of Scope

| Feature | Reason |
|---------|--------|
| 计费与套餐管理 | 不属于 claude-shell 范畴，属于 cloud-cli-proxy 主产品 |
| Web Terminal / 浏览器远程桌面 | claude-shell 是终端工具，不需要 Web 界面 |
| 多用户管理 | claude-shell 面向单个开发者，不需要用户管理系统 |
| 100% 反容器检测保证 | 技术上无法承诺绝对不可检测，作为工程最佳努力而非硬承诺 |
| Claude Code 自定义版本管理 | 使用官方安装的最新版本，不做版本锁定 |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| INFRA-01 | — | Pending |
| INFRA-02 | — | Pending |
| INFRA-03 | — | Pending |
| NET-01 | — | Pending |
| NET-02 | — | Pending |
| NET-03 | — | Pending |
| NET-04 | — | Pending |
| NET-05 | — | Pending |
| SPOOF-01 | — | Pending |
| SPOOF-02 | — | Pending |
| SPOOF-03 | — | Pending |
| SPOOF-04 | — | Pending |
| CLI-01 | — | Pending |
| CLI-02 | — | Pending |
| CLI-03 | — | Pending |
| CLI-04 | — | Pending |
| CLI-05 | — | Pending |
| CLI-06 | — | Pending |
| BUILD-01 | — | Pending |
| BUILD-02 | — | Pending |

**Coverage:**
- v1.3 requirements: 20 total
- Mapped to phases: 0
- Unmapped: 20 ⚠️

---
*Requirements defined: 2026-04-09*
*Last updated: 2026-04-09 after initial definition*
