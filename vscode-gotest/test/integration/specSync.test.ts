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
  panels: [] as {
    webview: { html: string; cspSource: string };
  }[],
}));

vi.mock("vscode", async () => {
  const { buildVscodeStub } = await import("./vscodeStub.js");
  return buildVscodeStub(state as never);
});

import { SpecViewPanel } from "../../src/specView.js";
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
  const panel = new SpecViewPanel(recorder.channel as never);
  return { panel, recorder };
}

function renderedHtml(): string {
  expect(state.panels.length).toBeGreaterThan(0);
  return state.panels[state.panels.length - 1].webview.html;
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

    const html = renderedHtml();
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

    const html = renderedHtml();
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

    const html = renderedHtml();
    expect(html).toContain('class="node fuzz"');
    expect(html).toContain("TrimIdempotent");
    expect(html).toContain("drops surrounding spaces");
    // The row is tagged as a property backed by seeds, and the trailer counts
    // the target the way the CLI does.
    expect(html).toContain('<span class="tag fuzz">FUZZ · 3 seeds</span>');
    expect(html).toContain("1 fuzz target");
    expect(recorder.errors).toEqual([]);
  });

  it("links a fuzz target's row to its method, as it does for test methods", async () => {
    // The location map is fed by discovery; the fuzz target sits beside the
    // methods there and must reach the row the same way.
    const cache = new DiscoveryCache();
    const fuzzingDir = path.join(
      extensionDir,
      "testdata",
      "fixtures",
      "fuzzing",
    );
    const method = {
      parallel: false,
      focused: false,
      excluded: false,
      file: "suite_test.go",
      col: 1,
    };
    cache.update(
      [
        {
          importPath: "gotest.fixtures/fuzzing",
          dir: fuzzingDir,
          suites: [
            {
              name: "FuzzingTestSuite",
              parallel: false,
              focused: false,
              excluded: false,
              guarded: false,
              file: "suite_test.go",
              line: 11,
              col: 6,
              lifecycle: [],
              fixtures: [],
              methods: [{ ...method, name: "TestTrim", line: 13 }],
              benchmarks: [],
              fuzzers: [{ ...method, name: "FuzzTrimIdempotent", line: 19 }],
            },
          ],
        } as never,
      ],
      true,
      path.join(extensionDir, "testdata", "fixtures"),
    );
    const recorder = createRecordingChannel();
    const panel = new SpecViewPanel(recorder.channel as never, cache);
    await panel.show();
    await panel.refresh(run("fuzz"));

    const html = renderedHtml();
    const fuzzRow = html.slice(
      html.indexOf('class="node fuzz"'),
      html.indexOf("</summary>", html.indexOf('class="node fuzz"')),
    );
    expect(fuzzRow).toContain('data-loc-line="19"');
    expect(fuzzRow).toContain("fuzzing");
    const methodRow = html.slice(
      html.indexOf('class="node method"'),
      html.indexOf("</summary>", html.indexOf('class="node method"')),
    );
    expect(methodRow).toContain('data-loc-line="13"');
  });

  it("renders a package that failed to build, which has no failing behaviour to speak for it", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(run("package-failure"));

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("FAIL");
    expect(html).toContain("undefined: helper");
    expect(html).toContain("1 failed packages");
  });

  // The tree is the CLI's: a slash in an It is one subtest, so one row; a
  // label reaches the webview raw and is escaped once, here.
  it("renders labels as the CLI's spec does: one row per slash, escaped once", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(run("labels"));

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain(
      '<div class="leaf block pass" data-display="reads a/b paths">',
    );
    expect(html).not.toContain('data-display="reads a"');
    expect(html).toContain('data-display="keeps &lt;b&gt;&amp;&quot; text">');
    expect(html).toContain(' keeps &lt;b&gt;&amp;" text <span class="dur">');
    expect(html).not.toContain("&amp;lt;");
    expect(html).not.toContain("&amp;amp;");
    // A run skips an excluded suite, and the CLI's run spec says so.
    expect(html).toContain(" Parked — SKIPPED ");
    expect(html).toContain("2 passed");
  });

  it("renders an empty stream as an empty spec, not as an error", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(run("empty"));

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    // No package and no rendered node — asserted on content rather than on the
    // static template, which carries status glyphs of its own.
    expect(html).not.toContain("example.com");
    expect(html).not.toContain('class="leaf');
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
    const panel = new SpecViewPanel(channel as never);
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
      const panel = new SpecViewPanel(channel as never);
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
      const panel = new SpecViewPanel(channel as never);
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
      expect(renderedHtml()).not.toContain("totals the basket");
    },
  );
});

// Tier 2 — refresh() is stateful. The panel keeps the newest lines of every
// test of every package it has seen and renders them as one stream, so what
// it shows depends on every run before the last. No exit-code assertion
// reaches this.

// How many times a rendered row carries this display text.
function rows(html: string, display: string): number {
  return html.split(`data-display="${display}"`).length - 1;
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
    expect(renderedHtml()).toContain("1 failed");

    await panel.refresh(run("all-pass"));
    const html = renderedHtml();
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

    const html = renderedHtml();
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

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("1 suite,");
    expect(html).toContain("2 behaviors");
    expect(html).toContain("2 passed");
    expect(html).not.toContain("4 passed");
  });

  it("keeps a package when a later run reports another one", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    await panel.refresh(run("all-pass"));
    await panel.refresh(run("other-package-fail"));

    const html = renderedHtml();
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

    const html = renderedHtml();
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

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    const summary = html.slice(
      html.indexOf('<summary class="node suite"'),
      html.indexOf("</summary>", html.indexOf('<summary class="node suite"')),
    );
    expect(summary).toContain('class="icon fail"');
    const said = /<pre class="error-output">([^<]*)<\/pre>/.exec(html)?.[1];
    expect(said).toBe("store_test.go:40: AfterAll: close db: connection reset");
  });
});

describe("go to source", () => {
  it("links every declared When and It to its line, not only suites and methods", async () => {
    const cache = new DiscoveryCache();
    cache.update(
      [
        {
          importPath: "example.com/cart",
          dir: "/src/cart",
          suites: [
            {
              name: "CartTestSuite",
              parallel: false,
              focused: false,
              excluded: false,
              guarded: false,
              file: "cart_test.go",
              line: 5,
              col: 6,
              lifecycle: [],
              fixtures: [],
              methods: [
                {
                  name: "TestAdd",
                  parallel: false,
                  focused: false,
                  excluded: false,
                  file: "add_test.go",
                  line: 10,
                  col: 1,
                  behaviors: [
                    {
                      name: "an_empty_cart",
                      display: "when an empty cart",
                      kind: "when",
                      line: 11,
                      children: [
                        {
                          name: "holds_one_item",
                          display: "holds one item",
                          kind: "it",
                          line: 12,
                        },
                      ],
                    },
                  ],
                },
              ],
              benchmarks: [],
              fuzzers: [],
            },
          ],
        } as never,
      ],
      true,
      "/src",
    );
    const nested = [
      "TestCartTestSuite",
      "TestCartTestSuite/TestAdd",
      "TestCartTestSuite/TestAdd/an_empty_cart",
      "TestCartTestSuite/TestAdd/an_empty_cart/holds_one_item",
    ];
    const events = [
      ...nested.map((t) => ({
        Action: "run",
        Package: "example.com/cart",
        Test: t,
      })),
      ...[...nested].reverse().map((t) => ({
        Action: "pass",
        Package: "example.com/cart",
        Test: t,
        Elapsed: 0.01,
      })),
      { Action: "pass", Package: "example.com/cart", Elapsed: 0.02 },
    ];
    const recorder = createRecordingChannel();
    const panel = new SpecViewPanel(recorder.channel as never, cache);
    await panel.show();
    await panel.refresh({
      stream: events.map((e) => JSON.stringify(e)).join("\n"),
      folder: state.workspaceDir,
    });

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    const rowOf = (display: string) => {
      const at = html.indexOf(`data-display="${display}"`);
      return html.slice(at, html.indexOf("\n", html.indexOf("<span", at)));
    };
    expect(rowOf("an empty cart")).toContain(
      'data-loc-file="/src/cart/add_test.go" data-loc-line="11"',
    );
    expect(rowOf("holds one item")).toContain(
      'data-loc-file="/src/cart/add_test.go" data-loc-line="12"',
    );
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

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain('data-id="example.com/cart/CartTestSuite"');
    expect(html).toContain(
      'data-id="example.com/cart/CartTestSuite/totals_the_basket"',
    );
    expect(html).toMatch(/id="rerun-failed-btn"[^>]*>Rerun failed/);
    expect(html).not.toMatch(/id="rerun-failed-btn"[^>]*disabled/);

    const handle = (msg: unknown) =>
      (panel as unknown as { handleMessage(m: unknown): void }).handleMessage(
        msg,
      );
    handle({ type: "run", id: "example.com/cart/CartTestSuite/adds_an_item" });
    handle({ type: "run", ids: ["a", "b"] });
    handle({ type: "debug", id: "example.com/cart/CartTestSuite" });
    expect(ran).toEqual([
      ["example.com/cart/CartTestSuite/adds_an_item"],
      ["a", "b"],
    ]);
    expect(debugged).toEqual(["example.com/cart/CartTestSuite"]);
  });

  it("offers no rerun of failures when nothing failed", async () => {
    const { panel } = newPanel();
    await panel.show();
    await panel.refresh(run("all-pass"));

    expect(renderedHtml()).toMatch(/id="rerun-failed-btn"[^>]*disabled/);
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
      const panel = new SpecViewPanel(recorder.channel as never, cache);
      await panel.show();
      await panel.refresh(run("mixed"));

      expect(recorder.errors).toEqual([]);
      expect(renderedHtml()).toContain(
        `<a class="src-link" data-loc-file="${path.join(dir, "cart_test.go")}" data-loc-line="42">cart_test.go:42</a>`,
      );
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
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
    expect(renderedHtml()).toContain("rejects an expired card");

    panel.clear();
    const cleared = renderedHtml();
    expect(cleared).not.toContain("rejects an expired card");
    expect(cleared).not.toContain("totals the basket");

    // A later run shows only itself — the cleared runs are gone for good.
    await panel.refresh(run("all-pass"));
    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("2 passed");
    expect(html).not.toContain("rejects an expired card");
  });
});

describe("autoRefresh honours the user's setting", () => {
  it("does not repaint the panel when autoRefresh is off", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    const before = renderedHtml();

    state.autoRefresh = false;
    await panel.refresh(run("mixed"));

    expect(recorder.errors).toEqual([]);
    expect(renderedHtml()).toBe(before);
  });

  it("keeps what ran meanwhile and shows it when asked", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    state.autoRefresh = false;
    await panel.refresh(run("all-pass"));
    await panel.refresh(run("other-package-fail"));
    await panel.show();

    const html = renderedHtml();
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
    const before = renderedHtml();

    shown.visible = false;
    await panel.refresh(run("all-pass"));
    await panel.refresh(run("other-package-fail"));
    expect(renderedHtml()).toBe(before);

    shown.visible = true;
    await panel.show();
    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("adds an item");
    expect(html).toContain("rejects an expired card");
  });
});

// Tier 2 with the source present. The Test Explorer labels a behavior from
// discovery; the Spec View labels the same behavior from a replay of the run's
// stream. The two share no code path, so they are the pair that can drift —
// and did: a replay rendered "classifying a number" while the tree next to it
// said "when classifying a number". The fixture streams above name packages no
// checkout can load, so only this case exercises the replay reading source.
describe("the Spec View speaks the vocabulary the tree speaks", () => {
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

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    // The string behaviorTree.test.ts expects as this item's label in the tree.
    expect(html).toContain('data-display="when classifying a number"');
    expect(html).not.toContain("when when");
    // Rows and expectations stand on their own — only a context takes a
    // connective.
    expect(html).toContain('data-display="negative"');
  }, 300_000);
});
