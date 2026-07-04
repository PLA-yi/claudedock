---
phase: 1
slug: foundation-control-plane-host-agent
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-03-26
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` + shell smoke scripts |
| **Config file** | `go.mod`（Wave 0 创建） |
| **Quick run command** | `go test ./...` |
| **Full suite command** | `go test ./... && bash scripts/verify-managed-image.sh && systemd-analyze verify deploy/systemd/claudedock-control-plane.service deploy/systemd/claudedock-host-agent.service` |
| **Estimated runtime** | ~120 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./...`
- **After every plan wave:** Run `go test ./... && bash scripts/verify-managed-image.sh && systemd-analyze verify deploy/systemd/claudedock-control-plane.service deploy/systemd/claudedock-host-agent.service`
- **Before `$gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 1-01-01 | 01 | 1 | RUNT-01 | static + compile | `rg -n "module github.com/zaneliu/claudedock|/healthz|/v1/tasks|RunMigrations" go.mod cmd/control-plane/main.go internal/controlplane/http/router.go internal/store/migrator/migrator.go && go test ./...` | ❌ W0 | ⬜ pending |
| 1-01-02 | 01 | 1 | RUNT-01 | schema + unit | `rg -n "CREATE TABLE users|CREATE TABLE hosts|CREATE TABLE tasks|CREATE TABLE events|pending|running|succeeded|failed|canceled|last_error_summary|ListTasksWithLastErrorSummary" internal/store/migrations/0001_initial.sql internal/store/repository/queries.go && go test ./...` | ❌ W0 | ⬜ pending |
| 1-01-03 | 01 | 1 | RUNT-01 | compose smoke | `docker compose -f deploy/compose/control-plane.dev.yml up -d postgres control-plane && docker compose -f deploy/compose/control-plane.dev.yml exec -T postgres psql -U claudedock -d claudedock -Atc "SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename IN ('users','hosts','egress_ips','host_egress_bindings','tasks','events') ORDER BY tablename" && curl --fail http://127.0.0.1:8080/healthz` | ❌ W0 | ⬜ pending |
| 1-02-01 | 02 | 1 | RUNT-02 | image content | `rg -n "FROM ubuntu:24.04|openssh-server|@anthropic-ai/claude-code|Port 22|sshd -D -e" deploy/docker/managed-user/Dockerfile deploy/docker/managed-user/sshd_config deploy/docker/managed-user/entrypoint.sh` | ❌ W0 | ⬜ pending |
| 1-02-02 | 02 | 1 | RUNT-02 | contract doc | `rg -n "image_name: ghcr.io/zaneliu/claudedock/managed-user:v0.1.0-phase1|pull_policy: never-implicit-latest|home_mount: /workspace|default_user: workspace|rebuild_mode_default: preserve-home|factory_reset_mode: wipe-/workspace" deploy/docker/managed-user/image.lock` | ❌ W0 | ⬜ pending |
| 1-02-03 | 02 | 1 | RUNT-01 | docker smoke | `bash scripts/verify-managed-image.sh` | ❌ W0 | ⬜ pending |
| 1-03-01 | 03 | 2 | RUNT-01 | task-state unit | `rg -n "HostActionRequest|/run/claudedock/host-agent.sock|pending|running|succeeded|failed|canceled|claudedock-|/var/lib/claudedock/hosts/|/workspace" cmd/host-agent/main.go internal/agentapi/contracts.go internal/runtime/tasks/worker.go && go test ./...` | ❌ W0 | ⬜ pending |
| 1-03-02 | 03 | 2 | RUNT-01 | API contract | `rg -n "POST /v1/hosts/\\{hostID\\}/create|task_id|image.lock|image_name|preserve-home|RunHostAction|GET /v1/tasks|last_error_summary|ListTasksWithLastErrorSummary" internal/controlplane/http/hosts.go internal/controlplane/http/tasks.go internal/runtime/runtime_service.go internal/agentapi/client.go internal/runtime/tasks/dispatcher.go internal/store/repository/queries.go && go test ./...` | ❌ W0 | ⬜ pending |
| 1-03-03 | 03 | 2 | RUNT-01 | boundary smoke | `rg -n "type Provider interface|NoopProvider|WorkingDirectory=/opt/claudedock|AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN|ExecStartPre=/usr/bin/env bash /opt/claudedock/deploy/scripts/host-preflight.sh" internal/network/provider.go deploy/systemd/claudedock-control-plane.service deploy/systemd/claudedock-host-agent.service deploy/scripts/host-preflight.sh && systemd-analyze verify deploy/systemd/claudedock-control-plane.service deploy/systemd/claudedock-host-agent.service` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `go.mod` — 建立 Go module 与基础依赖，保证 `go test ./...` 可运行
- [ ] `internal/testutil/taskfixture/taskfixture.go` — 为任务状态机与 repository 测试提供共享 fixture
- [ ] `scripts/verify-managed-image.sh` — 受管镜像的统一冒烟校验入口
- [ ] `deploy/scripts/host-preflight.sh` — 宿主机边界预检脚本，供 systemd 与执行前验证复用
- [ ] `deploy/docker/control-plane/Dockerfile` — 让 `docker compose` 真正能构建并启动 control-plane

---

## Manual-Only Verifications

All Phase 1 behaviors can be covered by `go test`、`rg` 静态校验和 Docker 冒烟脚本；不需要额外的纯手工验收门槛。

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 120s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending 2026-03-26
