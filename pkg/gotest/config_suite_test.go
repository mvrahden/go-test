package gotest_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ConfigTestSuite tests config constructors, presets, and overlay logic
// for FixtureConfig and SuiteConfig.
type ConfigTestSuite struct{}

func (s *ConfigTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *ConfigTestSuite) TestDefaultFixtureConfig(t *gotest.T) {
	t.It("returns 2-minute timeout with no retries", func(it *gotest.T) {
		cfg := gotest.DefaultFixtureConfig()
		gotest.Equal(it, 2*time.Minute, cfg.Timeout)
		gotest.Equal(it, 0, cfg.Retries)
		gotest.Equal(it, time.Duration(0), cfg.RetryDelay)
	})
}

func (s *ConfigTestSuite) TestContainerFixtureConfig(t *gotest.T) {
	t.It("returns 5-minute timeout with 1 retry and 5s delay", func(it *gotest.T) {
		cfg := gotest.ContainerFixtureConfig()
		gotest.Equal(it, 5*time.Minute, cfg.Timeout)
		gotest.Equal(it, 1, cfg.Retries)
		gotest.Equal(it, 5*time.Second, cfg.RetryDelay)
	})
}

func (s *ConfigTestSuite) TestDefaultSuiteConfig(t *gotest.T) {
	t.It("returns 30s timeout and 30s setup timeout", func(it *gotest.T) {
		cfg := gotest.DefaultSuiteConfig()
		gotest.Equal(it, 30*time.Second, cfg.Timeout)
		gotest.Equal(it, 30*time.Second, cfg.SetupTimeout)
		gotest.Equal(it, 0, cfg.Retries)
		gotest.False(it, cfg.FailFast)
	})
}

func (s *ConfigTestSuite) TestIntegrationSuiteConfig(t *gotest.T) {
	t.It("returns 2-min timeout and 5-min setup timeout", func(it *gotest.T) {
		cfg := gotest.IntegrationSuiteConfig()
		gotest.Equal(it, 2*time.Minute, cfg.Timeout)
		gotest.Equal(it, 5*time.Minute, cfg.SetupTimeout)
		gotest.Equal(it, 0, cfg.Retries)
		gotest.False(it, cfg.FailFast)
	})
}

func (s *ConfigTestSuite) TestOverlayFixtureConfig(t *gotest.T) {
	t.When("overlay has zero values", func(w *gotest.T) {
		w.It("preserves all defaults", func(it *gotest.T) {
			base := gotest.DefaultFixtureConfig()
			gotest.OverlayFixtureConfig(&base, gotest.FixtureConfig{})
			gotest.Equal(it, 2*time.Minute, base.Timeout)
			gotest.Equal(it, 0, base.Retries)
			gotest.Equal(it, time.Duration(0), base.RetryDelay)
		})
	})

	t.When("overlay has positive values", func(w *gotest.T) {
		w.It("overrides the corresponding fields", func(it *gotest.T) {
			base := gotest.DefaultFixtureConfig()
			gotest.OverlayFixtureConfig(&base, gotest.FixtureConfig{
				Timeout:    10 * time.Second,
				Retries:    3,
				RetryDelay: 1 * time.Second,
			})
			gotest.Equal(it, 10*time.Second, base.Timeout)
			gotest.Equal(it, 3, base.Retries)
			gotest.Equal(it, 1*time.Second, base.RetryDelay)
		})
	})

	t.When("overlay has negative timeout", func(w *gotest.T) {
		w.It("sets negative value to disable timeout", func(it *gotest.T) {
			base := gotest.DefaultFixtureConfig()
			gotest.OverlayFixtureConfig(&base, gotest.FixtureConfig{Timeout: -1})
			gotest.Equal(it, time.Duration(-1), base.Timeout)
		})
	})

	t.When("overlay has partial values", func(w *gotest.T) {
		w.It("only overrides non-zero fields", func(it *gotest.T) {
			base := gotest.DefaultFixtureConfig()
			gotest.OverlayFixtureConfig(&base, gotest.FixtureConfig{Retries: 2})
			gotest.Equal(it, 2*time.Minute, base.Timeout)
			gotest.Equal(it, 2, base.Retries)
			gotest.Equal(it, time.Duration(0), base.RetryDelay)
		})
	})
}

func (s *ConfigTestSuite) TestOverlaySuiteConfig(t *gotest.T) {
	t.When("overlay has zero values", func(w *gotest.T) {
		w.It("preserves all defaults", func(it *gotest.T) {
			base := gotest.DefaultSuiteConfig()
			gotest.OverlaySuiteConfig(&base, gotest.SuiteConfig{})
			gotest.Equal(it, 30*time.Second, base.Timeout)
			gotest.Equal(it, 30*time.Second, base.SetupTimeout)
			gotest.Equal(it, 0, base.Retries)
			gotest.False(it, base.FailFast)
		})
	})

	t.When("overlay has positive values", func(w *gotest.T) {
		w.It("overrides the corresponding fields", func(it *gotest.T) {
			base := gotest.DefaultSuiteConfig()
			gotest.OverlaySuiteConfig(&base, gotest.SuiteConfig{
				Timeout:      1 * time.Minute,
				SetupTimeout: 2 * time.Minute,
				Retries:      5,
				FailFast:     true,
			})
			gotest.Equal(it, 1*time.Minute, base.Timeout)
			gotest.Equal(it, 2*time.Minute, base.SetupTimeout)
			gotest.Equal(it, 5, base.Retries)
			gotest.True(it, base.FailFast)
		})
	})

	t.When("overlay has negative timeout", func(w *gotest.T) {
		w.It("sets negative value to disable timeout", func(it *gotest.T) {
			base := gotest.DefaultSuiteConfig()
			gotest.OverlaySuiteConfig(&base, gotest.SuiteConfig{Timeout: -1})
			gotest.Equal(it, time.Duration(-1), base.Timeout)
		})
	})

	t.When("overlay has negative setup timeout", func(w *gotest.T) {
		w.It("sets negative value to disable setup timeout", func(it *gotest.T) {
			base := gotest.DefaultSuiteConfig()
			gotest.OverlaySuiteConfig(&base, gotest.SuiteConfig{SetupTimeout: -1})
			gotest.Equal(it, time.Duration(-1), base.SetupTimeout)
		})
	})

	t.When("SetupTimeout is overlaid independently", func(w *gotest.T) {
		w.It("does not affect Timeout", func(it *gotest.T) {
			base := gotest.DefaultSuiteConfig()
			gotest.OverlaySuiteConfig(&base, gotest.SuiteConfig{SetupTimeout: 5 * time.Minute})
			gotest.Equal(it, 30*time.Second, base.Timeout)
			gotest.Equal(it, 5*time.Minute, base.SetupTimeout)
		})
	})

	t.When("FailFast overlay is false", func(w *gotest.T) {
		w.It("does not override a true base", func(it *gotest.T) {
			base := gotest.SuiteConfig{FailFast: true}
			gotest.OverlaySuiteConfig(&base, gotest.SuiteConfig{FailFast: false})
			gotest.True(it, base.FailFast)
		})
	})

	t.When("overlay has partial values", func(w *gotest.T) {
		w.It("only overrides non-zero fields", func(it *gotest.T) {
			base := gotest.DefaultSuiteConfig()
			gotest.OverlaySuiteConfig(&base, gotest.SuiteConfig{FailFast: true})
			gotest.Equal(it, 30*time.Second, base.Timeout)
			gotest.Equal(it, 30*time.Second, base.SetupTimeout)
			gotest.Equal(it, 0, base.Retries)
			gotest.True(it, base.FailFast)
		})
	})
}
