// benchReport parses and formats the versioned JSON document emitted by
// `gotest bench --json`. All statistics live in the CLI (internal/gotestbench)
// — this module only decodes and displays what the CLI computed. If the
// extension ever needs more data, the CLI grows a field; nothing is derived
// here beyond unit scaling and relative timestamps.

/** One benchmark's samples, mirroring gotestbench.Result. */
export interface BenchResult {
  package: string;
  suite: string;
  name: string;
  samples: BenchSample[];
}

/** Mirrors gotestbench.Sample: what go test prints per benchmark run. */
export interface BenchSample {
  iterations: number;
  nsPerOp: number;
  bytesPerOp: number;
  allocsPerOp: number;
}

/** Mirrors gotestbench.Delta. Significance is the CLI's verdict, never ours. */
export interface BenchDelta {
  key: string;
  oldNs: number;
  newNs: number;
  percentChange: number;
  significant: boolean;
  insufficientSample: boolean;
}

/** Mirrors gotestbench.Gate. */
export interface BenchGate {
  thresholdPct: number;
  worstPct: number;
  worstKey?: string;
  breached: boolean;
}

export interface BenchBaseline {
  schemaVersion: number;
  createdAt?: string;
  goVersion?: string;
  goos: string;
  goarch: string;
  results: BenchResult[];
}

export interface BenchReport {
  schemaVersion: number;
  baseline: BenchBaseline;
  deltas?: BenchDelta[];
  gate?: BenchGate;
}

const SUPPORTED_SCHEMA_VERSION = 1;

/**
 * parseBenchReport decodes one `gotest bench --json` stdout document.
 * Unknown schema versions are refused outright: silently misreading a future
 * format is worse than asking the user to update the extension.
 */
export function parseBenchReport(stdout: string): BenchReport {
  let doc: unknown;
  try {
    doc = JSON.parse(stdout);
  } catch {
    const head = stdout.trimStart().slice(0, 120);
    throw new Error(`not a bench report (expected --json output): ${head}`);
  }
  const report = doc as BenchReport;
  if (report.schemaVersion !== SUPPORTED_SCHEMA_VERSION) {
    throw new Error(
      `unsupported bench report schema version ${report.schemaVersion} (extension understands ${SUPPORTED_SCHEMA_VERSION})`,
    );
  }
  if (!report.baseline || !Array.isArray(report.baseline.results)) {
    throw new Error("bench report carries no baseline results");
  }
  return report;
}

/** formatNsPerOp scales a ns/op mean into the customary go bench units. */
export function formatNsPerOp(ns: number): string {
  if (ns < 1000) return `${Math.round(ns)} ns/op`;
  if (ns < 1_000_000) return `${(ns / 1000).toFixed(2)}µs/op`;
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(2)}ms/op`;
  return `${(ns / 1_000_000_000).toFixed(2)}s/op`;
}

/** formatAge renders a relative timestamp for annotations. */
export function formatAge(then: number, now: number = Date.now()): string {
  const ms = Math.max(0, now - then);
  if (ms < 10_000) return "just now";
  if (ms < 60_000) return `${Math.round(ms / 1000)}s ago`;
  if (ms < 3_600_000) return `${Math.round(ms / 60_000)}m ago`;
  if (ms < 86_400_000) return `${Math.round(ms / 3_600_000)}h ago`;
  return `${Math.round(ms / 86_400_000)}d ago`;
}

/** The per-run summary an annotation displays (last sample's numbers). */
export interface BenchNumbers {
  iterations: number;
  nsPerOp: number;
  bytesPerOp: number;
  allocsPerOp: number;
}

/**
 * formatBenchAnnotation renders the CodeLens line above a benchmark method:
 * "1.24µs/op · 480 B/op · 3 allocs/op — 2m ago". Allocation-free benchmarks
 * skip the B/op term rather than shouting "0 B/op".
 */
export function formatBenchAnnotation(
  numbers: BenchNumbers,
  recordedAt: number,
  now: number = Date.now(),
): string {
  const parts = [formatNsPerOp(numbers.nsPerOp)];
  if (numbers.bytesPerOp > 0) {
    parts.push(`${numbers.bytesPerOp} B/op`);
  }
  parts.push(`${numbers.allocsPerOp} allocs/op`);
  return `${parts.join(" · ")} — ${formatAge(recordedAt, now)}`;
}
