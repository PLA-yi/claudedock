import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createBypassRule,
  deleteBypassRule,
  listBypassRules,
  updateBypassRule,
} from "@/lib/api/bypass";
import type {
  BypassRule,
  BypassRuleCreatePayload,
  BypassRuleUpdatePayload,
} from "@/lib/api/types/bypass";

export function bypassRulesKey(hostId: string) {
  return ["bypass", "rules", hostId] as const;
}

export function useBypassRules(hostId: string) {
  return useQuery<{ rules: BypassRule[] }>({
    queryKey: bypassRulesKey(hostId),
    queryFn: () => listBypassRules(hostId),
    enabled: !!hostId,
    staleTime: 30_000,
  });
}

export function useCreateBypassRule(hostId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: BypassRuleCreatePayload) =>
      createBypassRule(hostId, payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: bypassRulesKey(hostId) });
    },
  });
}

export function useUpdateBypassRule(hostId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      ruleId,
      payload,
    }: {
      ruleId: string;
      payload: BypassRuleUpdatePayload;
    }) => updateBypassRule(hostId, ruleId, payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: bypassRulesKey(hostId) });
    },
  });
}

export function useDeleteBypassRule(hostId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ruleId: string) => deleteBypassRule(hostId, ruleId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: bypassRulesKey(hostId) });
    },
  });
}
