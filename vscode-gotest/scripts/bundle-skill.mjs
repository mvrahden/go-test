// Copies the repository's agent skill into the extension, where the
// `chatSkills` contribution can point at it: a contributed SKILL.md must live
// inside the extension root. Runs on prepublish; the copy is gitignored.

import { cpSync, rmSync } from "node:fs";
import * as path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const name = "writing-gotest-tests";
const source = path.join(here, "..", "..", "skills", name);
const target = path.join(here, "..", "skills", name);

rmSync(target, { recursive: true, force: true });
cpSync(source, target, { recursive: true });
