import { describe, it, expect } from "vitest";
import {
  BenchResultStore,
  benchKey,
  hostPlatform,
  type MementoLike,
} from "./benchResultStore.js";
import type { BenchReport } from "./benchReport.js";

function fakeMemento(): MementoLike & { data: Map<string, unknown> } {
  const data = new Map<string, unknown>();
  return {
    data,
    get<T>(key: string, defaultValue: T): T {
      return (data.has(key) ? data.get(key) : defaultValue) as T;
    },
    update(key: string, value: unknown) {
      data.set(key, value);
      return Promise.resolve();
    },
  };
}

function report(goos: string, goarch: string, nsPerOp: number): BenchReport {
  return {
    schemaVersion: 1,
    baseline: {
      schemaVersion: 1,
      goos,
      goarch,
      results: [
        {
          package: "example.com/pkg",
          suite: "CacheTestSuite",
          name: "BenchmarkGetHit",
          samples: [
            { iterations: 100, nsPerOp, bytesPerOp: 480, allocsPerOp: 3 },
          ],
        },
      ],
    },
  };
}

describe("benchKey", () => {
  it("keys by package, suite, method, and platform", () => {
    expect(
      benchKey(
        "example.com/pkg",
        "CacheTestSuite",
        "BenchmarkGetHit",
        "linux",
        "amd64",
      ),
    ).toBe("example.com/pkg/CacheTestSuite/BenchmarkGetHit@linux/amd64");
  });
});

describe("BenchResultStore", () => {
  it("records a report's results keyed by the report's own platform stamps", () => {
    const store = new BenchResultStore(fakeMemento());
    store.recordReport(
      report("linux", "amd64", 56.1),
      Date.parse("2026-08-14T12:00:00Z"),
    );

    const entry = store.getLatest(
      "example.com/pkg",
      "CacheTestSuite",
      "BenchmarkGetHit",
      { goos: "linux", goarch: "amd64" },
    );
    expect(entry?.nsPerOp).toBe(56.1);
    expect(entry?.bytesPerOp).toBe(480);
    expect(entry?.recordedAt).toBe(Date.parse("2026-08-14T12:00:00Z"));
  });

  it("never serves a result recorded on a different platform", () => {
    const store = new BenchResultStore(fakeMemento());
    store.recordReport(report("darwin", "arm64", 42), Date.now());

    const entry = store.getLatest(
      "example.com/pkg",
      "CacheTestSuite",
      "BenchmarkGetHit",
      { goos: "linux", goarch: "amd64" },
    );
    expect(entry).toBeUndefined();
  });

  it("uses the mean of a multi-sample run for the displayed numbers", () => {
    const store = new BenchResultStore(fakeMemento());
    const multi = report("linux", "amd64", 0);
    multi.baseline.results[0].samples = [
      { iterations: 100, nsPerOp: 100, bytesPerOp: 480, allocsPerOp: 3 },
      { iterations: 100, nsPerOp: 120, bytesPerOp: 480, allocsPerOp: 3 },
    ];
    store.recordReport(multi, Date.now());

    const entry = store.getLatest(
      "example.com/pkg",
      "CacheTestSuite",
      "BenchmarkGetHit",
      { goos: "linux", goarch: "amd64" },
    );
    expect(entry?.nsPerOp).toBe(110);
    expect(entry?.sampleCount).toBe(2);
  });

  it("survives a reload through the memento", () => {
    const memento = fakeMemento();
    const store = new BenchResultStore(memento);
    store.recordReport(report("linux", "amd64", 56.1), Date.now());

    const reloaded = new BenchResultStore(memento);
    const entry = reloaded.getLatest(
      "example.com/pkg",
      "CacheTestSuite",
      "BenchmarkGetHit",
      { goos: "linux", goarch: "amd64" },
    );
    expect(entry?.nsPerOp).toBe(56.1);
  });

  it("ignores stored data with an unknown version", () => {
    const memento = fakeMemento();
    memento.data.set("gotest.benchResults", { version: 99, entries: {} });
    const store = new BenchResultStore(memento);
    expect(
      store.getLatest("example.com/pkg", "CacheTestSuite", "BenchmarkGetHit", {
        goos: "linux",
        goarch: "amd64",
      }),
    ).toBeUndefined();
  });

  it("notifies listeners when a report lands", () => {
    const store = new BenchResultStore(fakeMemento());
    let fired = 0;
    store.onDidUpdate(() => fired++);
    store.recordReport(report("linux", "amd64", 56.1), Date.now());
    expect(fired).toBe(1);
  });
});

describe("hostPlatform", () => {
  it("maps the node process platform/arch to GOOS/GOARCH names", () => {
    expect(hostPlatform({ platform: "linux", arch: "x64" })).toEqual({
      goos: "linux",
      goarch: "amd64",
    });
    expect(hostPlatform({ platform: "darwin", arch: "arm64" })).toEqual({
      goos: "darwin",
      goarch: "arm64",
    });
    expect(hostPlatform({ platform: "win32", arch: "x64" })).toEqual({
      goos: "windows",
      goarch: "amd64",
    });
  });
});

describe("BenchResultStore deltas", () => {
  function compareReport(pct: number, significant: boolean): BenchReport {
    const r = report("linux", "amd64", 112.3);
    r.deltas = [
      {
        key: "example.com/pkg CacheTestSuite/BenchmarkGetHit",
        oldNs: 100,
        newNs: 112.3,
        percentChange: pct,
        significant,
        insufficientSample: false,
      },
    ];
    return r;
  }

  it("attaches the CLI's delta verdict to the matching entry", () => {
    const store = new BenchResultStore(fakeMemento());
    store.recordReport(compareReport(12.3, true), Date.now());

    const entry = store.getLatest(
      "example.com/pkg",
      "CacheTestSuite",
      "BenchmarkGetHit",
      { goos: "linux", goarch: "amd64" },
    );
    expect(entry?.delta).toEqual({
      percentChange: 12.3,
      significant: true,
      insufficientSample: false,
    });
  });

  it("clears a stale delta when a later run has no comparison", () => {
    const store = new BenchResultStore(fakeMemento());
    store.recordReport(compareReport(12.3, true), Date.now());
    store.recordReport(report("linux", "amd64", 60), Date.now());

    const entry = store.getLatest(
      "example.com/pkg",
      "CacheTestSuite",
      "BenchmarkGetHit",
      { goos: "linux", goarch: "amd64" },
    );
    expect(entry?.delta).toBeUndefined();
  });
});

describe("BenchResultStore history", () => {
  it("appends one history point per run, newest last", () => {
    const store = new BenchResultStore(fakeMemento());
    store.recordReport(report("linux", "amd64", 100), 1000);
    store.recordReport(report("linux", "amd64", 120), 2000);

    const history = store.getHistory(
      "example.com/pkg",
      "CacheTestSuite",
      "BenchmarkGetHit",
      { goos: "linux", goarch: "amd64" },
    );
    expect(history.map((h) => h.nsPerOp)).toEqual([100, 120]);
    expect(history.map((h) => h.recordedAt)).toEqual([1000, 2000]);
  });

  it("caps history at 50 runs per key, dropping the oldest", () => {
    const store = new BenchResultStore(fakeMemento());
    for (let i = 0; i < 55; i++) {
      store.recordReport(report("linux", "amd64", i), i);
    }
    const history = store.getHistory(
      "example.com/pkg",
      "CacheTestSuite",
      "BenchmarkGetHit",
      { goos: "linux", goarch: "amd64" },
    );
    expect(history).toHaveLength(50);
    expect(history[0].nsPerOp).toBe(5);
    expect(history[49].nsPerOp).toBe(54);
  });

  it("keeps history platform-scoped like everything else", () => {
    const store = new BenchResultStore(fakeMemento());
    store.recordReport(report("darwin", "arm64", 100), 1000);
    expect(
      store.getHistory("example.com/pkg", "CacheTestSuite", "BenchmarkGetHit", {
        goos: "linux",
        goarch: "amd64",
      }),
    ).toEqual([]);
  });

  it("records the sample spread of a multi-count run", () => {
    const store = new BenchResultStore(fakeMemento());
    const multi = report("linux", "amd64", 0);
    multi.baseline.results[0].samples = [
      { iterations: 10, nsPerOp: 100, bytesPerOp: 0, allocsPerOp: 0 },
      { iterations: 10, nsPerOp: 130, bytesPerOp: 0, allocsPerOp: 0 },
      { iterations: 10, nsPerOp: 110, bytesPerOp: 0, allocsPerOp: 0 },
    ];
    store.recordReport(multi, 1000);
    const entry = store.getLatest(
      "example.com/pkg",
      "CacheTestSuite",
      "BenchmarkGetHit",
      { goos: "linux", goarch: "amd64" },
    );
    expect(entry?.minNsPerOp).toBe(100);
    expect(entry?.maxNsPerOp).toBe(130);
  });
});
