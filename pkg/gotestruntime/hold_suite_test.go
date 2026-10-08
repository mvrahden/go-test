package gotestruntime_test

import (
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

type HoldTestSuite struct{}

func (s *HoldTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *HoldTestSuite) TestBuildConfig(t *gotest.T) {
	t.When("a config method panics while the hold builds its DAG", func(w *gotest.T) {
		w.It("reports the panic as a setup error instead of letting it escape", func(it *gotest.T) {
			_, err := gotestruntime.ExportBuildConfig(func() gotestruntime.MainConfig { panic("config blew up") })
			gotest.ErrorContains(it, err, "panic: config blew up")
		})
	})

	t.When("the build returns", func(w *gotest.T) {
		w.It("hands its config through", func(it *gotest.T) {
			cfg, err := gotestruntime.ExportBuildConfig(func() gotestruntime.MainConfig {
				return gotestruntime.MainConfig{MaxSuiteSetupTimeout: time.Minute}
			})
			gotest.NoError(it, err)
			gotest.Equal(it, time.Minute, cfg.MaxSuiteSetupTimeout)
		})
	})
}

// Sequential with the rest of the package: the watcher is process-wide.
type HoldWatchTestSuite struct{}

func (s *HoldWatchTestSuite) TestWatcher(t *gotest.T) {
	t.When("a function holds fixtures", func(w *gotest.T) {
		w.It("watches for a stop exactly while the hold lasts", func(it *gotest.T) {
			var during bool
			it.T().Run("holder", func(tt *testing.T) { //nolint:suite-lifecycle // the hold must end with a test
				gotestruntime.HoldFixtures(tt, func() gotestruntime.MainConfig { return gotestruntime.MainConfig{} })
				during = gotestruntime.ExportWatching()
			})
			gotest.True(it, during, "no watcher while the fixtures were held")
			gotest.False(it, gotestruntime.ExportWatching(), "the watcher outlived the hold: goleak would see it")
		})
	})
}
