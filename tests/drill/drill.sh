#!/usr/bin/env bash
# Compatibility entry point. The pristine Python supervisor owns workers,
# deadlines and verdict accounting; no mutated gotest code grades the drill.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
if [ "${1:-}" = "--list-unmutated" ]; then
  cd "$root"
  exec go run ./internal/speccatalog/clauses --unmutated
fi
exec python3 "$root/tests/drill/driver.py" "$@"
