import * as os from "node:os";
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
  mockShowErrorMessage,
  mockMkdtemp,
  mockWriteFile,
  mockRm,
  progressToken,
} = vi.hoisted(() => ({
  mockMkdtemp: vi.fn(async (prefix: string) => `${prefix}XYZ`),
  mockWriteFile: vi.fn(async (_p: string, _data: string) => undefined),
  mockRm: vi.fn(async (_p: string, _opts?: unknown) => undefined),
  // progressToken is the cancellation token every withProgress task gets.
  progressToken: {
    isCancellationRequested: false,
    listeners: [] as (() => void)[],
    onCancellationRequested(fn: () => void) {
      this.listeners.push(fn);
      return { dispose: () => undefined };
    },
    cancel() {
      this.isCancellationRequested = true;
      for (const fn of this.listeners) fn();
    },
    reset() {
      this.isCancellationRequested = false;
      this.listeners = [];
    },
  },
  mockShowErrorMessage: vi.fn(),
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
  ProgressLocation: { Notification: 15 },
  window: {
    withProgress: async (
      _opts: unknown,
      task: (p: unknown, t: unknown) => Promise<unknown>,
    ) => task({ report: () => undefined }, progressToken),
    showWarningMessage: mockShowWarningMessage,
    showInformationMessage: mockShowInformationMessage,
    showErrorMessage: mockShowErrorMessage,
  },
}));

vi.mock("./goBinary.js", () => ({
  resolveGoBinary: mockResolveGoBinary,
  fileExists: mockFileExists,
  clearGoBinaryCache: vi.fn(),
}));

vi.mock("node:fs/promises", () => ({
  readFile: mockReadFile,
  mkdtemp: mockMkdtemp,
  writeFile: mockWriteFile,
  rm: mockRm,
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
  CliBelowFloorError,
  CliUnavailableError,
  offerCliUpgrade,
  planUpgrade,
  compareGoVersions,
  goLineFits,
  goReleaseQuery,
  lookupGoFlags,
  type ReleaseQuery,
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

const PINNED = (version: string, extra: string[] = [], go = "1.25.0") =>
  [
    "module github.com/myapp",
    `go ${go}`,
    "require (",
    `\tgithub.com/mvrahden/go-test ${version}`,
    ")",
    ...extra,
  ].join("\n");

const GOTEST_MODULE = "github.com/mvrahden/go-test/cmd/gotest";

const LOOKUP_DIR = path.join(os.tmpdir(), "gotest-upgrade-XYZ");

type ExecOpts = { cwd?: string; env?: NodeJS.ProcessEnv; signal?: AbortSignal };

// listJSON answers go list -m -json for args the way the go command does:
// -versions as one object, otherwise one object per mod@version, with an
// Error for a release that is an Error, and a GOFLAGS for go env.
function listJSON(
  args: string[],
  releases: Record<string, string | Error>,
  goflags = "",
): string {
  if (args[0] === "env") return `${goflags}\n`;
  if (args.includes("-versions")) {
    return JSON.stringify(
      { Path: MOD, Versions: Object.keys(releases) },
      null,
      "\t",
    );
  }
  return args
    .filter((a) => a.startsWith(`${MOD}@`))
    .map((a) => a.split("@")[1])
    .map((v) => {
      const r = releases[v];
      const m =
        r instanceof Error
          ? { Path: MOD, Version: v, Error: { Err: r.message } }
          : { Path: MOD, Version: v, GoVersion: r };
      return JSON.stringify(m, null, "\t") + "\n";
    })
    .join("");
}

// scriptReleases makes go answer as the module proxy would for these
// releases and their go lines, or fail with the given error.
function scriptReleases(
  releases: Record<string, string | Error> | Error,
  goflags = "",
) {
  mockExecFileAsync.mockImplementation(async (_file, args) => {
    if (args[0] !== "list" && args[0] !== "env") {
      return { stdout: "", stderr: "" };
    }
    if (releases instanceof Error) throw releases;
    return { stdout: listJSON(args, releases, goflags), stderr: "" };
  });
}

// hangLookups makes every lookup go command run until its signal aborts,
// rejecting then as execFile does; anything else succeeds at once.
function hangLookups() {
  mockExecFileAsync.mockImplementation(async (_file, args, opts) => {
    if (args[0] !== "list" && args[0] !== "env") {
      return { stdout: "", stderr: "" };
    }
    const { signal } = opts as ExecOpts;
    return new Promise((_resolve, reject) => {
      signal?.addEventListener("abort", () => {
        const err = new Error("The operation was aborted");
        err.name = "AbortError";
        reject(err);
      });
    });
  });
}

// lookups lists the go commands of the release lookup.
function lookups() {
  return mockExecFileAsync.mock.calls.filter(
    (c) => c[1][0] === "list" || c[1][0] === "env",
  );
}

// goGets lists the go get invocations, each as [cwd, version argument].
function goGets(): [string, string][] {
  return mockExecFileAsync.mock.calls
    .filter((c) => c[1][0] === "get")
    .map((c) => [(c[2] as { cwd: string }).cwd, c[1][2]]);
}

// Pins and binaries in these fixtures are v1.90 and up: versions no floor
// will reach, so the floor can move without a fixture moving with it. The
// floor's own boundary is pinned where checkProducerVersion is tested.
describe("buildCliCommand", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConfigValues.clear();
    mockReadFile.mockRejectedValue(new Error("ENOENT"));
    mockFileExists.mockResolvedValue(false);
    mockResolveGoBinary.mockResolvedValue("/usr/local/go/bin/go");
    mockExecFileAsync.mockResolvedValue({ stdout: "", stderr: "" });
    progressToken.reset();
    clearBinaryCache();
  });

  describe("step 1: cliPath override", () => {
    it("uses cliPath when file exists and version meets minimum", async () => {
      mockConfigValues.set("cliPath", "/usr/local/bin/gotest");
      mockFileExists.mockResolvedValue(true);
      mockExecFileAsync.mockResolvedValue({
        stdout: "gotest v1.90.0\n",
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
        stdout: "gotest v1.90.0\n",
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
          "\tgithub.com/mvrahden/go-test v1.90.0",
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
          "\tgithub.com/mvrahden/go-test v1.90.0",
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
          "\tgithub.com/mvrahden/go-test v1.90.0",
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
          "\tgithub.com/mvrahden/go-test v1.90.0",
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
          "\tgithub.com/mvrahden/go-test v1.90.0",
          ")",
          "replace github.com/mvrahden/go-testing => ../go-testing",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.90.0`);
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
          "\tgithub.com/mvrahden/go-test v1.90.0",
          ")",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd).toEqual({
        bin: "/usr/local/go/bin/go",
        args: ["run", `${GOTEST_MODULE}@v1.90.0`, "spec", "./..."],
      });
    });

    it("extracts version from inline require format", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require github.com/mvrahden/go-test v1.90.0",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.90.0`);
    });

    it("finds version via parent module path walk", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.91.0",
          ")",
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.91.0`);
    });

    it("does not include -- separator", async () => {
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.24.0",
          "require (",
          "\tgithub.com/mvrahden/go-test v1.90.0",
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
      mockConfigValues.set("modulePath", `${GOTEST_MODULE}@v1.91.0`);

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(`${GOTEST_MODULE}@v1.91.0`);
    });
  });

  describe("version below minimum", () => {
    it("refuses instead of substituting a newer CLI", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));

      await expect(buildCliCommand(["spec"], "/workspace")).rejects.toThrow(
        CliUnavailableError,
      );
      await expect(buildCliCommand(["spec"], "/workspace")).rejects.toThrow(
        /v1\.0\.0.*v1\.34\.0/,
      );
      await expect(buildCliCommand(["spec"], "/workspace")).rejects.toThrow(
        `Run: go get -tool ${GOTEST_MODULE}@v1.34.0`,
      );
      expect(mockExecFileAsync).not.toHaveBeenCalled();
    });

    it("shows the upgrade warning once per session", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);

      expect(mockShowWarningMessage).toHaveBeenCalledTimes(1);
    });

    it("offers the upgrade again when a user action fails after the dialog closed", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();
      const err = await buildCliCommand(["spec"], "/workspace").catch(
        (e: unknown) => e,
      );

      expect(err).toBeInstanceOf(CliBelowFloorError);
      expect(offerCliUpgrade(err)).toBe(true);
      expect(mockShowWarningMessage).toHaveBeenCalledTimes(2);
      expect(mockShowWarningMessage).toHaveBeenLastCalledWith(
        expect.stringContaining("pins gotest v1.0.0"),
        "Upgrade",
      );
    });

    it("does not stack a second dialog while one is open", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      let close: (choice: string | undefined) => void = () => undefined;
      mockShowWarningMessage.mockReturnValueOnce(
        new Promise((r) => (close = r)),
      );

      const err = await buildCliCommand(["spec"], "/workspace").catch(
        (e: unknown) => e,
      );
      expect(offerCliUpgrade(err)).toBe(true);
      expect(mockShowWarningMessage).toHaveBeenCalledTimes(1);

      close(undefined);
      await flush();
    });

    it("leaves every other failure to the caller", () => {
      expect(offerCliUpgrade(new CliUnavailableError("no version"))).toBe(
        false,
      );
      expect(offerCliUpgrade(new Error("boom"))).toBe(false);
      expect(mockShowWarningMessage).not.toHaveBeenCalled();
    });

    it("Upgrade declares the newest release the module's go line takes, never @latest", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      scriptReleases({
        "v1.90.0": "1.24.0",
        "v1.91.0": "1.25.0",
        "v1.92.0": "1.26.0",
      });
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(goGets()).toEqual([["/workspace", `${GOTEST_MODULE}@v1.91.0`]]);
      expect(mockShowInformationMessage).toHaveBeenCalledWith(
        "gotest: upgraded to v1.91.0 and declared as a tool.",
      );
    });

    it("Upgrade asks the proxy from a private module, outside the user's module and workspace", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      scriptReleases({ "v1.91.0": "1.25.0" });
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(mockMkdtemp).toHaveBeenCalledWith(
        path.join(os.tmpdir(), "gotest-upgrade-"),
      );
      expect(mockWriteFile).toHaveBeenCalledWith(
        path.join(LOOKUP_DIR, "go.mod"),
        "module gotest.upgrade.lookup\n",
      );
      expect(lookups().map((c) => c[1].slice(0, 4))).toEqual([
        ["env", "GOFLAGS"],
        ["list", "-m", "-json", "-versions"],
        ["list", "-m", "-json", "-e"],
      ]);
      for (const [, , opts] of lookups()) {
        expect(opts).toMatchObject({
          cwd: LOOKUP_DIR,
          env: expect.objectContaining({ GOWORK: "off" }),
        });
      }
      expect(mockRm).toHaveBeenCalledWith(LOOKUP_DIR, {
        recursive: true,
        force: true,
      });
      expect(goGets()).toEqual([["/workspace", `${GOTEST_MODULE}@v1.91.0`]]);
    });

    it("Upgrade removes the private module when the lookup fails", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      scriptReleases(new Error("no network"));
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(mockRm).toHaveBeenCalledWith(LOOKUP_DIR, {
        recursive: true,
        force: true,
      });
    });

    it("Upgrade looks up without the user's -mod and -modfile, keeping every other flag", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      scriptReleases(
        { "v1.91.0": "1.25.0" },
        "-modfile=/workspace/go.alt.mod -mod=vendor -tags=integration -modcacherw",
      );
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      const lists = lookups().filter((c) => c[1][0] === "list");
      expect(lists).toHaveLength(2);
      for (const [, , opts] of lists) {
        expect((opts as ExecOpts).env?.GOFLAGS).toBe(
          "-tags=integration -modcacherw -mod=mod",
        );
      }
      expect(goGets()).toEqual([["/workspace", `${GOTEST_MODULE}@v1.91.0`]]);
    });

    it("Upgrade skips a release the proxy cannot serve and takes the next that fits", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      scriptReleases({
        "v1.90.0": "1.24.0",
        "v1.91.0": "1.25.0",
        "v1.91.1": new Error("reading v1.91.1.info: 410 Gone"),
      });
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(goGets()).toEqual([["/workspace", `${GOTEST_MODULE}@v1.91.0`]]);
      expect(mockShowErrorMessage).not.toHaveBeenCalled();
    });

    it("Upgrade fails only when no release resolves", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      scriptReleases({
        "v1.90.0": new Error("unknown revision v1.90.0"),
        "v1.91.0": new Error("410 Gone"),
      });
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(goGets()).toEqual([]);
      expect(mockShowErrorMessage).toHaveBeenCalledWith(
        "gotest: upgrade failed, go.mod is unchanged: could not look up gotest releases. " +
          "none of the releases of github.com/mvrahden/go-test at or above v1.34.0 could be resolved: v1.91.0, v1.90.0",
      );
    });

    it("a floor hit during the upgrade starts no second one and says it is running", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      let release: () => void = () => undefined;
      const lookupDone = new Promise<void>((r) => (release = r));
      mockExecFileAsync.mockImplementation(async (_file, args) => {
        if (args[0] !== "list" && args[0] !== "env") {
          return { stdout: "", stderr: "" };
        }
        await lookupDone;
        return { stdout: listJSON(args, { "v1.91.0": "1.25.0" }), stderr: "" };
      });
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      const err = await buildCliCommand(["spec"], "/workspace").catch(
        (e: unknown) => e,
      );
      await flush();
      expect(offerCliUpgrade(err)).toBe(true);
      release();
      await flush();

      expect(mockShowWarningMessage).toHaveBeenCalledTimes(1);
      expect(mockShowInformationMessage).toHaveBeenCalledWith(
        "gotest: the CLI upgrade is still running; try again when it finishes.",
      );
      expect(goGets()).toEqual([["/workspace", `${GOTEST_MODULE}@v1.91.0`]]);

      // Finished, the next floor hit offers the upgrade again.
      offerCliUpgrade(err);
      await flush();
      expect(mockShowWarningMessage).toHaveBeenCalledTimes(2);
    });

    it("a failed upgrade offers it again on the next floor hit", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      scriptReleases(new Error("no network"));
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      const err = await buildCliCommand(["spec"], "/workspace").catch(
        (e: unknown) => e,
      );
      await flush();
      offerCliUpgrade(err);
      await flush();

      expect(mockShowWarningMessage).toHaveBeenCalledTimes(2);
    });

    it("cancelling the lookup stops its go command and leaves go.mod unchanged", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      hangLookups();
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();
      const [, , opts] = lookups()[0];
      expect((opts as ExecOpts).signal?.aborted).toBe(false);
      progressToken.cancel();
      await flush();

      expect((opts as ExecOpts).signal?.aborted).toBe(true);
      expect(goGets()).toEqual([]);
      expect(mockShowErrorMessage).not.toHaveBeenCalled();
      expect(mockRm).toHaveBeenCalledWith(LOOKUP_DIR, {
        recursive: true,
        force: true,
      });
    });

    it("Upgrade changes nothing when no release fits the go line, and says what is needed", async () => {
      setGoMod("/workspace", PINNED("v1.0.0", [], "1.24.0"));
      scriptReleases({ "v1.91.0": "1.25.0", "v1.92.0": "1.26.0" });
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(goGets()).toEqual([]);
      expect(mockShowWarningMessage).toHaveBeenLastCalledWith(
        `gotest: v1.34.0 and newer need go 1.25.0, but go.mod declares go 1.24.0. ` +
          `Raise the go line to upgrade: go get -tool ${GOTEST_MODULE}@v1.91.0 raises it.`,
      );
      expect(mockShowInformationMessage).not.toHaveBeenCalled();
    });

    it("Upgrade changes nothing when the releases cannot be listed", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      scriptReleases(
        new Error("dial tcp: lookup proxy.golang.org: no such host"),
      );
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(goGets()).toEqual([]);
      expect(mockShowErrorMessage).toHaveBeenCalledWith(
        "gotest: upgrade failed, go.mod is unchanged: could not look up gotest releases. dial tcp: lookup proxy.golang.org: no such host",
      );
      expect(mockShowInformationMessage).not.toHaveBeenCalled();
    });

    it("Upgrade re-vendors a vendored module", async () => {
      setGoMod("/workspace", PINNED("v1.0.0"));
      scriptReleases({ "v1.91.0": "1.25.0" });
      mockFileExists.mockImplementation(
        async (p: string) =>
          p === path.join("/workspace", "vendor", "modules.txt"),
      );
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      const calls = mockExecFileAsync.mock.calls
        .map((c) => c[1])
        .filter((args) => args[0] !== "list" && args[0] !== "env");
      expect(calls).toEqual([
        ["get", "-tool", `${GOTEST_MODULE}@v1.91.0`],
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
        stdout: "gotest v1.90.0\n",
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
          "\tgithub.com/mvrahden/go-test v1.90.0",
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
          "\tgithub.com/mvrahden/go-test v1.90.0",
          ")",
          `replace github.com/mvrahden/go-test => ../go-test`,
        ].join("\n"),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args[1]).toBe(GOTEST_MODULE);
      expect(cmd.args).not.toContain(`${GOTEST_MODULE}@v1.90.0`);
    });
  });

  describe("tool directive", () => {
    it("runs go tool with the full package path when go.mod declares it", async () => {
      setGoMod("/workspace", PINNED("v1.92.0", [`tool ${GOTEST_MODULE}`]));

      const cmd = await buildCliCommand(["spec", "./..."], "/workspace");

      expect(cmd).toEqual({
        bin: "/usr/local/go/bin/go",
        args: ["tool", GOTEST_MODULE, "spec", "./..."],
      });
    });

    it("recognises the block form", async () => {
      setGoMod(
        "/workspace",
        PINNED("v1.92.0", [
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
        PINNED("v1.92.0", ["tool golang.org/x/tools/cmd/stringer"]),
      );

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args.slice(0, 2)).toEqual(["run", `${GOTEST_MODULE}@v1.92.0`]);
    });

    it("uses the configured module path, so a fork resolves to its own tool", async () => {
      const fork = "github.com/fork/go-test/cmd/gotest";
      mockConfigValues.set("modulePath", fork);
      setGoMod(
        "/workspace",
        [
          "module github.com/myapp",
          "go 1.25.0",
          "require github.com/fork/go-test v1.92.0",
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
      setGoMod("/workspace", PINNED("v1.92.0", [`tool ${GOTEST_MODULE}`]));

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
        "/workspace/a/go.mod": PINNED("v1.91.0"),
        "/workspace/b/go.mod": PINNED("v1.92.0"),
      });

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args.slice(0, 2)).toEqual(["run", `${GOTEST_MODULE}@v1.92.0`]);
    });

    it("runs go tool when any use module declares the directive", async () => {
      setFiles({
        "/workspace/go.work": work,
        "/workspace/a/go.mod": PINNED("v1.91.0"),
        "/workspace/b/go.mod": PINNED("v1.92.0", [`tool ${GOTEST_MODULE}`]),
      });

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args.slice(0, 2)).toEqual(["tool", GOTEST_MODULE]);
    });

    it("accepts single-line use directives and a dot entry", async () => {
      setFiles({
        "/workspace/go.work": "go 1.25.0\n\nuse .\nuse ./lib\n",
        "/workspace/go.mod": "module github.com/myapp\n\ngo 1.25.0\n",
        "/workspace/lib/go.mod": PINNED("v1.92.0"),
      });

      const cmd = await buildCliCommand(["spec"], "/workspace");

      expect(cmd.args.slice(0, 2)).toEqual(["run", `${GOTEST_MODULE}@v1.92.0`]);
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
      scriptReleases({ "v1.91.0": "1.25.0" });
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(goGets()).toEqual([
        [path.resolve("/workspace", "a"), `${GOTEST_MODULE}@v1.91.0`],
      ]);
    });

    it("Upgrade gives each use module the newest release its own go line takes", async () => {
      setFiles({
        "/workspace/go.work": "go 1.26.0\n\nuse (\n\t./a\n\t./b\n)\n",
        "/workspace/a/go.mod": PINNED("v1.25.0", [], "1.25.0"),
        "/workspace/b/go.mod": PINNED("v1.26.0", [], "1.26.0"),
      });
      scriptReleases({ "v1.91.0": "1.25.0", "v1.92.0": "1.26.0" });
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(goGets()).toEqual([
        [path.resolve("/workspace", "a"), `${GOTEST_MODULE}@v1.91.0`],
        [path.resolve("/workspace", "b"), `${GOTEST_MODULE}@v1.92.0`],
      ]);
      expect(mockShowInformationMessage).toHaveBeenCalledWith(
        "gotest: upgraded to v1.91.0 in a/go.mod, v1.92.0 in b/go.mod and declared as a tool.",
      );
    });

    it("Upgrade moves the use modules that fit and names the one that does not", async () => {
      setFiles({
        "/workspace/go.work": "go 1.26.0\n\nuse (\n\t./a\n\t./b\n)\n",
        "/workspace/a/go.mod": PINNED("v1.25.0", [], "1.24.0"),
        "/workspace/b/go.mod": PINNED("v1.26.0", [], "1.26.0"),
      });
      scriptReleases({ "v1.91.0": "1.25.0", "v1.92.0": "1.26.0" });
      mockShowWarningMessage.mockResolvedValue("Upgrade");

      await buildCliCommand(["spec"], "/workspace").catch(() => undefined);
      await flush();

      expect(goGets()).toEqual([
        [path.resolve("/workspace", "b"), `${GOTEST_MODULE}@v1.92.0`],
      ]);
      expect(mockShowWarningMessage).toHaveBeenLastCalledWith(
        expect.stringContaining("but a/go.mod declares go 1.24.0."),
      );
      expect(mockShowInformationMessage).toHaveBeenCalledWith(
        "gotest: upgraded to v1.92.0 and declared as a tool.",
      );
    });
  });

  describe("tool directive suggestion", () => {
    it("offers once per root when the pin is usable but no directive exists", async () => {
      setGoMod("/workspace", PINNED("v1.92.0"));

      await buildCliCommand(["spec"], "/workspace");
      await buildCliCommand(["discover"], "/workspace");
      await flush();

      expect(mockShowInformationMessage).toHaveBeenCalledTimes(1);
      expect(mockShowInformationMessage.mock.calls[0][0]).toMatch(
        /go get -tool/,
      );
    });

    it("adds the directive at the pinned version, never at latest", async () => {
      setGoMod("/workspace", PINNED("v1.92.0"));
      mockShowInformationMessage.mockResolvedValue("Add to go.mod");

      await buildCliCommand(["spec"], "/workspace");
      await flush();

      expect(mockExecFileAsync).toHaveBeenCalledWith(
        "/usr/local/go/bin/go",
        ["get", "-tool", `${GOTEST_MODULE}@v1.92.0`],
        expect.objectContaining({ cwd: "/workspace" }),
      );
    });

    it("stays silent in a vendored module", async () => {
      setGoMod("/workspace", PINNED("v1.92.0"));
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
      setGoMod("/workspace", PINNED("v1.92.0"));

      await buildCliCommand(["spec"], "/workspace");
      await flush();

      expect(mockShowInformationMessage).not.toHaveBeenCalled();
    });

    it("Don't ask again turns the setting off for the folder", async () => {
      setGoMod("/workspace", PINNED("v1.92.0"));
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
        PINNED("v1.92.0", [
          "replace github.com/mvrahden/go-test => ../go-test",
        ]),
      );

      await buildCliCommand(["spec"], "/workspace");
      await flush();

      expect(mockShowInformationMessage).not.toHaveBeenCalled();
    });
  });
});

describe("checkProducerVersion", () => {
  beforeEach(() => {
    mockConfigValues.clear();
  });

  it("accepts the floor, anything above it, a pseudo-version at the floor and a dev build", async () => {
    const { checkProducerVersion } = await import("./cli.js");
    for (const v of [
      "v1.34.0",
      "v1.34.1",
      "v2.0.0",
      "v1.34.0-0.20261011200000-3588ff09a1b2",
      "dev (source checkout)",
      "dev (replace directive)",
      "dev",
    ]) {
      expect(() => checkProducerVersion(v)).not.toThrow();
    }
  });

  it("refuses a version below the floor with the upgrade command", async () => {
    const { checkProducerVersion, CliUnavailableError } =
      await import("./cli.js");
    expect(() => checkProducerVersion("v1.33.0")).toThrow(CliUnavailableError);
    expect(() => checkProducerVersion("v1.29.0")).toThrow(
      "the CLI is v1.29.0, but >= v1.34.0 is required. Run: go get -tool github.com/mvrahden/go-test/cmd/gotest@v1.34.0",
    );
  });

  it("refuses a document without a version as one that predates the floor", async () => {
    const { checkProducerVersion, CliUnavailableError } =
      await import("./cli.js");
    expect(() => checkProducerVersion(undefined)).toThrow(CliUnavailableError);
    expect(() => checkProducerVersion(undefined)).toThrow(
      "the CLI wrote no version, so it predates v1.34.0",
    );
  });

  it("names the configured module path in the upgrade command", async () => {
    const { checkProducerVersion } = await import("./cli.js");
    mockConfigValues.set("modulePath", "example.com/fork/cmd/gotest");
    expect(() => checkProducerVersion("v1.0.0")).toThrow(
      "go get -tool example.com/fork/cmd/gotest@v1.34.0",
    );
  });
});

// fakeProxy answers like the module proxy for these releases, recording
// which versions were asked for their go line.
function fakeProxy(releases: Record<string, string> | Error) {
  const asked: string[] = [];
  const query: ReleaseQuery = {
    versions: async () => {
      if (releases instanceof Error) throw releases;
      return Object.keys(releases);
    },
    goLines: async (_module, versions) => {
      asked.push(...versions);
      return new Map(
        versions.map((v) => [v, (releases as Record<string, string>)[v]]),
      );
    },
  };
  return { query, asked };
}

const MOD = "github.com/mvrahden/go-test";

describe("planUpgrade", () => {
  const releases = {
    "v1.89.0": "1.23.0",
    "v1.90.0": "1.24.0",
    "v1.91.0": "1.25.0",
    "v1.91.1": "1.25.0",
    "v1.92.0": "1.26.0",
    "v1.93.0-rc.1": "1.24.0",
  };
  const at = (goLine: string) => [{ dir: "/m", goLine }];

  it("takes latest when its go line fits", async () => {
    const { query } = fakeProxy(releases);
    const plan = await planUpgrade(at("1.26.0"), MOD, "v1.90.0", query);
    expect(plan.upgrades).toEqual([{ dir: "/m", release: "v1.92.0" }]);
    expect(plan.blocked).toEqual([]);
  });

  it("takes the newest release in between when latest needs a newer Go", async () => {
    const { query } = fakeProxy(releases);
    const plan = await planUpgrade(at("1.25.0"), MOD, "v1.90.0", query);
    expect(plan.upgrades).toEqual([{ dir: "/m", release: "v1.91.1" }]);
  });

  it("takes the floor when only the floor fits", async () => {
    const { query } = fakeProxy(releases);
    const plan = await planUpgrade(at("1.24.0"), MOD, "v1.90.0", query);
    expect(plan.upgrades).toEqual([{ dir: "/m", release: "v1.90.0" }]);
  });

  it("blocks when even the floor needs a newer Go, naming the least it needs", async () => {
    const { query } = fakeProxy(releases);
    const plan = await planUpgrade(at("1.23.5"), MOD, "v1.90.0", query);
    expect(plan.upgrades).toEqual([]);
    expect(plan.blocked).toEqual([{ dir: "/m", goLine: "1.23.5" }]);
    expect(plan.needs).toEqual({ release: "v1.90.0", goLine: "1.24.0" });
  });

  it("finds a fitting release below a newer one with a lower go line", async () => {
    const { query } = fakeProxy({
      "v1.90.0": "1.24.0",
      "v1.91.0": "1.26.0",
      "v1.92.0": "1.25.0",
    });
    const plan = await planUpgrade(at("1.24.0"), MOD, "v1.90.0", query);
    expect(plan.upgrades).toEqual([{ dir: "/m", release: "v1.90.0" }]);
    expect(plan.needs).toEqual({ release: "v1.90.0", goLine: "1.24.0" });
  });

  it("asks only for releases at or above the floor, never pre-releases", async () => {
    const { query, asked } = fakeProxy(releases);
    await planUpgrade(at("1.26.0"), MOD, "v1.90.0", query);
    expect(asked.sort()).toEqual(["v1.90.0", "v1.91.0", "v1.91.1", "v1.92.0"]);
  });

  it("on tied go lines takes the newest release, to upgrade to and to name as needed", async () => {
    const tied = {
      "v1.90.0": "1.25.0",
      "v1.90.1": "1.25.0",
      "v1.91.0": "1.25.0",
    };
    const { query } = fakeProxy(tied);
    const fits = await planUpgrade(at("1.25.0"), MOD, "v1.90.0", query);
    expect(fits.upgrades).toEqual([{ dir: "/m", release: "v1.91.0" }]);
    const blocked = await planUpgrade(at("1.24.0"), MOD, "v1.90.0", query);
    expect(blocked.needs).toEqual({ release: "v1.91.0", goLine: "1.25.0" });
  });

  it("never takes a pre-release, however low its go line", async () => {
    const { query } = fakeProxy({
      "v1.90.0": "1.25.0",
      "v1.91.0-rc.1": "1.23.0",
      "v1.91.0-beta.1": "1.23.0",
      "v1.92.0": "1.25.0",
    });
    const plan = await planUpgrade(at("1.24.0"), MOD, "v1.90.0", query);
    expect(plan.upgrades).toEqual([]);
    expect(plan.needs).toEqual({ release: "v1.92.0", goLine: "1.25.0" });
  });

  it("skips a release whose go line could not be read", async () => {
    const { query } = fakeProxy({
      "v1.90.0": "1.24.0",
      "v1.91.0": "",
      "v1.92.0": "1.26.0",
    });
    const plan = await planUpgrade(at("1.25.0"), MOD, "v1.90.0", query);
    expect(plan.upgrades).toEqual([{ dir: "/m", release: "v1.90.0" }]);
  });

  it("takes a release at 1.N.0 for a module at go 1.N, which go get only spells out", async () => {
    const { query } = fakeProxy({
      "v1.90.0": "1.25.0",
      "v1.91.0": "1.26.0",
      "v1.92.0": "1.26.1",
    });
    const plan = await planUpgrade(at("1.26"), MOD, "v1.90.0", query);
    expect(plan.upgrades).toEqual([{ dir: "/m", release: "v1.91.0" }]);
  });

  it("fails when the releases cannot be listed", async () => {
    const { query } = fakeProxy(new Error("no network"));
    await expect(
      planUpgrade(at("1.26.0"), MOD, "v1.90.0", query),
    ).rejects.toThrow("no network");
  });

  it("fails when no release at or above the floor is published", async () => {
    const { query } = fakeProxy(releases);
    await expect(
      planUpgrade(at("1.26.0"), MOD, "v1.94.0", query),
    ).rejects.toThrow(
      "no release of github.com/mvrahden/go-test at or above v1.94.0 is published",
    );
  });

  it("plans each module against its own go line", async () => {
    const { query } = fakeProxy(releases);
    const plan = await planUpgrade(
      [
        { dir: "/a", goLine: "1.26.0" },
        { dir: "/b", goLine: "1.24.2" },
        { dir: "/c", goLine: "1.23.0" },
      ],
      MOD,
      "v1.90.0",
      query,
    );
    expect(plan.upgrades).toEqual([
      { dir: "/a", release: "v1.92.0" },
      { dir: "/b", release: "v1.90.0" },
    ]);
    expect(plan.blocked).toEqual([{ dir: "/c", goLine: "1.23.0" }]);
  });
});

describe("compareGoVersions", () => {
  it("orders go lines as Go does", () => {
    const ordered = [
      "",
      "1.16",
      "1.25",
      "1.25alpha1",
      "1.25beta1",
      "1.25beta2",
      "1.25rc1",
      "1.25rc2",
      "1.25.0",
      "1.25.1",
      "1.26.0",
      "2.0",
    ];
    for (let i = 0; i < ordered.length - 1; i++) {
      expect(compareGoVersions(ordered[i], ordered[i + 1])).toBeLessThan(0);
      expect(compareGoVersions(ordered[i + 1], ordered[i])).toBeGreaterThan(0);
    }
    expect(compareGoVersions("1.25.0", "1.25.0")).toBe(0);
  });
});

describe("goLineFits", () => {
  it("takes a release at or below the module's go line", () => {
    expect(goLineFits("1.25.0", "1.26.0")).toBe(true);
    expect(goLineFits("1.26.0", "1.26.0")).toBe(true);
    expect(goLineFits("1.26.1", "1.26.0")).toBe(false);
    expect(goLineFits("1.27.0", "1.26.5")).toBe(false);
  });

  it("takes 1.N.0 for a module at 1.N or a 1.N pre-release, nothing newer", () => {
    expect(goLineFits("1.26.0", "1.26")).toBe(true);
    expect(goLineFits("1.26.0", "1.26rc2")).toBe(true);
    expect(goLineFits("1.26.1", "1.26")).toBe(false);
    expect(goLineFits("1.27.0", "1.26")).toBe(false);
    expect(goLineFits("1.26.0", "1.25")).toBe(false);
    expect(goLineFits("1.26.0", "")).toBe(false);
  });
});

describe("lookupGoFlags", () => {
  it("drops -mod and -modfile in every spelling and always sets -mod=mod", () => {
    expect(lookupGoFlags("")).toBe("-mod=mod");
    expect(
      lookupGoFlags(
        "-modfile=/x/alt.mod  --mod=vendor -mod=readonly --modfile=y -modcacherw -tags=a,b",
      ),
    ).toBe("-modcacherw -tags=a,b -mod=mod");
  });
});

describe("goReleaseQuery", () => {
  const exec = (answer: (args: string[]) => string | Promise<string>): void => {
    mockExecFileAsync.mockImplementation(async (_file, args) => ({
      stdout: await answer(args),
      stderr: "",
    }));
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
  });

  it("lists exactly the published versions, none of the surrounding fields", async () => {
    exec((args) =>
      listJSON(args, { "v1.90.0": "1.25.0", "v1.91.0": "1.26.0" }),
    );
    const q = goReleaseQuery("go", "/d", new AbortController().signal);
    expect(await q.versions(MOD)).toEqual(["v1.90.0", "v1.91.0"]);
  });

  it("returns the go line of every release that resolves and leaves out the rest", async () => {
    exec((args) =>
      listJSON(args, {
        "v1.90.0": "1.25.0",
        "v1.91.0": new Error("410 Gone"),
        "v1.92.0": "1.26.0",
      }),
    );
    const q = goReleaseQuery("go", "/d", new AbortController().signal);
    const lines = await q.goLines(MOD, ["v1.90.0", "v1.91.0", "v1.92.0"]);
    expect([...lines]).toEqual([
      ["v1.90.0", "1.25.0"],
      ["v1.92.0", "1.26.0"],
    ]);
    const list = mockExecFileAsync.mock.calls.find((c) => c[1][0] === "list");
    expect(list?.[1]).toEqual([
      "list",
      "-m",
      "-json",
      "-e",
      `${MOD}@v1.90.0`,
      `${MOD}@v1.91.0`,
      `${MOD}@v1.92.0`,
    ]);
  });

  it("asks go env for GOFLAGS once, so the go env file's count too", async () => {
    exec((args) => listJSON(args, { "v1.90.0": "1.25.0" }, "-modfile=x"));
    const q = goReleaseQuery("go", "/d", new AbortController().signal);
    await q.versions(MOD);
    await q.goLines(MOD, ["v1.90.0"]);
    const envs = mockExecFileAsync.mock.calls.filter((c) => c[1][0] === "env");
    expect(envs).toHaveLength(1);
  });

  describe("timeout", () => {
    beforeEach(() => vi.useFakeTimers());

    it("stops a go command still running after 60s and fails the lookup", async () => {
      hangLookups();
      const q = goReleaseQuery("go", "/d", new AbortController().signal);
      const versions = q.versions(MOD);
      const settled = vi.fn();
      versions.then(settled, settled);

      await vi.advanceTimersByTimeAsync(59_999);
      expect(settled).not.toHaveBeenCalled();
      await vi.advanceTimersByTimeAsync(1);

      await expect(versions).rejects.toThrow("go env timed out after 60s");
      const [, , opts] = mockExecFileAsync.mock.calls[0];
      expect((opts as ExecOpts).signal?.aborted).toBe(true);
      vi.useRealTimers();
    });

    it("gives each go command its own 60s", async () => {
      mockExecFileAsync.mockImplementation(async (_file, args, opts) => {
        if (args[0] === "env") {
          await new Promise((r) => setTimeout(r, 50_000));
          return { stdout: "\n", stderr: "" };
        }
        const { signal } = opts as ExecOpts;
        return new Promise((_resolve, reject) =>
          signal?.addEventListener("abort", () => reject(new Error("aborted"))),
        );
      });
      const q = goReleaseQuery("go", "/d", new AbortController().signal);
      const versions = q.versions(MOD);
      const settled = vi.fn();
      versions.then(settled, settled);

      await vi.advanceTimersByTimeAsync(50_000 + 59_999);
      expect(settled).not.toHaveBeenCalled();
      await vi.advanceTimersByTimeAsync(1);

      await expect(versions).rejects.toThrow("go list timed out after 60s");
      vi.useRealTimers();
    });
  });
});
