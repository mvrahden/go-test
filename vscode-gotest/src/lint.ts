import * as vscode from "vscode";
import * as path from "node:path";
import { readFile } from "node:fs/promises";
import {
  buildCliCommand,
  offerCliUpgrade,
  formatCliCommand,
  stripGoRunExitEcho,
} from "./cli.js";
import { captureOutput } from "./capture.js";
import { discoveryTimeoutSeconds } from "./config.js";
import type { DiscoveryCache } from "./discovery.js";
import {
  lineOf,
  parseLintJson,
  positionAt,
  utf16Column,
  utf16Offset,
  type LintFinding,
  type LintFix,
} from "./lintFindings.js";

// What one lint run covered: a package's directory, or a folder's tree. Its
// findings replace whatever was shown for the files inside it.
interface LintScope {
  key: string;
  folder: string;
  patterns: string[];
  covers: (file: string) => boolean;
}

interface Entry {
  finding: LintFinding;
  diagnostic: vscode.Diagnostic;
}

// Each rule's tier, as `gotest lint -rules -json` states it; undefined when
// that document could not be read.
type RuleTiers = Map<string, string> | undefined;

const DEBOUNCE_MS = 500;

// LintService runs `gotest lint -json` and shows what it finds as
// diagnostics. Rule policy is the CLI's: it reads .gotest.yml itself.
export class LintService implements vscode.Disposable {
  private readonly collection =
    vscode.languages.createDiagnosticCollection("gotest-lint");
  private readonly timers = new Map<string, ReturnType<typeof setTimeout>>();
  private readonly inFlight = new Map<string, AbortController>();
  private readonly entries = new Map<string, Entry[]>();
  // The text each file had when the lint that reported on it read it, so a
  // fix's byte offsets are only applied to the same bytes.
  private readonly seen = new Map<string, string>();
  // Each folder's rule tiers, read once per session; a failed read is kept,
  // not retried.
  private readonly tiers = new Map<string, Promise<RuleTiers>>();

  constructor(
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.LogOutputChannel,
  ) {}

  dispose(): void {
    for (const t of this.timers.values()) clearTimeout(t);
    this.timers.clear();
    for (const a of this.inFlight.values()) a.abort();
    this.inFlight.clear();
    this.collection.dispose();
  }

  // onSaved lints the package of a saved test file, debounced per package.
  onSaved(document: vscode.TextDocument): void {
    if (!document.fileName.endsWith("_test.go")) return;
    const folder = vscode.workspace.getWorkspaceFolder(document.uri);
    if (!folder) return;
    const enabled =
      vscode.workspace
        .getConfiguration("gotest", folder.uri)
        .get<boolean>("lintOnSave") ?? true;
    if (!enabled) return;
    const importPath = this.cache.resolveFileToPackage(document.uri.fsPath);
    if (!importPath) return;

    const existing = this.timers.get(importPath);
    if (existing) clearTimeout(existing);
    this.timers.set(
      importPath,
      setTimeout(() => {
        this.timers.delete(importPath);
        void this.lintPackage(importPath);
      }, DEBOUNCE_MS),
    );
  }

  // lintPackage lints one package and replaces its diagnostics.
  async lintPackage(importPath: string): Promise<void> {
    const scope = this.packageScope(importPath);
    if (!scope) {
      this.outputChannel.warn(`[lint] no package directory for ${importPath}`);
      return;
    }
    await this.run(scope, false);
  }

  // lintFolder lints every package under a workspace folder.
  async lintFolder(folder: string): Promise<void> {
    const root = folder + path.sep;
    await this.run(
      {
        key: `folder:${folder}`,
        folder,
        patterns: ["./..."],
        covers: (file) => file.startsWith(root),
      },
      false,
    );
  }

  // fixPackage applies every suggested fix in a package through the CLI and
  // shows what is left. Unsaved edits there are saved first: the CLI fixes
  // the files on disk.
  async fixPackage(importPath: string): Promise<void> {
    const scope = this.packageScope(importPath);
    if (!scope) return;
    const dir = this.cache.getPackage(importPath)?.dir;
    for (const doc of vscode.workspace.textDocuments) {
      if (doc.isDirty && path.dirname(doc.uri.fsPath) === dir) await doc.save();
    }
    await this.run(scope, true);
  }

  entriesFor(uri: vscode.Uri): Entry[] {
    return this.entries.get(uri.fsPath) ?? [];
  }

  // editFor turns a fix into an edit, if every file it touches still holds
  // the bytes the lint read. Undefined means lint again first.
  async editFor(fix: LintFix): Promise<vscode.WorkspaceEdit | undefined> {
    const edit = new vscode.WorkspaceEdit();
    for (const e of fix.edits) {
      const before = this.seen.get(e.filename);
      if (before === undefined) return undefined;
      const now = await currentText(e.filename);
      if (now !== before) return undefined;
      const start = positionAt(now, utf16Offset(now, e.start));
      const end = positionAt(now, utf16Offset(now, e.end));
      edit.replace(
        vscode.Uri.file(e.filename),
        new vscode.Range(
          new vscode.Position(start.line, start.character),
          new vscode.Position(end.line, end.character),
        ),
        e.new,
      );
    }
    return edit;
  }

  private packageScope(importPath: string): LintScope | undefined {
    const pkg = this.cache.getPackage(importPath);
    const folder = this.cache.getWorkspaceDir(importPath);
    if (!pkg || !folder) return undefined;
    return {
      key: importPath,
      folder,
      patterns: [importPath],
      covers: (file) => path.dirname(file) === pkg.dir,
    };
  }

  private async run(scope: LintScope, fix: boolean): Promise<void> {
    // The newest request for a scope is the one worth answering.
    this.inFlight.get(scope.key)?.abort();
    const abort = new AbortController();
    this.inFlight.set(scope.key, abort);

    const args = ["lint", ...(fix ? ["-fix"] : []), "-json", ...scope.patterns];
    let cmd;
    try {
      cmd = await buildCliCommand(args, scope.folder, this.outputChannel);
    } catch (err: unknown) {
      if (this.inFlight.get(scope.key) === abort)
        this.inFlight.delete(scope.key);
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[lint] ${message}`);
      if (!offerCliUpgrade(err) && fix) {
        vscode.window.showErrorMessage(`gotest lint: ${message}`);
      }
      return;
    }
    this.outputChannel.info(`[lint] ${formatCliCommand(cmd)}`);
    const tiers = this.ruleTiers(scope.folder);
    const startedAt = Date.now();

    let out;
    try {
      out = await captureOutput(cmd.bin, cmd.args, {
        cwd: scope.folder,
        // A fix is not interrupted: the CLI writes every file or none, and
        // a timeout mid-write would be the one way to break that.
        timeoutSeconds: fix ? undefined : discoveryTimeoutSeconds(scope.folder),
        abort: fix ? undefined : abort.signal,
        grace: fix ? "teardown" : "prompt",
      });
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[lint] ${scope.key}: ${message}`);
      return;
    } finally {
      if (this.inFlight.get(scope.key) === abort)
        this.inFlight.delete(scope.key);
    }
    if (out.aborted) return;
    const tierOf = await tiers;
    // A save came in while this ran; the lint it starts answers it.
    if (!fix && this.timers.has(scope.key)) return;

    const ms = Date.now() - startedAt;
    // -json exits 0 with or without findings; 1 says nothing could be
    // analyzed and 2 that the command or its configuration is wrong. Either
    // way nothing was proven, so what is shown stays.
    if (out.code !== 0) {
      const reason = out.stderr.trim() || `exit code ${out.code}`;
      this.outputChannel.warn(
        `[lint] ${scope.key}: exit ${out.code}, diagnostics left as they were: ${reason}`,
      );
      if (fix) {
        vscode.window.showErrorMessage(
          `gotest lint -fix: ${reason.split("\n")[0]}`,
        );
      }
      return;
    }

    let findings: LintFinding[];
    try {
      findings = parseLintJson(out.stdout);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(
        `[lint] ${scope.key}: unreadable JSON: ${message}`,
      );
      return;
    }
    await this.publish(scope, findings, tierOf);
    this.outputChannel.info(
      `[lint] ${scope.key}: ${findings.length} finding(s) in ${ms}ms`,
    );
  }

  // ruleTiers is the folder's rule table; concurrent lints share one read.
  private ruleTiers(folder: string): Promise<RuleTiers> {
    let read = this.tiers.get(folder);
    if (!read) {
      read = this.readRuleTiers(folder);
      this.tiers.set(folder, read);
    }
    return read;
  }

  private async readRuleTiers(folder: string): Promise<RuleTiers> {
    const unread = (reason: string): RuleTiers => {
      this.outputChannel.warn(
        `[lint] ${folder}: rule tiers unread, every finding is a warning: ${reason}`,
      );
      return undefined;
    };
    let cmd;
    try {
      cmd = await buildCliCommand(
        ["lint", "-rules", "-json"],
        folder,
        this.outputChannel,
      );
    } catch (err: unknown) {
      offerCliUpgrade(err);
      return unread(err instanceof Error ? err.message : String(err));
    }
    let out;
    try {
      out = await captureOutput(cmd.bin, cmd.args, {
        cwd: folder,
        timeoutSeconds: discoveryTimeoutSeconds(folder),
      });
    } catch (err: unknown) {
      return unread(err instanceof Error ? err.message : String(err));
    }
    if (out.code !== 0) {
      return unread(stripGoRunExitEcho(out.stderr) || `exit code ${out.code}`);
    }
    try {
      return parseRuleTiers(out.stdout);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      return unread(`unreadable JSON: ${message}`);
    }
  }

  // publish replaces every diagnostic inside the scope in one step.
  private async publish(
    scope: LintScope,
    findings: LintFinding[],
    tiers?: RuleTiers,
  ): Promise<void> {
    const texts = new Map<string, string>();
    const files = new Set<string>();
    for (const f of findings) {
      files.add(f.file);
      for (const fix of f.fixes)
        for (const e of fix.edits) files.add(e.filename);
    }
    for (const file of files) {
      try {
        texts.set(file, await readFile(file, "utf-8"));
      } catch {
        texts.set(file, "");
      }
    }

    const next = new Map<string, Entry[]>();
    for (const f of findings) {
      const text = texts.get(f.file) ?? "";
      const entry = {
        finding: f,
        diagnostic: toDiagnostic(f, text, severityOf(f.category, tiers)),
      };
      const list = next.get(f.file);
      if (list) list.push(entry);
      else next.set(f.file, [entry]);
    }

    const updates: [vscode.Uri, vscode.Diagnostic[] | undefined][] = [];
    for (const file of Array.from(this.entries.keys())) {
      if (scope.covers(file) && !next.has(file)) {
        this.entries.delete(file);
        this.seen.delete(file);
        updates.push([vscode.Uri.file(file), undefined]);
      }
    }
    for (const [file, list] of next) {
      this.entries.set(file, list);
      updates.push([vscode.Uri.file(file), list.map((e) => e.diagnostic)]);
    }
    for (const [file, text] of texts) this.seen.set(file, text);
    this.collection.set(updates);
  }
}

// parseRuleTiers reads `gotest lint -rules -json` into each rule's tier.
function parseRuleTiers(stdout: string): Map<string, string> {
  const doc = JSON.parse(stdout) as { rules?: unknown } | null;
  if (!Array.isArray(doc?.rules)) throw new Error("no rules list");
  const tiers = new Map<string, string>();
  for (const r of doc.rules as { id?: unknown; tier?: unknown }[]) {
    if (typeof r?.id === "string" && typeof r.tier === "string") {
      tiers.set(r.id, r.tier);
    }
  }
  return tiers;
}

// severityOf makes a finding of an integrity rule an error and every other a
// warning; a finding's category is its rule.
function severityOf(
  category: string,
  tiers: RuleTiers,
): vscode.DiagnosticSeverity {
  return tiers?.get(category) === "integrity"
    ? vscode.DiagnosticSeverity.Error
    : vscode.DiagnosticSeverity.Warning;
}

// toDiagnostic places a finding on the text the lint read, from its start to
// its end. One whose end does not lie past its start names a point and is
// drawn to the end of its line, where a point would be invisible.
function toDiagnostic(
  f: LintFinding,
  text: string,
  severity: vscode.DiagnosticSeverity,
): vscode.Diagnostic {
  const startLine = lineOf(text, f.line);
  const start = new vscode.Position(f.line - 1, utf16Column(startLine, f.col));
  let end: vscode.Position;
  if (f.endLine > f.line || (f.endLine === f.line && f.endCol > f.col)) {
    end = new vscode.Position(
      f.endLine - 1,
      utf16Column(lineOf(text, f.endLine), f.endCol),
    );
  } else {
    const lineEnd = startLine.trimEnd().length;
    end = new vscode.Position(
      f.line - 1,
      Math.max(lineEnd, start.character + 1),
    );
  }
  const d = new vscode.Diagnostic(
    new vscode.Range(start, end),
    f.message,
    severity,
  );
  d.source = "gotest";
  d.code = f.category;
  return d;
}

// currentText is what the editor holds for a file, or the disk when it is
// not open.
async function currentText(file: string): Promise<string | undefined> {
  const open = vscode.workspace.textDocuments.find(
    (d) => d.uri.fsPath === file,
  );
  if (open) return open.getText();
  try {
    return await readFile(file, "utf-8");
  } catch {
    return undefined;
  }
}

// LintCodeActionProvider offers each of a finding's suggested fixes. When
// the file changed since the lint read it, the fix lints again before it
// applies, so its offsets land on the bytes they were computed for.
export class LintCodeActionProvider implements vscode.CodeActionProvider {
  static readonly providedCodeActionKinds = [vscode.CodeActionKind.QuickFix];

  constructor(private readonly service: LintService) {}

  async provideCodeActions(
    document: vscode.TextDocument,
    _range: vscode.Range | vscode.Selection,
    context: vscode.CodeActionContext,
  ): Promise<vscode.CodeAction[]> {
    const actions: vscode.CodeAction[] = [];
    for (const entry of this.service.entriesFor(document.uri)) {
      if (!context.diagnostics.some((d) => sameDiagnostic(d, entry.diagnostic)))
        continue;
      for (let i = 0; i < entry.finding.fixes.length; i++) {
        const fix = entry.finding.fixes[i];
        const action = new vscode.CodeAction(
          fix.message,
          vscode.CodeActionKind.QuickFix,
        );
        action.diagnostics = [entry.diagnostic];
        const edit = await this.service.editFor(fix);
        if (edit) {
          action.edit = edit;
        } else {
          action.command = {
            command: "gotest.applyLintFix",
            title: fix.message,
            arguments: [document.uri, entry.finding, i],
          };
        }
        actions.push(action);
      }
    }
    return actions;
  }
}

// The editor hands diagnostics back as copies, so they are matched by what
// they say and where.
function sameDiagnostic(a: vscode.Diagnostic, b: vscode.Diagnostic): boolean {
  return (
    a.source === b.source &&
    a.code === b.code &&
    a.message === b.message &&
    a.range.start.line === b.range.start.line &&
    a.range.start.character === b.range.start.character
  );
}

// applyFixAfterRelint saves the file, lints its package again and applies
// the same fix of the same finding, if the fresh lint still reports it.
export async function applyFixAfterRelint(
  service: LintService,
  cache: DiscoveryCache,
  uri: vscode.Uri,
  finding: LintFinding,
  fixIndex: number,
): Promise<boolean> {
  const doc = vscode.workspace.textDocuments.find(
    (d) => d.uri.fsPath === uri.fsPath,
  );
  if (doc?.isDirty) await doc.save();
  const importPath = cache.resolveFileToPackage(uri.fsPath);
  if (!importPath) return false;
  await service.lintPackage(importPath);
  const fresh = service
    .entriesFor(uri)
    .map((e) => e.finding)
    .filter(
      (f) => f.category === finding.category && f.message === finding.message,
    )
    .sort(
      (a, b) =>
        Math.abs(a.line - finding.line) - Math.abs(b.line - finding.line),
    )[0];
  const fix = fresh?.fixes[fixIndex];
  const edit = fix ? await service.editFor(fix) : undefined;
  if (!edit) {
    vscode.window.showInformationMessage(
      "gotest lint: the finding is no longer reported, so there is nothing to fix.",
    );
    return false;
  }
  return vscode.workspace.applyEdit(edit);
}
