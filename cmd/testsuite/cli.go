package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/mvrahden/go-test/about"
)

var (
	DEBUG bool
	CLEAN bool
)

func main() {
	ownArgs, goTestArgs := SplitArgs(os.Args[1:])
	DEBUG = slices.Contains(ownArgs, "-ƒƒ.internal.debug")
	CLEAN = slices.Contains(ownArgs, "-ƒƒ.clean")

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
		filepath.WalkDir(pattern, func(path string, d fs.DirEntry, err error) error {
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
