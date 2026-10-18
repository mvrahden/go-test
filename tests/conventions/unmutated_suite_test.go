package conventions_test

import (
	"os"
	"path/filepath"
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
	unmutated, unknown, err := speccatalog.Unmutated(clauses, filepath.Join(root, "tests", "drill", "mutants"))
	gotest.NoError(t, err)

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
