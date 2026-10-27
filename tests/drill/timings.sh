#!/usr/bin/env bash
# Rewrites tests/drill/timings.json from the drill shards' timings-i-of-n.json
# artifacts: per mutant the longest observation across the given runs,
# rounded to 0.1s. No run ids reads the last three successful quality.yml
# runs on main. Mutants without a patch here are dropped.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
runs=("$@")
if [ ${#runs[@]} -eq 0 ]; then
  mapfile -t runs < <(gh run list --workflow quality.yml --branch main --status success \
    --limit 3 --json databaseId --jq '.[].databaseId')
fi
[ ${#runs[@]} -gt 0 ] || { echo "no runs to read" >&2; exit 1; }
work=$(mktemp -d "${TMPDIR:-/tmp}/gotest-drill-timings-XXXXXX")
trap 'rm -rf "$work"' EXIT
for run in "${runs[@]}"; do
  gh run download "$run" --pattern 'drill-*' --dir "$work/$run" >/dev/null
done
mapfile -t files < <(find "$work" -name 'timings-*-of-*.json' | sort)
[ ${#files[@]} -gt 0 ] || { echo "no timings in runs ${runs[*]}" >&2; exit 1; }
names=$(find "$root/tests/drill/mutants" -name '*.patch' -exec basename {} .patch \; | jq -R . | jq -s .)
jq -s --argjson names "$names" '
  (reduce (.[] | to_entries[]) as $e ({}; .[$e.key] = ([.[$e.key] // 0, $e.value] | max)))
  | with_entries(select(.key as $k | $names | index($k)))
  | map_values([(. * 10 | round) / 10, 0.1] | max)
' "${files[@]}" | jq -S . >"$root/tests/drill/timings.json"
echo "tests/drill/timings.json: $(jq length "$root/tests/drill/timings.json") mutants from runs ${runs[*]}"
