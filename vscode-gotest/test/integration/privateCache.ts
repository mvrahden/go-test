// The integration runs keep gotest's cache, run records included, in a
// directory of their own, never in the user's. A test that needs another
// sets its own.
import { tmpdir } from "node:os";
import * as path from "node:path";
import { rmSync } from "node:fs";

export const PRIVATE_CACHE = path.join(
  tmpdir(),
  "gotest-vscode-integration-cache",
);

process.env.GOTEST_CACHE_DIR = PRIVATE_CACHE;

// The global teardown: the cache goes with the run.
export default function setup(): () => void {
  return () => rmSync(PRIVATE_CACHE, { recursive: true, force: true });
}
