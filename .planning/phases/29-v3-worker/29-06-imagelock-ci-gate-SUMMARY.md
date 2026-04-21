---
phase: 29-v3-worker
plan: 06-imagelock-ci-gate
subsystem: ci
tags: [github-actions, image.lock, size-gate]

key-files:
  modified:
    - deploy/docker/managed-user/image.lock
    - .github/workflows/build-images.yml

requirements-completed:
  - BASE-04
  - D-25
  - D-26
  - D-27
  - D-28
  - D-29
  - D-30
  - M18

completed: 2026-04-18
---

# Plan 06-imagelock-ci-gate Summary

`image.lock` 末尾追加 6 个 v3.0 扁平字段；`build-images.yml` 对 `managed-user` 采用 **方案 B**：在现有多架构 push 之外增加仅 `linux/amd64` 且 `load: true` 的构建，标签 `managed-user:size-gate`，随后 `docker image inspect` 校验 ≤ 734003200 bytes（700 MiB）。未修改 `deploy/scripts/build-managed-image.sh`（D-30）。

## Commits

- `b788c6b` — image.lock 字段
- `1487636` — CI size gate（scheme B）

## Self-Check: PASSED
