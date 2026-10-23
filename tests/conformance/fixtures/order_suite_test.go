package fixtures_test

import (
	"regexp"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureOrderTestSuite: a suite's package fixtures run their per-method
// hooks in dependency order, and which of them run when one goes wrong.
type FixtureOrderTestSuite struct{}

func (s *FixtureOrderTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
}

// failureLine matches a located failure line of a test's output.
var failureLine = regexp.MustCompile(`(?m)^\s+\S+\.go:\d+: `)

func (s *FixtureOrderTestSuite) TestHooksInDependencyOrder(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixturepairs/", "-run=^Test(Overrun|Ahead|Behind)TestSuite$")
	gotest.NoError(t, err)
	gotest.Equal(t, 1, run.Exit, "stderr: %s", run.Stderr)

	t.When("a fixture's BeforeEach overruns the Timeout ahead of another fixture's", func(w *gotest.T) {
		w.It("starts no later fixture's BeforeEach, fails the method once, at the hook that overran, and runs that fixture's AfterEach", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestOverrunTestSuite/TestBody"))
			gotest.Contains(it, run.Markers, "slow.BeforeEach")
			gotest.NotContains(it, run.Markers, "later.BeforeEach")
			gotest.NotContains(it, run.Markers, "overrun.body")
			gotest.Contains(it, run.Markers, "slow.AfterEach")
			out := outputOf(&run, "TestOverrunTestSuite/TestBody")
			gotest.Len(it, failureLine.FindAllString(out, -1), 1, out)
			gotest.Regexp(it, `(?m)^\s+fixture\.go:`+lineOf(it, "fixturepairs/fixture.go", "time.Sleep(time.Second)")+`: SlowFixture\.BeforeEach exceeded its configured Timeout`, out)
		})
	})

	t.When("a fixture with only an AfterEach comes before a fixture whose BeforeEach fails", func(w *gotest.T) {
		w.It("runs its AfterEach, as every BeforeEach before it returned", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestAheadTestSuite/TestBody"))
			gotest.NotContains(it, run.Markers, "ahead.body")
			gotest.Contains(it, run.Markers, "ahead.AfterEach")
		})
	})

	t.When("a fixture with only an AfterEach comes after a fixture whose BeforeEach fails", func(w *gotest.T) {
		w.It("does not run its AfterEach, as its turn never came", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestBehindTestSuite/TestBody"))
			gotest.NotContains(it, run.Markers, "behind.body")
			gotest.NotContains(it, run.Markers, "behind.AfterEach")
		})
	})
}
