---
phase: 51-qual-harden
plan: 51-04
status: completed
completed_at: 2026-05-14
---

# 51-04 SUMMARY — `GetContainerNetNS` 探测窗口参数化

## 落地清单

- `internal/network/namespace.go`：抽出 `nsConfig` 私有结构、`Option` 类型 +
  `WithProbeWindow` / `WithMaxRetries` 构造器、`defaultNsConfig()`。`GetContainerNetNS`
  签名改为可变参数 `opts ...Option`，4 个调用点零修改（默认值生效）。
- `internal/network/namespace_options_test.go`（新，linux build tag）：6 个
  Option 单测覆盖 default / positive / 零负数被忽略 / 组合应用。

## 验证

- `go build ./...` PASS。
- `GOOS=linux go build ./...` PASS。
- `go vet ./...` + `GOOS=linux go vet ./...` PASS。
- `go test ./... -count=1` 全绿。

## 偏差

- 单测文件带 `//go:build linux` 标签（与 namespace.go 一致），darwin 上自动 skip；
  darwin 侧通过跨编译验证。
