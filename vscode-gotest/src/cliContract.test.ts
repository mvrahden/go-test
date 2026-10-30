// Tier 0 — the recorded CLI contract.
//
// These assertions run against bytes the real `gotest spec --input=-` actually
// produced (see scripts/record-cli-contract.mjs), not bytes we imagined. The
// regression this guards shipped because the CLI's exit rule and the
// extension's reading of it were each tested in isolation and never together.
//
// Two variants are recorded per stream: the gating form, whose exit code CI
// depends on, and the --render-only form the Spec View sends. The `spec`
// runs replay a stream with --declared or --hide.

import { describe, it, expect, vi } from "vitest";
import { readFileSync } from "node:fs";
import * as path from "node:path";

vi.mock("vscode", () => {
  class Position {
    constructor(
      public line: number,
      public character: number,
    ) {}
  }
  class Range {
    constructor(
      public start: Position,
      public end: Position,
    ) {}
  }
  class StatementCoverage {
    constructor(
      public executed: number | boolean,
      public location: Range,
    ) {}
  }
  class EventEmitter {
    event = () => ({ dispose: () => {} });
    fire() {}
    dispose() {}
  }
  return {
    Uri: {
      joinPath: (...args: string[]) => ({ toString: () => args.join("/") }),
    },
    workspace: { getConfiguration: () => ({ get: () => true }) },
    window: {},
    ViewColumn: { Beside: 2 },
    commands: {},
    Position,
    Range,
    StatementCoverage,
    EventEmitter,
  };
});

import { interpretSpecExit } from "./specView.js";
import { parseTestEvents } from "./outputParser.js";
import { DiscoveryCache, parseDiscoverOutput } from "./discovery.js";
import { parseBenchReport } from "./benchReport.js";
import { parseCoverProfile } from "./coverage.js";
import { parsePromotedSeed } from "./fuzz.js";
import { parseLintJson } from "./lintFindings.js";

interface Invocation {
  exitCode: number | null;
  stdout: string;
  stderr: string;
}

interface Variant {
  direct: Invocation;
  goRun: {
    exitCode: number | null;
    stderr: string;
    stdoutMatchesDirect: boolean;
  };
}

interface LiveEvent {
  Action: string;
  Package?: string;
  Test?: string;
  Output?: string;
  RunLevel?: boolean;
}

// A live `gotest -json` run, times and durations masked as "<d>".
interface LiveRun {
  args: string[];
  exitCode: number | null;
  stderr: string;
  events: LiveEvent[];
}

// A `gotest lint -json` run, its paths masked to "<fixtures>".
interface LintRun {
  args: string[];
  exitCode: number | null;
  stderr: string;
  document: Record<string, Record<string, unknown[]>> | null;
}

interface LintRule {
  id: string;
  tier: string;
  scope: string;
  skippable: boolean;
  fixable: boolean;
  description: string;
}

type Stats = Record<string, number>;

interface SpecNode {
  name: string;
  status: string;
  duration: number;
  file?: string;
  line?: number;
  children: SpecNode[];
}

interface SpecPackage {
  path: string;
  dir?: string;
  status: string;
  nodes: SpecNode[];
  stats?: Stats;
}

interface SpecDocument {
  version: string;
  packages: SpecPackage[];
  stats: Stats;
}

// A `gotest spec --input=-` run over a stream under testdata/, with
// --declared or --hide; the repository's directory is masked as "<repo>".
interface SpecRun {
  stream: string;
  args: string[];
  exitCode: number | null;
  stderr: string;
  document: SpecDocument;
}

interface ContractCase {
  gating: Variant;
  renderOnly: Variant;
}

// One run of a command whose document the extension reads. Absolute paths
// are masked as <repo> or <stage>; measurements and host facts are zeroed.
interface OutputRun {
  args: string[];
  exitCode: number | null;
  stderr: string;
  // The stdout JSON document, when there is one.
  document?: unknown;
  // Plain stdout otherwise.
  stdout?: string;
  // The cover profile the run wrote.
  profile?: string[] | null;
  // The source line a promote confirmation names.
  revealedLine?: string | null;
}

const golden = JSON.parse(
  readFileSync(
    path.join(__dirname, "..", "testdata", "cli-contract.json"),
    "utf-8",
  ),
) as {
  cases: Record<string, ContractCase>;
  spec: Record<string, SpecRun>;
  live: Record<string, LiveRun>;
  outputs: Record<string, OutputRun>;
  lint: Record<string, LintRun>;
};

// nodesOf lists every node beneath pkg, keyed by its slash-joined path.
function nodesOf(pkg: SpecPackage): Map<string, SpecNode> {
  const out = new Map<string, SpecNode>();
  const walk = (nodes: SpecNode[], prefix: string) => {
    for (const n of nodes) {
      const key = prefix + n.name;
      out.set(key, n);
      walk(n.children, key + "/");
    }
  };
  walk(pkg.nodes, "");
  return out;
}

// What each stream shape means. Stated here rather than derived from the
// recording, so that re-recording a changed CLI cannot quietly relabel a case:
// the golden supplies the bytes, this table supplies the intent, and a genuine
// behaviour change has to break one against the other.
const EXPECTED: Record<
  string,
  {
    gatingExitCode: number;
    // What the gating form complains about on stderr; "" when unset.
    gatingStderr?: string;
    goRunEpilogue: string;
    stats: Record<string, number>;
    packages: string[];
    // Set only for a stream whose package the recorder could load: the
    // replay then reads the declared label and speaks its vocabulary.
    speaks?: { display: string; vocab: string };
  }
> = {
  "all-pass": {
    gatingExitCode: 0,
    goRunEpilogue: "",
    stats: { passed: 2, failed: 0, skipped: 0 },
    packages: ["example.com/cart:pass"],
  },
  mixed: {
    gatingExitCode: 1,
    goRunEpilogue: "exit status 1\n",
    stats: { passed: 1, failed: 1, skipped: 0 },
    packages: ["example.com/cart:fail"],
  },
  "all-fail": {
    gatingExitCode: 1,
    goRunEpilogue: "exit status 1\n",
    stats: { passed: 0, failed: 1, skipped: 0 },
    packages: ["example.com/cart:fail"],
  },
  "skip-only": {
    gatingExitCode: 0,
    goRunEpilogue: "",
    stats: { passed: 0, failed: 0, skipped: 1 },
    packages: ["example.com/cart:pass"],
  },
  // A package that fails to build carries no failing behaviour at all. Only the
  // exit code and failedPackages say anything is wrong, which is exactly why a
  // consumer must not treat a non-zero exit as "the renderer broke".
  "package-failure": {
    gatingExitCode: 1,
    goRunEpilogue: "exit status 1\n",
    stats: { passed: 0, failed: 0, skipped: 0, failedPackages: 1 },
    packages: ["example.com/broken:fail"],
  },
  // The same failure in go test's own shape (Go 1.24+): build events and a
  // verdict naming the failed build. It gates as the live run does, on 2, and
  // says nothing extra on stderr: the diagnostics are in the spec.
  "build-failure": {
    gatingExitCode: 2,
    goRunEpilogue: "exit status 2\n",
    stats: { passed: 0, failed: 0, skipped: 0, failedPackages: 1 },
    packages: ["example.com/broken:fail"],
  },
  // A stream with no events holds no verdict: the gating form must not pass
  // it as green. The Spec View renders it anyway, under --render-only.
  empty: {
    gatingExitCode: 2,
    gatingStderr:
      "FAIL: input holds no test events (expected a go test -json stream)\n",
    goRunEpilogue: "exit status 2\n",
    stats: { passed: 0, failed: 0, skipped: 0 },
    packages: [],
  },
  // A second package, used by the layer-accumulation tests: concatenating it
  // with a passing layer must still render both.
  "other-package-fail": {
    gatingExitCode: 1,
    goRunEpilogue: "exit status 1\n",
    stats: { passed: 0, failed: 1, skipped: 0 },
    packages: ["example.com/checkout:fail"],
  },
  // The one stream naming a package of this repository. Every other case
  // pins the name-only fallback; this one pins what the Spec View actually
  // gets in a workspace: the label the developer wrote, spoken in its
  // vocabulary, with `vocab` on the wire — and nothing on stderr while the
  // package loads.
  // A fuzz target counts once in the stats and is a `fuzz` node under its
  // suite; its seed rows are evidence, not behaviors.
  fuzz: {
    gatingExitCode: 0,
    goRunEpilogue: "",
    stats: { passed: 2, failed: 0, skipped: 0 },
    packages: ["gotest.fixtures/fuzzing:pass"],
  },
  // Packages a run executes nothing in still end in go test's verdict
  // (skip); the spec has no behaviour to render for them, and the stream is
  // whole, so it passes.
  "no-suites": {
    gatingExitCode: 0,
    goRunEpilogue: "",
    stats: { passed: 0, failed: 0, skipped: 0 },
    packages: [],
  },
  // A package whose suites are all excluded, and one whose methods -run
  // deselected: both pass with go test's "[no tests to run]".
  "no-tests-to-run": {
    gatingExitCode: 0,
    goRunEpilogue: "",
    stats: { passed: 1, failed: 0, skipped: 1 },
    packages: ["example.com/excluded:pass", "example.com/plain:pass"],
  },
  // An It whose label holds a slash and one holding markup, beside an
  // excluded suite: the Spec View renders this tree as the CLI built it.
  labels: {
    gatingExitCode: 0,
    goRunEpilogue: "",
    stats: { passed: 2, failed: 0, skipped: 1 },
    packages: ["example.com/labels:pass"],
  },
  "declared-source": {
    gatingExitCode: 0,
    goRunEpilogue: "",
    stats: { passed: 1, failed: 0, skipped: 0 },
    packages: ["github.com/mvrahden/go-test/examples/search:pass"],
    speaks: { display: "when searching for a title keyword", vocab: "when" },
  },
};

describe("recorded CLI contract", () => {
  it("states an expectation for every recorded case, and records every stated case", () => {
    // Adding a stream fixture without declaring what it means would otherwise
    // let an unreviewed contract change ride along with a re-recording.
    expect(Object.keys(golden.cases).sort()).toEqual(
      Object.keys(EXPECTED).sort(),
    );
  });

  for (const [name, expected] of Object.entries(EXPECTED)) {
    describe(name, () => {
      const recorded = golden.cases[name];

      it("gates on the documented exit code", () => {
        expect(recorded.gating.direct.exitCode).toBe(expected.gatingExitCode);
      });

      it("renders the spec on stdout regardless of that code", () => {
        const spec = JSON.parse(recorded.gating.direct.stdout);
        expect(spec).toHaveProperty("packages");
        expect(
          spec.packages.map(
            (p: { path: string; status: string }) => `${p.path}:${p.status}`,
          ),
        ).toEqual(expected.packages);
        expect(spec.stats).toMatchObject(expected.stats);
      });

      it("keeps stderr to the documented complaint when invoked directly", () => {
        expect(recorded.gating.direct.stderr).toBe(expected.gatingStderr ?? "");
      });

      if (expected.speaks) {
        it("speaks the declared vocabulary when the source is reachable", () => {
          const stdout = recorded.gating.direct.stdout;
          expect(stdout).toContain(`"display":"${expected.speaks!.display}"`);
          expect(stdout).toContain(`"vocab":"${expected.speaks!.vocab}"`);
        });
      }

      it("gains only go run's exit-status epilogue on stderr, never a changed spec", () => {
        // go run reports any non-zero exit of the program as 1.
        expect(recorded.gating.goRun.exitCode).toBe(
          expected.gatingExitCode === 0 ? 0 : 1,
        );
        expect(recorded.gating.goRun.stderr).toBe(
          (expected.gatingStderr ?? "") + expected.goRunEpilogue,
        );
        expect(recorded.gating.goRun.stdoutMatchesDirect).toBe(true);
      });

      // --render-only exists to separate "did you render" from "did the tests
      // pass". It must drop the verdict and change nothing else.
      it("renders byte-identically under --render-only", () => {
        expect(recorded.renderOnly.direct.stdout).toBe(
          recorded.gating.direct.stdout,
        );
      });

      it("always succeeds under --render-only, whatever the verdict", () => {
        expect(recorded.renderOnly.direct.exitCode).toBe(0);
        expect(recorded.renderOnly.direct.stderr).toBe("");
      });

      it("leaves go run with nothing to append under --render-only", () => {
        // With no non-zero exit there is no epilogue, so the hazard that broke
        // the Spec View cannot arise on this path at all.
        expect(recorded.renderOnly.goRun.exitCode).toBe(0);
        expect(recorded.renderOnly.goRun.stderr).toBe("");
        expect(recorded.renderOnly.goRun.stdoutMatchesDirect).toBe(true);
      });

      if (expected.gatingExitCode === 2) {
        // Exit 2 is the command's own failure: no client may read it as green.
        it("is read as a failure when invoked directly", () => {
          expect(
            interpretSpecExit(
              recorded.gating.direct.exitCode,
              recorded.gating.direct.stdout,
              recorded.gating.direct.stderr,
            ).ok,
          ).toBe(false);
        });
      } else {
        it("is read as a usable spec when invoked directly", () => {
          expect(
            interpretSpecExit(
              recorded.gating.direct.exitCode,
              recorded.gating.direct.stdout,
              recorded.gating.direct.stderr,
            ),
          ).toEqual({ ok: true, stdout: recorded.gating.direct.stdout });
        });
      }

      // The original regression in one assertion: same bytes, same verdict, but
      // reached through the extension's default `go run` resolution. The client
      // now sends --render-only, but this must keep holding — it is the safety
      // net for any CLI that predates the flag.
      it("is read as a usable spec when resolved through go run", () => {
        expect(
          interpretSpecExit(
            recorded.gating.goRun.exitCode,
            recorded.gating.direct.stdout,
            recorded.gating.goRun.stderr,
          ),
        ).toEqual({ ok: true, stdout: recorded.gating.direct.stdout });
      });

      it("is read as a usable spec on the render-only path the extension uses", () => {
        expect(
          interpretSpecExit(
            recorded.renderOnly.goRun.exitCode,
            recorded.renderOnly.direct.stdout,
            recorded.renderOnly.goRun.stderr,
          ),
        ).toEqual({ ok: true, stdout: recorded.renderOnly.direct.stdout });
      });
    });
  }

  describe("labels", () => {
    type Node = {
      display: string;
      excluded: boolean;
      status: string;
      children: Node[];
    };
    const spec = JSON.parse(golden.cases["labels"].gating.direct.stdout) as {
      packages: { nodes: Node[] }[];
    };
    const [paths, parked] = spec.packages[0].nodes;
    const reading = paths.children[0].children[0];

    it("keeps an It with a slash as one leaf under its When", () => {
      expect(reading.children.map((c) => c.display)).toEqual([
        "reads a/b paths",
        'keeps <b>&" text',
      ]);
      expect(reading.children[0].children).toEqual([]);
    });

    it("sends labels raw, for the Spec View to escape once", () => {
      expect(reading.children[1].display).toBe('keeps <b>&" text');
    });

    it("marks an excluded suite excluded and skipped", () => {
      expect(parked).toMatchObject({
        display: "Parked",
        excluded: true,
        status: "skip",
      });
    });
  });

  it("covers both a failing and a passing verdict, so a green-only fixture set cannot pass", () => {
    const codes = Object.values(EXPECTED).map((e) => e.gatingExitCode);
    expect(codes).toContain(0);
    expect(codes).toContain(1);
  });

  it("covers a case where go run actually appends an epilogue", () => {
    // Without this, every fixture could be green and the suite would still
    // look complete while testing nothing the regression touched.
    const withEpilogue = Object.values(EXPECTED).filter(
      (e) => e.goRunEpilogue !== "",
    );
    expect(withEpilogue.length).toBeGreaterThan(0);
  });

  it("keeps the gating verdict meaningful, so --render-only cannot become the only behaviour", () => {
    // If a future change made every stream exit 0, CI would silently stop
    // gating. The recorded gating variant is what keeps that honest.
    const failing = Object.entries(EXPECTED).filter(
      ([, e]) => e.gatingExitCode === 1,
    );
    for (const [name] of failing) {
      expect(golden.cases[name].gating.direct.exitCode).toBe(1);
    }
    expect(failing.length).toBeGreaterThan(0);
  });
});

describe("recorded spec runs", () => {
  it("records exactly the stated runs", () => {
    expect(Object.keys(golden.spec).sort()).toEqual([
      "declared",
      "hide-nothing",
      "hide-pass",
    ]);
  });

  for (const [name, recorded] of Object.entries(golden.spec)) {
    it(`${name}: renders with exit 0 and nothing on stderr`, () => {
      expect(recorded.exitCode).toBe(0);
      expect(recorded.stderr).toBe("");
    });
  }

  // The stream reports one method of examples/search and one the source
  // does not declare; the source declares the rest.
  describe("--declared", () => {
    const recorded = golden.spec.declared;
    const pkg = recorded.document.packages[0];
    const nodes = nodesOf(pkg);
    const streamOnly = "TestArticleSearchTestSuite/TestSearchByAuthor";

    it("renders the one package, with its directory", () => {
      expect(recorded.document.packages.map((p) => p.path)).toEqual([
        "github.com/mvrahden/go-test/examples/search",
      ]);
      expect(pkg.dir).toBe("<repo>/examples/search");
    });

    it("places every node the source declares at its file and line", () => {
      const declared = [...nodes].filter(([key]) => key !== streamOnly);
      expect(declared.length).toBeGreaterThan(0);
      for (const [, n] of declared) {
        expect(n.file).toMatch(/^[^/]+_test\.go$/);
        expect(Number.isInteger(n.line)).toBe(true);
        expect(n.line).toBeGreaterThan(0);
      }
    });

    it("keeps a node only the stream holds, with no file or line", () => {
      const n = nodes.get(streamOnly);
      expect(n?.status).toBe("pass");
      expect(n).not.toHaveProperty("file");
      expect(n).not.toHaveProperty("line");
    });

    it("gives a declared node the stream did not report no verdict", () => {
      const n = nodes.get("TestArticleSearchTestSuite/TestSearchByBody");
      expect(n?.status).toBe("none");
      expect(n?.duration).toBe(0);
      expect(n?.file).toBe("suite_test.go");
    });

    it("counts every behavior the stream did not report as not run", () => {
      const leaves = [...nodes.values()].filter((n) => n.children.length === 0);
      const unreported = leaves.filter((n) => n.status === "none");
      expect(unreported.length).toBeGreaterThan(0);
      expect(recorded.document.stats.notRun).toBe(unreported.length);
    });

    it("gives the package its own stats, here the document's", () => {
      expect(pkg.stats).toEqual(recorded.document.stats);
    });

    it("adds dir, file and line only under --declared", () => {
      const plain = JSON.parse(
        golden.cases["declared-source"].renderOnly.direct.stdout,
      ) as SpecDocument;
      expect(plain.packages[0]).not.toHaveProperty("dir");
      expect(plain.packages[0]).not.toHaveProperty("stats");
      for (const n of nodesOf(plain.packages[0]).values()) {
        expect(n).not.toHaveProperty("file");
        expect(n).not.toHaveProperty("line");
      }
    });
  });

  // Both replay mixed.jsonl: one behavior passes, one fails.
  describe("--hide", () => {
    const hidden = golden.spec["hide-pass"].document;
    const nothing = golden.spec["hide-nothing"].document;
    const plain = JSON.parse(
      golden.cases.mixed.renderOnly.direct.stdout,
    ) as SpecDocument;
    const leaves = (doc: SpecDocument) =>
      [...nodesOf(doc.packages[0]).keys()].sort();

    it("leaves the counts the run's", () => {
      expect(hidden.stats).toEqual(plain.stats);
      expect(nothing.stats).toEqual(plain.stats);
    });

    it("gives each package its own stats, even when it hides nothing", () => {
      expect(hidden.packages[0].stats).toEqual(plain.stats);
      expect(nothing.packages[0].stats).toEqual(plain.stats);
      expect(plain.packages[0]).not.toHaveProperty("stats");
    });

    it("leaves the passing behavior out under --hide=pass", () => {
      expect(leaves(hidden)).toEqual([
        "TestCartTestSuite",
        "TestCartTestSuite/totals_the_basket",
      ]);
    });

    it("hides nothing under --hide=", () => {
      const { stats, ...rest } = nothing.packages[0];
      expect(stats).toBeDefined();
      expect(rest).toEqual(plain.packages[0]);
      expect(leaves(nothing)).toContain("TestCartTestSuite/adds_an_item");
    });
  });
});

const unrunPkg = "github.com/mvrahden/go-test/cmd/gotest/testdata/unrun/";

describe("recorded live runs", () => {
  it("records exactly the stated runs", () => {
    expect(Object.keys(golden.live).sort()).toEqual([
      "no-packages",
      "run-level",
      "verdicts",
    ]);
  });

  describe("patterns that match no package", () => {
    const recorded = golden.live["no-packages"];
    it("exits 2, names the pattern on stderr and writes no stream", () => {
      expect(recorded.exitCode).toBe(2);
      expect(recorded.stderr).toBe(
        `FAIL: "${recorded.args[1]}" matched no packages\n`,
      );
      expect(recorded.events).toEqual([]);
    });
  });

  describe("a run with packages it executes nothing in", () => {
    const recorded = golden.live.verdicts;
    const pkgs = [
      `${unrunPkg}notestfiles`,
      `${unrunPkg}nosuites`,
      `${unrunPkg}excluded`,
      "github.com/mvrahden/go-test/cmd/gotest/testdata/census/plain",
    ];

    it("passes, saying on stderr only that the stdlib-only package was not run", () => {
      expect(recorded.exitCode).toBe(0);
      expect(recorded.stderr).toBe(
        `note: 1 stdlib test(s) in ${unrunPkg}nosuites not run — gotest runs suites; use 'go test' for stdlib tests\n`,
      );
    });

    it("opens every package with one start, before any other event of it", () => {
      for (const pkg of pkgs) {
        const own = recorded.events.filter((e) => e.Package === pkg);
        expect(own[0]?.Action).toBe("start");
        expect(own.filter((e) => e.Action === "start")).toHaveLength(1);
      }
    });

    it("ends every package in go test's verdict", () => {
      const verdict = (pkg: string) =>
        recorded.events
          .filter((e) => e.Package === pkg && !e.Test)
          .slice(-2)
          .map((e) => (e.Action === "output" ? e.Output : e.Action));
      expect(verdict(`${unrunPkg}notestfiles`)).toEqual([
        `?   \t${unrunPkg}notestfiles\t[no test files]\n`,
        "skip",
      ]);
      expect(verdict(`${unrunPkg}nosuites`)).toEqual([
        `?   \t${unrunPkg}nosuites\t[no suites]\n`,
        "skip",
      ]);
      expect(verdict(`${unrunPkg}excluded`)).toEqual([
        `ok  \t${unrunPkg}excluded\t<d>s [no tests to run]\n`,
        "pass",
      ]);
      expect(verdict(pkgs[3])).toEqual([`ok  \t${pkgs[3]}\t<d>s\n`, "pass"]);
    });

    it("parses in full with the extension's stream reader", () => {
      const stream = recorded.events.map((e) => JSON.stringify(e)).join("\n");
      expect(parseTestEvents(stream)).toHaveLength(recorded.events.length);
    });

    it("marks no event as the run's own", () => {
      expect(recorded.events.filter((e) => "RunLevel" in e)).toEqual([]);
    });
  });

  // A module the recorder writes: one suite reading a shared fixture whose
  // setup fails, booked by the run under a package of its own.
  describe("a shared fixture that never comes up", () => {
    const recorded = golden.live["run-level"];
    const runLevel = recorded.events.filter(
      (e) => e.Package === "shared fixtures",
    );
    const suite = recorded.events.filter(
      (e) => e.Package === "gotest.runlevel/down",
    );

    it("fails the run, the suite reading the fixture with it", () => {
      expect(recorded.exitCode).toBe(1);
      expect(recorded.stderr).toContain(
        "DownSharedFixture.BeforeAll failed after 1 attempt(s)",
      );
      expect(suite.at(-1)?.Action).toBe("fail");
      expect(runLevel.at(-1)?.Action).toBe("fail");
    });

    it("marks every event of the run's own package, its start first", () => {
      expect(runLevel[0]?.Action).toBe("start");
      for (const e of runLevel) expect(e.RunLevel).toBe(true);
    });

    it("marks the run's own package, and no other, in the spec document", () => {
      const spec = (
        recorded as unknown as {
          spec: {
            exitCode: number;
            packages: { path: string; runLevel: boolean }[];
          };
        }
      ).spec;
      expect(spec.exitCode).toBe(0);
      expect(spec.packages).toEqual([
        { path: "gotest.runlevel/down", runLevel: false },
        { path: "shared fixtures", runLevel: true },
      ]);
    });

    it("marks no event of a package it ran", () => {
      expect(suite.length).toBeGreaterThan(0);
      expect(
        recorded.events.filter(
          (e) => e.Package !== "shared fixtures" && "RunLevel" in e,
        ),
      ).toEqual([]);
    });
  });
});

const repoModule = "github.com/mvrahden/go-test";
const census = `${repoModule}/cmd/gotest/testdata/census/`;
const searchPkg = `${repoModule}/examples/search`;

// The recorded discover payload, through the extension's reader.
function discovered() {
  const recorded = golden.outputs.discover;
  return parseDiscoverOutput(JSON.stringify(recorded.document));
}

// A cache filled from the recorded payload, as a full workspace scan fills it.
function discoveredCache(): DiscoveryCache {
  const cache = new DiscoveryCache();
  const output = discovered();
  cache.update(output.packages, true, "<repo>", output.warnings);
  return cache;
}

// promotedSeeds is what the extension reads from promote's stdout.
function promotedSeeds(stdout: string) {
  return stdout
    .split("\n")
    .map(parsePromotedSeed)
    .filter((v) => v !== undefined);
}

describe("recorded outputs", () => {
  it("records exactly the stated outputs", () => {
    expect(Object.keys(golden.outputs).sort()).toEqual([
      "bench",
      "cover",
      "discover",
      "promote-nothing",
      "promote-refused",
      "promote-staged",
    ]);
  });

  describe("discover", () => {
    const recorded = golden.outputs.discover;

    it("succeeds with one JSON document and a silent stderr", () => {
      expect(recorded.exitCode).toBe(0);
      expect(recorded.stderr).toBe("");
      expect(recorded.document).toBeDefined();
    });

    it("lists every matched package with suites, each with its dir", () => {
      const pkgs = discovered().packages;
      expect(pkgs.map((p) => p.importPath)).toEqual([
        `${census}benchplain`,
        `${census}fuzzplain`,
        `${census}plain`,
        `${census}skipped`,
        searchPkg,
      ]);
      expect(pkgs.find((p) => p.importPath === searchPkg)?.dir).toBe(
        "<repo>/examples/search",
      );
    });

    it("sorts a suite's cases into tests, benchmarks and fuzzers", () => {
      const suite = (pkg: string) =>
        discovered().packages.find((p) => p.importPath === pkg)!.suites[0];
      const names = (ms: { name: string }[]) => ms.map((m) => m.name);
      const bench = suite(`${census}benchplain`);
      expect(names(bench.methods)).toEqual(["TestOne"]);
      expect(names(bench.benchmarks)).toEqual(["BenchmarkA", "BenchmarkB"]);
      expect(names(suite(`${census}fuzzplain`).fuzzers)).toEqual(["FuzzTrim"]);
      expect(suite(`${census}skipped`).lifecycle).toEqual(["BeforeAll"]);
    });

    it("carries a method's declared behaviors in their vocabulary", () => {
      const search = discovered().packages.find(
        (p) => p.importPath === searchPkg,
      )!;
      const first = search.suites[0].methods[0];
      expect(first.behaviorsComplete).toBe(true);
      expect(first.behaviors?.[0]).toMatchObject({
        display: "when searching for a title keyword",
        kind: "when",
      });
    });

    it("reads the same with output before the document", () => {
      const stdout = JSON.stringify(recorded.document);
      expect(parseDiscoverOutput(`go: downloading x\n${stdout}`)).toEqual(
        parseDiscoverOutput(stdout),
      );
    });

    it("fills the cache's module and file lookups", () => {
      const cache = discoveredCache();
      expect(cache.getModuleDir(repoModule)).toBe("<repo>");
      expect(cache.getModuleDir(`${repoModule}/examples`)).toBe(
        "<repo>/examples",
      );
      expect(
        cache.resolveFileToPackage("<repo>/examples/search/suite_test.go"),
      ).toBe(searchPkg);
    });
  });

  describe("bench --json", () => {
    const recorded = golden.outputs.bench;

    it("succeeds, noting on stderr only that the census was skipped", () => {
      expect(recorded.exitCode).toBe(0);
      expect(recorded.stderr).toBe(
        "note: census skipped under -run/-skip/-list/-bench\n",
      );
    });

    it("parses into one result per benchmark, one sample each", () => {
      const report = parseBenchReport(JSON.stringify(recorded.document));
      expect(
        report.baseline.results.map((r) => `${r.suite}.${r.name}`),
      ).toEqual([
        "BenchPlainTestSuite.BenchmarkA",
        "BenchPlainTestSuite.BenchmarkB",
      ]);
      for (const r of report.baseline.results) {
        expect(r.package).toBe(`${census}benchplain`);
        expect(r.samples.map((x) => x.iterations)).toEqual([1]);
      }
    });

    it("carries no comparison without a baseline to compare against", () => {
      const report = parseBenchReport(JSON.stringify(recorded.document));
      expect(report.deltas ?? []).toEqual([]);
      expect(report.gate).toBeUndefined();
      expect(report.failures ?? []).toEqual([]);
    });
  });

  describe("a run's merged cover profile", () => {
    const recorded = golden.outputs.cover;

    it("succeeds and writes the profile it was asked for", () => {
      expect(recorded.exitCode).toBe(0);
      expect(recorded.stderr).toBe("");
      expect(recorded.profile?.[0]).toBe("mode: atomic");
    });

    it("maps every block onto the file discovery placed", () => {
      const cache = discoveredCache();
      const files = parseCoverProfile(recorded.profile!.join("\n"), (p) =>
        cache.resolveImportPath(p),
      );
      expect(files.map((f) => f.absPath)).toEqual([
        path.join("<repo>/examples/search", "index.go"),
      ]);
      expect(files[0].statements).toHaveLength(recorded.profile!.length - 1);
    });

    it("reads a block no test reached as not executed", () => {
      const cache = discoveredCache();
      const [file] = parseCoverProfile(recorded.profile!.join("\n"), (p) =>
        cache.resolveImportPath(p),
      );
      const executed = file.statements.map((s) => s.executed);
      expect(executed).toContain(false);
      expect(executed.some((e) => typeof e === "number" && e > 0)).toBe(true);
    });
  });

  describe("fuzz promote", () => {
    it("with no session, succeeds and promotes nothing", () => {
      const recorded = golden.outputs["promote-nothing"];
      expect(recorded.exitCode).toBe(0);
      expect(recorded.stderr).toBe("");
      expect(promotedSeeds(recorded.stdout!)).toEqual([]);
    });

    describe("a crasher the session wrote, beside a file it did not", () => {
      const recorded = golden.outputs["promote-staged"];

      it("succeeds and confirms exactly one seed, by file and line", () => {
        expect(recorded.exitCode).toBe(0);
        expect(recorded.stderr).toBe("");
        const seeds = promotedSeeds(recorded.stdout!);
        expect(seeds).toHaveLength(1);
        expect(seeds[0].file).toBe("suite_test.go");
        expect(seeds[0].line).toBeGreaterThan(0);
      });

      it("names the line that holds the spliced seed", () => {
        expect(recorded.revealedLine).toBe('f.Add("stale")');
      });

      it("leaves the other file in place, saying so", () => {
        expect(recorded.stdout).toContain(
          "promote: testdata/fuzz/FuzzMessageTestSuite_FuzzTrim/planted: left in place: not written by the latest fuzz session\n",
        );
      });
    });

    it("refuses a crasher changed since the session, promoting nothing", () => {
      const recorded = golden.outputs["promote-refused"];
      expect(recorded.exitCode).toBe(1);
      expect(recorded.stderr).toBe("");
      expect(recorded.stdout).toContain(
        "stale-seed: changed since the fuzz session wrote it\n",
      );
      expect(promotedSeeds(recorded.stdout!)).toEqual([]);
    });
  });
});

describe("recorded lint runs", () => {
  it("records exactly the stated runs", () => {
    expect(Object.keys(golden.lint).sort()).toEqual([
      "findings",
      "rules",
      "uncompilable",
    ]);
  });

  describe("packages with findings", () => {
    const recorded = golden.lint.findings;
    const findings = parseLintJson(JSON.stringify(recorded.document));

    // -json reports findings on stdout and still exits 0; the extension
    // reads exit 0 as "analyzed", not as "clean".
    it("exits 0 with the findings in the document and nothing on stderr", () => {
      expect(recorded.exitCode).toBe(0);
      expect(recorded.stderr).toBe("");
      expect(findings.length).toBeGreaterThan(0);
    });

    it("keys packages by their test variant, as the go/analysis driver does", () => {
      expect(Object.keys(recorded.document!)).toContain(
        "gotest.fixtures/stdlibtests [gotest.fixtures/stdlibtests.test]",
      );
    });

    it("names the rule, the file:line:col and the message of every finding", () => {
      for (const f of findings) {
        expect(f.category).toMatch(/^[a-z-]+$/);
        expect(f.file).toMatch(/^<fixtures>\/.+_test\.go$/);
        expect(f.line).toBeGreaterThan(0);
        expect(f.col).toBeGreaterThan(0);
        expect(f.message.length).toBeGreaterThan(0);
      }
    });

    it("ends every finding past where it starts", () => {
      for (const f of findings) {
        expect([f.endLine, f.endCol]).not.toEqual([f.line, f.col]);
        expect(
          f.endLine > f.line || (f.endLine === f.line && f.endCol > f.col),
        ).toBe(true);
      }
    });

    it("carries fixes as byte-offset edits of named files", () => {
      const edits = findings.flatMap((f) => f.fixes.flatMap((x) => x.edits));
      expect(edits.length).toBeGreaterThan(0);
      for (const e of edits) {
        expect(e.filename).toMatch(/^<fixtures>\//);
        expect(Number.isInteger(e.start)).toBe(true);
        expect(e.end).toBeGreaterThan(e.start);
        expect(typeof e.new).toBe("string");
      }
    });
  });

  describe("a package that does not compile", () => {
    const recorded = golden.lint.uncompilable;
    it("exits 1 with no document, saying why on stderr", () => {
      expect(recorded.exitCode).toBe(1);
      expect(recorded.document).toBeNull();
      expect(recorded.stderr).toContain("cannot lint uncompilable packages");
    });
  });

  describe("the rule registry", () => {
    const recorded = golden.lint.rules;
    const doc = recorded.document as unknown as {
      version: string;
      rules: LintRule[];
    };

    it("exits 0 with one document and nothing on stderr", () => {
      expect(recorded.exitCode).toBe(0);
      expect(recorded.stderr).toBe("");
      expect(Object.keys(doc)).toEqual(["version", "rules"]);
      expect(doc.version.length).toBeGreaterThan(0);
      expect(doc.rules.length).toBeGreaterThan(0);
    });

    it("describes every rule by the same fields", () => {
      for (const r of doc.rules) {
        expect(Object.keys(r)).toEqual([
          "id",
          "tier",
          "scope",
          "skippable",
          "fixable",
          "description",
        ]);
        expect(r.id).toMatch(/^[a-z-]+$/);
        expect(typeof r.scope).toBe("string");
        expect(typeof r.fixable).toBe("boolean");
        expect(r.description.length).toBeGreaterThan(0);
      }
    });

    it("names each rule once", () => {
      const ids = doc.rules.map((r) => r.id);
      expect(new Set(ids).size).toBe(ids.length);
    });

    it("lets only a rule outside the integrity tier be skipped", () => {
      for (const r of doc.rules) {
        expect(["integrity", "expressiveness", "migration"]).toContain(r.tier);
        expect(r.skippable).toBe(r.tier !== "integrity");
      }
    });

    it("lists every rule the findings name", () => {
      const ids = new Set(doc.rules.map((r) => r.id));
      const findings = parseLintJson(
        JSON.stringify(golden.lint.findings.document),
      );
      for (const f of findings) expect(ids).toContain(f.category);
    });
  });
});
