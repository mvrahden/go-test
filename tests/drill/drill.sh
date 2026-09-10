#!/usr/bin/env bash
# The mutation drill: every patch in mutants/ sabotages one piece of the
# kernel; the ring-0 packages plus the canary must go red under each. A
# surviving mutant, or a patch that no longer applies, fails the drill. The
# shell judges by exit code alone, so nothing in gotest grades its own exam.
set -uo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
pkgs=(./pkg/gotest/internal/... ./pkg/gotestruntime/... ./internal/gotestspec/... ./tests/canary/...)
failed=0
work=$(mktemp -d)
judgedir=$(mktemp -d)
trap 'rm -rf "$work" "$judgedir"' EXIT

# The judge is a pristine build of the CLI. A mutant that zeroes the exit
# code would otherwise grade its own exam; the pristine judge compiles and
# runs the mutated copy's test binaries, which link the mutated runtime, and
# the canary inside builds the mutated CLI and holds it to the golden list.
judge="$judgedir/gotest"
if ! (cd "$root" && go build -o "$judge" ./cmd/gotest); then
  echo "DRILL  could not build the judge"; exit 1
fi
for patch in "$root"/tests/drill/mutants/*.patch; do
  name=$(basename "$patch" .patch)
  rm -rf "$work"; mkdir -p "$work"
  tar --exclude=./.git --exclude=./.worktrees --exclude=./node_modules --exclude='./vscode-gotest/node_modules' -C "$root" -cf - . | tar -xf - -C "$work"
  if ! patch -p1 -s -d "$work" < "$patch"; then
    echo "DRILL  $name: patch does not apply, refresh it"
    failed=1
    continue
  fi
  if (cd "$work" && "$judge" summary --no-cache "${pkgs[@]}" >"$work/drill.log" 2>&1); then
    echo "DRILL  $name: SURVIVED, the guards did not catch it"
    sed 's/^/       /' "$work/drill.log" | tail -5
    failed=1
  else
    echo "drill  $name: caught ($(grep -m1 -oE '(FAIL: census[^\n]*|[0-9]+ failed|[0-9]+ failed packages)' "$work/drill.log" | head -c 60))"
  fi
done
exit $failed
