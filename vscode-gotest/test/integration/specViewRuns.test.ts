// The Spec View's kept runs, rendered by the real CLI: a whole run of the
// twins fixture, then a rerun of part of it, merged as the panel merges
// them. Every row that ran must render with its verdict.

import { describe, it, expect, beforeAll, afterAll } from "vitest";
import { spawnSync } from "node:child_process";
import { writeFileSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";
import { SpecStore, coverOf } from "../../src/specStore.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const fixturesDir = path.join(extensionDir, "testdata", "fixtures");
const twins = "gotest.fixtures/twins";

interface Node {
  name: string;
  status: string;
  external: boolean;
  children?: Node[];
}

let savedGoWork: string | undefined;
let tmp = "";

function gotest(args: string[], input?: string): string {
  const res = spawnSync(
    "go",
    ["run", "github.com/mvrahden/go-test/cmd/gotest", ...args],
    { cwd: fixturesDir, encoding: "utf-8", input },
  );
  expect(res.error).toBeUndefined();
  return res.stdout;
}

const run = (...filter: string[]) => gotest(["-json", ...filter, "./twins/"]);

// render is the panel's call: the composed stream, rendered as it ran.
function render(composed: string): Node[] {
  const out = gotest(
    ["spec", "--input=-", "--format=json", "--render-only"],
    composed,
  );
  const doc = JSON.parse(out) as {
    packages: { path: string; nodes: Node[] }[];
  };
  return doc.packages.find((p) => p.path === twins)?.nodes ?? [];
}

// rows lists every row as its path from the suite down, with its verdict;
// an external suite is marked so its rows stay apart from its twin's.
function rows(nodes: Node[]): string[] {
  const out: string[] = [];
  const walk = (ns: Node[], prefix: string) => {
    for (const n of ns) {
      const key = prefix + n.name;
      out.push(`${key} ${n.status}`);
      walk(n.children ?? [], key + "/");
    }
  };
  for (const suite of nodes) {
    walk([suite], suite.external ? "ext:" : "");
  }
  return out.sort();
}

beforeAll(() => {
  tmp = mkdtempSync(path.join(tmpdir(), "gotest-specruns-"));
  const workFile = path.join(tmp, "corpus.work");
  writeFileSync(
    workFile,
    `go 1.26.0\n\nuse (\n\t${repoRoot}\n\t${fixturesDir}\n)\n`,
    "utf-8",
  );
  savedGoWork = process.env.GOWORK;
  process.env.GOWORK = workFile;
}, 300_000);

afterAll(() => {
  if (savedGoWork === undefined) delete process.env.GOWORK;
  else process.env.GOWORK = savedGoWork;
  if (tmp) rmSync(tmp, { recursive: true, force: true });
});

describe("the Spec View's kept runs, rendered by the CLI", () => {
  it("renders a whole run as it ran, stamped or as plain go test wrote it", () => {
    const whole = run();
    // A plain go test -json file carries no stamps: only its order places it.
    const plain = whole
      .split("\n")
      .filter(Boolean)
      .map((l) => {
        const ev = JSON.parse(l) as Record<string, unknown>;
        delete ev.Gotest;
        return JSON.stringify(ev);
      })
      .join("\n");
    for (const written of [whole, plain]) {
      const store = new SpecStore();
      store.merge({ stream: written, folder: fixturesDir });
      const composed = rows(render(store.compose(fixturesDir)));
      expect(composed).toEqual(rows(render(written)));
      expect(composed).toContain("TestPathsTestSuite/TestResolve/./pkg pass");
    }
  });

  it("rerunning one twin method keeps every other method of both twins", () => {
    const store = new SpecStore();
    store.merge({ stream: run(), folder: fixturesDir });
    store.merge({
      stream: run("-run", "^TestTwinsTestSuite$/^TestCreate#01$"),
      folder: fixturesDir,
      partial: new Map([
        [twins, coverOf(["TestTwinsTestSuite/TestCreate#01"])],
      ]),
    });

    const twinRows = rows(render(store.compose(fixturesDir))).filter((r) =>
      r.includes("TestTwinsTestSuite"),
    );
    // Every row of both twins ran, and kept its verdict.
    expect(twinRows.filter((r) => !r.endsWith(" pass"))).toEqual([]);
    expect(twinRows).toEqual(
      expect.arrayContaining([
        "TestTwinsTestSuite pass",
        "TestTwinsTestSuite/TestCreate/creates_inside pass",
        "TestTwinsTestSuite/TestRead/reads_inside pass",
        "ext:TestTwinsTestSuite pass",
        "ext:TestTwinsTestSuite/TestList/lists_outside pass",
      ]),
    );
    expect(twinRows.filter((r) => r.endsWith("/creates_outside pass"))).toEqual(
      [expect.stringMatching(/^ext:TestTwinsTestSuite\/TestCreate/)],
    );
    expect(twinRows.filter((r) => r.endsWith("/creates_inside pass"))).toEqual([
      "TestTwinsTestSuite/TestCreate/creates_inside pass",
    ]);
  });

  it("rerunning . keeps ./pkg", () => {
    const store = new SpecStore();
    store.merge({ stream: run(), folder: fixturesDir });
    store.merge({
      stream: run("-run", "^TestPathsTestSuite$/^TestResolve$/^\\.$"),
      folder: fixturesDir,
      partial: new Map([
        [twins, coverOf(["TestPathsTestSuite/TestResolve/."])],
      ]),
    });

    const pathRows = rows(render(store.compose(fixturesDir))).filter((r) =>
      r.startsWith("TestPathsTestSuite"),
    );
    expect(pathRows).toEqual([
      "TestPathsTestSuite pass",
      "TestPathsTestSuite/TestResolve pass",
      "TestPathsTestSuite/TestResolve/. pass",
      "TestPathsTestSuite/TestResolve/./pkg pass",
    ]);
  });
});
