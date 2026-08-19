// Discovery has to survive the payload a real repository produces.
//
// `discover` reports every behavior every suite declares, so its size grows
// with the test suite it describes. The extension used to read it through
// execFile's 1 MiB default buffer, and a repository past that size lost its
// whole test tree: three identical attempts, then an error naming a buffer
// nobody had configured. Nothing about the payload is bounded, so nothing
// reading it may be either.
//
// The module below is generated rather than committed, because the property
// under test is a size — and a size is what a checked-in fixture quietly stops
// having the moment the format gets leaner.

import { describe, it, expect, beforeAll, afterAll, vi } from "vitest";
import { spawnSync } from "node:child_process";
import { writeFileSync, mkdirSync, mkdtempSync } from "node:fs";
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
import { buildCliCommand } from "../../src/cli.js";
import { createRecordingChannel } from "./vscodeStub.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");

// Node's default execFile maxBuffer — the ceiling this suite exists to stay off.
const BUFFERED_READ_CAP = 1024 * 1024;

// Enough declared behavior to clear that cap with room to spare. The shape is
// ordinary: a context, a table, a few expectations under each row.
const SUITES = 48;
const ROWS = 20;
const EXPECTATIONS = 3;
const CONDITION =
  "the request carries a valid payload and the downstream service is reachable";

let payloadDir: string;
let savedGoWork: string | undefined;
let cache: DiscoveryCache;
let recorder: ReturnType<typeof createRecordingChannel>;

function generateSuites(): string {
  const lines = [
    "package payload_test",
    "",
    'import "github.com/mvrahden/go-test/pkg/gotest"',
    "",
  ];
  for (let s = 0; s < SUITES; s++) {
    lines.push(
      `type Feature${s}TestSuite struct{}`,
      `func (s *Feature${s}TestSuite) TestBehaviour(t *gotest.T) {`,
      `\tt.When("subsystem ${s} handles a request while ${CONDITION}", func(w *gotest.T) {`,
      "\t\tfor sub, tc := range gotest.Each(w, []struct {",
      "\t\t\tDesc string",
      "\t\t\tin   int",
      "\t\t}{",
    );
    for (let r = 0; r < ROWS; r++) {
      lines.push(
        `\t\t\t{Desc: "row ${r} exercises a boundary of the parser where ${CONDITION}", in: ${r}},`,
      );
    }
    lines.push("\t\t}) {");
    for (let e = 0; e < EXPECTATIONS; e++) {
      lines.push(
        `\t\t\tsub.It("reports outcome ${e} and leaves no shared state behind when ${CONDITION}", func(it *gotest.T) {`,
        "\t\t\t\tgotest.Equal(it, tc.in, tc.in)",
        "\t\t\t})",
      );
    }
    lines.push("\t\t}", "\t})", "}", "");
  }
  return lines.join("\n");
}

beforeAll(async () => {
  const tmp = mkdtempSync(path.join(tmpdir(), "gotest-payload-"));
  payloadDir = path.join(tmp, "payload");
  mkdirSync(payloadDir);

  // The require/replace pair is what makes the extension's CLI resolution pick
  // this working tree's gotest instead of a published release.
  writeFileSync(
    path.join(payloadDir, "go.mod"),
    [
      "module gotest.payload",
      "",
      "go 1.25.0",
      "",
      `replace github.com/mvrahden/go-test => ${repoRoot}`,
      "",
      "require github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000",
      "",
    ].join("\n"),
    "utf-8",
  );
  writeFileSync(
    path.join(payloadDir, "payload_suite_test.go"),
    generateSuites(),
    "utf-8",
  );

  const workFile = path.join(tmp, "payload.work");
  writeFileSync(
    workFile,
    `go 1.25.0\n\nuse (\n\t${repoRoot}\n\t${payloadDir}\n)\n`,
    "utf-8",
  );
  savedGoWork = process.env.GOWORK;
  process.env.GOWORK = workFile;

  state.workspaceDir = payloadDir;
  state.cliPath = "";
  state.config = {};

  recorder = createRecordingChannel();
  cache = new DiscoveryCache();
  await new DiscoveryService(cache, recorder.channel as never).discover(
    payloadDir,
  );
}, 300_000);

afterAll(() => {
  if (savedGoWork === undefined) delete process.env.GOWORK;
  else process.env.GOWORK = savedGoWork;
});

describe("a payload larger than a buffered read", () => {
  // Stated as a test because it is a precondition for the next one: if the
  // fixture ever slips back under the cap, this says so instead of leaving a
  // suite that passes without exercising anything.
  it("is what this fixture actually produces", async () => {
    const cmd = await buildCliCommand(
      ["discover", "./..."],
      payloadDir,
      recorder.channel as never,
    );
    const result = spawnSync(cmd.bin, cmd.args, {
      cwd: payloadDir,
      maxBuffer: Infinity,
    });

    expect(result.status, result.stderr?.toString()).toBe(0);
    expect(result.stdout.length).toBeGreaterThan(BUFFERED_READ_CAP);
  }, 300_000);

  it("is discovered whole, not lost to the reader", () => {
    const pkg = cache.getPackage("gotest.payload");

    expect(pkg?.suites).toHaveLength(SUITES);
    expect(recorder.errors).toEqual([]);
  });

  it("keeps every behavior each suite declares", () => {
    const suite = cache
      .getPackage("gotest.payload")
      ?.suites.find((s) => s.name === "Feature0TestSuite");
    const when = suite?.methods[0].behaviors?.[0];

    expect(when?.display).toBe(
      `subsystem 0 handles a request while ${CONDITION}`,
    );
    expect(when?.children).toHaveLength(ROWS);
    expect(when?.children?.[0].children).toHaveLength(EXPECTATIONS);
  });
});
