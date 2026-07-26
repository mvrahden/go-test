#!/usr/bin/env bash
# Windowed census runner: clones a batch of repos at their pinned SHAs, scans
# them, appends results to one CSV, deletes the batch, repeats. Peak disk usage
# is one window, not the whole corpus (~48 GB unwindowed).
#
# Usage: ./run.sh corpus/top1000-curated.csv results.csv [window-size] [workdir]
set -euo pipefail

CORPUS=${1:?corpus csv (repo,sha? use top1000-curated.csv)}
OUT=${2:?output csv}
WINDOW=${3:-50}
WORKDIR=${4:-$(mktemp -d)}
JOBS=${JOBS:-6}

command -v test-census >/dev/null || { echo "build first: go build -o test-census ." >&2; exit 1; }

# repo,keep -> "owner/repo sha" lines; sha column optional (falls back to
# HEAD). Repos already present in $OUT are skipped, so an interrupted run can
# simply be restarted with the same arguments.
mapfile -t REPOS < <(python3 - "$CORPUS" "$OUT" <<'EOF'
import csv, os, sys
done = set()
if os.path.exists(sys.argv[2]) and os.path.getsize(sys.argv[2]) > 0:
    done = {r["repo"] for r in csv.DictReader(open(sys.argv[2]))}
for r in csv.DictReader(open(sys.argv[1])):
    if r.get("keep", "true") != "true" or r["repo"] in done:
        continue
    print(r["repo"], r.get("sha", ""))
EOF
)

total=${#REPOS[@]}
if [ "$total" -eq 0 ]; then echo "nothing to do: all corpus repos already in $OUT"; exit 0; fi
echo "corpus: $total repos to scan (resuming past $(wc -l <"$OUT" 2>/dev/null || echo 0) rows), window=$WINDOW, workdir=$WORKDIR"

fetch_one() { # "owner/repo sha" -> shallow checkout at pinned sha (or HEAD)
  local repo sha dir
  repo=${1% *}; sha=${1#* }
  dir="$2/$(echo "$repo" | tr '/' '_')"
  [ -d "$dir" ] && return 0
  if [ -n "$sha" ] && [ "$sha" != "$repo" ]; then
    git init -q "$dir" &&
      git -C "$dir" remote add origin "https://github.com/$repo.git" &&
      git -C "$dir" fetch -q --depth 1 origin "$sha" &&
      git -C "$dir" checkout -q FETCH_HEAD && return 0
    rm -rf "$dir"
  fi
  timeout 240 git clone --quiet --depth 1 "https://github.com/$repo.git" "$dir" ||
    { echo "CLONE FAILED: $repo" >&2; rm -rf "$dir"; }
}
export -f fetch_one

for ((start = 0; start < total; start += WINDOW)); do
  batch=("${REPOS[@]:start:WINDOW}")
  bdir="$WORKDIR/window"
  mkdir -p "$bdir"
  printf '%s\n' "${batch[@]}" | xargs -P"$JOBS" -I{} bash -c 'fetch_one "{}" "'"$bdir"'"'
  test-census -append -out "$OUT" "$bdir"/*/
  rm -rf "$bdir"
  echo "window $((start / WINDOW + 1)): $((start + ${#batch[@]}))/$total done"
done
echo "census written to $OUT"
