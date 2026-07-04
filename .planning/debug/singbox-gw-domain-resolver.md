---
status: resolved
trigger: "sing-box gateway 升级 v1.13.3 后启动失败：FATAL missing route.default_domain_resolver / domain_resolver in dial fields。根因初判是 internal/network/gateway_singbox_config.go 的 buildGatewaySingBoxConfig 未在 route 块设置 default_domain_resolver，sing-box 1.12+ deprecation 在 1.13 升级为 FATAL，导致 gateway 容器启动后立即崩溃，下游连锁产生 'No such container: claudedock-gw-*' 和 'open /etc/sing-box/config.json: no such file or directory'（后者是 teardownGateway 删除配置目录的副作用）。修复方向：在 route 中增加 default_domain_resolver={\"server\":\"dns-local\"}，并在 gateway_singbox_config_test.go 中补断言。"
created: 2026-05-14T00:00:00Z
updated: 2026-05-14T00:00:00Z
---

## Current Focus

hypothesis: internal/network/gateway_singbox_config.go:46-51 的 route map 缺 `default_domain_resolver` 字段，sing-box 1.13.3 启动时遇到含 dial fields 的 outbound（direct / proxy-out）会触发 FATAL `missing route.default_domain_resolver or domain_resolver in dial fields`，gateway 容器立即退出，被 teardownGateway 清理 → 表面伴生症状是 'No such container' 与 'config.json no such file or directory'。
test: 在 `route` 块加入 `default_domain_resolver = {"server": "dns-local"}`，重启 host 启动流程，验证 claudedock-gw-* 容器能持续运行 + tun0 就绪，且 host start_host action 不再失败。
expecting: gateway 容器成功启动且 host 进入运行态；测试套件中 gateway_singbox_config_test.go 增加 default_domain_resolver 断言并通过。
next_action: 已完成补丁；待手工 start_host 端到端验证。

## Symptoms

expected:
- 管理后台执行 start_host 后 gateway 容器（claudedock-gw-*）能稳定运行
- worker 容器接入隔离网络，host 状态进入运行中
- sing-box 在 1.13.3 下不报 FATAL，tun0 在超时窗口内就绪

actual:
- start_host 失败：`prepare gateway before create: gateway sing-box failed (matched "FATAL")`
- gateway 容器进入失败/被清理状态：`No such container: claudedock-gw-036cd10b-04b3-4cdc-af5c-d21ff6a5203b`
- 配置文件丢失副作用：`read config at /etc/sing-box/config.json: open /etc/sing-box/config.json: no such file or directory`
- 重试时再次出现根因：`gateway container tun0 not ready in time (last=container not running: <nil>): ERROR[0000] missing 'route.default_domain_resolver' or 'domain_resolver' in dial fields is deprecated in sing-box 1.12.0 and will be removed in sing-box 1.14.0 ... FATAL[0000] to continuing using this feature, set environment variable ENABLE_DEPRECATED_MISSING_DOMAIN_RESOLVER=true`
- teardown 副作用日志：`teardown disconnect control-plane from isolated network failed: exit status 1`、`teardown disconnect worker from isolated network failed: exit status 1`、`teardown remove isolated network failed: exit status 1`

errors:
- 主要：`FATAL missing route.default_domain_resolver or domain_resolver in dial fields`（sing-box v1.13.3）
- 副作用：`No such container: claudedock-gw-*`
- 副作用：`open /etc/sing-box/config.json: no such file or directory`
- 副作用：`teardown disconnect / remove isolated network failed: exit status 1`

reproduction:
- 升级 sing-box gateway 镜像到 v1.13.3（与 deploy/docker/sing-box-gateway/Dockerfile、docker-compose.yml、claude-shell/docker/Dockerfile、internal/controlplane/http/admin_egress_ip_probe.go:25-27 中固定的版本一致）
- 在管理后台对任意 host 执行 start_host
- 观察 claudedock-gw-<host_id> 容器日志立即出现上述 FATAL，host action 失败

started: 升级 sing-box gateway 镜像到 v1.13.3 后立即出现；先前 1.11.x 版本下 deprecation 仅以 warning 形式存在，不阻塞启动。

## Eliminated

- 假设："需要给 direct / proxy-out outbound 各自加 domain_resolver 作为防御层"。结论：暂不补。`extractProxyServer`（internal/network/outbound_parse.go）保证 PrepareGateway 调用链上 server 字段必为已 resolve 的 IPv4，proxy-out 不会出现域名 dial。direct outbound 的 dial fields 域名解析（sniff 出的目标域名）由顶层 `route.default_domain_resolver` 兜底覆盖。多层重复声明会引入额外配置面但无明确收益。

## Evidence

- timestamp: 2026-05-14T02:30:02Z | source: control-plane log | finding: start_host 失败 `prepare gateway before create: gateway sing-box failed (matched "FATAL")`，claudedock-gw-* 容器消失
- timestamp: 2026-05-14T02:31:00Z | source: control-plane log | finding: 直接拿到 sing-box 原文 FATAL：`missing route.default_domain_resolver or domain_resolver in dial fields is deprecated in sing-box 1.12.0 ... set environment variable ENABLE_DEPRECATED_MISSING_DOMAIN_RESOLVER=true`
- timestamp: 2026-05-14T02:30:02Z | source: control-plane log | finding: 后续 teardown 链路失败：disconnect control-plane / worker / remove isolated network 均 `exit status 1`，与 gateway 缺位互为因果
- timestamp: 2026-05-14 | source: 代码 `internal/network/gateway_singbox_config.go:46-51` | finding: 当前 `route` map 只有 `default_interface / rule_set / rules / final` 四字段，无 `default_domain_resolver`，且 direct/proxy-out outbound 均未设置 `domain_resolver`
- timestamp: 2026-05-14 | source: 代码 `internal/network/container_proxy_provider.go:278-281` | finding: `teardownGateway` 在 PrepareGateway 失败回滚时 `os.RemoveAll(GatewayConfigDir)`，解释为何后续日志再看到 `config.json not found`
- timestamp: 2026-05-14 | source: 项目研究文档 `.planning/milestones/v1.1-phases/08-singboxprovider/08-RESEARCH.md`、`.planning/milestones/v3.5-phases/45-net-foundation/45-REVIEW.md` | finding: 既有研究笔记已记录 sing-box 1.11+ 对 DoH outbound 的 domain_resolver 强约束，但顶层 route.default_domain_resolver 兜底未落地
- timestamp: 2026-05-14 | source: 代码 `internal/network/outbound_parse.go:12-34` | finding: `extractProxyServer` 在 PrepareGateway 链路上会把 server 字段强制 resolve 为 IPv4，否则报错并阻断启动 → 排除 proxy-out 出现域名 dial 的可能，因此无需在 outbound 层重复声明 `domain_resolver`
- timestamp: 2026-05-14 | source: 代码 `internal/network/gateway_singbox_config.go:46-66` 补丁后 | finding: `route` 中新增 `default_domain_resolver={"server":"dns-local"}`，对象形式与 sing-box 1.14 schema 兼容
- timestamp: 2026-05-14 | source: 测试 `go test ./internal/network/...` | finding: 全部通过；新增 `TestBuildGatewaySingBoxConfig_DefaultDomainResolver` 断言 `route.default_domain_resolver.server=dns-local` 且引用的 tag 在 `dns.servers` 中已声明

## Resolution

root_cause: `buildGatewaySingBoxConfig` 渲染 sing-box 配置时未设置 `route.default_domain_resolver`。sing-box 1.13.0 将该缺失从 deprecation 升级为 FATAL，gateway 容器启动即崩，控制面 PrepareGateway 回滚 `teardownGateway` 删除 `GatewayConfigDir` 与隔离网络资源，下游 `No such container` / `config.json not found` / `teardown ... exit status 1` 全是同一根因的伴生症状。
fix: 在 `internal/network/gateway_singbox_config.go` 的 `route` map 中加入 `default_domain_resolver = {"server": "dns-local"}`（对象形式，兼容 1.14 schema；dns-local 已在 `dns.servers` 声明）。未给 direct / proxy-out 重复声明 `domain_resolver`，理由：`extractProxyServer` 保证 proxy-out 的 server 必为 IPv4，direct 的 dial fields 由顶层兜底已经语义覆盖。
verification:
  - `go test ./internal/network/...` 全绿；新增 `TestBuildGatewaySingBoxConfig_DefaultDomainResolver` 通过
  - `go vet ./internal/network/...` 无 warning
  - `go build ./...` 通过
  - 待手工验证：管理后台 start_host → 观察 claudedock-gw-<host_id> 持续运行 + tun0 就绪
files_changed:
  - internal/network/gateway_singbox_config.go（补 `route.default_domain_resolver`）
  - internal/network/gateway_singbox_config_test.go（新增 `TestBuildGatewaySingBoxConfig_DefaultDomainResolver`）
