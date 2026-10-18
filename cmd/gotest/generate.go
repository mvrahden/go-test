package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

func runGenerate(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
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

	loaded, broken, matched, err := gotestgen.LoadPackagesMatching(patterns, buildFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if !matched {
		// Nothing to generate for is not an error, as for go generate.
		warnMatchedNothing(patterns)
		return 0
	}
	if reportBrokenPackages(broken) {
		return 2
	}
	results, _, err := gotestgen.GenerateFromLoaded(loaded)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	outside := map[string]bool{}
	for _, lr := range loaded {
		if !lr.InMainModule() {
			outside[lr.PkgPath] = true
		}
	}
	for _, r := range results {
		// A dependency's suites run from an overlay; its files are not ours.
		if outside[r.PkgPath] {
			if len(r.PTest) > 0 || len(r.PXTest) > 0 {
				fmt.Fprintf(os.Stderr, "note: skipped %s: not in a main module\n", r.PkgPath)
			}
			continue
		}
		if len(r.PTest) > 0 {
			dst := filepath.Join(r.AbsPath, about.PSuite)
			if err := os.WriteFile(dst, r.PTest, 0644); err != nil { //nolint:gosec // G306: not sensitive data
				fmt.Fprintf(os.Stderr, "FAIL: writing %s: %s\n", dst, err)
				return 2
			}
			fmt.Println(dst)
		}
		if len(r.PXTest) > 0 {
			dst := filepath.Join(r.AbsPath, about.PXSuite)
			if err := os.WriteFile(dst, r.PXTest, 0644); err != nil { //nolint:gosec // G306: not sensitive data
				fmt.Fprintf(os.Stderr, "FAIL: writing %s: %s\n", dst, err)
				return 2
			}
			fmt.Println(dst)
		}
	}

	return 0
}
