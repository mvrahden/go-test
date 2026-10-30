import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

const { sets, captures, cli } = vi.hoisted(() => ({
  // What buildCliCommand throws for `lint -rules -json`, and the errors
  // offered an upgrade.
  cli: { rulesFail: undefined as Error | undefined, offers: [] as unknown[] },
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
    DiagnosticSeverity: { Error: 0, Warning: 1 },
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
  buildCliCommand: async (args: string[]) => {
    if (args.includes("-rules") && cli.rulesFail) throw cli.rulesFail;
    return { bin: "gotest", args };
  },
  offerCliUpgrade: (err: unknown) => {
    cli.offers.push(err);
    return false;
  },
  formatCliCommand: () => "gotest",
  stripGoRunExitEcho: (s: string) => s,
}));

import { LintService } from "./lint.js";
import type { LintFinding } from "./lintFindings.js";

interface Drawn {
  range: {
    start: { line: number; character: number };
    end: { line: number; character: number };
  };
  code: string;
  severity: number;
}

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

  it("draws a finding to its end when the end lies past its start", async () => {
    const { publish } = service();
    const call = "if 1+1 != 2";
    const onItsLine = {
      ...finding,
      endCol: finding.col + Buffer.byteLength(call),
    };
    // From the comment's first byte to the fifth byte of the next line.
    const acrossLines = {
      ...finding,
      message: "across lines",
      line: 3,
      col: 1,
      endLine: 4,
      endCol: 5,
    };
    await publish([onItsLine, acrossLines]);

    const [, diagnostics] = sets[0][0] as [unknown, Drawn[]];
    const line = source.split("\n")[3];
    const start = line.indexOf(call);
    expect(diagnostics[0].range.start).toEqual({ line: 3, character: start });
    expect(diagnostics[0].range.end).toEqual({
      line: 3,
      character: start + call.length,
    });
    expect(diagnostics[1].range.start).toEqual({ line: 2, character: 0 });
    expect(diagnostics[1].range.end).toEqual({ line: 3, character: 4 });
  });

  it("draws a finding whose end lies before its start to the end of its line", async () => {
    const { publish } = service();
    await publish([{ ...finding, endCol: finding.col - 3 }]);

    const [, diagnostics] = sets[0][0] as [unknown, Drawn[]];
    expect(diagnostics[0].range.end).toEqual({
      line: 3,
      character: source.split("\n")[3].length,
    });
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
  // The lint runs, without the one read of the rule table.
  const lints = () => captures.filter((c) => !c.args.includes("-rules"));

  it("lints a package once for a burst of saves, and never for a non-test file", async () => {
    const svc = new LintService(cache as never, channel);
    svc.onSaved(saved("/ws/p/a_test.go"));
    await vi.advanceTimersByTimeAsync(200);
    svc.onSaved(saved("/ws/p/b_test.go"));
    svc.onSaved(saved("/ws/p/c.go"));
    await vi.advanceTimersByTimeAsync(499);
    expect(lints()).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(1);

    expect(lints()).toHaveLength(1);
    expect(lints()[0].args).toEqual(["lint", "-json", "example.com/p"]);
    svc.dispose();
  });

  it("ends the lint in flight when a newer one for the package starts", async () => {
    const svc = new LintService(cache as never, channel);
    void svc.lintPackage("example.com/p");
    await vi.advanceTimersByTimeAsync(0);
    void svc.lintPackage("example.com/p");
    await vi.advanceTimersByTimeAsync(0);

    expect(lints()).toHaveLength(2);
    expect(lints()[0].abort?.aborted).toBe(true);
    expect(lints()[1].abort?.aborted).toBe(false);
    svc.dispose();
  });
});

describe("LintService severities", () => {
  const warnings: string[] = [];
  const recording = {
    info: () => {},
    debug: () => {},
    error: () => {},
    warn: (m: string) => warnings.push(m),
  } as never;
  let dir = "";
  let file = "";

  beforeEach(() => {
    sets.length = 0;
    captures.length = 0;
    warnings.length = 0;
    cli.rulesFail = undefined;
    cli.offers.length = 0;
    dir = mkdtempSync(path.join(tmpdir(), "gotest-lint-tiers-"));
    file = path.join(dir, "p_test.go");
    writeFileSync(file, source, "utf-8");
  });

  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  const cache = () => ({
    resolveFileToPackage: () => "example.com/p",
    getPackage: () => ({ dir }),
    getWorkspaceDir: () => "/ws",
  });

  const rulesDoc = JSON.stringify({
    version: "dev",
    rules: [
      { id: "focus", tier: "integrity" },
      { id: "fail-guard", tier: "expressiveness" },
      { id: "stdlib-test", tier: "migration" },
    ],
  });

  // A lint document with one finding of each rule named.
  const lintDoc = (...rules: string[]) =>
    JSON.stringify({
      "example.com/p": {
        gotest: rules.map((category, i) => ({
          category,
          posn: `${file}:4:${i + 1}`,
          end: `${file}:4:${i + 2}`,
          message: `finding ${i}`,
        })),
      },
    });

  const ok = (stdout: string) => ({
    code: 0,
    stdout,
    stderr: "",
    aborted: false,
  });
  const isRules = (c: (typeof captures)[number]) => c.args.includes("-rules");
  const lints = () => captures.filter((c) => !isRules(c));
  const rulesReads = () => captures.filter(isRules);

  // The newest diagnostics of the file, each rule's severity.
  const severities = () => {
    const [, diagnostics] = sets[sets.length - 1][0] as [unknown, Drawn[]];
    return Object.fromEntries(diagnostics.map((d) => [d.code, d.severity]));
  };

  it("makes an integrity rule's finding an error and every other one a warning, by the CLI's rule table", async () => {
    const svc = new LintService(cache() as never, recording);
    const done = svc.lintPackage("example.com/p");
    await vi.waitFor(() => expect(captures).toHaveLength(2));
    rulesReads()[0].finish(ok(rulesDoc));
    lints()[0].finish(
      ok(lintDoc("focus", "fail-guard", "stdlib-test", "unlisted")),
    );
    await done;

    expect(severities()).toEqual({
      focus: 0,
      "fail-guard": 1,
      "stdlib-test": 1,
      unlisted: 1,
    });
    expect(rulesReads()[0].args).toEqual(["lint", "-rules", "-json"]);
    svc.dispose();
  });

  it("reads the rule table once per folder, however many lints ask for it", async () => {
    const svc = new LintService(cache() as never, recording);
    const first = svc.lintPackage("example.com/p");
    const second = svc.lintFolder("/ws");
    await vi.waitFor(() => expect(lints()).toHaveLength(2));
    expect(rulesReads()).toHaveLength(1);
    rulesReads()[0].finish(ok(rulesDoc));
    for (const c of lints()) c.finish(ok(lintDoc("focus")));
    await Promise.all([first, second]);

    const third = svc.lintPackage("example.com/p");
    await vi.waitFor(() => expect(lints()).toHaveLength(3));
    lints()[2].finish(ok(lintDoc("focus")));
    await third;

    expect(rulesReads()).toHaveLength(1);
    expect(severities()).toEqual({ focus: 0 });
    svc.dispose();
  });

  it.each([
    {
      how: "exits non-zero",
      reason: "flag provided but not defined: -rules",
      answer: {
        code: 2,
        stdout: "",
        stderr: "flag provided but not defined: -rules\nexit status 2\n",
        aborted: false,
      },
    },
    {
      how: "writes no JSON",
      reason: "unreadable JSON",
      answer: ok("{not json"),
    },
    {
      how: "writes a document without rules",
      reason: "no rules list",
      answer: ok(JSON.stringify({ version: "dev" })),
    },
    {
      how: "cannot be built",
      reason: "go.mod pins gotest v1.20.0",
      fail: new Error("go.mod pins gotest v1.20.0"),
    },
  ])(
    "leaves every finding a warning when the CLI $how, says why once and never asks again",
    async ({ reason, answer, fail }) => {
      cli.rulesFail = fail;
      const svc = new LintService(cache() as never, recording);
      for (let n = 1; n <= 2; n++) {
        const done = svc.lintPackage("example.com/p");
        await vi.waitFor(() => expect(lints()).toHaveLength(n));
        if (n === 1 && answer) {
          await vi.waitFor(() => expect(rulesReads()).toHaveLength(1));
          rulesReads()[0].finish(answer);
        }
        lints()[n - 1].finish(ok(lintDoc("focus", "fail-guard")));
        await done;
        expect(severities()).toEqual({ focus: 1, "fail-guard": 1 });
      }

      expect(rulesReads()).toHaveLength(answer ? 1 : 0);
      const said = warnings.filter((w) => w.includes("rule tiers unread"));
      expect(said).toHaveLength(1);
      expect(said[0]).toContain(reason);
      expect(cli.offers).toEqual(fail ? [fail] : []);
      svc.dispose();
    },
  );
});
