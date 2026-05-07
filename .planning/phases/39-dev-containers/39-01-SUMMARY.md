---
phase: 39-dev-containers
plan: 01
status: complete
completed: 2026-05-07
commit: c41735a
---

# Plan 01: CLI local up 子命令 + internal/local 包 + entrypoint MODE=local

## What Was Built

创建了 `internal/local/` Go 包，实现本地容器生命周期管理；注册了 `cloud-claude local` cobra 子命令组；改造了 entrypoint.sh 支持 `MODE=local` 分支。

## Key Files

### Created
- `internal/local/local.go` — LocalManager 结构体，Up/Down/Status 方法，容器名生成，Docker create 参数构建
- `internal/local/container.go` — Docker 操作封装（DockerRunner 接口，containerExists，inspectSSHPort，inspectContainerStatus）
- `internal/local/password.go` — crypto/rand 安全密码生成
- `internal/local/local_test.go` — 完整单元测试（GeneratePassword, LocalContainerName, BuildCreateArgs, InspectSSHPort, ContainerExists, InspectContainerStatus）
- `cmd/cloud-claude/local.go` — cobra local 子命令组（up/down/status），runLocalUp/runLocalDown/runLocalStatus 实现

### Modified
- `cmd/cloud-claude/main.go` — 注册 newLocalCmd()，DisableFlagParsing 添加 "local" case
- `deploy/docker/managed-user/entrypoint.sh` — 添加 MODE 变量检测，KasmVNC + 桌面栈 + v3 stages 用 `if [ "$MODE" != "local" ]` 包裹，添加 sing-box 占位段

## Decisions

- 使用 `DockerRunner` 函数类型（而非 Go Docker SDK）保持与 worker.go 模式一致
- `localContainerName` 使用 MD5(绝对路径)[:4] 确保不同项目不冲突
- `local` 命令默认行为 = `local up`（localCmd.RunE 直接调用 runLocalUp）
- entrypoint MODE 检查放在 /dev/fuse 之后、KasmVNC 配置之前

## Test Results

所有 17 个单元测试通过（0.4s）：
- TestGeneratePassword: 7 cases
- TestLocalContainerName: 3 cases
- TestBuildCreateArgs: 9 cases
- TestBuildCreateArgsWithPort: 1 case
- TestInspectSSHPort: 2 cases
- TestContainerExists: 2 cases
- TestInspectContainerStatus: 2 cases
