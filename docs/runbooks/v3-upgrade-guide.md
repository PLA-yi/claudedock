# v2.0 → v3.0 升级指南（v3.0+）

> 适用版本：v3.0 起；对应阶段 Phase 29-34（v3.0 远端开发体验升级）
> 关联需求：C6（AppArmor override） / M13（禁止静默降级） / M5（APFS case-insensitive） / SC#9（运维手册收口）

---

## 1. 背景

v3.0 相对 v2.0 的核心变动集中在「文件映射 / 会话可靠性 / 持久化 / 自检体系」四条主线，运维侧需要在控制面、镜像、CLI 客户端三处协同升级，且任一处单边升级都不允许。

| 主线 | v2.0 | v3.0 |
|------|------|------|
| 文件映射 | 纯 sshfs | Mutagen（hot）+ sshfs（cold）+ mergerfs union 三层 |
| 会话可靠性 | 直连 SSH，断网即退出 | tmux 包装 + Reconnector（1/2/4/8/30s 退避）+ BufferedStdin 4KB 缓冲 |
| 多端连接 | 各开各的 | 同 `claude_account` 默认 attach 同一 tmux session，`--new-session` 独占 |
| Claude 登录态 | 容器重建即丢 | 按 `claude_account_id` 绑定 named volume `claude-state-<id>` |
| 自检 | `claudedock doctor`：PASS/FAIL 黑盒 | doctor v3：5 维度 18 项 + 四要素 + `--fix` 自动修复 |
| 错误体系 | 文本日志 | 8 域 ≥ 42 条错误码 + `claudedock explain <code>` 长说明 |

`docs/runbooks/v3-error-code-index.md` 列出全量错误码；`docs/runbooks/v3-doctor-troubleshoot.md` 给出 5 维度排障 cookbook。

---

## 2. 升级前置条件清单

| 维度 | v2.0 | v3.0 目标 | 检查命令 |
|------|------|-----------|----------|
| 控制面 | 任意 v2 镜像 | `claudedock:v3.0.0` 起 | `docker compose ps control-plane` |
| Worker / Host-agent | 任意 v2 镜像 | `claudedock:v3.0.0` 起 | `docker compose ps worker host-agent` |
| 受管容器镜像 | v2 `managed-user` | `ghcr.io/claudedock/claudedock/managed-user:latest`（含 `image_version: v3.0.0`） | `awk -F': ' '$1=="image_version"{print $2}' deploy/docker/managed-user/image.lock` |
| CLI 客户端 | v2.0 | v3.0.x | `claudedock --version` |
| 数据库迁移 | 截止 0013 | 至少 0014 (`claude_account_persistent_volume`) | `psql ... -tAc "SELECT max(version) FROM schema_migrations"` |
| 宿主机 AppArmor（仅 Ubuntu ≥ 25.04） | 无要求 | 写入 `capability dac_override,` override，详见 `v3-apparmor-deployment.md` | `bash deploy/scripts/host-preflight.sh` |
| 宿主机 FUSE | 任意 | `/dev/fuse` 可用 + `user_allow_other` 启用 | `bash scripts/verify-fuse-compat.sh` |

> AppArmor 三档处置：
> - Ubuntu < 25.04：跳过，无须 override
> - Ubuntu ≥ 25.04 且未配置：必须按 `v3-apparmor-deployment.md` 写入 override，否则容器内 mergerfs / sshfs 会 EPERM
> - 非 Ubuntu：跳过

---

## 3. 控制面升级步骤

按以下顺序执行，任一步失败立即停止并按 §7「回滚触发条件」决定是否回滚。

1. 取最新代码：

   ```bash
   cd <repo-root>
   git fetch origin && git checkout v3.0.0
   ```

2. 拉取镜像：

   ```bash
   docker compose pull control-plane worker host-agent
   ```

3. 应用数据库迁移（含 Phase 30 的 `0014_claude_account_persistent_volume.sql`）：

   ```bash
   make migrate-up   # 或: go run ./cmd/migrate up
   psql "$DATABASE_URL" -tAc \
     "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 5"
   # 期望首行 ≥ 0014
   ```

4. 重启控制面三件套（保留旧容器镜像 tag 以便回滚）：

   ```bash
   docker compose up -d control-plane worker host-agent
   docker compose ps   # 全部 healthy
   ```

5. 控制面健康检查：

   ```bash
   curl -fsS http://localhost:8080/health | jq .
   curl -fsS http://localhost:8080/v1/admin/claude-accounts \
     -H "Authorization: Bearer <ADMIN-JWT>" | jq 'length'
   ```

回滚：

```sql
-- 回滚 migration 0014（仅在控制面尚未生产化前可执行）
SELECT version FROM schema_migrations WHERE version = '0014';
```

```bash
# 回滚镜像 tag
docker compose down
git checkout v2.<旧版本>
docker compose up -d
```

> 0014 迁移仅追加 `claude_accounts.persistent_volume_name` 列，不会破坏 v2 行为；回滚时如该列已写值会被丢弃，但不会导致数据损坏。

---

## 4. 镜像升级步骤

受管 `managed-user` 镜像通过 `deploy/docker/managed-user/image.lock` 锁定。v3.0 字面量：

```
image_name: ghcr.io/claudedock/claudedock/managed-user:latest
local_dev_image_name: ghcr.io/claudedock/claudedock/managed-user:latest
base_image: ubuntu:24.04
image_version: v3.0.0
mergerfs_version: 2.41.1
tmux_version_min: "3.4"
supports_mergerfs: true
```

升级动作：

```bash
docker pull ghcr.io/claudedock/claudedock/managed-user:latest

# 触发已运行容器重建（按 host_id 滚动）
curl -fsS -X POST \
  -H "Authorization: Bearer <ADMIN-JWT>" \
  http://localhost:8080/v1/admin/hosts/<host_id>/recreate

# 验证容器内 image_version 已切换
docker exec <container> cat /etc/claudedock/image.version
# 期望: v3.0.0
```

容器重建后必须保留 `claude-state-<id>` named volume（worker `createHost` 自动幂等绑定，详见
`v3-claude-state-volumes.md`）。

---

## 5. CLI 客户端升级

```bash
curl -fsSL https://raw.githubusercontent.com/claudedock/claudedock/main/scripts/install.sh | bash
claudedock --version
# 期望: v3.0.x
```

升级后清理本地残留：

```bash
mutagen daemon stop || true
rm -rf ~/.claudedock/mutagen/sessions/   # 旧 session 数据，重启自动重建
ls ~/.claudedock/                         # 应保留 config.yaml + cache/
```

> `~/.claudedock/config.yaml` 在 v3.0 兼容 v2.0 schema，不需要重新 `claudedock init`。
> Mutagen v0.18.1 二进制由 install.sh 一并下发；如需手工校验，运行
> `claudedock doctor mount --json | jq '.checks[] | select(.name=="mutagen_version")'`。

---

## 6. 升级后自检

按顺序执行三条命令；任一项 FAIL 立即停止并按 §7 决策。

```bash
# (1) doctor 五维度全 pass
claudedock doctor --json | jq '.summary'
# 期望: {"total":N,"pass":N,"warn":0,"fail":0,"skip":0}

# (2) v3 新错误码可被 explain（验证 errcodes 注册表生效）
claudedock explain MOUNT_MUTAGEN_VERSION_SKEW
claudedock explain STATE_VOLUME_IN_USE_001

# (3) 容器内 mergerfs 选项就位
docker exec <container> mount | grep mergerfs
# 期望含: func.readdir=cor cache.attr=30 category.create=ff
```

补充验证（按需）：

```bash
# (4) 持久化 volume 已挂载
docker exec <container> readlink /home/claude/.claude
# 期望: /var/lib/claude-persist/.claude

# (5) tmux 会话基线
docker exec <container> tmux ls
```

---

## 7. 常见回归与回滚触发条件

满足以下任一条件即触发回滚（按 §3 末段步骤）：

1. **首连 > 8s**：`claudedock` 冷启动到 prompt 可输入超过 BASE-02 基线 8 秒，多机复现
2. **断网 30s 内 claude 退出**：`docker exec <ctr> pgrep -f claude` 在拔网期间返回非 0，违反 REQ-F4-A
3. **Mutagen 版本漂移**：`claudedock doctor mount` 输出 `MOUNT_MUTAGEN_VERSION_SKEW`，且 `--fix` 后仍未恢复
4. **mergerfs 参数漂移**：`docker exec <ctr> mount | grep mergerfs` 缺失 `category.create=ff` 或 `cache.attr=30`，且容器重建后仍未恢复
5. **AppArmor 阻断**：Ubuntu ≥ 25.04 宿主机容器内 mergerfs/sshfs EPERM，按 `v3-apparmor-deployment.md` 写入 override 后仍未恢复

非回滚类常见回归（按 `v3-doctor-troubleshoot.md` 排障即可）：

- `MOUNT_AUTO_DOWNGRADED`（Warn）：自动降级到 sshfs-only，功能可用但性能受损
- `NET_RECONNECT_BACKOFF`（Info）：弱网下退避重连，正常
- `STATE_VOLUME_IN_USE_001`：admin DELETE 拒绝，按 `v3-claude-state-volumes.md` 处置

---

## 8. 快速诊断命令

```bash
# 控制面与镜像版本
docker compose ps
awk -F': ' '$1=="image_version"{print $2}' deploy/docker/managed-user/image.lock

# 客户端与 doctor 一站式
claudedock --version
claudedock doctor --json | jq '{summary, schema_version}'

# 受管容器版本与持久化检查
docker ps --filter label=com.claudedock.managed=true
docker exec <container> cat /etc/claudedock/image.version
docker exec <container> readlink /home/claude/.claude
```

---

## 9. 参考

- `internal/runtime/tasks/worker.go::createHost` — 容器创建时自动绑定 `claude-state-<id>` volume
- `internal/claudedock/doctor/doctor.go::RunDoctor` — 5 维度 doctor 入口
- `deploy/docker/managed-user/entrypoint.sh::prepare_persistent_state` — `~/.claude` 软链到 named volume
- `deploy/docker/managed-user/image.lock` — `image_version: v3.0.0` 锁
- `scripts/install.sh` — CLI 客户端一键安装
- `docs/runbooks/v3-claude-state-volumes.md` — 持久化 volume 命名规范与生命周期
- `docs/runbooks/v3-apparmor-deployment.md` — Ubuntu 25.04 AppArmor override 部署
- `docs/runbooks/v3-doctor-troubleshoot.md` — doctor 5 维度排障
- `docs/runbooks/v3-error-code-index.md` — 错误码索引
