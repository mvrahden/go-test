import { describe, it, expect, beforeEach } from "vitest";
import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";
import { ensureGitExclude } from "./gitExclude.js";

function makeFakeOutputChannel() {
  return {
    debug: () => {},
    info: () => {},
    warn: () => {},
    error: () => {},
  } as any;
}

describe("ensureGitExclude", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "git-exclude-test-"));
  });

  it("appends patterns to an existing exclude file", () => {
    const infoDir = path.join(tmpDir, ".git", "info");
    fs.mkdirSync(infoDir, { recursive: true });
    fs.writeFileSync(path.join(infoDir, "exclude"), "# existing\n*.swp\n");

    ensureGitExclude(tmpDir, makeFakeOutputChannel());

    const content = fs.readFileSync(path.join(infoDir, "exclude"), "utf-8");
    expect(content).toContain("gotest_psuite_test.go");
    expect(content).toContain("gotest_pxsuite_test.go");
    expect(content).toContain("# gotest generated files");
    expect(content).toContain("*.swp");
  });

  it("is idempotent — does not duplicate on second call", () => {
    const infoDir = path.join(tmpDir, ".git", "info");
    fs.mkdirSync(infoDir, { recursive: true });
    fs.writeFileSync(path.join(infoDir, "exclude"), "");

    const ch = makeFakeOutputChannel();
    ensureGitExclude(tmpDir, ch);
    ensureGitExclude(tmpDir, ch);

    const content = fs.readFileSync(path.join(infoDir, "exclude"), "utf-8");
    const count = content.split("gotest_psuite_test.go").length - 1;
    expect(count).toBe(1);
  });

  it("does nothing when .git directory does not exist", () => {
    ensureGitExclude(tmpDir, makeFakeOutputChannel());
    expect(fs.existsSync(path.join(tmpDir, ".git", "info", "exclude"))).toBe(
      false,
    );
  });

  it("creates .git/info/exclude when .git exists but file does not", () => {
    fs.mkdirSync(path.join(tmpDir, ".git"), { recursive: true });

    ensureGitExclude(tmpDir, makeFakeOutputChannel());

    const content = fs.readFileSync(
      path.join(tmpDir, ".git", "info", "exclude"),
      "utf-8",
    );
    expect(content).toContain("gotest_psuite_test.go");
    expect(content).toContain("gotest_pxsuite_test.go");
  });

  it("does nothing when patterns already present", () => {
    const infoDir = path.join(tmpDir, ".git", "info");
    fs.mkdirSync(infoDir, { recursive: true });
    const original = "# manual\ngotest_psuite_test.go\n";
    fs.writeFileSync(path.join(infoDir, "exclude"), original);

    ensureGitExclude(tmpDir, makeFakeOutputChannel());

    const content = fs.readFileSync(path.join(infoDir, "exclude"), "utf-8");
    expect(content).toBe(original);
  });
});
