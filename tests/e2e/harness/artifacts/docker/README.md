# docker/ —— Docker 元信息（Phase 52 OBS-01..03 收集）

## 里面是什么

Docker daemon 当前状态快照：

- `ps.txt` — `docker ps -a`（含 STOPPED / EXITED 状态）
- `network-ls.txt` — `docker network ls`（所有 docker 网络，含 `cloudproxy-net-*`）
- `inspect-<name>.json` — 每个容器的 `docker inspect <name>` 完整输出

## 采集命令

```bash
docker ps -a > docker/ps.txt 2>&1
docker network ls > docker/network-ls.txt 2>&1
docker ps -a --format '{{.Names}}' | while read name; do
    docker inspect "$name" > "docker/inspect-${name}.json" 2>&1
done
```

## 典型排障场景

1. **容器没启动** → 看 `ps.txt` 的 STATUS 列；`Exited (137)` 通常是 OOM 或 SIGKILL，`Exited (1)` 是用户程序失败。
2. **容器跑在错的网络上（Phase 50 KILL-04）** → 看 `inspect-<name>.json` 的 `NetworkSettings.Networks` 字段，确认 `cloudproxy-net-<host_id>` 在不在。
3. **OOM / RestartLoop** → 看 `inspect-<name>.json` 的 `State.OOMKilled` 与 `State.RestartCount`。
4. **进程 capability 漂移（Phase 49 LEAK-08 / Phase 51 QUAL-06）** → 看 `inspect-<name>.json` 的 `HostConfig.CapAdd` / `CapDrop`，对照 `--cap-drop NET_RAW`、`--cap-add NET_ADMIN` 是否符合预期。
5. **挂载点漂移** → 看 `inspect-<name>.json` 的 `Mounts` 字段，确认 resolv.conf 是否 ro bind mount（Phase 48 MVS-10）。

## darwin 上为空怎么办

- Docker Desktop 没启动 → `ps.txt` / `network-ls.txt` 会含 "Cannot connect to the Docker daemon at unix://..." 错误占位。
- Docker Desktop 启动但无容器 → `ps.txt` 只有 header，inspect-*.json 不会生成。

这都是预期。要在 darwin 上看到真实容器，先跑 `go test -tags=e2e ./tests/e2e/...` 起 testcontainer 后再采集。

## 隐私守护

`docker inspect <name>.json` 的 `Config.Env` 字段可能带容器启动时的环境变量。e2e fixture 用 placeholder 凭据，不引入真实敏感数据；如未来 phase 引入真实凭据，请先 fixture 占位化。
