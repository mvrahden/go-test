package main

import (
	"strings"
)

type ExecConfig struct {
	GoTestArgs      []string
	PackagePatterns []string
}

func SplitArgs(inArgs []string) (ownArgs, goTestArgs []string) {
	for _, arg := range inArgs {
		if strings.HasPrefix(arg, "-ƒƒ.") {
			ownArgs = append(ownArgs, arg)
		} else {
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
