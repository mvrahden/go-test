package gotestruntime_test

import (
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

type DeprecatedTestSuite struct{}

func (s *DeprecatedTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

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

func (s *DeprecatedTestSuite) TestAHarnessThatAwaitsDone(t *gotest.T) {
	for t, call := range gotest.Each(t, []struct {
		Desc string
		run  func(*gotest.T)
	}{
		{Desc: "AwaitDoneAt", run: func(t *gotest.T) {
			gotestruntime.AwaitDoneAt(t, "TestPingAsync", "ping_test.go:7", func(func()) {}) //nolint:staticcheck // the older harness's shape
		}},
		{Desc: "AwaitDone", run: func(t *gotest.T) {
			gotestruntime.AwaitDone(t, "TestPingAsync", func(func()) {}) //nolint:staticcheck // the older harness's shape
		}},
	}) {
		got := fmt.Sprint(gotest.Panics(t, func() { call.run(t) }))
		gotest.Contains(t, got, "TestPingAsync")
		gotest.Contains(t, got, "Run `gotest generate` again")
		gotest.Contains(t, got, "make(chan struct{})")
	}
}
