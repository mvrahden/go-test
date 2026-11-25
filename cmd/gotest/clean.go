package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

// runClean removes the generated files from the packages the patterns
// match, expanded as generate expands them: only in a main module, where
// generate writes them. With --cache it clears the test binary cache and the
// audit's base trees instead.
func runClean(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	own, args, err := SplitArgs(inv.TagArgs(), cleanAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if own.Bool("--cache") {
		return clearCache(args)
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
			if !isGenerated(path) {
				fmt.Fprintf(os.Stderr, "warning: %s is named like a generated file but is not one; left in place\n", path)
				continue
			}
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

// clearCache removes every cached test binary no run is using and every
// base tree no audit is using; args may hold go test flags, never a
// package.
func clearCache(args []string) int {
	if len(gotestrunner.PatternIndices(args, ownFlag)) > 0 {
		fmt.Fprintln(os.Stderr, "FAIL: --cache clears the test binary cache and takes no packages; run gotest clean <packages> for generated files")
		return 2
	}
	removed, inUse, dir, err := gotestrunner.ClearBinaryCache()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "removed %d cached test binaries from %s\n", removed, dir)
	if inUse > 0 {
		fmt.Fprintf(os.Stderr, "note: %d left in place: a running gotest is using them\n", inUse)
	}
	trees, held, treeDir, err := gotestrunner.ClearAuditBases()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if trees > 0 {
		fmt.Fprintf(os.Stderr, "removed %d audit base tree%s from %s\n", trees, plural(trees), treeDir)
	}
	if held > 0 {
		fmt.Fprintf(os.Stderr, "note: %d audit base tree%s left in place: a running audit is using %s\n", held, plural(held), itThem(held))
	}
	return 0
}

func itThem(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

// generatedFiles returns the files in dir named as gotest names generated
// ones.
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

// isGenerated reports whether the file at path carries the generated-code
// comment; a file only named like one is someone's own.
func isGenerated(path string) bool {
	src, err := os.ReadFile(path) //nolint:gosec // G304: a file in a package directory clean was pointed at
	return err == nil && about.IsGeneratedSource(src)
}
