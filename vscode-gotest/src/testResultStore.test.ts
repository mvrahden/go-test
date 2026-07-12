import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdtemp, rm, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import * as path from "node:path";
import { TestResultStore } from "./testResultStore.js";

describe("TestResultStore", () => {
  let store: TestResultStore;

  beforeEach(() => {
    store = new TestResultStore(undefined);
  });

  it("record/get: records a result and retrieves it", () => {
    store.record("pkg/suite/test", "pass", 123);
    const result = store.get("pkg/suite/test");
    expect(result).toBeDefined();
    expect(result?.status).toBe("pass");
    expect(result?.duration).toBe(123);
  });

  it("returns undefined for unknown item", () => {
    expect(store.get("nonexistent")).toBeUndefined();
  });

  it("overwrites previous result for same item", () => {
    store.record("pkg/suite/test", "pass", 100);
    store.record("pkg/suite/test", "fail", 200);
    const result = store.get("pkg/suite/test");
    expect(result?.status).toBe("fail");
    expect(result?.duration).toBe(200);
  });

  it("delete: removes a single result", () => {
    store.record("pkg/suite/a", "pass");
    store.record("pkg/suite/b", "fail");
    store.delete("pkg/suite/a");
    expect(store.get("pkg/suite/a")).toBeUndefined();
    expect(store.get("pkg/suite/b")).toBeDefined();
  });

  it("forEach: iterates all entries", () => {
    store.record("pkg/suite/a", "pass", 10);
    store.record("pkg/suite/b", "skip");
    const collected: Array<{ id: string; status: string }> = [];
    store.forEach((result, id) => {
      collected.push({ id, status: result.status });
    });
    expect(collected).toHaveLength(2);
    expect(collected.map((c) => c.id).sort()).toEqual([
      "pkg/suite/a",
      "pkg/suite/b",
    ]);
  });

  it("size: reports correct count", () => {
    expect(store.size).toBe(0);
    store.record("a", "pass");
    expect(store.size).toBe(1);
    store.record("b", "fail");
    expect(store.size).toBe(2);
    store.delete("a");
    expect(store.size).toBe(1);
  });

  describe("persistence", () => {
    let tmpDir: string;

    beforeEach(async () => {
      tmpDir = await mkdtemp(path.join(tmpdir(), "testResultStore-"));
    });

    it("persistence round-trip: save then load in a new instance", async () => {
      const writer = new TestResultStore({ fsPath: tmpDir });
      writer.record("pkg/suite/a", "pass", 42);
      writer.record("pkg/suite/b", "fail");
      writer.record("pkg/suite/c", "skip", 7);
      writer.save();
      await writer.flush();

      const reader = new TestResultStore({ fsPath: tmpDir });
      await reader.load();

      expect(reader.size).toBe(3);
      expect(reader.get("pkg/suite/a")).toMatchObject({
        status: "pass",
        duration: 42,
      });
      expect(reader.get("pkg/suite/b")).toMatchObject({ status: "fail" });
      expect(reader.get("pkg/suite/c")).toMatchObject({
        status: "skip",
        duration: 7,
      });
    });

    it("load handles missing file gracefully", async () => {
      const s = new TestResultStore({ fsPath: tmpDir });
      await expect(s.load()).resolves.toBeUndefined();
      expect(s.size).toBe(0);
    });

    it("load handles corrupt data gracefully", async () => {
      const storagePath = path.join(tmpDir, "testResults.json");
      await writeFile(storagePath, "not valid json", "utf-8");
      const s = new TestResultStore({ fsPath: tmpDir });
      await expect(s.load()).resolves.toBeUndefined();
      expect(s.size).toBe(0);
    });

    it("evicts stale entries on load", async () => {
      const writer = new TestResultStore({ fsPath: tmpDir });
      writer.record("pkg/fresh", "pass");
      writer.record("pkg/stale", "fail");
      writer.save();
      await writer.flush();

      const filePath = path.join(tmpDir, "testResults.json");
      const raw = await readFile(filePath, "utf-8");
      const data = JSON.parse(raw);
      data.results["pkg/stale"].timestamp =
        Date.now() - 8 * 24 * 60 * 60 * 1000;
      await writeFile(filePath, JSON.stringify(data), "utf-8");

      const reader = new TestResultStore({ fsPath: tmpDir });
      await reader.load();
      expect(reader.get("pkg/fresh")).toBeDefined();
      expect(reader.get("pkg/stale")).toBeUndefined();
    });

    it("evicts stale entries on save", async () => {
      const store = new TestResultStore({ fsPath: tmpDir }, 0);
      store.record("pkg/a", "pass");
      await new Promise((r) => setTimeout(r, 10));
      store.save();
      await store.flush();

      const reader = new TestResultStore({ fsPath: tmpDir });
      await reader.load();
      expect(reader.size).toBe(0);
    });

    it("preserves fresh entries during eviction", async () => {
      const store = new TestResultStore({ fsPath: tmpDir }, 60_000);
      store.record("pkg/a", "pass");
      store.record("pkg/b", "fail");
      store.save();
      await store.flush();

      const reader = new TestResultStore({ fsPath: tmpDir });
      await reader.load();
      expect(reader.size).toBe(2);
    });

    afterEach(async () => {
      await rm(tmpDir, { recursive: true, force: true });
    });
  });
});
