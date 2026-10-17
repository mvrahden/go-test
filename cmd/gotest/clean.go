package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

// runClean removes the generated files from the packages the patterns
// match, expanded as generate expands them: only in a main module, where
// generate writes them.
func runClean(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	args := inv.TagArgs()
	patterns, err := ExtractPackagePatterns(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	tags, _ := extractTagsFlag(args)
	var buildFlags []string
	if tags != "" {
		buildFlags = append(buildFlags, "-tags="+tags)
	}

	pkgs, unmatched, err := gotestgen.ListPackageDirs("", patterns, buildFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	warnMatchedNothing(unmatched)

	var removed int
	for _, p := range pkgs {
		if p.Dir == "" {
			// Nothing there to remove, as for a missing directory.
			fmt.Fprintf(os.Stderr, "warning: %s\n", p.Err)
			continue
		}
		generated := generatedFiles(p.Dir)
		if !p.Main {
			if len(generated) > 0 {
				fmt.Fprintf(os.Stderr, "note: skipped %s: not in a main module\n", p.ImportPath)
			}
			continue
		}
		for _, path := range generated {
			if err := os.Remove(path); err != nil {
				fmt.Fprintf(os.Stderr, "warning: %s\n", err)
				continue
			}
			fmt.Println(path)
			removed++
		}
	}

	if removed > 0 {
		fmt.Fprintf(os.Stderr, "removed %d generated file(s)\n", removed)
	}
	return 0
}

// generatedFiles returns the generated files in dir.
func generatedFiles(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var files []string
	for _, e := range entries {
		if e.Type().IsRegular() && about.PSuiteRegex.MatchString(e.Name()) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}
