#!/usr/bin/env bash
# Checks that a version may be released from a commit, before anything is built
# or uploaded.
#
#   release-preflight.sh <version> [<sha>]           before tagging: the tag must
#                                                    not exist yet; sha defaults
#                                                    to origin/main
#   release-preflight.sh --tagged <version> <sha>    the release workflow: the
#                                                    tag exists and points at sha
#
# Both modes require: vX.Y.Z above every other release tag, a version the module
# proxy has not recorded at another commit, a commit on origin/main, and green
# gate runs for it.
set -euo pipefail

usage() { echo "usage: $0 [--tagged] <vX.Y.Z> [<sha>]" >&2; exit 2; }
fail() { echo "preflight: $*" >&2; exit 1; }
ok() { echo "ok    $*"; }

tagged=false
if [ "${1:-}" = --tagged ]; then tagged=true; shift; fi
[ $# -ge 1 ] && [ $# -le 2 ] || usage
version=$1
sha_arg=${2:-}
$tagged && [ -z "$sha_arg" ] && usage

repo=${GITHUB_REPOSITORY:-mvrahden/go-test}
module=$(sed -n 's/^module //p' go.mod)

# Required workflows, each with the paths its push trigger filters on.
workflows=(test.yml quality.yml contract.yml)
gate_path() {
	local wf=$1 f=$2
	case $wf in
	test.yml)
		case $f in
		vscode-gotest/src/cli.ts | vscode-gotest/testdata/*) return 0 ;;
		vscode-gotest/* | site/*) return 1 ;;
		*) return 0 ;;
		esac
		;;
	quality.yml)
		case $f in
		vscode-gotest/* | site/*) return 1 ;;
		*) return 0 ;;
		esac
		;;
	contract.yml)
		case $f in
		cmd/* | internal/* | pkg/* | go.mod | go.sum | vscode-gotest/* | .github/workflows/contract.yml) return 0 ;;
		*) return 1 ;;
		esac
		;;
	esac
}

# 1. The version is vX.Y.Z.
[[ $version =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] ||
	fail "$version is not a release version vX.Y.Z"
ok "$version is a release version"

# 2. Above every other release tag, as origin has them.
remote_tags=$(git ls-remote --tags origin) || fail "cannot list origin's tags"
highest=$(printf '%s\n' "$remote_tags" | sed -n 's|.*refs/tags/\(v[0-9]*\.[0-9]*\.[0-9]*\)$|\1|p' |
	grep -vx "$version" | sort -V | tail -n1 || true)
if [ -n "$highest" ]; then
	[ "$(printf '%s\n%s\n' "$highest" "$version" | sort -V | tail -n1)" = "$version" ] ||
		fail "$version is not above the highest release tag $highest"
	ok "$version is above $highest"
fi

# 3. The tag: absent before tagging, at sha when the release runs.
git fetch --quiet origin +refs/heads/main:refs/remotes/origin/main || fail "cannot fetch origin/main"
remote_tag=$(printf '%s\n' "$remote_tags" | awk -v t="refs/tags/$version^{}" '$2 == t { print $1 }')
[ -n "$remote_tag" ] || remote_tag=$(printf '%s\n' "$remote_tags" | awk -v t="refs/tags/$version" '$2 == t { print $1 }')
if $tagged; then
	sha=$(git rev-parse --verify --quiet "$sha_arg^{commit}") || fail "$sha_arg is not a commit here"
	[ -n "$remote_tag" ] || fail "origin has no tag $version"
	[ "$remote_tag" = "$sha" ] || fail "tag $version points at $remote_tag, not at $sha: was it moved?"
	ok "tag $version points at $sha"
else
	sha=$(git rev-parse --verify --quiet "${sha_arg:-origin/main}^{commit}") || fail "${sha_arg:-origin/main} is not a commit here"
	[ -z "$remote_tag" ] || fail "origin already has tag $version (at $remote_tag); a published version never changes"
	git rev-parse --verify --quiet "refs/tags/$version" >/dev/null &&
		fail "a local tag $version exists; delete it before tagging"
	ok "no tag $version yet"
fi

# 4. The proxy. Before tagging it must not list the version, and only @v/list
# is asked: asking for the version itself can make the proxy fetch it or
# remember it as missing. Once the tag exists the proxy may record it any
# moment, so a listed version must be recorded at sha.
proxy=https://proxy.golang.org/$module/@v
list=$(curl -fsS "$proxy/list") || fail "cannot read the module proxy's version list"
if ! printf '%s\n' "$list" | grep -qx "$version"; then
	ok "proxy.golang.org does not list $version"
elif $tagged; then
	recorded=$(curl -fsS "$proxy/$version.info" | jq -r '.Origin.Hash // empty') ||
		fail "cannot read the proxy's record of $version"
	[ -n "$recorded" ] || fail "the proxy's record of $version names no commit"
	[ "$recorded" = "$sha" ] || fail "the proxy recorded $version at $recorded, not $sha: a moved tag"
	ok "proxy.golang.org recorded $version at $sha"
else
	fail "proxy.golang.org already lists $version; its checksum is fixed, so pick the next version"
fi

# 5. The commit is on main.
git merge-base --is-ancestor "$sha" origin/main || fail "$sha is not on origin/main"
ok "$sha is on origin/main"

# 6. Each required workflow ran green on main for sha, or, where its path
# filter skipped sha, on the nearest ancestor with no gate change since.
run_of() { # workflow commit -> "status conclusion url" of its latest push run, or nothing
	gh api "repos/$repo/actions/workflows/$1/runs?head_sha=$2&event=push&branch=main&per_page=1" \
		--jq '.workflow_runs[0] | select(.) | "\(.status) \(.conclusion) \(.html_url)"'
}
for wf in "${workflows[@]}"; do
	found=""
	for c in $(git rev-list --first-parent --max-count=50 "$sha"); do
		run=$(run_of "$wf" "$c") || fail "cannot query $wf runs (gh needs a token with actions: read)"
		[ -n "$run" ] && { found=$c; break; }
	done
	[ -n "$found" ] || fail "$wf has no run on main within 50 commits of $sha"
	read -r status conclusion url <<<"$run"
	[ "$status" = completed ] || fail "$wf is still $status on ${found:0:12}; wait for it: $url"
	[ "$conclusion" = success ] || fail "$wf concluded $conclusion on ${found:0:12}: $url"
	if [ "$found" != "$sha" ]; then
		while IFS= read -r f; do
			gate_path "$wf" "$f" && fail "$wf last ran on ${found:0:12}, but $f changed since and no run covers $sha"
		done < <(git diff --name-only "$found" "$sha")
		ok "$wf green on ${found:0:12}, nothing it covers changed up to ${sha:0:12}"
	else
		ok "$wf green on ${sha:0:12}"
	fi
done

echo "preflight: $version may be released from $sha"
