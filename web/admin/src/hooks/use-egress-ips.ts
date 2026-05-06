import { useState, useCallback, useRef } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";

export interface EgressIP {
  id: string;
  label: string;
  ip_address: string;
  provider: string;
  status: string;
  proxy_config: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
}

export interface TestResult {
  status: "passed" | "partial" | "failed" | "error";
  tested_at: string;
  message?: string;
  results: {
    connectivity: {
      status: "pass" | "fail" | "error";
      latency_ms?: number;
      error?: string;
    };
    egress_ip: {
      status: "pass" | "fail" | "error";
      ip?: string;
      sources?: Record<string, string>;
      error?: string;
    };
    dns_leak: {
      status: "pass" | "fail" | "error" | "skip";
      dns_servers_detected?: string[];
      local_dns_leaked?: boolean;
      error?: string;
    };
  };
}

export function useEgressIPs() {
  return useQuery({
    queryKey: ["egress-ips"],
    queryFn: () => apiFetch<{ egress_ips: EgressIP[] }>("/egress-ips"),
  });
}

export function useEgressIP(ipId: string) {
  return useQuery({
    queryKey: ["egress-ips", ipId],
    queryFn: () => apiFetch<{ egress_ip: EgressIP }>(`/egress-ips/${ipId}`),
    enabled: !!ipId,
  });
}

export function useCreateEgressIP() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: Partial<EgressIP>) =>
      apiFetch<{ egress_ip: EgressIP }>("/egress-ips", {
        method: "POST",
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["egress-ips"] });
      qc.invalidateQueries({ queryKey: ["dashboard-stats"] });
    },
  });
}

export function useUpdateEgressIP() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ ipId, data }: { ipId: string; data: Partial<EgressIP> }) =>
      apiFetch<{ egress_ip: EgressIP }>(`/egress-ips/${ipId}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["egress-ips"] }),
  });
}

export function useDeleteEgressIP() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ipId: string) =>
      apiFetch(`/egress-ips/${ipId}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["egress-ips"] });
      qc.invalidateQueries({ queryKey: ["dashboard-stats"] });
    },
  });
}

export function useTestEgressIP() {
  return useMutation({
    mutationFn: (ipId: string) =>
      apiFetch<TestResult>(`/egress-ips/${ipId}/test`, { method: "POST" }),
  });
}

export type ProbeStage =
  | "pulling"
  | "starting"
  | "connecting"
  | "testing"
  | "done"
  | "error";

export interface ProbeStreamEvent {
  stage: ProbeStage;
  message: string;
  result?: TestResult;
}

export interface SingleTestState {
  stage: ProbeStage | null;
  message: string;
  result: TestResult | null;
  error: string | null;
  isRunning: boolean;
}

export function useTestEgressIPSSE() {
  const [states, setStates] = useState<Map<string, SingleTestState>>(
    new Map(),
  );
  const abortRefs = useRef<Map<string, AbortController>>(new Map());

  const start = useCallback((ipId: string) => {
    // 如果该 IP 已有正在运行的测试，先取消
    const existing = abortRefs.current.get(ipId);
    if (existing) existing.abort();

    setStates((prev) => {
      const next = new Map(prev);
      next.set(ipId, {
        stage: "pulling",
        message: "拉取探针镜像中...",
        result: null,
        error: null,
        isRunning: true,
      });
      return next;
    });

    const token = localStorage.getItem("admin_token");
    const url = `${window.location.origin}/v1/admin/egress-ips/${ipId}/test/stream`;

    const controller = new AbortController();
    abortRefs.current.set(ipId, controller);

    fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      signal: controller.signal,
    })
      .then(async (res) => {
        if (!res.ok) {
          const text = await res.text();
          throw new Error(`HTTP ${res.status}: ${text}`);
        }
        const reader = res.body?.getReader();
        if (!reader) throw new Error("response body is null");

        const decoder = new TextDecoder();
        let buffer = "";

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });

          const lines = buffer.split("\n\n");
          buffer = lines.pop() ?? "";

          for (const chunk of lines) {
            const line = chunk.trim();
            if (!line.startsWith("data: ")) continue;
            const data = line.slice(6);
            try {
              const event: ProbeStreamEvent = JSON.parse(data);
              setStates((prev) => {
                const next = new Map(prev);
                const cur = next.get(ipId);
                next.set(ipId, {
                  stage: event.stage,
                  message: event.message,
                  result: event.result ?? cur?.result ?? null,
                  error: cur?.error ?? null,
                  isRunning: event.stage !== "done" && event.stage !== "error",
                });
                return next;
              });
              if (event.stage === "done" || event.stage === "error") {
                reader.cancel();
                return;
              }
            } catch {
              // ignore malformed event
            }
          }
        }
      })
      .catch((err) => {
        if (err.name === "AbortError") return;
        setStates((prev) => {
          const next = new Map(prev);
          const cur = next.get(ipId);
          next.set(ipId, {
            stage: "error",
            message: err.message,
            result: cur?.result ?? null,
            error: err.message,
            isRunning: false,
          });
          return next;
        });
      });
  }, []);

  const stop = useCallback((ipId: string) => {
    abortRefs.current.get(ipId)?.abort();
    setStates((prev) => {
      const next = new Map(prev);
      const cur = next.get(ipId);
      if (cur) next.set(ipId, { ...cur, isRunning: false });
      return next;
    });
  }, []);

  return { states, start, stop };
}
