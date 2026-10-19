// The Promote CodeLens offers what `gotest fuzz promote` will take.
//
// Promote takes only the crashers the latest fuzz session wrote, as recorded
// in the gotest cache. Here a real fuzz session finds a crasher, a corpus file
// it did not write sits beside it, and the lens is checked against what the
// real promote then does.

import { describe, it, expect, beforeAll, afterAll, vi } from "vitest";
import { spawnSync } from "node:child_process";
import {
  writeFileSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

const state = vi.hoisted(() => ({
  workspaceDir: "",
  cliPath: "",
  autoRefresh: true,
  panels: [] as { webview: { html: string; cspSource: string } }[],
  controllers: [] as unknown[],
  config: {} as Record<string, unknown>,
}));

vi.mock("vscode", async () => {
  const { buildVscodeStub } = await import("./vscodeStub.js");
  return buildVscodeStub(state as never);
});

import { DiscoveryCache, DiscoveryService } from "../../src/discovery.js";
import { DiscoverySnapshotStore } from "../../src/discoverySnapshotStore.js";
import { GoTestCodeLensProvider } from "../../src/codeLens.js";
import { buildCliCommand, buildFuzzArgs } from "../../src/cli.js";
import { parsePromotedSeed } from "../../src/fuzz.js";
import { createRecordingChannel } from "./vscodeStub.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");

const SUITE = "CrashTestSuite";
const METHOD = "FuzzEcho";
const WRAPPER = `Fuzz${SUITE}_${METHOD}`;
const SUITE_SOURCE = [
  "package crash",
  "",
  'import "github.com/mvrahden/go-test/pkg/gotest"',
  "",
  "type CrashTestSuite struct{}",
  "",
  "func (s *CrashTestSuite) FuzzEcho(f *gotest.F) {",
  '\tf.Add("a")',
  "\tf.Fuzz(func(t *gotest.T, in string) {",
  '\t\tgotest.Equal(t, "a", in)',
  "\t})",
  "}",
  "",
].join("\n");

let moduleDir: string;
let corpusDir: string;
let cache: DiscoveryCache;
let recorder: ReturnType<typeof createRecordingChannel>;
const saved: Record<string, string | undefined> = {};

function setEnv(key: string, value: string) {
  saved[key] = process.env[key];
  process.env[key] = value;
}

async function cli(args: string[]) {
  const cmd = await buildCliCommand(args, moduleDir, recorder.channel as never);
  return spawnSync(cmd.bin, cmd.args, {
    cwd: moduleDir,
    encoding: "utf-8",
    maxBuffer: Infinity,
    timeout: 240_000,
  });
}

async function promoteLensTitles(): Promise<string[]> {
  const file = path.join(moduleDir, "crash_test.go");
  const lenses = await new GoTestCodeLensProvider(cache).provideCodeLenses(
    {
      fileName: file,
      getText: () => SUITE_SOURCE,
      lineCount: SUITE_SOURCE.split("\n").length,
      offsetAt: () => 0,
    } as never,
    {} as never,
  );
  return lenses
    .filter((l) => l.command?.command === "gotest.promoteCrashers")
    .map((l) => l.command!.title);
}

// Every dir the file makes, removed once it ran.
const temps: string[] = [];

beforeAll(async () => {
  const tmp = mkdtempSync(path.join(tmpdir(), "gotest-promote-lens-"));
  temps.push(tmp);
  moduleDir = path.join(tmp, "crash");
  mkdirSync(moduleDir);
  writeFileSync(
    path.join(moduleDir, "go.mod"),
    [
      "module gotest.crash",
      "",
      "go 1.26.0",
      "",
      `replace github.com/mvrahden/go-test => ${repoRoot}`,
      "",
      "require github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000",
      "",
    ].join("\n"),
  );
  writeFileSync(path.join(moduleDir, "crash_test.go"), SUITE_SOURCE);
  const workFile = path.join(tmp, "crash.work");
  writeFileSync(
    workFile,
    `go 1.26.0\n\nuse (\n\t${repoRoot}\n\t${moduleDir}\n)\n`,
  );
  setEnv("GOWORK", workFile);
  // The session record lives in the gotest cache; a private one keeps the
  // developer's own record out of it, and the lens must find it there too.
  setEnv("GOTEST_CACHE_DIR", path.join(tmp, "cache"));

  state.workspaceDir = moduleDir;
  state.cliPath = "";
  state.config = {};
  recorder = createRecordingChannel();

  const fuzz = await cli(buildFuzzArgs("gotest.crash", SUITE, METHOD, "60s"));
  expect(fuzz.status, `${fuzz.stdout}\n${fuzz.stderr}`).toBe(1);

  corpusDir = path.join(moduleDir, "testdata", "fuzz", WRAPPER);
  writeFileSync(
    path.join(corpusDir, "planted"),
    'go test fuzz v1\nstring("planted")\n',
  );

  cache = new DiscoveryCache();
  const snapshotDir = mkdtempSync(path.join(tmpdir(), "gotest-snapshot-"));
  temps.push(snapshotDir);
  const snapshots = new DiscoverySnapshotStore({
    fsPath: snapshotDir,
  } as never);
  await new DiscoveryService(
    cache,
    recorder.channel as never,
    snapshots,
  ).discover(moduleDir);
}, 300_000);

afterAll(() => {
  for (const [key, value] of Object.entries(saved)) {
    if (value === undefined) delete process.env[key];
    else process.env[key] = value;
  }
  for (const dir of temps) rmSync(dir, { recursive: true, force: true });
});

describe("the Promote CodeLens after a fuzz session", () => {
  it("offers the session's crasher and not the corpus file beside it", async () => {
    expect(readdirSync(corpusDir).sort()).toHaveLength(2);

    expect(await promoteLensTitles()).toEqual(["⚠ Promote 1 crasher"]);
  });

  it("offers exactly what promote then takes", async () => {
    const offered = await promoteLensTitles();

    const promote = await cli(["fuzz", "promote", "gotest.crash"]);

    expect(promote.status, `${promote.stdout}\n${promote.stderr}`).toBe(0);
    const promoted = promote.stdout
      .split("\n")
      .map(parsePromotedSeed)
      .filter((p) => p !== undefined);
    expect(offered).toEqual([`⚠ Promote ${promoted.length} crasher`]);
    expect(promote.stdout).toMatch(
      /planted: left in place: not written by the latest fuzz session/,
    );
    expect(await promoteLensTitles()).toEqual([]);
  });
});
