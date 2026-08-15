// A minimal but honest stand-in for the parts of the `vscode` API the extension
// touches. Everything below the editor — CLI resolution, process spawning,
// stream handling, result mapping — stays real, because that is the boundary
// the regressions live at.

import {
  FakeTestController,
  FakeEventEmitter,
  FakeCancellationTokenSource,
} from "./vscodeTestApi.js";

export interface StubPanel {
  webview: {
    html: string;
    cspSource: string;
    onDidReceiveMessage: () => void;
    asWebviewUri: (u: unknown) => unknown;
  };
  onDidDispose: () => void;
  reveal: () => void;
  dispose: () => void;
  iconPath?: unknown;
  viewColumn: number;
}

export interface StubState {
  workspaceDir: string;
  cliPath: string;
  autoRefresh: boolean;
  panels: StubPanel[];
  controllers?: unknown[];
  config?: Record<string, unknown>;
}

export function createStubState(): StubState {
  return { workspaceDir: "", cliPath: "", autoRefresh: true, panels: [] };
}

export function buildVscodeStub(state: StubState) {
  return {
    // The testing API the extension writes results into. Real collections and
    // items, so assertions can read the tree rather than count calls.
    tests: {
      createTestController: (id: string, label: string) => {
        const controller = new FakeTestController(id, label);
        (state.controllers ??= []).push(controller);
        return controller;
      },
    },
    TestRunProfileKind: { Run: 1, Debug: 2, Coverage: 3 },
    TestTag: class {
      constructor(public id: string) {}
    },
    TestMessage: class {
      constructor(public message: string) {}
    },
    TestRunRequest: class {
      constructor(
        public include?: unknown[],
        public exclude?: unknown[],
        public profile?: unknown,
      ) {}
    },
    EventEmitter: FakeEventEmitter,
    CancellationTokenSource: FakeCancellationTokenSource,
    Position: class {
      constructor(
        public line: number,
        public character: number,
      ) {}
    },
    // The real Range accepts either two Positions or four numbers; the
    // extension uses both forms, so keep the parameters untyped.
    Range: class {
      constructor(
        public start: unknown,
        public end?: unknown,
        public third?: unknown,
        public fourth?: unknown,
      ) {}
    },
    Location: class {
      constructor(
        public uri: unknown,
        public range: unknown,
      ) {}
    },
    // Coverage value types. Real classes rather than spies, so a test can read
    // the covered file and its statement counts back out.
    StatementCoverage: class {
      constructor(
        public executed: number | boolean,
        public location: unknown,
      ) {}
    },
    DeclarationCoverage: class {
      constructor(
        public name: string,
        public executed: number | boolean,
        public location: unknown,
      ) {}
    },
    TestCoverageCount: class {
      constructor(
        public covered: number,
        public total: number,
      ) {}
    },
    FileCoverage: class {
      declarationCoverage: unknown;
      constructor(
        public uri: unknown,
        public statementCoverage: unknown,
      ) {}
    },
    StatusBarAlignment: { Left: 1, Right: 2 },
    RelativePattern: class {
      constructor(
        public base: unknown,
        public pattern: string,
      ) {}
    },
    Uri: {
      file: (p: string) => ({ fsPath: p, toString: () => p }),
      joinPath: (base: { fsPath?: string }, ...parts: string[]) => ({
        fsPath: [base?.fsPath ?? "", ...parts].join("/"),
        toString: () => [base?.fsPath ?? "", ...parts].join("/"),
      }),
    },
    workspace: {
      get workspaceFolders() {
        return [{ uri: { fsPath: state.workspaceDir } }];
      },
      getConfiguration: () => ({
        get: (key: string, fallback?: unknown) => {
          if (state.config && key in state.config) return state.config[key];
          if (key === "cliPath") return state.cliPath;
          if (key === "specView.autoRefresh") return state.autoRefresh;
          if (key === "buildTags") return "";
          if (key === "modulePath") return undefined;
          return fallback;
        },
      }),
      getWorkspaceFolder: () => ({
        uri: { fsPath: state.workspaceDir },
        name: "fixtures",
        index: 0,
      }),
      createFileSystemWatcher: () => ({
        onDidCreate: () => ({ dispose: () => {} }),
        onDidChange: () => ({ dispose: () => {} }),
        onDidDelete: () => ({ dispose: () => {} }),
        dispose: () => {},
      }),
      onDidChangeWorkspaceFolders: () => ({ dispose: () => {} }),
      onDidSaveTextDocument: () => ({ dispose: () => {} }),
    },
    window: {
      createWebviewPanel: (): StubPanel => {
        const panel: StubPanel = {
          webview: {
            html: "",
            cspSource: "vscode-resource:",
            onDidReceiveMessage: () => {},
            asWebviewUri: (u: unknown) => u,
          },
          onDidDispose: () => {},
          reveal: () => {},
          dispose: () => {},
          viewColumn: 2,
        };
        state.panels.push(panel);
        return panel;
      },
      visibleTextEditors: [],
      showTextDocument: () => Promise.resolve(),
      showWarningMessage: () => Promise.resolve(undefined),
      showErrorMessage: () => Promise.resolve(undefined),
      showInformationMessage: () => Promise.resolve(undefined),
      createStatusBarItem: () => ({
        text: "",
        tooltip: "",
        command: "",
        show: () => {},
        hide: () => {},
        dispose: () => {},
      }),
      withProgress: (_opts: unknown, task: () => Promise<unknown>) => task(),
    },
    ViewColumn: { One: 1, Beside: 2 },
    commands: { executeCommand: () => Promise.resolve() },
    env: { clipboard: { writeText: () => Promise.resolve() } },
  };
}

// Captures what the extension reported, so a test can assert that a run
// produced no diagnostics — the regression was silent apart from one logged
// error, and asserting on that absence is what makes it visible.
export function createRecordingChannel() {
  const errors: string[] = [];
  const warnings: string[] = [];
  return {
    errors,
    warnings,
    channel: {
      info: () => {},
      debug: () => {},
      warn: (m: string) => warnings.push(m),
      error: (m: string) => errors.push(m),
    },
  };
}
