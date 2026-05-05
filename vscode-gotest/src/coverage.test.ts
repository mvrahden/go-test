import { describe, it, expect, vi } from "vitest";

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
  class Uri {
    constructor(public fsPath: string) {}
    static file(path: string) {
      return new Uri(path);
    }
  }
  class StatementCoverage {
    constructor(
      public executed: number | boolean,
      public location: Range,
    ) {}
  }
  class DeclarationCoverage {
    constructor(
      public name: string,
      public executed: number | boolean,
      public location: Position | Range,
    ) {}
  }
  class FileCoverage {
    constructor(
      public uri: Uri,
      public statementCoverage: { covered: number; total: number },
    ) {}
    static fromDetails(
      uri: Uri,
      details: (StatementCoverage | DeclarationCoverage)[],
    ) {
      let covered = 0;
      for (const d of details) {
        if ("executed" in d) {
          if (typeof d.executed === "number" && d.executed > 0) covered++;
          else if (d.executed === true) covered++;
        }
      }
      return new FileCoverage(uri, { covered, total: details.length });
    }
  }
  return {
    Position,
    Range,
    Uri,
    StatementCoverage,
    DeclarationCoverage,
    FileCoverage,
  };
});

import {
  parseCoverProfile,
  parseFuncCoverage,
  buildFileCoverages,
} from "./coverage.js";

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
    expect(result[0].absPath).toBe("/abs/pkg/main.go");
    expect(result[0].statements).toHaveLength(2);
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

    const aFile = result.find((r) => r.absPath === "/abs/pkg/a.go");
    expect(aFile).toBeDefined();
    expect(aFile!.statements).toHaveLength(2);
  });

  it("skips files with unresolvable import paths", () => {
    const content = ["mode: set", "unknown.com/nope/file.go:1.1,2.2 1 1"].join(
      "\n",
    );

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
    const paths = result.map((r) => r.absPath).sort();
    expect(paths).toEqual(["/abs/other/util.go", "/abs/pkg/main.go"]);
  });
});

describe("parseFuncCoverage", () => {
  const moduleToDir = (importPath: string) => {
    if (importPath === "example.com/pkg") return "/abs/pkg";
    return undefined;
  };

  it("parses go tool cover -func output", () => {
    const content = [
      "example.com/pkg/main.go:10:\tLogin\t\t85.7%",
      "example.com/pkg/main.go:42:\tLogout\t\t100.0%",
      "total:\t\t\t\t\t(statements)\t91.2%",
    ].join("\n");

    const result = parseFuncCoverage(content, moduleToDir);
    expect(result.size).toBe(1);
    const decls = result.get("/abs/pkg/main.go")!;
    expect(decls).toHaveLength(2);
    expect(decls[0].name).toBe("Login");
    expect(decls[0].executed).toBeCloseTo(0.857);
    expect(decls[1].name).toBe("Logout");
    expect(decls[1].executed).toBeCloseTo(1.0);
  });

  it("marks 0% functions as not executed", () => {
    const content = "example.com/pkg/main.go:5:\tUnused\t\t0.0%\n";
    const result = parseFuncCoverage(content, moduleToDir);
    const decls = result.get("/abs/pkg/main.go")!;
    expect(decls[0].executed).toBe(false);
  });

  it("skips total line and unresolvable paths", () => {
    const content = [
      "total:\t\t(statements)\t50.0%",
      "unknown.com/x/y.go:1:\tFoo\t100.0%",
    ].join("\n");
    const result = parseFuncCoverage(content, moduleToDir);
    expect(result.size).toBe(0);
  });

  it("returns empty map for empty input", () => {
    expect(parseFuncCoverage("", moduleToDir).size).toBe(0);
  });
});

describe("buildFileCoverages", () => {
  const moduleToDir = (importPath: string) => {
    if (importPath === "example.com/pkg") return "/abs/pkg";
    return undefined;
  };

  it("builds FileCoverage from parsed data", () => {
    const parsed = parseCoverProfile(
      "mode: set\nexample.com/pkg/main.go:1.1,2.2 1 1\n",
      moduleToDir,
    );
    const result = buildFileCoverages(parsed);
    expect(result).toHaveLength(1);
    expect(result[0].uri.fsPath).toBe("/abs/pkg/main.go");
  });

  it("merges declarations when provided", () => {
    const parsed = parseCoverProfile(
      "mode: set\nexample.com/pkg/main.go:1.1,2.2 1 1\n",
      moduleToDir,
    );
    const funcContent = "example.com/pkg/main.go:1:\tMyFunc\t\t100.0%\n";
    const declarations = parseFuncCoverage(funcContent, moduleToDir);
    const result = buildFileCoverages(parsed, declarations);
    expect(result).toHaveLength(1);
  });
});
