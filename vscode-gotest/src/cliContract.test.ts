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

interface ContractCase {
  gating: Variant;
  renderOnly: Variant;
}

const golden = JSON.parse(
  readFileSync(
    path.join(__dirname, "..", "testdata", "cli-contract.json"),
    "utf-8",
  ),
) as { cases: Record<string, ContractCase> };

// What each stream shape means. Stated here rather than derived from the
// recording, so that re-recording a changed CLI cannot quietly relabel a case:
// the golden supplies the bytes, this table supplies the intent, and a genuine
// behaviour change has to break one against the other.
const EXPECTED: Record<
  string,
  {
    gatingExitCode: number;
    goRunEpilogue: string;
    stats: Record<string, number>;
    packages: string[];
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
  empty: {
    gatingExitCode: 0,
    goRunEpilogue: "",
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

      it("keeps stderr clean when invoked directly", () => {
        expect(recorded.gating.direct.stderr).toBe("");
      });

      it("gains only go run's exit-status epilogue on stderr, never a changed spec", () => {
        expect(recorded.gating.goRun.exitCode).toBe(expected.gatingExitCode);
        expect(recorded.gating.goRun.stderr).toBe(expected.goRunEpilogue);
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

      it("is read as a usable spec when invoked directly", () => {
        expect(
          interpretSpecExit(
            recorded.gating.direct.exitCode,
            recorded.gating.direct.stdout,
            recorded.gating.direct.stderr,
          ),
        ).toEqual({ ok: true, stdout: recorded.gating.direct.stdout });
      });

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
