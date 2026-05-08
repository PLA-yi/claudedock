---
plan: 41-01
status: complete
completed: 2026-05-08
commits:
  - hash: 72d3b6a
    message: "feat(41-01): register remote-ssh error codes and explain coverage"
  - hash: a4de2eb
    message: "feat(41-01): add remote-ssh doctor dimension with 5 checks"
---

# Phase 41 Plan 01 Summary: remote-ssh 维度 + 错误码 + explain

## What Was Built

新增 `remote-ssh` doctor 维度，包含 5 项远端检查覆盖 VS Code Remote-SSH 场景诊断；注册 6 个新错误码（含 4 条长说明 + 2 条 ExplainExempt）。

## Key Files

### Created
- `internal/cloudclaude/errcodes/remote_ssh.go` — 6 条 MustRegister 注册
- `internal/cloudclaude/doctor/remote_ssh.go` — 5 个 check 函数

### Modified
- `internal/cloudclaude/errcodes/codes.go` — 6 个新 Code 常量
- `internal/cloudclaude/errcodes/explanations.go` — 4 条 registerExplanation + 2 条 ExplainExempt
- `internal/cloudclaude/doctor/doctor.go` — RunDoctor() 插入 remote-ssh 维度块
- `cmd/cloud-claude/doctor.go` — ValidArgs 加入 remote-ssh，描述更新

## Self-Check: PASSED

- [x] 6 个新 Code 常量声明
- [x] 6 条 MustRegister 调用
- [x] 4 条 registerExplanation 调用
- [x] 2 个 Info 码在 ExplainExempt 中
- [x] remote_ssh.go 编译通过
- [x] doctor.go 集成正确
- [x] cobra ValidArgs 包含 remote-ssh
- [x] go vet 无错误
- [x] go test ./internal/cloudclaude/doctor/... 通过
- [x] go test ./internal/cloudclaude/errcodes/... 通过

## Deviations

无偏差。
