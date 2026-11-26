// The Change tab's data: the audit document's view model laid out for the
// webview, or what stands in for it while there is none. Which findings
// are routine is one table below; everything else that weakens is a
// decision.
import {
  findingLine,
  type AuditDocument,
  type AuditFinding,
  type BehaviorRow,
  type ChangeModel,
  type DecisionRow,
} from "./auditModel.js";
import type {
  ChangeBehaviorView,
  ChangeTab,
  DecisionView,
  LinesView,
  Picker,
  RoutineGroupView,
} from "./panel/protocol.js";

// What the Change tab shows for its folder.
export type ChangeState =
  | { kind: "idle" }
  | { kind: "notGit" }
  | { kind: "noBase" }
  | {
      kind: "oldCli";
      version?: string;
      floor: string;
      advice: string;
      canUpgrade: boolean;
    }
  | { kind: "newSchema"; version?: string; schema: number; supported: number }
  | { kind: "error"; message: string }
  | {
      kind: "doc";
      model: ChangeModel;
      doc: AuditDocument;
      exitCode: number | null;
      // The audit's run was stopped: the document is incomplete, not failed.
      stopped?: boolean;
    };

export interface ChangeView {
  state: ChangeState;
  // The audit running now: a plain one, or one that runs the change.
  busy: false | "static" | "run";
  folders: Picker[];
  // The runs the audit takes as evidence: those chosen in the Runs tab, else
  // the newest run that matches the tree (auto). A missing one's events
  // file is gone and is not passed.
  evidence: { id: string; label: string; missing: boolean; auto?: boolean }[];
  // The folder's base as the user chose it.
  baseLabel?: string;
  // Said above the document, such as an audit that was stopped.
  notice?: string;
  // Which rows' buttons do something here.
  canRun: (key: string) => boolean;
  canOpen: (key: string) => boolean;
  // The run whose coverage profile the audit took, when it took one.
  coverage?: string;
  // Whether a row is accepted here, informational ones included.
  accepted?: (key: string) => boolean;
}

// A class of findings accepted as a group: what it is called, and which
// findings it takes.
export interface RoutineClass {
  id: string;
  what: string;
  takes: (f: AuditFinding) => boolean;
}

// hasReason says whether a nolint comment says why: a second // after it.
export function hasReason(comment: string): boolean {
  return /^\s*\/\/\s*nolint:\S+\s*\/\/\s*\S/.test(comment);
}

// The findings that are routine; edit this table to change what is.
export const ROUTINE: RoutineClass[] = [
  {
    id: "asserts-by",
    what: "tests now declare how they assert (AssertsBy)",
    takes: (f) => f.class === "asserts-by",
  },
  {
    id: "suppression-reason",
    what: "new nolint comments, each with a reason",
    takes: (f) => f.class === "suppression" && f.weakens && hasReason(f.head),
  },
  {
    id: "suppression-info",
    what: "nolint comments changed only their reason",
    takes: (f) => f.class === "suppression" && !f.weakens,
  },
];

export const routineOf = (f: AuditFinding): RoutineClass | undefined =>
  ROUTINE.find((r) => r.takes(f));

// The order decisions show in, by kind; evidence rows (a failing or
// unproven behavior) come first, kinds not listed last.
export const DECISION_ORDER = [
  "assertions",
  "budget",
  "snapshot",
  "removed",
  "suppression",
];

const rank = (d: DecisionView, evidence: boolean): number => {
  if (evidence) return -1;
  const i = DECISION_ORDER.indexOf(d.kind.toLowerCase());
  return i < 0 ? DECISION_ORDER.length : i;
};

// How many decisions show before the rest are counted.
export const FIRST_DECISIONS = 6;

export const ACCEPT_NOTE =
  "Accepting is a note in this workspace only. It resets when the head commit changes.";

const KIND_WORDS: Record<string, [string, string]> = {
  removed: ["removed", "removed"],
  snapshot: ["snapshot", "snapshots"],
  assertions: ["assertion count", "assertion counts"],
  budget: ["budget", "budgets"],
  suppression: ["nolint comment", "nolint comments"],
  "asserts-by": ["means of asserting", "means of asserting"],
};

const kindLabel = (tag: string) => tag.charAt(0).toUpperCase() + tag.slice(1);

function decisionOf(view: ChangeView, r: DecisionRow): DecisionView {
  const fact = r.tag === "removed" && !r.evidence ? "gone" : r.fact;
  return {
    key: r.key,
    kind: kindLabel(r.tag),
    tone: r.tone,
    what: r.labels.join(" › "),
    fact,
    where: r.where || r.pkg || "",
    open: view.canOpen(r.key) ? (r.evidence ? "source" : "diff") : undefined,
    mark: r.mark && r.mark.text !== r.tag ? r.mark : undefined,
    accepted: r.accepted,
  };
}

function countWords(rows: DecisionView[]): string {
  const by = new Map<string, number>();
  for (const r of rows)
    by.set(r.kind.toLowerCase(), (by.get(r.kind.toLowerCase()) ?? 0) + 1);
  return Array.from(by)
    .map(([k, n]) => {
      const w = KIND_WORDS[k] ?? [k, k];
      return `${n} ${n === 1 ? w[0] : w[1]}`;
    })
    .join(", ");
}

function emptyTab(view: ChangeView, extra: Partial<ChangeTab>): ChangeTab {
  return {
    folders: view.folders.length > 1 ? view.folders : undefined,
    busy: view.busy,
    canAudit: false,
    canChooseBase: view.state.kind !== "notGit",
    base: view.baseLabel,
    notice: view.notice,
    decisions: [],
    shown: 0,
    accepted: [],
    routine: [],
    packages: [],
    lists: [],
    openCount: 0,
    ...extra,
  };
}

function evidenceLine(
  view: ChangeView,
  doc: AuditDocument | undefined,
): ChangeTab["evidence"] {
  const streams = doc?.streams ?? [];
  // The audit's own run proves too, though no run was taken.
  const labels = [
    ...view.evidence.filter((e) => !e.missing).map((p) => p.label),
    ...(streams.some((s) => s.ran) ? ["this audit's run"] : []),
  ];
  const shown = view.evidence.filter((e) => !e.auto);
  if (labels.length === 0)
    return {
      text: "Nothing is proven yet: no kept run matches this tree.",
      picks: shown.map(({ id, label, missing }) => ({ id, label, missing })),
      action: { act: "auditRun", label: "Run what changed" },
    };
  const current = streams.length > 0 && streams.every((s) => s.current);
  const who =
    labels.length === 1
      ? labels[0]
      : `${labels.slice(0, -1).join(", ")} and ${labels[labels.length - 1]}`;
  const covered = view.coverage ? "; its coverage is used too" : "";
  const matches =
    streams.length === 0
      ? ""
      : current
        ? labels.length === 1
          ? ", which matches this tree"
          : ", which match this tree"
        : `, partly from before the tree changed (${streams
            .filter((st) => !st.current)
            .map(
              (st) =>
                `${st.ran ? "this audit's run" : (st.file ?? "a run")}: ${st.stale ?? "stale"}`,
            )
            .join("; ")})`;
  return {
    text: `Proven by ${who}${matches}${covered}.`,
    picks: shown.map(({ id, label, missing }) => ({ id, label, missing })),
    action: { act: "evidence", label: "Change evidence" },
  };
}

// linesOf words the changed lines no test ran, from the coverage the audit
// had: its run's, or the profile of the run taken as evidence.
export function linesOf(
  view: ChangeView,
  doc: AuditDocument,
  m: ChangeModel,
): LinesView {
  if (!doc.coverage)
    return {
      files: [],
      note: "No coverage profile is in use, so no changed line is checked: the run taken as evidence kept none.",
    };
  const s = doc.summary;
  const changed = s.changedLines ?? 0;
  const files = (m.unproven?.files ?? []).filter(
    (f) => f.coverage && f.ranges.length > 0,
  );
  const without = (m.unproven?.files ?? []).filter((f) => !f.coverage).length;
  const lines = (n: number) => `${n} changed line${n === 1 ? "" : "s"}`;
  return {
    head: `${s.unproven} of ${lines(changed)}`,
    files: files.map((f) => ({
      key: f.key,
      file: f.file,
      canOpen: view.canOpen(f.key),
      ranges: f.ranges,
    })),
    noCoverage:
      without > 0
        ? without === 1
          ? "1 changed file has no coverage data: its package did not run in this run."
          : `${without} changed files have no coverage data: their packages did not run in this run.`
        : undefined,
    note:
      files.length === 0 && without === 0
        ? changed === 0
          ? "No changed production line to check."
          : `Every one of the ${lines(changed)} ran.`
        : undefined,
  };
}

function judgedText(doc: AuditDocument): string {
  const s = doc.summary;
  if (doc.streams.length === 0) return "";
  const bad = [
    (s.failing ?? 0) > 0 ? `${s.failing} failing` : "",
    (s.notRun ?? 0) > 0 ? `${s.notRun} not run` : "",
    (s.vacuous ?? 0) > 0 ? `${s.vacuous} vacuous` : "",
    (s.stale ?? 0) > 0 ? `${s.stale} stale` : "",
  ].filter(Boolean);
  if (bad.length === 0) return ` All ${s.evidence} judged behaviors passed.`;
  return ` Of ${s.evidence} judged behaviors, ${bad.join(", ")}.`;
}

function behaviorRows(
  view: ChangeView,
  rows: BehaviorRow[],
  suite: string,
): ChangeBehaviorView[] {
  return rows.map((r) => ({
    key: r.key,
    suite,
    sign: r.sign,
    change: r.change,
    text: r.text,
    where: r.where,
    mark: r.mark,
    canOpen: view.canOpen(r.key),
    canRun: view.canRun(r.key),
  }));
}

// changePackageRows are one package's changed behaviors, for its opened row.
export function changePackageRows(
  view: ChangeView,
  pkg: string,
): ChangeBehaviorView[] {
  if (view.state.kind !== "doc") return [];
  const g = view.state.model.behaviors.find((p) => p.pkg === pkg);
  return g ? g.suites.flatMap((s) => behaviorRows(view, s.rows, s.suite)) : [];
}

function deltaText(rows: BehaviorRow[]): string {
  const by = new Map<string, number>();
  for (const r of rows) by.set(r.change, (by.get(r.change) ?? 0) + 1);
  const order = ["added", "removed", "reworded", "moved", "changed"];
  return Array.from(by)
    .sort((a, b) => order.indexOf(a[0]) - order.indexOf(b[0]))
    .map(([k, n]) => (k === "added" ? `+${n} added` : `${n} ${k}`))
    .join(" · ");
}

// buildChangeTab lays the tab out from what the scope holds.
export function buildChangeTab(view: ChangeView): ChangeTab {
  const s = view.state;
  switch (s.kind) {
    case "idle":
      return emptyTab(view, {
        empty: {
          title: "Change",
          text: view.busy
            ? "Auditing the working tree against its base…"
            : "No gotest folder to audit.",
        },
      });
    case "notGit":
      return emptyTab(view, {
        empty: {
          title: "Not a git checkout",
          text: "The Change tab compares the working tree with a commit, so it needs the folder to be in a git repository.",
        },
      });
    case "noBase":
      return emptyTab(view, {
        empty: {
          title: "Choose a base",
          text: "No remote default branch was found to compare with.",
          action: { act: "chooseBase", label: "Choose a base…" },
        },
      });
    case "oldCli":
      return emptyTab(view, {
        empty: {
          title: `The Change tab needs gotest ${s.floor}`,
          text: `${s.version ? `This folder's CLI is ${s.version}. ` : ""}It reads gotest audit, which ${s.floor} brings; the rest of the extension works as before. ${s.advice}`,
          action: s.canUpgrade
            ? { act: "upgrade", label: "Upgrade…" }
            : undefined,
        },
      });
    case "newSchema":
      return emptyTab(view, {
        empty: {
          title: "Update the extension",
          text: `This folder's CLI${s.version ? " " + s.version : ""} writes the audit document in schema ${s.schema}; this extension reads schema ${s.supported}.`,
        },
      });
    case "error":
      return emptyTab(view, { error: s.message, canAudit: !view.busy });
  }

  const m = s.model;
  const doc = s.doc;
  const byId = new Map(doc.findings.map((f) => [f.id, f]));
  const rows = [...m.open.flatMap((g) => g.rows), ...m.accepted];
  const routineRows = new Map<string, DecisionView[]>();
  const decisions: DecisionView[] = [];
  const ranks = new Map<string, number>();
  const accepted: DecisionView[] = [];
  for (const r of rows) {
    const f = r.evidence ? undefined : byId.get(r.key);
    const cls = f ? routineOf(f) : undefined;
    const v = decisionOf(view, r);
    if (cls) {
      const list = routineRows.get(cls.id) ?? [];
      list.push(v);
      routineRows.set(cls.id, list);
    } else if (r.accepted) accepted.push(v);
    else {
      decisions.push(v);
      ranks.set(v.key, rank(v, r.evidence));
    }
  }
  decisions.sort((a, b) => ranks.get(a.key)! - ranks.get(b.key)!);
  // Informational findings a routine class takes join their group; they
  // were never decisions.
  const information: string[] = [];
  for (const f of doc.findings) {
    if (f.weakens || f.class === "changed") continue;
    const cls = routineOf(f);
    if (!cls) {
      information.push(`${f.class}  ${findingLine(doc, f)}`);
      continue;
    }
    const list = routineRows.get(cls.id) ?? [];
    list.push({
      key: f.id,
      kind: kindLabel(f.class),
      tone: "info",
      what: f.subject,
      fact: f.base && f.head ? `${f.base} → ${f.head}` : f.head || f.base,
      where:
        f.location && f.location.line > 0
          ? `${f.location.file}:${f.location.line}`
          : "",
      open: view.canOpen(f.id) ? "diff" : undefined,
      accepted: view.accepted?.(f.id) ?? false,
    });
    routineRows.set(cls.id, list);
  }
  const routine: RoutineGroupView[] = ROUTINE.filter((c) =>
    routineRows.has(c.id),
  ).map((c) => {
    const list = routineRows.get(c.id)!;
    return {
      id: c.id,
      count: list.length,
      what: c.what,
      keys: list.map((r) => r.key),
      accepted: list.filter((r) => r.accepted).length,
      rows: list,
    };
  });
  const routineWeak = routine
    .flatMap((g) => g.rows)
    .filter((r) => byId.get(r.key)?.weakens).length;

  const open = decisions.length;
  const headline =
    open === 0
      ? "No change needs your decision"
      : `${open} ${open === 1 ? "change needs" : "changes need"} your decision`;
  const sm = doc.summary;
  const sub =
    (routineWeak > 0
      ? `${routineWeak} more ${routineWeak === 1 ? "is" : "are"} routine and grouped below. `
      : "") +
    `${sm.added} behaviors added, ${sm.removed} removed, ${sm.changed} changed.` +
    judgedText(doc);
  const hidden = decisions.slice(FIRST_DECISIONS);
  let exit: string | undefined;
  if (s.stopped) exit = "the run was stopped";
  else if (s.exitCode !== 0 && s.exitCode !== null) exit = `exit ${s.exitCode}`;

  const lists = [
    { title: "Information", items: information },
    { title: "Not compared", items: m.incomplete },
    { title: "Notes", items: m.notes },
  ].filter((l) => l.items.length > 0);

  return {
    folders: view.folders.length > 1 ? view.folders : undefined,
    busy: view.busy,
    canAudit: !view.busy,
    canChooseBase: true,
    notice: view.notice,
    head: m.headBranch ?? m.head,
    base: view.baseLabel ?? m.base,
    evidence: evidenceLine(view, doc),
    headline,
    sub,
    decisions,
    shown: Math.min(FIRST_DECISIONS, decisions.length),
    moreText:
      hidden.length > 0
        ? `${hidden.length} more: ${countWords(hidden)}`
        : undefined,
    accepted,
    routine,
    packages: m.behaviors.map((p) => ({
      pkg: p.pkg,
      name: p.pkg.replace(/^\.\//, ""),
      delta: deltaText(p.suites.flatMap((x) => x.rows)),
    })),
    lines: linesOf(view, doc, m),
    lists,
    exit,
    openCount: open,
  };
}
