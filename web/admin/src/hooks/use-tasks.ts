import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/lib/api";

export interface Task {
  task_id: string;
  host_id?: string | null;
  kind: string;
  status: string;
  requested_by?: string;
  error_code?: string;
  error_message?: string;
  last_error_summary?: string;
  progress_percent?: number;
  progress_message?: string;
  created_at?: string;
  updated_at: string;
}

export function useTasks() {
  return useQuery({
    queryKey: ["tasks"],
    queryFn: () => apiFetch<{ tasks: Task[] }>("/tasks"),
    refetchInterval: 5000,
  });
}

export function useTaskPolling(taskId: string | null) {
  return useQuery({
    queryKey: ["tasks", taskId],
    queryFn: async () => {
      const data = await apiFetch<{ tasks: Task[] }>("/tasks");
      return data.tasks.find((t) => t.task_id === taskId) ?? null;
    },
    enabled: !!taskId,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === "succeeded" || status === "failed" || status === "canceled")
        return false;
      return 2000;
    },
  });
}
