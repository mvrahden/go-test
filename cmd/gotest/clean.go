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
	_, args, err := SplitArgs(inv.TagArgs(), noOwnFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
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

// generatedFiles returns the generated files in dir: named as gotest names
// them and carrying the generated-code comment. A file only named so is
// someone's own, and is left alone with a warning.
func generatedFiles(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var files []string
	for _, e := range entries {
		if !e.Type().IsRegular() || !about.PSuiteRegex.MatchString(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		src, err := os.ReadFile(path) //nolint:gosec // G304: a file in a package directory clean was pointed at
		if err != nil || !about.IsGeneratedSource(src) {
			fmt.Fprintf(os.Stderr, "warning: %s is named like a generated file but is not one; left in place\n", path)
			continue
		}
		files = append(files, path)
	}
	return files
}
