#!/usr/bin/env bash
# Opens one issue for a red night, or comments on the open one. A Go tip
# failure alone has an issue of its own.
# Usage: nightly-report.sh <needs json> <run url>
set -euo pipefail

if [ $# -ne 2 ] || [ ! -r "$1" ]; then
	echo "usage: nightly-report.sh <needs json> <run url>" >&2
	exit 2
fi
needs=$1 url=$2

failed=$(jq -r '[to_entries[] | select(.value.result == "failure" or .value.result == "cancelled") | .key] | sort | join(", ")' "$needs")
tip=$(jq -r '."go-tip".outputs.outcome // ""' "$needs")

if [ -z "$failed" ] && [ "$tip" != failure ]; then
	echo "nightly: nothing to report"
	exit 0
fi

body="Run: $url"
if [ -n "$failed" ]; then
	body+=$'\n'"Failed: $failed"
fi
if [ "$tip" = failure ]; then
	body+=$'\n'"Go tip: failed"
fi
title="Nightly is red"
if [ -z "$failed" ]; then
	title="Nightly: Go tip is red"
fi

number=$(gh issue list --state open --search "in:title \"$title\"" --json number --jq '.[0].number')
if [ -n "$number" ] && [ "$number" != null ]; then
	gh issue comment "$number" --body "$body"
	echo "nightly: commented on #$number"
else
	gh issue create --title "$title" --body "$body"
	echo "nightly: opened \"$title\""
fi
