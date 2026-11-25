// Run records against the real CLI: an editor run that leaves a record with
// its events file, the Runs scope that lists, opens and takes it as evidence,
// a --changed run that leaves what did not change alone, and the Spec View
// that marks a package stale once its source changed. A staged module, a
// cache of its own; only the editor is stubbed.

import { describe, it, expect, beforeAll, afterAll, vi } from "vitest";
import { spawnSync } from "node:child_process";
import {
  appendFileSync,
  chmodSync,
  cpSync,
  existsSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

const state = vi.hoisted(() => ({
  workspaceDir: "",
  cliPath: "",
  autoRefresh: true,
  panels: [] as { webview: { html: string } }[],
  controllers: [] as unknown[],
  config: {} as Record<string, unknown>,
}));

vi.mock("vscode", async () => {
  const { buildVscodeStub } = await import("./vscodeStub.js");
  return buildVscodeStub(state as never);
});

import { DiscoveryCache, DiscoveryService } from "../../src/discovery.js";
import { GoTestController } from "../../src/testController.js";
import { TestResultStore } from "../../src/testResultStore.js";
import { executeBatch } from "../../src/batchRunner.js";
import { setEventsDir } from "../../src/eventsFiles.js";
import { RunsScope } from "../../src/runsScope.js";
import { EvidenceSelection } from "../../src/evidence.js";
import { SpecViewPanel } from "../../src/specView.js";
import { clearBinaryCache } from "../../src/cli.js";
import type { Memento } from "../../src/acceptance.js";
import { createRecordingChannel } from "./vscodeStub.js";
import { FakeTestRun } from "./vscodeTestApi.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const audit = path.join(repoRoot, "tests", "conformance", "testdata", "audit");
const shop = "auditmod/shop";
const sh = process.platform === "win32";

let tmp = "";
let mod = "";
let currentCli = "";
let oldCli = "";
let cache: DiscoveryCache;
let controller: GoTestController;
const saved: Record<string, string | undefined> = {};

const token = {
  isCancellationRequested: false,
  onCancellationRequested: () => ({ dispose: () => {} }),
};

function memento(): Memento {
  const data = new Map<string, unknown>();
  return {
    get: <T>(k: string) => data.get(k) as T | undefined,
    update: async (k: string, v: unknown) => {
      data.set(k, JSON.parse(JSON.stringify(v)));
    },
  };
}

// wrapper is a CLI that says it is version and runs the real binary; asked
// version --json, the real one answers, or none when json is false.
function wrapper(name: string, version: string, real: string, json: boolean) {
  const file = path.join(tmp, name);
  const answer = json
    ? `exec ${real} "$@"`
    : `echo "gotest ${version} integration"; exit 0`;
  writeFileSync(
    file,
    [
      "#!/bin/sh",
      `if [ "$1" = "version" ] && [ "$2" = "--json" ]; then ${answer}; fi`,
      `if [ "$1" = "version" ]; then echo "gotest ${version} integration"; exit 0; fi`,
      `exec ${real} "$@"`,
      "",
    ].join("\n"),
  );
  chmodSync(file, 0o755);
  return file;
}

async function runShop(changed = false): Promise<{
  run: FakeTestRun;
  stdout: string;
}> {
  const run = new FakeTestRun();
  const item = controller.findItem(shop);
  expect(item, "no item for the shop package").toBeDefined();
  const result = await executeBatch({
    pkgInfos: [
      {
        importPath: shop,
        items: [item!] as never,
        dir: path.join(mod, "shop"),
      },
    ],
    filter: undefined,
    workspaceDir: mod,
    testFlags: ["--agent=false"],
    run: run as never,
    token: token as never,
    controller,
    outputChannel: createRecordingChannel().channel as never,
    label: "test",
    changed,
  });
  return { run, stdout: result.stdout };
}

function runsScope(evidence = new EvidenceSelection(memento())) {
  const opened: string[][] = [];
  const s = new RunsScope({
    log: createRecordingChannel().channel as never,
    folders: () => [mod],
    evidence,
    openStream: async (id, label, folder, file) => {
      opened.push([id, label, folder, file]);
    },
    changed: () => {},
  });
  return { s, opened, evidence };
}

function rows(s: RunsScope) {
  const v = s.view();
  if (v.state.kind !== "doc") throw new Error(JSON.stringify(v.state));
  return v.state.rows;
}

beforeAll(async () => {
  tmp = realpathSync.native(mkdtempSync(path.join(tmpdir(), "gotest-runs-")));
  const real = path.join(tmp, "gotest-real");
  const built = spawnSync(
    "go",
    ["build", "-buildvcs=false", "-o", real, "./cmd/gotest"],
    { cwd: repoRoot, encoding: "utf-8" },
  );
  expect(built.status, `go build failed: ${built.stderr}`).toBe(0);
  currentCli = wrapper("gotest-current", "v1.42.0", real, true);
  oldCli = wrapper("gotest-old", "v1.41.0", real, false);

  mod = path.join(tmp, "mod");
  cpSync(path.join(audit, "head"), mod, { recursive: true });
  const goMod = path.join(mod, "go.mod");
  writeFileSync(
    goMod,
    readFileSync(goMod, "utf-8").replaceAll("REPO_ROOT", repoRoot),
  );
  for (const k of ["GOWORK", "GOTEST_CACHE_DIR", "GOTEST_SLOW_BUILD_THRESHOLD"])
    saved[k] = process.env[k];
  process.env.GOWORK = "off";
  process.env.GOTEST_CACHE_DIR = path.join(tmp, "cache");
  process.env.GOTEST_SLOW_BUILD_THRESHOLD = "1h";

  state.workspaceDir = mod;
  state.cliPath = currentCli;
  clearBinaryCache();
  setEventsDir(path.join(tmp, "events"));

  const rec = createRecordingChannel();
  cache = new DiscoveryCache();
  await new DiscoveryService(cache, rec.channel as never).discover(mod);
  controller = new GoTestController(
    cache,
    new TestResultStore(undefined),
    rec.channel as never,
    async () => {},
    async () => {},
    async () => {},
    async () => {},
  );
  controller.rebuild();
}, 300_000);

afterAll(() => {
  setEventsDir(undefined);
  for (const [k, v] of Object.entries(saved)) {
    if (v === undefined) delete process.env[k];
    else process.env[k] = v;
  }
  controller?.dispose();
  if (tmp) rmSync(tmp, { recursive: true, force: true });
});

describe.skipIf(sh)("run records", () => {
  let first = "";

  it("an editor run leaves a record whose events file the Runs scope opens and takes as evidence", async () => {
    const { stdout } = await runShop();
    first = stdout;
    const { s, opened, evidence } = runsScope();
    await s.refresh(mod);
    const [row] = rows(s);
    expect(row.eventsFile?.startsWith(path.join(tmp, "events"))).toBe(true);
    expect(existsSync(row.eventsFile!)).toBe(true);
    expect(row.canOpen).toBe(true);
    expect(row.args).toContain("--events-file=");
    expect(row.judges).toMatch(
      /^judges this tree: [1-9]\d* current \/ 0 stale/,
    );

    await s.act({ act: "open", key: row.id });
    expect(opened).toEqual([
      [`run:${row.id}`, expect.any(String), mod, row.eventsFile],
    ]);
    await s.act({ act: "evidence", key: row.id });
    expect(evidence.list(mod).map((p) => p.file)).toEqual([row.eventsFile]);
    expect(rows(s)[0].evidence).toBe(true);
  });

  it("a --changed run with nothing changed leaves the package alone and says it was narrowed", async () => {
    const { run, stdout } = await runShop(true);
    expect(run.output.join("")).toMatch(
      /narrowed by --changed: ran 0 packages, left out \d+ packages? whose inputs did not change/,
    );
    // The left-out package gets no verdict from this run.
    expect(run.verdicts.size).toBe(0);
    expect(stdout).toContain('"Changed"');
    const { s } = runsScope();
    await s.refresh(mod);
    expect(rows(s)[0].narrowed).toMatch(/^--changed: ran 0 packages, left out/);
  });

  it("imports a saved stream among the records", async () => {
    const { s } = runsScope();
    const file = path.join(audit, "proof", "current.json");
    expect(await s.importRun(mod, file)).toBe(true);
    const [row] = rows(s);
    expect(row.imported).toBe("current.json");
    expect(row.judges).toBe(
      "judges this tree: 0 current / 0 stale / 1 unknown",
    );
  });

  it("the Spec View marks the package stale once its source changes, and not before", async () => {
    const panel = new SpecViewPanel(
      createRecordingChannel().channel as never,
      cache,
    );
    try {
      await panel.refresh({ stream: first, folder: mod, at: Date.now() });
      await panel.show();
      await panel.freshIdle();
      const html = () => state.panels.at(-1)!.webview.html;
      expect(html()).toContain("TestStockTestSuite");
      expect(html()).not.toContain("stale: inputs changed since the run");

      appendFileSync(path.join(mod, "shop", "shop.go"), "\n// edited\n");
      panel.sourceChanged(mod);
      await panel.freshIdle();
      expect(html()).toContain("stale: inputs changed since the run");
      expect(html()).toContain(
        "1 of 1 package stale: their inputs changed since the run",
      );
    } finally {
      panel.dispose();
    }
  });

  it("a CLI without run records shows the release it needs, and marks nothing stale", async () => {
    state.cliPath = oldCli;
    clearBinaryCache();
    try {
      const { s } = runsScope();
      await s.refresh(mod);
      const v = s.view();
      expect(v.state).toMatchObject({ kind: "oldCli", floor: "v1.42.0" });

      const panel = new SpecViewPanel(
        createRecordingChannel().channel as never,
        cache,
      );
      try {
        await panel.refresh({ stream: first, folder: mod, at: Date.now() });
        await panel.show();
        await panel.freshIdle();
        expect(state.panels.at(-1)!.webview.html).not.toContain(
          "stale: inputs changed",
        );
      } finally {
        panel.dispose();
      }
    } finally {
      state.cliPath = currentCli;
      clearBinaryCache();
    }
  });
});
