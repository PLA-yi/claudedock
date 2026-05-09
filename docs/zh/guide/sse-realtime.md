# 实时推送（SSE）

管理后台和用户面板可通过 SSE（Server-Sent Events）订阅实时事件，无需轮询。

## 端点

### 管理员 SSE

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  "http://YOUR_HOST:8080/v1/admin/sse?topics=hosts,tasks,image-status"
```

### 用户 SSE

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  "http://YOUR_HOST:8080/v1/user/sse?topics=hosts,tasks"
```

## 查询参数

| 参数 | 说明 |
|------|------|
| `topics` | 逗号分隔的主题列表，可选值：`hosts`、`tasks`、`image-status` |

## 事件格式

每条事件为 JSON 行（newline-delimited JSON）：

```json
event: message
data: {"topic":"tasks","action":"update","id":"task-uuid"}
```

| 字段 | 说明 |
|------|------|
| `topic` | 事件主题：`hosts`、`tasks`、`image-status` |
| `action` | 操作类型：`update`、`create`、`delete` |
| `id` | 关联资源 ID（可选） |
| `payload` | 额外载荷（可选） |

## 前端集成建议

前端收到事件后，建议使用 `queryClient.invalidateQueries()` 刷新对应缓存。例如：

```typescript
const eventSource = new EventSource(
  '/v1/admin/sse?topics=hosts,tasks',
  { headers: { Authorization: `Bearer ${token}` } }
);

eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data);
  queryClient.invalidateQueries({ queryKey: [data.topic] });
};
```

## 实现细节

SSE 广播系统位于 `internal/broadcast/sse.go`，采用 topic-based pub/sub 模型：

- 控制面在资源变更时向对应 topic 发布事件
- 每个 SSE 连接维护独立的订阅通道
- 连接断开时自动清理，无内存泄漏
- 支持多客户端并发订阅同一 topic

## 与轮询的对比

| 方式 | 延迟 | 服务器负载 | 适用场景 |
|------|------|-----------|----------|
| SSE | 实时 | 低（事件驱动） | 主机状态、任务进度、镜像构建状态 |
| 轮询 | 秒级 | 高（固定频率） | 低频数据（如仪表盘统计） |

推荐管理后台对所有列表页使用 SSE，仪表盘统计使用轮询或按需刷新。
