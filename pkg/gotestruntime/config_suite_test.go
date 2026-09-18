package gotestruntime_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// ConfigDefaultsTestSuite pins how a marker's zero durations resolve.
type ConfigDefaultsTestSuite struct{}

func (s *ConfigDefaultsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ConfigDefaultsTestSuite) TestWithSuiteDefaults(t *gotest.T) {
	type row struct {
		Desc     string
		declared gotest.SuiteConfig
		want     gotest.SuiteConfig
	}
	for it, tc := range gotest.Each(t, []row{
		{Desc: "an empty marker runs like no marker", declared: gotest.SuiteConfig{}, want: gotest.DefaultSuiteConfig()},
		{
			Desc:     "a partial literal keeps its booleans and takes the default durations",
			declared: gotest.SuiteConfig{FailFast: true, Parallel: true, Exclusive: true},
			want:     gotest.SuiteConfig{Timeout: 30 * time.Second, SetupTimeout: 30 * time.Second, FailFast: true, Parallel: true, Exclusive: true},
		},
		{
			Desc:     "a declared duration is kept and only the zero one is defaulted",
			declared: gotest.SuiteConfig{Timeout: 5 * time.Second},
			want:     gotest.SuiteConfig{Timeout: 5 * time.Second, SetupTimeout: 30 * time.Second},
		},
		{
			Desc:     "NoDeadline stays",
			declared: gotest.SuiteConfig{Timeout: gotest.NoDeadline, SetupTimeout: gotest.NoDeadline},
			want:     gotest.SuiteConfig{Timeout: gotest.NoDeadline, SetupTimeout: gotest.NoDeadline},
		},
		{
			// The raw durations are the contract under test: any negative
			// disables a deadline, not only the named constant.
			Desc:     "any negative duration stays",
			declared: gotest.SuiteConfig{Timeout: -time.Second},                                 //nolint:config-no-deadline // see above
			want:     gotest.SuiteConfig{Timeout: -time.Second, SetupTimeout: 30 * time.Second}, //nolint:config-no-deadline // see above
		},
		{Desc: "a preset passes through", declared: gotest.IntegrationSuiteConfig(), want: gotest.IntegrationSuiteConfig()},
	}) {
		gotest.Equal(it, tc.want, gotestruntime.WithSuiteDefaults(tc.declared))
	}
}

func (s *ConfigDefaultsTestSuite) TestWithFixtureDefaults(t *gotest.T) {
	type row struct {
		Desc     string
		declared gotest.FixtureConfig
		want     gotest.FixtureConfig
	}
	for it, tc := range gotest.Each(t, []row{
		{Desc: "an empty marker runs like no marker", declared: gotest.FixtureConfig{}, want: gotest.DefaultFixtureConfig()},
		{
			Desc:     "a partial literal keeps its retries and takes the default Timeout",
			declared: gotest.FixtureConfig{Retries: 2, RetryDelay: time.Second},
			want:     gotest.FixtureConfig{Timeout: 2 * time.Minute, Retries: 2, RetryDelay: time.Second},
		},
		{
			Desc:     "NoDeadline stays",
			declared: gotest.FixtureConfig{Timeout: gotest.NoDeadline},
			want:     gotest.FixtureConfig{Timeout: gotest.NoDeadline},
		},
		{Desc: "a preset passes through", declared: gotest.ContainerFixtureConfig(), want: gotest.ContainerFixtureConfig()},
	}) {
		gotest.Equal(it, tc.want, gotestruntime.WithFixtureDefaults(tc.declared))
	}
}
