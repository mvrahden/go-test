// The document `gotest audit --format=json` prints, and the view model the
// Change scope draws from it. Everything here is the document's: the order
// is the CLI's, the words are the CLI's text renderer's, and nothing is
// judged that the document did not judge.

export interface AuditLocation {
  // Relative to the module root, slash-separated.
  file: string;
  line: number;
  endLine: number;
}

export interface AuditSide {
  version?: string;
  ref?: string;
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
}

export interface AuditFinding {
  class: string;
  weakens: boolean;
  package?: string;
  subject: string;
  base: string;
  head: string;
  location?: AuditLocation;
}

export interface AuditPlace extends AuditLocation {
  path: string[];
}

export interface AuditChange {
  change: string;
  kind: string;
  package: string;
  base?: AuditPlace;
  head?: AuditPlace;
  behaviors?: number;
}

// What a run proved of a changed behavior. The fields past verdict are
// shown when a CLI sends them.
export interface AuditEvidence {
  package: string;
  path: string[];
  verdict: string;
  commit?: string;
  source?: string;
  stale?: boolean;
}

export interface AuditUnproven {
  module?: string;
  file: string;
  line: number;
}

export interface AuditDocument {
  version: string;
  schema: number;
  base: AuditSide;
  head: AuditSide;
  modules: string[];
  summary: AuditSummary;
  findings: AuditFinding[];
  behaviors: AuditChange[];
  evidence: AuditEvidence[];
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
    summary: raw.summary as AuditSummary,
    findings: list<AuditFinding>(raw.findings),
    behaviors: list<AuditChange>(raw.behaviors),
    evidence: list<AuditEvidence>(raw.evidence),
    unproven: list<AuditUnproven>(raw.unproven),
    incomplete: list<string>(raw.incomplete),
    notes: list<string>(raw.notes),
  };
}

// findingKey is a finding's identity in the document: its class, what it is
// about and its values. The document has no id of its own.
export function findingKey(f: AuditFinding): string {
  return ["finding", f.class, f.package ?? "", f.subject, f.base, f.head].join(
    "\u0000",
  );
}

export function evidenceKey(e: AuditEvidence): string {
  return ["evidence", e.package, e.path.join("\u0001"), e.verdict].join(
    "\u0000",
  );
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
): string {
  const m = moduleOf(doc, pkg);
  if (!m) return pkg;
  if (pkg === m) return ".";
  return "./" + pkg.slice(m.length + 1);
}

// valueText is a finding's values: base → head, or the one there is.
export function valueText(f: Pick<AuditFinding, "base" | "head">): string {
  if (f.base && f.head) return `${f.base} → ${f.head}`;
  return f.head || f.base;
}

function whereText(loc: AuditLocation | undefined): string {
  return loc && loc.line > 0 ? `${loc.file}:${loc.line}` : "";
}

// A place a row can open: a file of a module, at lines. module is undefined
// for a finding about a file, which names no package.
export interface Target {
  module?: string;
  file: string;
  line: number;
  endLine: number;
  // Which side holds the lines: a removed behavior's are the base's.
  side: "head" | "base";
}

export type Tone = "fail" | "warn" | "info" | "none";

export interface DecisionRow {
  key: string;
  // The finding's class, or the evidence's verdict.
  tag: string;
  tone: Tone;
  pkg?: string;
  // The subject's labels; the last one is the row's own.
  labels: string[];
  fact: string;
  where: string;
  target?: Target;
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
  target?: Target;
}

export interface SuiteGroup {
  suite: string;
  rows: BehaviorRow[];
}

export interface PackageGroup {
  pkg: string;
  suites: SuiteGroup[];
}

export interface UnprovenFile {
  module?: string;
  file: string;
  lines: number[];
}

export interface Counter {
  label: string;
  value: number;
  tone?: Tone | "add" | "del";
}

export interface ChangeModel {
  base: string;
  head: string;
  counters: Counter[];
  open: DecisionGroup[];
  accepted: DecisionRow[];
  openCount: number;
  acceptedCount: number;
  behaviors: PackageGroup[];
  behaviorCount: number;
  information: string[];
  evidence: { pkg: string; text: string; verdict: string; facts: string }[];
  unproven: UnprovenFile[];
  incomplete: string[];
  notes: string[];
}

export const FOLD_ABOVE = 5;

const SIGNS: Record<string, string> = {
  added: "+",
  removed: "−",
  reworded: "~",
  moved: "→",
  changed: "±",
};

// Evidence verdicts a run proved; any other asks for a decision.
const PROVEN = new Set(["pass"]);

function findingTone(f: AuditFinding): Tone {
  if (!f.weakens) return "info";
  return f.class === "removed" ? "fail" : "warn";
}

function findingTarget(
  doc: AuditDocument,
  f: AuditFinding,
): Target | undefined {
  if (!f.location?.file) return undefined;
  return {
    module: f.package ? moduleOf(doc, f.package) : undefined,
    file: f.location.file,
    line: f.location.line,
    endLine: f.location.endLine,
    side: f.class === "removed" || f.head === "deleted" ? "base" : "head",
  };
}

function placeTarget(doc: AuditDocument, c: AuditChange): Target | undefined {
  const p = c.head ?? c.base;
  if (!p) return undefined;
  return {
    module: moduleOf(doc, c.package),
    file: p.file,
    line: p.line,
    endLine: p.endLine,
    side: c.head ? "head" : "base",
  };
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

// buildChangeModel lays the document out for the panel: the decisions first,
// the weakenings in the document's order with the unproven evidence after
// them, then the behaviors by package and suite. accepted says which rows
// the reviewer acknowledged here.
export function buildChangeModel(
  doc: AuditDocument,
  accepted: (key: string) => boolean = () => false,
): ChangeModel {
  const rows: DecisionRow[] = [];
  for (const f of doc.findings) {
    if (!f.weakens) continue;
    const key = findingKey(f);
    rows.push({
      key,
      tag: f.class,
      tone: findingTone(f),
      pkg: f.package ? shortPackage(doc, f.package) : undefined,
      labels: f.subject.split(" / "),
      fact: f.class === "removed" ? f.base : valueText(f),
      where: whereText(f.location),
      target: findingTarget(doc, f),
      accepted: accepted(key),
    });
  }
  for (const e of doc.evidence) {
    if (PROVEN.has(e.verdict) && !e.stale) continue;
    const key = evidenceKey(e);
    rows.push({
      key,
      tag: e.stale ? "stale" : e.verdict,
      tone: e.verdict === "fail" ? "fail" : "warn",
      pkg: shortPackage(doc, e.package),
      labels: e.path,
      fact: evidenceFacts(e),
      where: "",
      accepted: accepted(key),
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
  for (const [i, c] of doc.behaviors.entries()) {
    const pkg = shortPackage(doc, c.package);
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
      target: placeTarget(doc, c),
    });
  }

  // The informational findings the behaviors do not show already.
  const information = doc.findings
    .filter((f) => !f.weakens && f.class !== "changed")
    .map((f) => `${f.class}  ${findingLine(doc, f)}`);

  const unproven: UnprovenFile[] = [];
  for (const u of doc.unproven) {
    let file = unproven.find((x) => x.file === u.file && x.module === u.module);
    if (!file) {
      file = { module: u.module, file: u.file, lines: [] };
      unproven.push(file);
    }
    file.lines.push(u.line);
  }

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
  if (s.evidence > 0) counters.push({ label: "evidence", value: s.evidence });
  if (s.unproven > 0)
    counters.push({ label: "unproven", value: s.unproven, tone: "warn" });
  if (s.incomplete > 0)
    counters.push({ label: "not compared", value: s.incomplete, tone: "fail" });

  return {
    base: sideText(doc.base),
    head: sideText(doc.head),
    counters,
    open,
    accepted: acceptedRows,
    openCount: rows.length - acceptedRows.length,
    acceptedCount: acceptedRows.length,
    behaviors,
    behaviorCount: doc.behaviors.length,
    information,
    evidence: doc.evidence.map((e) => ({
      pkg: shortPackage(doc, e.package),
      text: e.path.join(" / "),
      verdict: e.stale ? "stale" : e.verdict,
      facts: evidenceFacts(e),
    })),
    unproven,
    incomplete: doc.incomplete,
    notes: doc.notes,
  };
}

function evidenceFacts(e: AuditEvidence): string {
  const parts: string[] = [];
  if (e.source) parts.push(e.source);
  if (e.commit) parts.push(`@${e.commit}`);
  return parts.join(" ");
}

// findingLine is a finding's line as the CLI's text writes it.
export function findingLine(doc: AuditDocument, f: AuditFinding): string {
  const loc = whereText(f.location);
  if (!f.package) return `${loc || f.subject} ${valueText(f)}`;
  const text = `${shortPackage(doc, f.package)} ${f.subject}`;
  if (f.class === "removed") return `${text} (${f.base}, ${loc})`;
  return `${text}: ${valueText(f)} (${loc})`;
}

// targets lists what each row of the model opens, by the row's key.
export function targetsOf(m: ChangeModel): Map<string, Target> {
  const out = new Map<string, Target>();
  const rows = [...m.open.flatMap((g) => g.rows), ...m.accepted];
  for (const r of rows) if (r.target) out.set(r.key, r.target);
  for (const p of m.behaviors)
    for (const s of p.suites)
      for (const r of s.rows) if (r.target) out.set(r.key, r.target);
  return out;
}
