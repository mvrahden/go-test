// The Runs tab's data: the folder's run records as the CLI lists them, as
// cards, or what stands in for them. Consecutive runs of one command by one
// hand fold into one card.
import type { Picker, RunCard, RunsTab } from "./panel/protocol.js";
import {
  commandParts,
  countsText,
  dayText,
  freshText,
  isCurrent,
  runTitle,
  verdictText,
  whenText,
  whoText,
  type RunsDocument,
  type RunsEntry,
} from "./runsModel.js";

export type RunsState =
  | { kind: "idle" }
  | {
      kind: "oldCli";
      version?: string;
      floor: string;
      advice: string;
      canUpgrade: boolean;
    }
  | { kind: "error"; message: string }
  | { kind: "doc"; doc: RunsDocument };

export interface RunsView {
  state: RunsState;
  busy: boolean;
  folders: Picker[];
  notice?: string;
  // The checkout's head commit, as records name it.
  head?: string;
  now: Date;
  exists: (file: string) => boolean;
  // The runs chosen as evidence, and the one taken when none is.
  chosen: (id: string) => boolean;
  auto?: string;
}

export const RUNS_EMPTY_TEXT =
  "Every gotest run of this folder leaves a record, whether it was started from a terminal, by an agent or from this editor; none is kept yet. Run the tests, or import the events file of a run made elsewhere, such as a CI run you downloaded.";

export const RUNS_KEPT =
  "The newest 50 records are kept, none longer than 30 days.";

function cardOf(view: RunsView, r: RunsEntry): RunCard {
  const at = new Date(r.recorded);
  const ok = !isNaN(at.getTime());
  const fresh = freshText(r.packages);
  return {
    key: r.id,
    time: ok ? whenText(at, view.now).slice(-5) : r.recorded,
    day: ok ? dayText(at, view.now) : "",
    title: runTitle(r.manifest),
    who: whoText(r),
    counts: countsText(r.result),
    verdict: verdictText(r.result.exitCode),
    tone: r.result.exitCode === 0 ? "pass" : "fail",
    fresh: fresh.text,
    freshTone: fresh.tone,
    canOpen: !!r.eventsFile && view.exists(r.eventsFile),
    evidence: view.chosen(r.id)
      ? "chosen"
      : view.auto === r.id
        ? "auto"
        : "none",
    command: ["gotest", ...r.manifest.args].join(" "),
    commit: r.manifest.commit
      ? `${r.manifest.commit}${r.manifest.dirty ? " dirty" : ""}`
      : "",
  };
}

const sameRun = (a: RunsEntry, b: RunsEntry) =>
  a.manifest.args.join("\u0000") === b.manifest.args.join("\u0000") &&
  (a.manifest.commit ?? "") === (b.manifest.commit ?? "") &&
  whoText(a) === whoText(b);

// cardsOf lays records out newest first, folding consecutive runs of one
// command, commit and source into one card.
export function cardsOf(view: RunsView, runs: RunsEntry[]): RunCard[] {
  const groups: RunsEntry[][] = [];
  for (const r of runs) {
    const last = groups[groups.length - 1];
    if (last && sameRun(last[0], r)) last.push(r);
    else groups.push([r]);
  }
  return groups.map((g) => {
    const card = cardOf(view, g[0]);
    if (g.length === 1) return card;
    const members = g.map((r) => cardOf(view, r));
    const failed = g.filter((r) => r.result.exitCode !== 0).length;
    const passed = g.length - failed;
    return {
      ...card,
      title: `${card.title}, ${g.length} runs in a row`,
      verdict:
        failed === 0
          ? `${g.length} passed`
          : passed === 0
            ? `${g.length} failed`
            : `${passed} passed · ${failed} failed`,
      tone: failed > 0 ? "fail" : "pass",
      runs: members,
    };
  });
}

function headline(view: RunsView, head: RunsEntry[]): string {
  if (head.length === 0) return "No run of this commit is recorded yet";
  const whole = head.find((r) => {
    const p = commandParts(r.manifest.args);
    return (
      isCurrent(r) &&
      !p.run &&
      (p.patterns.length === 0 || p.patterns.some((x) => x === "./..."))
    );
  });
  const verdict = (r: RunsEntry) =>
    r.result.exitCode === 0 ? "passed" : "failed";
  if (whole) return `One run covers this whole tree, and it ${verdict(whole)}`;
  const current = head.filter(isCurrent);
  if (current.length === 0) return "No run matches this tree any more";
  return `${current.length} ${current.length === 1 ? "run matches" : "runs match"} this tree; the newest ${verdict(current[0])}`;
}

// buildRunsTab lays the tab out from what the scope holds.
export function buildRunsTab(view: RunsView): RunsTab {
  const base: RunsTab = {
    folders: view.folders.length > 1 ? view.folders : undefined,
    busy: view.busy,
    notice: view.notice,
    cards: [],
    earlier: [],
  };
  const s = view.state;
  switch (s.kind) {
    case "idle":
      return {
        ...base,
        empty: {
          title: "Runs",
          text: view.busy ? "Reading the run records…" : "No gotest folder.",
        },
      };
    case "oldCli":
      return {
        ...base,
        empty: {
          title: `The Runs tab needs gotest ${s.floor}`,
          text: `${s.version ? `This folder's CLI is ${s.version}. ` : ""}It reads gotest runs, which ${s.floor} brings, as do Run changed and the stale marks of the Now tab; the rest of the extension works as before. ${s.advice}`,
          action: s.canUpgrade
            ? { act: "upgrade", label: "Upgrade…" }
            : undefined,
        },
      };
    case "error":
      return { ...base, error: s.message };
  }
  const runs = s.doc.runs;
  if (runs.length === 0)
    return {
      ...base,
      empty: {
        title: "No run recorded yet",
        text: RUNS_EMPTY_TEXT,
        action: { act: "import", label: "Import a CI run" },
      },
    };
  const head = view.head ?? runs[0].manifest.commit;
  const ofHead = runs.filter((r) => head && r.manifest.commit === head);
  const earlier = runs.filter((r) => !head || r.manifest.commit !== head);
  return {
    ...base,
    headline: headline(view, ofHead),
    sub: head
      ? `Records for ${head}. A run counts for this tree while the files it read are unchanged.`
      : undefined,
    cards: cardsOf(view, ofHead),
    earlier: cardsOf(view, earlier),
    footer: RUNS_KEPT,
  };
}
