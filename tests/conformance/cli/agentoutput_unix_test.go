//go:build unix

package cli_test

import (
	"regexp"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// The -exec row runs every suite binary under true, which starts none.
func (s *CLIAgentOutputTestSuite) TestASuiteBinaryThatNeverRunsItsTests(t *gotest.T) {
	noMRun, err := testkit.RunCLIWithEnv(t.Context(), ".", inCache(t.TempDir()), "../testdata/clinomrun/")
	gotest.NoError(t, err)
	execTrue, err := testkit.RunCLIWithEnv(t.Context(), ".", inCache(t.TempDir()), "-exec", "true", "../testdata/passing/")
	gotest.NoError(t, err)

	t.When("a text run's suite binary exits 0 having run nothing, under a TestMain that never calls m.Run or under -exec true", func(w *gotest.T) {
		w.It("fails with exit 2, naming on stderr the suite's declared test as never ran", func(it *gotest.T) {
			for sub, r := range gotest.Each(it, []struct {
				Desc string
				run  testkit.Run
				unit string
			}{
				{Desc: "a TestMain that never calls m.Run", run: noMRun, unit: "/testdata/clinomrun TestNoMRunTestSuite/TestNeverRuns\n"},
				{Desc: "-exec true", run: execTrue, unit: "/testdata/passing TestPassingTestSuite/TestOne\n"},
			}) {
				gotest.Equal(sub, 2, r.run.Exit, "stdout: %s\nstderr: %s", r.run.Stdout, r.run.Stderr)
				gotest.Regexp(sub, `\nFAIL: census: 1 declared test\(s\) never ran\n  \S+`+regexp.QuoteMeta(r.unit), r.run.Stderr)
			}
		})
		w.It("the closing line counts 1 test not run", func(it *gotest.T) {
			for sub, r := range gotest.Each(it, []struct {
				Desc string
				run  testkit.Run
			}{
				{Desc: "a TestMain that never calls m.Run", run: noMRun},
				{Desc: "-exec true", run: execTrue},
			}) {
				gotest.Equal(sub, "gotest: FAIL exit 2 | tests: 1 not run | packages: 0 passed, 1 failed, 0 skipped", lastLine(r.run.Stderr))
			}
		})
	})
}
