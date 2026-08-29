// A budget that cannot end the process it is bounding is not a budget. These
// cover the two ways a child outlives its SIGTERM.

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

vi.mock("./cli.js", () => ({
  stripGoRunExitEcho: (s: string) => s,
}));

vi.mock("node:child_process", async () => {
  const { createScriptedSpawn } =
    await import("./scriptedSpawn.test-support.js");
  return { spawn: createScriptedSpawn(script, mockKill) };
});

import { captureStdout, CaptureTimeoutError } from "./capture.js";

describe("captureStdout budgets", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    script.once = [];
    script.always = undefined;
    script.calls = [];
  });

  it("escalates to SIGKILL when the child ignores SIGTERM", async () => {
    script.always = { neverExits: true, ignoresSigterm: true };

    const p = captureStdout("go", ["run", "."], { timeoutSeconds: 120 });
    const settled = expect(p).rejects.toBeInstanceOf(CaptureTimeoutError);

    await vi.advanceTimersByTimeAsync(120_000);
    expect(mockKill).toHaveBeenCalledWith("SIGTERM");
    // Still alive: without the escalation below, this is where discovery used
    // to hang for the rest of the session.
    await vi.advanceTimersByTimeAsync(5_000);
    expect(mockKill).toHaveBeenCalledWith("SIGKILL");

    await settled;
  });

  // `go run` exits while the binary it compiled keeps the pipes open, so
  // `close` never arrives. Settling on `exit` is what stops that wedging us.
  it("settles when the child exits but the pipes stay open", async () => {
    script.always = { neverExits: true, holdsPipesAfterExit: true };

    const p = captureStdout("go", ["run", "."], { timeoutSeconds: 120 });
    const settled = expect(p).rejects.toThrow("timed out after 120s");
    await vi.advanceTimersByTimeAsync(120_000);
    await settled;
  });

  it("does not signal a child that finished on its own", async () => {
    script.always = { stdout: ['{"packages":[]}'], code: 0 };

    await expect(
      captureStdout("go", ["run", "."], { timeoutSeconds: 120 }),
    ).resolves.toBe('{"packages":[]}');
    await vi.advanceTimersByTimeAsync(200_000);
    expect(mockKill).not.toHaveBeenCalled();
  });
});
