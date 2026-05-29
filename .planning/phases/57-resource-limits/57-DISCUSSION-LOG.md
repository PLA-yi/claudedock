# Phase 57: 资源限制可配置化 - Discussion Log

**Date:** 2026-05-29
**Mode:** default (interactive)

## Discussion Summary

用户要求允许管理员在创建和停止主机时手动设置内存和 CPU 限制，支持"无限制"选项。核心诉求："优雅、好用"。用户将全部实现决策委托给 Claude。

## Areas Discussed

### Area 1: "无限制"的语义
- **Options presented:** 数据库 NULL / API 三态指针 / Docker 不传参数
- **User selection:** Claude 自行决定
- **Decision:** DB nullable (NULL=无限制), API 指针类型 (省略=默认, 0=无限制, 正值=限制)

### Area 2: 何时可调整限制
- **Options presented:** 仅创建时 / 创建+停止后 / 创建+停止+运行中
- **User selection:** Claude 自行决定
- **Decision:** 创建时 + 停止后（新增 PATCH API），不支持运行中热调

### Area 3: 前端交互方式
- **Options presented:** 下拉预设 / 滑块 / 自由输入
- **User selection:** Claude 自行决定
- **Decision:** 下拉预设 + "自定义"展开数字输入，创建表单和详情页都支持

### Area 4: 磁盘限制
- **Options presented:** 纳入 / 不纳入
- **User selection:** Claude 自行决定
- **Decision:** 纳入，Worker 新增 --storage-opt 参数

## Deferred Ideas
- 运行中容器热调资源（docker update）
- 用户自助面板查看资源限制
- 资源使用量监控
- 按角色限制可选范围

---
*Discussion completed: 2026-05-29*
