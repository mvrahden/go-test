// The Change scope against a real checkout: the conformance suite's audit
// base committed on main, its head on a branch, audited by the real CLI
// through the extension's command resolution. Only the editor is stubbed.

import {
  describe,
  it,
  expect,
  beforeAll,
  afterAll,
  beforeEach,
  vi,
} from "vitest";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  cpSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
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
  panels: [] as {
    webview: { html: string; post?: (msg: unknown) => void };
  }[],
}));

vi.mock("vscode", async () => {
  const { buildVscodeStub } = await import("./vscodeStub.js");
  return buildVscodeStub(state as never);
});

import { ChangeScope } from "../../src/changeScope.js";
import { SpecViewPanel } from "../../src/specView.js";
import { clearBinaryCache } from "../../src/cli.js";
import type { Memento } from "../../src/acceptance.js";
import { createRecordingChannel } from "./vscodeStub.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const fixtures = path.join(
  repoRoot,
  "tests",
  "conformance",
  "testdata",
  "audit",
);
const sh = process.platform === "win32";

let tmp = "";
let checkout = "";
let plainDir = "";
let currentCli = "";
let oldCli = "";

function git(dir: string, ...args: string[]): string {
  const r = spawnSync(
    "git",
    ["-c", "user.name=gotest", "-c", "user.email=gotest@example.com", ...args],
    { cwd: dir, encoding: "utf-8" },
  );
  expect(r.status, `git ${args.join(" ")}: ${r.stderr}`).toBe(0);
  return r.stdout;
}

// stage copies a fixture tree into dir, bound to this checkout.
function stage(tree: string, dir: string): void {
  cpSync(path.join(fixtures, tree), dir, { recursive: true });
  const goMod = path.join(dir, "go.mod");
  writeFileSync(
    goMod,
    readFileSync(goMod, "utf-8").replaceAll("REPO_ROOT", repoRoot),
  );
}

// wrapper is a CLI that says it is version and runs the real binary.
function wrapper(name: string, version: string, real: string): string {
  const file = path.join(tmp, name);
  writeFileSync(
    file,
    [
      "#!/bin/sh",
      `if [ "$1" = "version" ]; then echo "gotest ${version} integration"; exit 0; fi`,
      `exec ${real} "$@"`,
      "",
    ].join("\n"),
  );
  chmodSync(file, 0o755);
  return file;
}

function memento(): Memento {
  const data = new Map<string, unknown>();
  return {
    get: <T>(k: string) => data.get(k) as T | undefined,
    update: async (k: string, v: unknown) => {
      data.set(k, JSON.parse(JSON.stringify(v)));
    },
  };
}

function scope(folder: string, store = memento()) {
  const rec = createRecordingChannel();
  let changes = 0;
  const s = new ChangeScope({
    log: rec.channel as never,
    memento: store,
    folders: () => [folder],
    changed: () => changes++,
  });
  return { s, rec, changes: () => changes };
}

beforeAll(() => {
  tmp = realpathSync.native(mkdtempSync(path.join(tmpdir(), "gotest-change-")));
  const real = path.join(tmp, "gotest-real");
  const built = spawnSync(
    "go",
    ["build", "-buildvcs=false", "-o", real, "./cmd/gotest"],
    { cwd: repoRoot, encoding: "utf-8" },
  );
  expect(built.status, `go build failed: ${built.stderr}`).toBe(0);
  currentCli = wrapper("gotest-current", "v1.42.0", real);
  oldCli = wrapper("gotest-old", "v1.41.0", real);

  // main holds the base; feat/audit the head, one commit on top.
  checkout = path.join(tmp, "repo");
  stage("base", checkout);
  git(checkout, "init", "-q", "-b", "main");
  git(checkout, "add", "-A");
  git(checkout, "commit", "-q", "-m", "base");
  git(checkout, "remote", "add", "origin", path.join(tmp, "nowhere.git"));
  git(checkout, "update-ref", "refs/remotes/origin/main", "HEAD");
  git(checkout, "checkout", "-q", "-b", "feat/audit");
  for (const e of readdirSync(checkout)) {
    if (e !== ".git") rmSync(path.join(checkout, e), { recursive: true });
  }
  stage("head", checkout);
  git(checkout, "add", "-A");
  git(checkout, "commit", "-q", "-m", "head");

  plainDir = path.join(tmp, "plain");
  stage("head", plainDir);
}, 300_000);

afterAll(() => {
  if (tmp) rmSync(tmp, { recursive: true, force: true });
});

beforeEach(() => {
  state.workspaceDir = checkout;
  state.cliPath = currentCli;
  state.panels.length = 0;
  clearBinaryCache();
});

describe.skipIf(sh)("the Change scope", () => {
  it("audits the branch against its merge-base with the remote default branch", async () => {
    const { s, rec } = scope(checkout);
    await s.audit(checkout);
    const view = s.view();
    expect(rec.errors).toEqual([]);
    expect(view.state.kind).toBe("doc");
    expect(view.headBranch).toBe("feat/audit");
    const mb = git(checkout, "rev-parse", "--short", "main").trim();
    expect(view.baseLabel).toBe(`merge-base with origin/main (${mb})`);
    if (view.state.kind !== "doc") return;
    const m = view.state.model;
    // The CLI names the base by the commit it was given.
    expect(m.base).toBe(mb);
    expect(m.open.map((g) => g.tag)).toEqual([
      "removed",
      "excluded",
      "focus",
      "guard",
      "budget",
      "suppression",
      "snapshot",
      "assertions",
      "asserts-by",
    ]);
    expect(m.behaviorCount).toBe(10);
    // Every decision about a package's test opens its file; a removed one
    // has nothing in the working tree to run.
    const removed = m.open[0].rows[0];
    expect(view.canOpen(removed.key)).toBe(true);
    expect(view.canRun(removed.key)).toBe(false);
  });

  it("renders in the panel when the Change scope is chosen", async () => {
    const { s } = scope(checkout);
    const rec = createRecordingChannel();
    const panel = new SpecViewPanel(rec.channel as never);
    panel.setChangeScope(s);
    await panel.show();
    const webview = state.panels[0].webview;
    expect(webview.html).toContain('data-scope="change" aria-selected="false"');
    webview.post?.({ type: "scope", scope: "change" });
    await vi.waitFor(
      () => {
        panel.changeChanged();
        expect(webview.html).toContain("Needs a decision");
      },
      { timeout: 120_000, interval: 200 },
    );
    expect(webview.html).toContain('data-scope="change" aria-selected="true"');
    expect(webview.html).toContain("Changed behaviors");
    expect(webview.html).toContain("13 open · 0 accepted");
    panel.dispose();
  });

  it("keeps an acceptance for the head commit, and drops it on a new one", async () => {
    const store = memento();
    const first = scope(checkout, store);
    await first.s.audit(checkout);
    const v = first.s.view();
    if (v.state.kind !== "doc") throw new Error(v.state.kind);
    const key = v.state.model.open[0].rows[0].key;
    await first.s.act({ act: "accept", key });
    const after = first.s.view();
    if (after.state.kind !== "doc") throw new Error(after.state.kind);
    expect(after.state.model.acceptedCount).toBe(1);
    expect(after.state.model.accepted[0].key).toBe(key);

    // Another session over the same workspace state.
    const again = scope(checkout, store);
    await again.s.audit(checkout);
    const kept = again.s.view();
    if (kept.state.kind !== "doc") throw new Error(kept.state.kind);
    expect(kept.state.model.acceptedCount).toBe(1);

    git(checkout, "commit", "-q", "--allow-empty", "-m", "another head");
    await again.s.audit(checkout);
    const moved = again.s.view();
    if (moved.state.kind !== "doc") throw new Error(moved.state.kind);
    expect(moved.state.model.acceptedCount).toBe(0);
  });

  it("compares with the base the user chose", async () => {
    const { s } = scope(checkout);
    await s.setBase(checkout, { ref: "HEAD", mergeBase: false });
    await s.audit(checkout);
    const v = s.view();
    expect(v.baseLabel).toBe("HEAD");
    if (v.state.kind !== "doc") throw new Error(v.state.kind);
    expect(v.state.model.openCount).toBe(0);
    expect(v.state.model.behaviorCount).toBe(0);
  });

  it("shows the CLI's complaint for a base it cannot find", async () => {
    const { s } = scope(checkout);
    await s.setBase(checkout, { ref: "no-such-ref", mergeBase: false });
    await s.audit(checkout);
    const v = s.view();
    expect(v.state.kind).toBe("error");
    if (v.state.kind === "error") expect(v.state.message).toContain("--base");
  });

  it("says a folder outside git has no change to audit", async () => {
    state.workspaceDir = plainDir;
    const { s } = scope(plainDir);
    await s.audit(plainDir);
    expect(s.view().state.kind).toBe("notGit");
  });

  it("explains a CLI that predates audit, and runs no audit with it", async () => {
    state.cliPath = oldCli;
    const { s, rec } = scope(checkout);
    await s.audit(checkout);
    const v = s.view();
    expect(v.state).toMatchObject({
      kind: "oldCli",
      version: "v1.41.0",
      floor: "v1.42.0",
      canUpgrade: false,
    });
    expect(rec.errors).toEqual([]);
  });
});
