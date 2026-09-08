import * as path from "node:path";
import { describe, it, expect, vi, beforeEach } from "vitest";

const {
  mockExecFileAsync,
  mockReadFile,
  mockFileExists,
  mockResolveGoBinary,
  mockConfigValues,
  mockConfigUpdate,
  mockShowInformationMessage,
  mockShowWarningMessage,
} = vi.hoisted(() => ({
  mockConfigUpdate: vi.fn(async () => undefined),
  mockShowInformationMessage: vi.fn<
    (message: string, ...items: string[]) => Promise<string | undefined>
  >(async () => undefined),
  mockShowWarningMessage: vi.fn<
    (message: string, ...items: string[]) => Promise<string | undefined>
  >(async () => undefined),
  mockExecFileAsync: vi.fn(
    async (
      _file: string,
      _args: string[],
      _opts?: unknown,
    ): Promise<{ stdout: string; stderr: string }> => ({
      stdout: "",
      stderr: "",
    }),
  ),
  mockReadFile: vi.fn(async (_path?: unknown): Promise<string> => {
    throw new Error("ENOENT");
  }),
  mockFileExists: vi.fn(async (_p: string) => false),
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
      update: mockConfigUpdate,
    })),
  },
  Uri: { file: (p: string) => ({ fsPath: p }) },
  ConfigurationTarget: { WorkspaceFolder: 3 },
  window: {
    showWarningMessage: mockShowWarningMessage,
    showInformationMessage: mockShowInformationMessage,
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

import {
  buildCliCommand,
  clearBinaryCache,
  CliUnavailableError,
} from "./cli.js";

function setGoMod(dir: string, content: string) {
  setFiles({ [path.join(dir, "go.mod")]: content });
}

// setFiles makes readFile answer for exactly these absolute paths. Both
// sides are resolved so the fixtures read the same on Windows.
function setFiles(files: Record<string, string>) {
  const resolved = new Map(
    Object.entries(files).map(([p, c]) => [path.resolve(p), c]),
  );
  mockReadFile.mockImplementation(async (filePath: unknown) => {
    const content = resolved.get(path.resolve(String(filePath)));
    if (content !== undefined) return content;
    throw new Error("ENOENT");
  });
}

// flush lets the fire-and-forget prompt chains settle.
async function flush() {
  for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0));
}

const PINNED = (version: string, extra: string[] = []) =>
  [
    "module github.com/myapp",
    "go 1.25.0",
    "require (",
    `\tgithub.com/mvrahden/go-test ${version}`,
    ")",
    ...extra,
  ].join("\n");

const GOTEST_MODULE = "github.com/mvrahden/go-test/cmd/gotest";

describe("buildCliCommand", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConfigValues.clear();
    mockReadFile.mockRejectedValue(new Error("ENOENT"));
    mockFileExists.mockResolvedValue(false);
    mockResolveGoBinary.mockResolvedValue("/usr/local/go/bin/go");
    mockExecFileAsync.mockResolvedValue({ stdout: "", stderr: "" });
    clearBinaryCache();
  });

  describe("step 1: cliPath override", () => {
    it("uses cliPath when file exists and version meets minimum", async () => {
      mockConfigValues.set("cliPath", "/usr/local/bin/gotest");
      mockFileExists.mockResolvedValue(true);
      mockExecFileAsync.mockResolvedValue({
        stdout: "gotest v1.27.0\n",
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
        stdout: "gotest v1.27.0\n",
        stderr: "",
      });

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.bin).toBe(path.resolve("/workspace", "bin/gotest"));
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
          "\tgithub.com/mvrahden/go-test v1.27.0",
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
          "\tgithub.com/mvrahden/go-test v1.27.0",
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

    // A replace directive points at local source, and the require version
    // beside it is conventionally a placeholder. Gating the replace path on
    // that version silently downloaded a release instead of building the
    // developer's own working tree.
    it("honours replace even when the require version is a placeholder", async () => {
      setGoMod(
        "/workspace",
        [
          "module gotest.fixtures",
          "go 1.24.0",
          "require github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000",
          `replace github.com/mvrahden/go-test => ../../..`,
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["discover", "./..."], "/workspace");

      expect(cmd).toEqual({
        bin: "/usr/local/go/bin/go",
        args: ["run", GOTEST_MODULE, "discover", "./..."],
      });
    });

    it("honours replace even when the require version is below the minimum", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require github.com/mvrahden/go-test v1.0.0",
          `replace github.com/mvrahden/go-test => ../go-test`,
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args).toEqual(["run", GOTEST_MODULE, "spec"]);
    });

    it("detects replace for parent module path", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.27.0",
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
          "\tgithub.com/mvrahden/go-test v1.27.0",
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
          "\tgithub.com/mvrahden/go-test v1.27.0",
          ")",
          "replace github.com/mvrahden/go-testing => ../go-testing",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.27.0`);
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
          "\tgithub.com/mvrahden/go-test v1.27.0",
          ")",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd).toEqual({
        bin: "/usr/local/go/bin/go",
        args: ["run", `${GOTEST_MODULE}@v1.27.0`, "spec", "./..."],
      });
    });

    it("extracts version from inline require format", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require github.com/mvrahden/go-test v1.27.0",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.27.0`);
    });

    it("finds version via parent module path walk", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.28.0",
          ")",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.28.0`);
    });

    it("does not include -- separator", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.27.0",
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
      mockConfigValues.set("modulePath", `${GOTEST_MODULE}@v1.28.0`);

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.28.0`);
    });
  });

  describe("version below minimum", () => {
    it("refuses instead of substituting a newer CLI", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));

      await expect(buildCliCommand(["spec"], "/workspace")).rejects.toThrow(
        CliUnavailableError,
      );
      await expect(buildCliCommand(["spec"], "/workspace")).rejects.toThrow(
        /v1\.0\.0.*v1\.27\.0/,
      );
      expect(mockExecFileAsync).not.toHaveBeenCalled();
    });

    it("shows the upgrade warning once per session", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);

      expect(mockShowWarningMessage).toHaveBeenCalledTimes(1);
    });

    it("Upgrade declares the tool at latest in the module that pins", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(mockExecFileAsync).toHaveBeenCalledWith(
        "/usr/local/go/bin/go",
        ["get", "-tool", `${GOTEST_MODULE}@latest`],
        expect.objectContaining({ cwd: "/workspace" }),
      );
    });

    it("Upgrade re-vendors a vendored module", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      mockFileExists.mockImplementation(
        async (p: string) =>
          p === path.join("/workspace", "vendor", "modules.txt"),
      );
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      const calls = mockExecFileAsync.mock.calls.map((c) => c[1]);
      expect(calls).toEqual([
        ["get", "-tool", `${GOTEST_MODULE}@latest`],
        ["mod", "vendor"],
      ]);
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
        stdout: "gotest v1.27.0\n",
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
          "\tgithub.com/mvrahden/go-test v1.27.0",
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
          "\tgithub.com/mvrahden/go-test v1.27.0",
          ")",
          `replace github.com/mvrahden/go-test => ../go-test`,
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(GOTEST_MODULE);
      expect(cmd.args).not.toContain(`${GOTEST_MODULE}@v1.27.0`);
    });
  });

  describe("tool directive", () => {
    it("runs go tool with the full package path when go.mod declares it", async () => {
      setGoMod("/workspace", PINNED("v1.28.1", [`tool ${GOTEST_MODULE}`]));

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd).toEqual({
        bin: "/usr/local/go/bin/go",
        args: ["tool", GOTEST_MODULE, "spec", "./..."],
      });
    });

    it("recognises the block form", async () => {
      setGoMod(
        "/workspace",
        PINNED("v1.28.1", [
          "tool (",
          "\tgolang.org/x/tools/cmd/stringer",
          `\t${GOTEST_MODULE}`,
          ")",
        ]),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args.slice(0, 2)).toEqual(["tool", GOTEST_MODULE]);
    });

    it("ignores tool lines for other packages", async () => {
      setGoMod(
        "/workspace",
        PINNED("v1.28.1", ["tool golang.org/x/tools/cmd/stringer"]),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args.slice(0, 2)).toEqual(["run", `${GOTEST_MODULE}@v1.28.1`]);
    });

    it("uses the configured module path, so a fork resolves to its own tool", async () => {
      const fork = "github.com/fork/go-test/cmd/gotest";
      mockConfigValues.set("modulePath", fork);
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.25.0",
          "require github.com/fork/go-test v1.28.1",
          `tool ${fork}`,
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args.slice(0, 2)).toEqual(["tool", fork]);
    });

    it("wins over a replace directive, which go tool honours natively", async () => {
      setGoMod(
        "/workspace",
        PINNED("v0.0.0-00010101000000-000000000000", [
          "replace github.com/mvrahden/go-test => ../go-test",
          `tool ${GOTEST_MODULE}`,
        ]),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args.slice(0, 2)).toEqual(["tool", GOTEST_MODULE]);
    });

    it("still refuses a pin below the floor", async () => {
      setGoMod("/workspace", PINNED("v1.26.0", [`tool ${GOTEST_MODULE}`]));

      await expect(buildCliCommand(["spec"], "/workspace")).rejects.toThrow(
        CliUnavailableError,
      );
    });

    it("does not offer to add the directive when it is already declared", async () => {
      setGoMod("/workspace", PINNED("v1.28.1", [`tool ${GOTEST_MODULE}`]));

      await buildCliCommand(["spec"], "/workspace");
      await flush();

      expect(mockShowInformationMessage).not.toHaveBeenCalled();
    });
  });

  describe("go.work root", () => {
    const work = "go 1.25.0\n\nuse (\n\t./a\n\t./b\n)\n";

    it("pins the workspace's selected version, the max over use modules", async () => {
      setFiles({
        "/workspace/go.work": work,
        "/workspace/a/go.mod": PINNED("v1.28.0"),
        "/workspace/b/go.mod": PINNED("v1.28.1"),
      });

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args.slice(0, 2)).toEqual(["run", `${GOTEST_MODULE}@v1.28.1`]);
    });

    it("runs go tool when any use module declares the directive", async () => {
      setFiles({
        "/workspace/go.work": work,
        "/workspace/a/go.mod": PINNED("v1.28.0"),
        "/workspace/b/go.mod": PINNED("v1.28.1", [`tool ${GOTEST_MODULE}`]),
      });

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args.slice(0, 2)).toEqual(["tool", GOTEST_MODULE]);
    });

    it("accepts single-line use directives and a dot entry", async () => {
      setFiles({
        "/workspace/go.work": "go 1.25.0\n\nuse .\nuse ./lib\n",
        "/workspace/go.mod": "module github.com/myapp\n\ngo 1.25.0\n",
        "/workspace/lib/go.mod": PINNED("v1.28.1"),
      });

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args.slice(0, 2)).toEqual(["run", `${GOTEST_MODULE}@v1.28.1`]);
    });

    it("refuses when the workspace selects a version below the floor", async () => {
      setFiles({
        "/workspace/go.work": work,
        "/workspace/a/go.mod": PINNED("v1.25.0"),
        "/workspace/b/go.mod": PINNED("v1.26.0"),
      });

      await expect(buildCliCommand(["spec"], "/workspace")).rejects.toThrow(
        /v1\.26\.0/,
      );
    });

    it("keeps the bootstrap fallback when no use module requires gotest", async () => {
      setFiles({
        "/workspace/go.work": work,
        "/workspace/a/go.mod": "module github.com/a\n\ngo 1.25.0\n",
        "/workspace/b/go.mod": "module github.com/b\n\ngo 1.25.0\n",
      });

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@latest`);
    });

    it("Upgrade runs in every use module that pins gotest", async () => {
      setFiles({
        "/workspace/go.work": work,
        "/workspace/a/go.mod": PINNED("v1.25.0"),
        "/workspace/b/go.mod": "module github.com/b\n\ngo 1.25.0\n",
      });
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(mockExecFileAsync).toHaveBeenCalledTimes(1);
      expect(mockExecFileAsync).toHaveBeenCalledWith(
        "/usr/local/go/bin/go",
        ["get", "-tool", `${GOTEST_MODULE}@latest`],
        expect.objectContaining({ cwd: path.resolve("/workspace", "a") }),
      );
    });
  });

  describe("tool directive suggestion", () => {
    it("offers once per root when the pin is usable but no directive exists", async () => {
      setGoMod("/workspace", PINNED("v1.28.1"));

      await buildCliCommand(["spec"], "/workspace");
      await buildCliCommand(["discover"], "/workspace");
      await flush();

      expect(mockShowInformationMessage).toHaveBeenCalledTimes(1);
      expect(mockShowInformationMessage.mock.calls[0][0]).toMatch(
        /go get -tool/,
      );
    });

    it("adds the directive at the pinned version, never at latest", async () => {
      setGoMod("/workspace", PINNED("v1.28.1"));
      mockShowInformationMessage.mockResolvedValue("Add to go.mod");

      await buildCliCommand(["spec"], "/workspace");
      await flush();

      expect(mockExecFileAsync).toHaveBeenCalledWith(
        "/usr/local/go/bin/go",
        ["get", "-tool", `${GOTEST_MODULE}@v1.28.1`],
        expect.objectContaining({ cwd: "/workspace" }),
      );
    });

    it("stays silent in a vendored module", async () => {
      setGoMod("/workspace", PINNED("v1.28.1"));
      mockFileExists.mockImplementation(
        async (p: string) =>
          p === path.join("/workspace", "vendor", "modules.txt"),
      );

      await buildCliCommand(["spec"], "/workspace");
      await flush();

      expect(mockShowInformationMessage).not.toHaveBeenCalled();
    });

    it("stays silent when the setting is off", async () => {
      mockConfigValues.set("suggestToolDirective", false);
      setGoMod("/workspace", PINNED("v1.28.1"));

      await buildCliCommand(["spec"], "/workspace");
      await flush();

      expect(mockShowInformationMessage).not.toHaveBeenCalled();
    });

    it("Don't ask again turns the setting off for the folder", async () => {
      setGoMod("/workspace", PINNED("v1.28.1"));
      mockShowInformationMessage.mockResolvedValue("Don't ask again");

      await buildCliCommand(["spec"], "/workspace");
      await flush();

      expect(mockConfigUpdate).toHaveBeenCalledWith(
        "suggestToolDirective",
        false,
        3,
      );
      expect(mockExecFileAsync).not.toHaveBeenCalled();
    });

    it("does not offer when a replace directive is in play", async () => {
      setGoMod(
        "/workspace",
        PINNED("v1.28.1", [
          "replace github.com/mvrahden/go-test => ../go-test",
        ]),
      );

      await buildCliCommand(["spec"], "/workspace");
      await flush();

      expect(mockShowInformationMessage).not.toHaveBeenCalled();
    });
  });
});
