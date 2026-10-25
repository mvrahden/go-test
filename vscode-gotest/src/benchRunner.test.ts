import { describe, it, expect, vi, beforeEach, type Mock } from "vitest";
import type { EventEmitter } from "node:events";
import { existsSync } from "node:fs";
import * as path from "node:path";

vi.mock("vscode", () => {
  class CancellationTokenSource {
    private listeners: (() => void)[] = [];
    isCancellationRequested = false;
    token = {
      isCancellationRequested: false,
      onCancellationRequested: (fn: () => void) => {
        this.listeners.push(fn);
        return { dispose: () => {} };
      },
    };
    cancel() {
      this.isCancellationRequested = true;
      this.token.isCancellationRequested = true;
      for (const fn of this.listeners) fn();
    }
    dispose() {}
  }
  return {
    EventEmitter: class {},
    CancellationTokenSource,
    window: { showErrorMessage: vi.fn() },
  };
});

/** FakeChild is a spawned process the test drives by hand. */
interface FakeChild extends EventEmitter {
  stdout: EventEmitter;
  stderr: EventEmitter;
  kill: Mock;
}

const h = vi.hoisted(() => ({
  spawned: [] as { bin: string; child: FakeChild }[],
  cliArgs: [] as string[],
}));

vi.mock("node:child_process", async (importOriginal) => {
  // Imported here: the factory runs before the imports.
  const { EventEmitter: Emitter } = await import("node:events");
  return {
    ...(await importOriginal<typeof import("node:child_process")>()),
    spawn: vi.fn((bin: string) => {
      const child = Object.assign(new Emitter(), {
        stdout: new Emitter(),
        stderr: new Emitter(),
        kill: vi.fn(),
        unref: vi.fn(),
      });
      h.spawned.push({ bin, child });
      return child;
    }),
  };
});

vi.mock("./cli.js", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./cli.js")>()),
  buildCliCommand: vi.fn(async (args: string[]) => {
    h.cliArgs = args;
    return { bin: "gotest", args };
  }),
  offerCliUpgrade: vi.fn(() => false),
}));

import {
  BenchRunner,
  planBenchInvocations,
  benchTargetFromItemId,
  buildProfileArgs,
} from "./benchRunner.js";

describe("planBenchInvocations", () => {
  it("keeps distinct method targets in selection order", () => {
    const plan = planBenchInvocations([
      { importPath: "a/b", suiteName: "S", methodName: "BenchmarkX" },
      { importPath: "a/b", suiteName: "S", methodName: "BenchmarkY" },
    ]);
    expect(plan).toHaveLength(2);
    expect(plan[0].methodName).toBe("BenchmarkX");
  });

  it("collapses duplicates", () => {
    const plan = planBenchInvocations([
      { importPath: "a/b", suiteName: "S", methodName: "BenchmarkX" },
      { importPath: "a/b", suiteName: "S", methodName: "BenchmarkX" },
    ]);
    expect(plan).toHaveLength(1);
  });

  it("lets a suite-level target subsume its method targets", () => {
    const plan = planBenchInvocations([
      { importPath: "a/b", suiteName: "S", methodName: "BenchmarkX" },
      { importPath: "a/b", suiteName: "S" },
      { importPath: "a/b", suiteName: "Other", methodName: "BenchmarkZ" },
    ]);
    expect(plan).toEqual([
      { importPath: "a/b", suiteName: "S" },
      { importPath: "a/b", suiteName: "Other", methodName: "BenchmarkZ" },
    ]);
  });
});

describe("benchTargetFromItemId", () => {
  it("splits import path, suite, and method from the right", () => {
    expect(
      benchTargetFromItemId(
        "github.com/x/y/pkg/CacheTestSuite/BenchmarkGetHit",
      ),
    ).toEqual({
      importPath: "github.com/x/y/pkg",
      suiteName: "CacheTestSuite",
      methodName: "BenchmarkGetHit",
    });
  });

  it("refuses ids whose leaf is not a benchmark method", () => {
    expect(
      benchTargetFromItemId("github.com/x/y/pkg/CacheTestSuite/TestGet"),
    ).toBeUndefined();
  });

  it("refuses ids that are too short to carry a method", () => {
    expect(benchTargetFromItemId("pkg/Suite")).toBeUndefined();
  });
});

describe("buildProfileArgs", () => {
  it("adds the go test profile flag with an absolute output path", () => {
    expect(
      buildProfileArgs(
        { importPath: "a/b", suiteName: "S", methodName: "BenchmarkX" },
        "cpu",
        "/tmp/prof",
      ),
    ).toEqual([
      "bench",
      "a/b",
      "-bench=^BenchmarkS$/^BenchmarkX$",
      "-cpuprofile=/tmp/prof/cpu.pprof",
      "--json",
    ]);
  });

  it("uses -memprofile for the memory kind", () => {
    const args = buildProfileArgs(
      { importPath: "a/b", suiteName: "S" },
      "mem",
      "/tmp/prof",
    );
    expect(args).toContain("-memprofile=/tmp/prof/mem.pprof");
  });
});

describe("profileTarget", () => {
  const target = {
    importPath: "a/b",
    suiteName: "S",
    methodName: "BenchmarkX",
  };
  const report = JSON.stringify({
    schemaVersion: 1,
    baseline: { results: [] },
  });
  let runner: BenchRunner;

  beforeEach(() => {
    h.spawned.length = 0;
    h.cliArgs = [];
    const log = { info: vi.fn(), warn: vi.fn() };
    runner = new BenchRunner(
      {} as never,
      { getWorkspaceDir: () => "/ws" } as never,
      { recordReport: vi.fn() } as never,
      log as never,
    );
  });

  /** outDir is the profile directory the run was handed. */
  function outDir(): string {
    const flag = h.cliArgs.find((a) => a.startsWith("-cpuprofile="));
    return path.dirname(flag!.slice("-cpuprofile=".length));
  }

  /** benchStarted waits until the bench process is spawned. */
  async function benchStarted(): Promise<FakeChild> {
    await vi.waitFor(() => expect(h.spawned).toHaveLength(1));
    return h.spawned[0].child;
  }

  it("removes the profile directory once pprof is done with it", async () => {
    const run = runner.profileTarget(target, "cpu");
    const bench = await benchStarted();
    bench.stdout.emit("data", Buffer.from(report));
    bench.emit("close", 0);
    await run;
    expect(h.spawned[1].bin).toBe("go");
    expect(existsSync(outDir())).toBe(true);
    h.spawned[1].child.emit("exit", 0);
    await vi.waitFor(() => expect(existsSync(outDir())).toBe(false));
  });

  it("removes the profile directory when the run fails", async () => {
    const run = runner.profileTarget(target, "cpu");
    const bench = await benchStarted();
    bench.emit("close", 1);
    await run;
    expect(h.spawned).toHaveLength(1);
    expect(existsSync(outDir())).toBe(false);
  });

  it("removes the profile directory and opens no pprof when the run is cancelled", async () => {
    const run = runner.profileTarget(target, "cpu");
    const bench = await benchStarted();
    bench.stdout.emit("data", Buffer.from(report));
    runner.dispose();
    expect(bench.kill).toHaveBeenCalled();
    bench.emit("close", null);
    await run;
    expect(h.spawned).toHaveLength(1);
    expect(existsSync(outDir())).toBe(false);
  });
});
