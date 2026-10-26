import * as vscode from "vscode";
import * as os from "node:os";
import * as path from "node:path";
import { execFile } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { promisify } from "node:util";
import { resolveGoBinary, fileExists, clearGoBinaryCache } from "./goBinary.js";
import {
  goLine,
  hasReplaceDirective,
  parseToolDirectives,
  readModulePath,
  readModuleRoots,
  requiredModule,
  requireVersion,
  type ModuleRoot,
} from "./gomod.js";

export { resolveGoBinary } from "./goBinary.js";

const execFileAsync = promisify(execFile);
const DEFAULT_MODULE_PATH = "github.com/mvrahden/go-test/cmd/gotest";
// v1.34.0 is the release whose generated code calls gotestruntime.AfterEach
// and the shared fixture lifetime, so the CLI's runtime floor moved there
// and this floor with it; v1.34.0 is retracted, so both sit at v1.35.0.
// From v1.31.0 on the stream carries
// every verdict the tree shows, including a shared fixture that failed to
// set up. Every
// JSON document opens with `version` from v1.30.2 on, which is how the
// extension learns the version of a CLI it reaches through go run or go
// tool and gates it (checkProducerVersion). Treat this as a contract
// marker: bump it whenever the extension starts depending on CLI behaviour
// that older versions do not have. Never below the CLI's
// gotestgen.MinRuntimeVersion (a Go test guards the order): a CLI the
// extension accepts must enforce a runtime floor no newer than itself.
const MIN_CLI_VERSION = "v1.35.0";

export interface CliCommand {
  bin: string;
  args: string[];
}

let versionWarningShown = false;
// upgradeState: "asking" while the dialog is open, "upgrading" until the
// upgrade it started has finished, so a floor hit meanwhile starts no second
// go get in the same module.
let upgradeState: "idle" | "asking" | "upgrading" = "idle";

export async function validateGoBinary(
  log?: vscode.LogOutputChannel,
  workspaceDir?: string,
): Promise<string | undefined> {
  const goBin = await resolveGoBinary(log, workspaceDir);
  try {
    const { stdout } = await execFileAsync(goBin, ["version"], {
      timeout: 5_000,
      cwd: workspaceDir,
    });
    log?.debug(`[go] binary validated: ${stdout.trim()}`);
    return goBin;
  } catch {
    log?.warn(`[go] binary "${goBin}" failed validation`);
    clearGoBinaryCache();
    return undefined;
  }
}

export async function buildCliCommand(
  subcommandArgs: string[],
  workspaceDir?: string,
  log?: vscode.LogOutputChannel,
): Promise<CliCommand> {
  const config = scopedConfig(workspaceDir);

  const buildTags = config.get<string>("buildTags", "").trim();
  if (buildTags) {
    subcommandArgs = [...subcommandArgs, `-tags=${buildTags}`];
  }

  const modulePath = config.get<string>("modulePath") ?? DEFAULT_MODULE_PATH;
  const effectiveDir =
    workspaceDir ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;

  // 1. Explicit cliPath override — only trusted when user-provided and version-valid
  const cliPath = config.get<string>("cliPath", "").trim();
  if (cliPath) {
    const resolved = resolveCliPath(cliPath, workspaceDir);
    if (await fileExists(resolved)) {
      const version = await queryBinaryVersion(resolved, log);
      if (version && compareVersions(version, MIN_CLI_VERSION) >= 0) {
        log?.debug(`[cli] cliPath override: ${resolved} (${version})`);
        return { bin: resolved, args: subcommandArgs };
      }
      if (version) {
        log?.warn(
          `[cli] cliPath "${resolved}" is ${version}, requires >= ${MIN_CLI_VERSION} — falling back to go run`,
        );
      } else {
        log?.warn(
          `[cli] cliPath "${resolved}" failed version check — falling back to go run`,
        );
      }
    } else {
      log?.debug(`[cli] cliPath "${resolved}" not found, probing alternatives`);
    }
  }

  // 2. Workspace IS the gotest module (development / go.work overlap)
  if (effectiveDir) {
    const declaredModule = await readModulePath(effectiveDir);
    if (
      declaredModule &&
      (modulePath === declaredModule ||
        modulePath.startsWith(declaredModule + "/"))
    ) {
      const goBin = await resolveGoBinary(log, workspaceDir);
      const relPath = "./" + modulePath.slice(declaredModule.length + 1);
      log?.debug(`[cli] workspace is gotest module: ${goBin} run ${relPath}`);
      return {
        bin: goBin,
        args: ["run", relPath, ...subcommandArgs],
      };
    }
  }

  // 3. Module roots: the go.work `use` modules, else the folder's go.mod.
  // A workspace builds from one list: the highest pin wins and a tool
  // declared by any module is available at the root.
  const roots =
    effectiveDir && !modulePath.includes("@")
      ? await readModuleRoots(effectiveDir)
      : [];
  const pinned = roots
    .map((root) => ({ root, version: requireVersion(root.goMod, modulePath) }))
    .filter((p): p is { root: ModuleRoot; version: string } => !!p.version);
  const pin = pinned
    .map((p) => p.version)
    .sort(compareVersions)
    .at(-1);
  const replaced = roots.some((r) => hasReplaceDirective(r.goMod, modulePath));
  const toolDeclared = roots.some((r) =>
    parseToolDirectives(r.goMod).includes(modulePath),
  );

  // 4. Tool directive → go tool <package path>. Go builds it from the
  // module's own build list, so replace applies. The full path is deliberate:
  // a short name is shadowed by built-in tools and can be ambiguous.
  // Checked before the version gate because a replaced module's require
  // version is a placeholder.
  if (toolDeclared && (replaced || (pin && meetsFloor(pin)))) {
    const goBin = await resolveGoBinary(log, workspaceDir);
    log?.debug(`[cli] go.mod declares the tool: ${goBin} tool ${modulePath}`);
    return { bin: goBin, args: ["tool", modulePath, ...subcommandArgs] };
  }

  // 5. Replace directive → go run without version. The require version
  // beside a replace is a placeholder, so it is not gated.
  if (replaced) {
    const goBin = await resolveGoBinary(log, workspaceDir);
    log?.debug(
      `[cli] go.mod has replace directive: ${goBin} run ${modulePath}`,
    );
    return { bin: goBin, args: ["run", modulePath, ...subcommandArgs] };
  }

  // 6. Project-pinned version from go.mod
  if (pin && effectiveDir) {
    if (meetsFloor(pin)) {
      const goBin = await resolveGoBinary(log, workspaceDir);
      const qualified = `${modulePath}@${pin}`;
      log?.debug(`[cli] using go.mod: ${goBin} run ${qualified}`);
      suggestToolDirective(effectiveDir, modulePath, pinned, log);
      return { bin: goBin, args: ["run", qualified, ...subcommandArgs] };
    }
    // Below the floor a newer CLI would generate code the pinned runtime
    // cannot compile, so refuse rather than substitute one.
    log?.warn(`[cli] go.mod pins ${pin}, requires >= ${MIN_CLI_VERSION}`);
    const err = new CliBelowFloorError(
      pin,
      modulePath,
      pinned.map((p) => p.root.dir),
      log,
    );
    if (!versionWarningShown) {
      versionWarningShown = true;
      showUpgradeDialog(err);
    }
    throw err;
  }

  // 7. Fallback: go run @latest, only when no module requires gotest yet
  // (scaffold runs before a pin exists) or modulePath carries its own @version.
  const goBin = await resolveGoBinary(log, workspaceDir);
  const qualified = modulePath.includes("@")
    ? modulePath
    : `${modulePath}@latest`;
  log?.debug(`[cli] using fallback: ${goBin} run ${qualified}`);
  return { bin: goBin, args: ["run", qualified, ...subcommandArgs] };
}

// CliUnavailableError carries the user-facing fix when no CLI can run.
export class CliUnavailableError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "CliUnavailableError";
  }
}

// CliBelowFloorError is a go.mod pin below the floor. The upgrade dialog
// already tells the user, so callers show no toast of their own. The advice
// names the floor, the smallest move that satisfies it: @latest could raise
// the go line further than needed. The dialog picks the newest that fits.
export class CliBelowFloorError extends CliUnavailableError {
  constructor(
    readonly pin: string,
    readonly modulePath: string,
    readonly pinnedDirs: string[],
    readonly log?: vscode.LogOutputChannel,
  ) {
    super(
      `go.mod pins gotest ${pin}, but >= ${MIN_CLI_VERSION} is required. ` +
        `Run: go get -tool ${modulePath}@${MIN_CLI_VERSION}`,
    );
    this.name = "CliBelowFloorError";
  }
}

// offerCliUpgrade answers a user action that failed below the floor with the
// upgrade dialog, unless one is already open; while an upgrade runs it says
// so instead. It reports whether err was that case, so the caller can skip
// its own toast.
export function offerCliUpgrade(err: unknown): boolean {
  if (!(err instanceof CliBelowFloorError)) return false;
  showUpgradeDialog(err);
  return true;
}

function meetsFloor(version: string): boolean {
  return compareVersions(version, MIN_CLI_VERSION) >= 0;
}

// checkProducerVersion gates a CLI that could not be versioned up front: a
// go run, go tool or replace-directive build first says who it is in the
// document it writes. Every document opens with `version` from v1.30.2 on,
// so one without it predates the floor. A dev build — a source checkout or
// a replace directive — is current by construction.
export function checkProducerVersion(
  version: string | undefined,
  workspaceDir?: string,
): void {
  if (version === undefined) {
    throw new CliUnavailableError(
      `the CLI wrote no version, so it predates ${MIN_CLI_VERSION}. ${upgradeAdvice(workspaceDir)}`,
    );
  }
  if (version.startsWith("dev") || meetsFloor(version)) {
    return;
  }
  throw new CliUnavailableError(
    `the CLI is ${version}, but >= ${MIN_CLI_VERSION} is required. ${upgradeAdvice(workspaceDir)}`,
  );
}

// upgradeAdvice names the floor for the reason CliBelowFloorError does,
// without asking the module proxy while an error is being built.
function upgradeAdvice(workspaceDir?: string): string {
  const modulePath =
    scopedConfig(workspaceDir).get<string>("modulePath") ?? DEFAULT_MODULE_PATH;
  return `Run: go get -tool ${modulePath}@${MIN_CLI_VERSION}`;
}

function resolveCliPath(cliPath: string, workspaceDir?: string): string {
  if (path.isAbsolute(cliPath)) {
    return cliPath;
  }
  if (workspaceDir) {
    return path.resolve(workspaceDir, cliPath);
  }
  const wsFolder = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (wsFolder) {
    return path.resolve(wsFolder, cliPath);
  }
  return cliPath;
}

function showUpgradeDialog(err: CliBelowFloorError): void {
  if (upgradeState === "asking") return;
  if (upgradeState === "upgrading") {
    vscode.window.showInformationMessage(
      "gotest: the CLI upgrade is still running; try again when it finishes.",
    );
    return;
  }
  upgradeState = "asking";
  vscode.window
    .showWarningMessage(
      `gotest: go.mod pins gotest ${err.pin}, but >= ${MIN_CLI_VERSION} is required.`,
      "Upgrade",
    )
    .then(async (choice) => {
      if (choice !== "Upgrade") {
        upgradeState = "idle";
        return;
      }
      upgradeState = "upgrading";
      try {
        await upgradeCli(err);
      } finally {
        upgradeState = "idle";
      }
    });
}

// upgradeCli moves each pinning module to the newest release its go line
// takes. A module no release fits is named, not upgraded: raising a
// project's Go is the project's call, so there is no button for it.
async function upgradeCli(err: CliBelowFloorError): Promise<void> {
  const { modulePath, pinnedDirs, log } = err;
  const plan = await vscode.window.withProgress(
    {
      location: vscode.ProgressLocation.Notification,
      title: "gotest: looking up gotest releases",
      cancellable: true,
    },
    (_progress, token) => lookUpUpgrade(err, token),
  );
  if (!plan) return;
  const where = (dir: string) =>
    pinnedDirs.length > 1 ? `${path.basename(dir)}/go.mod` : "go.mod";
  if (plan.blocked.length > 0) {
    const declared = plan.blocked
      .map((b) => `${where(b.dir)} declares go ${b.goLine}`)
      .join(", ");
    log?.warn(`[cli] upgrade blocked: ${declared}`);
    vscode.window.showWarningMessage(
      `gotest: ${MIN_CLI_VERSION} and newer need go ${plan.needs.goLine}, but ${declared}. ` +
        `Raise the go line to upgrade: go get -tool ${modulePath}@${plan.needs.release} raises it.`,
    );
  }
  if (plan.upgrades.length === 0) return;
  const releases = new Set(plan.upgrades.map((u) => u.release));
  const to =
    releases.size === 1
      ? plan.upgrades[0].release
      : plan.upgrades.map((u) => `${u.release} in ${where(u.dir)}`).join(", ");
  try {
    // Not cancellable: a go get stopped midway could leave go.mod and
    // go.sum apart. One go get per pinning module: a go.work root has none.
    await vscode.window.withProgress(
      {
        location: vscode.ProgressLocation.Notification,
        title: `gotest: upgrading to ${to}`,
      },
      async () => {
        for (const { dir, release } of plan.upgrades) {
          await runGoInModule(
            dir,
            ["get", "-tool", `${modulePath}@${release}`],
            log,
          );
        }
      },
    );
    log?.info(`[cli] upgrade complete: ${to}`);
    vscode.window.showInformationMessage(
      `gotest: upgraded to ${to} and declared as a tool.`,
    );
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    log?.error(`[cli] upgrade failed: ${msg}`);
    vscode.window.showErrorMessage(`gotest: upgrade failed. ${msg}`);
  }
}

// lookUpUpgrade plans the upgrade, or says why there is none: undefined
// after a failed or cancelled lookup, which leaves go.mod unchanged.
async function lookUpUpgrade(
  { modulePath, pinnedDirs, log }: CliBelowFloorError,
  token: vscode.CancellationToken,
): Promise<UpgradePlan | undefined> {
  const cancel = new AbortController();
  const sub = token.onCancellationRequested(() =>
    cancel.abort(new Error("cancelled")),
  );
  try {
    const modules = await Promise.all(
      pinnedDirs.map(async (dir) => {
        const goMod = await readFile(path.join(dir, "go.mod"), "utf-8");
        return { dir, goMod };
      }),
    );
    const module = modules
      .map((m) => requiredModule(m.goMod, modulePath)?.path)
      .find((p) => p !== undefined);
    if (!module) throw new Error(`no go.mod requires ${modulePath}`);
    const goBin = await resolveGoBinary(log, pinnedDirs[0]);
    return await withLookupModule((dir) =>
      planUpgrade(
        modules.map((m) => ({ dir: m.dir, goLine: goLine(m.goMod) })),
        module,
        MIN_CLI_VERSION,
        goReleaseQuery(goBin, dir, cancel.signal, log),
      ),
    );
  } catch (err: unknown) {
    if (token.isCancellationRequested) {
      log?.info("[cli] release lookup cancelled; go.mod is unchanged");
      return undefined;
    }
    const msg = err instanceof Error ? err.message : String(err);
    log?.error(`[cli] release lookup failed: ${msg}`);
    vscode.window.showErrorMessage(
      `gotest: upgrade failed, go.mod is unchanged: could not look up gotest releases. ${msg}`,
    );
    return undefined;
  } finally {
    sub.dispose();
  }
}

// ReleaseQuery asks the module proxy for a module's releases and for the
// go line each of the given versions declares. goLines leaves out versions
// it could not resolve.
export interface ReleaseQuery {
  versions(module: string): Promise<string[]>;
  goLines(module: string, versions: string[]): Promise<Map<string, string>>;
}

export interface UpgradePlan {
  upgrades: { dir: string; release: string }[];
  // blocked: modules whose go line no release at or above the floor fits.
  blocked: { dir: string; goLine: string }[];
  // needs: the newest release with the lowest go line at or above the floor.
  needs: { release: string; goLine: string };
}

// planUpgrade picks per module the newest release at or above the floor
// whose go line fits the module's (goLineFits): go get raises a module's go
// line to the release's. Every candidate is checked, since go lines need
// not rise with releases; few sit above the floor, and one go list asks
// for all of them. A candidate without a go line is skipped: every gotest
// release declares one, so go list could not resolve it or read its go.mod,
// and go get would fail on it too.
export async function planUpgrade(
  modules: { dir: string; goLine: string }[],
  module: string,
  floor: string,
  query: ReleaseQuery,
): Promise<UpgradePlan> {
  const candidates = (await query.versions(module))
    .filter((v) => /^v\d+\.\d+\.\d+$/.test(v) && compareVersions(v, floor) >= 0)
    .sort((a, b) => compareVersions(b, a));
  if (candidates.length === 0) {
    throw new Error(
      `no release of ${module} at or above ${floor} is published`,
    );
  }
  const lines = await query.goLines(module, candidates);
  const resolved = candidates
    .map((release) => ({ release, goLine: lines.get(release) ?? "" }))
    .filter((c) => c.goLine !== "");
  if (resolved.length === 0) {
    throw new Error(
      `none of the releases of ${module} at or above ${floor} could be resolved: ${candidates.join(", ")}`,
    );
  }
  // Newest first, so a tie on the go line keeps the newest release.
  const needs = resolved.reduce((min, c) =>
    compareGoVersions(c.goLine, min.goLine) < 0 ? c : min,
  );
  const plan: UpgradePlan = { upgrades: [], blocked: [], needs };
  for (const m of modules) {
    const fit = resolved.find((c) => goLineFits(c.goLine, m.goLine));
    if (fit) plan.upgrades.push({ dir: m.dir, release: fit.release });
    else plan.blocked.push(m);
  }
  return plan;
}

// goLineFits reports whether a module at go line `module` takes a release
// declaring `release` without moving to a newer Go. Go orders 1.26 before
// 1.26.0, and go get rewrites the one to the other, but both name language
// version 1.26: the rewrite only drops 1.26 pre-release toolchains. So a
// module at 1.N (or 1.NrcK) takes a release at 1.N.0, though not 1.N.1.
export function goLineFits(release: string, module: string): boolean {
  if (compareGoVersions(release, module) <= 0) return true;
  const lang = /^(\d+\.\d+)(?:(?:alpha|beta|rc)\d+)?$/.exec(module)?.[1];
  return lang !== undefined && release === `${lang}.0`;
}

const LOOKUP_TIMEOUT_MS = 60_000;
const LOOKUP_MODULE = "module gotest.upgrade.lookup\n";

// withLookupModule runs fn in a fresh directory holding its own go.mod, so
// no module around the temp dir shapes the lookup. Without a go line it
// never asks for a toolchain switch.
async function withLookupModule<T>(fn: (dir: string) => Promise<T>) {
  const dir = await mkdtemp(path.join(os.tmpdir(), "gotest-upgrade-"));
  try {
    await writeFile(path.join(dir, "go.mod"), LOOKUP_MODULE);
    return await fn(dir);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
}

// lookupGoFlags drops the user's module-shaping flags, -mod and -modfile,
// and sets -mod=mod: a vendor-mode lookup cannot list versions. It is never
// empty, since an empty GOFLAGS falls back to the go env file's.
export function lookupGoFlags(goflags: string): string {
  return [
    ...goflags
      .split(/\s+/)
      .filter((f) => f !== "" && !/^--?mod(file)?(=|$)/.test(f)),
    "-mod=mod",
  ].join(" ");
}

// goReleaseQuery runs go list in the lookup module with GOWORK=off, so
// neither the user's go.mod, go.sum, vendoring nor a go.work shapes the
// answer; network settings (GOPROXY, GOPRIVATE, GONOSUMDB…) still apply.
// Each go command ends on signal or after LOOKUP_TIMEOUT_MS.
export function goReleaseQuery(
  goBin: string,
  dir: string,
  signal: AbortSignal,
  log?: vscode.LogOutputChannel,
): ReleaseQuery {
  const run = async (args: string[], env: NodeJS.ProcessEnv) => {
    const stop = new AbortController();
    const onAbort = () => stop.abort(signal.reason);
    signal.addEventListener("abort", onAbort);
    const timer = setTimeout(
      () =>
        stop.abort(
          new Error(
            `go ${args[0]} timed out after ${LOOKUP_TIMEOUT_MS / 1000}s`,
          ),
        ),
      LOOKUP_TIMEOUT_MS,
    );
    try {
      if (signal.aborted) throw signal.reason;
      const { stdout } = await execFileAsync(goBin, args, {
        cwd: dir,
        env,
        signal: stop.signal,
      });
      return stdout;
    } catch (err: unknown) {
      throw stop.signal.aborted ? stop.signal.reason : err;
    } finally {
      clearTimeout(timer);
      signal.removeEventListener("abort", onAbort);
    }
  };
  let env: Promise<NodeJS.ProcessEnv> | undefined;
  const list = async (args: string[]) => {
    env ??= run(["env", "GOFLAGS"], { ...process.env, GOWORK: "off" }).then(
      (goflags) => ({
        ...process.env,
        GOWORK: "off",
        GOFLAGS: lookupGoFlags(goflags.trim()),
      }),
    );
    return run(["list", "-m", "-json", ...args], await env);
  };
  return {
    versions: async (module) => {
      const out = JSON.parse(await list(["-versions", module])) as {
        Versions?: string[];
      };
      return out.Versions ?? [];
    },
    goLines: async (module, versions) => {
      // -e reports a release go list cannot resolve in its Error field
      // instead of failing the whole batch.
      const out = await list(["-e", ...versions.map((v) => `${module}@${v}`)]);
      const lines = new Map<string, string>();
      for (const m of parseJsonStream(out)) {
        if (m.Error) {
          log?.warn(`[cli] skipping ${m.Path}@${m.Version}: ${m.Error.Err}`);
        } else if (m.Version && m.GoVersion) {
          lines.set(m.Version, m.GoVersion);
        } else {
          log?.warn(`[cli] skipping ${m.Path}@${m.Version}: no go line`);
        }
      }
      return lines;
    },
  };
}

interface ListedModule {
  Path?: string;
  Version?: string;
  GoVersion?: string;
  Error?: { Err: string };
}

// parseJsonStream splits go list -json output: indented objects, each
// closed by a brace at the start of a line.
function parseJsonStream(out: string): ListedModule[] {
  return out
    .split(/^}$/m)
    .filter((chunk) => chunk.trim() !== "")
    .map((chunk) => JSON.parse(chunk + "}") as ListedModule);
}

// compareGoVersions orders go lines as Go does: 1.21 < 1.21rc1 < 1.21.0.
// An empty go line, a module without a go directive, sorts first.
export function compareGoVersions(a: string, b: string): number {
  const key = (v: string): number[] => {
    const m = /^(\d+)(?:\.(\d+))?(?:\.(\d+)|(alpha|beta|rc)(\d+))?$/.exec(v);
    if (!m) return [];
    const [, major, minor = "0", patch, pre, preNum] = m;
    const tier =
      patch !== undefined
        ? [3, Number(patch)]
        : pre
          ? [["alpha", "beta", "rc"].indexOf(pre), Number(preNum)]
          : [-1, 0];
    return [Number(major), Number(minor), ...tier];
  };
  const ka = key(a);
  const kb = key(b);
  for (let i = 0; i < Math.max(ka.length, kb.length); i++) {
    const diff = (ka[i] ?? -Infinity) - (kb[i] ?? -Infinity);
    if (diff !== 0) return diff;
  }
  return 0;
}

// suggestedRoots: folders already offered the directive this session.
const suggestedRoots = new Set<string>();

// suggestToolDirective offers once per folder to declare the pinned CLI as a
// tool, at the pinned version. Silent in vendored modules, where a go.mod
// edit breaks every build until `go mod vendor` reruns.
function suggestToolDirective(
  effectiveDir: string,
  modulePath: string,
  pinned: { root: ModuleRoot; version: string }[],
  log?: vscode.LogOutputChannel,
): void {
  if (suggestedRoots.has(effectiveDir)) return;
  suggestedRoots.add(effectiveDir);
  const config = scopedConfig(effectiveDir);
  if (!config.get<boolean>("suggestToolDirective", true)) return;
  void (async () => {
    for (const p of pinned) {
      if (await isVendored(p.root.dir)) return;
    }
    const first = pinned[0];
    const choice = await vscode.window.showInformationMessage(
      `gotest: declare the CLI as a tool so the editor runs it through Go's tool directive ` +
        `(faster, offline, replace-aware): go get -tool ${modulePath}@${first.version}`,
      "Add to go.mod",
      "Not now",
      "Don't ask again",
    );
    if (choice === "Don't ask again") {
      await config.update(
        "suggestToolDirective",
        false,
        vscode.ConfigurationTarget.WorkspaceFolder,
      );
      return;
    }
    if (choice !== "Add to go.mod") return;
    try {
      for (const p of pinned) {
        await runGoInModule(
          p.root.dir,
          ["get", "-tool", `${modulePath}@${p.version}`],
          log,
        );
      }
      vscode.window.showInformationMessage(
        "gotest: tool directive added; the editor now runs go tool gotest.",
      );
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      log?.error(`[cli] go get -tool failed: ${msg}`);
      vscode.window.showErrorMessage(`gotest: adding the tool failed. ${msg}`);
    }
  })();
}

async function isVendored(dir: string): Promise<boolean> {
  return fileExists(path.join(dir, "vendor", "modules.txt"));
}

// runGoInModule runs a go command in a module, re-vendoring when it vendors.
async function runGoInModule(
  dir: string,
  args: string[],
  log?: vscode.LogOutputChannel,
): Promise<void> {
  const goBin = await resolveGoBinary(log, dir);
  log?.info(`[cli] ${goBin} ${args.join(" ")} (cwd ${dir})`);
  await execFileAsync(goBin, args, { cwd: dir, timeout: 60_000 });
  if (await isVendored(dir)) {
    log?.info(`[cli] ${goBin} mod vendor (cwd ${dir})`);
    await execFileAsync(goBin, ["mod", "vendor"], {
      cwd: dir,
      timeout: 60_000,
    });
  }
}

async function queryBinaryVersion(
  binPath: string,
  log?: vscode.LogOutputChannel,
): Promise<string | undefined> {
  try {
    const { stdout } = await execFileAsync(binPath, ["version"], {
      timeout: 5_000,
    });
    const match = /^gotest\s+(v\S+)/m.exec(stdout);
    if (match) {
      return match[1];
    }
    log?.debug(
      `[cli] unexpected version output from ${binPath}: ${stdout.trim()}`,
    );
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    log?.debug(`[cli] version check failed for ${binPath}: ${msg}`);
  }
  return undefined;
}

// compareVersions orders by major.minor.patch; pre-release and build suffixes
// are dropped so a pseudo-version sorts by its base version.
export function compareVersions(a: string, b: string): number {
  const parse = (v: string) =>
    v
      .replace(/^v/, "")
      .replace(/[-+].*$/, "")
      .split(".")
      .map(Number);
  const pa = parse(a);
  const pb = parse(b);
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const diff = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (diff !== 0) return diff;
  }
  return 0;
}

export { escapeRegExp } from "./gomod.js";

export function formatCliCommand(cmd: CliCommand): string {
  return `${cmd.bin} ${cmd.args.join(" ")}`;
}

// buildBenchArgs constructs the `gotest bench` subcommand arguments for a
// single suite. The generated wrapper is named "Benchmark<Suite>" and runs
// each method under b.Run with its method name, so go test's slash matching
// scopes runs: "-bench=^Benchmark<Suite>$" runs the whole suite, and
// "-bench=^Benchmark<Suite>$/^<Method>$" a single method. Always the =
// form: the CLI pairs space-separated values too, but = keeps the argv
// unambiguous.
export function buildBenchArgs(
  importPath: string,
  suiteName: string,
  methodName?: string,
): string[] {
  const pattern = methodName
    ? `^Benchmark${suiteName}$/^${methodName}$`
    : `^Benchmark${suiteName}$`;
  return ["bench", importPath, `-bench=${pattern}`];
}

// buildFuzzArgs constructs the `gotest fuzz` arguments for one target: the
// generated wrapper is Fuzz<Suite>_<Method>, selected exactly via --target.
// A budget becomes --for (approximate wall-clock; with one target it is
// that target's -fuzztime). --timeout=0 always: the session is bounded by
// its --for budget or by the user's cancel, and the CLI's default 15m
// pipeline deadline must neither end an "until stopped" session behind the
// user's back nor reject a long --for up front.
export function buildFuzzArgs(
  importPath: string,
  suiteName: string,
  methodName: string,
  budget?: string,
): string[] {
  // --for is the session's only clock (the CLI refuses --timeout here);
  // "until stopped" is its explicit opt-out, and cancellation the only end.
  return [
    "fuzz",
    importPath,
    `--target=Fuzz${suiteName}_${methodName}`,
    budget ? `--for=${budget}` : "--for=0",
  ];
}

export function scopedConfig(
  workspaceDir?: string,
): vscode.WorkspaceConfiguration {
  const scope = workspaceDir ? vscode.Uri.file(workspaceDir) : undefined;
  return vscode.workspace.getConfiguration("gotest", scope);
}

export function clearBinaryCache(): void {
  clearGoBinaryCache();
  versionWarningShown = false;
  suggestedRoots.clear();
}

// stripGoRunExitEcho drops the "exit status N" line `go run` appends to stderr
// after any non-zero child. It restates the code the caller already has and
// would otherwise bury the real diagnostic under it.
export function stripGoRunExitEcho(stderr: string): string {
  return stderr
    .split("\n")
    .filter((line) => !/^exit status \d+$/.test(line.trim()))
    .join("\n")
    .trim();
}
