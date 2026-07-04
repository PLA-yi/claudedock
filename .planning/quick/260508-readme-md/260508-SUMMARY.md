# Quick Task 260508 Summary

## Description
基于当前代码、逻辑和功能，修正并更新各类文档（README.md、README.en.md、docs/ 引导文档、deploy/README.md），使其保持最新。

## Changes

### Task 1: 中文主文档修订
- **README.md**:
  - 功能特性列表新增：错误码自解释系统、tmux 多端会话管理、网络抖动自动恢复、doctor 五维度自检、大文件熔断
  - 删除"用户自助面板"过度承诺条目，修正为管理后台实际能力
  - claudedock 使用章节全面扩充：sessions / doctor / explain / --new-session / --take-over / --mount-mode / NO_PROMOTION / hot_sync_max_file_mb
  - 修正 docker compose 命令（去掉 `--policy always`）
  - 更新架构组件表，补充 claudedock CLI 新能力
  - KasmVNC 描述改为"管理后台"
- **docs/zh/guide/quickstart.md**: 同步上述 claudedock 功能更新，修正 docker compose 命令，"用户面板"改为"管理后台"

### Task 2: 英文与首页文档同步
- **README.en.md**: 与 README.md 修改一一对应，英文翻译
- **docs/index.md / docs/zh/index.md / docs/en/index.md**: 更新首页 features 列表，新增 doctor / 网络恢复 / 错误码 三大特性，删除"用户面板"
- **docs/en/guide/quickstart.md**: 同步英文版 claudedock 功能更新

### Task 3: 架构与运维文档修正
- **docs/zh/guide/architecture.md**:
  - 修正技术栈 Go 版本为 1.25.7（与 go.mod 一致）
  - 项目结构增加 `cmd/claudedock/`
  - 核心组件新增 claudedock CLI 章节
  - 数据流新增 claudedock 本地 CLI 接入流程
- **docs/en/guide/architecture.md**: 同步上述修改
- **deploy/README.md**: 补充 setup-env.sh、host-preflight.sh 说明，以及 v3.1+ 宿主机路径挂载注意事项

## Files Changed
- README.md
- README.en.md
- deploy/README.md
- docs/index.md
- docs/zh/index.md
- docs/en/index.md
- docs/zh/guide/quickstart.md
- docs/en/guide/quickstart.md
- docs/zh/guide/architecture.md
- docs/en/guide/architecture.md

## Commit
待提交。
