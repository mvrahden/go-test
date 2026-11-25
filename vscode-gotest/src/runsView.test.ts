import { describe, it, expect } from "vitest";
import { buildRunsBody, RUNS_EMPTY_TEXT, type RunsView } from "./runsView.js";
import type { RunRow } from "./runsModel.js";

function row(over: Partial<RunRow> = {}): RunRow {
  return {
    id: "r1",
    when: "14:02",
    whenTitle: "2026-11-25T13:02:00.000Z",
    pass: true,
    verdict: "PASS exit 0",
    who: "no agent",
    commit: "abc1234",
    args: "gotest -json ./...",
    counts: "3 passed",
    judges: "judges this tree: 2 current / 1 stale / 0 unknown",
    judgesTone: "warn",
    eventsFile: "/s/a.jsonl",
    canOpen: true,
    evidence: false,
    ...over,
  };
}

const view = (over: Partial<RunsView> = {}): RunsView => ({
  state: { kind: "doc", rows: [row()] },
  busy: false,
  folders: [{ path: "/w", label: "w", selected: true }],
  ...over,
});

describe("buildRunsBody", () => {
  it("lists a run with its facts, its judgement and its actions", () => {
    const html = buildRunsBody(view());
    expect(html).toContain('<span class="tag-chip pass">PASS exit 0</span>');
    expect(html).toContain("gotest -json ./...");
    expect(html).toContain(
      '<div class="run-judges warn">judges this tree: 2 current / 1 stale / 0 unknown</div>',
    );
    expect(html).toContain('data-runs="open" data-key="r1"');
    expect(html).toContain(">Use as evidence</button>");
    expect(html).toContain('data-runs="import"');
    expect(html).toContain('data-runs="runChanged"');
  });

  it("marks a narrowed and an imported run, and a chosen one as evidence", () => {
    const html = buildRunsBody(
      view({
        state: {
          kind: "doc",
          rows: [
            row({
              narrowed: "--changed: ran 1 package, left out 3 unchanged",
              imported: "ci.json",
              evidence: true,
            }),
          ],
        },
      }),
    );
    expect(html).toContain(">narrowed</span>");
    expect(html).toContain("--changed: ran 1 package, left out 3 unchanged");
    expect(html).toContain(">imported</span>");
    expect(html).toContain('<div class="run-row chosen"');
    expect(html).toContain('aria-pressed="true"');
  });

  it("offers nothing to open for a run without its events file", () => {
    const html = buildRunsBody(
      view({ state: { kind: "doc", rows: [row({ canOpen: false })] } }),
    );
    expect(html).not.toContain('data-runs="open"');
    expect(html).toContain("no events file");
  });

  it("explains an empty list: runs are recorded from the CLI and the editor alike", () => {
    const html = buildRunsBody(view({ state: { kind: "doc", rows: [] } }));
    expect(html).toContain("No run recorded yet");
    expect(html).toContain(RUNS_EMPTY_TEXT.slice(0, 60));
  });

  it("names the release a CLI without runs needs", () => {
    const html = buildRunsBody(
      view({
        state: {
          kind: "oldCli",
          version: "v1.41.0",
          floor: "v1.42.0",
          advice: "Run: go get -tool x@v1.42.0",
          canUpgrade: true,
        },
      }),
    );
    expect(html).toContain("The Runs scope needs gotest v1.42.0");
    expect(html).toContain("This folder's CLI is v1.41.0.");
    expect(html).toContain('data-runs="upgrade"');
    expect(html).toMatch(/data-runs="runChanged"[^>]*disabled/);
  });
});
