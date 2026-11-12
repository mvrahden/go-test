// The commands the extension registers and the ones package.json contributes
// are one table: a contributed command nothing registers fails in the palette,
// and a registered one the manifest omits cannot be found there.

import { describe, it, expect, vi } from "vitest";
import { readFileSync, readdirSync } from "node:fs";
import * as path from "node:path";

const registered = vi.hoisted(() => [] as string[]);

// Modules read a few vscode values as they load; those get a stand-in, and
// only registration is real.
vi.mock("vscode", () => {
  const standIn: unknown = new Proxy(function () {}, {
    get: (_, key) => (key === "then" ? undefined : standIn),
    apply: () => standIn,
    construct: () => standIn as object,
  });
  return {
    CodeActionKind: standIn,
    commands: {
      registerCommand: (id: string) => {
        registered.push(id);
        return { dispose: () => {} };
      },
    },
  };
});

import { registerCommands, staleOnChange } from "./extension.js";

const srcDir = __dirname;
const manifest = JSON.parse(
  readFileSync(path.join(srcDir, "..", "package.json"), "utf-8"),
) as { contributes: { commands: { command: string }[] } };
const contributed = manifest.contributes.commands.map((c) => c.command);

// Registration only stores the handlers, so no dependency is touched.
function registeredIds(): string[] {
  registered.length = 0;
  const disposables = registerCommands({} as never);
  expect(disposables).toHaveLength(registered.length);
  return [...registered];
}

describe("the command table", () => {
  it("registers each command once", () => {
    const ids = registeredIds();
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("registers every command package.json contributes", () => {
    const ids = new Set(registeredIds());
    expect(contributed.filter((c) => !ids.has(c))).toEqual([]);
  });

  it("contributes every command it registers", () => {
    const declared = new Set(contributed);
    expect(registeredIds().filter((c) => !declared.has(c))).toEqual([]);
  });

  it("contributes each command once", () => {
    expect(new Set(contributed).size).toBe(contributed.length);
  });

  // Activation registers commands only through registerCommands, so the
  // table above is every command activation registers.
  it("registers commands nowhere else", () => {
    const registering = readdirSync(srcDir)
      .filter((f) => f.endsWith(".ts") && !f.includes(".test"))
      .filter((f) =>
        /\bcommands\.register\w*\(/.test(
          readFileSync(path.join(srcDir, f), "utf-8"),
        ),
      );
    expect(registering).toEqual(["extension.ts"]);
    const calls = readFileSync(
      path.join(srcDir, "extension.ts"),
      "utf-8",
    ).match(/\bcommands\.register\w*\(/g);
    expect(calls).toHaveLength(registeredIds().length);
  });
});

// Read as text: the parsed object cannot show a duplicate key.
const manifestText = readFileSync(
  path.join(srcDir, "..", "package.json"),
  "utf-8",
);
const NEEDS_ARGUMENTS = [
  "gotest.applyLintFix",
  "gotest.debugFuzz",
  "gotest.debugTest",
  "gotest.promoteCrashers",
  "gotest.runBench",
  "gotest.runBenchStable",
  "gotest.runFile",
  "gotest.runFuzz",
  "gotest.runTest",
  "gotest.scaffoldTarget",
  "gotest.toggleFocus",
  "gotest.triageCrashers",
];

describe("the command palette", () => {
  it("is one array in the manifest", () => {
    expect(manifestText.match(/"commandPalette"\s*:/g)).toHaveLength(1);
  });
  it("hides exactly the commands that need arguments", () => {
    const menus = JSON.parse(manifestText).contributes.menus as {
      commandPalette: { command: string; when: string }[];
    };
    const hidden = menus.commandPalette
      .filter((e) => e.when === "false")
      .map((e) => e.command)
      .sort();
    expect(hidden).toEqual(NEEDS_ARGUMENTS);
  });
});

describe("staleOnChange", () => {
  const controller = { invalidateResults: vi.fn() };
  it("invalidates the package of a changed file", () => {
    staleOnChange(
      "/ws/cart/cart.go",
      { resolveFileToPackage: () => "example.com/cart" },
      controller,
    );
    expect(controller.invalidateResults).toHaveBeenCalledWith(
      "example.com/cart",
    );
  });
  it("ignores a file outside every discovered package", () => {
    controller.invalidateResults.mockClear();
    staleOnChange(
      "/ws/README.go",
      { resolveFileToPackage: () => undefined },
      controller,
    );
    expect(controller.invalidateResults).not.toHaveBeenCalled();
  });
});
