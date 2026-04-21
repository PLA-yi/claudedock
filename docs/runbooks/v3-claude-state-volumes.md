# Claude Code 状态持久化 Volume 运维手册（v3.0+）

> 适用版本：v3.0 起；对应阶段 Phase 33（claude-code-cli-admin-gc）
> 关联需求：REQ-F7-A（命名规范）/ REQ-F7-B（容器重建保留 OAuth）/ REQ-F7-D（admin DELETE 事务联动）

---

## 1. 背景

Phase 33 引入了基于 **Docker named volume** 的 Claude Code 状态持久化机制：

- 每个 `claude_account` 对应一个独立 volume，命名为 `claude-state-<claude_account_id>`
- 容器内通过 `/var/lib/claude-persist` mount target 挂载，并由 entrypoint 把
  `~/.claude` 与 `~/.cache/claude` 通过 `ln -sfn` 重定向到该挂载点
- 控制面在 worker `createHost` 阶段自动调 `docker volume create`（幂等）
- 控制面在 admin DELETE 阶段事务联动 host-agent 删除 volume

效果：**容器 stop / start / rebuild 后，OAuth credentials 与 Claude 缓存都不会丢失**。

---

## 2. 命名规范（REQ-F7-A / D-01 / D-02）

| 项 | 规范 | 示例 |
|----|------|------|
| Volume 名 | `claude-state-{claude_account_id}` | `claude-state-<account_id>` |
| 必带 label | `com.cloud-cli-proxy.account_id=<uuid>` | 唯一性键 |
| 必带 label | `com.cloud-cli-proxy.managed=true` | 二级保险，可批量过滤 |
| Mount target | `/var/lib/claude-persist` | 容器内统一路径 |

> account_id 使用 UUID 原格式（含连字符），不做截断或哈希。

---

## 3. 生命周期

### 3.1 创建

由 worker `createHost` 自动触发：

1. 收到 `HostActionRequest{ClaudeAccountID: "<uuid>", ...}`
2. `BuildClaudeStateVolumeName(<uuid>)` 计算 volume 名
3. `ensureDockerVolume` 幂等创建（`docker volume create --label ...`）
4. 自动追加 `VolumeMount{Name: "<vol>", Target: "/var/lib/claude-persist"}` 到 host create 参数
5. `Repository.UpsertClaudeAccountPersistentVolumeName` 写库（NULL→写入 / 一致跳过 / 冲突错误）
6. 失败写 audit `claude_account.volume_create_failed` 或 `claude_account.volume_name_persist_failed`

### 3.2 挂载

```
docker create \
  --mount type=volume,src=claude-state-<id>,dst=/var/lib/claude-persist \
  ...
```

### 3.3 删除（强一致 — 推荐默认）

调用：

```bash
curl -X DELETE \
  -H "Authorization: Bearer <admin-jwt>" \
  https://<host>/v1/admin/claude-accounts/<account_id>
```

行为（D-18）：

1. `BEGIN`
2. `SELECT id, persistent_volume_name FROM claude_accounts WHERE id = $1 FOR UPDATE`
3. 调 host-agent `ActionVolumeRemove`（10s 超时）
4. 成功 → `DELETE FROM claude_accounts ...` → `COMMIT` → audit `claude_account.deleted` → HTTP 200
5. 失败 → `ROLLBACK` → audit `claude_account.delete_volume_rm_failed` → HTTP 409 + 错误码 `STATE_VOLUME_IN_USE_001`

**409 响应体示例：**
```json
{
  "error": {
    "code": "STATE_VOLUME_IN_USE_001",
    "message": "请先停止使用该账号的所有 host 后重试，或追加 ?force=true 强删 volume",
    "next_action": "停止 host → 重试 DELETE，或附加 ?force=true"
  }
}
```

### 3.4 删除（最终一致 — `?force=true`）

调用：

```bash
curl -X DELETE \
  -H "Authorization: Bearer <admin-jwt>" \
  "https://<host>/v1/admin/claude-accounts/<account_id>?force=true"
```

行为（D-19）：

1. DB 先 `BEGIN → SELECT FOR UPDATE → DELETE → COMMIT`（30s 超时）
2. 后调 host-agent `ActionVolumeRemove` with `Labels:{"force":"true"}` （worker 走 `docker volume rm -f`）
3. rm 失败 → 仅写 audit `claude_account.force_volume_rm_failed` → 仍返回 HTTP 200，body 含
   `"volume_rm":"failed"` + `"next_action":"运维需手工 docker volume rm -f <name>"`

> **运维必关注 audit 事件**：`claude_account.force_volume_rm_failed` 出现意味着 DB 已删但 volume
> 残留，需要按 §5 孤儿审计脚本清理。

接受的 `force` 值（query 解析）：`true` / `1` / `yes`，其它（包括 `TRUE` / `false` / 空）均按非 force 处理。

---

## 4. Audit 事件清单 + Metadata 白名单

| 事件类型 | 触发场景 | Metadata key |
|----------|---------|--------------|
| `claude_account.deleted` | 任一路径成功删除 | account_id, volume_name, force |
| `claude_account.delete_volume_rm_failed` | 强一致路径 host-agent rm 失败 → ROLLBACK | account_id, volume_name, error_code, error_message |
| `claude_account.force_volume_rm_failed` | force 路径 host-agent rm 失败（DB 已删） | account_id, volume_name, error_message |
| `claude_account.volume_create_failed` | worker createHost ensureDockerVolume 失败 | account_id, volume_name |
| `claude_account.volume_name_persist_failed` | worker createHost UpsertClaudeAccountPersistentVolumeName 失败（冲突） | account_id, volume_name |
| `claude_account.volume_rm_failed` | worker removeVolumes 内层 docker rm 失败 | volume_name, force |

**Metadata 白名单（严守，T-33-12 mitigation）：** `account_id` / `volume_name` / `force` /
`host_id` / `error_code` / `error_message`。**永不写入** `email` / `entry_password` /
`credentials` / `oauth_token` 任一字段。

---

## 5. 孤儿 Volume 审计脚本（M16 兜底）

```bash
#!/usr/bin/env bash
# usage: bash audit-claude-state-volumes.sh

set -euo pipefail

# (a) 列出所有受管 claude-state-* volume
echo "=== Managed claude-state volumes (docker side) ==="
docker volume ls --filter label=com.cloud-cli-proxy.managed=true --format '{{.Name}}'

# (b) 与 DB 对比找出孤儿（DB 中无对应 account 但 docker 仍存在的 volume）
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

psql "$DATABASE_URL" -tAc "SELECT 'claude-state-' || id FROM claude_accounts" \
  | sort > "$TMPDIR/db-volumes.txt"
docker volume ls --filter label=com.cloud-cli-proxy.managed=true --format '{{.Name}}' \
  | grep '^claude-state-' | sort > "$TMPDIR/docker-volumes.txt"

echo
echo "=== Orphan volumes (in docker, not in DB) ==="
comm -23 "$TMPDIR/docker-volumes.txt" "$TMPDIR/db-volumes.txt"

echo
echo "=== Missing volumes (in DB, not in docker) ==="
comm -13 "$TMPDIR/docker-volumes.txt" "$TMPDIR/db-volumes.txt"
```

> 周期建议：每天 1 次（运维 cron）；输出非空时告警。
> v3.1 backlog 计划把它纳入独立 GC 定时任务（见 §7）。

---

## 6. 故障排查

### 6.1 admin DELETE 总是 409

事件：`claude_account.delete_volume_rm_failed`

排查步骤：

1. 取出 metadata.volume_name
2. `docker ps -a --filter volume=<volume_name>` 找出仍持有该 volume 的容器
3. 若容器仍 running：先停止容器再 retry DELETE；或者直接走 `?force=true`
4. 若容器已 stopped 但 volume 仍标记 in-use：`docker rm <stopped_container>` 释放引用

### 6.2 force=true 后 volume 残留

事件：`claude_account.force_volume_rm_failed`

排查步骤：

1. 取出 metadata.volume_name 与 metadata.error_message
2. 手工执行 `docker volume rm -f <volume_name>`
3. 若仍失败（"in use"）：`docker ps -a --filter volume=<vol>` 找占用容器 → `docker rm -f <container>` → 重试

### 6.3 worker createHost 时 volume 创建失败

事件：`claude_account.volume_create_failed`

排查步骤：

1. 检查 host-agent 与 docker daemon 连通性（`docker info` / `systemctl status docker`）
2. 检查 host-agent 是否有 docker socket 访问权限
3. 重新触发 host create（worker `createHost` 是幂等的）

### 6.4 volume_name_persist_failed（冲突）

事件：`claude_account.volume_name_persist_failed`

含义：DB 中 `claude_accounts.persistent_volume_name` 字段已被人工改过（值与 worker 计算值不一致）。
worker 不阻塞容器启动但留 audit 痕迹。需要人工修正：

```sql
SELECT id, persistent_volume_name FROM claude_accounts WHERE id = '<uuid>';
-- 如确认 worker 计算值更准确，回填规范名：
UPDATE claude_accounts SET persistent_volume_name = NULL WHERE id = '<uuid>';
-- 然后下次 worker 调度会重新写入正确值。
```

### 6.5 容器内 ~/.claude 不持久化

排查步骤：

1. `docker exec <ctr> readlink /home/claude/.claude` 应返回 `/var/lib/claude-persist/.claude`
   （未返回此值说明 entrypoint v3 stage 未执行）
2. 检查容器启动日志：`docker logs <ctr> | grep '\[entrypoint\] v3'` 应有
   `[entrypoint] v3: persistent state ready` 之类的输出
3. 检查 mount 是否就位：`docker inspect <ctr> | jq '.[0].Mounts'` 应包含 `claude-state-*` volume
4. 检查 owner：`docker exec -u root <ctr> stat -c "%U:%G %n" /home/claude/.claude /home/claude/.cache/claude`
   应输出 `claude:claude`（即 1000:1000）

---

## 7. Deferred 项（v3.1 backlog）

| 项 | 描述 | 关联 |
|----|------|------|
| 独立 GC 定时任务 | 把 §5 审计脚本扩展为 cron job（按 label + DB 反查），自动清理孤儿 volume | M16 |
| Volume 备份脚本 | tar `/var/lib/docker/volumes/claude-state-*` 到对象存储；支持按 account_id 选择性恢复 | 数据保护 |
| Label 一致性检测 | `ensureDockerVolume` 当前仅做存在性检查；v3.1 增 `docker volume inspect` 解析 + label 比对，不一致时输出 audit | RESEARCH §6.6 |
| Dispatcher 显式注入 ClaudeAccountID | Phase 29 RuntimeService.QueueHostAction 与 Phase 32 attach 链路目前未填充 ClaudeAccountID，导致 worker 自动补 volume 走 D-07 fallback | Plan 01 carry-over |

---

## 8. 参考

- `.planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md`
- `.planning/REQUIREMENTS.md` §F7
- `.planning/research/PITFALLS.md` M16 / M17
- `internal/runtime/tasks/worker.go`（BuildClaudeStateVolumeName / ensureDockerVolume / removeDockerVolume）
- `internal/controlplane/http/admin_claude_accounts.go`（admin DELETE handler）
- `internal/store/repository/queries.go`（Lock/DeleteClaudeAccountTx + UpsertClaudeAccountPersistentVolumeName）
