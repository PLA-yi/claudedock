# ClaudeDock — Deploy 运维手册

本目录收纳宿主侧部署与运维脚本、配置样例与故障处置指引。

## v3.0 AppArmor override 部署

### 适用范围

仅 **Ubuntu 25.04 及以上**宿主机需要执行本节；Ubuntu 24.04、Debian、CentOS、RHEL 等可跳过。

### 背景

Ubuntu 25.04 起默认加载针对 `fusermount3` 的 AppArmor profile，可能拒绝 `capability dac_override`。受管镜像 v3.0 在容器内以 root 挂载 mergerfs 时需要该能力。若未在宿主追加本地 override，容器内可能出现 `permission denied`，entrypoint 会 fail-fast。

### 修复步骤

在宿主执行（需要 root 权限）：

```bash
sudo tee /etc/apparmor.d/local/fusermount3 >/dev/null <<'APPARMOR'
# ClaudeDock v3.0 — allow mergerfs DAC override for multi-branch readdir
capability dac_override,
APPARMOR

sudo apparmor_parser -r /etc/apparmor.d/fusermount3
```

修复后建议再执行 `deploy/scripts/host-preflight.sh` 验收，直到 AppArmor fusermount3 检测项输出 OK。

### 自动检测

运行 `deploy/scripts/host-preflight.sh` 会调用 **`check_apparmor_fusermount3`**：

- Ubuntu 25.04+ 且 override 缺失 → 输出 FAIL + 修复指引（**不**自动写文件或执行 `apparmor_parser`）。
- Ubuntu 24.04 / 非 Ubuntu → 自动跳过。
- AppArmor 未启用或 `fusermount3` profile 未加载 → 自动跳过。

调用处使用 `check_apparmor_fusermount3 || true`，检测失败**不会**阻断后续的 `mkdir -p /var/lib/claudedock` 等核心初始化。

> 注：v1 不做自动修复；`AUTO_FIX=1` 由后续阶段引入。

### 参考

- 覆盖路径必须为 **`/etc/apparmor.d/local/fusermount3`**（不是 `docker-default`）。

## 部署脚本

### setup-env.sh — 环境初始化

`deploy/scripts/setup-env.sh` 交互式生成所有密码和密钥，支持：

- SQLite 单文件数据库（零配置，WAL 模式，`/data` 持久化）

两种方案都会自动生成管理员密码（20 位）和 JWT 密钥（48 位）。脚本执行完毕后会显示管理员密码，请立即保存，仅显示一次。

### host-preflight.sh — 宿主机预检

`deploy/scripts/host-preflight.sh` 在部署前检查宿主机环境，包括：

- Docker Engine 版本
- 内核模块（tun、nftables 等）
- AppArmor fusermount3 override（Ubuntu 25.04+）
- 磁盘空间

建议在首次部署前运行，确保环境满足要求。

## v3.1+ 宿主机路径挂载

创建主机时支持配置宿主机路径挂载（Host Bind Mounts），将宿主机的指定目录映射到容器内。挂载路径在创建主机时通过管理后台或 API 配置，支持目录联想功能。

注意事项：

- 挂载路径必须为绝对路径
- 宿主机路径需要存在且可读
- 容器内挂载点为 `/mnt/host/{name}`
