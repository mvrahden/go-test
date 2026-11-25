// The document `gotest audit --format=json` prints, and the view model the
// Change scope draws from it. Everything here is the document's: the order
// is the CLI's, the words are the CLI's text renderer's, and nothing is
// judged that the document did not judge.

// The audit document's schema this extension reads.
export const AUDIT_SCHEMA = 1;

export interface AuditLocation {
  // Relative to the module root, slash-separated.
  file: string;
  line: number;
  endLine: number;
}

export interface AuditSide {
  version?: string;
  ref?: string;
  branch?: string;
  commit?: string;
  dirty?: boolean;
  patterns?: string[];
}

export interface AuditSummary {
  weakenings: number;
  informational: number;
  classes?: Record<string, number>;
  added: number;
  removed: number;
  reworded: number;
  moved: number;
  changed: number;
  evidence: number;
  unproven: number;
  incomplete: number;
  failing?: number;
  notRun?: number;
  vacuous?: number;
  stale?: number;
  changedLines?: number;
  noCoverage?: number;
}

// The go test name of a test and the -run pattern that selects it.
export interface Selector {
  test?: string;
  run?: string;
}

export interface AuditFinding extends Selector {
  id: string;
  class: string;
  weakens: boolean;
  package?: string;
  subject: string;
  base: string;
  head: string;
  module?: string;
  location?: AuditLocation;
  baseLocation?: AuditLocation;
  headLocation?: AuditLocation;
  runtime?: { base: number | null; head: number | null };
  // The id of the evidence row that judges the finding's behavior.
  evidence?: string;
}

export interface AuditPlace extends AuditLocation {
  path: string[];
}

export interface AuditChange extends Selector {
  change: string;
  kind: string;
  package: string;
  base?: AuditPlace;
  head?: AuditPlace;
  behaviors?: number;
  module?: string;
  // The id of the evidence row that judges the behavior.
  evidence?: string;
  // The behavior's place in the CLI's text list.
  order?: number;
}

export interface StreamRun {
  version?: string;
  goVersion?: string;
  args?: string[];
  commit?: string;
  dirty?: boolean;
  agent?: string;
  exitCode?: number;
}

export interface AuditStream {
  file?: string;
  side: string;
  ran?: boolean;
  run: StreamRun | null;
  current: boolean;
  stale?: string;
}

export interface Leaves {
  passed: number;
  failed: number;
  skipped: number;
  notRun: number;
  vacuous: number;
}

// What the head's runs say of one behavior the comparison names.
export interface AuditEvidence extends Selector {
  id: string;
  package: string;
  path: string[];
  verdict: string;
  kind?: string;
  why?: string[];
  assertions?: number;
  vacuous?: boolean;
  assertsBy?: string;
  skipReason?: string;
  runs?: number;
  leaves?: Leaves;
  stream?: number;
  current: boolean;
  stale?: string;
  others?: { stream: number; verdict: string; current: boolean }[];
  needsDecision: boolean;
  source?: string;
  commit?: string;
  module?: string;
  // Where the head declares the behavior, relative to its module.
  location?: AuditLocation;
  // The CLI's own mark for the row, as its text writes it.
  mark?: string;
}

export interface AuditCoverage {
  profile?: string;
  lines: string;
  packages: string[];
}

export interface AuditUnproven {
  module?: string;
  file: string;
  package: string;
  line: number;
  coverage: boolean;
  changed: number;
  unexecuted: number;
  ranges: { line: number; endLine: number }[];
}

export interface ModuleDir {
  path: string;
  // Relative to the audited directory, slash-separated, "." for it.
  dir: string;
}

export interface AuditDocument {
  version: string;
  schema: number;
  base: AuditSide;
  head: AuditSide;
  modules: string[];
  moduleDirs: ModuleDir[];
  summary: AuditSummary;
  findings: AuditFinding[];
  behaviors: AuditChange[];
  streams: AuditStream[];
  evidence: AuditEvidence[];
  coverage?: AuditCoverage;
  unproven: AuditUnproven[];
  incomplete: string[];
  notes: string[];
}

// parseAuditDocument reads the CLI's stdout; undefined when it holds no
// audit document. Lists the CLI leaves out read as empty.
export function parseAuditDocument(stdout: string): AuditDocument | undefined {
  const text = stdout.trim();
  if (!text.startsWith("{")) return undefined;
  let raw: Record<string, unknown>;
  try {
    raw = JSON.parse(text) as Record<string, unknown>;
  } catch {
    return undefined;
  }
  if (
    !raw ||
    typeof raw.schema !== "number" ||
    typeof raw.summary !== "object" ||
    raw.summary === null
  ) {
    return undefined;
  }
  const list = <T>(v: unknown): T[] => (Array.isArray(v) ? (v as T[]) : []);
  const side = (v: unknown): AuditSide =>
    v && typeof v === "object" ? (v as AuditSide) : {};
  return {
    version: typeof raw.version === "string" ? raw.version : "",
    schema: raw.schema,
    base: side(raw.base),
    head: side(raw.head),
    modules: list<string>(raw.modules),
    moduleDirs: list<ModuleDir>(raw.moduleDirs),
    summary: raw.summary as AuditSummary,
    findings: list<AuditFinding>(raw.findings),
    behaviors: list<AuditChange>(raw.behaviors),
    streams: list<AuditStream>(raw.streams),
    evidence: list<AuditEvidence>(raw.evidence),
    coverage:
      raw.coverage && typeof raw.coverage === "object"
        ? (raw.coverage as AuditCoverage)
        : undefined,
    unproven: list<AuditUnproven>(raw.unproven),
    incomplete: list<string>(raw.incomplete),
    notes: list<string>(raw.notes),
  };
}

// sideText names a side as the CLI's text does: ref and commit, and whether
// the tree had uncommitted changes.
export function sideText(s: AuditSide): string {
  let name = s.commit ?? "";
  if (s.ref && s.ref !== s.commit) name = `${s.ref} ${s.commit ?? ""}`.trim();
  if (!name) name = "(no commit)";
  if (s.dirty) name += " with uncommitted changes";
  return name;
}

// moduleOf is the longest module path the document lists that holds pkg.
export function moduleOf(
  doc: Pick<AuditDocument, "modules">,
  pkg: string,
): string | undefined {
  let best: string | undefined;
  for (const m of doc.modules) {
    if (
      (pkg === m || pkg.startsWith(m + "/")) &&
      m.length > (best ?? "").length
    )
      best = m;
  }
  return best;
}

// shortPackage is a package's path in its module, "." for the module's root.
export function shortPackage(
  doc: Pick<AuditDocument, "modules">,
  pkg: string,
  module?: string,
): string {
  const m = module ?? moduleOf(doc, pkg);
  if (!m) return pkg;
  if (pkg === m) return ".";
  return "./" + pkg.slice(m.length + 1);
}

function countText(n: number | null | undefined): string {
  return typeof n === "number" ? String(n) : "?";
}

// valueText is a finding's values: base → head, or the one there is, and
// the counts the runs reported.
export function valueText(
  f: Pick<AuditFinding, "base" | "head" | "runtime">,
): string {
  let text = f.base;
  if (f.base && f.head) text = `${f.base} → ${f.head}`;
  else if (f.head) text = f.head;
  if (f.runtime)
    text += ` (run time ${countText(f.runtime.base)} → ${countText(f.runtime.head)})`;
  return text;
}

function whereText(loc: AuditLocation | undefined): string {
  return loc && loc.line > 0 ? `${loc.file}:${loc.line}` : "";
}

// markOf is an evidence row's mark as the CLI's text writes it: its
// verdict, the assertions a passing leaf made, and * for a stale stream.
export function markOf(e: AuditEvidence | undefined): string {
  if (!e) return "";
  if (typeof e.mark === "string") return e.mark;
  return legacyMark(e);
}

// --- For a document without the link fields (evidence, module, mark,
// order): what the CLI's text does, re-derived. Delete once every CLI the
// extension reads writes them.

function legacyMark(e: AuditEvidence): string {
  let m = e.verdict;
  if (e.verdict === "fail") m = "FAIL";
  else if (e.verdict === "pass" && e.vacuous) m = "vacuous";
  else if (e.verdict === "pass" && e.leaves && e.leaves.notRun > 0)
    m = "partly run";
  else if (e.verdict === "pass" && e.assertions !== undefined && !e.leaves)
    m = `pass ${e.assertions}`;
  if (e.stream !== undefined && !e.current) m += "*";
  return m;
}

// legacyLinks finds a row's evidence as the CLI's text finds it: by package
// and labels; a finding's subject is its head labels joined.
function legacyLinks(doc: AuditDocument): {
  finding: (f: AuditFinding) => AuditEvidence | undefined;
  behavior: (c: AuditChange) => AuditEvidence | undefined;
} {
  const ev = new Map<string, AuditEvidence>();
  const bySubject = new Map<string, AuditEvidence>();
  for (const e of doc.evidence) {
    ev.set(pathKey(e.package, e.path), e);
    bySubject.set(pathKey(e.package, [e.path.join(" / ")]), e);
  }
  return {
    finding: (f) =>
      f.package && f.headLocation
        ? bySubject.get(pathKey(f.package, [f.subject]))
        : undefined,
    behavior: (c) =>
      c.head ? ev.get(pathKey(c.package, c.head.path)) : undefined,
  };
}

// --- End of the fallback.

// hasLinks says the document links rows to evidence itself: every evidence
// row of such a document carries its mark.
function hasLinks(doc: AuditDocument): boolean {
  return doc.evidence.some((e) => typeof e.mark === "string");
}

// linksOf finds each row's evidence row, by the id the row names.
function linksOf(doc: AuditDocument): ReturnType<typeof legacyLinks> {
  if (!hasLinks(doc)) return legacyLinks(doc);
  const byId = new Map(doc.evidence.map((e) => [e.id, e]));
  return {
    finding: (f) => (f.evidence ? byId.get(f.evidence) : undefined),
    behavior: (c) => (c.evidence ? byId.get(c.evidence) : undefined),
  };
}

// behaviorOrder lists the behaviors' indexes in the CLI's text order.
function behaviorOrder(doc: AuditDocument): number[] {
  const idx = doc.behaviors.map((_, i) => i);
  if (!doc.behaviors.every((c) => typeof c.order === "number")) return idx;
  return idx.sort((a, b) => doc.behaviors[a].order! - doc.behaviors[b].order!);
}

export type Tone = "fail" | "warn" | "info" | "pass" | "none";

function markTone(e: AuditEvidence): Tone {
  if (e.verdict === "fail") return "fail";
  if (e.needsDecision) return "warn";
  return e.verdict === "pass" ? "pass" : "none";
}

// A row's evidence: the mark, and what the row judged by says beside it.
export interface Mark {
  text: string;
  tone: Tone;
  detail: string;
}

function streamLabel(st: AuditStream | undefined): string {
  if (!st) return "";
  if (st.ran) return "this audit's run";
  return st.file ?? "";
}

function markFor(
  doc: AuditDocument,
  e: AuditEvidence | undefined,
): Mark | undefined {
  if (!e) return undefined;
  const parts: string[] = [];
  const st = e.stream !== undefined ? doc.streams[e.stream] : undefined;
  if (st) {
    let by = `judged by ${streamLabel(st)}`;
    if (e.commit) by += ` of ${e.commit}`;
    parts.push(
      by + (e.current ? "" : ` (stale: ${e.stale ?? st.stale ?? ""})`),
    );
  }
  if (e.runs) parts.push(`${e.runs} runs`);
  if (e.leaves) {
    const l = e.leaves;
    parts.push(
      `leaves: ${l.passed} passed, ${l.failed} failed, ${l.skipped} skipped, ${l.notRun} not run, ${l.vacuous} vacuous`,
    );
  }
  if (e.assertsBy) parts.push(`AssertsBy: ${e.assertsBy}`);
  if (e.skipReason) parts.push(`skipped: ${e.skipReason}`);
  for (const o of e.others ?? []) {
    parts.push(
      `${streamLabel(doc.streams[o.stream])}: ${o.verdict}${o.current ? "" : " (stale)"}`,
    );
  }
  return { text: markOf(e), tone: markTone(e), detail: parts.join("\n") };
}

// A place a row can open: a file of a module, as each side has it. The base
// side is read from git, the head side is the working tree.
export interface Target {
  module?: string;
  base?: AuditLocation;
  head?: AuditLocation;
}

// What a Run button runs: go test's -run pattern in a package of the head.
export interface RunTarget {
  package: string;
  run: string;
}

export interface DecisionRow {
  // The finding's or the evidence row's id.
  key: string;
  // The finding's class, or the evidence's mark.
  tag: string;
  tone: Tone;
  pkg?: string;
  // The subject's labels; the last one is the row's own.
  labels: string[];
  fact: string;
  where: string;
  mark?: Mark;
  target?: Target;
  runs?: RunTarget;
  accepted: boolean;
}

export interface DecisionGroup {
  tag: string;
  rows: DecisionRow[];
  // A class with more rows than this is folded behind its count.
  folded: boolean;
}

export interface BehaviorRow {
  // "b:" and the change's index in the document.
  key: string;
  change: string;
  sign: string;
  kind: string;
  // Labels below the suite, as the CLI's text joins them.
  text: string;
  where: string;
  mark?: Mark;
  target?: Target;
  runs?: RunTarget;
}

export interface SuiteGroup {
  suite: string;
  rows: BehaviorRow[];
}

export interface PackageGroup {
  pkg: string;
  suites: SuiteGroup[];
}

export interface StreamRow {
  current: boolean;
  // The CLI's text for the stream: its file or the audit's run, side,
  // commit and exit code, and why it is stale.
  text: string;
  agent?: string;
}

export interface UnprovenRange {
  key: string;
  text: string;
}

export interface UnprovenFile {
  key: string;
  file: string;
  pkg: string;
  coverage: boolean;
  // "3 of 7" for a file with coverage data.
  counts: string;
  ranges: UnprovenRange[];
}

export interface Counter {
  label: string;
  value: number | string;
  tone?: Tone | "add" | "del";
}

export interface ChangeModel {
  base: string;
  head: string;
  headBranch?: string;
  counters: Counter[];
  streamsHead?: string;
  streams: StreamRow[];
  open: DecisionGroup[];
  accepted: DecisionRow[];
  openCount: number;
  acceptedCount: number;
  behaviors: PackageGroup[];
  behaviorCount: number;
  information: string[];
  // Absent when the audit looked for no unproven lines.
  unproven?: { head: string; source: string; files: UnprovenFile[] };
  incomplete: string[];
  notes: string[];
}

export const FOLD_ABOVE = 5;

// How many ranges of a file show before the rest fold, as the CLI's text.
export const MAX_RANGES = 6;

const SIGNS: Record<string, string> = {
  added: "+",
  removed: "−",
  reworded: "~",
  moved: "→",
  changed: "±",
};

function findingTone(f: AuditFinding): Tone {
  if (!f.weakens) return "info";
  return f.class === "removed" ? "fail" : "warn";
}

function findingTarget(f: AuditFinding): Target | undefined {
  if (!f.baseLocation && !f.headLocation) return undefined;
  return { module: f.module, base: f.baseLocation, head: f.headLocation };
}

function placeTarget(doc: AuditDocument, c: AuditChange): Target | undefined {
  if (!c.head && !c.base) return undefined;
  return {
    module: c.module ?? moduleOf(doc, c.package),
    base: c.base,
    head: c.head,
  };
}

// runTarget is what a row runs: its selector, when the head has the test.
function runTarget(
  pkg: string | undefined,
  s: Selector,
  inHead: boolean,
): RunTarget | undefined {
  return pkg && s.run && inHead ? { package: pkg, run: s.run } : undefined;
}

// behaviorText is a change's labels below its suite, as the CLI's text
// writes the whole path.
function behaviorText(c: AuditChange): string {
  const rest = (p: AuditPlace) => p.path.slice(1).join(" / ") || p.path[0];
  if (!c.base || !c.head) return rest((c.head ?? c.base)!);
  if (c.change === "changed") return rest(c.head);
  if (c.change === "reworded" && c.base.path.length === c.head.path.length) {
    const last = c.head.path.length - 1;
    const parent = c.head.path.slice(1, last).join(" / ");
    return `${parent ? parent + " / " : ""}${c.base.path[last]} → ${c.head.path[last]}`;
  }
  return `${rest(c.base)} → ${rest(c.head)}`;
}

function behaviorWhere(c: AuditChange): string {
  const w = (p: AuditPlace) => `${p.file}:${p.line}`;
  if (c.base && c.head && c.change !== "changed")
    return `${w(c.base)} → ${w(c.head)}`;
  const p = c.head ?? c.base;
  if (!p) return "";
  const leaves = (c.behaviors ?? 0) > 1 ? `, ${c.behaviors} behaviors` : "";
  return w(p) + leaves;
}

const pathKey = (pkg: string, path: string[]) => [pkg, ...path].join("\u0000");

// streamText is a stream's line under the CLI's evidence heading.
function streamText(st: AuditStream): string {
  let name = st.ran ? "this audit's run" : (st.file ?? "");
  if (st.side === "base") name = "base " + name;
  if (st.run?.commit) name += " of " + st.run.commit;
  if (typeof st.run?.exitCode === "number") name += `, exit ${st.run.exitCode}`;
  if (st.stale) name += ": " + st.stale;
  return name;
}

function plural(n: number): string {
  return n === 1 ? "" : "s";
}

// buildChangeModel lays the document out for the panel: the decisions first,
// the weakenings in the document's order with the evidence rows that need a
// decision after them, then the behaviors by package and suite. accepted
// says which rows the reviewer acknowledged here.
export function buildChangeModel(
  doc: AuditDocument,
  accepted: (key: string) => boolean = () => false,
): ChangeModel {
  const links = linksOf(doc);
  const findingEvidence = links.finding;

  const rows: DecisionRow[] = [];
  for (const f of doc.findings) {
    if (!f.weakens) continue;
    rows.push({
      key: f.id,
      tag: f.class,
      tone: findingTone(f),
      pkg: f.package ? shortPackage(doc, f.package, f.module) : undefined,
      labels: f.subject.split(" / "),
      fact: f.class === "removed" ? f.base : valueText(f),
      where: whereText(f.location),
      mark: markFor(doc, findingEvidence(f)),
      target: findingTarget(f),
      runs: runTarget(f.package, f, !!f.headLocation),
      accepted: accepted(f.id),
    });
  }
  for (const e of doc.evidence) {
    if (!e.needsDecision) continue;
    const mark = markFor(doc, e)!;
    rows.push({
      key: e.id,
      tag: mark.text,
      tone: e.verdict === "fail" ? "fail" : "warn",
      pkg: shortPackage(doc, e.package, e.module),
      labels: e.path,
      fact: (e.why ?? []).join(", "),
      where: "",
      mark,
      target: e.location ? { module: e.module, head: e.location } : undefined,
      runs: runTarget(e.package, e, true),
      accepted: accepted(e.id),
    });
  }

  const open: DecisionGroup[] = [];
  for (const row of rows) {
    if (row.accepted) continue;
    let group = open.find((g) => g.tag === row.tag);
    if (!group) {
      group = { tag: row.tag, rows: [], folded: false };
      open.push(group);
    }
    group.rows.push(row);
  }
  for (const g of open) g.folded = g.rows.length > FOLD_ABOVE;
  const acceptedRows = rows.filter((r) => r.accepted);

  const behaviors: PackageGroup[] = [];
  for (const i of behaviorOrder(doc)) {
    const c = doc.behaviors[i];
    const pkg = shortPackage(doc, c.package, c.module);
    const suite = (c.head ?? c.base)?.path[0] ?? "";
    let pg = behaviors.find((g) => g.pkg === pkg);
    if (!pg) {
      pg = { pkg, suites: [] };
      behaviors.push(pg);
    }
    let sg = pg.suites.find((s) => s.suite === suite);
    if (!sg) {
      sg = { suite, rows: [] };
      pg.suites.push(sg);
    }
    sg.rows.push({
      key: `b:${i}`,
      change: c.change,
      sign: SIGNS[c.change] ?? "·",
      kind: c.kind,
      text: behaviorText(c),
      where: behaviorWhere(c),
      mark: markFor(doc, links.behavior(c)),
      target: placeTarget(doc, c),
      runs: runTarget(c.package, c, !!c.head),
    });
  }

  // The informational findings the behaviors do not show already.
  const information = doc.findings
    .filter((f) => !f.weakens && f.class !== "changed")
    .map((f) => `${f.class}  ${findingLine(doc, f)}`);

  const s = doc.summary;
  const counters: Counter[] = [
    { label: "weakenings", value: s.weakenings, tone: "warn" },
    { label: "informational", value: s.informational, tone: "info" },
    { label: "added", value: s.added, tone: "add" },
    { label: "removed", value: s.removed, tone: "del" },
    { label: "reworded", value: s.reworded },
    { label: "moved", value: s.moved },
    { label: "changed", value: s.changed },
  ];
  if (doc.streams.length > 0) {
    counters.push(
      { label: "evidence", value: s.evidence },
      { label: "failing", value: s.failing ?? 0, tone: "fail" },
      { label: "not run", value: s.notRun ?? 0, tone: "warn" },
      { label: "vacuous", value: s.vacuous ?? 0, tone: "warn" },
      { label: "stale", value: s.stale ?? 0, tone: "warn" },
    );
  }
  if (doc.coverage) {
    counters.push({
      label: "unproven",
      value: `${s.unproven} of ${s.changedLines ?? 0} lines`,
      tone: s.unproven > 0 ? "warn" : undefined,
    });
    if ((s.noCoverage ?? 0) > 0)
      counters.push({
        label: "without coverage",
        value: s.noCoverage!,
        tone: "warn",
      });
  }
  if (s.incomplete > 0)
    counters.push({ label: "not compared", value: s.incomplete, tone: "fail" });

  let streamsHead: string | undefined;
  if (doc.streams.length > 0) {
    const n = doc.streams.length;
    const current = doc.streams.filter((st) => st.current).length;
    streamsHead = `evidence: ${n} stream${plural(n)}, ${current} current`;
    if (current < n) streamsHead += "; * marks a verdict of a stale one";
  }

  return {
    base: sideText(doc.base),
    head: sideText(doc.head),
    headBranch: doc.head.branch,
    counters,
    streamsHead,
    streams: doc.streams.map((st) => ({
      current: st.current,
      text: streamText(st),
      agent: st.run?.agent,
    })),
    open,
    accepted: acceptedRows,
    openCount: rows.length - acceptedRows.length,
    acceptedCount: acceptedRows.length,
    behaviors,
    behaviorCount: doc.behaviors.length,
    information,
    unproven: doc.coverage ? unprovenOf(doc) : undefined,
    incomplete: doc.incomplete,
    notes: doc.notes,
  };
}

function unprovenOf(doc: AuditDocument): ChangeModel["unproven"] {
  const s = doc.summary;
  let head = `${s.unproven} of ${s.changedLines ?? 0} changed executable lines not executed`;
  const none = s.noCoverage ?? 0;
  if (none > 0) head += `, ${none} file${plural(none)} without coverage data`;
  const cov = doc.coverage!;
  const source = `${cov.profile ? cov.profile : "the coverage of this audit's run"}, lines from ${cov.lines}`;
  const files = doc.unproven.map((u, i) => ({
    key: `u:${i}`,
    file: u.file,
    pkg: shortPackage(doc, u.package, u.module),
    coverage: u.coverage,
    counts: u.coverage ? `${u.unexecuted} of ${u.changed}` : "",
    ranges: u.ranges.map((r, j) => ({
      key: `u:${i}:${j}`,
      text: r.line === r.endLine ? `${r.line}` : `${r.line}-${r.endLine}`,
    })),
  }));
  return { head, source, files };
}

// findingLine is a finding's line as the CLI's text writes it.
export function findingLine(doc: AuditDocument, f: AuditFinding): string {
  const loc = whereText(f.location);
  if (!f.package) return `${loc || f.subject} ${valueText(f)}`;
  const text = `${shortPackage(doc, f.package, f.module)} ${f.subject}`;
  if (f.class === "removed") return `${text} (${f.base}, ${loc})`;
  return `${text}: ${valueText(f)} (${loc})`;
}

// targetsOf lists what each row of the model opens, by the row's key: the
// decisions and behaviors their file, an unproven file its first range and
// each range its lines.
export function targetsOf(
  doc: AuditDocument,
  m: ChangeModel,
): Map<string, Target> {
  const out = new Map<string, Target>();
  const rows = [...m.open.flatMap((g) => g.rows), ...m.accepted];
  for (const r of rows) if (r.target) out.set(r.key, r.target);
  for (const p of m.behaviors)
    for (const s of p.suites)
      for (const r of s.rows) if (r.target) out.set(r.key, r.target);
  for (const [i, u] of doc.unproven.entries()) {
    const first = u.ranges[0];
    out.set(`u:${i}`, {
      module: u.module,
      head: {
        file: u.file,
        line: first?.line ?? 0,
        endLine: first?.endLine ?? 0,
      },
    });
    for (const [j, r] of u.ranges.entries())
      out.set(`u:${i}:${j}`, {
        module: u.module,
        head: { file: u.file, line: r.line, endLine: r.endLine },
      });
  }
  return out;
}

// runsOf lists what each row's Run runs, by the row's key.
export function runsOf(m: ChangeModel): Map<string, RunTarget> {
  const out = new Map<string, RunTarget>();
  for (const r of [...m.open.flatMap((g) => g.rows), ...m.accepted])
    if (r.runs) out.set(r.key, r.runs);
  for (const p of m.behaviors)
    for (const s of p.suites)
      for (const r of s.rows) if (r.runs) out.set(r.key, r.runs);
  return out;
}
