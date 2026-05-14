---
phase: 51-qual-harden
plan: 51-08
status: completed
completed_at: 2026-05-14
---

# 51-08 SUMMARY — `goleak.VerifyTestMain` 接入

## 落地清单

- `go.mod` / `go.sum`：新增 `go.uber.org/goleak v1.3.0`（本里程碑唯一允许的新依赖）。
- 3 个 `testmain_test.go` 文件：
  - `cmd/cloud-claude/testmain_test.go`
  - `internal/network/testmain_test.go`
  - `internal/controlplane/app/testmain_test.go`

## IgnoreList（首跑实测）

- `github.com/zanel1u/cloud-cli-proxy/internal/broadcast.(*Hub).cleanupLoop`
  —— SSE Hub 包级 init 启动的清理 goroutine，生命周期与进程绑定，属于设计内
  合法常驻。三个 testmain 共享同一条 ignore。

## 验证

- `go test ./cmd/cloud-claude/...` PASS。
- `go test ./internal/network/...` PASS。
- `go test ./internal/controlplane/app/...` PASS。
- `go test ./... -count=1` 全绿（19 个包）。
- `GOOS=linux go build ./...` PASS。

## 偏差

- CONTEXT 提及「control-plane / host-agent / cloud-claude 三主 package」。本仓库
  实际无独立 `cmd/host-agent` 目录（host-agent 以 embedded 模式跑在 control-plane
  进程内），因此本 plan 把 goleak 接入到了：
  - `cmd/cloud-claude`（主二进制）
  - `internal/controlplane/app`（控制面核心包，等价 control-plane 服务）
  - `internal/network`（sing-box / nft 涉及大量 goroutine 的核心包）
  这是按源码真相的最小化合理偏差。
- `cmd/control-plane` 无 test 文件，不接入。
