import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { SpawnScript, FakeChild } from "./scriptedSpawn.test-support.js";

const h = vi.hoisted(() => ({
  script: { once: [], calls: [] } as SpawnScript,
  children: [] as FakeChild[],
  config: {} as Record<string, unknown>,
  sessionEnd: undefined as ((s: unknown) => void) | undefined,
  listener: { dispose: () => {} },
}));

vi.mock("vscode", () => {
  class CancellationTokenSource {
    private listeners: (() => void)[] = [];
    token = {
      isCancellationRequested: false,
      onCancellationRequested: (fn: () => void) => {
        this.listeners.push(fn);
        return { dispose: () => {} };
      },
    };
    cancel() {
      this.token.isCancellationRequested = true;
      for (const fn of this.listeners) fn();
    }
    dispose() {}
  }
  return {
    CancellationTokenSource,
    Uri: { file: (p: string) => ({ fsPath: p }) },
    workspace: {
      getWorkspaceFolder: (uri: { fsPath: string }) =>
        uri.fsPath.startsWith("/ws/") ? { uri: { fsPath: "/ws" } } : undefined,
      getConfiguration: () => ({
        get: (k: string, d?: unknown) => h.config[k] ?? d,
      }),
    },
    window: { showErrorMessage: vi.fn() },
    debug: {
      startDebugging: vi.fn(async () => true),
      onDidTerminateDebugSession: (fn: (s: unknown) => void) => {
        h.sessionEnd = fn;
        return h.listener;
      },
    },
  };
});

vi.mock("node:child_process", async () => {
  const { createScriptedSpawn } =
    await import("./scriptedSpawn.test-support.js");
  const spawn = createScriptedSpawn(h.script);
  return {
    spawn: (bin: string, args: string[], opts?: Record<string, unknown>) => {
      const child = spawn(bin, args, opts);
      const kill = child.kill;
      child.kill = vi.fn((signal?: NodeJS.Signals) => kill(signal));
      h.children.push(child);
      return child;
    },
  };
});

vi.mock("./cli.js", () => ({
  buildCliCommand: vi.fn(async (args: string[]) => ({ bin: "gotest", args })),
  formatCliCommand: (cmd: { bin: string; args: string[] }) =>
    `${cmd.bin} ${cmd.args.join(" ")}`,
  offerCliUpgrade: vi.fn(() => false),
  scopedConfig: () => ({
    get: (k: string, d?: unknown) => h.config[k] ?? d,
  }),
}));

import * as vscode from "vscode";
import { DebugLauncher } from "./debug.js";
import { buildCliCommand, offerCliUpgrade } from "./cli.js";

const startDebugging = vi.mocked(vscode.debug.startDebugging);
const showError = vi.mocked(vscode.window.showErrorMessage);
const log = { info: vi.fn(), debug: vi.fn() } as any;

const PKG = "/ws/pkg";
const READY = '{"overlayFile":"/tmp/overlay.json","dir":"/ws/pkg"}\n';

// Ready, then holding fixtures until it is told to stop.
function daemon(line = READY) {
  return { stdout: [line], neverExits: true };
}

function debugItems(
  launcher: DebugLauncher,
  token: vscode.CancellationToken,
  opts: { filter?: string; dir?: string | undefined } = {},
) {
  return launcher.debug(
    { include: [{ id: "item" }] } as any,
    token,
    () => opts.filter,
    () => ("dir" in opts ? opts.dir : PKG),
  );
}

function token() {
  return new vscode.CancellationTokenSource().token;
}

// prepare runs with --control=stdin: it is stopped by a line on its stdin,
// never by a signal.
function stopped(i: number): boolean {
  const child = h.children[i];
  if (child.written.includes("stop\n")) {
    expect(child.kill).not.toHaveBeenCalled();
    return true;
  }
  return false;
}

function launchedConfig(): any {
  return startDebugging.mock.calls[0][1];
}

describe("DebugLauncher", () => {
  let launcher: DebugLauncher;

  beforeEach(() => {
    vi.clearAllMocks();
    startDebugging.mockResolvedValue(true);
    h.script.once = [];
    h.script.always = undefined;
    h.script.calls = [];
    h.children = [];
    h.config = {};
    h.sessionEnd = undefined;
    h.listener = { dispose: vi.fn() };
    launcher = new DebugLauncher(log);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("launches nothing without a selection", async () => {
    await launcher.debug(
      { include: [] } as any,
      token(),
      () => "",
      () => PKG,
    );
    await launcher.debug(
      {} as any,
      token(),
      () => "",
      () => PKG,
    );
    expect(h.script.calls).toHaveLength(0);
    expect(startDebugging).not.toHaveBeenCalled();
    expect(showError).not.toHaveBeenCalled();
  });

  it("launches nothing for an item without a package dir", async () => {
    await debugItems(launcher, token(), { dir: undefined });
    expect(h.script.calls).toHaveLength(0);
    expect(startDebugging).not.toHaveBeenCalled();
    expect(showError).not.toHaveBeenCalled();
  });

  it("launches nothing for a package outside every workspace folder", async () => {
    await debugItems(launcher, token(), { dir: "/elsewhere/pkg" });
    expect(h.script.calls).toHaveLength(0);
    expect(startDebugging).not.toHaveBeenCalled();
    expect(showError).not.toHaveBeenCalled();
  });

  it("launches the Go debugger on the prepared overlay", async () => {
    h.config = { buildFlags: ["-tags=integration", "-race"] };
    h.script.once = [
      daemon(
        '{"overlayFile":"/tmp/overlay.json","dir":"/ws/pkg","stateFile":"/tmp/state.json"}\nignored\n',
      ),
    ];
    await debugItems(launcher, token(), { filter: "^TestS$/^TestM$" });

    expect(buildCliCommand).toHaveBeenCalledWith(["prepare", PKG], "/ws", log);
    expect(h.script.calls![0]).toMatchObject({
      bin: "gotest",
      args: ["prepare", PKG, "--control=stdin"],
      opts: { cwd: "/ws" },
    });
    expect(startDebugging).toHaveBeenCalledWith(
      { uri: { fsPath: "/ws" } },
      expect.objectContaining({
        type: "go",
        request: "launch",
        mode: "test",
        program: PKG,
        buildFlags: "-overlay=/tmp/overlay.json -tags=integration -race",
        args: ["-test.run", "^TestS$/^TestM$"],
        env: { GOTEST_SHARED_STATE_FILE: "/tmp/state.json" },
        __prepareKey: `gotest-prepare:${PKG}`,
      }),
    );
    expect(h.children[0].kill).not.toHaveBeenCalled();
    expect(stopped(0)).toBe(false);
  });

  it("passes no run filter and no env when there is neither", async () => {
    h.script.once = [daemon()];
    await debugItems(launcher, token());
    const cfg = launchedConfig();
    expect(cfg.args).toEqual([]);
    expect(cfg.buildFlags).toBe("-overlay=/tmp/overlay.json");
    expect(cfg).not.toHaveProperty("env");
  });

  it("reads the ready line across stdout chunks", async () => {
    h.script.once = [
      {
        stdout: ['{"overlayFile":"/tmp/split', '.json","dir":"/ws/pkg"}\n'],
        neverExits: true,
      },
    ];
    await debugItems(launcher, token());
    expect(launchedConfig().buildFlags).toBe("-overlay=/tmp/split.json");
  });

  it("reports a ready line that is not JSON and stops prepare", async () => {
    h.script.once = [daemon("not json\n")];
    await debugItems(launcher, token());
    expect(showError).toHaveBeenCalledWith(
      expect.stringContaining("Failed to parse prepare output"),
    );
    expect(stopped(0)).toBe(true);
    expect(startDebugging).not.toHaveBeenCalled();
  });

  it("reports prepare exiting before ready with its stderr", async () => {
    h.script.once = [{ stderr: "pkg/x.go:3: undefined: y\n", code: 2 }];
    await debugItems(launcher, token());
    expect(showError).toHaveBeenCalledWith(
      expect.stringContaining(
        "prepare exited with code 2 before ready:\npkg/x.go:3: undefined: y",
      ),
    );
    expect(startDebugging).not.toHaveBeenCalled();
  });

  it("gives up on prepare after the configured timeout", async () => {
    vi.useFakeTimers();
    h.config = { "debug.prepareTimeout": 3 };
    h.script.once = [{ neverExits: true }];
    const done = debugItems(launcher, token());
    await vi.advanceTimersByTimeAsync(2_999);
    expect(showError).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    await done;
    expect(showError).toHaveBeenCalledWith(
      expect.stringContaining("timed out after 3s"),
    );
    expect(stopped(0)).toBe(true);
  });

  it("stops prepare when the request is cancelled", async () => {
    h.script.once = [{ neverExits: true }];
    const cts = new vscode.CancellationTokenSource();
    const done = debugItems(launcher, cts.token);
    await vi.waitFor(() => expect(h.children).toHaveLength(1));
    cts.cancel();
    await done;
    expect(showError).toHaveBeenCalledWith(
      expect.stringContaining("cancelled"),
    );
    expect(stopped(0)).toBe(true);
    expect(startDebugging).not.toHaveBeenCalled();
  });

  it("leaves a CLI below the floor to the upgrade offer", async () => {
    vi.mocked(buildCliCommand).mockRejectedValueOnce(new Error("too old"));
    vi.mocked(offerCliUpgrade).mockReturnValueOnce(true);
    await debugItems(launcher, token());
    expect(h.script.calls).toHaveLength(0);
    expect(startDebugging).not.toHaveBeenCalled();
    expect(showError).not.toHaveBeenCalled();
  });

  it("stops prepare when the debugger does not start", async () => {
    startDebugging.mockResolvedValueOnce(false);
    h.script.once = [daemon()];
    await debugItems(launcher, token());
    expect(stopped(0)).toBe(true);
  });

  it("stops the previous prepare of a package relaunched", async () => {
    h.script.once = [daemon(), daemon()];
    await debugItems(launcher, token());
    await debugItems(launcher, token());
    expect(stopped(0)).toBe(true);
    expect(stopped(1)).toBe(false);
  });

  it("stops the prepare of the session that ended, and no other", async () => {
    const context = { subscriptions: [] as unknown[] };
    launcher.registerCleanupOnSessionEnd(context as any);
    expect(context.subscriptions).toContain(h.listener);

    h.script.once = [daemon()];
    await debugItems(launcher, token());

    h.sessionEnd!({ configuration: {} });
    h.sessionEnd!({
      configuration: { __prepareKey: "gotest-prepare:/ws/other" },
    });
    expect(stopped(0)).toBe(false);

    h.sessionEnd!({ configuration: { __prepareKey: `gotest-prepare:${PKG}` } });
    expect(stopped(0)).toBe(true);
  });

  it("stops every prepare and the listener on dispose", async () => {
    launcher.registerCleanupOnSessionEnd({ subscriptions: [] } as any);
    h.script.once = [daemon(), daemon()];
    await debugItems(launcher, token());
    await debugItems(launcher, token(), { dir: "/ws/two" });

    launcher.dispose();
    expect(stopped(0)).toBe(true);
    expect(stopped(1)).toBe(true);
    expect(h.listener.dispose).toHaveBeenCalled();
  });

  it("launches an explicit pattern", async () => {
    h.script.once = [daemon()];
    await launcher.debugPattern(PKG, "^FuzzS_FuzzX$");
    expect(h.script.calls![0].args).toEqual([
      "prepare",
      PKG,
      "--control=stdin",
    ]);
    expect(launchedConfig()).toMatchObject({
      program: PKG,
      args: ["-test.run", "^FuzzS_FuzzX$"],
    });
  });
});
