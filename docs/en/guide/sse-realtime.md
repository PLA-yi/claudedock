# Real-time Push (SSE)

The admin dashboard and user portal can subscribe to real-time events via SSE (Server-Sent Events) without polling.

## Endpoints

### Admin SSE

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  "http://YOUR_HOST:8080/v1/admin/sse?topics=hosts,tasks,image-status"
```

### User SSE

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  "http://YOUR_HOST:8080/v1/user/sse?topics=hosts,tasks"
```

## Query Parameters

| Parameter | Description |
|-----------|-------------|
| `topics`  | Comma-separated topic list. Values: `hosts`, `tasks`, `image-status` |

## Event Format

Each event is a JSON line (newline-delimited JSON):

```json
event: message
data: {"topic":"tasks","action":"update","id":"task-uuid"}
```

| Field     | Description                                          |
|-----------|------------------------------------------------------|
| `topic`   | Event topic: `hosts`, `tasks`, `image-status`        |
| `action`  | Action type: `update`, `create`, `delete`            |
| `id`      | Associated resource ID (optional)                    |
| `payload` | Additional payload (optional)                        |

## Frontend Integration

After receiving an event, frontends should call `queryClient.invalidateQueries()` to refresh the corresponding cache:

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

## Implementation Details

The SSE broadcast system lives in `internal/broadcast/sse.go` and uses a topic-based pub/sub model:

- The control plane publishes events to topics when resources change
- Each SSE connection maintains an independent subscription channel
- Disconnected clients are cleaned up automatically with no memory leaks
- Multiple clients can concurrently subscribe to the same topic

## SSE vs Polling

| Approach | Latency | Server Load | Best For |
|----------|---------|-------------|----------|
| SSE      | Real-time | Low (event-driven) | Host status, task progress, image build status |
| Polling  | Seconds   | High (fixed frequency) | Low-frequency data (e.g., dashboard stats) |

The admin dashboard should use SSE for all list pages, and polling or on-demand refresh for dashboard statistics.
