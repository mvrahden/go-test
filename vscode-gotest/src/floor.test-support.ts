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

// A commit after the floor, as Go writes it: the next patch's pseudo-version.
export const PSEUDO_ABOVE_FLOOR = `v${major}.${minor}.${patch + 1}-0.20261011200000-3588ff09a1b2`;

// A commit after the release below the floor, so still before the floor.
export const PSEUDO_BELOW_FLOOR = `${BELOW_FLOOR.replace(
  /\.(\d+)$/,
  (_, p: string) => `.${Number(p) + 1}`,
)}-0.20261011200000-3588ff09a1b2`;

// A release candidate of the floor: before the floor.
export const RC_OF_FLOOR = `${MIN_CLI_VERSION}-rc.1`;
