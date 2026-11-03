// Versions relative to the extension's CLI floor, so a floor bump moves the
// tests with it.

import { MIN_CLI_VERSION } from "./cli.js";

const [major, minor, patch] = MIN_CLI_VERSION.slice(1).split(".").map(Number);

// The release just below the floor.
export const BELOW_FLOOR =
  patch > 0
    ? `v${major}.${minor}.${patch - 1}`
    : minor > 0
      ? `v${major}.${minor - 1}.0`
      : `v${major - 1}.0.0`;

// The patch release after the floor.
export const ABOVE_FLOOR = `v${major}.${minor}.${patch + 1}`;

// A commit pin built on the floor: ordered by its base, so accepted.
export const PSEUDO_AT_FLOOR = `${MIN_CLI_VERSION}-0.20261011200000-3588ff09a1b2`;
