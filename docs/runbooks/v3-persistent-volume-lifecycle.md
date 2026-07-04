# v3.0 持久卷生命周期总览（v3.0+）

> 适用版本：v3.0 起；对应阶段 Phase 31-33（Mutagen 三层文件映射 + claude-state 持久化）
> 关联需求：REQ-F1-D（hot 50MB 白名单） / REQ-F7-A（claude-state 命名规范） / M16（孤儿卷审计） / SC#9（运维手册收口）

---

## 1. 背景

v3.0 持久化矩阵分三类，每类的生命周期、所有者与排障路径都不同。本手册是顶层总览，把读者按问题类型导流到对应的细分文档。

| 卷类别 | 用途 | 持久性 | 详细手册 |
|--------|------|--------|----------|
| `claude-state-<account_id>`（Docker named volume） | Claude OAuth 凭证 / `~/.claude` 缓存 | 跨容器重建保留 | [v3-claude-state-volumes.md](./v3-claude-state-volumes.md) |
| Mutagen 数据卷（容器内 `/var/lib/mutagen/`） | mutagen daemon staging / 同步 session 元数据 | 容器重建即丢（设计上 ephemeral） | 本手册 §3 |
| mergerfs union layer（hot + cold 双分支） | 把 Mutagen 同步目录与 sshfs 远端目录合并成 `/workspace` | 运行时存在，停容器即消失 | 本手册 §4 |

---

## 2. 本手册导航

| 问题 | 跳转 |
|------|------|
| Claude OAuth 缓存 / `~/.claude` 持久化 / admin DELETE 行为 | → [v3-claude-state-volumes.md](./v3-claude-state-volumes.md) |
| 孤儿 `claude-state-*` volume 审计 / GC | → [v3-claude-state-volumes.md](./v3-claude-state-volumes.md) §5 |
| Mutagen 数据卷寿命 / 撑满排障 | § 本文件 §3 |
| mergerfs union 上层（hot）/ 下层（cold）关系与漂移 | § 本文件 §4 |
| 错误码索引 (`MOUNT_MUTAGEN_*` / `MOUNT_MERGERFS_*` / `STATE_VOLUME_IN_USE_001`) | → [v3-error-code-index.md](./v3-error-code-index.md) |
| doctor mount / disk 排障流程 | → [v3-doctor-troubleshoot.md](./v3-doctor-troubleshoot.md) §3.4 / §3.5 |

> 本手册不重复 `v3-claude-state-volumes.md` 已有的命名规范、admin DELETE 强一致 / force 路径、audit 事件清单与孤儿审计脚本。任何与 OAuth 持久化相关的工作请先点击上面跳转链接。

---

## 3. Mutagen 数据卷生命周期（Phase 31-32 范畴）

### 3.1 容器内路径与持久性

容器内 mutagen daemon 把 staging / 元数据写到 `/var/lib/mutagen/`（位于容器 rootfs，不绑定宿主机 named volume）。

判定方式：

```bash
docker inspect <container> --format '{{range .Mounts}}{{.Type}} {{.Source}} {{.Destination}}{{"\n"}}{{end}}' \
  | grep -E '/var/lib/mutagen|claude-persist'
# 期望: 仅看到 claude-persist 那一条 (Phase 33)，没有 mutagen 行
```

`internal/runtime/tasks/worker.go::createHost` 仅为 `claude-state-<account_id>` 这一条数据创建 named volume；mutagen 数据故意不持久化，理由：

- 同步状态 = 双方文件树 + session 元数据，session 元数据跨容器重启意义有限
- 强行持久化会拉长容器重建路径，违反 BASE-02 首连 ≤ 8s 基线
- 接受重建后首轮全量同步约 200MB 的代价（参考 RESEARCH §10k 文件树基准）

### 3.2 撑满诊断与处置

容器内：

```bash
docker exec <container> du -sh /var/lib/mutagen
# 警戒线: 视容器 rootfs 大小，> 500MB 即建议清理
```

doctor 会通过 `disk.mutagen_data_size` 在本地 `~/.claudedock/mutagen/` 维度命中 `DISK_MUTAGEN_DATA_BLOAT`（> 1GB）；容器侧无独立检查项，靠 `DISK_CONTAINER_LOW`（`/workspace` < 100MB）间接暴露。

清理步骤（按破坏性递增）：

1. 重启容器（mutagen daemon 退出 → tmpfs / overlay 释放）：

   ```bash
   docker restart <container>
   ```

2. 进容器手工清 sessions：

   ```bash
   docker exec <container> mutagen daemon stop
   docker exec <container> rm -rf /var/lib/mutagen/sessions/
   ```

3. 整体重建容器（保留 `claude-state` volume，OAuth 不丢）：

   ```bash
   curl -fsS -X POST -H "Authorization: Bearer <ADMIN-JWT>" \
     http://localhost:8080/v1/admin/hosts/<host_id>/recreate
   ```

---

## 4. mergerfs union layer 上下层关系（Phase 31 范畴）

### 4.1 分支语义

`/workspace` 是 mergerfs 的 union mount，下面有两条分支：

| 分支 | 来源 | 角色 | 默认写策略 |
|------|------|------|-----------|
| `/workspace-hot` | Mutagen 双向同步目录 | hot 层（高速、白名单内 ≤ 50MB） | mergerfs `category.create=ff`：新文件总是写到 first found = hot |
| `/workspace-cold` | sshfs 远端 mount | cold 层（容量、全量代码） | 仅当 hot 分支不可写或不存在时才落到 cold |

mergerfs 关键挂载选项（容器 entrypoint 固化）：

```
func.readdir=cor       # 并发读目录提升 ls 性能
cache.attr=30          # 元数据缓存 30s（降低 sshfs RTT）
category.create=ff     # first found 写策略
```

### 4.2 检视 mergerfs 实际分支

```bash
docker exec <container> getfattr -n user.mergerfs.branches /workspace
# 期望输出片段:
#   user.mergerfs.branches="/workspace-hot=RW:/workspace-cold=RW"
```

`RW` 表示读写、`NC` 表示不创建、`RO` 表示只读。Phase 32 D-17 sync_lock 触发时，secondary client 会看到 hot 分支被降级为 `RO`（详见 `SESSION_SYNC_LOCKED`）。

### 4.3 hot 50MB 白名单与拒绝

REQ-F1-D 决策：单个写入 hot 的目录体积 > 50MB 时立即拒绝并降级 sshfs-only，触发错误码 `MOUNT_MUTAGEN_WHITELIST_REJECT`。设计意图见 `internal/claudedock/errcodes/explanations.go::MOUNT_MUTAGEN_WHITELIST_REJECT`：避免初始化扫描雪崩拖垮 BASE-02 首连基线。

---

## 5. 故障排查

### 5.1 Mutagen 数据卷撑满 → 容器写失败

事件：`docker exec <container> df -h /` 显示 rootfs 接近满；用户在容器内 `npm install` 报 `ENOSPC`。

排查步骤：

1. `docker exec <container> du -sh /var/lib/mutagen /workspace /workspace-hot`
2. 如果是 mutagen 占大头：按 §3.2 清理 sessions
3. 如果是 hot 分支撑爆 50MB 白名单：检查 `claudedock doctor mount` 是否已经报 `MOUNT_MUTAGEN_WHITELIST_REJECT`，按提示在仓库根加 `.mutagen.yml` ignore 大目录
4. 如果是用户文件：进容器 `du -sh /workspace/* | sort -h` 找大目录清理

### 5.2 mergerfs branches 参数漂移

事件：`getfattr -n user.mergerfs.branches /workspace` 输出的 `category.create` 不是 `ff`，或缺少 `cache.attr=30`。

排查步骤：

1. 容器版本：`docker exec <container> cat /etc/claudedock/image.version` 应为 v3.0.0+
2. mount table：`docker exec <container> mount | grep mergerfs` 看实际选项
3. 重启容器触发 entrypoint 重建：`docker restart <container>`
4. 仍漂移 → admin recreate（保留 `claude-state` volume）

### 5.3 用户在 hot 写入超过 50MB 被拒

事件：用户 `git clone` 大仓库后立刻看到 stderr 提示 `MOUNT_MUTAGEN_WHITELIST_REJECT`，文件落地但同步未启用。

排查步骤：

1. 确认大目录：错误消息会含 `当前最大子目录: <path>`
2. 在仓库根 `.mutagen.yml` 添加 ignore 规则（参考 `internal/claudedock/errcodes/explanations.go::MOUNT_MUTAGEN_WHITELIST_REJECT` 的「修复路径」）
3. 重新连接 claudedock 触发重新评估
4. 如果业务确需把大仓库纳入热同步：评估升级到 v3.1（hot 容量可调，列入 §7 deferred）

---

## 6. 快速诊断命令

```bash
docker inspect <container> --format '{{range .Mounts}}{{.Type}} {{.Source}} -> {{.Destination}}{{"\n"}}{{end}}'
docker exec <container> mount | grep -E 'mergerfs|fuse|sshfs'
docker exec <container> getfattr -n user.mergerfs.branches /workspace
docker exec <container> du -sh /var/lib/mutagen /workspace /workspace-hot
claudedock doctor mount --json | jq '.checks[] | select(.domain=="mount")'
```

---

## 7. Deferred（v3.1 backlog）

| 项 | 描述 | 关联 |
|----|------|------|
| `claude-state` volume 自动备份 | 把 `/var/lib/docker/volumes/claude-state-*` 周期性 tar 到对象存储；按 account_id 选择性恢复 | 数据保护 |
| hot 容量可调 | 50MB 白名单写死在代码，v3.1 改为 admin 可调（默认 50MB，最高 500MB）+ 性能基线 ≤ 12s 兜底 | REQ-F1-D 后续演进 |
| Mutagen 数据卷独立 GC | 当前依赖容器重启释放，v3.1 增 `claudedock admin hosts mutagen-gc` 子命令支持在线清理 | M16 |

---

## 8. 参考

- `internal/runtime/tasks/worker.go::createHost` — 容器创建时 named volume 绑定逻辑（仅 claude-state）
- `internal/runtime/tasks/worker.go::ensureDockerVolume` — 幂等 volume 创建
- `deploy/docker/managed-user/entrypoint.sh::prepare_persistent_state` — `~/.claude` 软链与 1000:1000 chown
- `deploy/docker/managed-user/entrypoint.sh` — mergerfs / sshfs / mutagen 三路挂载初始化
- `internal/claudedock/errcodes/mount.go` — 全部 `MOUNT_*` 错误码定义
- `internal/claudedock/errcodes/disk.go` — 三档 disk 警戒线
- `docs/runbooks/v3-claude-state-volumes.md` — claude-state volume 命名规范 / 生命周期 / audit 事件清单 / 孤儿审计脚本
- `docs/runbooks/v3-doctor-troubleshoot.md` §3.4-3.5 — doctor mount / disk 维度排障
- `docs/runbooks/v3-error-code-index.md` — 错误码全集
