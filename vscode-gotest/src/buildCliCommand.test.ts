import { describe, it, expect, vi, beforeEach } from "vitest";

const {
  mockExecFileAsync,
  mockReadFile,
  mockFileExists,
  mockResolveGoBinary,
  mockConfigValues,
} = vi.hoisted(() => ({
  mockExecFileAsync: vi.fn(
    async (): Promise<{ stdout: string; stderr: string }> => ({
      stdout: "",
      stderr: "",
    }),
  ),
  mockReadFile: vi.fn(async (_path?: unknown): Promise<string> => {
    throw new Error("ENOENT");
  }),
  mockFileExists: vi.fn(async () => false),
  mockResolveGoBinary: vi.fn(async () => "/usr/local/go/bin/go"),
  mockConfigValues: new Map<string, unknown>(),
}));

vi.mock("vscode", () => ({
  workspace: {
    workspaceFolders: [{ uri: { fsPath: "/workspace" } }],
    getConfiguration: vi.fn((_section: string, _scope?: unknown) => ({
      get: <T>(key: string, defaultValue?: T): T | undefined =>
        (mockConfigValues.has(key)
          ? mockConfigValues.get(key)
          : defaultValue) as T | undefined,
    })),
  },
  Uri: { file: (p: string) => ({ fsPath: p }) },
  window: {
    showWarningMessage: vi.fn(async () => undefined),
    showErrorMessage: vi.fn(),
  },
}));

vi.mock("./goBinary.js", () => ({
  resolveGoBinary: mockResolveGoBinary,
  fileExists: mockFileExists,
  clearGoBinaryCache: vi.fn(),
}));

vi.mock("node:fs/promises", () => ({
  readFile: mockReadFile,
}));

vi.mock("node:child_process", async () => {
  const util = await import("node:util");
  const execFileFn: Record<symbol, unknown> = vi.fn() as never;
  execFileFn[util.promisify.custom] = mockExecFileAsync;
  return { execFile: execFileFn };
});

import { buildCliCommand } from "./cli.js";

function setGoMod(dir: string, content: string) {
  mockReadFile.mockImplementation(async (filePath: unknown) => {
    if (filePath === `${dir}/go.mod`) return content;
    throw new Error("ENOENT");
  });
}

const GOTEST_MODULE = "github.com/mvrahden/go-test/cmd/gotest";

describe("buildCliCommand", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConfigValues.clear();
    mockReadFile.mockRejectedValue(new Error("ENOENT"));
    mockFileExists.mockResolvedValue(false);
    mockResolveGoBinary.mockResolvedValue("/usr/local/go/bin/go");
    mockExecFileAsync.mockResolvedValue({ stdout: "", stderr: "" });
  });

  describe("step 1: cliPath override", () => {
    it("uses cliPath when file exists and version meets minimum", async () => {
      mockConfigValues.set("cliPath", "/usr/local/bin/gotest");
      mockFileExists.mockResolvedValue(true);
      mockExecFileAsync.mockResolvedValue({
        stdout: "gotest v1.14.0\n",
        stderr: "",
      });

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd.bin).toBe("/usr/local/bin/gotest");
      expect(cmd.args).toEqual(["spec", "./..."]);
    });

    it("falls back when version is below minimum", async () => {
      mockConfigValues.set("cliPath", "/usr/local/bin/gotest");
      mockFileExists.mockResolvedValue(true);
      mockExecFileAsync.mockResolvedValue({
        stdout: "gotest v1.0.0\n",
        stderr: "",
      });

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd.bin).toBe("/usr/local/go/bin/go");
      expect(cmd.args[0]).toBe("run");
    });

    it("falls back when cliPath file does not exist", async () => {
      mockConfigValues.set("cliPath", "/nonexistent/gotest");
      mockFileExists.mockResolvedValue(false);

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd.bin).toBe("/usr/local/go/bin/go");
      expect(cmd.args[0]).toBe("run");
    });

    it("falls back when version output is unparseable", async () => {
      mockConfigValues.set("cliPath", "/usr/local/bin/gotest");
      mockFileExists.mockResolvedValue(true);
      mockExecFileAsync.mockResolvedValue({
        stdout: "unknown binary\n",
        stderr: "",
      });

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd.bin).toBe("/usr/local/go/bin/go");
      expect(cmd.args[0]).toBe("run");
    });

    it("falls back when version check throws", async () => {
      mockConfigValues.set("cliPath", "/usr/local/bin/gotest");
      mockFileExists.mockResolvedValue(true);
      mockExecFileAsync.mockRejectedValue(new Error("EACCES"));

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd.bin).toBe("/usr/local/go/bin/go");
      expect(cmd.args[0]).toBe("run");
    });

    it("resolves relative cliPath against workspaceDir", async () => {
      mockConfigValues.set("cliPath", "./bin/gotest");
      mockFileExists.mockResolvedValue(true);
      mockExecFileAsync.mockResolvedValue({
        stdout: "gotest v1.14.0\n",
        stderr: "",
      });

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.bin).toBe("/workspace/bin/gotest");
    });
  });

  describe("step 2: workspace is gotest module", () => {
    it("uses go run ./cmd/gotest when module declaration matches", async () => {
      setGoMod(
        "/workspace",
        "module github.com/mvrahden/go-test\n\ngo 1.24.0\n",
      );

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd).toEqual({
        bin: "/usr/local/go/bin/go",
        args: ["run", "./cmd/gotest", "spec", "./..."],
      });
    });

    it("matches when modulePath is a sub-path of declared module", async () => {
      setGoMod(
        "/workspace",
        "module github.com/mvrahden/go-test\n\ngo 1.24.0\n",
      );
      mockConfigValues.set(
        "modulePath",
        "github.com/mvrahden/go-test/cmd/gotest",
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.bin).toBe("/usr/local/go/bin/go");
      expect(cmd.args).toEqual(["run", "./cmd/gotest", "spec"]);
    });

    it("does not match when module declaration differs", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.14.0",
          ")",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).not.toBe("./cmd/gotest");
    });
  });

  describe("step 3: replace directive", () => {
    it("uses go run modulePath without version when replace exists", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.14.0",
          ")",
          `replace github.com/mvrahden/go-test => ../go-test`,
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd).toEqual({
        bin: "/usr/local/go/bin/go",
        args: ["run", GOTEST_MODULE, "spec", "./..."],
      });
    });

    it("detects replace for parent module path", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.14.0",
          ")",
          `replace github.com/mvrahden/go-test => ../go-test`,
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args).toEqual(["run", GOTEST_MODULE, "spec"]);
    });

    it("detects block-format replace", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.14.0",
          ")",
          "replace (",
          "\tgithub.com/mvrahden/go-test => ../go-test",
          ")",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(GOTEST_MODULE);
    });

    it("does not false-match replace for module with similar prefix", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.14.0",
          ")",
          "replace github.com/mvrahden/go-testing => ../go-testing",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.14.0`);
    });
  });

  describe("step 4: pinned version", () => {
    it("uses go run module@version", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.14.0",
          ")",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd).toEqual({
        bin: "/usr/local/go/bin/go",
        args: ["run", `${GOTEST_MODULE}@v1.14.0`, "spec", "./..."],
      });
    });

    it("extracts version from inline require format", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require github.com/mvrahden/go-test v1.14.0",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.14.0`);
    });

    it("finds version via parent module path walk", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.15.0",
          ")",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.15.0`);
    });

    it("does not include -- separator", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.14.0",
          ")",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd.args).not.toContain("--");
    });
  });

  describe("step 5: fallback", () => {
    it("uses go run module@latest when no go.mod exists", async () => {
      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd).toEqual({
        bin: "/usr/local/go/bin/go",
        args: ["run", `${GOTEST_MODULE}@latest`, "spec", "./..."],
      });
    });

    it("does not include -- separator", async () => {
      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd.args).not.toContain("--");
    });

    it("uses go run module@latest when version not found in go.mod", async () => {
      setGoMod(
        "/workspace",
        ["module github.com/myapp", "go 1.24.0"].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@latest`);
    });

    it("respects modulePath containing @ as-is", async () => {
      mockConfigValues.set("modulePath", `${GOTEST_MODULE}@v1.15.0`);

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.15.0`);
    });
  });

  describe("version below minimum", () => {
    it("falls through to fallback when pinned version is too old", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.0.0",
          ")",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@latest`);
    });
  });

  describe("buildTags", () => {
    it("appends -tags flag to subcommand args", async () => {
      mockConfigValues.set("buildTags", "integration,e2e");

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd.args).toContain("-tags=integration,e2e");
    });
  });

  describe("resolution priority", () => {
    it("cliPath takes precedence over workspace module match", async () => {
      mockConfigValues.set("cliPath", "/usr/local/bin/gotest");
      mockFileExists.mockResolvedValue(true);
      mockExecFileAsync.mockResolvedValue({
        stdout: "gotest v1.14.0\n",
        stderr: "",
      });
      setGoMod(
        "/workspace",
        "module github.com/mvrahden/go-test\n\ngo 1.24.0\n",
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.bin).toBe("/usr/local/bin/gotest");
    });

    it("workspace module takes precedence over pinned version", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/mvrahden/go-test",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.14.0",
          ")",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe("./cmd/gotest");
    });

    it("replace directive takes precedence over pinned version", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.14.0",
          ")",
          `replace github.com/mvrahden/go-test => ../go-test`,
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(GOTEST_MODULE);
      expect(cmd.args).not.toContain(`${GOTEST_MODULE}@v1.14.0`);
    });
  });
});
