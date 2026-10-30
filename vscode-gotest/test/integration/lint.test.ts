// Lint findings in the editor, driven against the real CLI: what
// `gotest lint -json` reports on the fixture corpus becomes diagnostics with
// the rule as their code and its tier as their severity, and a suggested fix
// becomes an edit that lands on the bytes the CLI meant.

import { describe, it, expect, beforeAll, afterAll, vi } from "vitest";
import {
  writeFileSync,
  mkdtempSync,
  mkdirSync,
  rmSync,
  readFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

const state = vi.hoisted(() => ({
  workspaceDir: "",
  cliPath: "",
  autoRefresh: true,
  panels: [] as { webview: { html: string; cspSource: string } }[],
  controllers: [] as unknown[],
  config: {} as Record<string, unknown>,
  diagnostics: new Map<string, Map<string, unknown[]>>(),
}));

vi.mock("vscode", async () => {
  const { buildVscodeStub } = await import("./vscodeStub.js");
  return buildVscodeStub(state as never);
});

import * as vscode from "vscode";
import { DiscoveryCache } from "../../src/discovery.js";
import { LintService, LintCodeActionProvider } from "../../src/lint.js";
import { createRecordingChannel } from "./vscodeStub.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const fixturesDir = path.join(extensionDir, "testdata", "fixtures");
// A fixed path, so repeated runs reuse one build cache entry.
const fixModuleDir = path.join(tmpdir(), "gotest-extension-lint-fix");

interface Diag {
  message: string;
  source: string;
  code: string;
  severity: number;
  range: {
    start: { line: number; character: number };
    end: { line: number; character: number };
  };
}

let cache: DiscoveryCache;
let recorder: ReturnType<typeof createRecordingChannel>;
let savedGoWork: string | undefined;
let tmp = "";

const lintDiagnostics = (file: string) =>
  (state.diagnostics.get("gotest-lint")?.get(file) ?? []) as Diag[];

function pkg(importPath: string, dir: string) {
  return { importPath, dir, suites: [] } as never;
}

beforeAll(() => {
  tmp = mkdtempSync(path.join(tmpdir(), "gotest-lint-"));
  rmSync(fixModuleDir, { recursive: true, force: true });
  mkdirSync(fixModuleDir, { recursive: true });
  writeFileSync(
    path.join(fixModuleDir, "go.mod"),
    "module gotest.lintfix\n\ngo 1.26.0\n\nrequire github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000\n\n" +
      `replace github.com/mvrahden/go-test => ${repoRoot}\n`,
    "utf-8",
  );
  const workFile = path.join(tmp, "lint.work");
  writeFileSync(
    workFile,
    `go 1.26.0\n\nuse (\n\t${repoRoot}\n\t${fixturesDir}\n\t${fixModuleDir}\n)\n`,
    "utf-8",
  );
  savedGoWork = process.env.GOWORK;
  process.env.GOWORK = workFile;
  state.workspaceDir = fixturesDir;

  recorder = createRecordingChannel();
  cache = new DiscoveryCache();
  cache.update(
    [
      pkg("gotest.fixtures/stdlibtests", path.join(fixturesDir, "stdlibtests")),
      pkg("gotest.fixtures/broken", path.join(fixturesDir, "broken")),
    ],
    true,
    fixturesDir,
  );
  cache.update(
    [
      pkg("gotest.lintfix", fixModuleDir),
      pkg("gotest.lintfix/tiers", path.join(fixModuleDir, "tiers")),
    ],
    true,
    fixModuleDir,
  );
});

afterAll(() => {
  if (savedGoWork === undefined) delete process.env.GOWORK;
  else process.env.GOWORK = savedGoWork;
  if (tmp) rmSync(tmp, { recursive: true, force: true });
  rmSync(fixModuleDir, { recursive: true, force: true });
});

describe("lint findings in the editor", () => {
  const plain = path.join(fixturesDir, "stdlibtests", "plain_test.go");

  it("shows a package's findings with the rule as their code", async () => {
    const lint = new LintService(cache, recorder.channel as never);
    await lint.lintPackage("gotest.fixtures/stdlibtests");

    const diags = lintDiagnostics(plain);
    expect(diags.map((d) => d.code).sort()).toEqual([
      "fail-guard",
      "fail-guard",
      "stdlib-test",
      "stdlib-test",
    ]);
    expect(diags.every((d) => d.source === "gotest")).toBe(true);
    // An expressiveness rule and a migration one: neither is an error.
    expect(
      diags.every((d) => d.severity === vscode.DiagnosticSeverity.Warning),
    ).toBe(true);
    const guard = diags.find((d) => d.message.startsWith("use Equal"))!;
    // plain_test.go:23:2 to 25:3, the whole if block, in the editor's
    // zero-based units.
    expect(guard.range.start).toEqual({ line: 22, character: 1 });
    expect(guard.range.end).toEqual({ line: 24, character: 2 });
    lint.dispose();
  }, 300_000);

  it("offers the CLI's fix as an edit of exactly the bytes it names", async () => {
    const lint = new LintService(cache, recorder.channel as never);
    await lint.lintPackage("gotest.fixtures/stdlibtests");
    const text = readFileSync(plain, "utf-8");
    const diags = lintDiagnostics(plain);
    const document = {
      uri: { fsPath: plain },
      getText: () => text,
      isDirty: false,
    };

    const actions = (await new LintCodeActionProvider(lint).provideCodeActions(
      document as never,
      undefined as never,
      { diagnostics: diags } as never,
    )) as unknown as {
      title: string;
      edit?: {
        replacements: {
          range: {
            start: { line: number; character: number };
            end: { line: number; character: number };
          };
          text: string;
        }[];
      };
    }[];

    const equal = actions.find((a) => a.title === "use Equal");
    expect(
      equal?.edit,
      "the file is unchanged, so the edit is direct",
    ).toBeDefined();
    const { range, text: replacement } = equal!.edit!.replacements[0];
    const lines = text.split("\n");
    const before = lines.slice(0, range.start.line).join("\n");
    const offsetOf = (p: { line: number; character: number }) =>
      lines.slice(0, p.line).join("\n").length +
      (p.line > 0 ? 1 : 0) +
      p.character;
    const fixed =
      text.slice(0, offsetOf(range.start)) +
      replacement +
      text.slice(offsetOf(range.end));
    expect(before.length).toBeGreaterThan(0);
    expect(fixed).toContain(
      'gotest.Equal(t, 1 + 1, 2, "arithmetic is broken")',
    );
    expect(fixed).not.toContain('t.Fatal("arithmetic is broken")');
    lint.dispose();
  }, 300_000);

  it("shows an integrity rule's finding as an error and an expressiveness one as a warning, each over what it names", async () => {
    const dir = path.join(fixModuleDir, "tiers");
    mkdirSync(dir, { recursive: true });
    const file = path.join(dir, "tiers_test.go");
    writeFileSync(
      file,
      [
        "package tiers",
        "",
        'import "github.com/mvrahden/go-test/pkg/gotest"',
        "",
        "type TierTestSuite struct{}",
        "",
        "func (s *TierTestSuite) F_TestFocused(t *gotest.T) {",
        "\tgotest.True(t, 1+1 == 2)",
        "}",
        "",
      ].join("\n"),
      "utf-8",
    );
    const lint = new LintService(cache, recorder.channel as never);
    await lint.lintPackage("gotest.lintfix/tiers");

    const diags = lintDiagnostics(file);
    const focus = diags.find((d) => d.code === "focus");
    const simplify = diags.find((d) => d.code === "assertion-simplify");
    expect(focus?.severity).toBe(vscode.DiagnosticSeverity.Error);
    expect(simplify?.severity).toBe(vscode.DiagnosticSeverity.Warning);
    // The focused method's signature, and the call through its closing paren.
    expect(focus!.range).toEqual({
      start: { line: 6, character: 0 },
      end: { line: 6, character: 50 },
    });
    expect(simplify!.range).toEqual({
      start: { line: 7, character: 1 },
      end: { line: 7, character: 25 },
    });
    lint.dispose();
  }, 300_000);

  it("leaves what it shows when the package cannot be analyzed", async () => {
    const lint = new LintService(cache, recorder.channel as never);
    recorder.warnings.length = 0;
    await lint.lintPackage("gotest.fixtures/broken");

    expect(
      recorder.warnings.some((w) =>
        /gotest\.fixtures\/broken: exit 1, diagnostics left as they were/.test(
          w,
        ),
      ),
    ).toBe(true);
    lint.dispose();
  }, 300_000);

  it("fixes a package through the CLI and clears what the fix resolved", async () => {
    const file = path.join(fixModuleDir, "fix_test.go");
    writeFileSync(
      file,
      [
        "package lintfix",
        "",
        "import (",
        '\t"testing"',
        "",
        '\t"github.com/mvrahden/go-test/pkg/gotest"',
        ")",
        "",
        "type SumTestSuite struct{}",
        "",
        "func (s *SumTestSuite) TestSum(t *gotest.T) {",
        "\tgotest.Equal(t, 2, 1+1)",
        "}",
        "",
        "func TestSum(t *testing.T) {",
        "\tif 1+1 != 2 {",
        '\t\tt.Fatal("sum is wrong")',
        "\t}",
        "}",
        "",
      ].join("\n"),
      "utf-8",
    );
    const lint = new LintService(cache, recorder.channel as never);
    await lint.lintPackage("gotest.lintfix");
    expect(lintDiagnostics(file).map((d) => d.code)).toContain("fail-guard");

    await lint.fixPackage("gotest.lintfix");

    expect(readFileSync(file, "utf-8")).toContain("gotest.Equal(t, 1+1, 2");
    expect(lintDiagnostics(file).map((d) => d.code)).not.toContain(
      "fail-guard",
    );
    lint.dispose();
  }, 300_000);
});
