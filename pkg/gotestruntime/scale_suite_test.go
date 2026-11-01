package gotestruntime_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// ScaleDurationTestSuite: a test's own wall-clock bound scales as the
// harness scales a budget.
// Sequential: each method swaps the package's scale reader.
type ScaleDurationTestSuite struct{}

func (s *ScaleDurationTestSuite) TestScalesAsBudgetsScale(t *gotest.T) {
	restore := gotestruntime.ExportSetTimeoutScale(2.5)
	defer restore()
	t.When("GOTEST_TIMEOUT_SCALE is 2.5", func(w *gotest.T) {
		w.It("multiplies a positive duration", func(it *gotest.T) {
			gotest.Equal(it, 5*time.Second, gotestruntime.ScaleDuration(2*time.Second))
		})
		w.It("leaves zero and negative durations as they are", func(it *gotest.T) {
			gotest.Zero(it, gotestruntime.ScaleDuration(0))
			gotest.Equal(it, gotest.NoDeadline, gotestruntime.ScaleDuration(gotest.NoDeadline))
		})
	})
}

func (s *ScaleDurationTestSuite) TestUnscaledByDefault(t *gotest.T) {
	restore := gotestruntime.ExportSetTimeoutScale(1)
	defer restore()
	t.When("GOTEST_TIMEOUT_SCALE is unset", func(w *gotest.T) {
		w.It("returns the duration unchanged", func(it *gotest.T) {
			gotest.Equal(it, 3*time.Second, gotestruntime.ScaleDuration(3*time.Second))
		})
	})
}
