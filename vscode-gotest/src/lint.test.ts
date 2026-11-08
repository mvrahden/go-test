import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

const { sets, captures } = vi.hoisted(() => ({
  sets: [] as [unknown, unknown][][],
  captures: [] as {
    args: string[];
    abort?: AbortSignal;
    finish: (out: {
      code: number;
      stdout: string;
      stderr: string;
      aborted: boolean;
    }) => void;
  }[],
}));

vi.mock("vscode", () => {
  class Position {
    constructor(
      public readonly line: number,
      public readonly character: number,
    ) {}
  }
  class Range {
    constructor(
      public readonly start: Position,
      public readonly end: Position,
    ) {}
  }
  class Diagnostic {
    source?: string;
    code?: unknown;
    constructor(
      public readonly range: Range,
      public readonly message: string,
      public readonly severity: number,
    ) {}
  }
  class WorkspaceEdit {
    readonly replacements: { uri: unknown; range: Range; text: string }[] = [];
    replace(uri: unknown, range: Range, text: string) {
      this.replacements.push({ uri, range, text });
    }
  }
  class CodeAction {
    constructor(
      public readonly title: string,
      public readonly kind: unknown,
    ) {}
  }
  return {
    Position,
    Range,
    Diagnostic,
    WorkspaceEdit,
    CodeAction,
    CodeActionKind: { QuickFix: "quickfix" },
    DiagnosticSeverity: { Warning: 1 },
    Uri: { file: (p: string) => ({ fsPath: p }) },
    languages: {
      createDiagnosticCollection: () => ({
        set: (entries: [unknown, unknown][]) => sets.push(entries),
        dispose: () => {},
      }),
    },
    workspace: {
      textDocuments: [],
      getWorkspaceFolder: () => ({ uri: { fsPath: "/ws" } }),
      getConfiguration: () => ({ get: () => undefined }),
    },
    window: {},
  };
});

vi.mock("./capture.js", () => ({
  captureOutput: (
    _bin: string,
    args: string[],
    opts: { abort?: AbortSignal },
  ) =>
    new Promise((finish) => {
      captures.push({ args, abort: opts.abort, finish });
    }),
}));

vi.mock("./cli.js", () => ({
  buildCliCommand: async (args: string[]) => ({ bin: "gotest", args }),
  offerCliUpgrade: () => false,
  formatCliCommand: () => "gotest",
  stripGoRunExitEcho: (s: string) => s,
}));

import { LintService } from "./lint.js";
import type { LintFinding } from "./lintFindings.js";

interface Internals {
  publish(
    scope: { key: string; covers: (f: string) => boolean },
    findings: LintFinding[],
  ): Promise<void>;
}

const channel = {
  info: () => {},
  warn: () => {},
  error: () => {},
  debug: () => {},
} as never;

// A fixable finding on the call after a comment with multi-byte characters:
// its byte offsets lie past where the editor's UTF-16 offsets would.
const source =
  "package p\n\n// café 𝔾\nfunc TestX(t *testing.T) { if 1+1 != 2 { t.Fatal() } }\n";
const target = "if 1+1 != 2 { t.Fatal() }";

describe("LintService", () => {
  let dir = "";
  let file = "";
  let finding: LintFinding;

  beforeEach(() => {
    sets.length = 0;
    dir = mkdtempSync(path.join(tmpdir(), "gotest-lint-unit-"));
    file = path.join(dir, "p_test.go");
    writeFileSync(file, source, "utf-8");
    const start = Buffer.from(source, "utf8").indexOf(Buffer.from(target));
    finding = {
      category: "fail-guard",
      message: "use Equal instead of if+Fatal for != comparison",
      file,
      line: 4,
      col: Buffer.from("func TestX(t *testing.T) { ").length + 1,
      endLine: 4,
      endCol: Buffer.from("func TestX(t *testing.T) { ").length + 1,
      fixes: [
        {
          message: "use Equal",
          edits: [
            {
              filename: file,
              start,
              end: start + Buffer.byteLength(target),
              new: "gotest.Equal(t, 1+1, 2)",
            },
          ],
        },
      ],
    };
  });

  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  function service() {
    const svc = new LintService({} as never, channel);
    const scope = { key: "p", covers: (f: string) => path.dirname(f) === dir };
    return {
      svc,
      publish: (fs: LintFinding[]) =>
        (svc as unknown as Internals).publish(scope, fs),
    };
  }

  it("places an edit past multi-byte characters on the characters the lint meant", async () => {
    const { svc, publish } = service();
    await publish([finding]);

    const edit = (await svc.editFor(finding.fixes[0])) as unknown as {
      replacements: {
        range: { start: { line: number; character: number }; end: unknown };
        text: string;
      }[];
    };

    expect(edit).toBeDefined();
    const { range, text } = edit.replacements[0];
    const line = source.split("\n")[range.start.line];
    expect(line.slice(range.start.character)).toMatch(/^if 1\+1 != 2/);
    expect(text).toBe("gotest.Equal(t, 1+1, 2)");
  });

  it("refuses to apply a fix to a file that changed since the lint read it", async () => {
    const { svc, publish } = service();
    await publish([finding]);
    writeFileSync(file, "// edited\n" + source, "utf-8");

    expect(await svc.editFor(finding.fixes[0])).toBeUndefined();
  });

  it("starts a finding where the lint put it and draws it to the end of the line", async () => {
    const { publish } = service();
    await publish([finding]);

    const [, diagnostics] = sets[0][0] as [
      unknown,
      {
        range: { start: { character: number }; end: { character: number } };
        source: string;
        code: string;
      }[],
    ];
    const d = diagnostics[0];
    const line = source.split("\n")[3];
    expect(line.slice(d.range.start.character)).toMatch(/^if 1\+1/);
    expect(d.range.end.character).toBe(line.length);
    expect(d.source).toBe("gotest");
    expect(d.code).toBe("fail-guard");
  });

  it("replaces a scope's diagnostics in one step, clearing files with none left", async () => {
    const { publish } = service();
    await publish([finding]);
    await publish([]);

    expect(sets).toHaveLength(2);
    expect(sets[1]).toEqual([[{ fsPath: file }, undefined]]);
  });
});

describe("LintService triggers", () => {
  beforeEach(() => {
    captures.length = 0;
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  const cache = {
    resolveFileToPackage: () => "example.com/p",
    getPackage: () => ({ dir: "/ws/p" }),
    getWorkspaceDir: () => "/ws",
  };
  const saved = (file: string) =>
    ({ fileName: file, uri: { fsPath: file } }) as never;

  it("lints a package once for a burst of saves, and never for a non-test file", async () => {
    const svc = new LintService(cache as never, channel);
    svc.onSaved(saved("/ws/p/a_test.go"));
    await vi.advanceTimersByTimeAsync(200);
    svc.onSaved(saved("/ws/p/b_test.go"));
    svc.onSaved(saved("/ws/p/c.go"));
    await vi.advanceTimersByTimeAsync(499);
    expect(captures).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(1);

    expect(captures).toHaveLength(1);
    expect(captures[0].args).toEqual(["lint", "-json", "example.com/p"]);
    svc.dispose();
  });

  it("ends the lint in flight when a newer one for the package starts", async () => {
    const svc = new LintService(cache as never, channel);
    void svc.lintPackage("example.com/p");
    await vi.advanceTimersByTimeAsync(0);
    void svc.lintPackage("example.com/p");
    await vi.advanceTimersByTimeAsync(0);

    expect(captures).toHaveLength(2);
    expect(captures[0].abort?.aborted).toBe(true);
    expect(captures[1].abort?.aborted).toBe(false);
    svc.dispose();
  });
});
