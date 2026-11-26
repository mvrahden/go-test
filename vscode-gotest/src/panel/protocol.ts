// The messages between the panel's host and its webview. The webview's
// document is set once; after that every change travels as one of these,
// scoped to a tab and kept small: a package's rows and a test's output
// only on request.

export type Tab = "now" | "change" | "runs";
export const TABS: readonly Tab[] = ["now", "change", "runs"];

export const isTab = (v: unknown): v is Tab => TABS.includes(v as Tab);

// The most one message may carry, but for a single opened package.
export const MESSAGE_BUDGET = 200 * 1024;

export type Tone = "fail" | "warn" | "pass" | "info" | "none";

// A piece of an output line: text, or a file:line token with where it opens.
export type LineSegment = string | [text: string, file: string, line: number];
export type OutputLine = LineSegment[];

// The statuses the Now tab's chips narrow the rows to.
export type FilterStatus =
  "fail" | "stale" | "vacuous" | "none" | "skip" | "pass";
export const FILTER_STATUSES: readonly FilterStatus[] = [
  "fail",
  "stale",
  "vacuous",
  "none",
  "skip",
  "pass",
];

export interface Chip {
  status: FilterStatus;
  label: string;
  count: number;
}

export interface NowContext {
  branch?: string;
  commit?: string;
  // Whether the verdicts on show belong to the working tree.
  fresh?: { text: string; tone: Tone };
  // One sentence on the run the verdicts come from.
  lastRun?: string;
  // The run's whole command line, for its tooltip.
  runTitle?: string;
  // A line per run in progress.
  live?: string[];
  // What the tab can show: the workspace's runs and each imported stream.
  sources?: { id: string; label: string; selected: boolean }[];
}

export interface Card {
  key: string;
  kind: "failed" | "stale" | "vacuous";
  pkg: string;
  // The package and the rows above the behavior.
  where: string;
  name: string;
  message?: OutputLine[];
  // The first file:line its output names.
  at?: OutputLine;
  // Where the source declares it.
  loc?: { file: string; line: number };
  flaky?: string;
  // The Test Explorer item and the row's place in its package.
  id?: string;
  pos?: number[];
  // From a run still in progress: the run's end replaces it.
  live?: boolean;
  // Output lines the card leaves out; Full output shows them.
  more?: boolean;
}

export interface PackageRow {
  pkg: string;
  name: string;
  counts: string;
  tone: Tone;
  time: string;
  title?: string;
  stale?: boolean;
  // Said after the counts: never ran, freshness unknown.
  note?: string;
  // Rows a search or a chip leaves.
  matches?: number;
  live?: boolean;
  // Nothing to open: a package without suites.
  empty?: boolean;
}

export interface NowModel {
  rev: number;
  empty?: { title: string; text: string };
  error?: string;
  context: NowContext;
  chips: Chip[];
  cards: Card[];
  moreCards?: string;
  noSuites?: { text: string; names: string[] };
  total: string;
  groups: string[];
  packages: PackageRow[];
  filtering: boolean;
  failed: number;
}

// One row of a package's tree, short keys: it is the largest message.
export interface TreeNode {
  d: string;
  k: string;
  s: string;
  // The row's place among its package's rows, dot-joined.
  pos: string;
  t?: string;
  id?: string;
  loc?: [file: string, line: number];
  tags?: { text: string; tone?: Tone }[];
  out?: OutputLine[];
  c?: TreeNode[];
}

export interface Mark {
  text: string;
  tone: Tone;
  detail: string;
}

export interface DecisionView {
  key: string;
  kind: string;
  tone: Tone;
  what: string;
  fact: string;
  where: string;
  // Diff opens the change, Open where the head declares an evidence row.
  open?: "diff" | "source";
  mark?: Mark;
  accepted: boolean;
}

export interface RoutineGroupView {
  id: string;
  count: number;
  what: string;
  keys: string[];
  accepted: number;
  rows: DecisionView[];
}

export interface ChangeBehaviorView {
  key: string;
  suite: string;
  sign: string;
  change: string;
  text: string;
  where: string;
  mark?: Mark;
  canOpen: boolean;
  canRun: boolean;
}

export interface UnprovenView {
  head: string;
  source: string;
  files: {
    key: string;
    file: string;
    pkg: string;
    counts: string;
    coverage: boolean;
    canOpen: boolean;
    ranges: { key: string; text: string }[];
  }[];
}

export interface Picker {
  path: string;
  label: string;
  selected: boolean;
}

export interface EmptyView {
  title: string;
  text: string;
  action?: { act: string; label: string };
}

export interface ChangeTab {
  folders?: Picker[];
  busy: false | "static" | "run";
  canAudit: boolean;
  canChooseBase: boolean;
  empty?: EmptyView;
  error?: string;
  notice?: string;
  head?: string;
  base?: string;
  evidence?: {
    text: string;
    picks: { id: string; label: string; missing: boolean }[];
    action?: { act: string; label: string };
  };
  headline?: string;
  sub?: string;
  decisions: DecisionView[];
  // How many decisions show before Show all.
  shown: number;
  moreText?: string;
  accepted: DecisionView[];
  routine: RoutineGroupView[];
  packages: { pkg: string; name: string; delta: string }[];
  unproven?: UnprovenView;
  lists: { title: string; items: string[] }[];
  exit?: string;
  openCount: number;
}

export interface RunCard {
  key: string;
  time: string;
  day: string;
  title: string;
  who: string;
  counts: string;
  verdict: string;
  tone: Tone;
  fresh: string;
  freshTone: Tone;
  canOpen: boolean;
  evidence: "none" | "chosen" | "auto";
  command: string;
  // The commit the run ran on, and whether its tree was dirty.
  commit: string;
  // A card standing for consecutive runs of one command.
  runs?: RunCard[];
}

export interface RunsTab {
  folders?: Picker[];
  busy: boolean;
  empty?: EmptyView;
  error?: string;
  notice?: string;
  headline?: string;
  sub?: string;
  cards: RunCard[];
  earlier: RunCard[];
  footer?: string;
}

export interface RunningItem {
  id: number;
  label: string;
  elapsed: string;
  counts?: string;
  stopping: boolean;
}

export type HostMessage =
  | { type: "tab"; tab: Tab }
  | { type: "busy"; tab: Tab; busy: boolean }
  | { type: "badges"; now: number; change: number }
  | { type: "now"; model: NowModel }
  | {
      type: "package";
      pkg: string;
      rev: number;
      nodes: TreeNode[];
      failure?: OutputLine[];
    }
  | { type: "output"; key: string; lines: OutputLine[]; more?: number }
  | { type: "change"; model: ChangeTab }
  | { type: "changePackage"; pkg: string; rows: ChangeBehaviorView[] }
  | { type: "runs"; model: RunsTab }
  | { type: "running"; items: RunningItem[] };

// What the webview keeps across a reload of its document.
export interface ClientState {
  tab: Tab;
  open: string[];
  changeOpen: string[];
  search: string;
  filters: FilterStatus[];
  scroll: Partial<Record<Tab, number>>;
}
