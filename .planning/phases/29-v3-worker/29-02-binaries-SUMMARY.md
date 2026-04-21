---
phase: 29-v3-worker
plan: 02-binaries
subsystem: infra
tags: [docker, mergerfs, mutagen, sha256]

key-files:
  modified:
    - deploy/docker/managed-user/Dockerfile

requirements-completed:
  - M3
  - BASE-04

completed: 2026-04-18
---

# Plan 02-binaries Summary

在 Chromium RUN 与 `locale-gen` 之间加入 mergerfs 2.41.1（GitHub noble deb + 双架构 SHA256）、Mutagen v0.18.1 外层 tarball 校验并抽出 `mutagen-agents.tar.gz` 至 `/opt`，以及 `/etc/cloud-claude/{mergerfs,mutagen,tmux}.version` 元数据。

## Commits

- `98f1c8d` — mergerfs 安装 RUN
- `8d16005` — mutagen tarball RUN
- `1b4fe11` — `/etc/cloud-claude/*.version` RUN

## Self-Check: PASSED
