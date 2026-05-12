import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createBypassBinding,
  deleteBypassBinding,
  listBypassBindings,
} from "@/lib/api/bypass";
import type { BypassBinding } from "@/lib/api/types/bypass";

export function bypassBindingsKey(hostId: string) {
  return ["bypass", "bindings", hostId] as const;
}

export function useBypassBindings(hostId: string) {
  return useQuery<{ bindings: BypassBinding[] }>({
    queryKey: bypassBindingsKey(hostId),
    queryFn: () => listBypassBindings(hostId),
    enabled: !!hostId,
    staleTime: 30_000,
  });
}

export function useCreateBypassBinding(hostId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (presetId: string) => createBypassBinding(hostId, presetId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: bypassBindingsKey(hostId) });
    },
  });
}

export function useDeleteBypassBinding(hostId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (bindingId: string) => deleteBypassBinding(hostId, bindingId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: bypassBindingsKey(hostId) });
    },
  });
}
