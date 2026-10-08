package gotestruntime_test

import (
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
