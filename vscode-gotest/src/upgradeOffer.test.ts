import { describe, it, expect, vi, beforeEach } from "vitest";

const { mockShowWarningMessage, mockShowErrorMessage, mockBuildCliCommand } =
  vi.hoisted(() => ({
    mockShowWarningMessage: vi.fn(async () => undefined),
    mockShowErrorMessage: vi.fn(async () => undefined),
    mockBuildCliCommand: vi.fn(),
  }));

vi.mock("vscode", () => ({
  workspace: {
    workspaceFolders: [{ uri: { fsPath: "/ws" } }],
    getConfiguration: () => ({ get: () => undefined }),
  },
  Uri: { file: (p: string) => ({ fsPath: p }) },
  CodeActionKind: { RefactorExtract: "refactor.extract" },
  window: {
    showWarningMessage: mockShowWarningMessage,
    showErrorMessage: mockShowErrorMessage,
    showInformationMessage: vi.fn(),
  },
}));

vi.mock("./cli.js", async (importOriginal) => ({
  // The error and the dialog are the real ones; only resolution is scripted.
  ...(await importOriginal<typeof import("./cli.js")>()),
  buildCliCommand: mockBuildCliCommand,
}));

import { CliBelowFloorError } from "./cli.js";
import { executeScaffold } from "./scaffold.js";
import { triageCrashers, type FuzzDeps } from "./fuzz.js";

function makeOutputChannel() {
  return {
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    debug: vi.fn(),
    show: vi.fn(),
  } as unknown as import("vscode").LogOutputChannel;
}

function fuzzDeps(): FuzzDeps {
  return {
    cache: { getWorkspaceDir: () => "/ws" },
    outputChannel: makeOutputChannel(),
    onSourceChanged: () => undefined,
  } as unknown as FuzzDeps;
}

const belowFloor = () =>
  new CliBelowFloorError("v1.31.1", "example.com/gotest", ["/ws"]);

// A command that fails below the floor answers with the upgrade dialog alone.
describe("a command below the floor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("scaffold shows the upgrade dialog and no error toast", async () => {
    mockBuildCliCommand.mockRejectedValueOnce(belowFloor());
    await executeScaffold("./pkg.T", makeOutputChannel(), () => undefined);
    expect(mockShowWarningMessage).toHaveBeenCalledTimes(1);
    expect(mockShowWarningMessage).toHaveBeenCalledWith(
      expect.stringContaining("pins gotest v1.31.1"),
      "Upgrade",
    );
    expect(mockShowErrorMessage).not.toHaveBeenCalled();
  });

  it("fuzz triage shows the upgrade dialog and no error toast", async () => {
    mockBuildCliCommand.mockRejectedValueOnce(belowFloor());
    await triageCrashers("example.com/a", fuzzDeps());
    expect(mockShowWarningMessage).toHaveBeenCalledTimes(1);
    expect(mockShowErrorMessage).not.toHaveBeenCalled();
  });

  it("any other failure still gets the command's own error toast", async () => {
    mockBuildCliCommand.mockRejectedValueOnce(new Error("boom"));
    await triageCrashers("example.com/a", fuzzDeps());
    mockBuildCliCommand.mockRejectedValueOnce(new Error("boom"));
    await executeScaffold("./pkg.T", makeOutputChannel(), () => undefined);
    expect(mockShowWarningMessage).not.toHaveBeenCalled();
    expect(mockShowErrorMessage).toHaveBeenCalledWith("gotest fuzz: boom");
    expect(mockShowErrorMessage).toHaveBeenCalledWith(
      "gotest scaffold failed: boom",
    );
  });
});
