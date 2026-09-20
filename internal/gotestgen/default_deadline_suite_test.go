package gotestgen_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DefaultDeadlineTestSuite runs generated harnesses whose markers leave
// durations at zero and reads the deadlines each lifecycle phase received.
type DefaultDeadlineTestSuite struct{}

func (s *DefaultDeadlineTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *DefaultDeadlineTestSuite) TestSuiteMarker(t *gotest.T) {
	run := runGeneratedSuite(t, "TestLifecycle_ZeroDurations")

	t.It("passes", func(it *gotest.T) {
		assertNoDeadlock(it, run)
		gotest.True(it, run.passed, run.output)
	})

	t.When("a suite's marker leaves its durations at zero", func(w *gotest.T) {
		w.It("gets the same deadlines as a suite without a marker", func(it *gotest.T) {
			for _, phase := range []string{"beforeall", "test", "afterall"} {
				gotest.Contains(it, run.output, "MARK:nomarker "+phase+" deadline 30s", run.output)
				gotest.Contains(it, run.output, "MARK:partial "+phase+" deadline 30s", run.output)
			}
		})
	})

	t.When("a suite's marker sets NoDeadline", func(w *gotest.T) {
		w.It("gets no deadline in any phase", func(it *gotest.T) {
			for _, phase := range []string{"beforeall", "test", "afterall"} {
				gotest.Contains(it, run.output, "MARK:nodeadline "+phase+" no deadline", run.output)
			}
		})
	})
}

func (s *DefaultDeadlineTestSuite) TestFixtureMarker(t *gotest.T) {
	t.When("a fixture's marker leaves its Timeout at zero", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_ZeroDurationsFixture")

		w.It("passes", func(it *gotest.T) {
			assertNoDeadlock(it, run)
			gotest.True(it, run.passed, run.output)
		})

		w.It("gets the 2m default deadline", func(it *gotest.T) {
			gotest.Contains(it, run.output, "MARK:partial fixture beforeall deadline 2m0s", run.output)
			gotest.Contains(it, run.output, "MARK:partial fixture afterall deadline 2m0s", run.output)
		})

		w.It("leaves a NoDeadline fixture unbounded", func(it *gotest.T) {
			gotest.Contains(it, run.output, "MARK:nodeadline fixture beforeall no deadline", run.output)
			gotest.Contains(it, run.output, "MARK:nodeadline fixture afterall no deadline", run.output)
		})

		w.It("gives the bound suite's zero durations the suite defaults", func(it *gotest.T) {
			gotest.Contains(it, run.output, "MARK:bound beforeall deadline 30s", run.output)
			gotest.Contains(it, run.output, "MARK:bound test deadline 30s", run.output)
		})
	})
}
