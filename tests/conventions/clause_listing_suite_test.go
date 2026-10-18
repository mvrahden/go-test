package conventions_test

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ClauseListingTestSuite: the clause listing the drill reads is the
// scenarios of specs/, one "when <WHEN> → <THEN>" line per THEN and AND.
type ClauseListingTestSuite struct{}

func (s *ClauseListingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func listing(t *gotest.T, root string, args ...string) []string {
	cmd := exec.CommandContext(t.Context(), "go", append([]string{"run", "./internal/speccatalog/clauses"}, args...)...) //nolint:gosec // G204: the repository's own command
	cmd.Dir = root
	out, err := cmd.Output()
	gotest.NoError(t, err)
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}

func (s *ClauseListingTestSuite) TestListingIsTheScenarios(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	specs, err := speccatalog.LoadSpecs(root)
	gotest.NoError(t, err)
	var want []string
	for _, sp := range specs {
		for _, r := range sp.Requirements {
			for _, sc := range r.Scenarios {
				if len(sc.Whens) != 1 {
					continue
				}
				for _, then := range sc.Thens {
					want = append(want, "when "+strings.TrimPrefix(sc.Whens[0], "when ")+" → "+then)
				}
			}
		}
	}
	got := listing(t, root)
	t.When("the clause listing is compared with the scenarios in specs/", func(w *gotest.T) {
		w.It("holds each WHEN and THEN pair once", func(it *gotest.T) {
			gotest.ElementsMatch(it, want, got)
		})
	})
}

func (s *ClauseListingTestSuite) TestUnmutatedListingIsTheBudgetReading(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	specs, err := speccatalog.LoadSpecs(root)
	gotest.NoError(t, err)
	unmutated, _, err := speccatalog.Unmutated(speccatalog.Clauses(specs), filepath.Join(root, "tests", "drill", "mutants"))
	gotest.NoError(t, err)
	got := listing(t, root, "--unmutated")
	t.When("the unmutated listing is compared with the reading the budget is held to", func(w *gotest.T) {
		w.It("lists the same clauses", func(it *gotest.T) {
			gotest.Equal(it, unmutated, got)
		})
	})
}
