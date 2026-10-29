package lifecycle_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// TimeoutScaleTestSuite: GOTEST_TIMEOUT_SCALE multiplies the budgets suites
// and fixtures run under.
type TimeoutScaleTestSuite struct{}

func (s *TimeoutScaleTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *TimeoutScaleTestSuite) TestBudgetsScaledForASlowMachine(t *gotest.T) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"GOTEST_TIMEOUT_SCALE=2"}, "../testdata/timeoutscale/")
	gotest.NoError(t, err)

	t.When("GOTEST_TIMEOUT_SCALE is 2 and a suite declares a 1s Timeout", func(w *gotest.T) {
		w.It("passes a method that runs for 1.5s", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestBudgetedTestSuite/TestRunsHalfPastItsTimeout"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("fails a method that runs past 2s by a 2s Timeout", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestBudgetedTestSuite/TestRunsPastTwiceItsTimeout"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout, "exceeded its configured Timeout of 2s")
		})
		w.It("leaves the scale out of the environment of a process a test starts", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestBudgetedTestSuite/TestStartsAProcess"), "stdout: %s", run.Stdout)
		})
	})

	t.When("GOTEST_TIMEOUT_SCALE is 2 and a fixture declares a 1s Timeout", func(w *gotest.T) {
		w.It("sets up a fixture whose BeforeAll takes 1.5s", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestFixtureBoundTestSuite/TestRuns"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("fails a fixture whose BeforeAll runs past 2s by a 2s Timeout", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestSlowerFixtureBoundTestSuite"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout+run.Stderr, "SlowerFixture.BeforeAll exceeded its configured Timeout of 2s")
		})
	})
}

func (s *TimeoutScaleTestSuite) TestAScaleThatIsNoNumber(t *gotest.T) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"GOTEST_TIMEOUT_SCALE=fast"}, "../testdata/timeoutscale/")
	gotest.NoError(t, err)
	named := map[string]bool{}
	for _, ev := range run.Events {
		if ev.Action == "output" && strings.Contains(ev.Output, `GOTEST_TIMEOUT_SCALE="fast" is not a number above zero`) {
			named[ev.Test] = true
		}
	}

	t.When("GOTEST_TIMEOUT_SCALE is not a number above zero", func(w *gotest.T) {
		w.It("fails every suite before its fixtures, naming the value", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestBudgetedTestSuite"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "fail", run.Verdict("TestFixtureBoundTestSuite"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "fail", run.Verdict("TestSlowerFixtureBoundTestSuite"), "stdout: %s", run.Stdout)
			gotest.Equal(it, map[string]bool{"TestBudgetedTestSuite": true, "TestFixtureBoundTestSuite": true, "TestSlowerFixtureBoundTestSuite": true}, named, "stdout: %s", run.Stdout)
			gotest.Equal(it, 1, run.Exit)
		})
	})
}
