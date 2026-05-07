---
phase: 39-dev-containers
plan: 02
status: complete
completed: 2026-05-07
commit: bdb6904
---

# Plan 02: egress config 注入 + 协议检测

## What Was Built

创建了 `internal/local/egress.go` 模块，实现 sing-box outbound JSON 的验证、协议检测和容器注入逻辑。集成了 egress config 到 LocalManager.Up 方法中。

## Key Files

### Created
- `internal/local/egress.go` — EgressMode 类型、DetectEgressMode（socks/http→proxy, 其他→tun）、ValidateEgressConfig（文件验证+JSON校验+模式检测）、egressMountArg

### Modified
- `internal/local/local.go` — Up 方法中集成 egress config 验证和注入（--cap-add NET_ADMIN for tun, -v mount, -e SING_BOX_MODE）
- `internal/local/local_test.go` — 新增 TestDetectEgressMode（8 cases）、TestValidateEgressConfig（4 cases）、TestEgressMountArg、TestBuildCreateArgsWithEgress、TestBuildCreateArgsWithProxyEgress

## Decisions

- tun 模式追加 `--cap-add NET_ADMIN` 和 `--device /dev/net/tun`（仅当协议类型需要 tun）
- proxy 模式不需要额外权限（SOCKS/HTTP 代理在用户空间运行）
- egress config 文件以只读模式挂载到容器内固定路径 `/etc/cloud-claude/sing-box-outbound.json`
- 通过 `-e SING_BOX_MODE=tun|proxy` 传递模式信息给 entrypoint

## Test Results

所有 30 个单元测试通过（0.4s）：
- 新增 13 个 egress 相关测试
- 原有 17 个测试无回归
