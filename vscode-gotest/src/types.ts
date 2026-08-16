export interface DiscoverWarning {
  importPath: string;
  file?: string;
  line?: number;
  col?: number;
  message: string;
}

export interface DiscoverOutput {
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

export interface DiscoverSuite {
  name: string;
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
}

export interface DiscoverMethod {
  name: string;
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
  overlayFile: string;
  dir: string;
  stateFile?: string;
}
