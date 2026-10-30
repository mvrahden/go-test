// Opening a saved event stream — a CI run's --events-file — in the editor.
// The stream is captured from the real CLI on the fixture corpus, then
// replayed: into the Test Explorer as its own run, into the Spec View as a
// source of its own, and never into the result store.

import { describe, it, expect, beforeAll, afterAll, vi } from "vitest";
import { spawnSync } from "node:child_process";
import { writeFileSync, mkdtempSync, rmSync, readFileSync } from "node:fs";
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
import { GoTestController } from "../../src/testController.js";
import { TestResultStore } from "../../src/testResultStore.js";
import { SpecViewPanel } from "../../src/specView.js";
import { openEventsFile } from "../../src/importEvents.js";
import { createRecordingChannel } from "./vscodeStub.js";
import type { FakeTestController } from "./vscodeTestApi.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const fixturesDir = path.join(extensionDir, "testdata", "fixtures");
const streamDir = path.join(extensionDir, "testdata", "streams");
const failing = "gotest.fixtures/failing";

let cache: DiscoveryCache;
let controller: GoTestController;
let store: TestResultStore;
let savedGoWork: string | undefined;
let tmp = "";
let eventsFile = "";

beforeAll(async () => {
  tmp = mkdtempSync(path.join(tmpdir(), "gotest-import-"));
  const workFile = path.join(tmp, "corpus.work");
  writeFileSync(
    workFile,
    `go 1.26.0\n\nuse (\n\t${repoRoot}\n\t${fixturesDir}\n)\n`,
    "utf-8",
  );
  savedGoWork = process.env.GOWORK;
  process.env.GOWORK = workFile;
  state.workspaceDir = fixturesDir;

  const recorder = createRecordingChannel();
  cache = new DiscoveryCache();
  await new DiscoveryService(cache, recorder.channel as never).discover(
    fixturesDir,
  );
  store = new TestResultStore(undefined);
  controller = new GoTestController(
    cache,
    store,
    recorder.channel as never,
    async () => {},
    async () => {},
    async () => {},
    async () => {},
  );
  controller.rebuild();

  // What CI would have saved: a run of this workspace's packages, a failure
  // that run booked for itself, and one package this checkout does not have.
  const ci = spawnSync(
    "go",
    [
      "run",
      "github.com/mvrahden/go-test/cmd/gotest",
      "-json",
      "./failing/",
      "./passing/",
    ],
    { cwd: fixturesDir, encoding: "utf-8" },
  );
  expect(ci.error).toBeUndefined();
  expect(ci.stdout).toContain(failing);
  eventsFile = path.join(tmp, "events.jsonl");
  // Framed as a gotest watch capture would be: its markers are no packages.
  writeFileSync(
    eventsFile,
    '{"Action":"watch-start","Package":"./..."}\n' +
      ci.stdout +
      '{"Action":"start","Package":"shared fixtures","RunLevel":true}\n' +
      '{"Action":"output","Package":"shared fixtures","Output":"FAIL: shared fixture teardown failed\\n","RunLevel":true}\n' +
      '{"Action":"fail","Package":"shared fixtures","RunLevel":true}\n' +
      readFileSync(path.join(streamDir, "all-pass.jsonl"), "utf-8") +
      '{"Action":"watch-end","Package":"./...","Exit":1}\n',
    "utf-8",
  );
}, 300_000);

afterAll(() => {
  if (savedGoWork === undefined) delete process.env.GOWORK;
  else process.env.GOWORK = savedGoWork;
  controller?.dispose();
  if (tmp) rmSync(tmp, { recursive: true, force: true });
});

describe("opening a saved event stream", () => {
  it("replays it as its own run in the Test Explorer, failures on their items", async () => {
    const recorder = createRecordingChannel();
    const specView = new SpecViewPanel(recorder.channel as never, cache);
    const fake = state.controllers[0] as FakeTestController;
    const before = fake.runs.length;

    const outcome = await openEventsFile(
      { controller, cache, specView, outputChannel: recorder.channel as never },
      eventsFile,
    );

    expect(outcome?.missing).toEqual(["example.com/cart"]);
    const run = fake.runs
      .slice(before)
      .find((r) => r.name === "Imported: events.jsonl");
    expect(run, "a run named after the file").toBeDefined();
    expect(run!.ended).toBe(true);
    const failed = run!.verdictsMatching("failed");
    const total = failed.find((id) =>
      id.endsWith("reports_the_expected_total"),
    );
    expect(total).toBeDefined();
    expect(run!.locations.get(total!)?.[0]).toBe(
      path.join(fixturesDir, "failing", "suite_test.go"),
    );
    expect(
      run!
        .verdictsMatching("passed")
        .some((id) => id.endsWith("still_runs_the_sibling_behavior")),
    ).toBe(true);
    // The missing package is said once, in the run.
    expect(run!.output.join("")).toContain("example.com/cart");
    // What the run booked for itself is no package to miss; its words
    // reach the run.
    expect(run!.output.join("")).toContain(
      "[shared fixtures] FAIL: shared fixture teardown failed",
    );
  }, 300_000);

  it("keeps the replayed verdicts out of the result store", async () => {
    const recorder = createRecordingChannel();
    const specView = new SpecViewPanel(recorder.channel as never, cache);
    await openEventsFile(
      { controller, cache, specView, outputChannel: recorder.channel as never },
      eventsFile,
    );

    let stored = 0;
    store.forEach(() => stored++);
    expect(stored).toBe(0);
  }, 300_000);

  it("renders a stream that was cut short instead of failing on it", async () => {
    const whole = readFileSync(eventsFile, "utf-8");
    const cut = path.join(tmp, "cut.jsonl");
    // A job killed mid-write: the last package's verdict never came, and
    // the stream ends inside a line.
    writeFileSync(cut, whole.slice(0, whole.length - 70), "utf-8");
    const recorder = createRecordingChannel();
    const specView = new SpecViewPanel(recorder.channel as never, cache);

    const outcome = await openEventsFile(
      { controller, cache, specView, outputChannel: recorder.channel as never },
      cut,
    );

    expect(outcome).toBeDefined();
    expect(recorder.errors).toEqual([]);
    expect(state.panels[state.panels.length - 1].webview.html).toContain(
      "Imported: cut.jsonl",
    );
  }, 300_000);

  it("shows it in the Spec View as its own source, every package included", async () => {
    const recorder = createRecordingChannel();
    const specView = new SpecViewPanel(recorder.channel as never, cache);
    await specView.show();
    await specView.refresh({
      stream: readFileSync(
        path.join(streamDir, "other-package-fail.jsonl"),
        "utf-8",
      ),
      folder: fixturesDir,
    });

    await openEventsFile(
      { controller, cache, specView, outputChannel: recorder.channel as never },
      eventsFile,
    );

    const html = state.panels[state.panels.length - 1].webview.html;
    expect(recorder.errors).toEqual([]);
    expect(html).toMatch(
      /<option value="file:[^"]*events\.jsonl" selected>Imported: events\.jsonl<\/option>/,
    );
    expect(html).toContain("Workspace runs");
    expect(html).toContain("reports the expected total");
    expect(html).toContain("example.com/cart");
    // The workspace's own runs are not part of what the file shows.
    expect(html).not.toContain("rejects an expired card");
  }, 300_000);
});
