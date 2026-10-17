package conventions_test

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// UnmutatedClausesTestSuite keeps the drill honest about the catalog: a
// mutant may name the clauses it falsifies on `Clause:` lines, every such
// line must name a real clause, and the number of clauses no mutant
// falsifies may not grow past the budget in tests/drill/unmutated-allow.txt.
type UnmutatedClausesTestSuite struct{}

func (s *UnmutatedClausesTestSuite) TestClausesWithoutAMutantStayWithinBudget(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	js, err := os.ReadFile(filepath.Join(root, speccatalog.JSONPath))
	gotest.NoError(t, err)
	clauses, err := speccatalog.Clauses(js)
	gotest.NoError(t, err)
	known := map[string]bool{}
	for _, c := range clauses {
		known[c] = true
	}

	patches, err := filepath.Glob(filepath.Join(root, "tests", "drill", "mutants", "*.patch"))
	gotest.NoError(t, err)
	named := map[string]bool{}
	var unknown []string
	for _, p := range patches {
		data, err := os.ReadFile(p) //nolint:gosec // G304: the repository's own patches
		gotest.NoError(t, err)
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

	var unmutated []string
	for _, c := range clauses {
		if !named[c] {
			unmutated = append(unmutated, c)
		}
	}
	sort.Strings(unmutated)

	allowData, err := os.ReadFile(filepath.Join(root, "tests", "drill", "unmutated-allow.txt"))
	gotest.NoError(t, err)
	allow, err := strconv.Atoi(strings.TrimSpace(string(allowData)))
	gotest.NoError(t, err)

	t.When("every mutant's Clause lines are checked", func(w *gotest.T) {
		w.It("names only clauses the catalog holds", func(it *gotest.T) {
			gotest.Empty(it, unknown)
		})
	})
	t.When("the clauses no mutant falsifies are counted", func(w *gotest.T) {
		w.It("stay within the allowed budget", func(it *gotest.T) {
			gotest.LessOrEqual(it, len(unmutated), allow, "add a mutant with a Clause: line for the new clause, or raise tests/drill/unmutated-allow.txt on purpose: %v", unmutated)
		})
	})
}
