import * as vscode from "vscode";
import * as path from "node:path";
import { killProcessTree } from "./processTree.js";
import { ManagedChild } from "./managedChild.js";
import { readFile } from "node:fs/promises";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import type { TestResultStore } from "./testResultStore.js";
import {
  extractDiagnosticLocation,
  extractTestMessages,
  isPackageSummaryLine,
  parseExpectedActual,
  type TestEvent,
} from "./outputParser.js";
import { EXTERNAL_ID } from "./types.js";

export { killProcessTree };

export function enqueueDescendants(
  run: vscode.TestRun,
  item: vscode.TestItem,
): void {
  item.children.forEach((child) => {
    run.enqueued(child);
    enqueueDescendants(run, child);
  });
}

export function skipUnresolved(
  run: vscode.TestRun,
  item: vscode.TestItem,
  controller: GoTestController,
): void {
  item.children.forEach((child) => {
    skipUnresolved(run, child, controller);
    if (!controller.getResult(child.id)) {
      run.skipped(child);
    }
  });
}

export function startAncestors(
  run: vscode.TestRun,
  items: vscode.TestItem[],
): void {
  const seen = new Set<string>();
  for (const item of items) {
    let ancestor = item.parent;
    while (ancestor) {
      if (seen.has(ancestor.id)) break;
      run.started(ancestor);
      seen.add(ancestor.id);
      ancestor = ancestor.parent;
    }
  }
}

export function collectItems(
  controller: GoTestController,
  request: vscode.TestRunRequest,
): vscode.TestItem[] {
  const items: vscode.TestItem[] = [];
  if (request.include && request.include.length > 0) {
    for (const item of request.include) {
      items.push(item);
    }
  } else {
    controller.testController.items.forEach((item) => {
      items.push(item);
    });
  }
  return expandToPackages(items);
}

export function groupByPackage(
  items: vscode.TestItem[],
): Map<string, vscode.TestItem[]> {
  const groups = new Map<string, vscode.TestItem[]>();
  for (const item of items) {
    const root = getPackageItem(item);
    let group = groups.get(root.id);
    if (!group) {
      group = [];
      groups.set(root.id, group);
    }
    group.push(item);
  }
  return groups;
}

export function getRootItem(item: vscode.TestItem): vscode.TestItem {
  let current = item;
  while (current.parent) {
    current = current.parent;
  }
  return current;
}

export function getItemDepth(item: vscode.TestItem): number {
  let depth = 0;
  let current = item;
  while (current.parent) {
    current = current.parent;
    depth++;
  }
  return depth;
}

const PACKAGE_TAG = "package";

export { EXTERNAL_ID } from "./types.js";

export function getPackageItem(item: vscode.TestItem): vscode.TestItem {
  let current: vscode.TestItem = item;
  if (current.tags.some((t) => t.id === PACKAGE_TAG)) {
    return current;
  }
  while (current.parent) {
    current = current.parent;
    if (current.tags.some((t) => t.id === PACKAGE_TAG)) {
      return current;
    }
  }
  return current;
}

export function getPackageDepth(item: vscode.TestItem): number {
  let depth = 0;
  let current: vscode.TestItem | undefined = item;
  while (current) {
    if (current.tags.some((t) => t.id === PACKAGE_TAG)) {
      return depth;
    }
    depth++;
    current = current.parent;
  }
  return -1;
}

export function expandToPackages(items: vscode.TestItem[]): vscode.TestItem[] {
  const result: vscode.TestItem[] = [];
  const visit = (item: vscode.TestItem) => {
    if (item.tags.some((t) => t.id === PACKAGE_TAG)) {
      result.push(item);
      return;
    }
    item.children.forEach((child) => visit(child));
  };
  for (const item of items) {
    visit(item);
  }
  return result.length > 0 ? result : items;
}

// OnceLog says each message once: a run's routers share one.
export class OnceLog {
  private readonly said = new Set<string>();

  constructor(private readonly sink?: (message: string) => void) {}

  note(message: string): void {
    if (this.said.has(message)) return;
    this.said.add(message);
    this.sink?.(message);
  }
}

export const UNSTAMPED_LIVE =
  "[runner] unstamped subtest events; CLI below the floor?";

// TestItemRouter finds the Test Explorer item a package's events belong to,
// by where the CLI says each test sits: a name's slashes cannot say it, since
// a label may hold slashes. One router per package per run: later events of
// a test are looked up by the item its run placed.
export class TestItemRouter {
  private readonly placed = new Map<string, vscode.TestItem | undefined>();
  // Per parent item and written level: the run that first reached it, the
  // own levels it reported in order, and how many each run has placed.
  private readonly occurrences = new Map<
    string,
    { first: number; owns: string[]; counts: Map<number, number> }
  >();

  constructor(
    private readonly controller: GoTestController,
    private readonly importPath: string,
    private readonly log: OnceLog = new OnceLog(),
    // What is said of subtest events without stamps: a live run's CLI is
    // below the floor, an imported stream is one the CLI could not place.
    private readonly unstamped = UNSTAMPED_LIVE,
  ) {}

  // The item an event's results belong to, by its stamps.
  route(event: TestEvent): vscode.TestItem | undefined {
    const test = event.Test;
    if (!test) return undefined;
    const instance = event.Gotest?.Instance ?? 1;
    const key = `${instance}\u0000${test}`;
    if (event.Action !== "run") {
      if (this.placed.has(key)) return this.placed.get(key);
      // A top-level name holds no slash; its item needs no run to find.
      return test.includes("/") ? undefined : this.topLevel(test, instance);
    }
    const item = this.place(test, event.Gotest, instance);
    this.placed.set(key, item);
    return item;
  }

  private place(
    test: string,
    stamp: TestEvent["Gotest"],
    instance: number,
  ): vscode.TestItem | undefined {
    const own = stamp?.Subtest;
    if (own === undefined || own === "") {
      if (!test.includes("/")) return this.topLevel(test, instance);
      this.log.note(this.unstamped);
      return undefined;
    }
    if (own === test) return this.topLevel(test, instance);
    if (!test.endsWith("/" + own)) return undefined;
    const parentName = test.slice(0, test.length - own.length - 1);
    const parent = this.placed.get(`${instance}\u0000${parentName}`);
    if (!parent) return undefined;
    // The repeat suffix testing appended is part of the id, not the label.
    const suffix = stamp?.Repeat
      ? `#${String(stamp.Repeat).padStart(2, "0")}`
      : "";
    const written =
      suffix && own.endsWith(suffix) ? own.slice(0, -suffix.length) : own;
    const level = this.levelOf(parent.id, written, own, instance);
    const declared = parent.children.get(`${parent.id}/${level}`);
    if (declared) return declared;
    return this.controller.createDynamicSubtest(parent, level, written);
  }

  // levelOf is the own level a repeated run's test takes under parent: the
  // one its occurrence had in the first run that reached parent. testing
  // numbers a repeat's names on from the earlier runs' (TestX#01 in the
  // second), so the nth "TestX" of every run is one item.
  private levelOf(
    parentId: string,
    written: string,
    own: string,
    instance: number,
  ): string {
    const key = `${parentId}\u0000${written}`;
    let seen = this.occurrences.get(key);
    if (!seen) {
      seen = { first: instance, owns: [], counts: new Map() };
      this.occurrences.set(key, seen);
    }
    const n = seen.counts.get(instance) ?? 0;
    seen.counts.set(instance, n + 1);
    if (instance === seen.first) {
      seen.owns.push(own);
      return own;
    }
    return seen.owns[n] ?? own;
  }

  // topLevel is a suite's item, or a generated fuzz wrapper's. Instance 2 is
  // an external twin's run; a lone external suite runs once, as instance 1.
  private topLevel(
    name: string,
    instance: number,
  ): vscode.TestItem | undefined {
    // Fuzz wrappers are Fuzz<Suite>_<Method> with <Method> starting with
    // Fuzz. Suite names may hold underscores, so each "_Fuzz" boundary is
    // tried against the tree; none matching falls through to a suite named
    // Fuzz*.
    if (name.startsWith("Fuzz")) {
      const body = name.slice(4);
      for (
        let idx = body.indexOf("_Fuzz");
        idx >= 0;
        idx = body.indexOf("_Fuzz", idx + 1)
      ) {
        const fuzzItem = this.controller.findItem(
          `${this.importPath}/${body.slice(0, idx)}/${body.slice(idx + 1)}`,
        );
        if (fuzzItem) return fuzzItem;
      }
    }
    const suite = name.startsWith("Test") ? name.slice(4) : name;
    const plain = `${this.importPath}/${suite}`;
    if (instance % 2 === 0) {
      // Under -count go alternates a twin's runs: internal, external, ...
      // Without a twin every run is the plain suite's.
      const twin = this.controller.findItem(`${plain}${EXTERNAL_ID}`);
      if (twin) return twin;
    }
    return this.controller.findItem(plain);
  }
}

// erroredItem marks an item whose run could not produce a verdict, in the
// run and in the result store, so a reload and Copy Test Results say so too.
export function erroredItem(
  run: vscode.TestRun,
  controller: GoTestController,
  item: vscode.TestItem,
  message: string,
): void {
  run.errored(item, new vscode.TestMessage(message));
  controller.recordResult(item.id, "errored");
}

export interface AppliedResult {
  itemId: string;
  status: "pass" | "fail" | "skip";
  duration?: number;
}

// BuildOutputs keeps the diagnostics of failed builds until the package
// verdict that names the build arrives. go test keys them by the build's
// ImportPath, not by a package: one build can fail several packages.
export class BuildOutputs {
  private readonly byBuild = new Map<string, string>();

  // absorb takes a build event and reports whether it was one; every other
  // event belongs to a package and is the caller's to route.
  absorb(event: TestEvent): boolean {
    if (event.Action !== "build-output" && event.Action !== "build-fail") {
      return false;
    }
    if (event.ImportPath && event.Output) {
      const existing = this.byBuild.get(event.ImportPath) ?? "";
      this.byBuild.set(event.ImportPath, existing + event.Output);
    }
    return true;
  }

  get(build: string): string {
    return this.byBuild.get(build) ?? "";
  }
}

export interface ApplyOptions {
  // Where a failed build's diagnostics wait, and the directory the
  // compiler's relative file names resolve against — the CLI's working
  // directory, not the package's.
  builds?: BuildOutputs;
  buildDir?: string;
  // The package's router for this run: it remembers where each test sat.
  router: TestItemRouter;
}

export function applyEvent(
  controller: GoTestController,
  run: vscode.TestRun,
  event: TestEvent,
  outputMap: Map<string, string>,
  importPath: string,
  pkgDir: string,
  options: ApplyOptions,
): AppliedResult | undefined {
  // A twin's second run reuses the top-level name: its output is its own.
  const outputKey =
    event.Test && (event.Gotest?.Instance ?? 1) > 1
      ? `${event.Gotest?.Instance}\u0000${event.Test}`
      : (event.Test ?? "");
  if (event.Action === "output") {
    const key = outputKey;
    const output = event.Output ?? "";
    if (!(key === "" && isPackageSummaryLine(output))) {
      const existing = outputMap.get(key) ?? "";
      outputMap.set(key, existing + output);
    }
  }

  if (event.Action === "output" && event.Output) {
    if (!event.Test && /^exit status \d+\n?$/.test(event.Output)) {
      return undefined;
    }
    const line = event.Output.replace(/\n$/, "\r\n");
    const testItem = event.Test ? options.router.route(event) : undefined;
    run.appendOutput(line, undefined, testItem);
    return undefined;
  }

  if (!event.Test) {
    if (
      event.Action === "pass" ||
      event.Action === "fail" ||
      event.Action === "skip"
    ) {
      const duration =
        event.Elapsed !== undefined ? event.Elapsed * 1000 : undefined;
      controller.recordResult(importPath, event.Action, duration);

      const pkgItem = controller.findItem(importPath);
      if (pkgItem) {
        if (event.Action === "fail") {
          let output = outputMap.get("") ?? "";
          let diagnosticsDir = pkgDir;
          if (event.FailedBuild) {
            output = (options.builds?.get(event.FailedBuild) ?? "") + output;
            diagnosticsDir = options.buildDir ?? pkgDir;
          }
          const message = new vscode.TestMessage(output || "Package failed");
          const loc = extractDiagnosticLocation(output, diagnosticsDir);
          if (loc) {
            message.location = new vscode.Location(
              vscode.Uri.file(loc.file),
              new vscode.Position(loc.line - 1, 0),
            );
          }
          run.failed(pkgItem, [message], duration);
        } else if (event.Action === "pass") {
          run.passed(pkgItem, duration);
        } else {
          run.skipped(pkgItem);
        }
        resolveAncestorsOf(run, pkgItem, controller);
      }
      return { itemId: importPath, status: event.Action, duration };
    }
    return undefined;
  }

  const item = options.router.route(event);
  if (!item) {
    return undefined;
  }

  const duration =
    event.Elapsed !== undefined ? event.Elapsed * 1000 : undefined;
  // The event's own clock, not ours: it brackets the node against every other
  // node in the stream, including ones from suites running in other processes.
  const stamp = Date.parse(event.Time);
  const at = Number.isNaN(stamp) ? undefined : stamp;

  switch (event.Action) {
    case "pass":
      run.passed(item, duration);
      controller.recordResult(item.id, "pass", duration, at);
      return { itemId: item.id, status: "pass", duration };
    case "fail": {
      const output = outputMap.get(outputKey) ?? "";
      const testMessages = extractTestMessages(output, pkgDir);
      const vscodeMessages = testMessages.map((msg) => {
        const parsed = parseExpectedActual(msg.message);
        const message = new vscode.TestMessage(
          parsed?.compact
            ? `${msg.message.split("\n")[0].replace(/:\s*$/, "")}: expected ${parsed.expected}, actual ${parsed.actual}`
            : msg.message,
        );
        if (parsed) {
          message.expectedOutput = parsed.expected;
          message.actualOutput = parsed.actual;
        }
        message.location = new vscode.Location(
          vscode.Uri.file(msg.file),
          new vscode.Position(msg.line - 1, 0),
        );
        return message;
      });
      if (vscodeMessages.length === 0) {
        const fallback = new vscode.TestMessage(output || "Test failed");
        const loc = extractDiagnosticLocation(output, pkgDir);
        if (loc) {
          fallback.location = new vscode.Location(
            vscode.Uri.file(loc.file),
            new vscode.Position(loc.line - 1, 0),
          );
        }
        vscodeMessages.push(fallback);
      }
      run.failed(item, vscodeMessages, duration);
      controller.recordResult(item.id, "fail", duration, at);
      return { itemId: item.id, status: "fail", duration };
    }
    case "skip":
      run.skipped(item);
      controller.recordResult(item.id, "skip", undefined, at);
      return { itemId: item.id, status: "skip", duration: undefined };
    case "run":
      run.started(item);
      if (at !== undefined) {
        controller.noteStart(item.id, at);
      }
      return undefined;
    case "pause":
    case "cont":
      // The node called t.Parallel. That is the one fact its own timestamps
      // cannot survive, so it has to be carried alongside them.
      controller.notePaused(item.id);
      return undefined;
  }

  return undefined;
}

export function applyResults(
  controller: GoTestController,
  run: vscode.TestRun,
  events: TestEvent[],
  importPath: string,
  pkgDir: string,
  options: Partial<ApplyOptions> = {},
): AppliedResult[] {
  const outputMap = new Map<string, string>();
  const router = options.router ?? new TestItemRouter(controller, importPath);
  const applied: AppliedResult[] = [];
  for (const event of events) {
    const result = applyEvent(
      controller,
      run,
      event,
      outputMap,
      importPath,
      pkgDir,
      { ...options, router },
    );
    if (result) applied.push(result);
  }
  return applied;
}

// StreamApplier applies one run's events as they arrive in pieces. A test's
// output and a build's diagnostics accumulate across pieces until the verdict
// that reads them, so where a pipe happened to split the stream changes
// nothing about the message a failure carries.
export class StreamApplier {
  private readonly outputMaps = new Map<string, Map<string, string>>();
  private readonly builds = new BuildOutputs();
  private readonly routers = new Map<string, TestItemRouter>();
  // Packages the events named that have no directory here.
  readonly unknownPackages = new Set<string>();

  constructor(
    private readonly controller: GoTestController,
    private readonly run: vscode.TestRun,
    private readonly packageDir: (importPath: string) => string | undefined,
    // Where the compiler's relative file names resolve: the CLI's cwd.
    private readonly buildDir: string,
    // Says once per run what could not be routed.
    private readonly log: OnceLog = new OnceLog(),
    private readonly unstamped = UNSTAMPED_LIVE,
  ) {}

  apply(events: TestEvent[]): AppliedResult[] {
    const applied: AppliedResult[] = [];
    for (const event of events) {
      if (this.builds.absorb(event)) continue;
      if (typeof event.Package !== "string") continue;
      const dir = this.packageDir(event.Package);
      if (!dir) {
        this.unknownPackages.add(event.Package);
        continue;
      }
      let outputMap = this.outputMaps.get(event.Package);
      if (!outputMap) {
        outputMap = new Map();
        this.outputMaps.set(event.Package, outputMap);
      }
      let router = this.routers.get(event.Package);
      if (!router) {
        router = new TestItemRouter(
          this.controller,
          event.Package,
          this.log,
          this.unstamped,
        );
        this.routers.set(event.Package, router);
      }
      const result = applyEvent(
        this.controller,
        this.run,
        event,
        outputMap,
        event.Package,
        dir,
        { builds: this.builds, buildDir: this.buildDir, router },
      );
      if (result) applied.push(result);
    }
    return applied;
  }
}

export interface SpawnResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}

export async function spawnTestProcess(
  bin: string,
  args: string[],
  cwd: string,
  token: vscode.CancellationToken,
  outputChannel: vscode.LogOutputChannel,
  label: string,
  env?: Record<string, string>,
  onStdoutLine?: (line: string) => void,
  onStderrLine?: (line: string) => void,
): Promise<SpawnResult> {
  const mc = new ManagedChild(bin, args, {
    cwd,
    env,
    kind: "test",
    control: true,
  });
  let stdout = "";
  let stderr = "";
  let lineBuffer = "";
  let stderrLineBuffer = "";
  let spawnError: Error | undefined;

  // Line assembly is the caller's job: ManagedChild decodes the stream so a
  // character split across two reads survives, but what counts as an event is
  // specific to this surface.
  mc.child.stdout?.on("data", (chunk: string) => {
    stdout += chunk;

    if (onStdoutLine) {
      lineBuffer += chunk;
      const lines = lineBuffer.split("\n");
      lineBuffer = lines.pop() ?? "";
      for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed) {
          onStdoutLine(trimmed);
        }
      }
    }
  });

  mc.child.stderr?.on("data", (chunk: string) => {
    stderr += chunk;

    // Fuzzing reads crasher detail off stderr as it arrives, so it needs the
    // same line framing stdout gets rather than one blob at the end.
    if (onStderrLine) {
      stderrLineBuffer += chunk;
      const lines = stderrLineBuffer.split("\n");
      stderrLineBuffer = lines.pop() ?? "";
      for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed) {
          onStderrLine(trimmed);
        }
      }
    }
  });

  mc.child.on("error", (err: Error) => {
    spawnError = err;
  });

  // A run: the stop is what starts fixture teardown, so it gets the teardown
  // grace rather than the prompt one.
  const cancelListener = token.onCancellationRequested(() => {
    outputChannel.info(
      `[${label}] cancellation requested, stopping (pid ${mc.pid})`,
    );
    void mc.terminate("teardown");
  });

  try {
    const { code } = await mc.exited;
    if (spawnError) {
      outputChannel.error(`[${label}] ${spawnError.message}`);
      throw spawnError;
    }
    if (onStdoutLine) {
      const remaining = lineBuffer.trim();
      if (remaining) {
        onStdoutLine(remaining);
      }
    }
    if (onStderrLine) {
      const remaining = stderrLineBuffer.trim();
      if (remaining) {
        onStderrLine(remaining);
      }
    }
    if (stderr) {
      for (const line of stderr.split("\n")) {
        if (line.trim()) {
          outputChannel.warn(`[${label}] stderr: ${line}`);
        }
      }
    }
    return { stdout, stderr, exitCode: code ?? 1 };
  } finally {
    cancelListener.dispose();
    mc.dispose();
  }
}

// isFuzzMethodLabel mirrors the framework's own method classification: an
// optional X_/F_ marker prefix, then "Fuzz" plus at least one character.
// Fuzz methods live in the same tree position as Test methods but map to
// top-level generated wrappers (Fuzz<Suite>_<Method>), not to subtests of
// ^Test<Suite>$ — the filter grammar differs, so classification matters.
export function isFuzzMethodLabel(label: string): boolean {
  return /^(?:X_|F_)?Fuzz./.test(label);
}

// itemTestPath is the go test path a filter built from item asks for: the
// suite's top-level test with the item's own segments below it, or a fuzz
// target's generated wrapper. Undefined for a package or anything above one.
export function itemTestPath(item: vscode.TestItem): string | undefined {
  const depth = getPackageDepth(item);
  if (depth <= 0) return undefined;
  let method = item;
  while (getPackageDepth(method) > 2) method = method.parent!;
  if (depth >= 2 && isFuzzMethodLabel(method.label) && method.parent) {
    const rest = item.id.slice(method.id.length);
    return `Fuzz${method.parent.label}_${method.label}${rest}`;
  }
  const pkg = getPackageItem(item);
  // The twin marker is an id detail; go test knows both twins by one name.
  const rest = item.id.slice(pkg.id.length + 1);
  return `Test${rest.replace(/~external(?=\/|$)/, "")}`;
}

export function buildRunFilter(items: vscode.TestItem[]): string | undefined {
  if (items.some((item) => getPackageDepth(item) === 0)) {
    return undefined;
  }

  const suiteGroups = new Map<
    string,
    {
      wholeSuite: boolean;
      methods: string[];
      fuzzers: string[];
      subtests: string[];
    }
  >();
  const groupFor = (suiteName: string) => {
    let group = suiteGroups.get(suiteName);
    if (!group) {
      group = { wholeSuite: false, methods: [], fuzzers: [], subtests: [] };
      suiteGroups.set(suiteName, group);
    }
    return group;
  };

  for (const item of items) {
    const depth = getPackageDepth(item);

    if (depth === 1) {
      groupFor(item.label).wholeSuite = true;
    } else if (depth === 2) {
      const group = groupFor(item.parent!.label);
      if (isFuzzMethodLabel(item.label)) {
        group.fuzzers.push(item.label);
      } else {
        group.methods.push(methodSegmentOf(item));
      }
    } else if (depth >= 3) {
      let current = item;
      const subtestParts: string[] = [];
      while (getPackageDepth(current) > 2) {
        // The id segment, not the label: a behavior is labelled with the text
        // the developer wrote ("classifying a number") while go test knows it
        // by its rewritten name ("classifying_a_number"). Filtering on the
        // label would never match.
        subtestParts.unshift(methodSegmentOf(current));
        current = current.parent!;
      }
      const methodName = methodSegmentOf(current);
      const methodLabel = current.label;
      const suiteName = current.parent!.label;
      const group = groupFor(suiteName);
      if (isFuzzMethodLabel(methodLabel)) {
        // Seed subtests of a fuzz target live under its top-level wrapper.
        group.subtests.push(
          `^Fuzz${suiteName}_${methodName}$/^${subtestParts
            .map(escapeRunPattern)
            .join("/")}$`,
        );
      } else {
        group.subtests.push(
          `^Test${suiteName}$/^${methodName}$/^${subtestParts
            .map(escapeRunPattern)
            .join("/")}$`,
        );
      }
    }
  }

  const filters: string[] = [];
  for (const [suiteName, group] of suiteGroups) {
    if (group.wholeSuite) {
      // Parity with CLI package runs: a whole-suite selection must replay
      // the suite's fuzz seeds too. The generated wrappers are top-level
      // Fuzz<Suite>_* functions that ^Test<Suite>$ can never match, and
      // the runner honors explicit filters verbatim — so the widening has
      // to happen here. A suite without fuzz targets matches nothing
      // extra, which is why this is unconditional.
      filters.push(`^Test${suiteName}$`, `^Fuzz${suiteName}_.*$`);
      continue;
    }
    if (group.subtests.length > 0) {
      filters.push(...group.subtests);
    } else if (group.methods.length === 1) {
      filters.push(`^Test${suiteName}$/^${group.methods[0]}$`);
    } else if (group.methods.length > 1) {
      filters.push(`^Test${suiteName}$/^(${group.methods.join("|")})$`);
    }
    // Fuzz targets are top-level names, disjoint from the ^Test<Suite>$
    // grammar above — selecting one alongside test methods or subtests
    // must include both, so these are additive rather than part of the
    // else-if chain. Overlap with a seed subtest of the same target is
    // harmless: the alternatives union into one -run regex.
    for (const fuzzer of group.fuzzers) {
      filters.push(`^Fuzz${suiteName}_${fuzzer}$`);
    }
  }

  return filters.length === 0
    ? undefined
    : filters.length === 1
      ? filters[0]
      : filters.join("|");
}

export function computeWildcard(
  importPaths: string[],
  modulePath?: string,
): string[] | undefined {
  if (importPaths.length <= 1) return undefined;

  const split = importPaths.map((p) => p.split("/"));
  const first = split[0];
  let prefixLen = 0;
  for (let i = 0; i < first.length; i++) {
    if (split.every((s) => s[i] === first[i])) {
      prefixLen = i + 1;
    } else {
      break;
    }
  }
  if (prefixLen === 0) return undefined;

  const prefix = first.slice(0, prefixLen).join("/");
  if (importPaths.every((p) => p === prefix)) return undefined;
  if (!modulePath || prefix !== modulePath) return [prefix + "/..."];

  const groups = new Map<string, string[]>();
  const ungrouped: string[] = [];
  for (const p of importPaths) {
    if (p === modulePath) {
      ungrouped.push(p);
      continue;
    }
    const rest = p.slice(modulePath.length + 1);
    const seg = rest.split("/")[0];
    let group = groups.get(seg);
    if (!group) {
      group = [];
      groups.set(seg, group);
    }
    group.push(p);
  }

  const result: string[] = [...ungrouped];
  for (const [seg, paths] of groups) {
    if (paths.length === 1) {
      result.push(paths[0]);
    } else {
      result.push(modulePath + "/" + seg + "/...");
    }
  }

  return result.length < importPaths.length ? result : undefined;
}

// KnownPackages are the discovered packages a widened pattern must not reach
// beyond the selection; dir-relative patterns resolve against workspaceDir.
export interface KnownPackages {
  workspaceDir: string;
  packages: { importPath: string; dir: string }[];
}

/**
 * Determines the gotest package patterns for a batch of packages.
 * Uses workspace patterns (from go.work) when available and the common
 * prefix equals the module path. Otherwise falls back to computeWildcard.
 * Focus is run-wide, so a pattern that would also match a discovered package
 * outside the selection is dropped for the exact import paths.
 */
export function resolveRunPatterns(
  importPaths: string[],
  modulePath: string | undefined,
  workspacePatterns?: string[],
  known?: KnownPackages,
): string[] | undefined {
  const patterns = widenRunPatterns(importPaths, modulePath, workspacePatterns);
  if (!patterns || !known) return patterns;
  const selected = new Set(importPaths);
  const reachesOutside = known.packages.some(
    (pkg) =>
      !selected.has(pkg.importPath) &&
      patterns.some((pattern) =>
        patternCovers(pattern, pkg, known.workspaceDir),
      ),
  );
  return reachesOutside ? undefined : patterns;
}

// patternCovers reports whether a go package pattern matches pkg: an import
// path or `<path>/...`, or a `./`-relative directory pattern.
export function patternCovers(
  pattern: string,
  pkg: { importPath: string; dir: string },
  workspaceDir: string,
): boolean {
  const recursive = pattern === "..." || pattern.endsWith("/...");
  const base = pattern === "..." ? "" : pattern.replace(/\/\.\.\.$/, "");
  if (pattern.startsWith(".")) {
    const rel = path.relative(path.resolve(workspaceDir, base), pkg.dir);
    if (rel === "") return true;
    return recursive && !rel.startsWith("..") && !path.isAbsolute(rel);
  }
  if (pkg.importPath === base) return true;
  return recursive && pkg.importPath.startsWith(base + "/");
}

function widenRunPatterns(
  importPaths: string[],
  modulePath: string | undefined,
  workspacePatterns?: string[],
): string[] | undefined {
  if (importPaths.length <= 1) return undefined;

  // If workspace patterns are available (go.work) and the common prefix
  // equals the module path, use the workspace patterns directly.
  // This handles multi-module workspaces: ["./...", "./examples/..."]
  if (workspacePatterns && workspacePatterns.length > 0 && modulePath) {
    const split = importPaths.map((p) => p.split("/"));
    const first = split[0];
    let prefixLen = 0;
    for (let i = 0; i < first.length; i++) {
      if (split.every((s) => s[i] === first[i])) {
        prefixLen = i + 1;
      } else {
        break;
      }
    }
    const prefix = first.slice(0, prefixLen).join("/");
    if (prefix === modulePath) {
      return workspacePatterns;
    }
  }

  return computeWildcard(importPaths, modulePath);
}

/**
 * Reads go.work use directives and returns per-module patterns.
 * Returns undefined if no go.work exists.
 */
export async function readWorkspacePatterns(
  dir: string,
): Promise<string[] | undefined> {
  try {
    const content = await readFile(path.join(dir, "go.work"), "utf-8");

    const blockMatch = /^\s*use\s*\(\s*([\s\S]*?)\s*\)/m.exec(content);
    if (blockMatch) {
      const dirs = blockMatch[1]
        .split("\n")
        .map((l) => l.trim())
        .filter((l) => l && !l.startsWith("//"));
      if (dirs.length > 0) {
        return dirs.map((d) => (d === "." ? "./..." : `${d}/...`));
      }
    }

    const singles: string[] = [];
    const singleUse = /^\s*use\s+(\S+)/gm;
    let m: RegExpExecArray | null;
    while ((m = singleUse.exec(content)) !== null) {
      singles.push(m[1]);
    }
    if (singles.length > 0) {
      return singles.map((d) => (d === "." ? "./..." : `${d}/...`));
    }
  } catch {}
  return undefined;
}

export function getPackageDir(
  item: vscode.TestItem,
  cache: DiscoveryCache,
): string | undefined {
  const pkg = getPackageItem(item);
  return cache.getPackage(pkg.id)?.dir;
}

export function resolveAncestorsOf(
  run: vscode.TestRun,
  item: vscode.TestItem,
  controller: GoTestController,
): void {
  let current = item.parent;
  while (current) {
    let anyFailed = false;
    let allResolved = true;
    current.children.forEach((child) => {
      const result = controller.getResult(child.id);
      if (result) {
        if (result.status === "fail" || result.status === "errored") {
          anyFailed = true;
        }
        return;
      }
      const isStructural =
        child.id.startsWith("dir:") ||
        child.id.startsWith("wsFolder:") ||
        child.id.startsWith("module:");
      if (isStructural) {
        const r = resolveItemRecursive(run, child, controller);
        if (r.anyResolved) {
          if (r.anyFailed) anyFailed = true;
        } else {
          allResolved = false;
        }
      } else {
        allResolved = false;
      }
    });
    if (!allResolved) break;
    run.started(current);
    if (anyFailed) {
      run.failed(current, []);
    } else {
      run.passed(current);
    }
    controller.recordResult(current.id, anyFailed ? "fail" : "pass", undefined);
    current = current.parent;
  }
}

// restoreStoredResults shows the stored verdicts as one run, so a reload
// starts from what the last session saw. It returns how many landed on an
// item: a stored id with no item is a test that no longer exists, or one the
// tree cannot name yet.
export function restoreStoredResults(
  controller: GoTestController,
  store: TestResultStore,
): number {
  const run = controller.createTestRun(
    new vscode.TestRunRequest(),
    "Restored Results",
  );
  let applied = 0;
  store.forEach((result, itemId) => {
    const item = controller.findItem(itemId);
    if (!item) return;
    applied++;
    switch (result.status) {
      case "pass":
        run.passed(item, result.duration);
        break;
      case "fail":
        run.failed(
          item,
          [new vscode.TestMessage("(restored from previous session)")],
          result.duration,
        );
        break;
      case "skip":
        run.skipped(item);
        break;
      case "errored":
        run.errored(
          item,
          new vscode.TestMessage("(errored in the previous session)"),
        );
        break;
    }
  });
  resolveAncestorItems(run, controller);
  run.end();
  return applied;
}

export function resolveAncestorItems(
  run: vscode.TestRun,
  controller: GoTestController,
): void {
  controller.testController.items.forEach((item) => {
    resolveItemRecursive(run, item, controller);
  });
}

function resolveItemRecursive(
  run: vscode.TestRun,
  item: vscode.TestItem,
  controller: GoTestController,
): { anyFailed: boolean; anyResolved: boolean } {
  const directResult = controller.getResult(item.id);
  const isPackage = item.tags.some((t) => t.id === "package");
  const isStructural =
    item.id.startsWith("dir:") ||
    item.id.startsWith("wsFolder:") ||
    item.id.startsWith("module:");

  if (directResult && !isPackage && !isStructural) {
    return {
      anyFailed:
        directResult.status === "fail" || directResult.status === "errored",
      anyResolved: true,
    };
  }

  let anyFailed = false;
  let anyResolved = false;
  item.children.forEach((child) => {
    const r = resolveItemRecursive(run, child, controller);
    if (r.anyResolved) anyResolved = true;
    if (r.anyFailed) anyFailed = true;
  });

  // A package the run errored keeps that verdict: its passing children do
  // not make the run that failed it green.
  if (isPackage && directResult?.status === "errored") {
    return { anyFailed: true, anyResolved: true };
  }

  if (anyResolved) {
    run.started(item);
    if (anyFailed) {
      run.failed(item, []);
    } else {
      run.passed(item);
    }
    if (isStructural) {
      controller.recordResult(item.id, anyFailed ? "fail" : "pass", undefined);
    }
  }

  return { anyFailed, anyResolved };
}

// methodSegmentOf returns the single go test path segment an item adds to its
// parent. Ids are the test path, so the segment is what follows the parent's id.
export function methodSegmentOf(item: vscode.TestItem): string {
  const parentId = item.parent?.id;
  if (parentId && item.id.startsWith(parentId + "/")) {
    return item.id.slice(parentId.length + 1);
  }
  return item.label;
}

// escapeRunPattern quotes regex metacharacters in a subtest name. Behavior
// descriptions are prose — "handles (nested) values" is an ordinary thing to
// write — and go test's -run is a regular expression, so the name has to be
// matched literally rather than interpreted.
function escapeRunPattern(segment: string): string {
  return segment.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
