import { describe, it, expect, vi, afterEach, afterAll } from "vitest";

vi.mock("vscode", () => {
  class TestTag {
    constructor(public readonly id: string) {}
  }
  class Position {
    constructor(
      public readonly line: number,
      public readonly character: number,
    ) {}
  }
  class Uri {
    static file(path: string) {
      return { fsPath: path, toString: () => path };
    }
  }
  class TestMessage {
    constructor(public readonly message: string) {}
  }
  class Location {
    constructor(
      public readonly uri: unknown,
      public readonly range: unknown,
    ) {}
  }
  return { TestTag, Position, Uri, TestMessage, Location };
});

import * as path from "node:path";
import { createHash } from "node:crypto";
import { existsSync } from "node:fs";
import { mkdtemp, mkdir, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import {
  parseFuzzProgress,
  parseNewCrasher,
  parsePromotedSeed,
  isValidGoDuration,
  fuzzSessionFile,
  sessionCrasherCounts,
} from "./fuzz.js";
import { resolveTestItem } from "./runnerUtils.js";

describe("parseFuzzProgress", () => {
  it("reads go's status line through the orchestrator prefix", () => {
    const p = parseFuzzProgress(
      "[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] fuzz: elapsed: 3s, execs: 393973 (131314/sec), new interesting: 2 (total: 78)",
    );
    expect(p).toEqual({
      elapsed: "3s",
      execs: 393973,
      rate: 131314,
      interesting: 2,
    });
  });

  it("tolerates lines without the interesting counter", () => {
    const p = parseFuzzProgress(
      "[FuzzX] fuzz: elapsed: 10s, execs: 1407280 (131111/sec)",
    );
    expect(p).toEqual({
      elapsed: "10s",
      execs: 1407280,
      rate: 131111,
      interesting: undefined,
    });
  });

  it("ignores unrelated lines", () => {
    expect(parseFuzzProgress("[FuzzX] PASS")).toBeUndefined();
    expect(
      parseFuzzProgress("[FuzzX] gathering baseline coverage: 0/76 completed"),
    ).toBeUndefined();
  });
});

describe("parseNewCrasher", () => {
  it("extracts the corpus entry path", () => {
    expect(
      parseNewCrasher(
        "[FuzzX] new crasher: /abs/pkg/testdata/fuzz/FuzzX/582528ddfad69eb5",
      ),
    ).toBe("/abs/pkg/testdata/fuzz/FuzzX/582528ddfad69eb5");
  });

  it("ignores the follow-up hint line", () => {
    expect(
      parseNewCrasher(
        "[FuzzX] inspect it with `gotest fuzz triage`, then `gotest fuzz promote` to keep it as a typed seed",
      ),
    ).toBeUndefined();
  });
});

describe("parsePromotedSeed", () => {
  it("reads promote's confirmation with a typed literal", () => {
    expect(
      parsePromotedSeed(
        "promoted FuzzFrameCodecTestSuite_FuzzFrameRoundTrip/582528dd -> f.Add(Frame{Version: 48, Kind: Kind(0)}) in examples/fuzzing/suite_test.go:83",
      ),
    ).toEqual({ file: "examples/fuzzing/suite_test.go", line: 83 });
  });

  it("ignores skip and warning lines", () => {
    expect(
      parsePromotedSeed("promote: FuzzX/1a2b: skipped: could not resolve"),
    ).toBeUndefined();
  });
});

describe("isValidGoDuration", () => {
  it("accepts compound Go durations", () => {
    for (const ok of ["30s", "5m", "2m30s", "1h", "1.5h", "90s", "100ms"]) {
      expect(isValidGoDuration(ok), ok).toBe(true);
    }
  });

  it("rejects everything else", () => {
    for (const bad of ["", "5", "5 m", "m5", "five minutes", "-5m"]) {
      expect(isValidGoDuration(bad), bad).toBe(false);
    }
  });
});

describe("fuzzSessionFile", () => {
  it("lives in $GOTEST_CACHE_DIR, resolved against the CLI's cwd", () => {
    expect(
      fuzzSessionFile({ GOTEST_CACHE_DIR: "/c", HOME: "/h" }, "linux"),
    ).toBe(path.join("/c", "fuzz-session.json"));
    expect(fuzzSessionFile({ GOTEST_CACHE_DIR: "rel" }, "linux", "/ws")).toBe(
      path.resolve("/ws", "rel", "fuzz-session.json"),
    );
  });

  it("otherwise lives where Go's os.UserCacheDir points, under gotest", () => {
    expect(fuzzSessionFile({ HOME: "/h" }, "linux")).toBe(
      path.join("/h/.cache", "gotest", "fuzz-session.json"),
    );
    expect(fuzzSessionFile({ XDG_CACHE_HOME: "/x", HOME: "/h" }, "linux")).toBe(
      path.join("/x", "gotest", "fuzz-session.json"),
    );
    expect(fuzzSessionFile({ HOME: "/h" }, "darwin")).toBe(
      path.join("/h/Library/Caches", "gotest", "fuzz-session.json"),
    );
    expect(fuzzSessionFile({ LocalAppData: "L" }, "win32")).toBe(
      path.join("L", "gotest", "fuzz-session.json"),
    );
  });

  it("is unknown where Go's cache dir is undefined", () => {
    expect(
      fuzzSessionFile({ XDG_CACHE_HOME: "rel", HOME: "/h" }, "linux"),
    ).toBe(undefined);
    expect(fuzzSessionFile({}, "linux")).toBe(undefined);
    expect(fuzzSessionFile({ HOME: "/h" }, "win32")).toBe(undefined);
  });
});

describe("sessionCrasherCounts", () => {
  const roots: string[] = [];

  afterEach(async () => {
    for (const root of roots) await rm(root, { recursive: true, force: true });
  });

  // Every dir a test made is gone once they ran.
  afterAll(() => {
    expect(roots).not.toEqual([]);
    for (const root of roots) expect(existsSync(root)).toBe(false);
  });

  async function setup() {
    const root = await mkdtemp(path.join(tmpdir(), "gotest-fuzz-"));
    roots.push(root);
    const pkgDir = path.join(root, "pkg");
    const cacheDir = path.join(root, "cache");
    await mkdir(cacheDir);
    const write = async (fn: string, name: string, input: string) => {
      const dir = path.join(pkgDir, "testdata", "fuzz", fn);
      await mkdir(dir, { recursive: true });
      const content = `go test fuzz v1\nstring(${JSON.stringify(input)})\n`;
      await writeFile(path.join(dir, name), content);
      return createHash("sha256").update(content).digest("hex");
    };
    const record = (crashers: object[]) =>
      writeFile(
        path.join(cacheDir, "fuzz-session.json"),
        JSON.stringify({ crashers }),
      );
    return {
      root,
      pkgDir,
      write,
      record,
      env: { GOTEST_CACHE_DIR: cacheDir },
    };
  }

  it("counts recorded crashers per wrapper, matching the package dir through a symlink", async () => {
    const { root, pkgDir, write, record, env } = await setup();
    const a = await write("FuzzS_A", "a1", "x");
    const b = await write("FuzzS_B", "b1", "y");
    await write("FuzzS_A", "older", "z");
    const link = path.join(root, "link");
    await symlink(pkgDir, link);
    await record([
      { dir: link, func: "FuzzS_A", name: "a1", sha256: a },
      { dir: pkgDir, func: "FuzzS_B", name: "b1", sha256: b },
      { dir: pkgDir, func: "FuzzS_B", name: "b1", sha256: b },
    ]);

    const counts = await sessionCrasherCounts(
      pkgDir,
      new Set(["FuzzS_A", "FuzzS_B"]),
      { env },
    );

    expect(Object.fromEntries(counts)).toEqual({ FuzzS_A: 1, FuzzS_B: 1 });
  });

  it("ignores records of other packages and of wrappers the package lacks", async () => {
    const { root, pkgDir, write, record, env } = await setup();
    const a = await write("FuzzS_A", "a1", "x");
    await record([
      {
        dir: path.join(root, "elsewhere"),
        func: "FuzzS_A",
        name: "a1",
        sha256: a,
      },
      { dir: pkgDir, func: "FuzzS_Gone", name: "a1", sha256: a },
    ]);

    const counts = await sessionCrasherCounts(pkgDir, new Set(["FuzzS_A"]), {
      env,
    });

    expect(counts.size).toBe(0);
  });

  it("counts nothing when the record is missing or unreadable", async () => {
    const { pkgDir, write, env } = await setup();
    await write("FuzzS_A", "a1", "x");

    expect(
      (await sessionCrasherCounts(pkgDir, new Set(["FuzzS_A"]), { env })).size,
    ).toBe(0);
    await writeFile(
      path.join(env.GOTEST_CACHE_DIR, "fuzz-session.json"),
      "not json",
    );
    expect(
      (await sessionCrasherCounts(pkgDir, new Set(["FuzzS_A"]), { env })).size,
    ).toBe(0);
  });
});

describe("resolveTestItem fuzz wrappers", () => {
  function makeController(existingIds: string[]) {
    const dynamicCalls: { parentId: string; path: string; label: string }[] =
      [];
    const items = new Map(existingIds.map((id) => [id, { id }]));
    return {
      controller: {
        findItem: (id: string) => items.get(id),
        createDynamicSubtest: (
          parent: { id: string },
          subtestPath: string,
          label: string,
        ) => {
          dynamicCalls.push({ parentId: parent.id, path: subtestPath, label });
          const child = { id: `${parent.id}/dynamic/${subtestPath}` };
          items.set(child.id, child);
          return child;
        },
      },
      dynamicCalls,
    };
  }

  it("maps Fuzz<Suite>_<Method> events to the fuzz item", () => {
    const { controller } = makeController([
      "example.com/pkg/MySuite/FuzzParse",
    ]);
    const item = resolveTestItem(
      controller as never,
      "FuzzMySuite_FuzzParse",
      "example.com/pkg",
    );
    expect(item?.id).toBe("example.com/pkg/MySuite/FuzzParse");
  });

  it("resolves underscored suite names by trying each _Fuzz boundary", () => {
    const { controller } = makeController([
      "example.com/pkg/My_Suite/FuzzParse",
    ]);
    const item = resolveTestItem(
      controller as never,
      "FuzzMy_Suite_FuzzParse",
      "example.com/pkg",
    );
    expect(item?.id).toBe("example.com/pkg/My_Suite/FuzzParse");
  });

  it("attaches seed subtests as dynamic children of the fuzz item", () => {
    const { controller, dynamicCalls } = makeController([
      "example.com/pkg/MySuite/FuzzParse",
    ]);
    const item = resolveTestItem(
      controller as never,
      "FuzzMySuite_FuzzParse/seed#0",
      "example.com/pkg",
    );
    expect(dynamicCalls).toEqual([
      {
        parentId: "example.com/pkg/MySuite/FuzzParse",
        path: "seed#0",
        label: "seed#0",
      },
    ]);
    expect(item?.id).toBe("example.com/pkg/MySuite/FuzzParse/dynamic/seed#0");
  });

  it("falls back to plain suite resolution for suites named Fuzz*", () => {
    const { controller } = makeController(["example.com/pkg/FuzzySuite"]);
    const item = resolveTestItem(
      controller as never,
      "FuzzySuite",
      "example.com/pkg",
    );
    expect(item?.id).toBe("example.com/pkg/FuzzySuite");
  });
});
