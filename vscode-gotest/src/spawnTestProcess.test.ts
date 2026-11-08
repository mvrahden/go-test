// A test run's output is the developer's own text, and it arrives in whatever
// pieces the pipe delivers. Decoding each piece on its own would replace any
// character a read boundary splits with U+FFFD — in the captured stdout the
// spec view renders, and in the line events the Test Explorer maps to results.

import { describe, it, expect, vi } from "vitest";
import type { SpawnScript, FakeChild } from "./scriptedSpawn.test-support.js";

const { script, children } = vi.hoisted(() => ({
  script: { once: [], always: undefined, calls: [] } as SpawnScript,
  children: [] as FakeChild[],
}));

vi.mock("vscode", () => ({
  workspace: { getConfiguration: () => ({ get: () => 600 }) },
  Uri: { file: (p: string) => ({ fsPath: p }) },
}));

vi.mock("node:child_process", async () => {
  const { createScriptedSpawn } =
    await import("./scriptedSpawn.test-support.js");
  const spawn = createScriptedSpawn(script);
  return {
    spawn: (bin: string, args: string[], opts?: Record<string, unknown>) => {
      const child = spawn(bin, args, opts);
      children.push(child);
      return child;
    },
  };
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

  // A run stops on Windows too only through its stdin: a signal there kills.
  it("starts the CLI with --control=stdin and stops a cancelled run by its stdin", async () => {
    script.calls = [];
    script.once.push({ neverExits: true, ignoresSigterm: true });
    let cancel = () => {};
    const cancellable = {
      isCancellationRequested: false,
      onCancellationRequested: (fn: () => void) => {
        cancel = fn;
        return { dispose: () => {} };
      },
    };
    const done = spawnTestProcess(
      "go",
      ["tool", "gotest", "-json", "./pkg/...", "--", "-run", "X"],
      "/ws",
      cancellable as never,
      channel as never,
      "test",
    );
    const child = children[children.length - 1];
    expect(script.calls?.[0].args).toEqual([
      "tool",
      "gotest",
      "-json",
      "./pkg/...",
      "--control=stdin",
      "--",
      "-run",
      "X",
    ]);

    cancel();
    expect(child.written).toEqual(["stop\n"]);
    child.emit("exit", 130, null);
    child.stdout.end();
    child.stderr.end();
    await expect(done).resolves.toMatchObject({ exitCode: 130 });
  });
});
