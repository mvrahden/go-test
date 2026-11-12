import { describe, it, expect, vi, beforeEach } from "vitest";
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
    TestMessage: class {
      constructor(public readonly message: string) {}
    },
    Uri: { file: (p: string) => ({ fsPath: p }) },
    window: { showErrorMessage: vi.fn() },
    workspace: { getConfiguration: () => ({ get: () => undefined }) },
  };
});

const h = vi.hoisted(() => ({
  script: { once: [], always: undefined, calls: [] } as SpawnScript,
  children: [] as (FakeChild & { unref: () => void })[],
  cliArgs: [] as string[],
}));

vi.mock("node:child_process", async (importOriginal) => {
  // Imported here: the factory runs before the imports.
  const { createScriptedSpawn } =
    await import("./scriptedSpawn.test-support.js");
  const spawn = createScriptedSpawn(h.script);
  return {
    ...(await importOriginal<typeof import("node:child_process")>()),
    spawn: (bin: string, args: string[], opts?: Record<string, unknown>) => {
      const child = Object.assign(spawn(bin, args, opts), {
        unref: vi.fn(),
      });
      const kill = child.kill;
      child.kill = vi.fn((signal?: NodeJS.Signals) => kill(signal));
      h.children.push(child);
      return child;
    },
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

import type { SpawnScript, FakeChild } from "./scriptedSpawn.test-support.js";
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

  it("names an external twin's benchmark by the suite go test knows", () => {
    expect(
      benchTargetFromItemId("example.com/p/TwinsTestSuite~external/BenchmarkX"),
    ).toEqual({
      importPath: "example.com/p",
      suiteName: "TwinsTestSuite",
      methodName: "BenchmarkX",
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
  let store: { recordReport: ReturnType<typeof vi.fn> };

  beforeEach(() => {
    h.script.once = [];
    // pprof: runs until the test ends it.
    h.script.always = { neverExits: true };
    h.script.calls = [];
    h.children = [];
    h.cliArgs = [];
    const log = { info: vi.fn(), warn: vi.fn(), error: vi.fn() };
    store = { recordReport: vi.fn() };
    runner = new BenchRunner(
      {} as never,
      { getWorkspaceDir: () => "/ws" } as never,
      store as never,
      log as never,
    );
  });

  /** outDir is the profile directory the run was handed. */
  function outDir(): string {
    const flag = h.cliArgs.find((a) => a.startsWith("-cpuprofile="));
    return path.dirname(flag!.slice("-cpuprofile=".length));
  }

  it("removes the profile directory once pprof is done with it", async () => {
    h.script.once = [{ stdout: [report], code: 0 }];
    await runner.profileTarget(target, "cpu");
    expect(h.script.calls?.[1].bin).toBe("go");
    expect(existsSync(outDir())).toBe(true);
    h.children[1].emit("exit", 0);
    await vi.waitFor(() => expect(existsSync(outDir())).toBe(false));
  });

  it("removes the profile directory when the run fails", async () => {
    h.script.once = [{ code: 1 }];
    await runner.profileTarget(target, "cpu");
    expect(h.children).toHaveLength(1);
    expect(existsSync(outDir())).toBe(false);
  });

  it("removes the profile directory and opens no pprof when the run is cancelled", async () => {
    h.script.once = [{ stdout: [report], neverExits: true }];
    const run = runner.profileTarget(target, "cpu");
    await vi.waitFor(() => expect(h.children).toHaveLength(1));
    runner.dispose();
    expect(h.children[0].written).toEqual(["stop\n"]);
    h.children[0].emit("close", null, null);
    await run;
    expect(h.children).toHaveLength(1);
    expect(existsSync(outDir())).toBe(false);
  });

  it("starts bench with --control=stdin, last", async () => {
    h.script.once = [{ stdout: [report], code: 0 }];
    await runner.runTarget(target);
    expect(h.script.calls?.[0].args.at(-1)).toBe("--control=stdin");
  });

  it("stops a cancelled bench by its stdin and records nothing", async () => {
    h.script.once = [{ stdout: [report], neverExits: true }];
    const run = runner.runTarget(target);
    await vi.waitFor(() => expect(h.children).toHaveLength(1));
    runner.dispose();
    expect(h.children[0].written).toEqual(["stop\n"]);
    expect(h.children[0].kill).not.toHaveBeenCalled();
    h.children[0].emit("close", 0, null);
    await run;
    expect(store.recordReport).not.toHaveBeenCalled();
  });
});

describe("runProfile", () => {
  const benchItem = (id: string) => ({
    id,
    tags: [{ id: "benchmark" }],
  });
  const ok = benchItem("a/b/S/BenchmarkOk");
  const fails = benchItem("a/b/S/BenchmarkFails");
  const request = { include: [ok, fails] } as never;
  const token = {
    isCancellationRequested: false,
    onCancellationRequested: () => ({ dispose: () => {} }),
  } as never;
  const result = (name: string) => ({
    package: "a/b",
    suite: "S",
    name,
    samples: [{ iterations: 1, nsPerOp: 10, bytesPerOp: 0, allocsPerOp: 0 }],
  });
  const doc = (over: object) =>
    JSON.stringify({
      schemaVersion: 1,
      baseline: {
        schemaVersion: 1,
        goos: "linux",
        goarch: "amd64",
        results: [result("BenchmarkOk")],
      },
      ...over,
    });
  let runner: BenchRunner;
  let run: {
    started: ReturnType<typeof vi.fn>;
    passed: ReturnType<typeof vi.fn>;
    failed: ReturnType<typeof vi.fn>;
    errored: ReturnType<typeof vi.fn>;
    end: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    h.script.once = [];
    h.script.calls = [];
    h.children = [];
    run = {
      started: vi.fn(),
      passed: vi.fn(),
      failed: vi.fn(),
      errored: vi.fn(),
      end: vi.fn(),
    };
    const log = { info: vi.fn(), warn: vi.fn(), error: vi.fn() };
    runner = new BenchRunner(
      { createTestRun: () => run } as never,
      { getWorkspaceDir: () => "/ws" } as never,
      { recordReport: vi.fn() } as never,
      log as never,
    );
  });

  it("fails the benchmark the report names and leaves its sibling passed", async () => {
    h.script.always = {
      stdout: [
        doc({
          failures: [
            {
              package: "a/b",
              suite: "S",
              name: "BenchmarkFails",
              output: "bench_test.go:9: boom",
            },
          ],
        }),
      ],
      code: 1,
    };
    await runner.runProfile(request, token);
    expect(run.passed).toHaveBeenCalledWith(ok);
    expect(run.failed.mock.calls[0][0]).toBe(fails);
    expect(run.failed.mock.calls[0][1].message).toBe("bench_test.go:9: boom");
    expect(run.errored).not.toHaveBeenCalled();
  });

  it("errors the selection when the run failed and no benchmark did", async () => {
    h.script.always = {
      stdout: [
        doc({
          failures: [
            {
              package: "a/b",
              suite: "S",
              output: "AfterAll: teardown failed\nmore",
            },
          ],
        }),
      ],
      stderr:
        "FAIL: bench gate: nothing to compare; no benchmark is in both the baseline and this run\n",
      code: 1,
    };
    await runner.runProfile(request, token);
    expect(run.errored).toHaveBeenCalledTimes(2);
    expect(run.errored.mock.calls[0][1].message).toBe(
      "FAIL: bench gate: nothing to compare; no benchmark is in both the baseline and this run\n" +
        "a/b/S: AfterAll: teardown failed\n\nexit code 1",
    );
  });

  it("leaves the benchmarks passed when only the gate was breached", async () => {
    h.script.always = {
      stdout: [
        doc({ gate: { thresholdPct: 10, worstPct: 12, breached: true } }),
      ],
      stderr: "bench gate: a/b/S/BenchmarkOk +12.0% exceeds 10% gate\n",
      code: 1,
    };
    await runner.runProfile(request, token);
    expect(run.passed).toHaveBeenCalledWith(ok);
    expect(run.errored).not.toHaveBeenCalled();
  });

  it("leaves a clean run alone", async () => {
    h.script.always = { stdout: [doc({})], code: 0 };
    await runner.runProfile({ include: [ok] } as never, token);
    expect(run.errored).not.toHaveBeenCalled();
    expect(run.failed).not.toHaveBeenCalled();
  });
});
