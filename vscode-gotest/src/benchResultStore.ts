// benchResultStore keeps the last benchmark numbers per method so CodeLens
// annotations survive editor reloads. Entries are keyed by package, suite,
// method, AND goos/goarch — a number measured on another platform is a
// different number, and the store refuses to serve it for this host, the
// same way the CLI refuses cross-platform baseline comparisons.
//
// Persistence is the workspace Memento (workspaceState): benchmark numbers
// are workspace-scoped working state, not artifacts. The store depends only
// on the minimal MementoLike surface so tests run without a vscode mock.

import type { BenchReport } from "./benchReport.js";

export interface MementoLike {
  get<T>(key: string, defaultValue: T): T;
  update(key: string, value: unknown): Thenable<void>;
}

export interface PlatformKey {
  goos: string;
  goarch: string;
}

/** The stored summary of one benchmark's most recent run. */
/**
 * The CLI's comparison verdict for one entry, copied verbatim from the
 * report. `significant` is Welch's t-test speaking — the UI never overrides
 * it, and an insignificant delta renders as nothing at all.
 */
export interface BenchEntryDelta {
  percentChange: number;
  significant: boolean;
  insufficientSample: boolean;
}

export interface BenchEntry {
  nsPerOp: number;
  bytesPerOp: number;
  allocsPerOp: number;
  iterations: number;
  /** Number of samples behind the mean (>1 under -count=N). */
  sampleCount: number;
  /** Fastest/slowest rep of a multi-sample run (equal to nsPerOp for 1×). */
  minNsPerOp: number;
  maxNsPerOp: number;
  recordedAt: number;
  goos: string;
  goarch: string;
  /** Present only when the recording run compared against a baseline. */
  delta?: BenchEntryDelta;
}

/** One point of a benchmark's run-over-run trend. */
export interface BenchHistoryPoint {
  nsPerOp: number;
  recordedAt: number;
  sampleCount: number;
}

/** Bounded run-over-run history per key; the trend a hover shows. */
const MAX_HISTORY = 50;

interface StoredData {
  version: 1;
  entries: Record<string, BenchEntry>;
  history?: Record<string, BenchHistoryPoint[]>;
}

const STORAGE_KEY = "gotest.benchResults";

export function benchKey(
  importPath: string,
  suiteName: string,
  methodName: string,
  goos: string,
  goarch: string,
): string {
  return `${importPath}/${suiteName}/${methodName}@${goos}/${goarch}`;
}

/** hostPlatform maps Node's platform/arch names onto GOOS/GOARCH. */
export function hostPlatform(
  proc: { platform: string; arch: string } = process,
): PlatformKey {
  const goos =
    proc.platform === "win32"
      ? "windows"
      : proc.platform === "sunos"
        ? "solaris"
        : proc.platform;
  const archMap: Record<string, string> = {
    x64: "amd64",
    ia32: "386",
    arm64: "arm64",
    arm: "arm",
  };
  return { goos, goarch: archMap[proc.arch] ?? proc.arch };
}

export class BenchResultStore {
  private entries = new Map<string, BenchEntry>();
  private history = new Map<string, BenchHistoryPoint[]>();
  private listeners: Array<() => void> = [];

  constructor(private readonly memento: MementoLike) {
    const stored = this.memento.get<StoredData | undefined>(
      STORAGE_KEY,
      undefined,
    );
    if (stored && stored.version === 1) {
      for (const [key, entry] of Object.entries(stored.entries)) {
        this.entries.set(key, entry);
      }
      for (const [key, points] of Object.entries(stored.history ?? {})) {
        this.history.set(key, points);
      }
    }
  }

  onDidUpdate(listener: () => void): { dispose(): void } {
    this.listeners.push(listener);
    return {
      dispose: () => {
        this.listeners = this.listeners.filter((l) => l !== listener);
      },
    };
  }

  /**
   * recordReport stores one entry per result in the report, stamped with the
   * report's own goos/goarch. Multi-sample runs (-count=N) store the mean —
   * the CLI's comparison logic consumes the full samples, the annotation
   * only needs one honest number.
   */
  recordReport(report: BenchReport, recordedAt: number): void {
    const { goos, goarch } = report.baseline;

    // Delta rows are keyed "pkg Suite/Name" by the CLI; import paths never
    // contain spaces and suite/method names never contain slashes.
    const deltaByKey = new Map<string, BenchEntryDelta>();
    for (const d of report.deltas ?? []) {
      deltaByKey.set(d.key, {
        percentChange: d.percentChange,
        significant: d.significant,
        insufficientSample: d.insufficientSample,
      });
    }

    for (const result of report.baseline.results) {
      const n = result.samples.length;
      if (n === 0) continue;
      const mean = (pick: (s: (typeof result.samples)[number]) => number) =>
        result.samples.reduce((sum, s) => sum + pick(s), 0) / n;
      const nsValues = result.samples.map((s) => s.nsPerOp);
      const meanNs = mean((s) => s.nsPerOp);

      const key = benchKey(
        result.package,
        result.suite,
        result.name,
        goos,
        goarch,
      );
      this.entries.set(key, {
        nsPerOp: meanNs,
        bytesPerOp: Math.round(mean((s) => s.bytesPerOp)),
        allocsPerOp: Math.round(mean((s) => s.allocsPerOp)),
        iterations: result.samples[0].iterations,
        sampleCount: n,
        minNsPerOp: Math.min(...nsValues),
        maxNsPerOp: Math.max(...nsValues),
        recordedAt,
        goos,
        goarch,
        // A run without a comparison clears any stale delta: the verdict
        // belonged to the numbers it was computed against.
        delta: deltaByKey.get(
          `${result.package} ${result.suite}/${result.name}`,
        ),
      });

      const points = this.history.get(key) ?? [];
      points.push({ nsPerOp: meanNs, recordedAt, sampleCount: n });
      if (points.length > MAX_HISTORY) {
        points.splice(0, points.length - MAX_HISTORY);
      }
      this.history.set(key, points);
    }
    void this.persist();
    for (const listener of this.listeners) {
      listener();
    }
  }

  getLatest(
    importPath: string,
    suiteName: string,
    methodName: string,
    platform: PlatformKey,
  ): BenchEntry | undefined {
    return this.entries.get(
      benchKey(
        importPath,
        suiteName,
        methodName,
        platform.goos,
        platform.goarch,
      ),
    );
  }

  /** getHistory returns the trend for one key, oldest first. */
  getHistory(
    importPath: string,
    suiteName: string,
    methodName: string,
    platform: PlatformKey,
  ): BenchHistoryPoint[] {
    return (
      this.history.get(
        benchKey(
          importPath,
          suiteName,
          methodName,
          platform.goos,
          platform.goarch,
        ),
      ) ?? []
    );
  }

  private persist(): Thenable<void> {
    const data: StoredData = {
      version: 1,
      entries: Object.fromEntries(this.entries),
      history: Object.fromEntries(this.history),
    };
    return this.memento.update(STORAGE_KEY, data);
  }
}
