import { describe, it, expect, afterEach } from "vitest";
import { spawn } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

const script = path.resolve(
  __dirname,
  "..",
  "scripts",
  "record-cli-contract.mjs",
);

// The recorder's build dir, under a go on PATH that runs goScript.
describe.skipIf(process.platform === "win32")(
  "record-cli-contract build dir",
  () => {
    let dir = "";

    afterEach(() => {
      if (dir) rmSync(dir, { recursive: true, force: true });
    });

    function start(goScript: string) {
      dir = mkdtempSync(path.join(tmpdir(), "gotest-recorder-test-"));
      const bin = path.join(dir, "bin");
      const tmp = path.join(dir, "tmp");
      for (const d of [bin, tmp]) mkdirSync(d);
      writeFileSync(path.join(bin, "go"), "#!/bin/sh\n" + goScript, {
        mode: 0o700,
      });
      const child = spawn(process.execPath, [script], {
        env: {
          ...process.env,
          PATH: bin + path.delimiter + process.env.PATH,
          TMPDIR: tmp,
          REC: dir,
        },
        stdio: "ignore",
      });
      const ended = new Promise<{
        code: number | null;
        signal: NodeJS.Signals | null;
      }>((resolve) =>
        child.on("exit", (code, signal) => resolve({ code, signal })),
      );
      return { child, ended, tmp };
    }

    it("is removed when the build fails", async () => {
      const { ended, tmp } = start("exit 1\n");
      const { code } = await ended;
      expect(code).not.toBe(0);
      expect(readdirSync(tmp)).toEqual([]);
    });

    it("is removed on SIGINT, which still ends the script", async () => {
      const { child, ended, tmp } = start(
        'touch "$REC/started"\nexec sleep 30\n',
      );
      await expect
        .poll(() => existsSync(path.join(dir, "started")), { timeout: 10_000 })
        .toBe(true);
      expect(readdirSync(tmp)).not.toEqual([]);
      child.kill("SIGINT");
      const { signal } = await ended;
      expect(signal).toBe("SIGINT");
      expect(readdirSync(tmp)).toEqual([]);
    });
  },
);
