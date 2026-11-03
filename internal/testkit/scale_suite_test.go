package testkit_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// ScaledTestSuite: a body's wall-clock bound follows the run's timeout scale.
type ScaledTestSuite struct{}

func (s *ScaledTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *ScaledTestSuite) TestFollowsTheRuntimeScale(t *gotest.T) {
	t.When("a bound is scaled", func(w *gotest.T) {
		w.It("is the duration as the runtime scales a budget", func(it *gotest.T) {
			gotest.Equal(it, gotestruntime.ScaleDuration(7*time.Second), testkit.Scaled(7*time.Second))
		})
		w.It("leaves zero and negative durations as they are", func(it *gotest.T) {
			gotest.Zero(it, testkit.Scaled(0))
			gotest.Equal(it, gotest.NoDeadline, testkit.Scaled(gotest.NoDeadline))
		})
	})
}
