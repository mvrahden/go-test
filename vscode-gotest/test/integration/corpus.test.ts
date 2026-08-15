// The full pipeline, end to end.
//
// Every other test in this repo feeds the Spec View a stream someone wrote by
// hand. This one runs a corpus of real Go suites — clean and adverse — through
// the real CLI, captures stdout with the extension's own capture function, and
// renders it through the real Spec View. Nothing about the stream is assumed;
// it is produced.
//
// The corpus lives in testdata/fixtures and deliberately contains suites that
// fail, panic, abort, emit megabytes, carry non-ASCII, forge event lines, and
// fail to compile. A `testdata` directory is invisible to `go ./...` patterns,
// so none of it can reach the repository's own build.

import { describe, it, expect, beforeAll, vi } from "vitest";
import { readFileSync, writeFileSync, mkdtempSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

const state = vi.hoisted(() => ({
  workspaceDir: "",
  cliPath: "",
  autoRefresh: true,
  panels: [] as { webview: { html: string; cspSource: string } }[],
}));

vi.mock("vscode", async () => {
  const { buildVscodeStub } = await import("./vscodeStub.js");
  return buildVscodeStub(state as never);
});

import { SpecViewPanel } from "../../src/specView.js";
import { spawnTestProcess } from "../../src/runnerUtils.js";
import { createRecordingChannel } from "./vscodeStub.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const fixturesDir = path.join(extensionDir, "testdata", "fixtures");

interface Expectation {
  status: "pass" | "fail";
  passed: number;
  failed: number;
  skipped: number;
  mustContain?: string[];
  mustNotContain?: string[];
}

// One entry per fixture package. Counts are leaf behaviors by status, so the
// suite totals below are derived rather than restated — adding a fixture
// updates the totals by construction, and forgetting to declare one is caught
// by the completeness guard.
const FIXTURES: Record<string, Expectation> = {
  passing: { status: "pass", passed: 2, failed: 0, skipped: 0 },
  nested: {
    status: "pass",
    passed: 3,
    failed: 0,
    skipped: 0,
    mustContain: ["stacks both discounts", "and a coupon applies"],
  },
  table: {
    status: "pass",
    passed: 3,
    failed: 0,
    skipped: 0,
    mustContain: ["negative", "zero", "positive"],
  },
  skipping: {
    status: "pass",
    passed: 1,
    failed: 0,
    skipped: 1,
    mustContain: ["SKIPPED"],
  },
  lifecycle: { status: "pass", passed: 2, failed: 0, skipped: 0 },
  parallel: { status: "pass", passed: 3, failed: 0, skipped: 0 },
  failing: {
    status: "fail",
    passed: 1,
    failed: 1,
    skipped: 0,
    mustContain: ["still runs the sibling behavior"],
  },
  // A panic unwinds the enclosing When, so the sibling behavior below it never
  // reports at all — one leaf, not two. Contrast with `fatal`.
  panicking: {
    status: "fail",
    passed: 0,
    failed: 1,
    skipped: 0,
    mustContain: ["deliberate panic from fixture"],
  },
  // The panic happened in BeforeEach, so the failure lands on the test method
  // rather than on any behavior beneath it.
  panichook: {
    status: "fail",
    passed: 0,
    failed: 1,
    skipped: 0,
    mustContain: ["deliberate panic in BeforeEach"],
  },
  // FailNow aborts only its own behavior: unlike a panic, the sibling still
  // runs and still reports.
  fatal: {
    status: "fail",
    passed: 1,
    failed: 1,
    skipped: 0,
    mustContain: ["does not stop the sibling behavior"],
  },
  htmlish: {
    status: "fail",
    passed: 0,
    failed: 2,
    skipped: 0,
    // Markup from test output must arrive escaped, as text.
    mustContain: ["&lt;script&gt;"],
    mustNotContain: ["<script>alert", "<img src=x onerror"],
  },
  jsonish: { status: "pass", passed: 2, failed: 0, skipped: 0 },
  // Behaviors guarded by a condition: readable only by running. The walker
  // reports the method incomplete rather than passing off what it can see as
  // the whole specification.
  runtimebehaviors: { status: "pass", passed: 2, failed: 0, skipped: 0 },
  // Descriptions carrying every regex metacharacter in Go's QuoteMeta set, so
  // that addressing one exercises the -run escaping rather than a happy-path
  // name. The slash case is included because -run splits on "/" before
  // compiling, which escaping cannot influence either way.
  metachars: {
    status: "pass",
    passed: 6,
    failed: 0,
    skipped: 0,
    mustContain: ["a description has {braces}", "handles https:// URIs"],
  },
  // Descriptions that repeat among their siblings, and descriptions carrying a
  // single slash. go test numbers the one and nests the other, and discovery
  // has to predict both or the declared item and the observed one are two.
  duplicates: {
    status: "pass",
    passed: 5,
    failed: 0,
    skipped: 0,
    mustContain: ["names the first", "names the third", "shares it too"],
  },
  bigoutput: { status: "pass", passed: 2, failed: 0, skipped: 0 },
  unicode: {
    status: "fail",
    passed: 3,
    failed: 1,
    skipped: 0,
    mustContain: ["café naïve", "日本語のテスト", "🧪"],
  },
  // A gotest suite and plain stdlib tests in one package. Only the suite is
  // reported: gotest runs the generated suite entrypoints, so the plain
  // `func TestX(t *testing.T)` tests never execute at all.
  stdlibtests: { status: "pass", passed: 1, failed: 0, skipped: 0 },
  // Fails to compile. Carries a package-level verdict and no behaviors, and
  // must not stop any other package from reporting.
  broken: {
    status: "fail",
    passed: 0,
    failed: 0,
    skipped: 0,
    mustContain: ["undefinedHelper"],
  },
};

interface SpecNode {
  display: string;
  status: string;
  children: SpecNode[];
}
interface SpecPackage {
  path: string;
  status: string;
  nodes: SpecNode[];
}

let capturedStdout = "";
let streamLines: string[] = [];
let spec: { packages: SpecPackage[]; stats: Record<string, number> };
let html = "";
let renderErrors: string[] = [];

function leavesOf(node: SpecNode, acc: SpecNode[] = []): SpecNode[] {
  if (!node.children || node.children.length === 0) {
    acc.push(node);
    return acc;
  }
  for (const child of node.children) leavesOf(child, acc);
  return acc;
}

function packageNamed(name: string): SpecPackage | undefined {
  return spec.packages.find((p) => p.path === `gotest.fixtures/${name}`);
}

beforeAll(async () => {
  // The fixture module is not part of the repository's workspace — that is the
  // point. A generated go.work joins it to the repo so the suites compile
  // against this working tree's gotest rather than a published release.
  const tmp = mkdtempSync(path.join(tmpdir(), "gotest-corpus-"));
  const workFile = path.join(tmp, "corpus.work");
  writeFileSync(
    workFile,
    `go 1.24.0\n\nuse (\n\t${repoRoot}\n\t${fixturesDir}\n)\n`,
    "utf-8",
  );

  const recorder = createRecordingChannel();
  const token = {
    isCancellationRequested: false,
    onCancellationRequested: () => ({ dispose: () => {} }),
  };

  // Captured through the extension's own stdout handling, so the line
  // reassembly that a 200KB single-line event depends on runs for real.
  const result = await spawnTestProcess(
    "go",
    [
      "run",
      "github.com/mvrahden/go-test/cmd/gotest",
      "--",
      "-json",
      "-count=1",
      "./...",
    ],
    fixturesDir,
    token as never,
    recorder.channel as never,
    "corpus",
    { GOWORK: workFile },
    (line) => streamLines.push(line),
  );
  capturedStdout = result.stdout;

  state.workspaceDir = repoRoot;
  state.cliPath = "";
  state.autoRefresh = true;
  state.panels.length = 0;

  const panelRecorder = createRecordingChannel();
  renderErrors = panelRecorder.errors;
  const panel = new SpecViewPanel(panelRecorder.channel as never);
  await panel.show();
  await panel.refresh(capturedStdout, "run");
  html = state.panels[state.panels.length - 1].webview.html;

  spec = JSON.parse(
    /const SPEC_STATE = (.*);\n/.exec(html)?.[1] ?? "null",
  ) as typeof spec;
});

describe("the corpus reaches the spec view", () => {
  it("produced a stream and rendered it without reporting a problem", () => {
    expect(capturedStdout.length).toBeGreaterThan(100_000);
    expect(renderErrors).toEqual([]);
    expect(spec).not.toBeNull();
  });

  it("declares an expectation for every fixture package, and finds every declared one", () => {
    const onDisk = readdirSync(fixturesDir, { withFileTypes: true })
      .filter((e) => e.isDirectory())
      .map((e) => e.name)
      .sort();
    expect(onDisk).toEqual(Object.keys(FIXTURES).sort());

    const rendered = spec.packages
      .map((p) => p.path.replace("gotest.fixtures/", ""))
      .sort();
    expect(rendered).toEqual(Object.keys(FIXTURES).sort());
  });

  it("totals match the sum of the per-package expectations", () => {
    const sum = (key: keyof Expectation) =>
      Object.values(FIXTURES).reduce((n, f) => n + (f[key] as number), 0);
    expect(spec.stats.passed).toBe(sum("passed"));
    expect(spec.stats.failed).toBe(sum("failed"));
    expect(spec.stats.skipped).toBe(sum("skipped"));
    expect(spec.stats.failedPackages).toBe(
      Object.values(FIXTURES).filter(
        (f) => f.status === "fail" && f.failed === 0,
      ).length,
    );
  });
});

describe.each(Object.entries(FIXTURES))("fixture %s", (name, expected) => {
  it("reports the expected package verdict", () => {
    expect(packageNamed(name)?.status).toBe(expected.status);
  });

  it("reports the expected behaviors by status", () => {
    const pkg = packageNamed(name)!;
    const leaves = pkg.nodes.flatMap((n) => leavesOf(n));
    const counts = { pass: 0, fail: 0, skip: 0 } as Record<string, number>;
    for (const leaf of leaves)
      counts[leaf.status] = (counts[leaf.status] ?? 0) + 1;
    expect(counts.pass).toBe(expected.passed);
    expect(counts.fail).toBe(expected.failed);
    expect(counts.skip).toBe(expected.skipped);
  });

  it("renders the text this fixture exists to produce", () => {
    for (const needle of expected.mustContain ?? []) {
      expect(html).toContain(needle);
    }
    for (const needle of expected.mustNotContain ?? []) {
      expect(html).not.toContain(needle);
    }
  });
});

describe("adverse inputs cannot corrupt the pipeline", () => {
  it("carries every captured line as a well-formed test event", () => {
    // What executeBatch parses line by line. A single truncated line here would
    // mean the extension silently dropped a verdict.
    expect(streamLines.length).toBeGreaterThan(1_000);
    for (const line of streamLines) {
      const event = JSON.parse(line) as { Action?: string };
      expect(event.Action).toBeTruthy();
    }
  });

  it("reassembles lines across many pipe reads without losing one", () => {
    // `go test -json` chunks test output into events of about a kilobyte, so
    // no single event spans a read. What does span reads is the stream itself:
    // ~900KB arrives in 64KB chunks, and every line above parsed, which is
    // only possible if the remainder of each chunk was carried forward.
    expect(capturedStdout.length).toBeGreaterThan(500_000);
    expect(streamLines.join("\n").length).toBeGreaterThan(500_000);
  });

  it("carries a 200KB payload through the pipeline intact", () => {
    const pkg = packageNamed("bigoutput")!;
    let chars = 0;
    const walk = (n: SpecNode) => {
      chars += ((n as unknown as { output?: string[] }).output ?? []).join(
        "",
      ).length;
      (n.children ?? []).forEach(walk);
    };
    pkg.nodes.forEach(walk);
    expect(chars).toBeGreaterThan(190_000);
  });

  it("treats a forged event line as output, never as a verdict", () => {
    // The jsonish fixture prints lines shaped like `pass` and `fail` events.
    // They must arrive inside another event's Output field, not as packages.
    expect(spec.packages.map((p) => p.path)).not.toContain(
      "gotest.fixtures/forged",
    );
    expect(packageNamed("passing")?.status).toBe("pass");
    expect(capturedStdout).toContain("gotest.fixtures/forged");
  });

  it("escapes markup from test output instead of rendering it", () => {
    expect(html).toContain("&lt;script&gt;");
    expect(html).not.toContain("<script>alert");
  });

  it("keeps one uncompilable package from blocking the rest", () => {
    expect(packageNamed("broken")?.status).toBe("fail");
    expect(packageNamed("broken")?.nodes).toHaveLength(0);
    const others = spec.packages.filter(
      (p) => p.path !== "gotest.fixtures/broken",
    );
    expect(others).toHaveLength(Object.keys(FIXTURES).length - 1);
    for (const pkg of others) {
      expect(["pass", "fail"]).toContain(pkg.status);
    }
  });
});
