export interface DiscoverWarning {
  importPath: string;
  file?: string;
  line?: number;
  col?: number;
  message: string;
}

export interface DiscoverOutput {
  /** The CLI that produced the document; absent before v1.30.2. */
  version?: string;
  packages: DiscoverPackage[];
  warnings?: DiscoverWarning[];
}

export interface DiscoverPackage {
  importPath: string;
  dir: string;
  modulePath?: string; // NEW: Go module path from go.mod
  testOnly?: boolean;
  // A broken package failed to load; its diagnostics arrive as top-level
  // warnings. Its suite list is unknowable — discovery needs a successful
  // parse — so `suites` being empty means "unknown", not "none".
  broken?: boolean;
  suites: DiscoverSuite[];
}

// EXTERNAL_ID marks an external twin's id: a suite declared in both the
// internal and the external test package shares one name, so one id would
// lose the external copy's results.
export const EXTERNAL_ID = "~external";

// suiteItemId is a suite's Test Explorer id. Only an external twin — one with
// a same-named internal suite beside it — carries the marker; a lone external
// suite has nothing to collide with.
export function suiteItemId(
  importPath: string,
  suite: { name: string; external?: boolean },
  suites: readonly { name: string; external?: boolean }[],
): string {
  const twin =
    !!suite.external &&
    suites.some((s) => !s.external && s.name === suite.name);
  return `${importPath}/${suite.name}${twin ? EXTERNAL_ID : ""}`;
}

// methodItemId is a method's id under its suite's: the name go test gives it.
export function methodItemId(
  suiteId: string,
  method: { name: string; testName?: string },
): string {
  return `${suiteId}/${method.testName ?? method.name}`;
}

export interface DiscoverSuite {
  name: string;
  // Declared in the package's external (_test) test files.
  external?: boolean;
  parallel: boolean;
  focused: boolean;
  excluded: boolean;
  guarded: boolean;
  file: string;
  line: number;
  col: number;
  lifecycle: string[];
  fixtures: string[];
  methods: DiscoverMethod[];
  benchmarks: DiscoverMethod[];
  fuzzers: DiscoverMethod[];
}

export interface DiscoverMethod {
  name: string;
  // The name go test gives this method when it differs from `name`: an
  // external twin's copy of a method the internal twin declares ("TestX#01").
  testName?: string;
  parallel: boolean;
  focused: boolean;
  excluded: boolean;
  file: string;
  line: number;
  col: number;
  // The When/It blocks the method declares, read from source. Absent on a CLI
  // that predates static behavior discovery.
  behaviors?: DiscoverBehavior[];
  // Whether `behaviors` is exhaustive. False means the method declares
  // behaviors whose names or existence depend on runtime values, so the list
  // is a floor and the rest appear only once the method has run.
  behaviorsComplete?: boolean;
}

export interface DiscoverBehavior {
  // The subtest segment go test will produce — identical to the runtime one,
  // which is what lets a declared behavior and an observed one be the same
  // tree node rather than two.
  name: string;
  // The label to show: the description as the developer wrote it, spoken in its
  // vocabulary, and the same string `gotest spec` renders for this node. Do not re-derive it from the
  // name here, or the test tree and the spec view will drift apart.
  display: string;
  // Which call declared it: "when" | "it" | "each".
  kind?: string;
  line: number;
  children?: DiscoverBehavior[];
}

export interface PrepareOutput {
  /** The CLI that produced the document; absent before v1.30.2. */
  version?: string;
  overlayFile: string;
  dir: string;
  stateFile?: string;
}
