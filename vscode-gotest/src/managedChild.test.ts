import { describe, it, expect, vi, beforeEach } from "vitest";
import type { SpawnScript } from "./scriptedSpawn.test-support.js";

const { script, mockKill } = vi.hoisted(() => ({
  script: { once: [], always: undefined, calls: [] } as SpawnScript,
  mockKill: vi.fn(),
}));

vi.mock("vscode", () => ({
  workspace: { getConfiguration: () => ({ get: () => undefined }) },
  Uri: { file: (p: string) => ({ fsPath: p }) },
}));

vi.mock("node:child_process", async () => {
  const { createScriptedSpawn } =
    await import("./scriptedSpawn.test-support.js");
  return { spawn: createScriptedSpawn(script, mockKill) };
});

import { ManagedChild } from "./managedChild.js";

// Callers always consume; an unread pipe would stall the child by design.
function drain(mc: ManagedChild): ManagedChild {
  mc.child.stdout?.on("data", () => {});
  mc.child.stderr?.on("data", () => {});
  return mc;
}

describe("ManagedChild", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    script.once = [];
    script.always = undefined;
    script.calls = [];
  });

  it("settles on close for a child that finishes on its own", async () => {
    script.always = { stdout: ["done\n"], code: 0 };
    const mc = drain(new ManagedChild("go", ["run", "."]));
    await expect(mc.exited).resolves.toMatchObject({ code: 0 });
    expect(mockKill).not.toHaveBeenCalled();
  });

  it("escalates to SIGKILL when the child ignores SIGTERM", async () => {
    script.always = { neverExits: true, ignoresSigterm: true };
    const mc = drain(new ManagedChild("go", ["run", "."]));

    const done = mc.terminate("prompt");
    expect(mockKill).toHaveBeenCalledWith("SIGTERM");
    await vi.advanceTimersByTimeAsync(5_000);
    expect(mockKill).toHaveBeenCalledWith("SIGKILL");
    await done;
  });

  // `go run` exits while the binary it compiled keeps the pipes open, so
  // `close` never arrives. Settling on `exit` is what stops that wedging us.
  it("settles when the child exits but the pipes stay open", async () => {
    script.always = { neverExits: true, holdsPipesAfterExit: true };
    const mc = drain(new ManagedChild("go", ["run", "."]));
    await expect(mc.terminate("prompt")).resolves.toBeDefined();
  });

  it("is idempotent and does not re-arm the escalation", async () => {
    script.always = { neverExits: true, ignoresSigterm: true };
    const mc = drain(new ManagedChild("go", ["run", "."]));

    const first = mc.terminate("prompt");
    mc.terminate("prompt");
    mc.terminate("teardown");
    await vi.advanceTimersByTimeAsync(5_000);
    await first;

    const kills = mockKill.mock.calls.map((c) => c[0]);
    expect(kills.filter((s) => s === "SIGTERM")).toHaveLength(1);
    expect(kills.filter((s) => s === "SIGKILL")).toHaveLength(1);
  });

  // The bug the first capture.ts fix shipped with: the escalation was armed
  // after the signal, so a child that died inside kill() left a SIGKILL
  // scheduled against a process that no longer existed.
  it("never signals after the child has already gone", async () => {
    script.always = { neverExits: true };
    const mc = drain(new ManagedChild("go", ["run", "."]));

    await mc.terminate("prompt");
    mockKill.mockClear();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(mockKill).not.toHaveBeenCalled();
  });

  it("resolves immediately when terminating an already-dead child", async () => {
    script.always = { stdout: [], code: 0 };
    const mc = drain(new ManagedChild("go", ["run", "."]));
    await mc.exited;

    await expect(mc.terminate("teardown")).resolves.toBeDefined();
    expect(mockKill).not.toHaveBeenCalled();
  });
});
