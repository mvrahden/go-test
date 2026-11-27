import { describe, it, expect } from "vitest";
import {
  buildRunsTab,
  cardsOf,
  RUNS_EMPTY_TEXT,
  RUNS_KEPT,
  type RunsView,
} from "./runsView.js";
import type { RunsEntry } from "./runsModel.js";

const now = new Date(2026, 10, 26, 9, 0);

function entry(
  id: string,
  over: Partial<RunsEntry> = {},
  at = new Date(2026, 10, 26, 8, 38),
): RunsEntry {
  return {
    id,
    recorded: at.toISOString(),
    eventsFile: `/e/${id}.jsonl`,
    manifest: {
      args: ["--control=stdin", "--", "-json", "./..."],
      commit: "e50c1f36",
    },
    result: {
      exitCode: 1,
      packages: { passed: 75, failed: 1, skipped: 19 },
      tests: { passed: 11672, failed: 4, skipped: 18 },
    },
    failed: [],
    packages: { total: 95, current: 95, stale: 0, unknown: 0 },
    ...over,
  };
}

const agentRun = (id: string, exitCode: number, minute: number) =>
  entry(
    id,
    {
      eventsFile: undefined,
      manifest: {
        args: [
          "--agent=false",
          "./pkg/gotestruntime/",
          "-run",
          "TestControlLinkTestSuite",
        ],
        commit: "e50c1f36",
        agent: "claude-code_2_agent",
      },
      result: {
        exitCode,
        packages: {
          passed: exitCode ? 0 : 1,
          failed: exitCode ? 1 : 0,
          skipped: 0,
        },
      },
      packages: { total: 1, current: 1, stale: 0, unknown: 0 },
    },
    new Date(2026, 10, 26, 8, minute),
  );

const view = (runs: RunsEntry[], over: Partial<RunsView> = {}): RunsView => ({
  state: { kind: "doc", doc: { version: "v1", dir: "/w", runs } },
  busy: false,
  folders: [{ path: "/w", label: "w", selected: true }],
  head: "e50c1f36",
  now,
  exists: () => true,
  chosen: () => false,
  ...over,
});

describe("the Runs tab", () => {
  it("shows the head commit's records as cards: time, title, who, counts, verdict, freshness, command", () => {
    const t = buildRunsTab(view([entry("r1")]));
    expect(t.headline).toBe("One run covers this whole tree, and it failed");
    expect(t.sub).toBe(
      "Records for e50c1f36. A run counts for this tree while the files it read are unchanged.",
    );
    expect(t.cards).toEqual([
      {
        key: "r1",
        time: "08:38",
        day: "today",
        title: "Everything",
        who: "You, from this editor",
        counts: "11,672 passed · 4 failed · 18 skipped",
        verdict: "Failed",
        tone: "fail",
        fresh: "95 packages, current",
        freshTone: "pass",
        canOpen: true,
        evidence: "none",
        command: "gotest --control=stdin -- -json ./...",
        commit: "e50c1f36",
      },
    ]);
    expect(t.footer).toBe(RUNS_KEPT);
  });

  it("folds consecutive runs of one command, commit and source into one card", () => {
    const runs = [
      agentRun("a6", 1, 47),
      agentRun("a5", 0, 46),
      agentRun("a4", 0, 45),
      agentRun("a3", 1, 44),
      entry("editor"),
      agentRun("a1", 0, 30),
    ];
    const cards = cardsOf(view(runs), runs);
    expect(cards.map((c) => [c.title, c.verdict, c.runs?.length])).toEqual([
      [
        "pkg/gotestruntime, only ControlLink, 4 runs in a row",
        "2 passed · 2 failed",
        4,
      ],
      ["Everything", "Failed", undefined],
      ["pkg/gotestruntime, only ControlLink", "Passed", undefined],
    ]);
    expect(cards[0]).toMatchObject({
      who: "Agent (Claude Code)",
      counts: "no test counts kept",
      canOpen: false,
      time: "08:47",
      tone: "fail",
    });
    expect(cards[0].runs!.map((r) => r.key)).toEqual(["a6", "a5", "a4", "a3"]);
  });

  it("marks the run chosen as evidence, else the newest current one in use", () => {
    expect(
      buildRunsTab(view([entry("r1")], { chosen: (id) => id === "r1" }))
        .cards[0].evidence,
    ).toBe("chosen");
    expect(
      buildRunsTab(view([entry("r1")], { auto: "r1" })).cards[0].evidence,
    ).toBe("auto");
  });

  it("puts the records of other commits under Earlier commits, naming the commit", () => {
    const old = entry("old", {
      manifest: { args: ["./..."], commit: "0a1b2c3d", dirty: true },
      packages: { total: 2, current: 1, stale: 1, unknown: 0 },
    });
    const t = buildRunsTab(view([entry("r1"), old]));
    expect(t.cards.map((c) => c.key)).toEqual(["r1"]);
    expect(
      t.earlier.map((c) => [c.key, c.commit, c.fresh, c.freshTone]),
    ).toEqual([["old", "0a1b2c3d dirty", "1 of 2 packages stale", "warn"]]);
    expect(buildRunsTab(view([old])).headline).toBe(
      "No run of this commit is recorded yet",
    );
  });

  it("says when no run matches the tree any more, and how many do", () => {
    const stale = entry("s", {
      packages: { total: 2, current: 1, stale: 1, unknown: 0 },
    });
    expect(buildRunsTab(view([stale])).headline).toBe(
      "No run matches this tree any more",
    );
    const narrow = agentRun("n", 0, 10);
    expect(buildRunsTab(view([narrow, agentRun("m", 1, 5)])).headline).toBe(
      "2 runs match this tree; the newest passed",
    );
  });

  it("explains an empty list, offering to import a CI run", () => {
    const t = buildRunsTab(view([]));
    expect(t.empty).toEqual({
      title: "No run recorded yet",
      text: RUNS_EMPTY_TEXT,
      action: { act: "import", label: "Import a CI run" },
    });
  });

  it("names the release a CLI without runs needs, and shows an error as it came", () => {
    const t = buildRunsTab(
      view([], {
        state: {
          kind: "oldCli",
          version: "v1.41.0",
          floor: "v1.42.0",
          advice: "Run: go get -tool x@v1.42.0",
          canUpgrade: true,
        },
      }),
    );
    expect(t.empty!.title).toBe("The Runs tab needs gotest v1.42.0");
    expect(t.empty!.text).toContain("This folder's CLI is v1.41.0.");
    expect(t.empty!.action).toEqual({ act: "upgrade", label: "Upgrade…" });
    expect(
      buildRunsTab(view([], { state: { kind: "error", message: "boom" } }))
        .error,
    ).toBe("boom");
    expect(
      buildRunsTab(view([], { state: { kind: "idle" }, busy: true })).empty!
        .text,
    ).toBe("Reading the run records…");
  });

  it("offers the folders when there are several", () => {
    expect(buildRunsTab(view([])).folders).toBeUndefined();
    const two = [
      { path: "/a", label: "a", selected: true },
      { path: "/b", label: "b", selected: false },
    ];
    expect(buildRunsTab(view([], { folders: two })).folders).toEqual(two);
  });
});
