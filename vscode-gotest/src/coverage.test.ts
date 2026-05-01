import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => {
  class Position {
    constructor(public line: number, public character: number) {}
  }
  class Range {
    constructor(public start: Position, public end: Position) {}
  }
  class Uri {
    constructor(public fsPath: string) {}
    static file(path: string) { return new Uri(path); }
  }
  class StatementCoverage {
    constructor(public executed: number | boolean, public location: Range) {}
  }
  class FileCoverage {
    constructor(public uri: Uri, public statementCoverage: { covered: number; total: number }) {}
    static fromDetails(uri: Uri, details: StatementCoverage[]) {
      let covered = 0;
      for (const d of details) {
        if (typeof d.executed === "number" && d.executed > 0) covered++;
        else if (d.executed === true) covered++;
      }
      return new FileCoverage(uri, { covered, total: details.length });
    }
  }
  return { Position, Range, Uri, StatementCoverage, FileCoverage };
});

import { parseCoverProfile } from "./coverage.js";

describe("parseCoverProfile", () => {
  const moduleToDir = (importPath: string) => {
    if (importPath === "example.com/pkg") return "/abs/pkg";
    if (importPath === "example.com/other") return "/abs/other";
    return undefined;
  };

  it("parses a simple coverprofile", () => {
    const content = [
      "mode: set",
      "example.com/pkg/main.go:10.2,15.3 1 1",
      "example.com/pkg/main.go:20.5,25.10 1 0",
    ].join("\n");

    const result = parseCoverProfile(content, moduleToDir);
    expect(result).toHaveLength(1);
    expect(result[0].uri.fsPath).toBe("/abs/pkg/main.go");
    expect(result[0].statementCoverage.total).toBe(2);
    expect(result[0].statementCoverage.covered).toBe(1);
  });

  it("groups statements by file", () => {
    const content = [
      "mode: atomic",
      "example.com/pkg/a.go:1.1,2.2 1 5",
      "example.com/pkg/b.go:3.1,4.2 1 0",
      "example.com/pkg/a.go:5.1,6.2 1 3",
    ].join("\n");

    const result = parseCoverProfile(content, moduleToDir);
    expect(result).toHaveLength(2);

    const aFile = result.find((r) => r.uri.fsPath === "/abs/pkg/a.go");
    expect(aFile).toBeDefined();
    expect(aFile!.statementCoverage.total).toBe(2);
    expect(aFile!.statementCoverage.covered).toBe(2);
  });

  it("skips files with unresolvable import paths", () => {
    const content = [
      "mode: set",
      "unknown.com/nope/file.go:1.1,2.2 1 1",
    ].join("\n");

    const result = parseCoverProfile(content, moduleToDir);
    expect(result).toHaveLength(0);
  });

  it("skips mode lines and blank lines", () => {
    const content = "mode: set\n\n\n";
    const result = parseCoverProfile(content, moduleToDir);
    expect(result).toHaveLength(0);
  });

  it("skips malformed lines", () => {
    const content = [
      "mode: set",
      "not a valid line",
      "example.com/pkg/main.go:1.1,2.2 1 1",
    ].join("\n");

    const result = parseCoverProfile(content, moduleToDir);
    expect(result).toHaveLength(1);
  });

  it("returns empty array for empty input", () => {
    expect(parseCoverProfile("", moduleToDir)).toHaveLength(0);
  });

  it("handles multiple packages", () => {
    const content = [
      "mode: set",
      "example.com/pkg/main.go:1.1,2.2 1 1",
      "example.com/other/util.go:1.1,2.2 1 1",
    ].join("\n");

    const result = parseCoverProfile(content, moduleToDir);
    expect(result).toHaveLength(2);
    const paths = result.map((r) => r.uri.fsPath).sort();
    expect(paths).toEqual(["/abs/other/util.go", "/abs/pkg/main.go"]);
  });
});
