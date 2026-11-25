import { describe, it, expect } from "vitest";
import { AcceptanceStore, type Memento } from "./acceptance.js";

function memento(): Memento & { data: Map<string, unknown> } {
  const data = new Map<string, unknown>();
  return {
    data,
    get: <T>(key: string) => data.get(key) as T | undefined,
    update: async (key: string, value: unknown) => {
      // Stored as the editor stores it: a copy, not the live object.
      data.set(key, JSON.parse(JSON.stringify(value)));
    },
  };
}

describe("AcceptanceStore", () => {
  it("accepts a row and takes the acceptance back", async () => {
    const store = new AcceptanceStore(memento());
    expect(await store.toggle("/ws", "c1", "k")).toBe(true);
    expect(store.accepted("/ws", "c1").has("k")).toBe(true);
    expect(await store.toggle("/ws", "c1", "k")).toBe(false);
    expect(store.accepted("/ws", "c1").has("k")).toBe(false);
  });

  it("forgets the acceptances when the head commit changes", async () => {
    const store = new AcceptanceStore(memento());
    await store.toggle("/ws", "c1", "a");
    expect(store.accepted("/ws", "c2").size).toBe(0);
    await store.toggle("/ws", "c2", "b");
    expect(Array.from(store.accepted("/ws", "c2"))).toEqual(["b"]);
    expect(store.accepted("/ws", "c1").size).toBe(0);
  });

  it("keeps each folder's apart, across a new store over the same memento", async () => {
    const m = memento();
    await new AcceptanceStore(m).toggle("/a", "c1", "k");
    const again = new AcceptanceStore(m);
    expect(again.accepted("/a", "c1").has("k")).toBe(true);
    expect(again.accepted("/b", "c1").size).toBe(0);
  });
});
