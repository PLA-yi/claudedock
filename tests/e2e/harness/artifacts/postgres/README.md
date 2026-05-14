# postgres/ —— Postgres 状态（Phase 52 OBS-01..03 收集）

## 里面是什么

控制面 Postgres 的**非敏感**状态快照：

- `schema.sql` — `pg_dump --schema-only --no-owner --no-privileges` 输出（仅表结构，**不含**行数据）
- `users.tsv` — `users` 表的 `id / username / status` 三列前 50 行（不含 password_hash / token）
- `host-egress-bindings.tsv` — `host_egress_bindings` 表的 `id / host_id / egress_ip_id / created_at` 前 50 行
- `events.tsv` — `events` 表的 `id / type / created_at` 最新 100 行（按 id 倒序）

`schema.err` / `users.err` 等 `.err` 文件是对应 pg_dump / psql 的 stderr。

如未设 `PG_DUMP_URL` 或 `DATABASE_URL` 环境变量、或本机无 `pg_dump`/`psql`，本目录只剩 `_skipped.txt` 占位。

## 采集命令

```bash
url="${PG_DUMP_URL:-${DATABASE_URL}}"
pg_dump --schema-only --no-owner --no-privileges "$url" > postgres/schema.sql
psql "$url" -At -F$'\t' \
    -c "select id, username, status from users limit 50" \
    > postgres/users.tsv
psql "$url" -At -F$'\t' \
    -c "select id, host_id, egress_ip_id, created_at from host_egress_bindings limit 50" \
    > postgres/host-egress-bindings.tsv
psql "$url" -At -F$'\t' \
    -c "select id, type, created_at from events order by id desc limit 100" \
    > postgres/events.tsv
```

## 典型排障场景

1. **用户被标记为 expired（Phase 47 MVS-06）** → 看 `users.tsv` 的 status 列；同时看 `events.tsv` 是否含 `user.expired` / `host.stop.expired` 类型。
2. **出口 IP 双绑互斥（Phase 51 D-47-3 修复后）** → 看 `host-egress-bindings.tsv` 是否同一 `egress_ip_id` 对应多行 `host_id`（应该不会，Phase 51-09 加了 pre-check）。
3. **心跳事件丢失（Phase 47 MVS-08）** → 看 `events.tsv` 最新 100 行是否含 `host.heartbeat.*` 类型；间隔是否符合 30s 心跳契约。
4. **schema 漂移** → 比对 `schema.sql` 与最新 migration 版本（`internal/store/migrations/*.sql`）。
5. **PG connection refused** → 看 `*.err` 文件，确认 testcontainer PG 是否就绪。

## 隐私守护（重要）

**本目录永远不导出**：

- `users.password_hash` / `users.entry_password`
- `users.email`（如未来引入）
- `admin_tokens.*`
- 任何 `*_secret` / `*_token` / `*_password` 列

采集脚本明确白名单字段，schema dump 用 `--schema-only` 不带行数据。

未来 phase 如新增字段需要纳入采集，**必须**在 PLAN 中显式列出字段名 + 评估隐私后再加入；不允许「select * from ...」式宽采集。

## darwin 上为空怎么办

本地没起 e2e 控制面（即没有跑 `go test -tags=e2e ./tests/e2e/...` 触发 testcontainer PG 启动）→ `DATABASE_URL` 没设 → 目录只剩 `_skipped.txt`。

要在 darwin 上看到真实输出：

```bash
# 起 testcontainer PG（不跑用例，只起容器）→ 手动 export DATABASE_URL
docker run --rm -d -p 5432:5432 \
    -e POSTGRES_PASSWORD=your-secret-here \
    -e POSTGRES_USER=postgres \
    -e POSTGRES_DB=cloud_cli_proxy_e2e \
    --name pg-debug postgres:18
export DATABASE_URL="postgres://postgres:your-secret-here@127.0.0.1:5432/cloud_cli_proxy_e2e?sslmode=disable"
bash tests/e2e/harness/collect-artifacts.sh ./out scenario_xyz
```

注意：`pg_dump` / `psql` 在 macOS 上通常通过 `brew install postgresql` 或 `brew install libpq` 获取。
