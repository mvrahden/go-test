#!/usr/bin/env bash
# The mutation drill: every patch in mutants/ sabotages one piece of the
# kernel; the ring-0 packages plus the canary must go red under each, and
# the check the patch names must be the one that fires. A surviving mutant,
# a patch that no longer applies exactly, a mutant that does not compile, or
# one caught by the wrong check fails the drill. The shell judges by exit
# code and log text, so nothing in gotest grades its own exam.
set -uo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
pkgs=(./pkg/gotest/internal/... ./pkg/gotestruntime/... ./internal/gotestspec/... ./tests/canary/...)
failed=0
work=$(mktemp -d)
judgedir=$(mktemp -d)
trap 'rm -rf "$work" "$judgedir"' EXIT
esc=$(printf '\033')

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
  # Expect: lines precede the diff; each is an ERE the judge's log must match.
  mapfile -t expects < <(sed -n '/^--- /q; s/^Expect: //p' "$patch")
  if [ ${#expects[@]} -eq 0 ]; then
    echo "DRILL  $name: no Expect: line, name the check that must catch it"
    failed=1
    continue
  fi
  rm -rf "$work"; mkdir -p "$work"
  tar --exclude=./.git --exclude=./.worktrees --exclude=./node_modules --exclude='./vscode-gotest/node_modules' -C "$root" -cf - . | tar -xf - -C "$work"
  # Exact context only; a line offset is fine since the context still matches verbatim.
  if ! patch -p1 -s --fuzz=0 --forward -r - -d "$work" -i "$patch" >"$work/patch.log" 2>&1; then
    echo "DRILL  $name: patch does not apply, refresh it"
    sed 's/^/       /' "$work/patch.log" | tail -5
    failed=1
    continue
  fi
  # The judge exits 2 on a build failure, which would pass for caught.
  if ! (cd "$work" && go build ./... && go test -vet=off -c -o "$work/.drillbin/" "${pkgs[@]}") >"$work/build.log" 2>&1; then
    echo "DRILL  $name: mutant does not compile, refresh it"
    sed 's/^/       /' "$work/build.log" | tail -5
    failed=1
    continue
  fi
  if (cd "$work" && "$judge" summary --no-cache "${pkgs[@]}" >"$work/drill.log" 2>&1); then
    echo "DRILL  $name: SURVIVED, the guards did not catch it"
    sed 's/^/       /' "$work/drill.log" | tail -5
    failed=1
    continue
  fi
  sed "s/$esc\[[0-9;]*m//g" "$work/drill.log" >"$work/plain.log"
  missing=()
  for e in "${expects[@]}"; do
    grep -qE -- "$e" "$work/plain.log" || missing+=("$e")
  done
  if [ ${#missing[@]} -gt 0 ]; then
    echo "DRILL  $name: caught by the wrong check"
    printf '       no line matches: %s\n' "${missing[@]}"
    grep -m5 -E '^FAIL' "$work/plain.log" | sed 's/^/       /'
    failed=1
  else
    echo "drill  $name: caught ($(grep -m1 -E -- "${expects[0]}" "$work/plain.log" | sed 's/^ *//' | head -c 80))"
  fi
done
exit $failed
