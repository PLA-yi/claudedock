import { useQuery } from "@tanstack/react-query";
import { listBypassPresets } from "@/lib/api/bypass";
import type { BypassPreset } from "@/lib/api/types/bypass";

export const BYPASS_PRESETS_KEY = ["bypass", "presets"] as const;

export function useBypassPresets() {
  return useQuery<{ presets: BypassPreset[] }>({
    queryKey: BYPASS_PRESETS_KEY,
    queryFn: listBypassPresets,
    staleTime: 30_000,
  });
}
