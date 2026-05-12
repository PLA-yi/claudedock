import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PresetGrid } from "../preset-grid";
import type { BypassPreset } from "@/lib/api/types/bypass";

const apiFetchMock = vi.fn();
vi.mock("@/lib/api", () => ({
  apiFetch: (...args: unknown[]) => apiFetchMock(...args),
}));
vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

function renderWithClient(ui: React.ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

const loopback: BypassPreset = {
  id: "p-loopback",
  slug: "loopback",
  name: "loopback（本机回环）",
  description: "127.0.0.0/8 + 169.254.0.0/16，强制启用",
  is_system: true,
  is_forced: true,
  rule_count: 2,
  sample_rules: [
    { rule_type: "cidr", value: "127.0.0.0/8" },
    { rule_type: "cidr", value: "169.254.0.0/16" },
  ],
  created_at: "",
  updated_at: "",
};
const lan: BypassPreset = {
  id: "p-lan",
  slug: "lan",
  name: "lan（内网与 ULA）",
  description: "RFC1918 + CGNAT 100.64/10 + ULA fc00::/7",
  is_system: true,
  is_forced: false,
  rule_count: 5,
  sample_rules: [
    { rule_type: "cidr", value: "10.0.0.0/8" },
    { rule_type: "cidr", value: "172.16.0.0/12" },
  ],
  created_at: "",
  updated_at: "",
};

describe("PresetGrid", () => {
  beforeEach(() => {
    apiFetchMock.mockReset();
  });

  it("loadiing 期间显示骨架占位", () => {
    apiFetchMock.mockImplementation(() => new Promise(() => {})); // never resolves
    renderWithClient(<PresetGrid hostId="h-1" />);
    expect(screen.getAllByTestId("preset-skeleton")).toHaveLength(3);
  });

  it("加载后 loopback 卡片处于 forced-on 状态（disabled checkbox + Lock 图标）", async () => {
    apiFetchMock.mockImplementation(async (path: string) => {
      if (path === "/bypass/presets") return { presets: [loopback, lan] };
      if (path.includes("/bindings")) return { bindings: [] };
      throw new Error("unexpected path " + path);
    });
    renderWithClient(<PresetGrid hostId="h-1" />);

    const card = await screen.findByTestId("preset-card-loopback");
    expect(card).toHaveAttribute("data-state", "forced-on");
    const checkbox = card.querySelector(
      "input[type=checkbox]",
    ) as HTMLInputElement;
    expect(checkbox.disabled).toBe(true);
    expect(checkbox.checked).toBe(true);
    expect(screen.getByLabelText("强制启用，不可关闭")).toBeInTheDocument();
  });

  it("lan 卡片可点击切换，触发 createBinding mutation", async () => {
    apiFetchMock.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === "/bypass/presets") return { presets: [loopback, lan] };
      if (path.includes("/bindings") && (!init || !init.method))
        return { bindings: [] };
      if (path.includes("/bindings") && init?.method === "POST") {
        return { binding: { id: "b-new" } };
      }
      throw new Error("unexpected path " + path);
    });
    const user = userEvent.setup();
    renderWithClient(<PresetGrid hostId="h-1" />);

    const lanCard = await screen.findByTestId("preset-card-lan");
    expect(lanCard).toHaveAttribute("data-state", "unselected");

    const lanCheckbox = lanCard.querySelector(
      "input[type=checkbox]",
    ) as HTMLInputElement;
    await user.click(lanCheckbox);

    // 应该发出 POST bindings 请求
    const postCalls = apiFetchMock.mock.calls.filter(
      (c) =>
        typeof c[0] === "string" &&
        (c[0] as string).includes("/bindings") &&
        (c[1] as RequestInit | undefined)?.method === "POST",
    );
    expect(postCalls.length).toBeGreaterThanOrEqual(1);
  });

  it("3 个预设不足时填充 placeholder", async () => {
    apiFetchMock.mockImplementation(async (path: string) => {
      if (path === "/bypass/presets") return { presets: [loopback] };
      if (path.includes("/bindings")) return { bindings: [] };
      throw new Error("unexpected " + path);
    });
    renderWithClient(<PresetGrid hostId="h-1" />);

    await screen.findByTestId("preset-card-loopback");
    expect(screen.getAllByText("敬请期待")).toHaveLength(2);
  });
});
