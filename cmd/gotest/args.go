package main

import (
	"strings"
)

type ExecConfig struct {
	GoTestArgs      []string
	PackagePatterns []string
}

// knownSubcommands is the set of recognized subcommands.
var knownSubcommands = map[string]bool{
	"generate": true,
	"scaffold": true,
	"migrate":  true,
	"spec":     true,
	"watch":    true,
	"clean":    true,
	"version":  true,
	"help":     true,
}

// ParseSubcommand checks the first positional argument against known
// subcommands. If it matches, it is consumed and returned along with
// the remaining args. Otherwise, subcmd is empty and the full args
// slice is returned unchanged.
func ParseSubcommand(args []string) (subcmd string, remaining []string) {
	if len(args) == 0 {
		return "", nil
	}
	first := args[0]
	if knownSubcommands[first] {
		remaining = args[1:]
		if len(remaining) == 0 {
			remaining = nil
		}
		return first, remaining
	}
	return "", args
}

func SplitArgs(inArgs []string) (ownArgs, goTestArgs []string) {
	for i := 0; i < len(inArgs); i++ {
		arg := inArgs[i]
		switch {
		case arg == "--debug" || arg == "--ci" || arg == "--spec" || arg == "--update-snapshots":
			ownArgs = append(ownArgs, arg)
		case strings.HasPrefix(arg, "--min="):
			ownArgs = append(ownArgs, arg)
		case arg == "--min" && i+1 < len(inArgs):
			ownArgs = append(ownArgs, arg, inArgs[i+1])
			i++
		default:
			goTestArgs = append(goTestArgs, arg)
		}
	}
	return ownArgs, goTestArgs
}

func ExtractPackagePatterns(goTestArgs []string) []string {
	var patterns []string
	for _, arg := range goTestArgs {
		if arg == "-args" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if looksLikePackagePattern(arg) {
			patterns = append(patterns, arg)
		}
	}
	if len(patterns) == 0 {
		return []string{"."}
	}
	return patterns
}

func looksLikePackagePattern(s string) bool {
	return strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") || strings.Contains(s, "/")
}
