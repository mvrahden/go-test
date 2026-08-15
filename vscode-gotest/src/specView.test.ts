import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  Uri: {
    joinPath: (...args: string[]) => ({ toString: () => args.join("/") }),
  },
  workspace: {
    getConfiguration: () => ({ get: () => true }),
  },
  window: {},
  ViewColumn: { Beside: 2 },
  commands: {},
}));

import {
  specDataToReport,
  interpretSpecExit,
  encodeStateForScript,
} from "./specView.js";

function leaf(
  name: string,
  status: string,
  duration = 0,
  output: string[] = [],
): {
  name: string;
  display: string;
  kind: string;
  status: string;
  duration: number;
  focused: boolean;
  excluded: boolean;
  external: boolean;
  output: string[];
  children: never[];
} {
  return {
    name,
    display: name,
    kind: "behavior",
    status,
    duration,
    focused: false,
    excluded: false,
    external: false,
    output,
    children: [],
  };
}

function suite(
  name: string,
  children: ReturnType<typeof leaf>[],
): {
  name: string;
  display: string;
  kind: string;
  status: string;
  duration: number;
  focused: boolean;
  excluded: boolean;
  external: boolean;
  output: string[];
  children: ReturnType<typeof leaf>[];
} {
  return {
    name,
    display: name,
    kind: "suite",
    status: "pass",
    duration: 0,
    focused: false,
    excluded: false,
    external: false,
    output: [],
    children,
  };
}

describe("specDataToReport", () => {
  const data = {
    packages: [
      {
        path: "example.com/pkg",
        status: "fail",
        duration: 1.5,
        nodes: [
          suite("MySuite", [
            leaf("passes", "pass", 0.5),
            leaf("fails", "fail", 0.8),
            leaf("skipped", "skip", 0),
          ]),
        ],
      },
    ],
    stats: {
      suites: 1,
      behaviors: 3,
      tests: 0,
      passed: 1,
      failed: 1,
      skipped: 1,
    },
  };

  it("includes all statuses when no filter is set", () => {
    const report = specDataToReport(data, []);
    expect(report).toContain("passes");
    expect(report).toContain("fails");
    expect(report).toContain("skipped");
    expect(report).toContain("1 passed");
    expect(report).toContain("1 failed");
    expect(report).toContain("1 skipped");
  });

  it("excludes passed leaves when pass is hidden", () => {
    const report = specDataToReport(data, [], new Set(["pass"]));
    expect(report).not.toContain("passes");
    expect(report).toContain("fails");
    expect(report).toContain("skipped");
    expect(report).not.toContain("1 passed");
    expect(report).toContain("1 failed");
    expect(report).toContain("1 skipped");
  });

  it("excludes failed leaves when fail is hidden", () => {
    const report = specDataToReport(data, [], new Set(["fail"]));
    expect(report).toContain("passes");
    expect(report).not.toContain("  fails");
    expect(report).toContain("skipped");
    expect(report).toContain("1 passed");
    expect(report).not.toContain("1 failed");
    expect(report).toContain("1 skipped");
  });

  it("excludes skipped leaves when skip is hidden", () => {
    const report = specDataToReport(data, [], new Set(["skip"]));
    expect(report).toContain("passes");
    expect(report).toContain("fails");
    expect(report).not.toMatch(/\bskipped\b/);
    expect(report).toContain("1 passed");
    expect(report).toContain("1 failed");
  });

  it("hides multiple statuses at once", () => {
    const report = specDataToReport(data, [], new Set(["pass", "skip"]));
    expect(report).not.toContain("passes");
    expect(report).toContain("fails");
    expect(report).not.toMatch(/\bskipped\b/);
    expect(report).toContain("1 failed");
  });

  it("omits branches with no visible leaves", () => {
    const report = specDataToReport(
      data,
      [],
      new Set(["pass", "fail", "skip"]),
    );
    expect(report).not.toContain("MySuite");
    expect(report).not.toContain("pkg");
  });

  it("preserves package duration when unfiltered", () => {
    const report = specDataToReport(data, []);
    expect(report).toContain("1.500s");
  });

  it("uses leaf-aggregated duration when filtered", () => {
    const report = specDataToReport(data, [], new Set(["pass", "skip"]));
    expect(report).not.toContain("1.500s");
    expect(report).toContain("0.800s");
  });

  it("preserves structural counts in summary", () => {
    const report = specDataToReport(data, [], new Set(["pass"]));
    expect(report).toContain("1 suites");
    expect(report).toContain("3 behaviors");
  });

  it("includes error output for failed leaves", () => {
    const dataWithOutput = {
      packages: [
        {
          path: "example.com/pkg",
          status: "fail",
          duration: 1.0,
          nodes: [
            suite("MySuite", [
              leaf("passes", "pass", 0.2),
              leaf("fails", "fail", 0.8, [
                "    file_test.go:42: Expected 1 to equal 2\n",
              ]),
            ]),
          ],
        },
      ],
      stats: {
        suites: 1,
        behaviors: 2,
        tests: 0,
        passed: 1,
        failed: 1,
        skipped: 0,
      },
    };
    const report = specDataToReport(dataWithOutput, []);
    expect(report).toContain("│ file_test.go:42: Expected 1 to equal 2");
  });

  it("filters === and --- delimiters from error output", () => {
    const dataWithOutput = {
      packages: [
        {
          path: "example.com/pkg",
          status: "fail",
          duration: 0.5,
          nodes: [
            suite("S", [
              leaf("fails", "fail", 0.5, [
                "=== RUN   TestFoo\n",
                "--- FAIL: TestFoo (0.00s)\n",
                "    foo_test.go:10: oops\n",
              ]),
            ]),
          ],
        },
      ],
      stats: {
        suites: 1,
        behaviors: 1,
        tests: 0,
        passed: 0,
        failed: 1,
        skipped: 0,
      },
    };
    const report = specDataToReport(dataWithOutput, []);
    expect(report).toContain("│ foo_test.go:10: oops");
    expect(report).not.toContain("=== RUN");
    expect(report).not.toContain("--- FAIL");
  });

  it("does not include error output when fail is hidden", () => {
    const dataWithOutput = {
      packages: [
        {
          path: "example.com/pkg",
          status: "fail",
          duration: 1.0,
          nodes: [
            suite("MySuite", [
              leaf("passes", "pass", 0.2),
              leaf("fails", "fail", 0.8, [
                "    file_test.go:42: Expected 1 to equal 2\n",
              ]),
            ]),
          ],
        },
      ],
      stats: {
        suites: 1,
        behaviors: 2,
        tests: 0,
        passed: 1,
        failed: 1,
        skipped: 0,
      },
    };
    const report = specDataToReport(dataWithOutput, [], new Set(["fail"]));
    expect(report).not.toContain("file_test.go:42");
    expect(report).not.toContain("Expected 1 to equal 2");
  });

  it("error output does not inflate column widths", () => {
    const longError = "x".repeat(200);
    const dataWithOutput = {
      packages: [
        {
          path: "example.com/pkg",
          status: "fail",
          duration: 0.5,
          nodes: [
            suite("S", [
              leaf("fails", "fail", 0.5, [
                `    file_test.go:1: ${longError}\n`,
              ]),
            ]),
          ],
        },
      ],
      stats: {
        suites: 1,
        behaviors: 1,
        tests: 0,
        passed: 0,
        failed: 1,
        skipped: 0,
      },
    };
    const report = specDataToReport(dataWithOutput, []);
    const headerLine = report.split("\n")[0];
    expect(headerLine.length).toBeLessThan(100);
  });
});

describe("specDataToReport broken packages", () => {
  const data = {
    packages: [
      {
        path: "example.com/healthy",
        status: "pass",
        duration: 0.5,
        nodes: [suite("OkSuite", [leaf("passes", "pass", 0.5)])],
      },
      {
        path: "example.com/broken",
        status: "fail",
        duration: 0,
        nodes: [],
        output: [
          "# example.com/broken\n",
          "svc.go:4:17: cannot use 42 as string value\n",
        ],
      },
    ],
    stats: {
      suites: 1,
      behaviors: 1,
      tests: 0,
      passed: 1,
      failed: 0,
      skipped: 0,
      failedPackages: 1,
    },
  };

  it("lists a package-level failure as its own row", () => {
    const report = specDataToReport(data, ["example.com"]);
    expect(report).toContain("broken");
    expect(report).toContain("FAIL (package)");
  });

  it("carries failed packages into the summary line", () => {
    const report = specDataToReport(data, ["example.com"]);
    expect(report).toContain("1 failed packages");
  });

  it("does not count package failures as behavior failures", () => {
    const report = specDataToReport(data, ["example.com"]);
    expect(report).not.toContain("1 failed,");
    expect(report).toContain("1 passed");
  });
});

describe("interpretSpecExit", () => {
  const SPEC_JSON = '{"packages":[],"stats":{}}';

  it("keeps the rendered spec when the stream carried failures", () => {
    // `gotest spec --input=-` exits 1 to report "this tree has failures".
    // That is a verdict about the tests, not a failure to render.
    const outcome = interpretSpecExit(1, SPEC_JSON, "");
    expect(outcome).toEqual({ ok: true, stdout: SPEC_JSON });
  });

  it("keeps the rendered spec when go run appends its exit status epilogue", () => {
    // The extension resolves the CLI through `go run` by default, which
    // prints "exit status 1" on stderr whenever the child exits non-zero.
    const outcome = interpretSpecExit(1, SPEC_JSON, "exit status 1\n");
    expect(outcome).toEqual({ ok: true, stdout: SPEC_JSON });
  });

  it("keeps the rendered spec on a clean run", () => {
    const outcome = interpretSpecExit(0, SPEC_JSON, "");
    expect(outcome).toEqual({ ok: true, stdout: SPEC_JSON });
  });

  it("reports an operational failure that produced no spec", () => {
    const outcome = interpretSpecExit(
      2,
      "",
      "FAIL: parsing test events: unexpected EOF\n",
    );
    expect(outcome).toEqual({
      ok: false,
      message:
        "gotest spec exited with code 2: FAIL: parsing test events: unexpected EOF",
    });
  });

  it("reports a go run build failure instead of swallowing it as a verdict", () => {
    // `go run` also exits 1 when the module fails to build. There is no spec
    // on stdout, so this must not be mistaken for a failing test tree.
    const outcome = interpretSpecExit(1, "", "spec.go:12:2: undefined: Foo\n");
    expect(outcome).toEqual({
      ok: false,
      message: "gotest spec exited with code 1: spec.go:12:2: undefined: Foo",
    });
  });

  it("strips the go run epilogue from a reported failure", () => {
    const outcome = interpretSpecExit(
      2,
      "",
      "FAIL: opening input file: no such file\nexit status 2\n",
    );
    expect(outcome).toEqual({
      ok: false,
      message:
        "gotest spec exited with code 2: FAIL: opening input file: no such file",
    });
  });

  it("reports a signal death that left no spec behind", () => {
    const outcome = interpretSpecExit(null, "", "");
    expect(outcome).toEqual({
      ok: false,
      message: "gotest spec exited with code null",
    });
  });
});

describe("encodeStateForScript", () => {
  // Regression: the spec is embedded inside a <script> block, and a test whose
  // failure output contained "</script>" closed that block early, spilling the
  // rest of the document into the page as live markup. Found by running a
  // suite whose assertion message carried markup.
  it("does not let test output close the script block", () => {
    const encoded = encodeStateForScript({
      message: "</script><script>alert('xss')</script>",
    });
    expect(encoded).not.toContain("</script>");
    expect(encoded).not.toContain("<script>");
  });

  it("round-trips to the identical value", () => {
    const data = {
      message: "</script><img src=x onerror=alert(1)>",
      nested: { markup: "<b>&amp;</b>", unicode: "日本語 🧪" },
    };
    expect(JSON.parse(encodeStateForScript(data))).toEqual(data);
  });

  it("leaves ordinary payloads parseable", () => {
    expect(
      JSON.parse(encodeStateForScript({ packages: [], stats: {} })),
    ).toEqual({ packages: [], stats: {} });
  });
});
