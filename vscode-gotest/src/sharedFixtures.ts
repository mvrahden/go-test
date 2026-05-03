import { spawn } from "node:child_process";
import type * as vscode from "vscode";
import type { CliCommand } from "./cli.js";
import type { SharedFixtureInfo } from "./types.js";

export interface SharedSetupProcess {
  stateFile: string;
  dispose(): void;
}

export function startSharedSetup(
  cmd: CliCommand,
  cwd: string,
  fixtures: SharedFixtureInfo[],
  outputChannel: vscode.OutputChannel,
): Promise<SharedSetupProcess> {
  return new Promise<SharedSetupProcess>((resolve, reject) => {
    const child = spawn(cmd.bin, cmd.args, { cwd });
    let stdout = "";
    let settled = false;

    child.stdout.on("data", (data: Buffer) => {
      stdout += data.toString();
      if (!settled && stdout.includes("\n")) {
        settled = true;
        try {
          const output = JSON.parse(stdout.split("\n")[0]);
          resolve({
            stateFile: output.stateFile,
            dispose() {
              child.kill("SIGTERM");
            },
          });
        } catch {
          child.kill("SIGTERM");
          reject(
            new Error(`Failed to parse shared-setup output: ${stdout.trim()}`),
          );
        }
      }
    });

    child.stderr.on("data", (data: Buffer) => {
      outputChannel.appendLine(`[shared-setup] ${data.toString().trimEnd()}`);
    });

    child.on("error", (err: Error) => {
      if (!settled) {
        settled = true;
        reject(err);
      }
    });

    child.on("close", (code) => {
      if (!settled) {
        settled = true;
        reject(new Error(`shared-setup exited with code ${code} before ready`));
      }
    });

    child.stdin.write(JSON.stringify(fixtures));
    child.stdin.end();
  });
}
