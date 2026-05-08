import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
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

  it("clear: removes all results", () => {
    store.record("pkg/suite/a", "pass");
    store.record("pkg/suite/b", "fail");
    store.clear();
    expect(store.size).toBe(0);
    expect(store.get("pkg/suite/a")).toBeUndefined();
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
      await writer.save();

      const reader = new TestResultStore({ fsPath: tmpDir });
      await reader.load();

      expect(reader.size).toBe(3);
      expect(reader.get("pkg/suite/a")).toMatchObject({ status: "pass", duration: 42 });
      expect(reader.get("pkg/suite/b")).toMatchObject({ status: "fail" });
      expect(reader.get("pkg/suite/c")).toMatchObject({ status: "skip", duration: 7 });
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

    afterEach(async () => {
      await rm(tmpDir, { recursive: true, force: true });
    });
  });
});
