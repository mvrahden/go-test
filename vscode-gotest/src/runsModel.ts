// The document `gotest runs --format=json` prints, and the words the Runs
// tab draws from it. The CLI keeps the records and judges each against the
// working tree; this only words what it says.
import * as path from "node:path";

export interface RunsTally {
  passed: number;
  failed: number;
  skipped: number;
}

export interface RunsManifest {
  version?: string;
  goVersion?: string;
  args: string[];
  commit?: string;
  dirty?: boolean;
  agent?: string;
  build?: string[];
  // A --changed run: how many packages it ran and left out.
  changed?: {
    ran: number;
    unchanged: number;
    noRecord?: boolean;
    packages?: string[];
  };
}

export interface RunsEntry {
  id: string;
  started?: string;
  recorded: string;
  imported?: boolean;
  eventsFile?: string;
  manifest: RunsManifest;
  result: {
    exitCode: number;
    packages: RunsTally;
    // Absent for a run that kept no test counts (a text run).
    tests?: RunsTally;
    notRun?: number;
    vacuous?: number;
  };
  // The tests that failed, named down to their method.
  failed: { package: string; test?: string }[];
  // Each package the run ran, with the hash of its inputs; absent from a
  // record an older CLI wrote.
  ran?: { package: string; status: string; inputHash?: string }[];
  // The record's packages by whether their verdict belongs to the tree now.
  packages: { total: number; current: number; stale: number; unknown: number };
}

export interface RunsDocument {
  version: string;
  dir: string;
  runs: RunsEntry[];
}

const isObj = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";

// parseRunsDocument reads the CLI's stdout; undefined when it is not the
// document. A record's lists of another shape read as absent.
export function parseRunsDocument(stdout: string): RunsDocument | undefined {
  let raw: Record<string, unknown>;
  try {
    raw = JSON.parse(stdout.trim()) as Record<string, unknown>;
  } catch {
    return undefined;
  }
  if (!isObj(raw) || !Array.isArray(raw.runs)) return undefined;
  const runs: RunsEntry[] = [];
  for (const r of raw.runs as unknown[]) {
    if (!isObj(r) || typeof r.id !== "string") continue;
    if (!isObj(r.manifest) || !isObj(r.result)) continue;
    const e = r as unknown as RunsEntry;
    const failed = Array.isArray(r.failed)
      ? (r.failed as unknown[]).filter(
          (f): f is { package: string; test?: string } =>
            isObj(f) && typeof f.package === "string",
        )
      : [];
    const ran = Array.isArray(r.ran)
      ? (r.ran as unknown[]).filter(
          (x): x is { package: string; status: string; inputHash?: string } =>
            isObj(x) && typeof x.package === "string",
        )
      : undefined;
    const packages = isObj(r.packages)
      ? (r.packages as RunsEntry["packages"])
      : { total: 0, current: 0, stale: 0, unknown: 0 };
    const tests = isObj(e.result.tests) ? e.result.tests : undefined;
    runs.push({
      ...e,
      manifest: {
        ...e.manifest,
        args: Array.isArray(e.manifest.args) ? e.manifest.args : [],
      },
      result: { ...e.result, tests },
      failed,
      ran,
      packages,
    } as RunsEntry);
  }
  return {
    version: typeof raw.version === "string" ? raw.version : "",
    dir: typeof raw.dir === "string" ? raw.dir : "",
    runs,
  };
}

const two = (v: number) => String(v).padStart(2, "0");
const num = (n: number) => n.toLocaleString("en-US");
const pkgs = (n: number) => `${num(n)} package${n === 1 ? "" : "s"}`;

// whenText is a record's local time: HH:MM today, else the date too.
export function whenText(at: Date, now: Date): string {
  const time = `${two(at.getHours())}:${two(at.getMinutes())}`;
  return sameDay(at, now) ? time : `${dayText(at, now)} ${time}`;
}

function sameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

// dayText names a record's day: today, yesterday, or its date.
export function dayText(at: Date, now: Date): string {
  if (sameDay(at, now)) return "today";
  const y = new Date(now);
  y.setDate(y.getDate() - 1);
  if (sameDay(at, y)) return "yesterday";
  return `${at.getFullYear()}-${two(at.getMonth() + 1)}-${two(at.getDate())}`;
}

// narrowedText says how a --changed run narrowed itself.
export function narrowedText(m: RunsManifest): string | undefined {
  const c = m.changed;
  if (!c) return undefined;
  if (c.noRecord) return `Run changed found no record, ran all ${num(c.ran)}`;
  return `Run changed: ran ${pkgs(c.ran)}, left out ${num(c.unchanged)} unchanged`;
}

// agentName words an agent's id as the run record stamps it.
export function agentName(agent: string): string {
  const head = agent.split("_")[0] || agent;
  return head
    .split("-")
    .map((w) => (w ? w[0].toUpperCase() + w.slice(1) : w))
    .join(" ");
}

// fromEditor says whether the extension started a run: it always holds the
// run's stdin for its stop.
export const fromEditor = (args: string[] | undefined): boolean =>
  (args ?? []).includes("--control=stdin");

// whoText says who started a record's run.
export function whoText(
  r: Pick<RunsEntry, "imported" | "eventsFile" | "manifest">,
): string {
  const agent = r.manifest.agent
    ? `Agent (${agentName(r.manifest.agent)})`
    : "";
  if (r.imported) {
    const from = r.eventsFile
      ? `Imported from ${path.basename(r.eventsFile)}`
      : "Imported";
    return agent ? `${from}, ${agent.toLowerCase()}` : from;
  }
  // The editor's own run, though an agent's environment started the editor.
  if (fromEditor(r.manifest.args)) return "You, from this editor";
  if (r.manifest.agent) return `Agent (${agentName(r.manifest.agent)})`;
  return "From a terminal";
}

// The go test flags whose value is the next argument when not joined by =.
const VALUE_FLAGS = new Set([
  "run",
  "skip",
  "count",
  "timeout",
  "bench",
  "benchtime",
  "tags",
  "p",
  "parallel",
  "coverprofile",
  "covermode",
  "coverpkg",
  "cpu",
  "o",
  "exec",
  "fuzz",
  "fuzztime",
  "shuffle",
]);

// commandParts reads a run's patterns and its -run filter from its args.
export function commandParts(args: string[]): {
  patterns: string[];
  run?: string;
  changed: boolean;
} {
  const patterns: string[] = [];
  let run: string | undefined;
  let changed = false;
  for (let i = 0; i < args.length; i++) {
    const a = args[i];
    if (a === "--") continue;
    if (a === "--changed") changed = true;
    if (!a.startsWith("-")) {
      patterns.push(a);
      continue;
    }
    const m = /^--?([\w.-]+)(=(.*))?$/.exec(a);
    if (!m) continue;
    const value =
      m[2] !== undefined ? m[3] : VALUE_FLAGS.has(m[1]) ? args[++i] : undefined;
    if (m[1] === "run" && value !== undefined) run = value;
  }
  return { patterns, run, changed };
}

// levelLabel words a go test name level as the spec does: a suite without
// Test and TestSuite, a method without Test, underscores as spaces.
export function levelLabel(level: string): string {
  let s = level.replace(/#\d+$/, "");
  if (/^Test.+TestSuite$/.test(s)) s = s.slice(4, -9);
  else if (/^Test[A-Z]/.test(s)) s = s.slice(4);
  return s.replace(/_/g, " ");
}

function filterLabel(run: string): string {
  const parts = run
    .split("/")
    .map((p) => p.replace(/^\^/, "").replace(/\$$/, ""));
  if (parts.every((p) => /^[\w#]+$/.test(p)))
    return parts.map(levelLabel).join(" › ");
  return run;
}

function scopeText(patterns: string[]): string {
  if (
    patterns.length === 0 ||
    patterns.some((p) => p === "./..." || p === "...")
  )
    return "Everything";
  const names = patterns.map(
    (p) =>
      p
        .replace(/^\.\//, "")
        .replace(/\/\.\.\.$/, "/…")
        .replace(/\/$/, "") || ".",
  );
  if (names.length <= 2) return names.join(", ");
  return `${names.slice(0, 2).join(", ")} and ${names.length - 2} more`;
}

// runTitle names what a record's run ran, in words.
export function runTitle(m: RunsManifest): string {
  const parts = commandParts(m.args);
  let title = scopeText(parts.patterns);
  const narrowed = narrowedText(m);
  if (m.changed?.noRecord) title += `: ${narrowed} packages`;
  else if (narrowed) title = narrowed;
  else if (parts.changed) title = "Run changed";
  if (parts.run) title += `, only ${filterLabel(parts.run)}`;
  return title;
}

// countsText words a record's tests; a record without counts says so, never
// zero passed.
export function countsText(r: RunsEntry["result"]): string {
  const t = r.tests;
  const pk = r.packages;
  if (
    !t ||
    (t.passed + t.failed + t.skipped === 0 && pk.passed + pk.failed > 0)
  )
    return "no test counts kept";
  const parts = [`${num(t.passed)} passed`];
  if (t.failed > 0) parts.push(`${num(t.failed)} failed`);
  if (t.skipped > 0) parts.push(`${num(t.skipped)} skipped`);
  if ((r.notRun ?? 0) > 0) parts.push(`${num(r.notRun!)} not run`);
  return parts.join(" · ");
}

// verdictText is a record's verdict by its exit code.
export function verdictText(exitCode: number): string {
  if (exitCode === 0) return "Passed";
  if (exitCode === 1) return "Failed";
  return `Failed, exit ${exitCode}`;
}

// freshText is what a record says of the tree now, as the CLI counts it.
export function freshText(p: RunsEntry["packages"]): {
  text: string;
  tone: "pass" | "warn" | "none";
} {
  if (p.total === 0) return { text: "no packages", tone: "none" };
  if (p.stale > 0)
    return { text: `${p.stale} of ${pkgs(p.total)} stale`, tone: "warn" };
  if (p.current === p.total)
    return { text: `${pkgs(p.total)}, current`, tone: "pass" };
  if (p.unknown === p.total)
    return { text: `${pkgs(p.total)}, freshness unknown`, tone: "none" };
  return { text: `${p.unknown} of ${pkgs(p.total)} unknown`, tone: "none" };
}

// judgesText is the CLI's count of a record's packages against the tree.
export function judgesText(p: RunsEntry["packages"]): string {
  return `judges this tree: ${p.current} current / ${p.stale} stale / ${p.unknown} unknown`;
}

// isCurrent says whether every package of a record still belongs to the tree.
export const isCurrent = (r: RunsEntry): boolean =>
  r.packages.total > 0 && r.packages.current === r.packages.total;

// A run taken as the Change tab's evidence: its events file, and how it is
// named.
export interface EvidencePick {
  id: string;
  file: string;
  label: string;
}

// evidenceLabel names a run taken as evidence: who ran it and when.
export function evidenceLabel(r: RunsEntry, now: Date): string {
  const at = new Date(r.recorded);
  const when = isNaN(at.getTime()) ? r.recorded : whenText(at, now);
  if (r.imported) return `the imported run of ${when}`;
  if (fromEditor(r.manifest.args)) return `the editor run of ${when}`;
  if (r.manifest.agent)
    return `the run of ${when} by ${agentName(r.manifest.agent)}`;
  return `the terminal run of ${when}`;
}

// newestCurrent is the newest record whose every package is current and
// whose events file is there: the evidence an audit takes unless runs are
// chosen.
export function newestCurrent(
  doc: RunsDocument | undefined,
  exists: (file: string) => boolean,
  now: Date,
): EvidencePick | undefined {
  const r = doc?.runs.find(
    (r) => isCurrent(r) && !!r.eventsFile && exists(r.eventsFile),
  );
  return r
    ? { id: r.id, file: r.eventsFile!, label: evidenceLabel(r, now) }
    : undefined;
}

// flakyHint says how a failed test fared in the other records that ran its
// package with the same inputs; undefined when the records cannot say. A
// record names a failure down to its method, so that is what is matched.
export function flakyHint(
  doc: RunsDocument | undefined,
  pkg: string,
  test: string,
  inputHash: string | undefined,
  skip: (r: RunsEntry) => boolean = () => false,
): string | undefined {
  if (!doc || !inputHash) return undefined;
  const method = test.split("/").slice(0, 2).join("/");
  let n = 0;
  let passed = 0;
  for (const r of doc.runs) {
    if (skip(r)) continue;
    if (!r.ran?.some((x) => x.package === pkg && x.inputHash === inputHash))
      continue;
    n++;
    const failedIt = r.failed.some(
      (f) => f.package === pkg && (f.test === undefined || f.test === method),
    );
    if (!failedIt) passed++;
  }
  if (n === 0) return undefined;
  const runs = `${n} other run${n === 1 ? "" : "s"} on this same tree`;
  if (passed > 0) return `Passed in ${passed} of ${runs}. Likely flaky.`;
  return n === 1 ? `Failed in the ${runs} too.` : `Failed in all ${runs} too.`;
}
