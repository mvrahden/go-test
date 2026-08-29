// A test run's output is the developer's own text, and it arrives in whatever
// pieces the pipe delivers. Decoding each piece on its own would replace any
// character a read boundary splits with U+FFFD — in the captured stdout the
// spec view renders, and in the line events the Test Explorer maps to results.

import { describe, it, expect, vi } from "vitest";
import type { SpawnScript } from "./scriptedSpawn.test-support.js";

const { script } = vi.hoisted(() => ({
  script: { once: [], always: undefined } as SpawnScript,
}));

vi.mock("vscode", () => ({
  workspace: { getConfiguration: () => ({ get: () => 600 }) },
}));

vi.mock("node:child_process", async () => {
  const { createScriptedSpawn } =
    await import("./scriptedSpawn.test-support.js");
  return { spawn: createScriptedSpawn(script) };
});

import { spawnTestProcess } from "./runnerUtils.js";

const token = {
  isCancellationRequested: false,
  onCancellationRequested: () => ({ dispose: () => {} }),
};

const channel = {
  info: () => {},
  debug: () => {},
  warn: () => {},
  error: () => {},
};

function run(onStdoutLine?: (line: string) => void) {
  return spawnTestProcess(
    "go",
    ["test"],
    "/ws",
    token as never,
    channel as never,
    "test",
    undefined,
    onStdoutLine,
  );
}

describe("spawnTestProcess", () => {
  it("keeps a character whose bytes span two reads", async () => {
    const line = '{"Output":"café 日本語 🧪 passed\\n"}\n';
    const bytes = Buffer.from(line, "utf-8");
    const split = Buffer.byteLength(line.slice(0, line.indexOf("日"))) + 1;
    script.once.push({
      stdout: [bytes.subarray(0, split), bytes.subarray(split)],
    });

    const lines: string[] = [];
    const result = await run((l) => lines.push(l));

    expect(result.stdout).toBe(line);
    expect(lines).toEqual([line.trim()]);
  });

  it("reports the exit code and the stderr it saw", async () => {
    script.once.push({ stdout: ["ok\n"], stderr: "build failed", code: 2 });

    const result = await run();

    expect(result.exitCode).toBe(2);
    expect(result.stderr).toBe("build failed");
  });
});
