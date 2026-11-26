// The file watchers and the activation steps, driven through the functions
// activate wires them with: a watcher or a call that is dropped fails here.

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import * as os from "node:os";
import * as path from "node:path";

type Handler = (uri: { fsPath: string }) => void;

const h = vi.hoisted(() => ({
  watchers: new Map<string, Record<string, Handler>>(),
  executeCommand: vi.fn(async (..._args: unknown[]) => undefined),
  folder: "/ws",
  writeGitSettings: vi.fn(),
}));

// Modules read a few vscode values as they load; those get a stand-in, and
// only what the wiring calls is real.
vi.mock("vscode", () => {
  const standIn: unknown = new Proxy(function () {}, {
    get: (_, key) => (key === "then" ? undefined : standIn),
    apply: () => standIn,
    construct: () => standIn as object,
  });
  const on =
    (handlers: Record<string, Handler>, name: string) => (cb: Handler) => {
      handlers[name] = cb;
      return { dispose: () => {} };
    };
  return {
    CodeActionKind: standIn,
    Uri: { file: (p: string) => ({ fsPath: p }) },
    TestRunRequest: class {},
    commands: {
      registerCommand: () => ({ dispose: () => {} }),
      executeCommand: h.executeCommand,
    },
    window: { showErrorMessage: vi.fn() },
    workspace: {
      getConfiguration: () => ({ get: () => undefined }),
      getWorkspaceFolder: () => ({ uri: { fsPath: h.folder } }),
      createFileSystemWatcher: (pattern: string) => {
        const handlers: Record<string, Handler> = {};
        h.watchers.set(pattern, handlers);
        return {
          onDidChange: on(handlers, "change"),
          onDidCreate: on(handlers, "create"),
          onDidDelete: on(handlers, "delete"),
          dispose: () => {},
        };
      },
    },
  };
});

vi.mock("./cli.js", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./cli.js")>()),
  validateGoBinary: vi.fn(async () => "go"),
  clearBinaryCache: vi.fn(),
}));

vi.mock("./activeFolders.js", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./activeFolders.js")>()),
  writeGitSettings: h.writeGitSettings,
}));

import { initializeAsync, setupFileWatchers } from "./extension.js";
import { ActiveFolders } from "./activeFolders.js";

const REQUIRING =
  "module example.com/app\n\ngo 1.26.0\n\nrequire github.com/mvrahden/go-test v1.38.0\n";
const PLAIN = "module example.com/app\n\ngo 1.26.0\n";
const log = { info: vi.fn(), debug: vi.fn(), warn: vi.fn(), error: vi.fn() };

let dirs: string[] = [];
function folderWith(goMod: string): string {
  const dir = mkdtempSync(path.join(os.tmpdir(), "gotest-wiring-"));
  writeFileSync(path.join(dir, "go.mod"), goMod);
  dirs.push(dir);
  return dir;
}

beforeEach(() => {
  h.watchers.clear();
  h.executeCommand.mockClear();
  h.writeGitSettings.mockClear();
});
afterEach(() => {
  for (const d of dirs) rmSync(d, { recursive: true, force: true });
  dirs = [];
});

describe("the file watchers", () => {
  function wire(active = new ActiveFolders()) {
    const controller = { invalidateResults: vi.fn() };
    const discovery = {
      discover: vi.fn(async () => {}),
      discoverPackage: vi.fn(async () => {}),
    };
    const disposables = setupFileWatchers(
      { resolveFileToPackage: () => "example.com/cart" } as never,
      discovery as never,
      { invalidate: () => false, save: () => {} } as never,
      { run: vi.fn() } as never,
      log as never,
      active,
      controller as never,
    );
    return { controller, discovery, disposables };
  }
  const fire = (pattern: string, event: string, fsPath: string) => {
    const handler = h.watchers.get(pattern)?.[event];
    expect(handler, `${pattern} ${event}`).toBeDefined();
    handler!({ fsPath });
  };

  it.each([
    ["**/*_test.go", "change", "/ws/cart/cart_test.go"],
    ["**/*_test.go", "create", "/ws/cart/cart_test.go"],
    ["**/*_test.go", "delete", "/ws/cart/cart_test.go"],
    ["**/*.go", "change", "/ws/cart/cart.go"],
    ["**/*.go", "create", "/ws/cart/cart.go"],
    ["**/*.go", "delete", "/ws/cart/cart.go"],
  ])("marks the package outdated on a %s %s", (pattern, event, file) => {
    const { controller } = wire();
    fire(pattern, event, file);
    expect(controller.invalidateResults).toHaveBeenCalledWith(
      "example.com/cart",
    );
  });

  it.each(["change", "create", "delete"])(
    "follows a go.mod %s: the folder is activated, its git settings written, the context set and the folder rediscovered",
    async (event) => {
      const dir = folderWith(REQUIRING);
      h.folder = dir;
      const { discovery, disposables } = wire();
      try {
        fire("**/{go.mod,go.work}", event, path.join(dir, "go.mod"));
        await vi.waitFor(() =>
          expect(discovery.discover).toHaveBeenCalledWith(dir),
        );
        expect(h.writeGitSettings).toHaveBeenCalledWith(dir, log);
        expect(h.executeCommand).toHaveBeenCalledWith(
          "setContext",
          "gotest.active",
          true,
        );
      } finally {
        for (const d of disposables) d.dispose();
        h.folder = "/ws";
      }
    },
  );
});

describe("activation", () => {
  function deps(folders: string[], active: ActiveFolders) {
    const subscriptions: { dispose(): void }[] = [];
    return {
      subscriptions,
      deps: {
        workspaceFolders: folders.map((f) => ({ uri: { fsPath: f } })),
        outputChannel: log,
        discoveryService: { discover: vi.fn(async () => {}) },
        coverageStore: { load: async () => {}, size: 0 },
        testResultStore: { load: async () => {} },
        controller: {},
        cache: { packages: [] },
        runRegistry: {
          load: async () => {},
          sweepStale: () => [],
          sweep: () => {},
          save: async () => {},
        },
        processRegistry: { load: async () => {}, inheritedSize: 0 },
        context: { subscriptions },
        snapshotStore: { load: async () => {}, get: () => undefined },
        panelHost: { load: async () => {} },
        active,
      } as never,
    };
  }

  it("writes git settings in the folders that use gotest only, and sets the context", async () => {
    const using = folderWith(REQUIRING);
    const plain = folderWith(PLAIN);
    const { deps: d, subscriptions } = deps(
      [using, plain],
      new ActiveFolders(),
    );
    try {
      await initializeAsync(d);
      expect(h.writeGitSettings.mock.calls.map((c) => c[0])).toEqual([using]);
      expect(h.executeCommand).toHaveBeenCalledWith(
        "setContext",
        "gotest.active",
        true,
      );
    } finally {
      for (const s of subscriptions) s.dispose();
    }
  });

  it("leaves the context off and writes nothing where no folder uses gotest", async () => {
    const plain = folderWith(PLAIN);
    const { deps: d, subscriptions } = deps([plain], new ActiveFolders());
    try {
      await initializeAsync(d);
      expect(h.writeGitSettings).not.toHaveBeenCalled();
      expect(h.executeCommand).toHaveBeenCalledWith(
        "setContext",
        "gotest.active",
        false,
      );
    } finally {
      for (const s of subscriptions) s.dispose();
    }
  });
});
