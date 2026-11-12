#!/usr/bin/env bash
# Lists every test whose verdict differs between go test -json streams of
# the same tree. A test missing from a stream counts as "none".
# Usage: mixed-verdicts.sh <events file> <events file>...
set -euo pipefail

usage() {
	echo "usage: mixed-verdicts.sh <events file> <events file>..."
	exit 2
}
[ $# -ge 2 ] || usage
for f in "$@"; do
	[ -f "$f" ] && [ -r "$f" ] || usage
done

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
i=0
for f in "$@"; do
	i=$((i + 1))
	# The last verdict per package and test wins.
	jq -rR 'fromjson? | select(.Test != null and (.Action == "pass" or .Action == "fail" or .Action == "skip")) | [.Package, .Test, .Action] | @tsv' "$f" |
		awk -F'\t' -v run="$i" '{ v[$1 " " $2] = $3 } END { for (k in v) print run "\t" k "\t" v[k] }' >>"$tmp/verdicts"
done
touch "$tmp/verdicts"

n=$#
rows=$(awk -F'\t' -v n="$n" '
	{ v[$2, $1] = $3; keys[$2] = 1 }
	END {
		for (k in keys) {
			row = "| " k " |"; mixed = 0
			for (i = 1; i <= n; i++) {
				x = ((k, i) in v) ? v[k, i] : "none"
				if (i == 1) first = x; else if (x != first) mixed = 1
				row = row " " x " |"
			}
			if (mixed) print row
		}
	}' "$tmp/verdicts" | LC_ALL=C sort)

report() {
	if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
		printf '%s\n' "$1" | tee -a "$GITHUB_STEP_SUMMARY"
	else
		printf '%s\n' "$1"
	fi
}

if [ -z "$rows" ]; then
	report "No test changed its verdict across $n runs."
	exit 0
fi
header="| Test |"
divider="|---|"
for ((i = 1; i <= n; i++)); do
	header+=" Run $i |"
	divider+="---|"
done
k=$(printf '%s\n' "$rows" | wc -l | tr -d ' ')
report "$k tests changed their verdict across $n runs:

$header
$divider
$rows"
exit 1
