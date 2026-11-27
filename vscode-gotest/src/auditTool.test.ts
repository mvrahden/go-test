import { describe, it, expect, vi } from "vitest";

const lm = vi.hoisted(() => ({
  registerTool: vi.fn((name: string) => ({ name, dispose: () => {} })),
}));

vi.mock("vscode", () => ({
  lm,
  MarkdownString: class {
    constructor(public value: string) {}
  },
  LanguageModelTextPart: class {
    constructor(public value: string) {}
  },
  LanguageModelToolResult: class {
    constructor(public content: unknown[]) {}
  },
}));

import * as vscode from "vscode";
import {
  AUDIT_TOOL,
  AuditTool,
  readInput,
  registerAuditTool,
} from "./auditTool.js";
import { readFileSync } from "node:fs";
import * as path from "node:path";

const token = {} as vscode.CancellationToken;

describe("the gotest_audit tool", () => {
  it("asks the user before an audit that runs tests, and only then", () => {
    const tool = new AuditTool({ folder: () => "/w", audit: async () => "" });
    const plain = tool.prepareInvocation({ input: {} } as never);
    expect(plain.confirmationMessages).toBeUndefined();
    const run = tool.prepareInvocation({
      input: { run: true, base: "origin/main" },
    } as never);
    expect(run.confirmationMessages?.title).toBe("Run tests for the audit?");
    expect(run.invocationMessage).toContain("against origin/main");
  });

  it("returns the folder's audit as the CLI's Markdown", async () => {
    const audit = vi.fn(async () => "# gotest audit\n");
    const tool = new AuditTool({ folder: () => "/w", audit });
    const result = (await tool.invoke(
      { input: { base: " main ", run: "yes" } } as never,
      token,
    )) as unknown as { content: { value: string }[] };
    expect(audit).toHaveBeenCalledWith("/w", { base: "main", run: false });
    expect(result.content[0].value).toBe("# gotest audit\n");
  });

  it("says so when no folder uses gotest", async () => {
    const tool = new AuditTool({
      folder: () => undefined,
      audit: async () => "x",
    });
    const result = (await tool.invoke(
      { input: {} } as never,
      token,
    )) as unknown as { content: { value: string }[] };
    expect(result.content[0].value).toMatch(/No workspace folder uses gotest/);
  });

  it("keeps only what the schema allows", () => {
    expect(readInput(null)).toEqual({ base: undefined, run: false });
    expect(readInput({ base: "", run: true })).toEqual({
      base: undefined,
      run: true,
    });
  });

  it("registers under the name package.json contributes, where the API is", () => {
    expect(
      registerAuditTool({ folder: () => "/w", audit: async () => "" }),
    ).toBeDefined();
    expect(lm.registerTool).toHaveBeenCalledWith(
      AUDIT_TOOL,
      expect.any(AuditTool),
    );
    const manifest = JSON.parse(
      readFileSync(path.join(__dirname, "..", "package.json"), "utf-8"),
    );
    const tool = manifest.contributes.languageModelTools[0];
    expect(tool.name).toBe(AUDIT_TOOL);
    expect(Object.keys(tool.inputSchema.properties).sort()).toEqual([
      "base",
      "run",
    ]);
  });

  it("is not offered where the editor lacks the API", () => {
    const saved = lm.registerTool;
    (lm as { registerTool?: unknown }).registerTool = undefined;
    try {
      expect(
        registerAuditTool({ folder: () => "/w", audit: async () => "" }),
      ).toBeUndefined();
    } finally {
      lm.registerTool = saved;
    }
  });
});
