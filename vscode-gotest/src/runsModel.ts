// The document `gotest runs --format=json` prints, and the rows the Runs
// scope draws from it. The CLI keeps the records and judges each against
// the working tree; this only words what it says.
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
  changed?: { Ran: number; Unchanged: number; NoRecord?: boolean };
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
    tests: RunsTally;
    notRun?: number;
    vacuous?: number;
  };
  failed: { package: string; test?: string }[];
  // The record's packages by whether their verdict belongs to the tree now.
  packages: { total: number; current: number; stale: number; unknown: number };
}

export interface RunsDocument {
  version: string;
  dir: string;
  runs: RunsEntry[];
}

// parseRunsDocument reads the CLI's stdout; undefined when it is not the
// document.
export function parseRunsDocument(stdout: string): RunsDocument | undefined {
  let raw: Record<string, unknown>;
  try {
    raw = JSON.parse(stdout.trim()) as Record<string, unknown>;
  } catch {
    return undefined;
  }
  if (!raw || !Array.isArray(raw.runs)) return undefined;
  return {
    version: typeof raw.version === "string" ? raw.version : "",
    dir: typeof raw.dir === "string" ? raw.dir : "",
    runs: (raw.runs as RunsEntry[]).filter(
      (r) => r && typeof r.id === "string" && r.manifest && r.result,
    ),
  };
}

// One run as the scope lists it.
export interface RunRow {
  id: string;
  when: string;
  // The time the record was written, in full, for the tooltip.
  whenTitle: string;
  pass: boolean;
  verdict: string;
  who: string;
  commit: string;
  args: string;
  counts: string;
  narrowed?: string;
  imported?: string;
  judges: string;
  // A tone for the judgement: stale when any package is.
  judgesTone: "pass" | "warn" | "none";
  eventsFile?: string;
  // Whether the events file is on disk to open or use.
  canOpen: boolean;
  evidence: boolean;
}

const two = (v: number) => String(v).padStart(2, "0");

// whenText is a record's local time: HH:MM today, else the date too.
export function whenText(at: Date, now: Date): string {
  const time = `${two(at.getHours())}:${two(at.getMinutes())}`;
  const same =
    at.getFullYear() === now.getFullYear() &&
    at.getMonth() === now.getMonth() &&
    at.getDate() === now.getDate();
  return same
    ? time
    : `${at.getFullYear()}-${two(at.getMonth() + 1)}-${two(at.getDate())} ${time}`;
}

// narrowedText says how a --changed run narrowed itself.
export function narrowedText(m: RunsManifest): string | undefined {
  const c = m.changed;
  if (!c) return undefined;
  if (c.NoRecord) return `--changed found no record and ran all ${c.Ran}`;
  const pkgs = (n: number) => `${n} package${n === 1 ? "" : "s"}`;
  return `--changed: ran ${pkgs(c.Ran)}, left out ${c.Unchanged} unchanged`;
}

// judgesText is what a record says of the tree now, as the CLI counts it.
export function judgesText(p: RunsEntry["packages"]): string {
  return `judges this tree: ${p.current} current / ${p.stale} stale / ${p.unknown} unknown`;
}

function countsText(r: RunsEntry["result"]): string {
  const t = r.tests;
  const parts = [`${t.passed} passed`];
  if (t.failed > 0) parts.push(`${t.failed} failed`);
  if (t.skipped > 0) parts.push(`${t.skipped} skipped`);
  if ((r.notRun ?? 0) > 0) parts.push(`${r.notRun} not run`);
  return parts.join(", ");
}

// buildRunRows lays the records out newest first, as the CLI lists them.
export function buildRunRows(
  doc: RunsDocument,
  opts: {
    now: Date;
    exists: (file: string) => boolean;
    evidence: (id: string) => boolean;
  },
): RunRow[] {
  return doc.runs.map((r) => {
    const at = new Date(r.recorded);
    const m = r.manifest;
    const pass = r.result.exitCode === 0;
    const commit = m.commit ? `${m.commit}${m.dirty ? " dirty" : ""}` : "";
    const p = r.packages;
    return {
      id: r.id,
      // A time the CLI wrote that does not parse is shown as written.
      when: isNaN(at.getTime()) ? r.recorded : whenText(at, opts.now),
      whenTitle: isNaN(at.getTime()) ? r.recorded : at.toISOString(),
      pass,
      verdict: `${pass ? "PASS" : "FAIL"} exit ${r.result.exitCode}`,
      who: m.agent ? `agent ${m.agent}` : "no agent",
      commit,
      args: ["gotest", ...m.args].join(" "),
      counts: countsText(r.result),
      narrowed: narrowedText(m),
      imported:
        r.imported && r.eventsFile ? path.basename(r.eventsFile) : undefined,
      judges: judgesText(p),
      judgesTone: p.stale > 0 ? "warn" : p.current > 0 ? "pass" : "none",
      eventsFile: r.eventsFile,
      canOpen: !!r.eventsFile && opts.exists(r.eventsFile),
      evidence: opts.evidence(r.id),
    };
  });
}

// A run taken as the Change scope's evidence: its events file, and how its
// chip names it.
export interface EvidencePick {
  id: string;
  file: string;
  label: string;
}

// evidenceLabel names a run on its chip: when it was recorded, and its
// commit when it has one.
export function evidenceLabel(row: Pick<RunRow, "when" | "commit">): string {
  return row.commit ? `${row.when} ${row.commit}` : row.when;
}
