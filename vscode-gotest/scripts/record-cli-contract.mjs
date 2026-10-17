// Records the real exit-code / stream contract of `gotest spec --input=-`.
//
// The extension is a renderer client of that command, and the two are tested on
// opposite sides of a process boundary that neither suite crosses. This script
// pins the boundary itself: every case is executed twice, once against a
// prebuilt binary and once through `go run` — the extension's default CLI
// resolution, and the participant that appends "exit status N" to stderr.
//
// Both builds pass -buildvcs=false: since Go 1.24 `go build` stamps the main
// module's version from git, which the CLI reports in every JSON document's
// `version` field, so a stamped build would change the recording with every
// commit and never match the `go run` path.
//
// Regenerate with `npm run contract:record`. CI runs `npm run contract:check`,
// so a change in CLI behaviour lands as a reviewable diff instead of a silent
// break in a consumer nobody re-ran.
//
// Most streams name packages no checkout can load, which pins the name-only
// fallback. `declared-source.jsonl` names a package of this repository, so with
// the repo root as cwd the replay reads its source: it pins the label rendered
// in its vocabulary, the `vocab` field on the wire, and a silent stderr while
// the package loads. It is the one case that pays a type-check.
//
// The `live` cases run the CLI itself on packages of this repository and pin
// what a run writes: the -json stream's shape (one start per package before
// its other events, the verdicts of packages it runs nothing in) and a run
// whose patterns match no package. Times and durations are masked.

import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync, readdirSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const extensionDir = path.resolve(here, "..");
const repoRoot = path.resolve(extensionDir, "..");
const streamDir = path.join(extensionDir, "testdata", "streams");
const goldenPath = path.join(extensionDir, "testdata", "cli-contract.json");

// Both invocations the contract has to hold for: the gating form, and the
// render-only form the Spec View actually uses. Recording only the one the
// extension sends would leave the CI-facing verdict unpinned, and recording
// only the gating form is how the original regression stayed invisible.
const VARIANTS = {
  gating: ["spec", "--input=-", "--format=json"],
  renderOnly: ["spec", "--input=-", "--format=json", "--render-only"],
};

function buildBinary() {
  const dir = mkdtempSync(path.join(tmpdir(), "gotest-contract-"));
  const bin = path.join(
    dir,
    process.platform === "win32" ? "gotest.exe" : "gotest",
  );
  const built = spawnSync(
    "go",
    ["build", "-buildvcs=false", "-o", bin, "./cmd/gotest"],
    {
      cwd: repoRoot,
      encoding: "utf-8",
    },
  );
  if (built.status !== 0) {
    throw new Error(`go build failed: ${built.stderr}`);
  }
  return bin;
}

function run(bin, args, input) {
  const result = spawnSync(bin, args, {
    cwd: repoRoot,
    input,
    encoding: "utf-8",
  });
  if (result.error) throw result.error;
  return {
    exitCode: result.status,
    stdout: result.stdout,
    stderr: result.stderr,
  };
}

const bin = buildBinary();
const cases = {};

for (const file of readdirSync(streamDir).sort()) {
  if (!file.endsWith(".jsonl")) continue;
  const name = file.replace(/\.jsonl$/, "");
  const input = readFileSync(path.join(streamDir, file), "utf-8");

  cases[name] = {};
  for (const [variant, args] of Object.entries(VARIANTS)) {
    const direct = run(bin, args, input);
    const goRun = run(
      "go",
      ["run", "-buildvcs=false", "./cmd/gotest", ...args],
      input,
    );

    cases[name][variant] = {
      direct,
      goRun: {
        exitCode: goRun.exitCode,
        stderr: goRun.stderr,
        // `go run` must not alter what the CLI renders — it only wraps the exit
        // status. If this ever goes false the composition changed, not the CLI.
        stdoutMatchesDirect: goRun.stdout === direct.stdout,
      },
    };
  }
}

// Live runs: args relative to the repo root. At most one package runs a
// binary, so the stream's order is deterministic.
const unrun = "./cmd/gotest/testdata/unrun/";
const LIVE = {
  "no-packages": ["-json", `${unrun}nogo/...`],
  verdicts: [
    "-json",
    `${unrun}notestfiles/`,
    `${unrun}nosuites/`,
    `${unrun}excluded/`,
    "./cmd/gotest/testdata/census/plain/",
  ],
};

// mask drops what changes from run to run: times and durations.
function mask(stdout) {
  return stdout
    .split("\n")
    .filter((line) => line.trim() !== "")
    .map((line) => {
      const ev = JSON.parse(line);
      delete ev.Time;
      if ("Elapsed" in ev) ev.Elapsed = 0;
      if (typeof ev.Output === "string") {
        ev.Output = ev.Output.replace(/\d+\.\d+s\b/g, "<d>s");
      }
      return ev;
    });
}

const live = {};
for (const [name, args] of Object.entries(LIVE)) {
  const result = run(bin, args, "");
  live[name] = {
    args,
    exitCode: result.exitCode,
    stderr: result.stderr,
    events: mask(result.stdout),
  };
}

const golden = {
  note: [
    "Recorded contract of `gotest spec --input=- --format=json`, and of live",
    "`gotest -json` runs (`live`).",
    "Generated by scripts/record-cli-contract.mjs — do not hand-edit.",
    "A diff here is a change to an interface the VS Code extension consumes.",
  ],
  cases,
  live,
};

writeFileSync(goldenPath, JSON.stringify(golden, null, 2) + "\n", "utf-8");
console.log(
  `recorded ${Object.keys(cases).length} cases and ${Object.keys(live).length} live runs -> ${path.relative(extensionDir, goldenPath)}`,
);
