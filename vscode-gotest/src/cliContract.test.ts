// Tier 0 — the recorded CLI contract.
//
// These assertions run against bytes the real `gotest spec --input=-` actually
// produced (see scripts/record-cli-contract.mjs), not bytes we imagined. The
// regression this guards shipped because the CLI's exit rule and the
// extension's reading of it were each tested in isolation and never together.
//
// Two variants are recorded per stream: the gating form, whose exit code CI
// depends on, and the --render-only form the Spec View sends.

import { describe, it, expect, vi } from "vitest";
import { readFileSync } from "node:fs";
import * as path from "node:path";

vi.mock("vscode", () => ({
  Uri: {
    joinPath: (...args: string[]) => ({ toString: () => args.join("/") }),
  },
  workspace: { getConfiguration: () => ({ get: () => true }) },
  window: {},
  ViewColumn: { Beside: 2 },
  commands: {},
}));

import { interpretSpecExit } from "./specView.js";
import { parseTestEvents } from "./outputParser.js";

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
}

// A live `gotest -json` run, times and durations masked as "<d>".
interface LiveRun {
  args: string[];
  exitCode: number | null;
  stderr: string;
  events: LiveEvent[];
}

interface ContractCase {
  gating: Variant;
  renderOnly: Variant;
}

const golden = JSON.parse(
  readFileSync(
    path.join(__dirname, "..", "testdata", "cli-contract.json"),
    "utf-8",
  ),
) as { cases: Record<string, ContractCase>; live: Record<string, LiveRun> };

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

const unrunPkg = "github.com/mvrahden/go-test/cmd/gotest/testdata/unrun/";

describe("recorded live runs", () => {
  it("records exactly the stated runs", () => {
    expect(Object.keys(golden.live).sort()).toEqual([
      "no-packages",
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
  });
});
