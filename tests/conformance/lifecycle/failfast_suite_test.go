package lifecycle_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FailFastTestSuite: after the first failure a FailFast suite reports the
// methods it did not run, naming the method that stopped it.
type FailFastTestSuite struct{}

func (s *FailFastTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *FailFastTestSuite) TestTheRemainingMethodsAreReported(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/failfast/")
	gotest.NoError(t, err)
	t.When("the first method of a FailFast suite fails", func(w *gotest.T) {
		w.It("fails the method", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestFailFastTestSuite/TestFirst"))
		})
		w.It("fails the run", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("skips every method that has not started, with the failing method as the reason", func(it *gotest.T) {
			for _, method := range []string{"TestSecond", "TestThird"} {
				path := "TestFailFastTestSuite/" + method
				gotest.Equal(it, "skip", run.Verdict(path), "verdict of %s", path)
				var output strings.Builder
				for _, ev := range run.Events {
					if ev.Test == path && ev.Action == "output" {
						output.WriteString(ev.Output)
					}
				}
				gotest.Contains(it, output.String(), "FailFast: TestFirst")
				gotest.NotContains(it, output.String(), "methods.go:")
			}
		})
	})
}

func (s *FailFastTestSuite) TestASoftFailureBeforeTheMethods(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/failfastsoft/")
	gotest.NoError(t, err)
	t.When("BeforeAll of a FailFast suite records a failure without stopping and no method fails", func(w *gotest.T) {
		w.It("runs every method", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestSoftFailFastTestSuite/TestFirst"))
			gotest.Equal(it, "pass", run.Verdict("TestSoftFailFastTestSuite/TestSecond"))
		})
	})
}

func (s *FailFastTestSuite) TestACleanupFailure(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/failfastcleanup/", "-parallel=1")
	gotest.NoError(t, err)
	t.When("a parallel method of a FailFast suite fails only in a cleanup, the methods running one at a time", func(w *gotest.T) {
		w.It("skips the method that has not started, with the failing method as the reason", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			verdicts := []string{run.Verdict("TestCleanupFailFastTestSuite/TestA"), run.Verdict("TestCleanupFailFastTestSuite/TestB")}
			gotest.ElementsMatch(it, []string{"fail", "skip"}, verdicts)
			first, skipped := "TestA", "TestCleanupFailFastTestSuite/TestB"
			if verdicts[0] == "skip" {
				first, skipped = "TestB", "TestCleanupFailFastTestSuite/TestA"
			}
			var output strings.Builder
			for _, ev := range run.Events {
				if ev.Test == skipped && ev.Action == "output" {
					output.WriteString(ev.Output)
				}
			}
			gotest.Contains(it, output.String(), "FailFast: "+first)
		})
	})
}
