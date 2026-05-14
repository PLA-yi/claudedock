# logs/ —— 容器日志（Phase 52 OBS-01..03 收集）

## 里面是什么

`docker logs --tail 500 --timestamps <container>` 的输出，e2e 用例涉及的容器各一份。

命名规则：`<container-name>.log`，常见的几个：

- `cloudproxy-gw-<host_id>.log` — sing-box gateway 容器
- `cloudproxy-<host_id>.log` — worker 容器（用户 shell 环境）
- 控制面 / host-agent 进程在 Phase 52 时仍是宿主机子进程，**不**会出现在本目录；它们的 stderr 由 `system/` 内的 `wait-timeout.txt` 与 Go test stdout 捕获。

如果 `docker ps -a` 在采集时返回空（daemon 未起 / 无容器），本目录只会有一个 `_empty.txt` 占位。

## 采集命令

```bash
docker ps -a --format '{{.Names}}' | while read name; do
    docker logs --tail 500 --timestamps "$name" > "logs/${name}.log" 2>&1
done
```

## 典型排障场景

1. **sing-box gateway 启不来** → 看 `cloudproxy-gw-*.log` 末尾，常见错误：`outbound config invalid`、`tun device create failed`、`bind 1080 already in use`。
2. **worker 容器内 SSH 拒绝连接** → 看 `cloudproxy-<host_id>.log` 头部，确认 sshd 是否成功 `Server listening on 0.0.0.0 port 22`。
3. **gateway 健康检查抖动** → grep `level=error` / `WARN` 时间戳，比对 e2e 用例 `system/wait-timeout.txt` 的时间窗。
4. **host-agent 上报失败** → host-agent 不在容器里，看 `system/` 与 control-plane 进程 stderr；本目录无关。

## darwin 上为空怎么办

darwin 本地通常没起 Docker Desktop daemon，本目录大概率只剩 `_empty.txt` 占位。这是预期，不是 bug。

要在 darwin 上真实采集：

1. 起 Docker Desktop。
2. 跑一遍涉及 testcontainers 的 e2e 用例（`go test -tags=e2e ./tests/e2e/... -run TestE2ESmoke`）。
3. 再手动 `bash tests/e2e/harness/collect-artifacts.sh ./out scenario_xyz`。

## 隐私守护

`docker logs` 输出可能带容器内进程的环境变量 / 命令行参数；e2e fixture 用 `secret-placeholder-pw` / `test@example.com` 等占位凭据，不引入真实敏感数据。

如未来 phase 引入真实凭据，必须在 `tests/e2e/` 内**首先**改 fixture 使其占位化，**不要**靠下游过滤 `docker logs` 输出。
