import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApplyProgressDialog } from "../apply-progress-dialog";
import type { Task } from "@/hooks/use-tasks";

const apiFetchMock = vi.fn();
vi.mock("@/lib/api", () => ({
  apiFetch: (...args: unknown[]) => apiFetchMock(...args),
}));
vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/hooks/use-sse", () => ({
  useSSE: () => {},
}));

// 直接 mock useTaskPolling，避免与 react-query polling 时序耦合。
let mockTask: Task | null = null;
vi.mock("@/hooks/use-tasks", () => ({
  useTaskPolling: () => ({ data: mockTask }),
}));

function renderWithClient(ui: React.ReactNode) {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe("ApplyProgressDialog", () => {
  beforeEach(() => {
    apiFetchMock.mockReset();
    mockTask = null;
    vi.useRealTimers();
  });

  it("打开 Dialog 自动调 apply mutation + 5 个固定阶段名渲染", async () => {
    apiFetchMock.mockResolvedValue({
      snapshot_id: "snap-1",
      version: 24,
      config_hash: "h",
      applied_status: "pending",
      task_id: "t-1",
      message: "ok",
    });
    renderWithClient(
      <ApplyProgressDialog
        hostId="h-1"
        open={true}
        onOpenChange={() => {}}
      />,
    );

    expect(await screen.findByText("应用白名单配置")).toBeInTheDocument();
    expect(screen.getByText("生成快照")).toBeInTheDocument();
    expect(screen.getByText("下发到 agent")).toBeInTheDocument();
    expect(screen.getByText("Reload 配置")).toBeInTheDocument();
    expect(screen.getByText("健康检查")).toBeInTheDocument();
    expect(screen.getByText("完成")).toBeInTheDocument();

    await waitFor(() => {
      expect(apiFetchMock).toHaveBeenCalledWith(
        "/hosts/h-1/bypass/apply",
        expect.objectContaining({ method: "POST" }),
      );
    });
  });

  it("mutation 进行中：snapshot 阶段为 active，其它 pending", async () => {
    // 永不 resolve 的 promise，让 apply mutation 停留在 pending
    apiFetchMock.mockReturnValue(new Promise(() => {}));
    renderWithClient(
      <ApplyProgressDialog
        hostId="h-1"
        open={true}
        onOpenChange={() => {}}
      />,
    );

    const snapshot = await screen.findByTestId("apply-stage-snapshot");
    expect(snapshot).toHaveAttribute("data-status", "active");
    expect(screen.getByTestId("apply-stage-dispatch")).toHaveAttribute(
      "data-status",
      "pending",
    );
    expect(screen.getByTestId("apply-stage-done")).toHaveAttribute(
      "data-status",
      "pending",
    );
  });

  it("task succeeded：全部 5 阶段 done + 500ms 后自动关闭 + toast.success", async () => {
    mockTask = {
      task_id: "t-success",
      kind: "reload_host_bypass",
      status: "succeeded",
      updated_at: "2026-05-12T00:00:00Z",
      progress_percent: 100,
    };
    apiFetchMock.mockResolvedValue({
      snapshot_id: "snap",
      version: 24,
      config_hash: "h",
      applied_status: "pending",
      task_id: "t-success",
      message: "ok",
    });

    const onOpenChange = vi.fn();
    renderWithClient(
      <ApplyProgressDialog
        hostId="h-1"
        open={true}
        onOpenChange={onOpenChange}
      />,
    );

    await waitFor(
      () => {
        expect(screen.getByTestId("apply-stage-done")).toHaveAttribute(
          "data-status",
          "done",
        );
      },
      { timeout: 3000 },
    );

    // 真实 timer：500ms 后自动关闭
    await waitFor(
      () => {
        expect(onOpenChange).toHaveBeenCalledWith(false);
      },
      { timeout: 3000, interval: 100 },
    );

    const { toast } = await import("sonner");
    expect(toast.success).toHaveBeenCalledWith(
      "已应用 · 白名单变更不影响现有 TCP 连接，新连接才用新规则",
    );
  });

  it("apply mutation onError (BYPASS_HOST_UNREACHABLE)：snapshot=failed + 错误码中文文案 + 关闭按钮", async () => {
    apiFetchMock.mockRejectedValue(
      new Error(
        JSON.stringify({
          code: "BYPASS_HOST_UNREACHABLE",
          message: "host unreachable",
        }),
      ),
    );
    renderWithClient(
      <ApplyProgressDialog
        hostId="h-1"
        open={true}
        onOpenChange={() => {}}
      />,
    );

    const failure = await screen.findByTestId("apply-failure");
    expect(failure).toHaveTextContent(
      "host-agent 当前不可达，配置已保存但未生效，将自动重试",
    );
    expect(failure).toHaveTextContent("BYPASS_HOST_UNREACHABLE");

    expect(screen.getByTestId("apply-stage-snapshot")).toHaveAttribute(
      "data-status",
      "failed",
    );
    expect(screen.getByRole("button", { name: "关闭" })).toBeInTheDocument();
  });

  it("task failed：snapshot=done + dispatch=failed + 显示 task last_error_summary", async () => {
    mockTask = {
      task_id: "t-fail",
      kind: "reload_host_bypass",
      status: "failed",
      updated_at: "2026-05-12T00:00:00Z",
      error_code: "BYPASS_RELOAD_TIMEOUT",
      last_error_summary: "sing-box reload timed out",
    };
    apiFetchMock.mockResolvedValue({
      snapshot_id: "snap",
      version: 24,
      config_hash: "h",
      applied_status: "pending",
      task_id: "t-fail",
      message: "ok",
    });

    renderWithClient(
      <ApplyProgressDialog
        hostId="h-1"
        open={true}
        onOpenChange={() => {}}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("apply-stage-dispatch")).toHaveAttribute(
        "data-status",
        "failed",
      );
    });
    expect(screen.getByTestId("apply-stage-snapshot")).toHaveAttribute(
      "data-status",
      "done",
    );
    const failure = screen.getByTestId("apply-failure");
    expect(failure).toHaveTextContent("sing-box reload timed out");
  });

  it("task running with progress_percent=60：snapshot/dispatch/reload=done, health=active, done=pending", async () => {
    mockTask = {
      task_id: "t-running",
      kind: "reload_host_bypass",
      status: "running",
      updated_at: "2026-05-12T00:00:00Z",
      progress_percent: 60,
    };
    apiFetchMock.mockResolvedValue({
      snapshot_id: "snap",
      version: 24,
      config_hash: "h",
      applied_status: "pending",
      task_id: "t-running",
      message: "ok",
    });

    renderWithClient(
      <ApplyProgressDialog
        hostId="h-1"
        open={true}
        onOpenChange={() => {}}
      />,
    );

    // pct=60 ∈ [50, 75) → 第 4 阶段（健康检查）active
    await waitFor(() => {
      expect(screen.getByTestId("apply-stage-health")).toHaveAttribute(
        "data-status",
        "active",
      );
    });
    expect(screen.getByTestId("apply-stage-snapshot")).toHaveAttribute(
      "data-status",
      "done",
    );
    expect(screen.getByTestId("apply-stage-dispatch")).toHaveAttribute(
      "data-status",
      "done",
    );
    expect(screen.getByTestId("apply-stage-reload")).toHaveAttribute(
      "data-status",
      "done",
    );
    expect(screen.getByTestId("apply-stage-done")).toHaveAttribute(
      "data-status",
      "pending",
    );
  });
});
