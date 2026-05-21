import { describe, it, expect, beforeEach } from "vitest";
import { RunRegistry } from "./runRegistry.js";
import * as fs from "node:fs/promises";
import * as path from "node:path";
import { tmpdir } from "node:os";

describe("RunRegistry", () => {
  let registry: RunRegistry;
  let storageDir: string;

  beforeEach(async () => {
    storageDir = await fs.mkdtemp(path.join(tmpdir(), "gotest-reg-"));
    registry = new RunRegistry(storageDir);
  });

  it("registers and retrieves a run", () => {
    const record = registry.register({
      kind: "test",
      packages: ["./pkg/..."],
    });

    expect(record.id).toBeTruthy();
    expect(record.status).toBe("running");

    const found = registry.get(record.id);
    expect(found).toEqual(record);
  });

  it("updates status to completed", () => {
    const record = registry.register({
      kind: "coverage",
      packages: [],
    });

    registry.complete(record.id);

    const updated = registry.get(record.id);
    expect(updated?.status).toBe("completed");
    expect(updated?.endedAt).toBeGreaterThan(0);
  });

  it("updates status to cancelled", () => {
    const record = registry.register({
      kind: "test",
      packages: [],
    });

    registry.cancel(record.id);

    expect(registry.get(record.id)?.status).toBe("cancelled");
  });

  it("lists active runs", () => {
    const r1 = registry.register({ kind: "test", packages: [] });
    const r2 = registry.register({ kind: "coverage", packages: [] });
    registry.complete(r1.id);

    const active = registry.active();
    expect(active).toHaveLength(1);
    expect(active[0].id).toBe(r2.id);
  });

  it("expires terminal records after TTL", () => {
    const record = registry.register({
      kind: "test",
      packages: [],
    });

    registry.complete(record.id);

    // Backdate endedAt to 16 minutes ago
    const updated = registry.get(record.id)!;
    (updated as { endedAt: number }).endedAt = Date.now() - 16 * 60 * 1000;

    registry.sweep();

    expect(registry.get(record.id)).toBeUndefined();
  });

  it("keeps terminal records within TTL", () => {
    const record = registry.register({ kind: "test", packages: [] });
    registry.complete(record.id);

    registry.sweep();

    expect(registry.get(record.id)).toBeDefined();
  });

  it("sweepStale marks all running records as crashed", () => {
    const r1 = registry.register({ kind: "test", packages: [] });
    const r2 = registry.register({ kind: "coverage", packages: [] });
    registry.complete(r2.id);

    const crashed = registry.sweepStale();

    expect(crashed).toHaveLength(1);
    expect(crashed[0].id).toBe(r1.id);
    expect(registry.get(r1.id)?.status).toBe("crashed");
    expect(registry.get(r2.id)?.status).toBe("completed");
  });

  it("persists and loads from disk", async () => {
    registry.register({
      kind: "watch",
      packages: ["./..."],
    });

    await registry.save();

    const loaded = new RunRegistry(storageDir);
    await loaded.load();

    expect(loaded.all()).toHaveLength(1);
    expect(loaded.all()[0].kind).toBe("watch");
  });

  it("load with no file starts empty", async () => {
    const fresh = new RunRegistry(storageDir + "/nonexistent");
    await fresh.load();

    expect(fresh.all()).toHaveLength(0);
  });
});
