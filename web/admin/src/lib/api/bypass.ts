import { apiFetch } from "@/lib/api";
import type {
  BypassPreset,
  BypassRule,
  BypassBinding,
  BypassRuleCreatePayload,
  BypassRuleUpdatePayload,
  BypassPreviewResponse,
  BypassApplyResponse,
  BypassRollbackResponse,
  BypassEffectiveResponse,
  BypassAuditLogResponse,
} from "./types/bypass";

// 系统预设列表（admin 全局可用预设）
export function listBypassPresets() {
  return apiFetch<{ presets: BypassPreset[] }>("/bypass/presets");
}

// host 维度规则集：后端路由是 GET /v1/admin/bypass/rules?host_id=...
// （host 维度通过 query 参数过滤，没有 /hosts/{hostId}/bypass/rules 路由）
export function listBypassRules(hostId: string) {
  return apiFetch<{ rules: BypassRule[] }>(
    `/bypass/rules?host_id=${encodeURIComponent(hostId)}`,
  );
}

export function createBypassRule(
  hostId: string,
  payload: BypassRuleCreatePayload,
) {
  // 后端是 POST /v1/admin/bypass/rules，并强制要求 scope ∈ {global, host}。
  // 这里固定 scope="host" 并把 hostId 注入 body.host_id（前端 UI 仅创建 host 维度规则）。
  return apiFetch<{ rule: BypassRule }>(`/bypass/rules`, {
    method: "POST",
    body: JSON.stringify({ scope: "host", host_id: hostId, ...payload }),
  });
}

export function updateBypassRule(
  _hostId: string,
  ruleId: string,
  payload: BypassRuleUpdatePayload,
) {
  // 后端是 PATCH /v1/admin/bypass/rules/{ruleID}（没有 host 前缀，且仅接受 PATCH）。
  return apiFetch<{ rule: BypassRule }>(
    `/bypass/rules/${ruleId}`,
    {
      method: "PATCH",
      body: JSON.stringify(payload),
    },
  );
}

export function deleteBypassRule(_hostId: string, ruleId: string) {
  // 后端是 DELETE /v1/admin/bypass/rules/{ruleID}（同上，没有 host 前缀）。
  return apiFetch<void>(`/bypass/rules/${ruleId}`, {
    method: "DELETE",
  });
}

// host 与预设绑定。后端路由（router.go:272-274）：
//   - GET    /v1/admin/hosts/{hostID}/bypass     （不带 /bindings 后缀）
//   - POST   /v1/admin/hosts/{hostID}/bypass
//   - DELETE /v1/admin/bypass/bindings/{bindingID}  （删除不带 host 前缀）
export function listBypassBindings(hostId: string) {
  return apiFetch<{ bindings: BypassBinding[] }>(
    `/hosts/${hostId}/bypass`,
  );
}

export function createBypassBinding(hostId: string, presetId: string) {
  return apiFetch<{ binding: BypassBinding }>(
    `/hosts/${hostId}/bypass`,
    {
      method: "POST",
      body: JSON.stringify({ preset_id: presetId }),
    },
  );
}

export function deleteBypassBinding(_hostId: string, bindingId: string) {
  return apiFetch<void>(`/bypass/bindings/${bindingId}`, {
    method: "DELETE",
  });
}

// ===== 46-04 扩展：preview / apply / rollback / effective / auditLog =====

export function previewBypass(hostId: string) {
  return apiFetch<BypassPreviewResponse>(`/hosts/${hostId}/bypass/preview`, {
    method: "POST",
    body: "{}",
  });
}

export function applyBypass(hostId: string, note?: string) {
  return apiFetch<BypassApplyResponse>(`/hosts/${hostId}/bypass/apply`, {
    method: "POST",
    body: JSON.stringify({ note: note ?? "" }),
  });
}

export function rollbackBypass(hostId: string, targetSnapshotId: string) {
  return apiFetch<BypassRollbackResponse>(`/hosts/${hostId}/bypass/rollback`, {
    method: "POST",
    body: JSON.stringify({ target_snapshot_id: targetSnapshotId }),
  });
}

export function effectiveBypass(hostId: string) {
  return apiFetch<BypassEffectiveResponse>(`/hosts/${hostId}/bypass/effective`);
}

export function auditLogBypass(
  hostId: string,
  opts?: { limit?: number; before?: string },
) {
  const q = new URLSearchParams();
  if (opts?.limit) q.set("limit", String(opts.limit));
  if (opts?.before) q.set("before", opts.before);
  const qs = q.toString();
  return apiFetch<BypassAuditLogResponse>(
    `/hosts/${hostId}/bypass/audit-log${qs ? "?" + qs : ""}`,
  );
}
