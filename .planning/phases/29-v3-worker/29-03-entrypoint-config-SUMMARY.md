---
phase: 29-v3-worker
plan: 03-entrypoint-config
subsystem: infra
tags: [entrypoint, tmux, sshd, mutagen]

key-files:
  created:
    - deploy/docker/managed-user/tmux.conf
    - deploy/docker/managed-user/profile.d-cloud-claude.sh
  modified:
    - deploy/docker/managed-user/entrypoint.sh
    - deploy/docker/managed-user/sshd_config
    - deploy/docker/managed-user/Dockerfile

requirements-completed:
  - C1
  - C2
  - C3
  - C5
  - C7
  - M4
  - M7
  - M8
  - M12
  - M17
  - Q10

completed: 2026-04-18
---

# Plan 03-entrypoint-config Summary

新增 tmux/profile.d 静态配置、`sshd_config` KeepAlive/并发参数；entrypoint 在 `exec sshd` 前串行执行 `prepare_v3_dirs`、`prepare_mutagen_agent`、`prepare_mergerfs_check`、`assert_tmux_version`；Dockerfile `COPY` 上述配置并 `chmod 0644`。

## Commits

- `4e2dffa` — tmux.conf + profile.d
- `8d64d2d` — sshd_config
- `60f2ec5` — entrypoint v3 阶段
- `8705b73` — Dockerfile COPY + chmod

## Self-Check: PASSED
