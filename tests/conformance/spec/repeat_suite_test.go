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
func repeatRuns(doc specDoc) []specNode { //nolint:gocritic // hugeParam: a test helper
	var out []specNode
	for _, n := range doc.Packages[0].Nodes { //nolint:gocritic // rangeValCopy: a few rows of a test
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
			for i, n := range runs { //nolint:gocritic // rangeValCopy: a few rows of a test
				gotest.False(it, n.External, "run %d", i+1)
				gotest.Equal(it, int32(i+1), n.Run)
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
			for _, n := range runs { //nolint:gocritic // rangeValCopy: a few rows of a test
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

// stdTwins are the TestFoo nodes of a replayed specstdtwin stream, in order.
func stdTwins(doc specDoc) []specNode { //nolint:gocritic // hugeParam: a test helper
	var out []specNode
	for _, n := range doc.Packages[0].Nodes { //nolint:gocritic // rangeValCopy: a few rows of a test
		if n.Name == "TestFoo" {
			out = append(out, n)
		}
	}
	return out
}

func (s *SpecRepeatTestSuite) TestAStdlibTestOfBothTestPackages(t *gotest.T) {
	const once, twice = "../testdata/specstdtwin/once.jsonl", "../testdata/specstdtwin/twice.jsonl"
	t.When("a saved stream holds one run of a stdlib test both test packages declare", func(w *gotest.T) {
		w.It("the spec shows two tests, the external one marked, neither marked as a run", func(it *gotest.T) {
			doc := specJSON(it, 1, "--input="+once)
			nodes := stdTwins(doc)
			gotest.Len(it, nodes, 2)
			gotest.False(it, nodes[0].External)
			gotest.True(it, nodes[1].External)
			gotest.Zero(it, nodes[0].Runs)
			gotest.Zero(it, nodes[1].Runs)
			terminal := spec(it, 1, "--input="+once)
			gotest.Contains(it, terminal.Stdout, "Foo — EXTERNAL")
			gotest.NotContains(it, terminal.Stdout, "RUN 2 OF 2")
		})
		w.It("under -count=2 each run shows the internal test and the external one, marked", func(it *gotest.T) {
			doc := specJSON(it, 1, "--input="+twice)
			nodes := stdTwins(doc)
			gotest.Len(it, nodes, 4)
			var external []bool
			var runs []int32
			for _, n := range nodes { //nolint:gocritic // rangeValCopy: a few rows of a test
				external = append(external, n.External)
				runs = append(runs, n.Run)
				gotest.Equal(it, 2, n.Runs)
			}
			gotest.Equal(it, []bool{false, true, false, true}, external)
			gotest.Equal(it, []int32{1, 1, 2, 2}, runs)
			terminal := spec(it, 1, "--input="+twice)
			gotest.Contains(it, terminal.Stdout, "Foo — EXTERNAL — RUN 2 OF 2")
		})
	})
}
