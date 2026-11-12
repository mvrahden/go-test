import { describe, it, expect, vi, beforeEach } from "vitest";
import * as path from "node:path";

const h = vi.hoisted(() => ({
  showWarningMessage: vi.fn(
    async (..._args: unknown[]): Promise<unknown> => undefined,
  ),
  showErrorMessage: vi.fn(async (..._args: unknown[]) => undefined),
  openTextDocument: vi.fn(async (p: string) => ({ fileName: p })),
  showTextDocument: vi.fn(async () => undefined),
  buildCliCommand: vi.fn(),
  runGoInModule: vi.fn(async () => undefined),
  readModuleRoots: vi.fn(
    async (): Promise<{ dir: string; goMod: string }[]> => [],
  ),
  captureStdout: vi.fn(async (): Promise<string> => ""),
  folderOf: undefined as { uri: { fsPath: string } } | undefined,
}));

vi.mock("vscode", () => ({
  workspace: {
    workspaceFolders: [{ uri: { fsPath: "/ws" } }],
    getConfiguration: () => ({ get: () => undefined }),
    getWorkspaceFolder: () => h.folderOf,
    openTextDocument: h.openTextDocument,
  },
  Uri: { file: (p: string) => ({ fsPath: p }) },
  CodeActionKind: { RefactorExtract: "refactor.extract" },
  CodeAction: class {
    command: unknown;
    constructor(
      public readonly title: string,
      public readonly kind: unknown,
    ) {}
  },
  window: {
    showWarningMessage: h.showWarningMessage,
    showErrorMessage: h.showErrorMessage,
    showInformationMessage: vi.fn(),
    showTextDocument: h.showTextDocument,
  },
}));

vi.mock("./cli.js", async (importOriginal) => ({
  // The errors are the real ones; resolution and go are scripted.
  ...(await importOriginal<typeof import("./cli.js")>()),
  buildCliCommand: h.buildCliCommand,
  runGoInModule: h.runGoInModule,
}));

vi.mock("./gomod.js", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./gomod.js")>()),
  readModuleRoots: h.readModuleRoots,
}));

vi.mock("./capture.js", () => ({ captureStdout: h.captureStdout }));

import { GotestNotRequiredError } from "./cli.js";
import { moduleRootFor } from "./gomod.js";
import {
  executeScaffold,
  scaffoldTargetDir,
  ScaffoldCodeActionProvider,
} from "./scaffold.js";

const MODULE = "github.com/mvrahden/go-test/cmd/gotest";
const root = (dir: string) => ({ dir, goMod: "module example.com/x\n" });
const log = {
  info: vi.fn(),
  warn: vi.fn(),
  error: vi.fn(),
  debug: vi.fn(),
} as never;
const discover = () => undefined;

describe("executeScaffold", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    h.buildCliCommand.mockReset();
    h.buildCliCommand.mockResolvedValue({
      bin: "go",
      args: ["tool", MODULE, "scaffold"],
    });
    h.captureStdout.mockResolvedValue("");
  });

  it("asks before go.mod is changed, adds gotest in the target's module, then scaffolds", async () => {
    h.buildCliCommand
      .mockRejectedValueOnce(new GotestNotRequiredError(MODULE))
      .mockResolvedValueOnce({
        bin: "go",
        args: ["tool", MODULE, "scaffold", "./svc/user.go"],
      });
    h.readModuleRoots.mockResolvedValue([
      root("/ws"),
      root(path.join("/ws", "svc")),
    ]);
    h.showWarningMessage.mockResolvedValueOnce("Add to go.mod");
    h.captureStdout.mockResolvedValueOnce(
      "Generated: svc/user_suite_test.go\n",
    );

    await executeScaffold("./svc/user.go", log, discover, "/ws");

    expect(h.showWarningMessage).toHaveBeenCalledWith(
      `gotest: no go.mod here requires gotest. Add it to scaffold a suite? This runs: go get -tool ${MODULE}@latest`,
      "Add to go.mod",
    );
    expect(h.runGoInModule).toHaveBeenCalledWith(
      path.join("/ws", "svc"),
      ["get", "-tool", `${MODULE}@latest`],
      log,
    );
    expect(h.captureStdout).toHaveBeenCalledTimes(1);
    expect(h.openTextDocument).toHaveBeenCalledWith(
      path.join("/ws", "svc/user_suite_test.go"),
    );
    expect(h.showErrorMessage).not.toHaveBeenCalled();
  });

  it("does nothing when the offer is dismissed", async () => {
    h.buildCliCommand.mockRejectedValueOnce(new GotestNotRequiredError(MODULE));
    h.readModuleRoots.mockResolvedValue([root("/ws")]);
    h.showWarningMessage.mockResolvedValueOnce(undefined);
    await executeScaffold("./svc/user.go", log, discover, "/ws");
    expect(h.runGoInModule).not.toHaveBeenCalled();
    expect(h.captureStdout).not.toHaveBeenCalled();
    expect(h.showErrorMessage).not.toHaveBeenCalled();
  });

  it("asks once: a second refusal is reported, not asked again", async () => {
    h.buildCliCommand.mockRejectedValue(new GotestNotRequiredError(MODULE));
    h.readModuleRoots.mockResolvedValue([root("/ws")]);
    h.showWarningMessage.mockResolvedValueOnce("Add to go.mod");
    await executeScaffold("./a.go", log, discover, "/ws");
    expect(h.showWarningMessage).toHaveBeenCalledTimes(1);
    expect(h.showErrorMessage).toHaveBeenCalledWith(
      `gotest scaffold failed: no go.mod here requires gotest. Add it with: go get -tool ${MODULE}@latest`,
    );
  });

  it("says when there is no module to add gotest to", async () => {
    h.buildCliCommand.mockRejectedValueOnce(new GotestNotRequiredError(MODULE));
    h.readModuleRoots.mockResolvedValue([]);
    await executeScaffold("./a.go", log, discover, "/ws");
    expect(h.showWarningMessage).not.toHaveBeenCalled();
    expect(h.showErrorMessage).toHaveBeenCalledWith(
      "gotest scaffold failed: no go.mod in /ws",
    );
  });

  it("reports a go get that failed, and scaffolds nothing", async () => {
    h.buildCliCommand.mockRejectedValueOnce(new GotestNotRequiredError(MODULE));
    h.readModuleRoots.mockResolvedValue([root("/ws")]);
    h.showWarningMessage.mockResolvedValueOnce("Add to go.mod");
    h.runGoInModule.mockRejectedValueOnce(new Error("no network"));
    await executeScaffold("./a.go", log, discover, "/ws");
    expect(h.showErrorMessage).toHaveBeenCalledTimes(1);
    expect(h.showErrorMessage).toHaveBeenCalledWith(
      "gotest: adding gotest failed. no network",
    );
    expect(h.captureStdout).not.toHaveBeenCalled();
  });

  it("opens a generated file the CLI names by an absolute path", async () => {
    const abs = path.resolve("/ws", "svc", "user_suite_test.go");
    h.captureStdout.mockResolvedValueOnce(`Generated: ${abs}\n`);
    await executeScaffold("./svc/user.go", log, discover, "/ws");
    expect(h.openTextDocument).toHaveBeenCalledWith(abs);
  });
});

describe("ScaffoldCodeActionProvider", () => {
  const doc = (text: string) =>
    ({
      fileName: path.join("/ws", "user.go"),
      uri: { fsPath: path.join("/ws", "user.go") },
      lineCount: 1,
      lineAt: () => ({ text }),
    }) as never;
  const range = (line: number) => ({ start: { line } }) as never;

  beforeEach(() => {
    h.folderOf = { uri: { fsPath: "/ws" } };
  });

  it("offers no code action in a folder that does not use gotest", () => {
    const provider = new ScaffoldCodeActionProvider(() => false);
    expect(
      provider.provideCodeActions(doc("type User struct{}"), range(0)),
    ).toBeUndefined();
  });

  it("offers its code actions in a folder that does", () => {
    const provider = new ScaffoldCodeActionProvider(() => true);
    expect(
      provider.provideCodeActions(doc("type User struct{}"), range(0)),
    ).toHaveLength(2);
  });

  it("offers none for a file outside every workspace folder", () => {
    h.folderOf = undefined;
    const provider = new ScaffoldCodeActionProvider(() => true);
    expect(
      provider.provideCodeActions(doc("type User struct{}"), range(0)),
    ).toBeUndefined();
  });
});

describe("scaffoldTargetDir", () => {
  it.each([
    ["./pkg/path/file.go", "./pkg/path"],
    ["./pkg/path.TypeName", "./pkg/path"],
    ["./file.go", "."],
  ])("%s is in %s", (target, want) => {
    expect(scaffoldTargetDir(target)).toBe(want);
  });
});

describe("moduleRootFor", () => {
  const ws = path.resolve("/ws");
  const svc = path.join(ws, "svc");
  it("picks the deepest module that contains the directory", () => {
    expect(
      moduleRootFor(path.join(svc, "user"), [root(ws), root(svc)])?.dir,
    ).toBe(svc);
  });
  it("does not take a sibling directory that shares a prefix", () => {
    expect(
      moduleRootFor(path.join(ws, "svc2"), [root(ws), root(svc)])?.dir,
    ).toBe(ws);
    expect(moduleRootFor(path.join(ws, "svc2"), [root(svc)])).toBeUndefined();
  });
  it("finds the module of its own directory", () => {
    expect(moduleRootFor(svc, [root(svc)])?.dir).toBe(svc);
  });
  it("is undefined without a root", () => {
    expect(moduleRootFor(ws, [])).toBeUndefined();
  });
});
