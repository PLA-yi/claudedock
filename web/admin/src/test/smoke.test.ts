import { describe, it, expect } from "vitest";

describe("vitest bootstrap smoke", () => {
  it("基础断言生效", () => {
    expect(1 + 1).toBe(2);
  });

  it("jsdom 环境可用", () => {
    const el = document.createElement("div");
    el.textContent = "hello";
    document.body.appendChild(el);
    expect(document.body.textContent).toContain("hello");
  });
});
