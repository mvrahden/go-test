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
