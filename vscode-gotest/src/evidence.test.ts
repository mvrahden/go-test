import { describe, it, expect } from "vitest";
import { EvidenceSelection } from "./evidence.js";
import type { Memento } from "./acceptance.js";

function memento(): Memento {
  const data = new Map<string, unknown>();
  return {
    get: <T>(k: string) => data.get(k) as T | undefined,
    update: async (k: string, v: unknown) => {
      data.set(k, JSON.parse(JSON.stringify(v)));
    },
  };
}

describe("EvidenceSelection", () => {
  it("takes a run per folder, drops it on a second toggle, and tells", async () => {
    const sel = new EvidenceSelection(memento());
    let told = 0;
    sel.onChange(() => told++);
    const pick = { id: "r1", file: "/s/a.jsonl", label: "14:02" };
    await sel.toggle("/w", pick);
    expect(sel.list("/w")).toEqual([pick]);
    expect(sel.has("/w", "r1")).toBe(true);
    expect(sel.list("/other")).toEqual([]);
    await sel.toggle("/w", pick);
    expect(sel.list("/w")).toEqual([]);
    expect(told).toBe(2);
  });

  it("is kept in the workspace's state", async () => {
    const store = memento();
    await new EvidenceSelection(store).toggle("/w", {
      id: "r1",
      file: "/f",
      label: "x",
    });
    const again = new EvidenceSelection(store);
    expect(again.list("/w").map((p) => p.id)).toEqual(["r1"]);
    await again.remove("/w", "r1");
    expect(new EvidenceSelection(store).list("/w")).toEqual([]);
  });
});
