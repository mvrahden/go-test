import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  Uri: { joinPath: (...args: string[]) => ({ toString: () => args.join("/") }) },
  workspace: {
    getConfiguration: () => ({ get: () => true }),
  },
  window: {},
  ViewColumn: { Beside: 2 },
  commands: {},
}));

// We can't unit-test the full SpecViewPanel (requires VS Code runtime),
// but we test the pure helper functions that are exported or extractable.
// Since the module is structured with the HTML builders as module-level functions,
// we test through the class's buildHtml output by checking HTML structure.
// For now, we verify the module imports cleanly.

describe("specView module", () => {
  it("imports without error", async () => {
    const mod = await import("./specView.js");
    expect(mod.SpecViewPanel).toBeDefined();
  });
});
