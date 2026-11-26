// What a test run in progress has streamed so far: how many test events of
// each kind it applied, what each package's tests said, and its stream as
// run pieces. The panel shows these until the run's end has the CLI render
// it; what a test's verdict means stays the CLI's to say.
import type { TestEvent } from "./outputParser.js";
import type { RunPiece } from "./specStore.js";

// Test events applied so far: run, pass and fail of every test, subtests
// included, as the stream carried them.
export interface Tally {
  started: number;
  passed: number;
  failed: number;
}

// countEvent adds one applied event to t. The package's own verdicts and the
// CLI's run-level events are not a test's.
export function countEvent(t: Tally, ev: TestEvent): void {
  if (!ev.Test || ev.RunLevel === true) return;
  if (ev.Action === "run") t.started++;
  else if (ev.Action === "pass") t.passed++;
  else if (ev.Action === "fail") t.failed++;
}

// What a run said so far of one package's tests, by go test name.
export interface LivePackage {
  tests: Map<string, LiveTest>;
  // Tests that have tests of their own.
  parents: Set<string>;
  // Tests one of whose own tests failed.
  failedParents: Set<string>;
  // The package's own verdict, once it came.
  done?: "pass" | "fail" | "skip";
}

export interface LiveTest {
  status: "run" | "pass" | "fail" | "skip";
  // What it printed, while it runs and once it failed.
  output: string[];
}

// The lines a test in progress keeps of what it printed.
export const LIVE_OUTPUT_LINES = 40;

function ancestorsOf(test: string): string[] {
  const out: string[] = [];
  for (let i = test.indexOf("/"); i > 0; i = test.indexOf("/", i + 1))
    out.push(test.slice(0, i));
  return out;
}

// noteLive adds one event to its package's tests.
export function noteLive(
  packages: Map<string, LivePackage>,
  ev: TestEvent,
): void {
  if (ev.RunLevel === true || !ev.Package) return;
  let lp = packages.get(ev.Package);
  if (!lp) {
    if (ev.Action === "output" || ev.Action === "start") return;
    lp = { tests: new Map(), parents: new Set(), failedParents: new Set() };
    packages.set(ev.Package, lp);
  }
  if (!ev.Test) {
    if (ev.Action === "pass" || ev.Action === "fail" || ev.Action === "skip")
      lp.done = ev.Action;
    return;
  }
  let t = lp.tests.get(ev.Test);
  if (!t) {
    t = { status: "run", output: [] };
    lp.tests.set(ev.Test, t);
    for (const a of ancestorsOf(ev.Test)) lp.parents.add(a);
  }
  switch (ev.Action) {
    case "run":
      t.status = "run";
      break;
    case "output":
      if (t.status === "pass" || t.status === "skip") break;
      t.output.push((ev.Output ?? "").replace(/\n$/, ""));
      if (t.output.length > LIVE_OUTPUT_LINES) t.output.shift();
      break;
    case "pass":
    case "skip":
      t.status = ev.Action;
      t.output = [];
      break;
    case "fail":
      t.status = "fail";
      for (const a of ancestorsOf(ev.Test)) lp.failedParents.add(a);
      break;
  }
}

// tallyText is a tally as the panel words it.
export function tallyText(t: Tally): string {
  return `${t.started} started · ${t.passed} passed · ${t.failed} failed`;
}

// elapsedText is a duration in ms as m:ss, or h:mm:ss from an hour on.
export function elapsedText(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  const two = (v: number) => String(v).padStart(2, "0");
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return h > 0 ? `${h}:${two(m)}:${two(s % 60)}` : `${m}:${two(s % 60)}`;
}

// RunProgress is one test run in progress: one or more CLI invocations, one
// after the other.
export class RunProgress {
  readonly tally: Tally = { started: 0, passed: 0, failed: 0 };
  // Each package's tests as the run reported them so far.
  readonly packages = new Map<string, LivePackage>();
  readonly startedAt: number;
  private readonly done: RunPiece[] = [];
  private current: (() => RunPiece) | undefined;
  private ended = false;

  constructor(
    // What the run is, as its session names it.
    readonly label: string,
    private readonly changed: (p: RunProgress) => void,
    now = Date.now(),
  ) {
    this.startedAt = now;
  }

  get active(): boolean {
    return !this.ended;
  }

  // note counts an event the run applied.
  note(ev: TestEvent): void {
    if (this.ended) return;
    countEvent(this.tally, ev);
    noteLive(this.packages, ev);
    this.changed(this);
  }

  // streaming names the invocation now running: piece is its stream so far.
  streaming(piece: () => RunPiece): void {
    this.current = piece;
  }

  // finished keeps an invocation's whole stream once it ended.
  finished(piece: RunPiece): void {
    this.done.push(piece);
    this.current = undefined;
  }

  // pieces is what the run has streamed so far, in the order it ran.
  pieces(): RunPiece[] {
    const now = this.current?.();
    return now ? [...this.done, now] : [...this.done];
  }

  // folders are those the run has streamed into so far.
  folders(): string[] {
    return Array.from(new Set(this.pieces().map((p) => p.folder)));
  }

  end(): void {
    if (this.ended) return;
    this.ended = true;
    this.current = undefined;
    this.changed(this);
  }
}

// LiveRuns are the window's test runs in progress.
export class LiveRuns {
  private readonly runs = new Set<RunProgress>();
  private readonly listeners = new Set<(run: RunProgress) => void>();

  // onDidChange is told of a run that started, applied an event or ended.
  onDidChange(listener: (run: RunProgress) => void): { dispose(): void } {
    this.listeners.add(listener);
    return { dispose: () => this.listeners.delete(listener) };
  }

  // start lists a run until it ends.
  start(label: string, now = Date.now()): RunProgress {
    const run = new RunProgress(
      label,
      (r) => {
        if (!r.active) this.runs.delete(r);
        for (const fn of Array.from(this.listeners)) fn(r);
      },
      now,
    );
    this.runs.add(run);
    for (const fn of Array.from(this.listeners)) fn(run);
    return run;
  }

  active(): RunProgress[] {
    return Array.from(this.runs);
  }
}

// The window's runs in progress, which the runners add to.
export const liveRuns = new LiveRuns();
