import { describe, it, expect, vi, beforeEach } from "vitest";

const h = vi.hoisted(() => ({
  config: {} as Record<string, unknown>,
  collections: [] as any[],
  statusBars: [] as any[],
}));

vi.mock("vscode", () => {
  class Position {
    constructor(
      public line: number,
      public character: number,
    ) {}
  }
  class Range {
    start: Position;
    end: Position;
    constructor(a: any, b: any, c?: number, d?: number) {
      if (typeof a === "number") {
        this.start = new Position(a, b);
        this.end = new Position(c!, d!);
      } else {
        this.start = a;
        this.end = b;
      }
    }
  }
  return {
    Position,
    Range,
    Selection: class extends Range {},
    Diagnostic: class {
      source: string | undefined;
      constructor(
        public range: Range,
        public message: string,
        public severity: number,
      ) {}
    },
    DiagnosticSeverity: { Error: 0, Warning: 1 },
    StatusBarAlignment: { Left: 1, Right: 2 },
    TextEditorRevealType: { InCenter: 2 },
    Uri: { file: (p: string) => ({ fsPath: p }) },
    languages: {
      createDiagnosticCollection: (name: string) => {
        const c = { name, clear: vi.fn(), set: vi.fn(), dispose: vi.fn() };
        h.collections.push(c);
        return c;
      },
    },
    window: {
      createStatusBarItem: (alignment: number, priority: number) => {
        const s = {
          alignment,
          priority,
          text: "",
          command: undefined,
          show: vi.fn(),
          hide: vi.fn(),
          dispose: vi.fn(),
        };
        h.statusBars.push(s);
        return s;
      },
      showQuickPick: vi.fn(),
      showTextDocument: vi.fn(),
    },
    workspace: {
      getConfiguration: () => ({
        get: (k: string, d?: unknown) => h.config[k] ?? d,
      }),
      openTextDocument: vi.fn(),
    },
  };
});

import * as vscode from "vscode";
import { FocusDiagnostics } from "./diagnostics.js";

function method(name: string, line: number, focused = false) {
  return { name, file: "s_test.go", line, focused };
}

function suite(
  name: string,
  opts: { focused?: boolean; methods?: any[] } = {},
) {
  return {
    name,
    file: "s_test.go",
    line: 10,
    focused: opts.focused ?? false,
    methods: opts.methods ?? [],
  };
}

function makeCache(packages: any[] = [], warnings: any[] = []) {
  const subscription = { dispose: vi.fn() };
  const cache = {
    packages,
    warnings,
    getPackage: (ip: string) => packages.find((p) => p.importPath === ip),
    update: undefined as (() => void) | undefined,
    onDidUpdate: vi.fn((fn: () => void) => {
      cache.update = fn;
      return subscription;
    }),
  };
  return { cache, subscription };
}

const focusedPkg = () => ({
  importPath: "m/a",
  dir: "/ws/a",
  suites: [
    suite("F_ShopTestSuite", {
      focused: true,
      methods: [method("F_TestBuy", 20, true), method("TestSell", 30)],
    }),
    suite("CartTestSuite", { methods: [method("F_TestAdd", 40, true)] }),
  ],
});

// Diagnostics per file, as the collection received them.
function setCalls(collection: any) {
  return collection.set.mock.calls.map(([uri, diags]: [any, any[]]) => ({
    file: uri.fsPath,
    diags: diags.map((d) => ({
      line: d.range.start.line,
      from: d.range.start.character,
      to: d.range.end.character,
      message: d.message,
      severity: d.severity,
      source: d.source,
    })),
  }));
}

describe("FocusDiagnostics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    h.config = {};
    h.collections = [];
    h.statusBars = [];
  });

  it("creates its collection and status bar item and refreshes on discovery", () => {
    const { cache } = makeCache([focusedPkg()]);
    new FocusDiagnostics(cache as any);

    expect(h.collections[0].name).toBe("gotest");
    expect(h.statusBars[0]).toMatchObject({
      alignment: vscode.StatusBarAlignment.Left,
      command: "gotest.showFocusedTests",
    });
    expect(cache.onDidUpdate).toHaveBeenCalledTimes(1);
    expect(h.collections[0].set).not.toHaveBeenCalled();

    cache.update!();
    expect(h.collections[0].set).toHaveBeenCalled();
  });

  it("marks every focused suite and method and counts them in the status bar", () => {
    const { cache } = makeCache([focusedPkg()]);
    new FocusDiagnostics(cache as any).refresh();

    const W = vscode.DiagnosticSeverity.Warning;
    expect(setCalls(h.collections[0])).toEqual([
      {
        file: "/ws/a/s_test.go",
        diags: [
          {
            line: 9,
            from: 0,
            to: "F_ShopTestSuite".length,
            message: "Focused test suite — will cause CI failure (gotest --ci)",
            severity: W,
            source: "gotest",
          },
          {
            line: 19,
            from: 0,
            to: "F_TestBuy".length,
            message: "Focused test — will cause CI failure (gotest --ci)",
            severity: W,
            source: "gotest",
          },
          {
            line: 39,
            from: 0,
            to: "F_TestAdd".length,
            message: "Focused test — will cause CI failure (gotest --ci)",
            severity: W,
            source: "gotest",
          },
        ],
      },
    ]);
    expect(h.statusBars[0].text).toContain("3 focused test(s)");
    expect(h.statusBars[0].show).toHaveBeenCalled();
    expect(h.statusBars[0].hide).not.toHaveBeenCalled();
  });

  it("hides the status bar when nothing is focused", () => {
    const { cache } = makeCache([
      {
        importPath: "m/a",
        dir: "/ws/a",
        suites: [suite("S", { methods: [method("TestX", 3)] })],
      },
    ]);
    new FocusDiagnostics(cache as any).refresh();
    expect(h.collections[0].clear).toHaveBeenCalled();
    expect(h.collections[0].set).not.toHaveBeenCalled();
    expect(h.statusBars[0].hide).toHaveBeenCalled();
    expect(h.statusBars[0].show).not.toHaveBeenCalled();
  });

  it("shows nothing with focus warnings turned off", () => {
    h.config = { showFocusWarnings: false };
    const { cache } = makeCache(
      [focusedPkg()],
      [{ importPath: "m/a", file: "s_test.go", line: 2, message: "bad" }],
    );
    new FocusDiagnostics(cache as any).refresh();
    expect(h.collections[0].clear).toHaveBeenCalled();
    expect(h.collections[0].set).not.toHaveBeenCalled();
    expect(h.statusBars[0].hide).toHaveBeenCalled();
    expect(h.statusBars[0].show).not.toHaveBeenCalled();
  });

  it("places discovery warnings, defaulting to the first line and column", () => {
    const pkg = { importPath: "m/b", dir: "/ws/b", suites: [] };
    const { cache } = makeCache(
      [pkg],
      [
        {
          importPath: "m/b",
          file: "x_test.go",
          line: 7,
          col: 3,
          message: "placed",
        },
        { importPath: "m/b", file: "y_test.go", message: "unplaced" },
        { importPath: "m/b", message: "no file" },
        { importPath: "m/unknown", file: "z_test.go", message: "unknown pkg" },
      ],
    );
    new FocusDiagnostics(cache as any).refresh();

    const W = vscode.DiagnosticSeverity.Warning;
    expect(setCalls(h.collections[0])).toEqual([
      {
        file: "/ws/b/x_test.go",
        diags: [
          {
            line: 6,
            from: 2,
            to: 3,
            message: "placed",
            severity: W,
            source: "gotest",
          },
        ],
      },
      {
        file: "/ws/b/y_test.go",
        diags: [
          {
            line: 0,
            from: 0,
            to: 1,
            message: "unplaced",
            severity: W,
            source: "gotest",
          },
        ],
      },
    ]);
  });

  it("sets focus marks and warnings of one file together", () => {
    const { cache } = makeCache(
      [focusedPkg()],
      [
        {
          importPath: "m/a",
          file: "s_test.go",
          line: 2,
          col: 1,
          message: "shadowed",
        },
      ],
    );
    new FocusDiagnostics(cache as any).refresh();

    const calls = setCalls(h.collections[0]);
    expect(calls).toHaveLength(1);
    expect(calls[0].file).toBe("/ws/a/s_test.go");
    expect(calls[0].diags.map((d: any) => d.message)).toEqual([
      expect.stringContaining("Focused test suite"),
      expect.stringContaining("Focused test"),
      expect.stringContaining("Focused test"),
      "shadowed",
    ]);
  });

  it("lists focused suites and methods and opens the one picked", async () => {
    const { cache } = makeCache([focusedPkg()]);
    const editor = { selection: undefined as any, revealRange: vi.fn() };
    vi.mocked(vscode.workspace.openTextDocument).mockResolvedValueOnce(
      "doc" as any,
    );
    vi.mocked(vscode.window.showTextDocument).mockResolvedValueOnce(
      editor as any,
    );
    vi.mocked(vscode.window.showQuickPick).mockImplementationOnce(
      async (items: any) => items[1],
    );

    await new FocusDiagnostics(cache as any).showFocusedTests();

    const offered = vi.mocked(vscode.window.showQuickPick).mock
      .calls[0][0] as any[];
    expect(
      offered.map(({ label, description, detail }) => ({
        label,
        description,
        detail,
      })),
    ).toEqual([
      {
        label: "F_ShopTestSuite",
        description: "s_test.go:10",
        detail: "Focused suite",
      },
      {
        label: "F_TestBuy",
        description: "s_test.go:20",
        detail: "Focused method in F_ShopTestSuite",
      },
      {
        label: "F_TestAdd",
        description: "s_test.go:40",
        detail: "Focused method in CartTestSuite",
      },
    ]);
    expect(vscode.workspace.openTextDocument).toHaveBeenCalledWith(
      "/ws/a/s_test.go",
    );
    expect(vscode.window.showTextDocument).toHaveBeenCalledWith("doc");
    expect(editor.selection.start).toEqual({ line: 19, character: 0 });
    expect(editor.revealRange).toHaveBeenCalledWith(
      expect.objectContaining({ start: { line: 19, character: 0 } }),
      vscode.TextEditorRevealType.InCenter,
    );
  });

  it("opens nothing when the pick is dismissed", async () => {
    const { cache } = makeCache([focusedPkg()]);
    vi.mocked(vscode.window.showQuickPick).mockResolvedValueOnce(undefined);
    await new FocusDiagnostics(cache as any).showFocusedTests();
    expect(vscode.workspace.openTextDocument).not.toHaveBeenCalled();
    expect(vscode.window.showTextDocument).not.toHaveBeenCalled();
  });

  it("disposes the collection, the status bar item and the subscription", () => {
    const { cache, subscription } = makeCache();
    new FocusDiagnostics(cache as any).dispose();
    expect(h.collections[0].dispose).toHaveBeenCalled();
    expect(h.statusBars[0].dispose).toHaveBeenCalled();
    expect(subscription.dispose).toHaveBeenCalled();
  });
});
