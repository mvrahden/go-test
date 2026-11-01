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
//
// The `outputs` cases pin the other documents the extension reads: the
// `discover` payload, the `bench --json` report, the cover profile a run
// merges for `-coverprofile`, and what `fuzz promote` prints. Absolute paths
// become <repo> or <stage>, and measurements and host facts are zeroed or
// masked. Promote runs on a staged copy of cmd/gotest/testdata/fuzzcrash with
// a session record written here, so no fuzzing happens and the bytes hold.

import { spawn } from "node:child_process";
import {
  cpSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  writeFileSync,
  readdirSync,
  mkdtempSync,
  rmSync,
} from "node:fs";
import { createHash } from "node:crypto";
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

// The command running now, stopped with the script.
let current;

// A stop removes the build dir and ends the script by the signal. The
// commands run asynchronously, so the handler runs as the signal arrives.
const removals = [];
for (const signal of ["SIGINT", "SIGTERM"]) {
  process.once(signal, () => {
    const stop = () => {
      for (const remove of removals) remove();
      process.kill(process.pid, signal);
    };
    // The command ends first, so nothing writes in the dir once it is gone.
    if (current) {
      current.once("close", stop);
      current.kill(signal);
    } else {
      stop();
    }
  });
}

// spawnAsync runs command to its end, feeding it input.
function spawnAsync(command, args, input, opts = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd: repoRoot, ...opts });
    current = child;
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf-8").on("data", (d) => (stdout += d));
    child.stderr.setEncoding("utf-8").on("data", (d) => (stderr += d));
    child.on("error", reject);
    child.on("close", (status) => {
      current = undefined;
      resolve({ status, stdout, stderr });
    });
    child.stdin.on("error", () => {});
    child.stdin.end(input);
  });
}

// tempDir makes a dir that is removed however the script ends, a failed
// build or a stop included.
function tempDir(prefix) {
  const dir = mkdtempSync(path.join(tmpdir(), prefix));
  const remove = () => rmSync(dir, { recursive: true, force: true });
  process.on("exit", remove);
  removals.push(remove);
  return dir;
}

async function buildBinary() {
  const dir = tempDir("gotest-contract-");
  const bin = path.join(
    dir,
    process.platform === "win32" ? "gotest.exe" : "gotest",
  );
  const built = await spawnAsync(
    "go",
    ["build", "-buildvcs=false", "-o", bin, "./cmd/gotest"],
    "",
  );
  if (built.status !== 0) {
    throw new Error(`go build failed: ${built.stderr}`);
  }
  return bin;
}

async function run(bin, args, input, opts) {
  const result = await spawnAsync(bin, args, input, opts);
  return {
    exitCode: result.status,
    stdout: result.stdout,
    stderr: result.stderr,
  };
}

const bin = await buildBinary();
const cases = {};

for (const file of readdirSync(streamDir).sort()) {
  if (!file.endsWith(".jsonl")) continue;
  const name = file.replace(/\.jsonl$/, "");
  const input = readFileSync(path.join(streamDir, file), "utf-8");

  cases[name] = {};
  for (const [variant, args] of Object.entries(VARIANTS)) {
    const direct = await run(bin, args, input);
    const goRun = await run(
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
  const result = await run(bin, args, "");
  live[name] = {
    args,
    exitCode: result.exitCode,
    stderr: result.stderr,
    events: mask(result.stdout),
  };
}

// The environment of the outputs cases: no CI variables, which change what
// the CLI writes, and no slow-build notice on a slow machine.
function outputEnv(extra = {}) {
  const env = { ...process.env, GOTEST_SLOW_BUILD_THRESHOLD: "1h", ...extra };
  for (const name of [
    "CI",
    "GOTEST_CI",
    "GOTEST_UPDATE_SNAPSHOTS",
    "GITHUB_ACTIONS",
    "GITHUB_STEP_SUMMARY",
    "GITHUB_WORKSPACE",
  ]) {
    delete env[name];
  }
  return env;
}

// maskPaths replaces every string under root, at any depth, with label plus
// its slash-separated path below root.
function maskPaths(value, root, label) {
  if (typeof value === "string") {
    const rel = path.relative(root, value);
    if (
      path.isAbsolute(value) &&
      !rel.startsWith("..") &&
      !path.isAbsolute(rel)
    ) {
      return rel === "" ? label : `${label}/${rel.split(path.sep).join("/")}`;
    }
    return value;
  }
  if (Array.isArray(value)) return value.map((v) => maskPaths(v, root, label));
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([k, v]) => [k, maskPaths(v, root, label)]),
    );
  }
  return value;
}

// document parses a JSON stdout, or keeps it raw when it is not one document.
function document(stdout, mask) {
  try {
    return { document: mask(JSON.parse(stdout)) };
  } catch {
    return { stdout };
  }
}

// maskBench zeroes what a benchmark measures and masks the host it ran on.
function maskBench(report) {
  const base = report.baseline;
  if (base) {
    for (const key of ["createdAt", "goVersion", "goos", "goarch", "cpu"]) {
      if (key in base) base[key] = `<${key}>`;
    }
    if ("gomaxprocs" in base) base.gomaxprocs = 0;
    for (const r of base.results ?? []) {
      for (const sample of r.samples ?? []) {
        sample.nsPerOp = 0;
        sample.bytesPerOp = 0;
        sample.allocsPerOp = 0;
      }
    }
  }
  return report;
}

const outputs = {};

const discoverArgs = [
  "discover",
  "./cmd/gotest/testdata/census/...",
  "./examples/search/",
];
{
  const result = await run(bin, discoverArgs, "", { env: outputEnv() });
  outputs.discover = {
    args: discoverArgs,
    exitCode: result.exitCode,
    stderr: result.stderr,
    ...document(result.stdout, (d) => maskPaths(d, repoRoot, "<repo>")),
  };
}

// One iteration, so iterations is the only number left to pin.
const benchArgs = [
  "bench",
  "./cmd/gotest/testdata/census/benchplain/",
  "-bench=^BenchmarkBenchPlainTestSuite$",
  "-benchtime=1x",
  "--json",
];
{
  const result = await run(bin, benchArgs, "", { env: outputEnv() });
  outputs.bench = {
    args: benchArgs,
    exitCode: result.exitCode,
    stderr: result.stderr,
    ...document(result.stdout, maskBench),
  };
}

// The profile is written to a temp file; the golden names it <profile>.
{
  const profile = path.join(tempDir("gotest-contract-cover-"), "cover.out");
  const shown = [
    "--",
    "-count=1",
    "-covermode=atomic",
    "-coverprofile=<profile>",
    "./examples/search/",
  ];
  const args = shown.map((a) => a.replace("<profile>", profile));
  const result = await run(bin, args, "", { env: outputEnv() });
  let lines;
  try {
    lines = readFileSync(profile, "utf-8")
      .split("\n")
      .filter((l) => l !== "");
  } catch {
    lines = null;
  }
  outputs.cover = {
    args: shown,
    exitCode: result.exitCode,
    stderr: result.stderr,
    profile: lines,
  };
}

// stageFuzzModule copies the fuzzcrash module into a fresh dir, bound to this
// checkout, with a gotest cache of its own for the session record.
function stageFuzzModule() {
  // Resolved, so the CLI's paths relative to it never climb through a link.
  const root = realpathSync.native(tempDir("gotest-contract-fuzz-"));
  const dir = path.join(root, "fuzzcrash");
  cpSync(path.join(repoRoot, "cmd", "gotest", "testdata", "fuzzcrash"), dir, {
    recursive: true,
  });
  const goMod = path.join(dir, "go.mod");
  writeFileSync(
    goMod,
    readFileSync(goMod, "utf-8").replaceAll("REPO_ROOT", repoRoot),
  );
  const goWork = path.join(dir, "go.work");
  writeFileSync(goWork, `go 1.26.0\n\nuse (\n\t.\n\t${repoRoot}\n)\n`);
  const cache = path.join(root, "cache");
  mkdirSync(cache);
  return {
    dir,
    cache,
    env: outputEnv({ GOWORK: goWork, GOTEST_CACHE_DIR: cache }),
  };
}

// plant writes a corpus file under the target's testdata/fuzz dir.
function plant(stage, func, name, content) {
  const corpus = path.join(stage.dir, "testdata", "fuzz", func);
  mkdirSync(corpus, { recursive: true });
  writeFileSync(path.join(corpus, name), content);
  return { dir: stage.dir, func, name, content };
}

// recordSession writes the session record a fuzz session that found the
// given crashers would have left.
function recordSession(stage, crashers) {
  const session = {
    crashers: crashers.map((c) => ({
      dir: c.dir,
      func: c.func,
      name: c.name,
      sha256: createHash("sha256").update(c.content).digest("hex"),
    })),
  };
  writeFileSync(
    path.join(stage.cache, "fuzz-session.json"),
    JSON.stringify(session, null, 2),
  );
}

const promoteArgs = ["fuzz", "promote", "fuzzcrash"];
const trim = "FuzzMessageTestSuite_FuzzTrim";

async function promote(stage) {
  const result = await run(bin, promoteArgs, "", {
    cwd: stage.dir,
    env: stage.env,
  });
  // Paths print relative to the stage; Windows spells them with backslashes,
  // which no recorded argument holds.
  const slash = (s) => {
    const masked = s.replaceAll(stage.dir, "<stage>");
    return process.platform === "win32" ? masked.replaceAll("\\", "/") : masked;
  };
  return {
    args: promoteArgs,
    exitCode: result.exitCode,
    stderr: slash(result.stderr),
    stdout: slash(result.stdout),
  };
}

// No session record: nothing to promote.
outputs["promote-nothing"] = await promote(stageFuzzModule());

// A crasher the session wrote is spliced as a seed; a corpus file beside it
// that the session did not write stays.
{
  const stage = stageFuzzModule();
  const crasher = plant(
    stage,
    trim,
    "stale-seed",
    'go test fuzz v1\nstring("stale")\n',
  );
  plant(stage, trim, "planted", 'go test fuzz v1\nstring("planted")\n');
  recordSession(stage, [crasher]);
  const recorded = await promote(stage);
  // The line the confirmation names, as the editor then reveals it.
  const m = /^promoted \S+ -> f\.Add\(.*\) in (.+):(\d+)$/m.exec(
    recorded.stdout,
  );
  recorded.revealedLine = m
    ? readFileSync(path.join(stage.dir, m[1]), "utf-8")
        .split("\n")
        [Number(m[2]) - 1].trim()
    : null;
  outputs["promote-staged"] = recorded;
}

// A crasher changed since the session wrote it: promote refuses the run.
{
  const stage = stageFuzzModule();
  const crasher = plant(
    stage,
    trim,
    "stale-seed",
    'go test fuzz v1\nstring("stale")\n',
  );
  recordSession(stage, [crasher]);
  plant(stage, trim, "stale-seed", 'go test fuzz v1\nstring("swapped")\n');
  outputs["promote-refused"] = await promote(stage);
}

const golden = {
  note: [
    "Recorded contract of `gotest spec --input=- --format=json`, of live",
    "`gotest -json` runs (`live`), and of the other documents the extension",
    "reads (`outputs`).",
    "Generated by scripts/record-cli-contract.mjs — do not hand-edit.",
    "A diff here is a change to an interface the VS Code extension consumes.",
  ],
  cases,
  live,
  outputs,
};

writeFileSync(goldenPath, JSON.stringify(golden, null, 2) + "\n", "utf-8");
console.log(
  `recorded ${Object.keys(cases).length} cases, ${Object.keys(live).length} live runs and ${Object.keys(outputs).length} outputs -> ${path.relative(extensionDir, goldenPath)}`,
);
