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
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FailFastTestSuite) TestTheRemainingMethodsAreReported(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/failfast/")
	gotest.NoError(t, err)
	t.When("the first method of a FailFast suite fails", func(w *gotest.T) {
		w.It("fails the method and the run", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "fail", run.Verdict("TestFailFastTestSuite/TestFirst"))
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
			}
		})
	})
}
