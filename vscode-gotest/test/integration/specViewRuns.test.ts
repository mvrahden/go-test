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

// render is the panel's call: the composed stream laid over what the
// fixture declares, so a declared row the stream misses shows as not run.
function render(composed: string): Node[] {
  const out = gotest(
    [
      "spec",
      "--input=-",
      "--format=json",
      "--render-only",
      "--hide=",
      "--declared",
      "./twins/",
    ],
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
    expect(twinRows).toEqual(
      [
        "TestTwinsTestSuite pass",
        "TestTwinsTestSuite/TestCreate pass",
        "TestTwinsTestSuite/TestCreate/creates_inside pass",
        "TestTwinsTestSuite/TestRead pass",
        "TestTwinsTestSuite/TestRead/reads_inside pass",
        "ext:TestTwinsTestSuite pass",
        "ext:TestTwinsTestSuite/TestCreate pass",
        "ext:TestTwinsTestSuite/TestCreate/creates_outside pass",
        "ext:TestTwinsTestSuite/TestList pass",
        "ext:TestTwinsTestSuite/TestList/lists_outside pass",
      ].sort(),
    );
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

// copy is the panel's Copy: the Markdown the CLI writes for the same stream,
// with the flags the panel adds for a search or a node.
function copy(composed: string, ...extra: string[]): string {
  return gotest(
    [
      "spec",
      "--input=-",
      "--format=md",
      "--render-only",
      "--hide=",
      ...extra,
      "--declared",
      "./twins/",
    ],
    composed,
  );
}

describe("the Spec View's Copy, rendered by the CLI", () => {
  it("copying after typing a search holds only the matching behaviors", () => {
    const whole = run();
    const all = copy(whole);
    const found = copy(whole, "--match=outside");
    expect(all).toContain("creates inside");
    expect(found).toContain("creates outside");
    expect(found).toContain("lists outside");
    expect(found).not.toContain("creates inside");
    expect(found).not.toContain("./pkg");
    expect(found).toContain('search "outside"');
  });

  it("copying one row holds that node alone, placed as the panel's JSON places it", () => {
    const whole = run();
    const at = render(whole).findIndex(
      (n) => n.name === "TestPathsTestSuite" && !n.external,
    );
    expect(at).toBeGreaterThanOrEqual(0);
    const one = copy(whole, `--only=${twins}:${at}`);
    expect(one).toContain("./pkg");
    expect(one).not.toContain("creates inside");
  });
});
