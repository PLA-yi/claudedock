import { toast } from "sonner";
import { useBypassPresets } from "@/hooks/use-bypass-presets";
import {
  useBypassBindings,
  useCreateBypassBinding,
  useDeleteBypassBinding,
} from "@/hooks/use-bypass-bindings";
import { parseBypassError } from "@/lib/i18n/bypass-error-codes";
import { PresetCard } from "./preset-card";
import type { BypassPreset } from "@/lib/api/types/bypass";

interface PresetGridProps {
  hostId: string;
}

/**
 * 3 列网格：loopback 锁定 / lan 可勾选 / 占位 disabled。
 * - 数据来源：useBypassPresets() + useBypassBindings(hostId) 求差集判定 selected
 * - 后续未来的 cn-dev 等预设由后端 is_system + is_forced=false 控制；如果系统返回的预设不足 3 个，
 *   用前端占位卡片填充至 3 列，提示「敬请期待」。
 */
export function PresetGrid({ hostId }: PresetGridProps) {
  const presetsQuery = useBypassPresets();
  const bindingsQuery = useBypassBindings(hostId);
  const createMutation = useCreateBypassBinding(hostId);
  const deleteMutation = useDeleteBypassBinding(hostId);

  const presets = presetsQuery.data?.presets ?? [];
  const bindings = bindingsQuery.data?.bindings ?? [];

  // 预设 id → 已绑定 binding id
  const bindingByPreset = new Map<string, string>();
  for (const b of bindings) {
    bindingByPreset.set(b.preset_id, b.id);
  }

  function togglePreset(preset: BypassPreset, nextSelected: boolean) {
    const existingBindingId = bindingByPreset.get(preset.id);
    if (nextSelected && !existingBindingId) {
      createMutation.mutate(preset.id, {
        onSuccess: () => toast.success(`预设「${preset.name}」已启用`),
        onError: (err) => toast.error(parseBypassError(err).message),
      });
    } else if (!nextSelected && existingBindingId) {
      deleteMutation.mutate(existingBindingId, {
        onSuccess: () => toast.success(`预设「${preset.name}」已停用`),
        onError: (err) => toast.error(parseBypassError(err).message),
      });
    }
  }

  if (presetsQuery.isLoading) {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            data-testid="preset-skeleton"
            className="h-24 animate-pulse rounded-xl bg-muted"
          />
        ))}
      </div>
    );
  }

  // 至少展示 3 个槽位；不足的用占位卡填充
  const placeholders = Math.max(0, 3 - presets.length);

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-3" data-testid="preset-grid">
      {presets.map((preset) => (
        <PresetCard
          key={preset.id}
          preset={preset}
          forced={preset.is_forced}
          selected={bindingByPreset.has(preset.id)}
          onToggle={(next) => togglePreset(preset, next)}
        />
      ))}
      {Array.from({ length: placeholders }).map((_, i) => (
        <div
          key={`placeholder-${i}`}
          className="flex h-24 items-center justify-center rounded-xl border border-dashed bg-muted/30 text-xs text-muted-foreground"
        >
          敬请期待
        </div>
      ))}
    </div>
  );
}
