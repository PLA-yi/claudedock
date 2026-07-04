---
status: passed
phase: 29-v3-worker
completed: 2026-04-18
---

# Phase 29 验证摘要

## 自动化

- `go test ./...` 通过（含 `worker_volume_test.go`）。
- `bash -n deploy/scripts/host-preflight.sh`、`bash -n deploy/docker/managed-user/entrypoint.sh` 通过。

## 计划交付核对

| 计划 | 核心交付物 |
|------|------------|
| 01 | Dockerfile BuildKit / tini / 预建目录 / ENTRYPOINT |
| 02 | mergerfs + mutagen tarball + `/etc/claudedock/*.version` |
| 03 | entrypoint v3 阶段 + tmux/profile.d/sshd + COPY |
| 04 | `VolumeMount` + `buildCreateArgs` + 单测 |
| 05 | `check_apparmor_fusermount3` + deploy README |
| 06 | `image.lock` 6 字段 + CI size gate（方案 B） |

## 说明

完整 `docker build` 受网络环境影响未在本会话内全量复跑；CI 中 size gate 与 `go test` 为合并后主要回归信号。
