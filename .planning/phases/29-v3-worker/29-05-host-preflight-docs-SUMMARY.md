---
phase: 29-v3-worker
plan: 05-host-preflight-docs
subsystem: infra
tags: [bash, apparmor, ubuntu, host-preflight]

key-files:
  created:
    - deploy/README.md
  modified:
    - deploy/scripts/host-preflight.sh

requirements-completed:
  - D-23
  - D-24
  - C6

completed: 2026-04-18
---

# Plan 05-host-preflight-docs Summary

扩展 `deploy/scripts/host-preflight.sh`：在 Ubuntu 25.04+ 上对 `/etc/apparmor.d/local/fusermount3` 做 advisory 检测（缺 `capability dac_override` 则打印修复指引，`|| true` 不阻断后续初始化）；新增 `deploy/README.md` 中文运维章节。

## Commits

- `df5e37d` — `check_apparmor_fusermount3` 函数与调用
- `5c6b2a8` — README v3.0 AppArmor 章节

## Self-Check: PASSED
