// Package pkgpattern reads package patterns the way go test does where go
// list, which the loaders call, reads them differently.
package pkgpattern

import (
	"os"
	"slices"
	"strings"
)

// Clean returns patterns with each empty one as the current directory: go
// test reads "" as ".", go list rejects it.
func Clean(patterns []string) []string {
	if !slices.Contains(patterns, "") {
		return patterns
	}
	out := slices.Clone(patterns)
	for i, p := range out {
		if p == "" {
			out[i] = "."
		}
	}
	return out
}

// Files returns the patterns that name Go files, as go test tells them
// apart: a ".go" suffix on an existing file, not a directory.
func Files(patterns []string) []string {
	var files []string
	for _, p := range patterns {
		if !strings.HasSuffix(p, ".go") {
			continue
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			files = append(files, p)
		}
	}
	return files
}
