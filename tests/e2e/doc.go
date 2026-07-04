//go:build e2e

// Package e2e 是 ClaudeDock 端到端测试根包。
//
// 设计原则：
//   - 所有 e2e 测试文件统一带 //go:build e2e 构建标签，
//     默认 `go test ./...` 不会触发本目录下任何用例；
//     必须显式 `go test -tags=e2e ./tests/e2e/...` 才会执行。
//   - 通用 helper（Scenario builder / waitFor / artifact dump）
//     统一收敛到 tests/e2e/harness 子包，避免每个用例文件重复样板。
//   - testcontainers-go 负责容器编排，testify/suite 负责生命周期。
//
// 引入历史：Phase 45 Plan 01（v3.6 milestone）。
package e2e
