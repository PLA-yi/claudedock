---
phase: 39-dev-containers
plan: 03
status: complete
completed: 2026-05-07
commit: 295d558
---

# Plan 03: devcontainer.json 更新 + sing-box 启动逻辑

## What Was Built

更新了 `.devcontainer/devcontainer.json` 支持 VS Code Dev Containers 的 MODE=local 工作流，完善了 entrypoint.sh 中 sing-box 启动逻辑（tun 和 proxy 两种模式）。

## Key Files

### Modified
- `.devcontainer/devcontainer.json` — 添加 containerEnv MODE=local、forwardPorts [22]、runArgs --env MODE=local
- `deploy/docker/managed-user/entrypoint.sh` — 完善 sing-box 启动段：tun 模式（需 NET_ADMIN）和 proxy 模式（SOCKS5/HTTP 代理），自动检测协议类型，设置代理环境变量到 /etc/environment

## Decisions

- devcontainer.json 使用 containerEnv + runArgs --env 双重设置 MODE=local
- proxy 模式在容器内生成最小 sing-box 配置（inbound socks/http + outbound from file）
- tun 模式需要预生成的 /etc/sing-box/config.json，不存在时自动降级为 proxy 模式
- sing-box 二进制不存在时给出 WARNING 但不阻塞 sshd 启动
- 代理环境变量写入 /etc/environment 确保所有用户 session 继承

## Notable Deviations

- Dockerfile 未修改（sing-box 二进制安装属于镜像构建阶段的独立工作，不在 Phase 39 范围内）
