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

import { ManagedChild, withControl } from "./managedChild.js";
import type { FakeChild } from "./scriptedSpawn.test-support.js";

function written(mc: ManagedChild): string[] {
  return (mc.child as unknown as FakeChild).written;
}

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

  // Detaching is what makes group signalling possible — and on Windows there is
  // no group path (killProcessTree falls back to child.kill) and no registry to
  // reap with, so it would only buy a child that outlives the extension host.
  it("detaches only where the group can actually be signalled", async () => {
    script.always = { stdout: [], code: 0 };
    drain(new ManagedChild("go", ["run", "."]));
    expect(script.calls?.[0].opts?.detached).toBe(process.platform !== "win32");
  });

  it("resolves immediately when terminating an already-dead child", async () => {
    script.always = { stdout: [], code: 0 };
    const mc = drain(new ManagedChild("go", ["run", "."]));
    await mc.exited;

    await expect(mc.terminate("teardown")).resolves.toBeDefined();
    expect(mockKill).not.toHaveBeenCalled();
  });

  describe("with control", () => {
    // forceKillTimeout's default: the vscode mock has no setting.
    const FORCE_AT_MS = 360_000;

    it("starts the CLI with --control=stdin, before a bare --", () => {
      script.always = { neverExits: true };
      drain(
        new ManagedChild("go", ["tool", "gotest", "watch", "--", "-json"], {
          control: true,
        }),
      );
      drain(
        new ManagedChild("gotest", ["fuzz", "triage", "./pkg"], {
          control: true,
        }),
      );
      expect(script.calls?.map((c) => c.args)).toEqual([
        ["tool", "gotest", "watch", "--control=stdin", "--", "-json"],
        ["fuzz", "triage", "./pkg", "--control=stdin"],
      ]);
      expect(withControl(["a", "-args", "--", "b"])).toEqual([
        "a",
        "--control=stdin",
        "-args",
        "--",
        "b",
      ]);
    });

    it("stops a teardown by stop, forces it at the timeout, kills it 5s later", async () => {
      script.always = { neverExits: true, ignoresSigterm: true };
      const mc = drain(new ManagedChild("go", ["run", "."], { control: true }));

      const done = mc.terminate("teardown");
      expect(written(mc)).toEqual(["stop\n"]);
      expect(mockKill).not.toHaveBeenCalled();

      await vi.advanceTimersByTimeAsync(FORCE_AT_MS - 1);
      expect(written(mc)).toEqual(["stop\n"]);
      await vi.advanceTimersByTimeAsync(1);
      expect(written(mc)).toEqual(["stop\n", "force\n"]);
      expect(mockKill).not.toHaveBeenCalled();

      await vi.advanceTimersByTimeAsync(4_999);
      expect(mockKill).not.toHaveBeenCalled();
      await vi.advanceTimersByTimeAsync(1);
      expect(mockKill.mock.calls).toEqual([["SIGKILL"]]);
      await done;
    });

    it("writes nothing more once the CLI has ended", async () => {
      script.always = { neverExits: true };
      const mc = drain(new ManagedChild("go", ["run", "."], { control: true }));
      const done = mc.terminate("teardown");
      mc.child.emit("exit", 130, null);
      await done;
      await vi.advanceTimersByTimeAsync(FORCE_AT_MS + 10_000);
      expect(written(mc)).toEqual(["stop\n"]);
      expect(mockKill).not.toHaveBeenCalled();
    });

    it("keeps the signal path for a prompt", async () => {
      script.always = { neverExits: true, ignoresSigterm: true };
      const mc = drain(new ManagedChild("go", ["run", "."], { control: true }));
      const done = mc.terminate("prompt");
      expect(mockKill).toHaveBeenCalledWith("SIGTERM");
      await vi.advanceTimersByTimeAsync(5_000);
      expect(mockKill).toHaveBeenCalledWith("SIGKILL");
      await done;
      expect(written(mc)).toEqual([]);
    });

    it("does not throw on a closed stdin, and still kills at the end", async () => {
      script.always = { neverExits: true, ignoresSigterm: true };
      const mc = drain(new ManagedChild("go", ["run", "."], { control: true }));
      mc.child.stdin?.destroy();

      const done = mc.terminate("teardown");
      await vi.advanceTimersByTimeAsync(FORCE_AT_MS + 5_000);
      expect(written(mc)).toEqual([]);
      expect(mockKill.mock.calls).toEqual([["SIGKILL"]]);
      await done;
    });

    it("does not throw when stdin fails under a write", async () => {
      script.always = { neverExits: true, ignoresSigterm: true };
      const mc = drain(new ManagedChild("go", ["run", "."], { control: true }));
      const stdin = mc.child.stdin!;
      stdin.write = (() => {
        throw new Error("EPIPE");
      }) as typeof stdin.write;
      expect(() => mc.terminate("teardown")).not.toThrow();
      stdin.emit("error", new Error("EPIPE"));
      await vi.advanceTimersByTimeAsync(FORCE_AT_MS + 5_000);
      expect(mockKill.mock.calls).toEqual([["SIGKILL"]]);
    });

    it("asks a child to stop by its stdin, and any other by SIGTERM", () => {
      script.always = { neverExits: true };
      const control = drain(
        new ManagedChild("go", ["run", "."], { control: true }),
      );
      control.requestStop();
      expect(written(control)).toEqual(["stop\n"]);
      expect(mockKill).not.toHaveBeenCalled();

      const plain = drain(new ManagedChild("go", ["run", "."]));
      plain.requestStop();
      expect(written(plain)).toEqual([]);
      expect(mockKill).toHaveBeenCalledWith("SIGTERM");
    });

    it("never writes to a child started without control", async () => {
      script.always = { neverExits: true, ignoresSigterm: true };
      const mc = drain(new ManagedChild("go", ["run", "."]));
      const done = mc.terminate("teardown");
      expect(mockKill).toHaveBeenCalledWith("SIGTERM");
      await vi.advanceTimersByTimeAsync(FORCE_AT_MS);
      expect(mockKill).toHaveBeenCalledWith("SIGKILL");
      await done;
      expect(written(mc)).toEqual([]);
      expect(script.calls?.[0].args).toEqual(["run", "."]);
    });
  });
});
