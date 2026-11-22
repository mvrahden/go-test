package cli_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLINoTestsTestSuite: a suite process that ran none of its tests fails a
// text run, and the suites that run nothing on purpose do not.
type CLINoTestsTestSuite struct{}

func (s *CLINoTestsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute} // six CLI runs in one method
}

func (s *CLINoTestsTestSuite) TestSuitesThatLegitimatelyRunNothing(t *gotest.T) {
	t.When("a text run covers a guarded suite, a suite whose methods are all excluded, a run narrowed by focus and a -run that matches nothing", func(w *gotest.T) {
		w.It("passes each, the last noted as no tests to run", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "a guarded suite and one whose methods are all excluded", args: []string{"../testdata/clinotests/"}, exit: 0, stdoutHas: "ok"},
				{Desc: "a run narrowed by focus", args: []string{"../testdata/focused/"}, exit: 0, stdoutHas: "ok"},
				{Desc: "a -run that matches nothing", args: []string{"-run", "NothingMatches", "../testdata/passing/"}, exit: 0, stdoutHas: "[no tests to run]"},
			}) {
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", []string{"GOTEST_CI=0"}, row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Stdout, row.stdoutHas)
				gotest.NotContains(sub, run.Stderr, "ran no tests")
			}
		})
	})
}

func (s *CLINoTestsTestSuite) TestTheBinarysOwnSelectionFlags(t *gotest.T) {
	t.When("a text run gives the binary -test.count=0, -test.run, -test.skip, -test.list or -test.bench after -args", func(w *gotest.T) {
		w.It("passes each, never naming a suite as one that ran no tests", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "-test.count=0", args: []string{"../testdata/passing/", "-args", "-test.count=0"}},
				{Desc: "-test.count 0", args: []string{"../testdata/passing/", "-args", "-test.count", "0"}},
				{Desc: "-test.run", args: []string{"../testdata/passing/", "-args", "-test.run=NothingMatches"}},
				{Desc: "-test.skip", args: []string{"../testdata/passing/", "-args", "-test.skip=."}},
				{Desc: "-test.list", args: []string{"../testdata/passing/", "-args", "-test.list=."}},
				{Desc: "-test.bench", args: []string{"../testdata/passing/", "-args", "-test.bench=."}},
			}) {
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", []string{"GOTEST_CI=0"}, row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Stdout, "ok")
				gotest.NotContains(sub, run.Stderr, "ran no tests")
			}
		})
	})
}
