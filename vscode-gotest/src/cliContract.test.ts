// Tier 0 — the recorded CLI contract.
//
// These assertions run against bytes the real `gotest spec --input=-` actually
// produced (see scripts/record-cli-contract.mjs), not bytes we imagined. The
// regression this guards shipped because the CLI's exit rule and the
// extension's reading of it were each tested in isolation and never together.

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

interface ContractCase {
  direct: Invocation;
  goRun: {
    exitCode: number | null;
    stderr: string;
    stdoutMatchesDirect: boolean;
  };
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
    exitCode: number;
    goRunEpilogue: string;
    stats: Record<string, number>;
    packages: string[];
  }
> = {
  "all-pass": {
    exitCode: 0,
    goRunEpilogue: "",
    stats: { passed: 2, failed: 0, skipped: 0 },
    packages: ["example.com/cart:pass"],
  },
  mixed: {
    exitCode: 1,
    goRunEpilogue: "exit status 1\n",
    stats: { passed: 1, failed: 1, skipped: 0 },
    packages: ["example.com/cart:fail"],
  },
  "all-fail": {
    exitCode: 1,
    goRunEpilogue: "exit status 1\n",
    stats: { passed: 0, failed: 1, skipped: 0 },
    packages: ["example.com/cart:fail"],
  },
  "skip-only": {
    exitCode: 0,
    goRunEpilogue: "",
    stats: { passed: 0, failed: 0, skipped: 1 },
    packages: ["example.com/cart:pass"],
  },
  // A package that fails to build carries no failing behaviour at all. Only the
  // exit code and failedPackages say anything is wrong, which is exactly why a
  // consumer must not treat a non-zero exit as "the renderer broke".
  "package-failure": {
    exitCode: 1,
    goRunEpilogue: "exit status 1\n",
    stats: { passed: 0, failed: 0, skipped: 0, failedPackages: 1 },
    packages: ["example.com/broken:fail"],
  },
  empty: {
    exitCode: 0,
    goRunEpilogue: "",
    stats: { passed: 0, failed: 0, skipped: 0 },
    packages: [],
  },
  // A second package, used by the layer-accumulation tests: concatenating it
  // with a passing layer must still render both.
  "other-package-fail": {
    exitCode: 1,
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

      it("exits with the documented code", () => {
        expect(recorded.direct.exitCode).toBe(expected.exitCode);
      });

      it("renders the spec on stdout regardless of that code", () => {
        const spec = JSON.parse(recorded.direct.stdout);
        expect(spec).toHaveProperty("packages");
        expect(
          spec.packages.map(
            (p: { path: string; status: string }) => `${p.path}:${p.status}`,
          ),
        ).toEqual(expected.packages);
        expect(spec.stats).toMatchObject(expected.stats);
      });

      it("keeps stderr clean when invoked directly", () => {
        expect(recorded.direct.stderr).toBe("");
      });

      it("gains only go run's exit-status epilogue on stderr, never a changed spec", () => {
        expect(recorded.goRun.exitCode).toBe(expected.exitCode);
        expect(recorded.goRun.stderr).toBe(expected.goRunEpilogue);
        expect(recorded.goRun.stdoutMatchesDirect).toBe(true);
      });

      it("is read as a usable spec when invoked directly", () => {
        expect(
          interpretSpecExit(
            recorded.direct.exitCode,
            recorded.direct.stdout,
            recorded.direct.stderr,
          ),
        ).toEqual({ ok: true, stdout: recorded.direct.stdout });
      });

      // The regression in one assertion: same bytes, same verdict, but reached
      // through the extension's default `go run` resolution.
      it("is read as a usable spec when resolved through go run", () => {
        expect(
          interpretSpecExit(
            recorded.goRun.exitCode,
            recorded.direct.stdout,
            recorded.goRun.stderr,
          ),
        ).toEqual({ ok: true, stdout: recorded.direct.stdout });
      });
    });
  }

  it("covers both a failing and a passing verdict, so a green-only fixture set cannot pass", () => {
    const codes = Object.values(EXPECTED).map((e) => e.exitCode);
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
});
