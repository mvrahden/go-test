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
// The `spec` cases replay a stream with the flags that reshape the rendering:
// --declared over a package of this repository, and --hide. The repository's
// directory is masked.
//
// The `live` cases run the CLI itself on packages of this repository and pin
// what a run writes: the -json stream's shape (one start per package before
// its other events, the verdicts of packages it runs nothing in) and a run
// whose patterns match no package. One more runs a module the script writes,
// whose shared fixture never comes up. Times and durations are masked, and
// so are the Go, the checkout and the agent the run's manifest names.
//
// The `outputs` cases pin the other documents the extension reads: the
// `discover` payload, the `bench --json` report, the cover profile a run
// merges for `-coverprofile`, and what `fuzz promote` prints. Absolute paths
// become <repo> or <stage>, and measurements and host facts are zeroed or
// masked. Promote runs on a staged copy of cmd/gotest/testdata/fuzzcrash with
// a session record written here, so no fuzzing happens and the bytes hold.
//
// The `audit` cases record the base and head modules the conformance suite
// audits, each staged with its go.mod bound to this checkout, and compare the
// two records as the Change scope reads them; one more asks a tree outside
// git for a base. One more commits the head and gives it the conformance
// suite's saved runs, diff and profile as evidence, as JSON and as text.
// `version --json` is what the Change scope asks a CLI before it audits.
//
// The `lint` cases run `gotest lint -json` from the fixture module, through
// a go.work that spans this checkout, on packages with known findings and on
// one that does not compile, and print the rule registry. Paths are masked to
// the module root.

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
const testdataDir = path.join(extensionDir, "testdata");
const streamDir = path.join(testdataDir, "streams");
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

// maskDir replaces dir with token, however a string spells it.
function maskDir(text, dir, token) {
  return text
    .split(dir)
    .join(token)
    .split(JSON.stringify(dir).slice(1, -1))
    .join(token);
}

const bin = await buildBinary();
const cases = {};

// Every run here keeps its record in a cache of its own, never the user's.
process.env.GOTEST_CACHE_DIR = tempDir("gotest-contract-cache-");

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

// Replays reshaped by flags, from the repo root: its go.work loads
// examples/search for --declared. The declared stream reports one method of
// that package and one the source does not declare.
const RENDER = ["spec", "--input=-", "--format=json", "--render-only"];
const SPEC = {
  declared: {
    stream: "declared/search.jsonl",
    args: [...RENDER, "--declared", "./examples/search"],
  },
  "hide-pass": {
    stream: "streams/mixed.jsonl",
    args: [...RENDER, "--hide=pass"],
  },
  "hide-nothing": {
    stream: "streams/mixed.jsonl",
    args: [...RENDER, "--hide="],
  },
  // A folder nothing ran in: the declared tree over an empty stream.
  "declared-unrun": {
    stream: null,
    args: [...RENDER, "--hide=", "--declared", "./examples/search"],
  },
};

const spec = {};
for (const [name, { stream, args }] of Object.entries(SPEC)) {
  const input = stream
    ? readFileSync(path.join(testdataDir, stream), "utf-8")
    : "";
  const result = await run(bin, args, input);
  const stdout = result.stdout.trim();
  // The document's directories are masked as paths, so Windows spells them
  // with slashes too, as the Linux recording does.
  spec[name] = {
    stream,
    args,
    exitCode: result.exitCode,
    stderr: maskDir(result.stderr, repoRoot, "<repo>"),
    document: stdout ? maskPaths(JSON.parse(stdout), repoRoot, "<repo>") : null,
  };
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

// mask drops what changes from run to run: times and durations, the host
// facts the run's manifest names (the Go, the checkout and the agent) and
// the input hashes, which move with every commit of the packages run.
function mask(stdout) {
  return stdout
    .split("\n")
    .filter((line) => line.trim() !== "")
    .map((line) => {
      const ev = JSON.parse(line);
      delete ev.Time;
      if ("Elapsed" in ev) ev.Elapsed = 0;
      if ("InputHash" in ev) ev.InputHash = "<hash>";
      if (ev.Manifest) {
        if ("GoVersion" in ev.Manifest) ev.Manifest.GoVersion = "<go>";
        if ("Commit" in ev.Manifest) ev.Manifest.Commit = "<commit>";
        if ("Dirty" in ev.Manifest) ev.Manifest.Dirty = "<dirty>";
        delete ev.Manifest.Agent;
      }
      if (typeof ev.Output === "string") {
        ev.Output = ev.Output.replace(/\d+\.\d+s\b/g, "<d>s");
      }
      return ev;
    });
}

// They build, so a slow runner's build notice must not reach their stderr.
const live = {};
for (const [name, args] of Object.entries(LIVE)) {
  const result = await run(bin, args, "", {
    env: { ...process.env, GOTEST_SLOW_BUILD_THRESHOLD: "1h" },
  });
  live[name] = {
    args,
    exitCode: result.exitCode,
    stderr: result.stderr,
    events: mask(result.stdout),
  };
}

// The `stream` cases pin what an interleaved stream is to the CLI. Each
// testdata/live stream holds the events of its burst twin, the suites
// interleaved as a live run writes them, and must render as the twin does.
// A real --live run of the conformance suite's clilive package, whose two
// suites hold until the script releases them, is recorded per suite: the
// order across suites is the scheduler's, each suite's own is the contract.
const liveDir = path.join(testdataDir, "live");
const stream = { replays: {}, run: null };
for (const file of readdirSync(liveDir).sort()) {
  if (!file.endsWith("-live.jsonl")) continue;
  const name = file.replace(/-live\.jsonl$/, "");
  const live = readFileSync(path.join(liveDir, file), "utf-8");
  const burst = readFileSync(
    path.join(liveDir, `${name}-burst.jsonl`),
    "utf-8",
  );
  stream.replays[name] = {};
  for (const [variant, args] of Object.entries(VARIANTS)) {
    const a = await run(bin, args, live);
    const b = await run(bin, args, burst);
    stream.replays[name][variant] = {
      exitCode: a.exitCode,
      stdoutMatchesBurst: a.stdout === b.stdout && a.exitCode === b.exitCode,
    };
  }
}

// runHeld runs args with clilive's suites holding until both have passed
// their first method, then releases them. held says both first passes were
// on stdout while neither suite had ended.
async function runHeld(args) {
  const release = tempDir("gotest-contract-release-");
  const events = path.join(release, "events.jsonl");
  const child = spawn(bin, [`--events-file=${events}`, ...args], {
    cwd: repoRoot,
    env: {
      ...outputEnv({ LIVE_RELEASE_DIR: release }),
      GOTEST_LIVE: "",
    },
  });
  current = child;
  let stdout = "";
  let stderr = "";
  let held;
  const firstPasses = (text) =>
    ["TestFirstTestSuite", "TestSecondTestSuite"].every((s) =>
      text.includes(
        `"Action":"pass","Package":"${livePkg}","Test":"${s}/TestAPasses"`,
      ),
    );
  const ended = (text) =>
    /"Action":"(pass|fail)","Package":"[^"]*","Test":"Test[A-Za-z]+Suite"/.test(
      text,
    );
  const free = () => {
    for (const name of ["first", "second"])
      writeFileSync(path.join(release, name), "");
  };
  // A run that never shows both passes is released anyway, and says so.
  const giveUp = setTimeout(() => {
    held ??= false;
    free();
  }, 120_000);
  child.stdout.setEncoding("utf-8").on("data", (d) => {
    stdout += d;
    if (held === undefined && firstPasses(stdout)) {
      held = !ended(stdout);
      free();
    }
  });
  child.stderr.setEncoding("utf-8").on("data", (d) => (stderr += d));
  child.stdin.end();
  const status = await new Promise((resolve, reject) => {
    child.on("error", reject);
    child.on("close", resolve);
  });
  clearTimeout(giveUp);
  current = undefined;
  return {
    status,
    stdout,
    stderr,
    held: held ?? false,
    file: readFileSync(events, "utf-8"),
  };
}

const livePkg =
  "github.com/mvrahden/go-test/tests/conformance/testdata/clilive";
const liveArgs = ["-json", "--live", "./tests/conformance/testdata/clilive/"];
// Built and warmed first, so the held run compiles nothing while it waits.
await run(bin, ["-json", "./tests/conformance/testdata/clilive/"], "", {
  env: outputEnv(),
});
{
  const r = await runHeld(liveArgs);
  const events = mask(r.stdout);
  const rootOf = (ev) => (ev.Test ?? "").split("/")[0];
  const suites = {};
  for (const ev of events) {
    if (!ev.Test) continue;
    (suites[rootOf(ev)] ??= []).push(ev);
  }
  const own = events.filter((ev) => !ev.Test && ev.Package === livePkg);
  const lastSuite = Math.max(...events.map((ev, i) => (ev.Test ? i : -1)));
  const verdict = events.findIndex(
    (ev) =>
      !ev.Test &&
      ev.Package === livePkg &&
      (ev.Action === "pass" || ev.Action === "fail"),
  );
  // The same events, each suite whole, as a run without --live writes them.
  const lines = r.stdout.split("\n").filter((l) => l.trim() !== "");
  const regrouped = [
    ...lines.filter(
      (l) => !JSON.parse(l).Test && JSON.parse(l).Action === "start",
    ),
    ...Object.keys(suites)
      .sort()
      .flatMap((s) => lines.filter((l) => rootOf(JSON.parse(l)) === s)),
    ...lines.filter(
      (l) => !JSON.parse(l).Test && JSON.parse(l).Action !== "start",
    ),
  ];
  const render = (text) => run(bin, VARIANTS.renderOnly, text);
  const asLive = await render(r.stdout);
  const asBurst = await render(regrouped.join("\n") + "\n");
  stream.run = {
    args: liveArgs,
    exitCode: r.status,
    stderr: r.stderr,
    heldWhileBothSuitesRan: r.held,
    packageStartsFirst:
      own[0]?.Action === "start" &&
      events.findIndex((ev) => ev.Package === livePkg) ===
        events.findIndex((ev) => ev === own[0]),
    packageVerdictAfterItsSuites: verdict > lastSuite,
    eventsFileMatchesStdout: r.file === r.stdout,
    renderMatchesBurst:
      asLive.exitCode === asBurst.exitCode && asLive.stdout === asBurst.stdout,
    package: own,
    suites: Object.fromEntries(
      Object.keys(suites)
        .sort()
        .map((s) => [s, suites[s]]),
    ),
  };
}

// The environment of the outputs cases: no CI or agent variables, which
// change what the CLI writes, and no slow-build notice on a slow machine.
function outputEnv(extra = {}) {
  const env = { ...process.env, GOTEST_SLOW_BUILD_THRESHOLD: "1h", ...extra };
  for (const name of [
    "AI_AGENT",
    "CLAUDECODE",
    "GOTEST_AGENT",
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

// A module whose one suite reads a shared fixture that never comes up: the
// run books the failure under a package of its own. Its path is fixed, so the
// build cache keys it the same each recording.
const runLevelDir = path.join(tmpdir(), "gotest-contract-runlevel");
const removeRunLevel = () =>
  rmSync(runLevelDir, { recursive: true, force: true });
process.on("exit", removeRunLevel);
removals.push(removeRunLevel);
removeRunLevel();
mkdirSync(path.join(runLevelDir, "down"), { recursive: true });
const RUN_LEVEL_FILES = {
  "go.mod": [
    "module gotest.runlevel",
    "",
    "go 1.26.0",
    "",
    "require github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000",
    "",
    `replace github.com/mvrahden/go-test => ${repoRoot}`,
  ],
  "down/fixture.go": [
    "package down",
    "",
    'import (\n\t"context"\n\t"errors"\n)',
    "",
    "type DownSharedFixture struct{ State string }",
    "",
    "func (f *DownSharedFixture) BeforeAll(ctx context.Context) error {",
    '\treturn errors.New("the fixture would not come up")',
    "}",
    "",
    "func (f *DownSharedFixture) AfterAll(ctx context.Context) error { return nil }",
  ],
  "down/down_test.go": [
    "package down",
    "",
    'import "github.com/mvrahden/go-test/pkg/gotest"',
    "",
    "type DownTestSuite struct{ Probe *DownSharedFixture }",
    "",
    "func (s *DownTestSuite) TestFixtureIsUp(t *gotest.T) {",
    '\tgotest.Equal(t, "up", s.Probe.State)',
    "}",
  ],
};
for (const [file, lines] of Object.entries(RUN_LEVEL_FILES)) {
  writeFileSync(path.join(runLevelDir, file), lines.join("\n") + "\n", "utf-8");
}
const runLevelWork = path.join(path.dirname(bin), "runlevel.work");
writeFileSync(
  runLevelWork,
  `go 1.26.0\n\nuse (\n\t${repoRoot}\n\t${runLevelDir}\n)\n`,
  "utf-8",
);
{
  const args = ["-json", "./..."];
  const result = await run(bin, args, "", {
    cwd: runLevelDir,
    // A slow first build would otherwise add a notice to stderr.
    env: {
      ...process.env,
      GOWORK: runLevelWork,
      GOTEST_SLOW_BUILD_THRESHOLD: "1h",
    },
  });
  live["run-level"] = {
    args,
    exitCode: result.exitCode,
    // Sorted: the fixture's failure and the suite's are written by two
    // processes, in either order.
    stderr: maskDir(result.stderr, runLevelDir, "<module>")
      .split("\n")
      .filter((line) => line !== "")
      .sort()
      .map((line) => line + "\n")
      .join(""),
    events: mask(maskDir(result.stdout, runLevelDir, "<module>")),
  };
  // The same stream rendered as the Spec View renders it: the document marks
  // the run's own package.
  const rendered = await run(bin, [...RENDER, "--hide="], result.stdout);
  live["run-level"].spec = {
    exitCode: rendered.exitCode,
    packages: JSON.parse(rendered.stdout).packages.map((pkg) => ({
      path: pkg.path,
      runLevel: pkg.runLevel ?? false,
    })),
  };
}
removeRunLevel();

const fixturesDir = path.join(extensionDir, "testdata", "fixtures");
const workFile = path.join(path.dirname(bin), "lint.work");
writeFileSync(
  workFile,
  `go 1.26.0\n\nuse (\n\t${repoRoot}\n\t${fixturesDir}\n)\n`,
  "utf-8",
);
const LINT = {
  findings: ["lint", "-json", "./stdlibtests", "./lifecycle"],
  uncompilable: ["lint", "-json", "./broken"],
  rules: ["lint", "-rules", "-json"],
};

// unroot masks the fixture module's directory.
function unroot(text) {
  return maskDir(text, fixturesDir, "<fixtures>");
}

// slashed spells a path the way the Linux recording does; Windows prints
// one below <fixtures> with backslashes, and relative ones in stderr too.
function slashed(text) {
  return process.platform === "win32" ? text.replaceAll("\\", "/") : text;
}

// slashPaths applies slashed to every masked path in a lint document: the
// finding positions and the files its fixes edit.
function slashPaths(value) {
  if (typeof value === "string") {
    return value.startsWith("<fixtures>") ? slashed(value) : value;
  }
  if (Array.isArray(value)) {
    return value.map(slashPaths);
  }
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([k, v]) => [k, slashPaths(v)]),
    );
  }
  return value;
}

const lint = {};
for (const [name, args] of Object.entries(LINT)) {
  const result = await run(bin, args, "", {
    cwd: fixturesDir,
    env: { ...process.env, GOWORK: workFile },
  });
  const stdout = unroot(result.stdout).trim();
  lint[name] = {
    args,
    exitCode: result.exitCode,
    stderr: slashed(unroot(result.stderr)),
    document: stdout ? slashPaths(JSON.parse(stdout)) : null,
  };
}

// Staged at fixed paths, so the build cache keys them the same each time.
const auditRoot = path.join(tmpdir(), "gotest-contract-audit");
const removeAudit = () => rmSync(auditRoot, { recursive: true, force: true });
process.on("exit", removeAudit);
removals.push(removeAudit);
removeAudit();
const auditSource = path.join(
  repoRoot,
  "tests",
  "conformance",
  "testdata",
  "audit",
);
const audit = {};
{
  const env = outputEnv({ GOWORK: "off" });
  const records = {};
  for (const side of ["base", "head"]) {
    const dir = path.join(auditRoot, side);
    cpSync(path.join(auditSource, side), dir, { recursive: true });
    const goMod = path.join(dir, "go.mod");
    writeFileSync(
      goMod,
      readFileSync(goMod, "utf-8").replaceAll("REPO_ROOT", repoRoot),
    );
    const args = ["audit", "--record", "./..."];
    const result = await run(bin, args, "", { cwd: dir, env });
    records[side] = path.join(auditRoot, `${side}.json`);
    writeFileSync(records[side], result.stdout);
    audit[`record-${side}`] = {
      args,
      exitCode: result.exitCode,
      stderr: result.stderr,
    };
  }
  const shown = [
    "audit",
    "--base-record=<base>",
    "--head-record=<head>",
    "--format=json",
  ];
  const args = shown.map((a) =>
    a.replace("<base>", records.base).replace("<head>", records.head),
  );
  const result = await run(bin, args, "", { env });
  audit.compare = {
    args: shown,
    exitCode: result.exitCode,
    stderr: result.stderr,
    ...document(result.stdout, (d) => d),
  };
  // A tree outside git has no commit for --base to name.
  const noGit = ["audit", "--base=HEAD", "--format=json", "./..."];
  const outside = await run(bin, noGit, "", {
    cwd: path.join(auditRoot, "head"),
    env,
  });
  audit["base-outside-git"] = {
    args: noGit,
    exitCode: outside.exitCode,
    stderr: maskDir(outside.stderr, auditRoot, "<stage>"),
    stdout: outside.stdout,
  };

  // The head committed, with the conformance suite's saved runs, diff and
  // profile as its proof; the streams name the head's commit where they ran
  // it. The commit hashes the staged go.mod, so it is masked.
  const proofHead = path.join(auditRoot, "proof-head");
  cpSync(path.join(auditRoot, "head"), proofHead, { recursive: true });
  const gitEnv = {
    ...env,
    GIT_AUTHOR_DATE: "2026-01-01T00:00:00Z",
    GIT_COMMITTER_DATE: "2026-01-01T00:00:00Z",
  };
  const git = async (...args) => {
    const r = await run(
      "git",
      [
        "-c",
        "user.name=gotest",
        "-c",
        "user.email=gotest@example.com",
        "-c",
        "commit.gpgsign=false",
        "-c",
        "init.defaultBranch=trunk",
        ...args,
      ],
      "",
      { cwd: proofHead, env: gitEnv },
    );
    if (r.exitCode !== 0) throw new Error(`git ${args.join(" ")}: ${r.stderr}`);
    return r.stdout.trim();
  };
  await git("init", "-q");
  await git("add", "-A");
  await git("commit", "-q", "-m", "head");
  const commit = await git("rev-parse", "--short", "HEAD");
  const proofDir = path.join(auditRoot, "proof");
  mkdirSync(proofDir, { recursive: true });
  const proofSource = path.join(auditSource, "proof");
  for (const file of readdirSync(proofSource)) {
    writeFileSync(
      path.join(proofDir, file),
      readFileSync(path.join(proofSource, file), "utf-8").replaceAll(
        "HEAD_COMMIT",
        commit,
      ),
    );
  }
  const proofShown = [
    "audit",
    "--base-record=<base>",
    "--evidence=<proof>/another-commit.json",
    "--evidence=<proof>/current.json",
    "--evidence=<proof>/uncommitted.json",
    "--evidence=<proof>/no-manifest.json",
    "--base-evidence=<proof>/base-run.json",
    "--diff=<proof>/shop.diff",
    "--coverage=<proof>/shop.cover",
  ];
  const proofArgs = proofShown.map((a) =>
    a.replace("<base>", records.base).replace("<proof>", proofDir),
  );
  const unstage = (text) =>
    maskDir(maskDir(text, proofDir, "<proof>"), auditRoot, "<stage>")
      .split(commit)
      .join("<commit>");
  for (const [name, format] of [
    ["evidence", ["--format=json"]],
    ["evidence-text", []],
  ]) {
    const result = await run(bin, [...proofArgs, ...format], "", {
      cwd: proofHead,
      env,
    });
    const stdout = unstage(result.stdout);
    audit[name] = {
      args: [...proofShown, ...format],
      exitCode: result.exitCode,
      stderr: unstage(result.stderr),
      ...(format.length > 0 ? document(stdout, (d) => d) : { stdout }),
    };
  }
}
removeAudit();

{
  const result = await run(bin, ["version"], "", { env: outputEnv() });
  outputs.version = {
    args: ["version"],
    exitCode: result.exitCode,
    stderr: result.stderr,
    // The Go and the platform it was built on are the host's.
    stdout: result.stdout.replace(/ go\S+ \S+\/\S+$/m, " <go> <platform>"),
  };
}

{
  const args = ["version", "--json"];
  const result = await run(bin, args, "", { env: outputEnv() });
  outputs["version-json"] = {
    args,
    exitCode: result.exitCode,
    stderr: result.stderr,
    // The Go and the platform it was built on are the host's.
    ...document(result.stdout, (d) => ({
      ...d,
      goVersion: "<go>",
      platform: "<platform>",
    })),
  };
}

// The run records: the conformance suite's saved runs imported into a
// staged module's history, listed as the Runs scope reads them; then a run
// of the module, one of its files changed, the run's stream judged against
// the tree (--check-fresh), and a --changed run's manifest. Ids, times,
// the stage, versions and hashes are masked.
const runsRoot = path.join(tmpdir(), "gotest-contract-runs");
const removeRuns = () => rmSync(runsRoot, { recursive: true, force: true });
process.on("exit", removeRuns);
removals.push(removeRuns);
removeRuns();
const runs = {};
{
  const mod = path.join(runsRoot, "mod");
  cpSync(path.join(auditSource, "head"), mod, { recursive: true });
  const goMod = path.join(mod, "go.mod");
  writeFileSync(
    goMod,
    readFileSync(goMod, "utf-8").replaceAll("REPO_ROOT", repoRoot),
  );
  const env = outputEnv({
    GOWORK: "off",
    GOTEST_CACHE_DIR: path.join(runsRoot, "cache"),
  });
  const proofSource = path.join(auditSource, "proof");
  const unstage = (text) =>
    maskDir(maskDir(text, proofSource, "<proof>"), runsRoot, "<stage>");
  const maskRecord = (r) => ({
    ...r,
    id: "<id>",
    recorded: "<time>",
    // A live run's start is now; an imported one's is its stream's.
    ...(r.started && !r.imported ? { started: "<time>" } : {}),
    ...(r.eventsFile ? { eventsFile: slashed(unstage(r.eventsFile)) } : {}),
    // An input hash covers the Go version and the platform.
    ...(r.ran
      ? {
          ran: r.ran.map((p) => ({
            ...p,
            ...(p.inputHash ? { inputHash: "<hash>" } : {}),
          })),
        }
      : {}),
    manifest: {
      ...r.manifest,
      ...(r.manifest.version ? { version: "<version>" } : {}),
      ...(r.manifest.goVersion ? { goVersion: "<go>" } : {}),
      ...(r.manifest.commit && r.manifest.commit !== "HEAD_COMMIT"
        ? { commit: "<commit>" }
        : {}),
      ...(r.manifest.agent ? { agent: "<agent>" } : {}),
      args: r.manifest.args.map(unstage),
    },
  });
  const list = async (name) => {
    const args = ["runs", "--format=json"];
    const result = await run(bin, args, "", { cwd: mod, env });
    runs[name] = {
      args,
      exitCode: result.exitCode,
      stderr: unstage(result.stderr),
      ...document(result.stdout, (d) => ({
        ...d,
        version: "<version>",
        dir: unstage(d.dir),
        runs: d.runs.map(maskRecord),
      })),
    };
  };
  await list("list-empty");
  for (const name of [
    "no-manifest",
    "another-commit",
    "uncommitted",
    "current",
  ]) {
    const file = path.join(proofSource, `${name}.json`);
    const args = ["runs", `--import=${file}`];
    const result = await run(bin, args, "", { cwd: mod, env });
    runs[`import-${name}`] = {
      args: args.map(unstage),
      exitCode: result.exitCode,
      stderr: unstage(result.stderr),
    };
  }
  await list("list-imported");

  const events = path.join(runsRoot, "run.jsonl");
  const runArgs = [
    "--agent=false",
    `--events-file=${events}`,
    "--",
    "-json",
    "-count=1",
    "./...",
  ];
  const first = await run(bin, runArgs, "", { cwd: mod, env });
  writeFileSync(
    path.join(mod, "shop", "shop.go"),
    readFileSync(path.join(mod, "shop", "shop.go"), "utf-8") +
      "\n// changed after the run\n",
  );
  const freshArgs = [
    "spec",
    "--input=-",
    "--render-only",
    "--check-fresh",
    "--format=json",
    "--hide=fail,none,pass,skip",
  ];
  const fresh = await run(bin, freshArgs, readFileSync(events, "utf-8"), {
    cwd: mod,
    env,
  });
  runs["check-fresh"] = {
    run: { args: runArgs.map(unstage), exitCode: first.exitCode },
    args: freshArgs,
    exitCode: fresh.exitCode,
    stderr: unstage(fresh.stderr),
    ...document(fresh.stdout, (d) => ({
      packages: d.packages.map((p) => ({
        path: p.path,
        freshness: p.freshness,
        inputHash: p.inputHash ? "<hash>" : undefined,
      })),
      stale: d.stats.stale,
    })),
  };
  const changedArgs = [
    "--agent=false",
    "--changed",
    "--",
    "-json",
    "-count=1",
    "./...",
  ];
  const changed = await run(bin, changedArgs, "", { cwd: mod, env });
  const manifest = changed.stdout
    .split("\n")
    .filter((l) => l.includes('"Manifest"'))
    .map((l) => JSON.parse(l).Manifest)[0];
  runs.changed = {
    args: changedArgs,
    exitCode: changed.exitCode,
    manifestChanged: manifest?.Changed ?? null,
  };
  await list("list-after-changed");
}
removeRuns();

const golden = {
  note: [
    "Recorded contract of `gotest spec --input=- --format=json`, of its",
    "--declared and --hide forms (`spec`), of live `gotest -json` runs",
    "(`live`), of interleaved streams and a --live run (`stream`), of the",
    "other documents the extension reads (`outputs`), and",
    "of `gotest lint -json` and `gotest lint -rules -json` (`lint`), and",
    "of `gotest audit` over the conformance suite's base and head (`audit`),",
    "and of `gotest runs`, --check-fresh and --changed over a staged module (`runs`).",
    "Generated by scripts/record-cli-contract.mjs — do not hand-edit.",
    "A diff here is a change to an interface the VS Code extension consumes.",
  ],
  cases,
  spec,
  live,
  stream,
  outputs,
  lint,
  audit,
  runs,
};

writeFileSync(goldenPath, JSON.stringify(golden, null, 2) + "\n", "utf-8");
console.log(
  `recorded ${Object.keys(cases).length} cases, ${Object.keys(spec).length} spec runs, ${Object.keys(live).length} live runs, ${Object.keys(outputs).length} outputs, ${Object.keys(lint).length} lint runs, ${Object.keys(audit).length} audit runs and ${Object.keys(runs).length} runs cases -> ${path.relative(extensionDir, goldenPath)}`,
);
