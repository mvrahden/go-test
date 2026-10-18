package speccatalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Unmutated reads the "Clause: <when> → <it>" lines of every *.patch in
// patchDir and returns the clauses no patch names, sorted, and every Clause
// line naming a sentence the catalog does not hold, as "<patch>: <clause>".
// One reading serves the conventions suite and the listing on the command
// line, so the two cannot drift apart.
func Unmutated(clauses []string, patchDir string) (unmutated, unknown []string, err error) {
	known := make(map[string]bool, len(clauses))
	for _, c := range clauses {
		known[c] = true
	}
	patches, err := filepath.Glob(filepath.Join(patchDir, "*.patch"))
	if err != nil {
		return nil, nil, err
	}
	named := map[string]bool{}
	for _, p := range patches {
		data, err := os.ReadFile(p) //nolint:gosec // G304: the repository's own patches
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", p, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			clause, ok := strings.CutPrefix(line, "Clause: ")
			if !ok {
				continue
			}
			if !known[clause] {
				unknown = append(unknown, filepath.Base(p)+": "+clause)
			}
			named[clause] = true
		}
	}
	for _, c := range clauses {
		if !named[c] {
			unmutated = append(unmutated, c)
		}
	}
	sort.Strings(unmutated)
	sort.Strings(unknown)
	return unmutated, unknown, nil
}

// linkedDirs hold the code a test binary links: a mutant there is caught by
// the ring-0 packages and the canary, never by a conformance suite.
var linkedDirs = []string{"pkg/gotest/", "pkg/gotestruntime/", "internal/gotestgen/"}

// LinksIntoTestBinaries reports whether the patch changes a file under a
// directory test binaries link.
func LinksIntoTestBinaries(patch []byte) bool {
	for _, line := range strings.Split(string(patch), "\n") {
		path, ok := strings.CutPrefix(line, "--- a/")
		if !ok {
			path, ok = strings.CutPrefix(line, "+++ b/")
		}
		if !ok {
			continue
		}
		for _, d := range linkedDirs {
			if strings.HasPrefix(path, d) {
				return true
			}
		}
	}
	return false
}

// PackagesOf returns the conformance package directories, as "./<dir>",
// whose specs state the clauses the patch names on its Clause: lines, sorted
// and distinct, and every named clause no spec states.
func PackagesOf(specs []Spec, patch []byte) (pkgs, unknown []string) {
	byClause := map[string]string{}
	for _, s := range specs {
		for _, c := range Clauses([]Spec{s}) {
			byClause[c] = "./" + ConformanceDir + "/" + s.Capability
		}
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(patch), "\n") {
		clause, ok := strings.CutPrefix(line, "Clause: ")
		if !ok {
			continue
		}
		dir, ok := byClause[clause]
		if !ok {
			unknown = append(unknown, clause)
			continue
		}
		if !seen[dir] {
			seen[dir] = true
			pkgs = append(pkgs, dir)
		}
	}
	sort.Strings(pkgs)
	return pkgs, unknown
}
