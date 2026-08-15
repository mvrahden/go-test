// A minimal but honest stand-in for the parts of the `vscode` API that the
// Spec View touches. Everything below the editor — CLI resolution, process
// spawning, stream handling — stays real, because that is the boundary the
// regression lived at.

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
}

export function createStubState(): StubState {
  return { workspaceDir: "", cliPath: "", autoRefresh: true, panels: [] };
}

export function buildVscodeStub(state: StubState) {
  return {
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
          if (key === "cliPath") return state.cliPath;
          if (key === "specView.autoRefresh") return state.autoRefresh;
          if (key === "buildTags") return "";
          if (key === "modulePath") return undefined;
          return fallback;
        },
      }),
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
    },
    ViewColumn: { One: 1, Beside: 2 },
    commands: { executeCommand: () => Promise.resolve() },
    env: { clipboard: { writeText: () => Promise.resolve() } },
    Range: class {
      constructor(
        public a: number,
        public b: number,
        public c: number,
        public d: number,
      ) {}
    },
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
