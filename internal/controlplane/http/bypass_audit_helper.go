package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	nethttp "net/http"
	"strings"

	"github.com/claudedock/claudedock/internal/store/repository"
)

// BypassAuditLogWriter 抽出 InsertBypassAuditLog 让 handler 在测试中注入 stub。
// 真实实现由 Phase 45 Plan 03 的 Repository 提供。
type BypassAuditLogWriter interface {
	InsertBypassAuditLog(ctx context.Context, params repository.InsertBypassAuditLogParams) (string, error)
}

// extractActorIP 优先 X-Forwarded-For 第一段 / X-Real-IP / RemoteAddr 去端口。
// 命中 trusted proxy header 时使用 header，否则回落到 socket peer。
func extractActorIP(r *nethttp.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		first := strings.TrimSpace(parts[0])
		if first != "" {
			return first
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	if r.RemoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// actorIDPtr 把 UserIDFromContext 的字符串转 *string；空串落 nil 让 audit 列存 NULL。
func actorIDPtr(ctx context.Context) *string {
	id := UserIDFromContext(ctx)
	if id == "" {
		return nil
	}
	return &id
}

// writeBypassAuditLog 同时落两轨（per 46-CONTEXT.md L107 锁定的双轨制）：
//
//  1. 第一轨：host_bypass_audit_log（详细 diff，含 before / after JSONB），
//     由 writer.InsertBypassAuditLog 同步写入；
//  2. 第二轨：EventRecorder.RecordEvent 也是 **同步** 写入 events 表 + 触发
//     SSE 广播。原本旧注释写「异步消费」是错的（实现一直是同步的）。
//
// 任一轨失败仅记 logger.Warn，不向上抛错。原因：audit 失败不应阻塞主请求成功，
// 后续可通过 audit 表 / events 表互相对账兜底。两轨顺序固定：audit_log 在前
// （持久化 diff 优先），events 在后（同步发布、广播）。
//
// WR-03：保留同步实现，仅修正注释 —— 改成 goroutine 会让现有单元测试
// （断言 events.hasType("bypass.xxx")）出现 race；批量操作下延迟叠加问题
// 留待 Phase 47 配 Outbox 模式时一起优化。
func writeBypassAuditLog(
	ctx context.Context,
	logger *slog.Logger,
	writer BypassAuditLogWriter,
	events EventRecorder,
	r *nethttp.Request,
	action, targetKind string,
	targetID *string,
	before, after any,
	note string,
) {
	beforeJSON := marshalAuditPayload(logger, "before", action, before)
	afterJSON := marshalAuditPayload(logger, "after", action, after)

	actorID := actorIDPtr(ctx)
	actorIP := extractActorIP(r)

	// 第一轨：host_bypass_audit_log 详细 diff。
	if writer != nil {
		if _, err := writer.InsertBypassAuditLog(ctx, repository.InsertBypassAuditLogParams{
			ActorID:    actorID,
			ActorIP:    actorIP,
			Action:     action,
			TargetKind: targetKind,
			TargetID:   targetID,
			Before:     beforeJSON,
			After:      afterJSON,
			Note:       note,
		}); err != nil {
			logger.Warn("write bypass audit_log failed", "action", action, "target_kind", targetKind, "err", err)
		}
	}

	// 第二轨：events 事件流（异步消费 / SSE 广播）。
	if events != nil {
		metadata := map[string]any{
			"actor_id":    derefStringPtr(actorID),
			"actor_ip":    actorIP,
			"action":      action,
			"target_kind": targetKind,
			"target_id":   derefStringPtr(targetID),
			"note":        note,
		}
		if _, err := events.RecordEvent(ctx, repository.RecordEventParams{
			Level:    "info",
			Type:     "bypass." + action,
			Message:  "Bypass " + action,
			Metadata: metadata,
		}); err != nil {
			logger.Warn("publish bypass event failed", "kind", "bypass."+action, "err", err)
		}
	}
}

// marshalAuditPayload 把 any 序列化为 json.RawMessage；nil 返回 nil。
// 失败仅记日志后返回 nil，让 audit 行的列存 NULL 而不阻塞主流程。
func marshalAuditPayload(logger *slog.Logger, slot, action string, v any) json.RawMessage {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		logger.Warn("marshal bypass audit payload failed", "slot", slot, "action", action, "err", err)
		return nil
	}
	return raw
}

func derefStringPtr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
