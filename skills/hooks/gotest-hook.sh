#!/bin/sh
# Hands an agent hook event on stdin to `gotest hook <event>`, which holds
# the logic. The CLI is the module's `go tool gotest` when go.mod declares
# the tool, else `gotest` on PATH. Without one that knows the hook command,
# it prints nothing. It always exits 0 and never downloads.

event=$1
input=$(cat)

# Most events are none of gotest's business; skip starting a CLI for them.
case $event in
post-edit) case $input in *_test.go*) ;; *) exit 0 ;; esac ;;
pre-shell) case $input in *go*test*) ;; *) exit 0 ;; esac ;;
esac

# answer runs the hook with the CLI given as arguments, once it knows the
# hook command; an older gotest would read "hook" as a package.
answer() {
	case $("$@" help hook 2>/dev/null) in
	"gotest hook"*) ;;
	*) return 1 ;;
	esac
	out=$(printf '%s' "$input" | "$@" hook "$event" 2>/dev/null) || return 1
	[ -n "$out" ] && printf '%s\n' "$out"
	exit 0
}

# module_root prints the directory of the go.mod at or above $1.
module_root() {
	dir=$1
	while [ -n "$dir" ]; do
		if [ -f "$dir/go.mod" ]; then
			printf '%s' "$dir"
			return 0
		fi
		[ "$dir" = / ] && return 1
		dir=$(dirname "$dir")
	done
	return 1
}

export GOPROXY=off
if command -v go >/dev/null 2>&1; then
	for start in "$PWD" "${CLAUDE_PROJECT_DIR:-}"; do
		[ -n "$start" ] || continue
		root=$(module_root "$start") || continue
		if grep -Eq '^[[:space:]]*(tool[[:space:]]+)?github\.com/mvrahden/go-test/cmd/gotest[[:space:]]*$' "$root/go.mod"; then
			(cd "$root" && answer go tool gotest)
			[ $? -eq 0 ] && exit 0
		fi
	done
fi
if command -v gotest >/dev/null 2>&1; then
	answer gotest
fi
exit 0
