// Tier 1 + Tier 2 — the real seam.
//
// Everything here spawns the actual `gotest` CLI. `node:child_process` is NOT
// mocked; only the editor is stubbed. The regression this suite exists for was
// invisible to every mocked test in the repo, and invisible again to any
// integration test that used a prebuilt binary or merged stdout with stderr —
// so resolution mode is an explicit axis and the streams are kept apart.

import {
  describe,
  it,
  expect,
  beforeAll,
  afterAll,
  beforeEach,
  vi,
} from "vitest";
import { spawnSync } from "node:child_process";
import {
  readFileSync,
  writeFileSync,
  mkdtempSync,
  chmodSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

const state = vi.hoisted(() => ({
  workspaceDir: "",
  cliPath: "",
  autoRefresh: true,
  panels: [] as import("./vscodeStub.js").StubPanel[],
}));

vi.mock("vscode", async () => {
  const { buildVscodeStub } = await import("./vscodeStub.js");
  return buildVscodeStub(state as never);
});

import { PanelHost } from "../../src/panel/host.js";
import { shownHtml } from "./shownPanel.js";
import { DiscoveryCache } from "../../src/discovery.js";
import { createRecordingChannel } from "./vscodeStub.js";
import { clearBinaryCache } from "../../src/cli.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const streamDir = path.join(extensionDir, "testdata", "streams");

const stream = (name: string) =>
  readFileSync(path.join(streamDir, `${name}.jsonl`), "utf-8");

// A recorded stream as a run of the workspace folder hands it to the panel.
const run = (name: string) => ({
  stream: stream(name),
  folder: state.workspaceDir,
});

let realBinary = "";
let prebuiltBinary = "";
let brokenBinary = "";
let binDir = "";

beforeAll(() => {
  const dir = mkdtempSync(path.join(tmpdir(), "gotest-integration-"));
  binDir = dir;

  // Named as the platform expects an executable to be named: Windows will not
  // spawn a file without the suffix, however valid its contents.
  realBinary = path.join(
    dir,
    process.platform === "win32" ? "gotest-real.exe" : "gotest-real",
  );
  // Stamped, not left to the build's own pseudo-version: that is derived from
  // the newest reachable tag, so once a release at or above MIN_CLI_VERSION
  // exists the working-tree build outranks the floor and this fixture silently
  // becomes a binary the extension accepts. A shallow CI checkout hides it and
  // a local clone with tags does not.
  const built = spawnSync(
    "go",
    [
      "build",
      "-ldflags",
      "-X github.com/mvrahden/go-test/internal/about.Version=v1.0.0",
      "-o",
      realBinary,
      "./cmd/gotest",
    ],
    { cwd: repoRoot, encoding: "utf-8" },
  );
  expect(built.status, `go build failed: ${built.stderr}`).toBe(0);

  // realBinary reports a version below MIN_CLI_VERSION, which the extension
  // rejects — correctly, but that would silently turn the
  // cliPath axis into a second `go run` axis. The wrapper reports a releasable
  // version and delegates everything else to the real binary, so the axis tests
  // cliPath resolution rather than this tree's version string.
  prebuiltBinary = path.join(dir, "gotest");
  writeFileSync(
    prebuiltBinary,
    [
      "#!/bin/sh",
      'if [ "$1" = "version" ]; then echo "gotest v99.0.0 integration"; exit 0; fi',
      `exec ${realBinary} "$@"`,
      "",
    ].join("\n"),
    "utf-8",
  );
  chmodSync(prebuiltBinary, 0o755);

  // A binary that passes the extension's version probe but fails to produce a
  // spec. This is the shape of a toolchain or build failure, and it must stay
  // distinguishable from "the tests failed".
  brokenBinary = path.join(dir, "broken-gotest");
  writeFileSync(
    brokenBinary,
    [
      "#!/bin/sh",
      'if [ "$1" = "version" ]; then echo "gotest v1.99.0 stub"; exit 0; fi',
      'echo "simulated toolchain failure" >&2',
      "exit 1",
      "",
    ].join("\n"),
    "utf-8",
  );
  chmodSync(brokenBinary, 0o755);
});

afterAll(() => {
  if (binDir) rmSync(binDir, { recursive: true, force: true });
});

beforeEach(() => {
  state.workspaceDir = repoRoot;
  state.cliPath = "";
  state.autoRefresh = true;
  state.panels.length = 0;
  clearBinaryCache();
});

function newPanel() {
  const recorder = createRecordingChannel();
  const panel = new PanelHost(recorder.channel as never);
  return { panel, recorder };
}

function renderedHtml(): Promise<string> {
  expect(state.panels.length).toBeGreaterThan(0);
  return shownHtml(state.panels[state.panels.length - 1].webview);
}

// Both resolution modes the extension actually uses. `go run` is the default
// and the only one that exposes the exit-status epilogue; a suite that tested
// solely the prebuilt path would have shipped the regression untouched.
const POSIX = process.platform !== "win32";

const MODES = [
  { name: "go run (extension default)", cliPath: () => "" },
  // The prebuilt axis routes through a /bin/sh wrapper, so it is POSIX-only.
  ...(POSIX
    ? [{ name: "prebuilt cliPath", cliPath: () => prebuiltBinary }]
    : []),
];

describe.each(MODES)("spec sync via $name", (mode) => {
  beforeEach(() => {
    state.cliPath = mode.cliPath();
  });

  it("renders a run whose tests all passed", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(run("all-pass"));

    const html = await renderedHtml();
    expect(html).toContain("adds an item");
    expect(html).toContain("totals the basket");
    expect(html).toContain("2 passed");
    expect(recorder.errors).toEqual([]);
  });

  // The regression, stated as user-visible behaviour: a run containing a
  // failure must still reach the panel.
  it("renders a run that contained a failure", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(run("mixed"));

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("totals the basket");
    expect(html).toContain("expected 300, got 250");
    expect(html).toContain("1 passed");
    expect(html).toContain("1 failed");
  });

  it("renders a fuzz target as a property row under its suite", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(run("fuzz"));

    const html = await renderedHtml();
    expect(html).toContain("TrimIdempotent");
    expect(html).toContain("drops surrounding spaces");
    // The row is tagged as a property backed by seeds, and the trailer counts
    // the target the way the CLI does.
    expect(html).toContain('<span class="tag t-info">FUZZ · 3 seeds</span>');
    expect(html).toContain("1 fuzz target");
    expect(recorder.errors).toEqual([]);
  });

  it("renders a package that failed to build, which has no failing behaviour to speak for it", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(run("package-failure"));

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("FAIL");
    expect(html).toContain("undefined: helper");
    expect(html).toContain("package failed");
  });

  // The tree is the CLI's: a slash in an It is one subtest, so one row; a
  // label reaches the webview raw and is escaped once, here.
  it("renders labels as the CLI's spec does: one row per slash, escaped once", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(run("labels"));

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    // A slash is one row's label, not two rows.
    expect(html).toContain('<span class="lbl">reads a/b paths</span>');
    expect(html).not.toContain('<span class="lbl">reads a</span>');
    // A fast behavior shows no time, so its actions follow its label.
    expect(html).toContain(
      '<span class="lbl">keeps &lt;b&gt;&amp;&quot; text</span><span class="grow"></span>',
    );
    expect(html).not.toContain("&amp;lt;");
    expect(html).not.toContain("&amp;amp;");
    // A run skips an excluded suite, and the CLI's run spec says so.
    expect(html).toMatch(/Parked<\/span> <span class="tag t-warn">SKIPPED/);
    expect(html).toContain("2 passed");
  });

  it("renders an empty stream as an empty spec, not as an error", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(run("empty"));

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    // No package and no rendered node — asserted on content rather than on the
    // static template, which carries status glyphs of its own.
    expect(html).not.toContain("example.com");
    expect(html).not.toContain('class="ti k-leaf');
  });
});

describe("CLI resolution is the mode the test intends", () => {
  // Guards the trap this suite was designed around: if the prebuilt path
  // silently fell back to `go run` (or vice versa), the matrix above would be
  // testing one mode twice and quietly lose its only failing-path coverage.
  it("uses go run when no cliPath is configured", async () => {
    const debug: string[] = [];
    const recorder = createRecordingChannel();
    const channel = {
      ...recorder.channel,
      debug: (m: string) => debug.push(m),
    };
    const panel = new PanelHost(channel as never);
    state.cliPath = "";
    await panel.show();
    await panel.refresh(run("mixed"));

    // Every go-run branch logs "<goBin> run <target>", and goBin carries the
    // executable's own name — `go` on POSIX, `go.exe` on Windows. Match the
    // invocation, not the filename.
    expect(debug.some((m) => / run /.test(m))).toBe(true);
    expect(debug.some((m) => m.includes("cliPath override"))).toBe(false);
  });

  it.skipIf(!POSIX)(
    "uses the configured binary when cliPath is set",
    async () => {
      const debug: string[] = [];
      const recorder = createRecordingChannel();
      const channel = {
        ...recorder.channel,
        debug: (m: string) => debug.push(m),
      };
      const panel = new PanelHost(channel as never);
      state.cliPath = prebuiltBinary;
      await panel.show();
      await panel.refresh(run("mixed"));

      expect(debug.some((m) => m.includes("cliPath override"))).toBe(true);
    },
  );

  // The extension now sends --render-only, which older CLIs would reject. The
  // version floor is what keeps that from reaching them, so its enforcement is
  // asserted against a real binary rather than only against mocked probes.
  it.skipIf(!POSIX)(
    "refuses a cliPath binary below the version floor and falls back to go run",
    async () => {
      const debug: string[] = [];
      const warnings: string[] = [];
      const channel = {
        info: () => {},
        error: () => {},
        warn: (m: string) => warnings.push(m),
        debug: (m: string) => debug.push(m),
      };
      const panel = new PanelHost(channel as never);
      state.cliPath = realBinary; // reports a working-tree pseudo-version
      await panel.show();
      await panel.refresh(run("mixed"));

      expect(warnings.some((m) => m.includes("requires >="))).toBe(true);
      expect(debug.some((m) => m.includes("cliPath override"))).toBe(false);
      expect(debug.some((m) => m.includes("go run"))).toBe(true);
    },
  );
});

describe("a CLI that produced no spec is reported, not swallowed", () => {
  // The stand-in binaries are /bin/sh scripts, so this case is POSIX-only.
  // Everything above runs on every platform; only the fabricated binaries here
  // and the cliPath wrapper depend on a shell.
  it.skipIf(process.platform === "win32")(
    "surfaces the real diagnostic when the binary fails to render",
    async () => {
      const { panel, recorder } = newPanel();
      state.cliPath = brokenBinary;
      await panel.show();
      await panel.refresh(run("mixed"));

      expect(recorder.errors.length).toBe(1);
      expect(recorder.errors[0]).toContain("simulated toolchain failure");
      // The failure must read as a failure, not as an unhelpful parse error
      // downstream of a silently discarded exit code.
      expect(recorder.errors[0]).not.toContain("JSON");
      expect(await renderedHtml()).not.toContain("totals the basket");
    },
  );

  it.skipIf(process.platform === "win32")(
    "keeps the last picture under a notice, and drops the notice when a render works again",
    async () => {
      const { panel } = newPanel();
      await panel.show();
      await panel.refresh(run("mixed"));
      expect(await renderedHtml()).toContain("totals the basket");
      expect(await renderedHtml()).not.toContain("This view is out of date");

      state.cliPath = brokenBinary;
      clearBinaryCache();
      await panel.refresh(run("all-fail"));
      expect(await renderedHtml()).toContain(
        "This view is out of date: the last render failed.",
      );
      expect(await renderedHtml()).toContain("simulated toolchain failure");
      expect(await renderedHtml()).toContain("totals the basket");

      state.cliPath = "";
      clearBinaryCache();
      await panel.refresh(run("mixed"));
      expect(await renderedHtml()).not.toContain("This view is out of date");
    },
  );
});

// Tier 2 — refresh() is stateful. The panel keeps the newest lines of every
// test of every package it has seen and renders them as one stream, so what
// it shows depends on every run before the last. No exit-code assertion
// reaches this.

// How many times a rendered row carries this display text.
function rows(html: string, display: string): number {
  return html.split(`<span class="lbl">${display}</span>`).length - 1;
}

// A rerun of one cart behavior that now passes, as a -run filter on it
// produces: its parent suite and the package report again, nothing else does.
const rerunTotals = [
  { Action: "run", Package: "example.com/cart", Test: "TestCartTestSuite" },
  {
    Action: "run",
    Package: "example.com/cart",
    Test: "TestCartTestSuite/totals_the_basket",
  },
  {
    Action: "pass",
    Package: "example.com/cart",
    Test: "TestCartTestSuite/totals_the_basket",
    Elapsed: 0.02,
  },
  {
    Action: "pass",
    Package: "example.com/cart",
    Test: "TestCartTestSuite",
    Elapsed: 0.02,
  },
  { Action: "pass", Package: "example.com/cart", Elapsed: 0.03 },
]
  .map((e) => JSON.stringify(e))
  .join("\n");

describe("runs kept across refreshes", () => {
  it("replaces a package's failing run with its later passing run", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    await panel.refresh(run("mixed"));
    expect(await renderedHtml()).toContain("1 failed");

    await panel.refresh(run("all-pass"));
    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("2 passed");
    expect(html).not.toContain("1 failed");
  });

  // This used to keep one layer per producer and render them all, so a
  // coverage run's failure stayed beside a later passing run of the same
  // package. One package is one set of verdicts, whoever reported them.
  it("shows a package once when two producers reported it, with the newer verdict", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    await panel.refresh(run("mixed")); // a coverage run
    await panel.refresh(run("all-pass")); // a test run of the same package

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(rows(html, "adds an item")).toBe(1);
    expect(rows(html, "totals the basket")).toBe(1);
    expect(html).toContain("2 passed");
    expect(html).not.toContain("1 failed");
  });

  it("counts a stream fed twice once", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    await panel.refresh(run("all-pass"));
    await panel.refresh(run("all-pass"));

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("2 in 1 suite, 1 package");
    expect(html).toContain("2 passed");
    expect(html).not.toContain("4 passed");
  });

  it("keeps a package when a later run reports another one", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    await panel.refresh(run("all-pass"));
    await panel.refresh(run("other-package-fail"));

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("example.com/cart");
    expect(html).toContain("example.com/checkout");
    expect(html).toContain("adds an item");
    expect(html).toContain("rejects an expired card");
    expect(html).toContain("2 passed");
    expect(html).toContain("1 failed");
  });

  it("keeps the other behaviors' verdicts when one is rerun", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    await panel.refresh(run("mixed"));
    await panel.refresh({
      stream: rerunTotals,
      folder: state.workspaceDir,
      partial: new Map([
        [
          "example.com/cart",
          (p: string) =>
            p === "TestCartTestSuite/totals_the_basket" ||
            p.startsWith("TestCartTestSuite/totals_the_basket/"),
        ],
      ]),
    });

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(rows(html, "adds an item")).toBe(1);
    expect(rows(html, "totals the basket")).toBe(1);
    expect(html).toContain("2 passed");
    expect(html).not.toContain("1 failed");
  });
});

// A suite's own verdict: its hooks run outside any behavior, so a failure
// there — or a panic go test books on the suite — is on the suite node alone.
const suiteOwnFailure = [
  { Action: "run", Package: "example.com/store", Test: "TestStoreTestSuite" },
  {
    Action: "run",
    Package: "example.com/store",
    Test: "TestStoreTestSuite/saves",
  },
  {
    Action: "pass",
    Package: "example.com/store",
    Test: "TestStoreTestSuite/saves",
    Elapsed: 0.01,
  },
  {
    Action: "output",
    Package: "example.com/store",
    Test: "TestStoreTestSuite",
    Output: "    store_test.go:40: AfterAll: close db: connection reset\n",
  },
  {
    Action: "output",
    Package: "example.com/store",
    Test: "TestStoreTestSuite",
    Output: "--- FAIL: TestStoreTestSuite (0.02s)\n",
  },
  {
    Action: "fail",
    Package: "example.com/store",
    Test: "TestStoreTestSuite",
    Elapsed: 0.02,
  },
  { Action: "fail", Package: "example.com/store", Elapsed: 0.03 },
]
  .map((e) => JSON.stringify(e))
  .join("\n");

describe("a branch's own verdict", () => {
  it("shows a suite that failed with every behavior passing as failed, with what it said", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh({
      stream: suiteOwnFailure,
      folder: state.workspaceDir,
    });

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toMatch(/class="ti k-suite lv-\d+ s-fail"/);
    const said = /<pre class="row-out">([^<]*)<\/pre>/.exec(html)?.[1];
    expect(said).toBe("store_test.go:40: AfterAll: close db: connection reset");
  });
});

// discovered is what gotest discover says of packages: the oracle for where
// the CLI's spec places the nodes the source declares.
function discovered(patterns: string[]): Map<string, string> {
  const out = spawnSync(
    "go",
    ["run", "./cmd/gotest", "discover", ...patterns],
    { cwd: repoRoot, encoding: "utf-8" },
  );
  expect(out.status, out.stderr).toBe(0);
  type Behavior = { name: string; line: number; children?: Behavior[] };
  type Method = {
    name: string;
    file: string;
    line: number;
    behaviors?: Behavior[];
  };
  const doc = JSON.parse(out.stdout) as {
    packages: {
      importPath: string;
      dir: string;
      suites: {
        name: string;
        file: string;
        line: number;
        methods: Method[];
        fuzzers: Method[];
      }[];
    }[];
  };
  // Test Explorer id to "file:line".
  const at = new Map<string, string>();
  const walk = (id: string, file: string, bs: Behavior[]) => {
    for (const b of bs) {
      at.set(`${id}/${b.name}`, `${file}:${b.line}`);
      walk(`${id}/${b.name}`, file, b.children ?? []);
    }
  };
  for (const pkg of doc.packages) {
    for (const suite of pkg.suites) {
      const id = `${pkg.importPath}/${suite.name}`;
      at.set(id, `${path.join(pkg.dir, suite.file)}:${suite.line}`);
      for (const m of [...suite.methods, ...suite.fuzzers]) {
        const file = path.join(pkg.dir, m.file);
        at.set(`${id}/${m.name}`, `${file}:${m.line}`);
        walk(`${id}/${m.name}`, file, m.behaviors ?? []);
      }
    }
  }
  return at;
}

// linked reads each row's go-to-source target from the rendered panel.
function linked(html: string): Map<string, string> {
  const at = new Map<string, string>();
  // A row carries where its source declares it.
  const row = /data-id="([^"]+)" data-file="([^"]+)" data-line="(\d+)"/g;
  for (const m of html.matchAll(row)) {
    at.set(
      m[1].replace(/&quot;/g, '"').replace(/&#39;/g, "'"),
      `${m[2]}:${m[3]}`,
    );
  }
  return at;
}

describe("the source under the runs", () => {
  const patterns = ["./examples/search", "./examples/fuzzing"];
  const declaredPanel = () => {
    const recorder = createRecordingChannel();
    const panel = new PanelHost(
      recorder.channel as never,
      undefined,
      undefined,
      async () => patterns,
    );
    return { panel, recorder };
  };

  it("links every row the source declares to the line the CLI names, fuzz targets and When and It rows included", async () => {
    const { panel, recorder } = declaredPanel();
    await panel.show();
    await panel.refresh(run("declared-source"));

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    const want = discovered(patterns);
    const got = linked(html);
    expect(want.size).toBeGreaterThan(10);
    expect([...want.keys()].some((id) => /\/Fuzz[^/]*$/.test(id))).toBe(true);
    for (const [id, loc] of want) expect(got.get(id), id).toBe(loc);
  });

  it("shows what the run did not report as not run, in the CLI's words and counts", async () => {
    const { panel, recorder } = declaredPanel();
    await panel.show();
    await panel.refresh(run("declared-source"));

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    const spec = (
      panel as unknown as {
        lastSpecData: {
          stats: { notRun?: number };
          packages: { path: string }[];
        };
      }
    ).lastSpecData;
    expect(spec.stats.notRun ?? 0).toBeGreaterThan(0);
    expect(html).toContain(
      `Not run <strong>${spec.stats.notRun!.toLocaleString("en-US")}</strong>`,
    );
    expect(html).toContain('<span class="tag t-none">NOT RUN</span>');
    expect(spec.packages.map((p) => p.path)).toContain(
      "github.com/mvrahden/go-test/examples/fuzzing",
    );
  });

  it("narrows the rows to the chips pressed, and counts the rest still", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(run("mixed"));
    expect(await renderedHtml()).toMatch(/class="ti k-leaf lv-\d+ s-pass"/);

    (panel as unknown as { handleMessage(m: unknown): void }).handleMessage({
      type: "filters",
      filters: ["fail"],
    });

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).not.toMatch(/class="ti k-leaf lv-\d+ s-pass"/);
    expect(html).toMatch(/class="ti k-leaf lv-\d+ s-fail"/);
    expect(html).toContain("1 passed");
  });
});

describe("acting on a node", () => {
  it("names each row by its Test Explorer id and hands Run and Debug on", async () => {
    const { panel, recorder } = newPanel();
    const ran: string[][] = [];
    const debugged: string[] = [];
    panel.setActions({
      run: async (ids) => {
        ran.push(ids);
      },
      debug: async (id) => {
        debugged.push(id);
      },
    });
    await panel.show();
    await panel.refresh(run("mixed"));

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain('data-id="example.com/cart/CartTestSuite"');
    expect(html).toContain(
      'data-id="example.com/cart/CartTestSuite/totals_the_basket"',
    );
    const handle = (msg: unknown) =>
      (panel as unknown as { handleMessage(m: unknown): void }).handleMessage(
        msg,
      );
    handle({ type: "run", id: "example.com/cart/CartTestSuite/adds_an_item" });
    handle({ type: "rerunFailed" });
    handle({ type: "debug", id: "example.com/cart/CartTestSuite" });
    expect(ran).toEqual([
      ["example.com/cart/CartTestSuite/adds_an_item"],
      ["example.com/cart/CartTestSuite/totals_the_basket"],
    ]);
    expect(debugged).toEqual(["example.com/cart/CartTestSuite"]);
  });

  it("reruns no failure when nothing failed", async () => {
    const { panel } = newPanel();
    const ran: string[][] = [];
    panel.setActions({
      run: async (ids) => void ran.push(ids),
      debug: async () => {},
    });
    await panel.show();
    await panel.refresh(run("all-pass"));
    (panel as unknown as { handleMessage(m: unknown): void }).handleMessage({
      type: "rerunFailed",
    });
    expect(ran).toEqual([]);
  });
});

describe("failure output", () => {
  it("links the file:line it names to the source in the package", async () => {
    const dir = mkdtempSync(path.join(tmpdir(), "gotest-cart-"));
    try {
      writeFileSync(path.join(dir, "cart_test.go"), "package cart\n");
      const cache = new DiscoveryCache();
      cache.update(
        [{ importPath: "example.com/cart", dir, suites: [] } as never],
        true,
        dir,
      );
      const recorder = createRecordingChannel();
      const panel = new PanelHost(recorder.channel as never, cache);
      await panel.show();
      await panel.refresh(run("mixed"));

      expect(recorder.errors).toEqual([]);
      expect(await renderedHtml()).toContain(
        `<a class="src-link" href="#" data-act="goTo" data-file="${path.join(dir, "cart_test.go")}" data-line="42">cart_test.go:42</a>`,
      );
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});

describe("packages discovery no longer reports", () => {
  it("leave the panel when their folder's discovery comes back without them", async () => {
    const cache = new DiscoveryCache();
    // Each package declares the suite and the methods its recorded stream
    // reports, so only the package prune decides what leaves.
    const pkg = (importPath: string, suite: string, methods: string[]) =>
      ({
        importPath,
        dir: `/src/${importPath}`,
        suites: [{ name: suite, methods: methods.map((name) => ({ name })) }],
      }) as never;
    const cart = pkg("example.com/cart", "CartTestSuite", [
      "adds_an_item",
      "totals_the_basket",
    ]);
    const checkout = pkg("example.com/checkout", "CheckoutTestSuite", [
      "rejects_an_expired_card",
    ]);
    cache.update([cart, checkout], true, state.workspaceDir);
    const recorder = createRecordingChannel();
    const panel = new PanelHost(recorder.channel as never, cache);
    await panel.show();
    await panel.refresh(run("all-pass"));
    await panel.refresh(run("other-package-fail"));
    expect(await renderedHtml()).toContain("adds an item");

    // cart was renamed: the next discovery of the folder no longer has it.
    cache.update([checkout], true, state.workspaceDir);
    await panel.show();

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).not.toContain("adds an item");
    expect(html).toContain("rejects an expired card");
  });
});

describe("clearing drops the kept runs", () => {
  // Clearing has to drop what is kept, not just repaint, or the next refresh
  // would resurrect what was cleared.
  it("empties the panel and starts the next refresh from nothing", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    await panel.refresh(run("mixed"));
    await panel.refresh(run("other-package-fail"));
    expect(await renderedHtml()).toContain("rejects an expired card");

    panel.clear();
    const cleared = await renderedHtml();
    expect(cleared).not.toContain("rejects an expired card");
    expect(cleared).not.toContain("totals the basket");

    // A later run shows only itself — the cleared runs are gone for good.
    await panel.refresh(run("all-pass"));
    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("2 passed");
    expect(html).not.toContain("rejects an expired card");
  });
});

describe("autoRefresh honours the user's setting", () => {
  it("does not repaint the panel when autoRefresh is off", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    const before = await renderedHtml();

    state.autoRefresh = false;
    await panel.refresh(run("mixed"));

    expect(recorder.errors).toEqual([]);
    expect(await renderedHtml()).toBe(before);
  });

  it("keeps what ran meanwhile and shows it when asked", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    state.autoRefresh = false;
    await panel.refresh(run("all-pass"));
    await panel.refresh(run("other-package-fail"));
    await panel.show();

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("adds an item");
    expect(html).toContain("rejects an expired card");
  });
});

describe("a hidden panel", () => {
  it("renders nothing until it is seen, then shows every run it missed", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    const shown = state.panels[state.panels.length - 1] as unknown as {
      visible: boolean;
    };
    const before = await renderedHtml();

    shown.visible = false;
    await panel.refresh(run("all-pass"));
    await panel.refresh(run("other-package-fail"));
    expect(await renderedHtml()).toBe(before);

    shown.visible = true;
    await panel.show();
    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("adds an item");
    expect(html).toContain("rejects an expired card");
  });
});

// Tier 2 with the source present. The Test Explorer labels a behavior from
// discovery; the panel labels the same behavior from a replay of the run's
// stream. The two share no code path, so they are the pair that can drift —
// and did: a replay rendered "classifying a number" while the tree next to it
// said "when classifying a number". The fixture streams above name packages no
// checkout can load, so only this case exercises the replay reading source.
describe("the panel speaks the vocabulary the tree speaks", () => {
  const fixturesDir = path.join(extensionDir, "testdata", "fixtures");
  let savedGoWork: string | undefined;
  let captured = "";
  let tmp = "";

  beforeAll(() => {
    // The same workspace behaviorTree.test.ts discovers: the fixture module
    // resolves gotest through a go.work that spans this checkout.
    tmp = mkdtempSync(path.join(tmpdir(), "gotest-spec-vocab-"));
    const workFile = path.join(tmp, "corpus.work");
    writeFileSync(
      workFile,
      `go 1.26.0\n\nuse (\n\t${repoRoot}\n\t${path.join(repoRoot, "examples")}\n\t${fixturesDir}\n)\n`,
      "utf-8",
    );
    savedGoWork = process.env.GOWORK;
    process.env.GOWORK = workFile;

    const run = spawnSync(realBinary, ["gotest.fixtures/table", "-json"], {
      cwd: fixturesDir,
      encoding: "utf-8",
    });
    expect(run.error, "spawning the built CLI").toBeUndefined();
    expect(run.status, run.stderr).toBe(0);
    captured = run.stdout;
  }, 300_000);

  afterAll(() => {
    if (savedGoWork === undefined) delete process.env.GOWORK;
    else process.env.GOWORK = savedGoWork;
    if (tmp) rmSync(tmp, { recursive: true, force: true });
  });

  it("labels a context in the replay exactly as the tree labels it", async () => {
    state.workspaceDir = fixturesDir;
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh({ stream: captured, folder: fixturesDir });

    const html = await renderedHtml();
    expect(recorder.errors).toEqual([]);
    // The string behaviorTree.test.ts expects as this item's label in the tree.
    expect(html).toContain(
      '<span class="lbl when">when classifying a number</span>',
    );
    expect(html).not.toContain("when when");
    // Rows and expectations stand on their own — only a context takes a
    // connective.
    expect(html).toContain('<span class="lbl">negative</span>');
  }, 300_000);
});
