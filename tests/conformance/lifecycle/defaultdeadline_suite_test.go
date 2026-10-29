package lifecycle_test

import (
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DefaultDeadlineTestSuite: a context the default deadline ended is named
// once, and the line fails nothing.
type DefaultDeadlineTestSuite struct{}

func (s *DefaultDeadlineTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 90 * time.Second}
}

func (s *DefaultDeadlineTestSuite) TestAContextTheDefaultDeadlineEnded(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/defaultdeadline/")
	gotest.NoError(t, err)
	named, lines := map[string]int{}, []string{}
	for _, ev := range run.Events {
		if ev.Action == "output" && strings.Contains(ev.Output, "t.Context() ended at the default ") {
			named[ev.Test]++
			lines = append(lines, strings.TrimSpace(ev.Output))
		}
	}

	t.When("a method of a suite that declares no Timeout waits on t.Context() past the default 30s deadline", func(w *gotest.T) {
		w.It("prints one line naming the method and the default deadline", func(it *gotest.T) {
			gotest.Equal(it, map[string]int{"TestWaitingTestSuite/TestWaitsOnItsContext": 1}, named, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, []string{"TestWaitingTestSuite/TestWaitsOnItsContext: t.Context() ended at the default 30s deadline; the suite declares no Timeout"}, lines)
		})
		w.It("leaves the method's verdict as it is", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestWaitingTestSuite/TestWaitsOnItsContext"), "stdout: %s", run.Stdout)
		})
	})

	t.When("a suite declares a Timeout and its method waits on t.Context() past it", func(w *gotest.T) {
		w.It("fails the method by the Timeout and prints no line about the default deadline", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestBudgetedTestSuite/TestWaitsOnItsContext"), "stdout: %s", run.Stdout)
			gotest.Zero(it, named["TestBudgetedTestSuite/TestWaitsOnItsContext"], "stdout: %s", run.Stdout)
		})
	})
}
