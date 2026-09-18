package gotest_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ConfigTestSuite tests config constructors, presets, and overlay logic
// for FixtureConfig and SuiteConfig.
type ConfigTestSuite struct{}

func (s *ConfigTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
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
		gotest.False(it, cfg.FailFast)
	})
}

func (s *ConfigTestSuite) TestIntegrationSuiteConfig(t *gotest.T) {
	t.It("returns 2-min timeout and 5-min setup timeout", func(it *gotest.T) {
		cfg := gotest.IntegrationSuiteConfig()
		gotest.Equal(it, 2*time.Minute, cfg.Timeout)
		gotest.Equal(it, 5*time.Minute, cfg.SetupTimeout)
		gotest.False(it, cfg.FailFast)
	})
}

func (s *ConfigTestSuite) TestLiteralSemantics(t *gotest.T) {
	t.When("a config is composed from a preset", func(w *gotest.T) {
		w.It("keeps preset values and applies overrides as-is", func(it *gotest.T) {
			cfg := gotest.DefaultSuiteConfig()
			cfg.Parallel = true
			gotest.Equal(it, 30*time.Second, cfg.Timeout)
			gotest.Equal(it, 30*time.Second, cfg.SetupTimeout)
			gotest.True(it, cfg.Parallel)
		})
	})

	t.When("a duration is NoDeadline", func(w *gotest.T) {
		w.It("is negative, which every use site reads as no deadline", func(it *gotest.T) {
			gotest.Less(it, gotest.NoDeadline, time.Duration(0), "use sites gate a deadline on > 0")
		})
	})
}
