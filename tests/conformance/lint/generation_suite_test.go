package lint_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterGenerationTestSuite: what the generator rejects, lint reports and
// discover marks broken, at the place the generator rejects it.
type LinterGenerationTestSuite struct{}

func (s *LinterGenerationTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

const genRejectPkg = "../testdata/lintgenreject/"

// genRejections are the fixture's rejections: file:line:col and message.
var genRejections = []struct{ posn, message string }{
	{"value_suite_test.go:18:2", "suite ValueTestSuite has fixture field Store of value type StoreFixture; a fixture is bound through a pointer field (*StoreFixture)"},
	{"result_suite_test.go:8:27", "TestLoads on ResultTestSuite returns a value; a Test method returns nothing"},
}

type lintDiagnostic struct {
	Posn     string `json:"posn"`
	Message  string `json:"message"`
	Category string `json:"category"`
}

func (s *LinterGenerationTestSuite) TestGenerationRejects(t *gotest.T) {
	t.When("a package holds shapes the generator rejects", func(w *gotest.T) {
		w.It("lint reports each shape the generator rejects at the position it rejects it", func(it *gotest.T) {
			plain, err := testkit.RunCLI(it.Context(), ".", "lint", genRejectPkg)
			gotest.NoError(it, err)
			gotest.NotEqual(it, 0, plain.Exit, "stderr: %s", plain.Stderr)
			for _, r := range genRejections {
				gotest.Contains(it, plain.Stderr, r.posn+": "+r.message)
			}

			js, err := testkit.RunCLI(it.Context(), ".", "lint", "-json", genRejectPkg)
			gotest.NoError(it, err)
			var byPkg map[string]map[string][]lintDiagnostic
			gotest.NoError(it, json.Unmarshal([]byte(js.Stdout), &byPkg), "stdout: %s\nstderr: %s", js.Stdout, js.Stderr)
			var got []string
			for _, analyzers := range byPkg {
				for _, d := range analyzers["gotestlint"] {
					if d.Category == "generation-rejects" {
						got = append(got, filepath.Base(d.Posn)+": "+d.Message)
					}
				}
			}
			for _, r := range genRejections {
				gotest.Contains(it, got, r.posn+": "+r.message)
			}
		})
		w.It("discover marks the package broken and names each rejection at its position", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "discover", genRejectPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			var out struct {
				Packages []struct {
					ImportPath string `json:"importPath"`
					Broken     bool   `json:"broken"`
				} `json:"packages"`
				Warnings []struct {
					ImportPath string `json:"importPath"`
					File       string `json:"file"`
					Line       int    `json:"line"`
					Col        int    `json:"col"`
					Message    string `json:"message"`
				} `json:"warnings"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &out), "stdout: %s", run.Stdout)
			gotest.Len(it, out.Packages, 1, "stdout: %s", run.Stdout)
			gotest.Regexp(it, "/lintgenreject$", out.Packages[0].ImportPath)
			gotest.True(it, out.Packages[0].Broken, "stdout: %s", run.Stdout)
			var got []string
			for _, wn := range out.Warnings {
				got = append(got, fmt.Sprintf("%s:%d:%d: %s", wn.File, wn.Line, wn.Col, wn.Message))
			}
			for _, r := range genRejections {
				gotest.Contains(it, got, r.posn+": "+r.message)
			}
		})
	})
}

func (s *LinterGenerationTestSuite) TestAnotherFindingOnTheLine(t *gotest.T) {
	t.When("a skippable rule reports on a line the generator rejects, shown or suppressed by a nolint comment", func(w *gotest.T) {
		w.It("lint still reports the generator's rejection on that line", func(it *gotest.T) {
			got := findingsOf(ruleFindings(it, ".", "../testdata/lintgenshown/skippable/"), "generation-rejects ")
			gotest.ElementsMatch(it, []string{
				"generation-rejects shown_suite_test.go:8:25: fuzz method StubTestSuite.FuzzShown never calls f.Fuzz",
				"generation-rejects shown_suite_test.go:10:25: fuzz method StubTestSuite.FuzzQuiet never calls f.Fuzz",
			}, got)
		})
		w.It("lint leaves the rejection to an integrity rule whose finding is shown on that line", func(it *gotest.T) {
			gotest.Equal(it, []string{"receiver held_suite_test.go:9:1: suite method HeldTestSuite.TestByValue should use a pointer receiver"},
				onLine(ruleFindings(it, ".", "../testdata/lintgenshown/integrity/"), "held_suite_test.go:9:"))
		})
	})
}
