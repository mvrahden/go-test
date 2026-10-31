import { describe, it, expect, vi, beforeEach } from "vitest";
import * as path from "node:path";

const {
  mockSaveAll,
  mockShowErrorMessage,
  mockShowWarningMessage,
  mockBuildCliCommand,
  mockCaptureStdout,
} = vi.hoisted(() => ({
  mockSaveAll: vi.fn(async (_includeUntitled?: boolean) => true),
  mockShowErrorMessage: vi.fn(async (_msg: string) => undefined),
  mockShowWarningMessage: vi.fn(async () => undefined),
  mockBuildCliCommand: vi.fn(),
  mockCaptureStdout: vi.fn(async (..._args: unknown[]) => ""),
}));

vi.mock("vscode", async () => {
  const path = await import("node:path");
  const ws = path.resolve("/ws");
  class CodeAction {
    edit?: unknown;
    command?: { command: string; title: string; arguments?: unknown[] };
    constructor(
      public readonly title: string,
      public readonly kind: string,
    ) {}
  }
  class WorkspaceEdit {
    readonly replacements: { range: unknown; newText: string }[] = [];
    replace(_uri: unknown, range: unknown, newText: string) {
      this.replacements.push({ range, newText });
    }
  }
  class Range {
    constructor(
      public readonly startLine: number,
      public readonly startChar: number,
      public readonly endLine: number,
      public readonly endChar: number,
    ) {}
  }
  return {
    CodeAction,
    WorkspaceEdit,
    Range,
    CodeActionKind: { QuickFix: "quickfix" },
    workspace: {
      workspaceFolders: [{ uri: { fsPath: ws } }],
      getWorkspaceFolder: () => ({ uri: { fsPath: ws } }),
      getConfiguration: () => ({ get: () => undefined }),
      saveAll: mockSaveAll,
    },
    Uri: { file: (p: string) => ({ fsPath: p }) },
    window: {
      showErrorMessage: mockShowErrorMessage,
      showWarningMessage: mockShowWarningMessage,
      showInformationMessage: vi.fn(),
    },
  };
});

vi.mock("./cli.js", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./cli.js")>()),
  buildCliCommand: mockBuildCliCommand,
}));

vi.mock("./capture.js", () => ({ captureStdout: mockCaptureStdout }));

import {
  executeToggleFocus,
  FocusExcludeProvider,
  receiverTypeName,
  TOGGLE_FOCUS_COMMAND,
} from "./focusExclude.js";
import type { DiscoveryCache } from "./discovery.js";
import type { DiscoverMethod, DiscoverSuite } from "./types.js";

// Platform paths: the provider joins a package dir and a file name with the
// platform separator and compares the result with the document's path.
const ws = path.resolve("/ws");
const pkgDir = path.join(ws, "search");
const file = path.join(pkgDir, "suite_test.go");

function method(name: string, line: number): DiscoverMethod {
  return {
    name,
    parallel: false,
    focused: name.startsWith("F_"),
    excluded: name.startsWith("X_"),
    file: "suite_test.go",
    line,
    col: 1,
  };
}

function suite(
  name: string,
  line: number,
  methods: DiscoverMethod[],
): DiscoverSuite {
  return {
    name,
    parallel: false,
    focused: name.startsWith("F_"),
    excluded: name.startsWith("X_"),
    guarded: false,
    file: "suite_test.go",
    line,
    col: 1,
    lifecycle: [],
    fixtures: [],
    methods,
    benchmarks: [],
    fuzzers: [],
  };
}

const source = [
  "package search",
  "type IndexContractTestSuite[T any] struct{}",
  "func (s *IndexContractTestSuite[T]) TestEmpty(t *gotest.T) {}",
  "type ArticleIndexTestSuite = IndexContractTestSuite[Article]",
  "type CartTestSuite struct{}",
  "func (s *CartTestSuite) F_TestAdd(t *gotest.T) {}",
  "func (s *CartTestSuite) TestRemove(t *gotest.T) {}",
];

function provider(): FocusExcludeProvider {
  const cache = {
    packages: [
      {
        importPath: "example.com/search",
        dir: pkgDir,
        suites: [
          suite("ArticleIndexTestSuite", 4, [method("TestEmpty", 3)]),
          suite("CartTestSuite", 5, [
            method("F_TestAdd", 6),
            method("TestRemove", 7),
          ]),
        ],
      },
    ],
  } as unknown as DiscoveryCache;
  return new FocusExcludeProvider(cache);
}

function actionsAt(line: number) {
  const document = {
    fileName: file,
    uri: { fsPath: file },
    lineCount: source.length,
    lineAt: (i: number) => ({ text: source[i] }),
  } as unknown as import("vscode").TextDocument;
  const range = { start: { line: line - 1 } } as import("vscode").Range;
  return provider().provideCodeActions(document, range);
}

function byTitle(line: number, title: string) {
  const action = actionsAt(line).find((a) => a.title === title);
  expect(action).toBeDefined();
  return action!;
}

describe("focus code actions", () => {
  it("focus a suite through the CLI, named by its discovered name", () => {
    const action = byTitle(5, "Focus this suite");
    expect(action.edit).toBeUndefined();
    expect(action.command?.command).toBe(TOGGLE_FOCUS_COMMAND);
    expect(action.command?.arguments).toEqual([file, "CartTestSuite", ws]);
  });

  it("unfocus a method under its receiver type", () => {
    const action = byTitle(6, "Unfocus this test");
    expect(action.command?.arguments).toEqual([
      file,
      "CartTestSuite.F_TestAdd",
      ws,
    ]);
  });

  it("name an alias suite's method by the generic type it is declared on", () => {
    const action = byTitle(3, "Focus this test");
    expect(action.command?.arguments).toEqual([
      file,
      "IndexContractTestSuite.TestEmpty",
      ws,
    ]);
  });

  it("keep exclude as an in-document edit", () => {
    const action = byTitle(7, "Exclude this test");
    expect(action.command).toBeUndefined();
    expect(action.edit).toBeDefined();
  });
});

describe("receiverTypeName", () => {
  it("reads plain, pointer and generic receivers", () => {
    expect(receiverTypeName("func (s *S) M() {}")).toBe("S");
    expect(receiverTypeName("func (S) M() {}")).toBe("S");
    expect(receiverTypeName("func (s *S[K, V]) M() {}")).toBe("S");
  });
});

describe("executeToggleFocus", () => {
  const out = {
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    debug: vi.fn(),
  } as unknown as import("vscode").LogOutputChannel;

  beforeEach(() => {
    vi.clearAllMocks();
    mockSaveAll.mockResolvedValue(true);
    mockBuildCliCommand.mockImplementation(async (args: string[]) => ({
      bin: "gotest",
      args,
    }));
    mockCaptureStdout.mockResolvedValue("");
  });

  it("saves, runs toggle-focus on the file and rediscovers", async () => {
    const order: string[] = [];
    mockSaveAll.mockImplementation(async () => {
      order.push("save");
      return true;
    });
    mockCaptureStdout.mockImplementation(async () => {
      order.push("cli");
      return "";
    });
    const discover = vi.fn(() => order.push("discover"));

    await executeToggleFocus(file, "CartTestSuite.TestRemove", out, discover);

    expect(mockBuildCliCommand).toHaveBeenCalledWith(
      ["refactor", "toggle-focus", file, "CartTestSuite.TestRemove"],
      ws,
    );
    expect(mockCaptureStdout).toHaveBeenCalledWith(
      "gotest",
      ["refactor", "toggle-focus", file, "CartTestSuite.TestRemove"],
      { cwd: ws },
    );
    expect(order).toEqual(["save", "cli", "discover"]);
    expect(mockShowErrorMessage).not.toHaveBeenCalled();
  });

  it("does not run when unsaved changes could not be saved", async () => {
    mockSaveAll.mockResolvedValue(false);
    const discover = vi.fn();

    await executeToggleFocus(file, "CartTestSuite", out, discover);

    expect(mockCaptureStdout).not.toHaveBeenCalled();
    expect(discover).not.toHaveBeenCalled();
    expect(mockShowErrorMessage).toHaveBeenCalledTimes(1);
  });

  it("surfaces the CLI's refusal", async () => {
    mockCaptureStdout.mockRejectedValue(
      new Error(
        "exited with code 1: toggle-focus: F_CartTestSuite is already declared",
      ),
    );
    const discover = vi.fn();

    await executeToggleFocus(file, "CartTestSuite", out, discover);

    expect(discover).not.toHaveBeenCalled();
    expect(mockShowErrorMessage).toHaveBeenCalledWith(
      expect.stringContaining("F_CartTestSuite is already declared"),
    );
  });
});
