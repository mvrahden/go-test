package gotestruntime_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

type DeprecatedTestSuite struct{}

func (s *DeprecatedTestSuite) TestAnOlderHarness(t *gotest.T) {
	t.When("a harness from before HoldFixtures runs against this runtime", func(w *gotest.T) {
		w.It("fails its setup with the fix instead of a missing symbol", func(it *gotest.T) {
			var once gotestruntime.FixtureOnce //nolint:staticcheck // the older harness's shape
			err := once.Do(func() error {
				gotestruntime.CountMatchingTests([]string{"TestSuite"}) //nolint:staticcheck // the older harness's shape
				return nil
			})
			gotest.ErrorContains(it, err, "run `gotest generate` again")
		})
	})
}
