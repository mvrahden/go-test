import { describe, it, expect, vi } from "vitest";

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
import { mkdtemp, mkdir, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import {
  parseFuzzProgress,
  parseNewCrasher,
  parsePromotedSeed,
  isValidGoDuration,
  countCrasherEntries,
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

describe("countCrasherEntries", () => {
  it("counts files in the wrapper's corpus dir and 0 when absent", async () => {
    const dir = await mkdtemp(path.join(tmpdir(), "gotest-fuzz-"));
    const corpus = path.join(dir, "testdata", "fuzz", "FuzzMySuite_FuzzParse");
    await mkdir(corpus, { recursive: true });
    await writeFile(path.join(corpus, "aa"), "go test fuzz v1\n");
    await writeFile(path.join(corpus, "bb"), "go test fuzz v1\n");

    expect(await countCrasherEntries(dir, "FuzzMySuite_FuzzParse")).toBe(2);
    expect(await countCrasherEntries(dir, "FuzzMySuite_FuzzOther")).toBe(0);
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
