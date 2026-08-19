// `go tool cover -func` writes one line per function in the profile, so its
// output grows with the repository just as discovery's does — and it used to be
// read through the same 1 MiB buffer.

import { describe, it, expect, vi } from "vitest";
import type { SpawnScript } from "./scriptedSpawn.test-support.js";

const { script } = vi.hoisted(() => ({
  script: { once: [], always: undefined, calls: [] } as SpawnScript,
}));

vi.mock("./cli.js", () => ({
  resolveGoBinary: async () => "/usr/bin/go",
}));

vi.mock("node:child_process", async () => {
  const { createScriptedSpawn } =
    await import("./scriptedSpawn.test-support.js");
  return { spawn: createScriptedSpawn(script) };
});

import { runGoToolCoverFunc } from "./coverageUtils.js";

describe("runGoToolCoverFunc", () => {
  it("returns a profile report larger than a buffered read", async () => {
    const line =
      "example.com/org/repo/internal/service/file.go:42:\tMethod\t100.0%\n";
    const report = line.repeat(20_000);
    expect(report.length).toBeGreaterThan(1024 * 1024);

    const chunks: string[] = [];
    for (let i = 0; i < report.length; i += 64 * 1024) {
      chunks.push(report.slice(i, i + 64 * 1024));
    }
    script.once.push({ stdout: chunks });

    const out = await runGoToolCoverFunc("/tmp/cover.out", "/ws");

    expect(out).toHaveLength(report.length);
    expect(script.calls?.[0]).toEqual({
      bin: "/usr/bin/go",
      args: ["tool", "cover", "-func=/tmp/cover.out"],
    });
  });
});
