package lint_test

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LintExtentTestSuite: a finding covers the code it is about, not only the
// column it starts at.
type LintExtentTestSuite struct{}

func (s *LintExtentTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

func (s *LintExtentTestSuite) TestAFindingsExtent(t *gotest.T) {
	t.When("lint -json reports a finding on a call and one on a function", func(w *gotest.T) {
		w.It("ends each past its posn, at the end of the call and of the function's signature", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "lint", "-json", "../testdata/lintextent/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			var doc map[string]map[string][]struct {
				Category string `json:"category"`
				Posn     string `json:"posn"`
				End      string `json:"end"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &doc), "stdout: %s", run.Stdout)
			got := map[string][2]string{}
			for _, analyzers := range doc {
				for _, f := range analyzers["gotestlint"] {
					got[f.Category] = [2]string{filepath.Base(f.Posn), filepath.Base(f.End)}
				}
			}
			gotest.Equal(it, map[string][2]string{
				// gotest.True(t, a == b) on line 15 from column 2.
				"assertion-simplify": {"extent_suite_test.go:15:2", "extent_suite_test.go:15:24"},
				// func TestPlain(t *testing.T) on line 18.
				"stdlib-test": {"extent_suite_test.go:18:1", "extent_suite_test.go:18:29"},
			}, got)
		})
	})
}
