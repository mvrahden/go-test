package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/about"
)

var (
	DEBUG bool
	CLEAN bool
	CI    bool
)

func main() {
	ownArgs, goTestArgs := SplitArgs(os.Args[1:])
	DEBUG = slices.Contains(ownArgs, "-ƒƒ.internal.debug")
	CLEAN = slices.Contains(ownArgs, "-ƒƒ.clean")
	CI = slices.Contains(ownArgs, "--ci")

	if CLEAN {
		runClean(goTestArgs)
		return
	}

	patterns := ExtractPackagePatterns(goTestArgs)
	cfg := ExecConfig{
		GoTestArgs:      goTestArgs,
		PackagePatterns: patterns,
	}

	os.Exit(Run(cfg))
}

func runClean(goTestArgs []string) {
	patterns := ExtractPackagePatterns(goTestArgs)
	for _, pattern := range patterns {
		dir := strings.TrimSuffix(pattern, "/...")
		if dir == "" {
			dir = "."
		}
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && about.PSuiteRegex.MatchString(d.Name()) {
				fmt.Fprintf(os.Stdout, "removing %s\n", path)
				os.Remove(path)
			}
			return nil
		})
	}
}
