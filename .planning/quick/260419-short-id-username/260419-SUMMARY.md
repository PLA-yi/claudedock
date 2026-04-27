---
phase: quick
plan: 260419-short-id-username
subsystem: auth
tags:
  - username-auth
  - ssh-proxy
  - private-key
  - entry-api
  - cloud-claude-cli
dependency_graph:
  requires: []
  provides:
    - AUTH-SHORT-ID-REMOVAL
    - AUTH-USERNAME-SSH
    - AUTH-PRIVATE-KEY-CONNECT
  affects:
    - internal/store/repository
    - internal/controlplane/http
    - internal/sshproxy
    - internal/cloudclaude
    - cmd/cloud-claude
tech_stack:
  added: []
  patterns:
    - "数据库存储的 SSH 私钥用于控制面到容器的公钥认证"
    - "username 替代 short_id 作为 SSH 登录标识"
key_files:
  created:
    - internal/sshproxy/resolver_test.go
  modified:
    - internal/store/repository/models.go
    - internal/store/repository/queries.go
    - internal/controlplane/http/entry.go
    - internal/controlplane/http/entry_auth_test.go
    - internal/controlplane/http/auth_handler.go
    - internal/controlplane/http/router.go
    - internal/sshproxy/resolver.go
    - internal/sshproxy/proxy.go
    - internal/cloudclaude/entry.go
    - internal/cloudclaude/config.go
    - internal/cloudclaude/session.go
    - internal/cloudclaude/ssh.go
    - internal/cloudclaude/doctor/auth.go
    - internal/cloudclaude/doctor/fix.go
    - internal/cloudclaude/doctor/auth_test.go
    - internal/cloudclaude/doctor/fix_test.go
    - internal/cloudclaude/doctor/mount_test.go
    - cmd/cloud-claude/main.go
    - cmd/cloud-claude/sessions.go
decisions:
  - "HostSSHAuth 移除 HostShortID，新增 SSHPrivateKey 字段，控制面用数据库私钥连接容器"
  - "Entry API 路由从 /v1/entry/{shortId} 改为 /v1/entry/{username}，返回 ssh_user = username"
  - "SSH proxy resolver 按 username 查 host，resolveTarget 返回 User + PrivateKey"
  - "proxy.go handleChannel 识别 PEM 私钥并优先用 ssh.PublicKeys 认证，密码始终 fallback"
  - "cloud-claude CLI Config.ShortID 改为 Username，--short-id flag 改为 --username"
  - "SessionConfig.ShortID 改为 SessionID，与用户 short_id 彻底解耦"
  - "保留 GetHostByShortID / GetUserByShortID 做一期兼容 fallback，二期移除"
metrics:
  duration: "~25 min"
  completed_date: "2026-04-27"
---

# Phase quick Plan 260419: 统一用户名 + 公钥认证架构改造 Summary

**One-liner:** 去掉 short_id，容器内 SSH 用户改为 username，控制面用数据库存储的私钥通过公钥认证连接容器。

## 任务完成情况

| 任务 | 名称 | 提交 | 状态 |
|------|------|------|------|
| 1 | 数据层 + Entry API 改造 | 1c25127 | 完成 |
| 2 | SSH proxy 改造 | c1832bc | 完成 |
| 3 | cloud-claude CLI 改造 | 8bfc486 | 完成 |

## 变更摘要

### Task 1: 数据层 + Entry API

- **models.go**: `HostSSHAuth` 移除 `HostShortID`，新增 `SSHPrivateKey`。
- **queries.go**: `GetHostByShortID` 重命名为 `GetHostByUsername`，SQL WHERE 改为 `u.username = $1`，SELECT 追加 `COALESCE(u.ssh_private_key, '')`。保留 `GetHostByShortID` 做一期兼容。新增 `GetUserByUsername`。
- **entry.go**: `EntryStore` 接口方法名改；`Script()` 和 `Auth()` handler 路径参数从 `shortId` 改为 `username`；`Auth()` 返回 `"ssh_user": user.Username`。
- **auth_handler.go**: 移除 `ShortID` 请求字段和 `"short_id"` 响应字段，只用 `Username`。
- **router.go**: 路由注册改为 `/v1/entry/{username}`。
- **entry_auth_test.go**: stub 方法名同步改，所有测试用例改为 username 路径，断言 `ssh_user` 为 username。

### Task 2: SSH proxy

- **resolver.go**: `resolverRepo` 接口 `GetHostByShortID` 改为 `GetHostByUsername`；`ContainerTarget` 新增 `PrivateKey`；`ResolveContainer` / `ResolveContainerByPublicKey` 参数改为 `username`；`resolveTarget` 返回 `User` 和 `PrivateKey`。
- **proxy.go**: `ContainerResolver` 接口同步改；`PasswordCallback` / `PublicKeyCallback` 扩展字段增加 `target_private_key`；`handleChannel` 识别 PEM 私钥并优先用 `ssh.PublicKeys(signer)` 认证，密码始终作为 fallback 保留。
- **resolver_test.go**（新建）: 覆盖密码错误、公钥匹配/不匹配、无 inbound keys、username 参数传递等路径。

### Task 3: cloud-claude CLI + runtime

- **config.go**: `Config.ShortID` 改为 `Config.Username`，`Validate()` 检查 `Username`。
- **entry.go**: `Authenticate` / `AuthenticateAndWait` 参数名改为 `username`，URL 路径同步改。
- **main.go**: `--short-id` flag 改为 `--username`，`CLOUD_CLAUDE_SHORT_ID` 改为 `CLOUD_CLAUDE_USERNAME`，所有 `cfg.ShortID` 改为 `cfg.Username`。
- **sessions.go**: `AuthenticateAndWait` 传入 `cfg.Username`。
- **session.go**: `SessionConfig.ShortID` 改为 `SessionID`，与 user short_id 解耦。
- **ssh.go**: 透传 `SessionShortID` 到 `SessionConfig.SessionID`。
- **doctor/auth.go + fix.go**: 参数和调用同步改 username。
- **测试文件**: `auth_test.go`、`fix_test.go`、`mount_test.go` 中 config yaml 的 `short_id` 改为 `username`。

## 验证结果

- `go build ./...` 零错误
- `go test ./... -count=1` 全部 PASS（14 个测试包）
- 新增 `internal/sshproxy/resolver_test.go` 通过

## Deviations from Plan

**无偏差** — 计划按预期执行，未触发 Rule 1-4 偏差。

## 已知遗留

- `GetHostByShortID` / `GetUserByShortID` 保留在一期做兼容 fallback，二期计划移除。
- `User.ShortID` 字段在 models.go 中保留（二期再移除，避免一次改动过大）。

## Self-Check: PASSED

- [x] 所有修改文件存在
- [x] 所有提交存在（1c25127, c1832bc, 8bfc486）
- [x] `go build ./...` 成功
- [x] `go test ./...` 全部通过
