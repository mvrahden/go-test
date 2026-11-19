#!/usr/bin/env bash
# Merges the coverage the CLI's subprocess runs wrote under GOCOVERDIR into
# the suites' profile, prints both totals and fails below the minimum.
# Usage: coverage-total.sh <suites profile> <coverage data dir> <minimum percent> <package pattern>...
set -euo pipefail

usage() {
	echo "usage: coverage-total.sh <suites profile> <coverage data dir> <minimum percent> <package pattern>..."
	exit 2
}
[ $# -ge 4 ] || usage
suites=$1 dir=$2 min=$3
shift 3
[ -f "$suites" ] && [ -r "$suites" ] || usage

if ! compgen -G "$dir/covmeta.*" >/dev/null; then
	echo "no coverage data in $dir: set GOCOVERDIR for the run, and run a suite that starts the CLI"
	exit 2
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
go tool covdata textfmt -i="$dir" -pkg="$(
	IFS=,
	echo "$*"
)" -o "$tmp/cli.out"

suites_mode=$(head -n 1 "$suites")
cli_mode=$(head -n 1 "$tmp/cli.out")
if [ "$suites_mode" != "$cli_mode" ]; then
	echo "the suites' profile is $suites_mode and the CLI's data is $cli_mode: run the suites with -race or -covermode=atomic"
	exit 2
fi

# go tool cover sums the counts of identical blocks.
{
	cat "$suites"
	tail -n +2 "$tmp/cli.out"
} >"$tmp/total.out"
percent() { go tool cover -func="$1" | tail -n 1 | awk '{ sub(/%/, "", $NF); print $NF }'; }
total=$(percent "$tmp/total.out")
alone=$(percent "$suites")

line="coverage: ${total}% with the CLI's subprocess runs, ${alone}% from the suites alone (minimum ${min}%)"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
	echo "$line" >>"$GITHUB_STEP_SUMMARY"
fi
echo "$line"
if awk -v t="$total" -v m="$min" 'BEGIN { exit !(t + 0 < m + 0) }'; then
	echo "FAIL: ${total}% coverage with the CLI's subprocess runs (minimum ${min}%)"
	exit 1
fi
