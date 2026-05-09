# v3.0 错误码索引（v3.0+）

> 适用版本：v3.0 起；对应阶段 Phase 31-34（errcodes 注册表 + cloud-claude explain），v3.4 追加 Phase 38-44
> 关联需求：M13（禁止静默降级 → 每次降级必带 reason_code） / M14（doctor 输出含错误码） / SC#9（运维手册收口）

---

## 1. 背景

v3.0 起，所有用户可见的异常都通过 `internal/cloudclaude/errcodes` 注册表统一编排。命名规范（`codes.go::codeRe` L56 字面量）：

```
^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$
```

抽象成 **DOMAIN_KIND_NUM** 三段（多段允许尾部追加 `_NAME`）。共 8 个域前缀，由 `internal/cloudclaude/errcodes/{auth,disk,mount,net,session,ssh,state,system}.go` 中的 `init() MustRegister(...)` 各自登记。

每条错误码包含五元组（`Entry` 定义在 `codes.go::43-51`）：

- `Code`（字面量）
- `Severity` ∈ {`INFO`, `WARN`, `ERROR`, `FATAL`}
- `Message`（中文，可含 `%s/%d` 占位符）
- `NextAction`（中文，≤ 80 runes）
- 是否登记 `ExtendedExplanation`（在 `explanations.go::ExtendedExplanations` 中是否有长说明）

`Severity` 枚举源（`codes.go::20-25`）：`SeverityInfo / SeverityWarn / SeverityError / SeverityFatal`。

---

## 2. 生成方式

> 本表 Code/Severity/Message/NextAction 与
> `internal/cloudclaude/errcodes/{auth,disk,mount,net,session,ssh,state,system}.go` 中的
> `init() MustRegister(...)` 保持一一对应；Extended 列对应 `explanations.go` 的
> `ExtendedExplanations` 登记项（`✅` = 有长说明，`—` = 已豁免，列入 `ExplainExempt`）。
>
> 权威来源：`cloud-claude explain <code>` 子命令；若本表与 `explain` 输出不一致，以代码为准。

校准命令（CI / 手册维护时执行）：

```bash
grep -hE 'Code\s*=\s*"[A-Z]+_[A-Z]+' internal/cloudclaude/errcodes/*.go \
  | awk -F'"' '{print $2}' | sort -u | wc -l
# 期望 ≥ 51（v3.4 收尾时为 51）
```

反向一致性 diff（应输出空）：

```bash
comm -23 \
  <(grep -hE 'Code\s*=\s*"[A-Z]+_[A-Z]+' internal/cloudclaude/errcodes/*.go \
    | awk -F'"' '{print $2}' | sort -u) \
  <(grep -oE '[A-Z]+_[A-Z]+_[A-Z0-9_]+' docs/runbooks/v3-error-code-index.md \
    | grep -E '^(AUTH|DISK|MOUNT|NET|SESSION|SSH|STATE|SYSTEM)_' | sort -u)
```

---

## 3. 错误码全集（按域分组）

### 3.1 AUTH_*（CLI 配置 / Entry token / OAuth 刷新）

| Code | Severity | Message（摘要） | NextAction（摘要） | Extended |
|------|----------|-----------------|---------------------|----------|
| AUTH_CONFIG_MISSING       | FATAL | ~/.cloud-claude/config.yaml 不存在或解析失败           | 运行 cloud-claude init 重新配置网关与凭证                 | ✅ |
| AUTH_GATEWAY_UNREACHABLE  | ERROR | 网关 %s 不可达                                          | 检查网络与 gateway 配置，或运行 cloud-claude init          | ✅ |
| AUTH_TOKEN_EXPIRED        | WARN  | Entry API token 已过期或 401                            | 运行 cloud-claude doctor auth --fix 自动刷新              | ✅ |
| AUTH_OAUTH_REFRESH_FAILED | ERROR | Claude OAuth 刷新失败                                   | 在容器内运行 cloud-claude exec claude login 重新登录      | ✅ |

### 3.2 DISK_*（本地 / 容器 disk usage）

| Code | Severity | Message（摘要） | NextAction（摘要） | Extended |
|------|----------|-----------------|---------------------|----------|
| DISK_LOCAL_LOW          | WARN | 本地 ~/.cloud-claude/ 可用空间 < 500MB              | 清理 ~/.cloud-claude/mutagen/ 或释放本地磁盘                       | ✅ |
| DISK_CONTAINER_LOW      | WARN | 容器内 /workspace 可用空间 < 100MB                  | 清理容器内大文件，或联系管理员扩容 volume                          | ✅ |
| DISK_HOTSYNC_DATA_BLOAT | WARN | 热同步数据目录 ~/.cloud-claude/mutagen/ > 1GB       | 运行 mutagen daemon stop && rm -rf ~/.cloud-claude/mutagen/sessions/ | ✅ |
| DISK_VSCODE_SERVER_WARN | WARN | ~/.vscode-server/ 占用 %dMB，超过 500MB 警戒线     | 清理 ~/.vscode-server/extensions-cache/ 或删除不常用的扩展          | ✅ |
| DISK_VSCODE_SERVER_BLOAT | ERROR | ~/.vscode-server/ 占用 %dMB，超过 2GB 严重警戒线   | 运行 rm -rf ~/.vscode-server/ 完全清理（VS Code 重连时会自动重建） | ✅ |

### 3.3 MOUNT_*（Mutagen / sshfs / mergerfs 三层）

| Code | Severity | Message（摘要） | NextAction（摘要） | Extended |
|------|----------|-----------------|---------------------|----------|
| MOUNT_MUTAGEN_VERSION_SKEW       | ERROR | Mutagen 客户端与容器内 agent 版本不一致，已降级 sshfs-only | 升级容器镜像到 v3.0.0+ 或重装 cloud-claude                        | ✅ |
| MOUNT_MUTAGEN_WHITELIST_REJECT   | ERROR | 同步候选目录 > 50MB 白名单，已自动降级 sshfs               | 在 .mutagen.yml 添加 ignore 规则，或运行 du -sh ./* 查看大目录    | ✅ |
| MOUNT_MUTAGEN_SAFETY_GUARD       | FATAL | 本地空目录 vs 远端有文件，拒绝同步以防反向清空              | 如确认从远端拉取，先 cloud-claude exec rsync /workspace-hot/ ./   | ✅ |
| MOUNT_MUTAGEN_DAEMON_UNAVAILABLE | ERROR | Mutagen daemon 启动失败                                    | 检查 ~/.cloud-claude/mutagen/ 目录权限，或重启 cloud-claude       | ✅ |
| MOUNT_MUTAGEN_SYNC_FAILED        | ERROR | Mutagen sync 创建失败                                      | 检查 SSH 连通性，或运行 cloud-claude doctor mount                 | ✅ |
| MOUNT_MUTAGEN_TRANSPORT_FAILED   | ERROR | Mutagen ssh 子进程启动失败                                 | 检查本机 ssh 客户端，或安装 sshpass 作为后备                       | ✅ |
| MOUNT_HOT_SYNC_FAILED            | ERROR | 热同步失败                                                 | 检查当前目录可读写、远端 staging 路径权限，或回退 sshfs-only       | ✅ |
| MOUNT_SSHFS_FAILED               | ERROR | sshfs 挂载失败                                             | 检查 /dev/fuse 是否可用，或运行 cloud-claude doctor ssh           | ✅ |
| MOUNT_SSHFS_DISCONNECTED         | WARN  | sshfs 已断开 ≥15 秒，已从 mergerfs 摘除 cold 分支          | 网络恢复后 cloud-claude doctor mount --fix 重新挂载               | ✅ |
| MOUNT_MERGERFS_FAILED            | ERROR | mergerfs 挂载失败                                          | 检查容器是否启用 SYS_ADMIN + /dev/fuse，或运行 doctor mount       | ✅ |
| MOUNT_AUTO_DOWNGRADED            | WARN  | 文件映射已从 %s 降级到 %s（M13 留痕）                     | 运行 cloud-claude doctor mount 查看详细修复建议                    | ✅ |
| MOUNT_FORCE_MODE_FAILED          | FATAL | --mount-mode=%s 强制模式下某层失败                        | 移除 --mount-mode flag 让自动降级生效                             | ✅ |
| MOUNT_APFS_CASE_INSENSITIVE      | INFO  | 检测到 macOS APFS case-insensitive，已强制 two-way-resolved | 无需操作；如需 case-sensitive 请创建 case-sensitive APFS 卷       | — |
| MOUNT_REQUIRE_GIT_REPO           | ERROR | 当前目录 %s 不在 git 仓库内，cloud-claude 拒绝挂载以避免误同步整个家目录 | cd 到 git 仓库根目录后重试，或在当前目录运行 git init 后再启动 cloud-claude       | ✅ |
| MOUNT_OVERSIZED_FILE_SKIPPED     | WARN  | %s (%dMB) 超过 hot_sync_max_file_mb=%d 阈值，已跳过热同步，由 cold sshfs 兜底 | 编辑 ~/.cloud-claude/config.yaml 调整 hot_sync_max_file_mb，或在 .gitignore 加入该路径 | ✅ |
| MOUNT_GIT_PROXY_DISABLED         | WARN  | proxy_commands 未包含 git，git 子命令不会走本地代理转发         | 编辑 ~/.cloud-claude/config.yaml 的 proxy_commands 字段加入 git 后重启 cloud-claude | ✅ |
| MOUNT_DEFAULT_IGNORE_DISABLED    | WARN  | CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1，默认二进制黑名单已禁用，大文件可能进入热同步 | 如非排查需要，请 unset CLOUD_CLAUDE_NO_DEFAULT_IGNORE 后重启 cloud-claude          | ✅ |
| MOUNT_PROMOTER_FAILED            | WARN  | 远程 cold-promoter 启动失败: %s，降级为无晋升模式（cold 分支仍可读写） | 检查容器内 SSH session 是否可用，或设 CLOUD_CLAUDE_NO_PROMOTION=1 禁用晋升          | ✅ |

### 3.4 NET_*（OAuth / Reconnect / Egress IP）

| Code | Severity | Message（摘要） | NextAction（摘要） | Extended |
|------|----------|-----------------|---------------------|----------|
| NET_OAUTH_EXPIRED             | FATAL | Claude OAuth 凭证已过期                                 | 在容器内运行 cloud-claude exec claude login 重新登录             | ✅ |
| NET_OAUTH_EXPIRING_SOON       | WARN  | Claude OAuth 凭证将在 %d 分钟后过期                     | 建议尽快 cloud-claude exec claude login                          | ✅ |
| NET_OAUTH_NOT_FOUND           | FATAL | 容器内未找到 Claude OAuth 凭证文件                      | 在容器内运行 cloud-claude exec claude login 完成首次登录         | ✅ |
| NET_RECONNECT_BACKOFF         | INFO  | 网络中断，正在重连（已等待 %s）                         | 按 Enter 立即重试，或等待自动重连                                 | — |
| NET_RECONNECT_GAVE_UP         | FATAL | 重连失败（已重试 %d 次，耗时 %s）                       | 检查网络后重新运行 cloud-claude，或运行 cloud-claude doctor       | ✅ |
| NET_TCP_KEEPALIVE_UNSUPPORTED | WARN  | TCP keepalive 平台特化失败                              | 无需操作；SSH 应用层 keepalive 仍生效                            | ✅ |
| NET_EGRESS_IP_DRIFT           | WARN  | 容器出口 IP 与 Entry API 期望值不一致                   | 检查代理出口配置，或运行 cloud-claude doctor network             | ✅ |

### 3.5 SESSION_*（tmux / sync_lock / 输入缓冲）

| Code | Severity | Message（摘要） | NextAction（摘要） | Extended |
|------|----------|-----------------|---------------------|----------|
| SESSION_KEEPALIVE_TOO_AGGRESSIVE | FATAL | SSH KeepAlive 间隔 %s 低于 15s 下限                  | 调整 keepalive_interval 至 >= 15s                             | ✅ |
| SESSION_TMUX_UNAVAILABLE         | WARN  | 容器内 tmux 不可用，会话恢复已禁用                   | 检查容器镜像是否升级到 v3.0.0                                | ✅ |
| SESSION_NOT_FOUND                | ERROR | tmux 会话 %s 不存在                                  | 运行 cloud-claude sessions ls 查看当前会话列表                | ✅ |
| SESSION_TAKEOVER_NOTIFIED        | INFO  | 已通知其它 %d 个客户端断开                            | 无需操作；其它客户端 3 秒后看到中断提示                       | — |
| SESSION_TAKEOVER_FAILED          | ERROR | tmux detach-client 命令失败                           | 运行 cloud-claude sessions ls 检查会话状态                    | ✅ |
| SESSION_SYNC_LOCKED              | WARN  | 账号 %s 已有另一端在执行热同步，本端只读 sshfs        | 无需操作；如需独占同步请先关闭另一端 cloud-claude              | ✅ |
| SESSION_BUFFER_OVERFLOW          | WARN  | 本地输入缓冲已满（4KB），部分历史输入已丢弃           | 等待网络恢复后重新输入丢失部分；避免断网期间粘贴大段           | ✅ |

### 3.6 SSH_*（known_hosts / sshd 基线）

| Code | Severity | Message（摘要） | NextAction（摘要） | Extended |
|------|----------|-----------------|---------------------|----------|
| SSH_KNOWN_HOSTS_CONFLICT           | WARN | ~/.ssh/known_hosts 中 %s fingerprint 与本次握手不一致              | 运行 cloud-claude doctor ssh --fix 自动 ssh-keygen -R                              | ✅ |
| SSH_SSHD_KEEPALIVE_DRIFT           | WARN | 远端 sshd ClientAlive 配置与基线 (15/8) 不一致                   | 重建容器以恢复基线（参考 deploy/docker/managed-user/sshd_config）                  | ✅ |
| SSH_VSCODE_SERVER_NOT_RUNNING      | INFO | VS Code Server 进程未运行（用户可能未使用 VS Code Remote-SSH）   | 无需操作；如需使用 VS Code Remote-SSH，请通过 VS Code 连接容器                     | ✅ |
| SSH_VSCODE_PORT_NOT_LISTENING      | WARN | VS Code Server 进程存在但未监听端口，服务可能未就绪               | 等待 VS Code Server 完成启动，或重启 VS Code Remote-SSH 连接                       | ✅ |
| SSH_FORWARDING_SOCKET_MISSING      | INFO | SSH forwarding socket 不存在（可能未建立 VS Code forwarding）    | 无需操作；forwarding 会在 VS Code Remote-SSH 连接时自动建立                        | ✅ |
| SSH_FORWARDING_BLOCKED             | WARN | 检测到 OUTPUT 链存在 DROP 规则，可能拦截 SSH forwarding 流量    | 检查容器内 iptables OUTPUT 链规则，确认 forwarding 流量未被拦截                    | ✅ |
| SSH_SSHD_FORWARDING_DISABLED       | WARN | 远端 sshd AllowTcpForwarding 未开启（当前值: %s），端口转发不可用 | 检查 deploy/docker/managed-user/sshd_config 并重建容器恢复基线                      | ✅ |
| SSH_SSHD_STREAM_FORWARDING_DISABLED | WARN | 远端 sshd AllowStreamLocalForwarding 未开启（当前值: %s）        | 检查 deploy/docker/managed-user/sshd_config 并重建容器恢复基线                      | ✅ |
| SSH_SSHD_GATEWAY_PORTS_OPEN        | WARN | 远端 sshd GatewayPorts 非 no（当前值: %s），可能导致外部暴露    | 将 GatewayPorts 改为 no 并重启 sshd，或重建容器                                    | ✅ |

### 3.7 STATE_*（持久化 / volume / 容器状态）

| Code | Severity | Message（摘要） | NextAction（摘要） | Extended |
|------|----------|-----------------|---------------------|----------|
| STATE_LAST_SESSION_MISSING  | INFO  | 未找到上次会话快照                                | 首次运行 cloud-claude 后再 doctor 即可看到                                | — |
| STATE_VOLUME_IN_USE_001     | ERROR | 持久化 volume %s 仍被容器持有，DELETE 拒绝         | 先停止容器：`cloud-claude admin hosts stop <id>`                            | ✅ |
| STATE_CONTAINER_NOT_RUNNING | WARN  | 主机 %s 状态非 running，远端 doctor 检查跳过       | 运行 `cloud-claude admin hosts start <id>` 启动容器                         | ✅ |

### 3.8 SYSTEM_*（OS / kernel / FUSE / DNS / timeout）

| Code | Severity | Message（摘要） | NextAction（摘要） | Extended |
|------|----------|-----------------|---------------------|----------|
| SYSTEM_APPARMOR_FUSERMOUNT3_MISSING | ERROR | AppArmor 缺 fusermount3 override                       | 按 host-preflight.sh 写入 capability dac_override 行                  | ✅ |
| SYSTEM_FUSE_RESIDUAL_MOUNT          | WARN  | 发现 %d 个残留 FUSE 挂载                                | 运行 cloud-claude doctor mount --fix 自动解挂                          | ✅ |
| SYSTEM_DNS_RESOLVE_FAILED           | ERROR | DNS 解析失败                                            | 运行 cloud-claude doctor network --fix 刷新 DNS 缓存                   | ✅ |
| SYSTEM_CHECK_TIMEOUT                | WARN  | 检查 %s 超时（>%s）                                     | 加 --verbose 放宽到 30s，或检查远端容器状态                            | ✅ |

---

## 4. 错误码使用 FAQ

### 4.1 如何 grep 所有出现点

```bash
rg 'errcodes\.(MOUNT_|AUTH_|DISK_|NET_|SESSION_|SSH_|STATE_|SYSTEM_)' internal/
```

### 4.2 `cloud-claude explain <code>` 长说明在哪

`internal/cloudclaude/errcodes/explanations.go::ExtendedExplanations`，每条 ≥ 200 中文字符，五段模板：触发场景 / 根本原因 / 复现方式 / 修复路径 / 关联文档。

```bash
cloud-claude explain MOUNT_MERGERFS_FAILED
cloud-claude explain STATE_VOLUME_IN_USE_001
```

豁免长说明的 6 条 informational：`MOUNT_APFS_CASE_INSENSITIVE` / `SESSION_TAKEOVER_NOTIFIED` / `NET_RECONNECT_BACKOFF` / `STATE_LAST_SESSION_MISSING` / `SSH_VSCODE_SERVER_NOT_RUNNING` / `SSH_FORWARDING_SOCKET_MISSING`（登记在 `explanations.go::ExplainExempt`）。

### 4.3 新增错误码的 PR checklist

1. 在对应域文件（如 `mount.go`）`init()` 中追加 `MustRegister(Entry{...})`
2. Severity / Message（含 `%s` 占位） / NextAction（≤ 80 runes）三字段必填
3. 非 informational 类必须在 `explanations.go::init()` 注册长说明（≥ 200 中文字符）
4. 在 `codes.go` 末段 `const` 块按域分组追加常量
5. 跑 `go test ./internal/cloudclaude/errcodes/...` 验证 `MustRegister` 不重复 panic
6. 更新本手册 §3 对应域子节
7. 反向 diff 命令（§2 末段）输出空

---

## 5. 已知跨 Phase 引用场景

帮助运维拿到错误码后快速定位代码与历史决策。

- `STATE_VOLUME_IN_USE_001` — 来源 Phase 33 admin DELETE handler（`internal/controlplane/http/admin_claude_accounts.go`），强一致路径返回 409；详见 `v3-claude-state-volumes.md` §3.3
- `MOUNT_AUTO_DOWNGRADED` — Warn 级，Phase 31 三层文件映射状态机；M13 防御「禁止静默降级」核心证据，stderr + last-session.json downgrade_chain 双留痕
- `NET_EGRESS_IP_DRIFT` — Warn 级，Phase 29 隧道强制层 + Phase 34 doctor network.egress_ip_visible；与项目核心价值「严格出网受控」直接关联
- `MOUNT_APFS_CASE_INSENSITIVE` — Info 级，Phase 31 macOS APFS 检测；Phase 35 真机验收（M5）专项覆盖
- `SESSION_SYNC_LOCKED` — Warn 级，Phase 32 D-17 sync_lock 互斥保护；secondary client 走 sshfs 只读视图
- `SSH_VSCODE_SERVER_NOT_RUNNING` / `SSH_VSCODE_PORT_NOT_LISTENING` / `SSH_FORWARDING_SOCKET_MISSING` / `SSH_FORWARDING_BLOCKED` — Phase 41 remote-ssh doctor 维度；VS Code Remote-SSH 兼容性检测
- `DISK_VSCODE_SERVER_WARN` / `DISK_VSCODE_SERVER_BLOAT` — Phase 41；~/.vscode-server/ 磁盘占用监控
- `SSH_SSHD_FORWARDING_DISABLED` / `SSH_SSHD_STREAM_FORWARDING_DISABLED` / `SSH_SSHD_GATEWAY_PORTS_OPEN` — Phase 44；sshd_config 基线安全校验，与 Phase 38 SSH Proxy 端口转发直接关联
- `MOUNT_REQUIRE_GIT_REPO` / `MOUNT_OVERSIZED_FILE_SKIPPED` / `MOUNT_GIT_PROXY_DISABLED` / `MOUNT_DEFAULT_IGNORE_DISABLED` / `MOUNT_PROMOTER_FAILED` — Phase 39 本地 Dev Containers + hot-sync 加固；WR-01 修复 AUTH_CONFIG_MISSING 误用

---

## 6. 快速诊断命令

```bash
cloud-claude explain MOUNT_MERGERFS_FAILED
cloud-claude doctor --json | jq '.checks[] | {domain, name, status, code}'
grep -hE 'Code\s*=\s*"[A-Z]+_[A-Z]+' internal/cloudclaude/errcodes/*.go | awk -F'"' '{print $2}' | sort -u | wc -l
rg 'errcodes\.MOUNT_' internal/runtime/ internal/cloudclaude/
bash scripts/ci-doctor-grep.sh ./cloud-claude
```

---

## 7. 参考

- `internal/cloudclaude/errcodes/codes.go::Registry` / `::Format` / `::MustRegister` — 注册表 API
- `internal/cloudclaude/errcodes/explanations.go::ExtendedExplanations` / `::ExplainExempt` — 长说明 + 豁免登记
- `internal/cloudclaude/errcodes/{auth,disk,mount,net,session,ssh,state,system,remote_ssh}.go` — 各域 init 登记
- `internal/cloudclaude/doctor/render.go` — 文本/JSON 双渲染含错误码
- `cmd/cloud-claude/explain.go` — `cloud-claude explain <code>` 子命令实现
- `scripts/ci-doctor-grep.sh` — M14 三段断言含错误码格式
- `docs/runbooks/v3-doctor-troubleshoot.md` — 错误码触发场景排障入口
- `docs/runbooks/v3-claude-state-volumes.md` — `STATE_VOLUME_IN_USE_001` 上下文
- `docs/runbooks/v3-apparmor-deployment.md` — `SYSTEM_APPARMOR_FUSERMOUNT3_MISSING` 部署修复
