# claudedock doctor 五维度排障手册（v3.0+）

> 适用版本：v3.0 起；对应阶段 Phase 34（claudedock-doctor-v3）
> 关联需求：REQ-F6-A（5 维度顺序） / REQ-F6-B（四要素输出契约） / REQ-F6-C（--fix 5 类自动修复） / REQ-F6-D（退出码 brew 对齐） / M13（降级历史第一屏） / M14（CI gate 三断言）

---

## 1. 背景

v3.0 把 v2 时代的 `claudedock doctor`（PASS/FAIL 黑盒）升级为 5 维度 18 项自检 + `--fix` 自动修复 + JSON schema 契约。设计目标：

- 让运维拿到任一 [!]/[✗] 行都能直接 copy-paste 修复命令（M14 四要素：`[图标] 中文原因 + 建议: &lt;next_action&gt; + 错误码: &lt;CODE&gt;`）
- 让 CI/CD 把 doctor JSON 当作健康度门禁（M14：`schema_version=1` 锁死 + `next_action` 必填 + 错误码命名规范）
- 把过去散落在多个 SUMMARY 的降级链整合到 doctor 启动第一屏（M13：禁止静默降级）

5 维度执行顺序串行：**network → auth → ssh → mount → disk**（与 `internal/claudedock/doctor/doctor.go::RunDoctor` L83-84 注释一致），任一维度可单独跑（`claudedock doctor mount`）。

---

## 2. 输出格式契约

### 2.1 文本模式（默认）

四要素硬约束（`scripts/ci-doctor-grep.sh::(2)(3)` 双重验证）：

```
[✓]  network.dns_resolve: 解析 gateway.example.com 成功
[!]  mount.mergerfs_branches: 仅检测到 1 个分支
       建议: 运行 claudedock doctor mount --fix 自动重挂
       错误码: MOUNT_MERGERFS_FAILED
[✗]  auth.config_present: 找不到 ~/.claudedock/config.yaml
       建议: 运行 claudedock init 重新配置网关与凭证
       错误码: AUTH_CONFIG_MISSING
```

图标语义：

| 图标 | Status | 含义 |
|------|--------|------|
| `[✓]` | `pass` | 通过 |
| `[!]` | `warn` | 警告，非阻塞，建议修复 |
| `[✗]` | `fail` | 失败，必须修复 |
| `[~]` | `skip` | 跳过（前置依赖未满足，常见于未 init / 未连远端） |

> 「建议:」与「错误码:」两段缩进 7 个空格，格式由 `internal/claudedock/doctor/render.go` 锁定。CI gate 会硬断 warn/fail 行必须同时含两段。

### 2.2 JSON 模式（`--json`）

`schema_version=1` 是不带 `omitempty` 的硬编码（`doctor.go::Report.SchemaVersion`），让 jq 可以稳定 select。完整片段：

```json
{
  "schema_version": 1,
  "started_at": "2026-04-22T11:00:00Z",
  "duration_ms": 1820,
  "claudedock_version": "v3.0.3",
  "remote_image_version": "v3.0.0",
  "downgrade_history": {
    "snapshot_age_seconds": 42,
    "intended_mode": "Auto",
    "actual_mode": "MutagenOnly",
    "downgrade_chain": [
      {"from": "Auto", "to": "MutagenOnly", "reason_code": "MOUNT_SSHFS_DISCONNECTED", "reason_message": "sshfs 已断开 ≥15 秒"}
    ]
  },
  "summary": {"total": 18, "pass": 16, "warn": 1, "fail": 1, "skip": 0},
  "checks": [
    {"domain": "network", "name": "dns_resolve", "status": "pass"},
    {"domain": "mount", "name": "mergerfs_branches", "status": "warn",
     "code": "MOUNT_MERGERFS_FAILED",
     "message": "仅检测到 1 个分支",
     "next_action": "运行 claudedock doctor mount --fix 自动重挂"}
  ]
}
```

### 2.3 退出码（REQ-F6-D，对齐 brew doctor）

| 退出码 | 含义 |
|--------|------|
| `0` | 全部 pass |
| `1` | 至少一项 warn |
| `2` | 至少一项 fail（CI 必须当作 hard fail） |

---

## 3. 五维度检查逻辑

执行顺序：**network → auth → ssh → mount → disk**（与 `doctor.go` L83 注释字面量一致；后端 lazy SSH 在 auth 维度初次 ensureRemote）。

### 3.1 network（3 项）

| Check | 触发的常见错误码 |
|-------|----------------|
| `dns_resolve` | `SYSTEM_DNS_RESOLVE_FAILED` |
| `gateway_reachable` | `AUTH_GATEWAY_UNREACHABLE` |
| `egress_ip_visible` | `NET_EGRESS_IP_DRIFT` |

排障流程（任一项 [!]/[✗] 时）：

1. 确认本机网络：`ping -c 3 1.1.1.1` + `ping -c 3 <gateway-host>`
2. 检查 DNS：`dig +short <gateway-host>`，缓存陈旧时手工 flush（macOS：`sudo dscacheutil -flushcache`；Linux：`sudo systemd-resolve --flush-caches`）
3. 出口 IP 漂移：`docker exec <container> curl -s https://ifconfig.me`，与 admin 后台 binding 比对
4. 自动修复：`claudedock doctor network --fix`（5 类修复中的「DNS 缓存 flush」）

### 3.2 auth（3 项）

| Check | 触发的常见错误码 |
|-------|----------------|
| `config_present` | `AUTH_CONFIG_MISSING` |
| `entry_token_valid` | `AUTH_TOKEN_EXPIRED` / `AUTH_GATEWAY_UNREACHABLE` |
| `oauth_credentials` | `NET_OAUTH_EXPIRED` / `NET_OAUTH_NOT_FOUND` / `NET_OAUTH_EXPIRING_SOON` / `AUTH_OAUTH_REFRESH_FAILED` |

排障流程：

1. `AUTH_CONFIG_MISSING` → 运行 `claudedock init` 交互式重建配置
2. `AUTH_TOKEN_EXPIRED` → `claudedock doctor auth --fix` 自动用 short_id/password 换新 token
3. `NET_OAUTH_EXPIRED` / `NET_OAUTH_NOT_FOUND` → 进容器执行 `claude login` 重新授权（Phase 33 后登录态会持久化到 `claude-state-<account_id>` volume）
4. `AUTH_OAUTH_REFRESH_FAILED` → 通常是 30 天 refresh_token 也已过期，同 OAuth 重新登录路径

### 3.3 ssh（4 项）

| Check | 触发的常见错误码 |
|-------|----------------|
| `keepalive_config` | `SESSION_KEEPALIVE_TOO_AGGRESSIVE` |
| `sshd_keepalive_drift` | `SSH_SSHD_KEEPALIVE_DRIFT` |
| `known_hosts` | `SSH_KNOWN_HOSTS_CONFLICT` |
| `workspace_ssh_keys` | `MOUNT_SSHFS_FAILED`（衍生：authorized_keys 缺失） |

排障流程：

1. `SESSION_KEEPALIVE_TOO_AGGRESSIVE` → 把 `~/.claudedock/config.yaml` 的 `keepalive_interval` 改为 ≥ `15s`
2. `SSH_SSHD_KEEPALIVE_DRIFT` → 重建容器恢复基线（`ClientAliveInterval=15` / `ClientAliveCountMax=8`，参考 `deploy/docker/managed-user/sshd_config`）
3. `SSH_KNOWN_HOSTS_CONFLICT` → `claudedock doctor ssh --fix` 自动 `ssh-keygen -R <host>`，下次握手会重新写入正确 fingerprint
4. workspace ssh keys 异常 → 用 `claudedock ssh doctor` 子命令做 owner/mode/PEM 自检与修复

### 3.4 mount（5 项）

| Check | 触发的常见错误码 |
|-------|----------------|
| `mutagen_version` | `MOUNT_MUTAGEN_VERSION_SKEW` |
| `mergerfs_branches` | `MOUNT_MERGERFS_FAILED` / `MOUNT_AUTO_DOWNGRADED` |
| `sshfs_mountpoint` | `MOUNT_SSHFS_DISCONNECTED` / `MOUNT_SSHFS_FAILED` |
| `fuse_residual` | `SYSTEM_FUSE_RESIDUAL_MOUNT` |
| `apparmor_fusermount3` | `SYSTEM_APPARMOR_FUSERMOUNT3_MISSING` |

排障流程：

1. `MOUNT_MUTAGEN_VERSION_SKEW` → 升级容器镜像到 v3.0.0+（含 mutagen v0.18.1 agent），或 `claudedock doctor mount --fix` 重启 daemon
2. `MOUNT_MERGERFS_FAILED` → 检查容器是否启用 `--cap-add SYS_ADMIN --device /dev/fuse`；常见根因为 AppArmor 拦截，按 `v3-apparmor-deployment.md` 配置 override
3. `MOUNT_SSHFS_DISCONNECTED` → 等网络恢复后 `claudedock doctor mount --fix` 自动 `fusermount3 -u` + 重新 mount + mergerfs add 回去
4. `SYSTEM_FUSE_RESIDUAL_MOUNT` → `claudedock doctor mount --fix` 自动批量 unmount，或手工 `mount | grep fuse` 列出后逐个 `fusermount3 -u`
5. `SYSTEM_APPARMOR_FUSERMOUNT3_MISSING` → 跳到 `v3-apparmor-deployment.md` §4 部署步骤

### 3.5 disk（3 项）

| Check | 触发的常见错误码 |
|-------|----------------|
| `local_disk` | `DISK_LOCAL_LOW` |
| `container_disk` | `DISK_CONTAINER_LOW` |
| `mutagen_data_size` | `DISK_MUTAGEN_DATA_BLOAT` |

阈值（硬编码在 `internal/claudedock/doctor/disk.go`）：

| 检查 | 警戒线 |
|------|--------|
| 本地 `~/.claudedock/` 可用空间 | < 500MB |
| 容器内 `/workspace` 可用空间 | < 100MB |
| 本地 `~/.claudedock/mutagen/` 总大小 | > 1GB |

排障流程：

1. `DISK_LOCAL_LOW` → `du -sh ~/* | sort -h` 找大文件；或 `mutagen daemon stop && rm -rf ~/.claudedock/mutagen/sessions/`
2. `DISK_CONTAINER_LOW` → 进容器 `du -sh /workspace/* | sort -h`，清理 `node_modules` / `target` / `dist`；最坏调用 admin recreate（保留 `claude-state` volume）
3. `DISK_MUTAGEN_DATA_BLOAT` → 同 `DISK_LOCAL_LOW` 的 mutagen sessions 清理路径

---

## 4. `--fix` 自动修复能力（REQ-F6-C）

5 类自动修复均通过 `internal/claudedock/doctor/fix.go::FixerRegistry` 注册，`ApplyFixes` 在 60s timeout 内完成。

| 类别 | 修复动作 | 幂等性 | 失败回退 |
|------|----------|--------|----------|
| `mutagen daemon 重启` | `mutagen daemon stop && mutagen daemon start` | 幂等 | 失败时 status 不降级（D-16），仅 stderr 记录 |
| `FUSE 残留挂载清理` | 遍历 `mount | grep fuse` → `fusermount3 -u <path>` | 幂等（重复 unmount 容忍） | 单条失败不阻塞其它 |
| `known_hosts 冲突` | `ssh-keygen -R <host>` | 幂等 | 文件不存在时静默跳过 |
| `OAuth token 刷新` | 触发 entry API refresh 流程 | 幂等（refresh_token 仍有效时） | refresh 也过期 → AUTH_OAUTH_REFRESH_FAILED |
| `DNS 缓存 flush` | macOS：`dscacheutil -flushcache`；Linux：`systemd-resolve --flush-caches` | 幂等 | 平台不支持时跳过 |

破坏性命令（如 `rm -rf ~/.claudedock/mutagen/sessions/`）走 `confirmDestructive` 三级判定：TTY 交互 / `--yes` / JSON 模式自动拒绝。

---

## 5. 降级历史第一屏（M13 锁定）

doctor 启动时读取 `~/.claudedock/state/last-session.json`（`claudedock.LoadLastSession`）→ 转换为 `DowngradeBanner` → 在所有 check 之前打印第一屏。

```
── 上次会话快照（42 秒前） ──
意图模式: Auto    实际模式: MutagenOnly    重连次数: 2
降级链:
  Auto → MutagenOnly  原因: [MOUNT_SSHFS_DISCONNECTED] sshfs 已断开 ≥15 秒
```

JSON 模式同等内容写在 `report.downgrade_history`：

```bash
claudedock doctor --json | jq '.downgrade_history.downgrade_chain'
```

> 没有 last-session.json（首次运行或被清理）时，降级第一屏空，对应 banner 行 `[!] 未找到上次会话快照` 走 `STATE_LAST_SESSION_MISSING`（Info 级，已豁免长说明）。该行不在 5 维度 check 之内，CI gate 不要求其带「建议:」（详见 `ci-doctor-grep.sh` L49 注释）。

---

## 6. CI gate 集成（M14）

`scripts/ci-doctor-grep.sh` 三段断言，与 `Makefile::ci-gate` target 联动：

1. `--json` 输出合法 JSON 且 `schema_version == 1`
2. 所有 `status ∈ {warn,fail}` 的 check 必带非空 `next_action`
3. 文本模式所有 `[!]/[✗]` 行必含 `建议:` + 错误码（正则 `[A-Z]+_[A-Z]+_[A-Z0-9]+`）

接入方式（CI 任意 job）：

```bash
go build -o claudedock ./cmd/claudedock
bash scripts/ci-doctor-grep.sh ./claudedock
# 期望: OK: claudedock doctor M14 gate passed (schema=1 / next_action / 错误码).
```

任一失败退出 1 + stderr 列出违例行，PR 应被拦截。

---

## 7. 故障排查案例集

### 7.1 案例：首连后 `[✗] mount.mergerfs_branches` 报 `MOUNT_MERGERFS_FAILED`

事件：`claudedock` 第一次连上后立刻 `[✗] mount.mergerfs_branches: 仅检测到 1 个分支`，错误码 `MOUNT_MERGERFS_FAILED`。

排查步骤：

1. 进容器看 mount table：

   ```bash
   docker exec <container> mount | grep -E 'fuse|mergerfs|sshfs'
   ```

2. 三路对照：sshfs（cold）+ mutagen（hot）+ mergerfs（union），缺哪条就看哪个错误码
3. 90% 概率根因：宿主机 AppArmor 缺 `fusermount3` override（Ubuntu 25.04+）→ 跳到 `v3-apparmor-deployment.md` §4
4. 如果 AppArmor 已就位，检查容器 `--cap-add SYS_ADMIN` 与 `--device /dev/fuse`：

   ```bash
   docker inspect <container> --format '{{.HostConfig.CapAdd}} {{.HostConfig.Devices}}'
   ```

修复命令（顺序尝试，每步后重跑 `claudedock doctor mount`）：

```bash
claudedock doctor mount --fix       # 自动 fusermount3 -u + remount
docker exec <container> systemctl restart claudedock-mount  # 容器侧重挂
# 仍失败：admin recreate 容器（保留 claude-state volume）
curl -fsS -X POST -H "Authorization: Bearer <ADMIN-JWT>" \
  http://localhost:8080/v1/admin/hosts/<host_id>/recreate
```

### 7.2 案例：断网恢复后 `[!] auth.oauth_credentials` 报 `NET_OAUTH_EXPIRED`

事件：拔网 30s 后恢复，doctor 出现 `[!] auth.oauth_credentials: Claude OAuth 凭证已过期`。

排查步骤：

1. 进容器查 credentials 时间：

   ```bash
   docker exec <container> stat /home/claude/.claude/.credentials.json
   docker exec <container> cat /home/claude/.claude/.credentials.json | jq '.expires_at'
   ```

2. 与系统时间比对（容器 / 宿主机时区可能不同）
3. 如果是断网期间 token 真到期：进容器 `claude login` 重新授权
4. 如果是 token 还有效但被误判：检查容器内时钟漂移（`docker exec <container> date`）

修复命令：

```bash
docker exec -it <container> claude login
claudedock doctor auth                # 验证恢复 pass
```

### 7.3 案例：CI 跑 `--json` 但 jq 拿到空对象，退出码 2 但没 stderr

事件：CI step `claudedock doctor --json | jq '.summary'` 输出 `{}`、退出码 2，但 stderr 无内容。

排查步骤：

1. 加 `--verbose` 重跑：

   ```bash
   claudedock doctor --json --verbose 2>doctor-stderr.log >doctor.json
   echo "exit=$?"
   ```

2. 检查是否 `[fix]` 头被提前写到 stdout（破坏 JSON）：JSON 模式下 fix 输出全部写 stderr 是 D-16 守恒，正常运行时 stdout 应是单段合法 JSON
3. 如果 JSON 文件第一行非 `{`，说明 stdout 被污染 → 联系维护者，附 `--verbose` log
4. 退出码 2 但无 stderr 通常是某个 fail check 的 message 为空，反查 `report.json` 对应 entry：

   ```bash
   jq '.checks[] | select(.status=="fail")' doctor.json
   ```

修复命令：通常是上游某项 [✗]，按 §3 对应维度处置。

---

## 8. 快速诊断命令

```bash
claudedock doctor --json | jq '.summary'
claudedock doctor --json | jq '.checks[] | select(.status=="warn" or .status=="fail")'
claudedock doctor mount --fix
claudedock explain MOUNT_MERGERFS_FAILED
bash scripts/ci-doctor-grep.sh ./claudedock
```

---

## 9. 参考

- `internal/claudedock/doctor/doctor.go::RunDoctor`（L83-84 5 维度顺序注释，L122-151 lazy SSH）
- `internal/claudedock/doctor/network.go` / `auth.go` / `ssh.go` / `mount.go` / `disk.go` — 各维度 check 实现
- `internal/claudedock/doctor/fix.go::FixerRegistry` / `ApplyFixes` — 5 类自动修复
- `internal/claudedock/doctor/render.go` — 文本/JSON 双渲染（schema_version=1 锁死）
- `internal/claudedock/errcodes/codes.go::Format` — 四要素文案模板（`[CODE] Message\n  建议: NextAction`）
- `internal/claudedock/errcodes/explanations.go` — `claudedock explain <code>` 长说明
- `scripts/ci-doctor-grep.sh` — M14 三段断言 CI gate
- `docs/runbooks/v3-error-code-index.md` — 错误码全集索引
- `docs/runbooks/v3-apparmor-deployment.md` — AppArmor override 部署
- `docs/runbooks/v3-claude-state-volumes.md` — Claude OAuth 持久化 volume 生命周期
