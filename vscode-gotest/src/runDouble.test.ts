import { describe, it, expect } from "vitest";
import {
  FakeTestItem,
  FakeTestRun,
} from "../test/integration/vscodeTestApi.js";

describe("the run double", () => {
  const item = new FakeTestItem("p/S/T", "T");
  it("keeps failed when passed is reported after it", () => {
    const run = new FakeTestRun();
    run.failed(item, "boom");
    run.passed(item);
    expect(run.verdicts.get(item.id)).toBe("failed");
  });
  it("keeps a terminal verdict when started or enqueued follows", () => {
    const run = new FakeTestRun();
    run.passed(item);
    run.started(item);
    run.enqueued(item);
    expect(run.verdicts.get(item.id)).toBe("passed");
  });
  it("lets a higher terminal verdict replace a lower one", () => {
    const run = new FakeTestRun();
    run.passed(item);
    run.skipped(item);
    run.failed(item, "a");
    run.errored(item, "b");
    expect(run.verdicts.get(item.id)).toBe("errored");
    expect(run.messages.get(item.id)).toEqual(["a", "b"]);
  });
  it("still moves from started to any verdict", () => {
    const run = new FakeTestRun();
    run.started(item);
    run.passed(item);
    expect(run.verdicts.get(item.id)).toBe("passed");
  });
});
