package lifecycle_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DeadlineStopEndedTestSuite: a method that returns once an expired
// --timeout ends its context did not pass.
type DeadlineStopEndedTestSuite struct{}

func (s *DeadlineStopEndedTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

const stopEndedPkg = "../testdata/clitimeoutctx/"

func (s *DeadlineStopEndedTestSuite) TestAMethodThatHonoursItsContext(t *gotest.T) {
	t.When("a run's --timeout ends a method that honours its context", func(w *gotest.T) {
		// The warm run builds the binary, so the limit times the method alone.
		warm, err := testkit.RunPackageWithEnv(w.Context(), w.TempDir(), []string{"CLITIMEOUT_WARM=1"}, stopEndedPkg)
		gotest.NoError(w, err)
		gotest.Equal(w, 0, warm.Exit, "stdout: %s\nstderr: %s", warm.Stdout, warm.Stderr)
		limit := "--timeout=" + testkit.Scaled(5*time.Second).String()
		stream, err := testkit.RunPackage(w.Context(), w.TempDir(), stopEndedPkg, limit)
		gotest.NoError(w, err)
		spec, err := testkit.RunCLI(w.Context(), ".", "spec", "--no-color", limit, stopEndedPkg)
		gotest.NoError(w, err)

		w.It("the method is booked failed in -json, and spec shows it failed", func(it *gotest.T) {
			gotest.Contains(it, outputOf(&stream, "TestWaitsTestSuite/TestWaits"), "run stopping: the stop came while this method ran", "stream:\n%s", stream.Stdout)
			gotest.Equal(it, "fail", stream.Verdict("TestWaitsTestSuite/TestWaits"), "stream:\n%s", stream.Stdout)
			gotest.Equal(it, 1, spec.Exit, "stdout: %s\nstderr: %s", spec.Stdout, spec.Stderr)
			gotest.Contains(it, spec.Stdout, "✗ Waits", "stderr: %s", spec.Stderr)
		})
	})
}
