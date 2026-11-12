package spec_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecRepeatTestSuite: a suite run more than once reads as its runs, each
// with its own verdicts, never as an external twin.
type SpecRepeatTestSuite struct{}

func (s *SpecRepeatTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const repeatPkg = "../testdata/specrepeat/"

// repeatRuns are the top-level nodes of the repeated suite, in order.
func repeatRuns(doc specDoc) []specNode {
	var out []specNode
	for _, n := range doc.Packages[0].Nodes {
		if n.Name == "TestRepeatTestSuite" {
			out = append(out, n)
		}
	}
	return out
}

func (s *SpecRepeatTestSuite) TestAMethodThatFailsInOneRunOfThree(t *gotest.T) {
	t.When("a suite is run three times and one of its methods fails in the second", func(w *gotest.T) {
		w.It("the spec shows three suites marked run 1, 2 and 3 of 3, none external", func(it *gotest.T) {
			doc := specJSON(it, 1, repeatPkg, "-count=3")
			runs := repeatRuns(doc)
			gotest.Len(it, runs, 3)
			for i, n := range runs {
				gotest.False(it, n.External, "run %d", i+1)
				gotest.Equal(it, i+1, n.Run)
				gotest.Equal(it, 3, n.Runs)
			}
			terminal := spec(it, 1, repeatPkg, "-count=3")
			gotest.Contains(it, terminal.Stdout, "Repeat — RUN 1 OF 3")
			gotest.Contains(it, terminal.Stdout, "Repeat — RUN 2 OF 3")
			gotest.Contains(it, terminal.Stdout, "Repeat — RUN 3 OF 3")
			gotest.NotContains(it, terminal.Stdout, "EXTERNAL")
		})
		w.It("shows the method failed, with its message, in the second run and passed in the others", func(it *gotest.T) {
			doc := specJSON(it, 1, repeatPkg, "-count=3")
			runs := repeatRuns(doc)
			gotest.Len(it, runs, 3)
			var statuses []string
			for _, n := range runs {
				statuses = append(statuses, node(it, n.Children, "TestFailsOnItsSecondRun").Status)
			}
			gotest.Equal(it, []string{"pass", "fail", "pass"}, statuses)
			terminal := spec(it, 1, repeatPkg, "-count=3")
			gotest.Contains(it, terminal.Stdout, "second run")
		})
		w.It("the summary names the failure with its run", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "summary", "--no-color", repeatPkg, "-count=3")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stdout, "Repeat (run 2 of 3) / FailsOnItsSecondRun")
		})
	})
}

func (s *SpecRepeatTestSuite) TestASuiteOfBothTestPackagesRunTwice(t *gotest.T) {
	t.When("a suite of both test packages is run twice", func(w *gotest.T) {
		w.It("each run shows the internal suite and the external one, marked", func(it *gotest.T) {
			doc := specJSON(it, 0, "../testdata/specslashes/", "-count=2")
			gotest.Equal(it, []bool{false, true, false, true}, suiteFlags(doc))
		})
	})
}
