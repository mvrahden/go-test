import * as vscode from "vscode";
import * as path from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { resolveGoBinary, fileExists, clearGoBinaryCache } from "./goBinary.js";
import {
  hasReplaceDirective,
  parseToolDirectives,
  readModulePath,
  readModuleRoots,
  requireVersion,
  type ModuleRoot,
} from "./gomod.js";

export { resolveGoBinary } from "./goBinary.js";

const execFileAsync = promisify(execFile);
const DEFAULT_MODULE_PATH = "github.com/mvrahden/go-test/cmd/gotest";
// Raised to the release that introduced `spec --input --render-only`. The Spec
// View passes that flag, so an older CLI would reject the invocation outright.
// Treat this as a contract marker: bump it whenever the extension starts
// depending on CLI behaviour that older versions do not have. Equals the
// CLI's gotestgen.MinRuntimeVersion; a Go test keeps them in step.
const MIN_CLI_VERSION = "v1.27.0";

export interface CliCommand {
  bin: string;
  args: string[];
}

let versionWarningShown = false;

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
    showVersionWarning(
      pin,
      modulePath,
      pinned.map((p) => p.root.dir),
      log,
    );
    throw new CliUnavailableError(
      `go.mod pins gotest ${pin}, but >= ${MIN_CLI_VERSION} is required. ` +
        `Run: go get -tool ${modulePath}@latest`,
    );
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

function meetsFloor(version: string): boolean {
  return compareVersions(version, MIN_CLI_VERSION) >= 0;
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

function showVersionWarning(
  version: string,
  modulePath: string,
  pinnedDirs: string[],
  log?: vscode.LogOutputChannel,
): void {
  if (versionWarningShown) {
    return;
  }
  versionWarningShown = true;
  vscode.window
    .showWarningMessage(
      `gotest: go.mod pins gotest ${version}, but >= ${MIN_CLI_VERSION} is required.`,
      "Upgrade",
    )
    .then(async (choice) => {
      if (choice !== "Upgrade") return;
      try {
        // One go get per pinning module: a go.work root has no module.
        for (const dir of pinnedDirs) {
          await runGoInModule(
            dir,
            ["get", "-tool", `${modulePath}@latest`],
            log,
          );
        }
        log?.info("[cli] upgrade complete");
        vscode.window.showInformationMessage(
          "gotest: dependency upgraded to latest and declared as a tool.",
        );
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err);
        log?.error(`[cli] upgrade failed: ${msg}`);
        vscode.window.showErrorMessage(`gotest: upgrade failed. ${msg}`);
      }
    });
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
