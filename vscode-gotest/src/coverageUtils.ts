import { resolveGoBinary } from "./cli.js";
import { captureStdout } from "./capture.js";

export function splitCoverByPackage(
  content: string,
  importPaths: string[],
): Map<string, string> {
  const lines = content.split("\n");
  const buckets = new Map<string, string[]>();
  let modeLine = "";

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    if (trimmed.startsWith("mode:")) {
      modeLine = trimmed;
      continue;
    }
    for (const ip of importPaths) {
      if (trimmed.startsWith(ip + "/")) {
        let bucket = buckets.get(ip);
        if (!bucket) {
          bucket = [];
          buckets.set(ip, bucket);
        }
        bucket.push(trimmed);
        break;
      }
    }
  }

  const result = new Map<string, string>();
  for (const [ip, pkgLines] of buckets) {
    result.set(ip, modeLine + "\n" + pkgLines.join("\n") + "\n");
  }
  return result;
}

export function splitFuncCoverageByPackage(
  content: string,
  importPaths: string[],
): Map<string, string> {
  const lines = content.split("\n");
  const buckets = new Map<string, string[]>();

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("total:")) continue;
    for (const ip of importPaths) {
      if (trimmed.startsWith(ip + "/")) {
        let bucket = buckets.get(ip);
        if (!bucket) {
          bucket = [];
          buckets.set(ip, bucket);
        }
        bucket.push(trimmed);
        break;
      }
    }
  }

  const result = new Map<string, string>();
  for (const [ip, pkgLines] of buckets) {
    result.set(ip, pkgLines.join("\n") + "\n");
  }
  return result;
}

export async function runGoToolCoverFunc(
  coverFile: string,
  workspaceDir: string,
): Promise<string> {
  const goBin = await resolveGoBinary(undefined, workspaceDir);
  // One line per function in the profile: a repository large enough to have a
  // coverage problem is large enough to write past a buffered read.
  return captureStdout(goBin, ["tool", "cover", `-func=${coverFile}`], {
    cwd: workspaceDir,
    timeoutSeconds: 10,
  });
}
