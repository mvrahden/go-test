package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
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
		if r.AbsPath == "" {
			continue
		}
		for _, v := range []struct {
			name string
			src  []byte
		}{{about.PSuite, r.PTest}, {about.PXSuite, r.PXTest}} {
			dst := filepath.Join(r.AbsPath, v.name)
			if err := syncGenerated(dst, v.src); err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: writing %s: %s\n", dst, err)
				return 2
			}
			if len(v.src) > 0 {
				fmt.Println(dst)
			}
		}
		// An older gotest's file would redeclare the harness for go vet,
		// go test and gopls.
		for _, name := range []string{about.LegacyPSuite, about.LegacyPXSuite} {
			if err := removeGenerated(filepath.Join(r.AbsPath, name)); err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: removing %s: %s\n", filepath.Join(r.AbsPath, name), err)
				return 2
			}
		}
	}

	return 0
}

// removeGenerated removes the file at path when it carries the
// generated-code comment; a file of that name that does not is left alone.
func removeGenerated(path string) error {
	src, err := os.ReadFile(path) //nolint:gosec // G304: a generated file name in the package's own directory
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !about.IsGeneratedSource(src)) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// syncGenerated brings the generated file at dst in line with src: written
// only when its content differs, so an unchanged file keeps its mtime, and
// removed when src is empty, so a variant without suites leaves no stale file.
func syncGenerated(dst string, src []byte) error {
	if len(src) == 0 {
		if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if have, err := os.ReadFile(dst); err == nil && bytes.Equal(have, src) { //nolint:gosec // G304: the package's own generated file
		return nil
	}
	return os.WriteFile(dst, src, 0644) //nolint:gosec // G306: not sensitive data
}
