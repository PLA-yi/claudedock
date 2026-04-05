# Quick Task 260405-hio: Summary

## 变更概要

将 Claude Code 配置编辑器从简单的 JSON 文本框升级为结构化设置面板，新增系统指纹查看功能。

### 后端
- 新增 `GET /v1/admin/hosts/{hostID}/claude/info` 端点
  - 读取容器内 `~/.claude.json`（OAuth 会话、信任设置等身份信息）
  - 返回主机名、内核信息、Node.js 版本
  - 单次 `docker exec` 完成所有信息采集

### 前端 — 结构化设置面板（3 标签页）
- **常规** 标签：
  - 默认模型（model）
  - 响应语言（language）
  - 始终启用深度思考开关（alwaysThinkingEnabled）
  - 思考力度选择（effortLevel: low/medium/high）
  - 自动更新频道（autoUpdatesChannel: latest/stable）
- **权限** 标签：
  - 默认权限模式下拉选择（6 种模式：default/acceptEdits/plan/auto/dontAsk/bypassPermissions）
  - Allow 规则列表编辑器（添加/删除）
  - Deny 规则列表编辑器（添加/删除）
- **JSON** 标签：保留原始 JSON 编辑器作为高级模式
- 标签切换时自动同步结构化数据 ↔ JSON 文本

### 前端 — 系统指纹
- Claude 状态卡片底部新增「查看系统指纹」折叠区域
- 展示：主机名、内核版本、Node.js 版本
- 展示 `~/.claude.json` 完整内容（OAuth 会话、组织信息等）

## 修改文件
| 文件 | 变更 |
|------|------|
| `internal/controlplane/http/admin_hosts.go` | 新增 GetClaudeInfo handler |
| `internal/controlplane/http/router.go` | 注册 claude/info 路由 |
| `web/admin/src/hooks/use-hosts.ts` | 新增 useClaudeInfo hook + ClaudeInfoResponse 类型 |
| `web/admin/src/components/hosts/claude-settings-dialog.tsx` | 完全重写为 3 标签页结构化面板 |
| `web/admin/src/components/hosts/claude-status-card.tsx` | 新增 SystemFingerprint 折叠组件 |
