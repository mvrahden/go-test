#!/usr/bin/env bash
# Entry point. A driver built from the unmodified tree owns workers,
# deadlines and verdict accounting; no mutated gotest code grades the drill.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
if [ "${1:-}" = "--list-unmutated" ]; then
  cd "$root"
  exec go run ./internal/speccatalog/clauses --unmutated
fi
# The driver is exec'd, so signals meant for the drill reach it directly.
# It removes its build directory when it exits.
bin=$(mktemp -d "${TMPDIR:-/tmp}/gotest-drill-bin-XXXXXX")
if ! (cd "$root" && go build -o "$bin/drill" ./tests/drill/driver); then
  rm -rf "$bin"
  echo "DRILL  could not build the drill driver" >&2
  exit 1
fi
exec "$bin/drill" --root "$root" --binary-dir "$bin" "$@"
