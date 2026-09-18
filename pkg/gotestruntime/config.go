package gotestruntime

import "github.com/mvrahden/go-test/pkg/gotest"

// WithSuiteDefaults is the config a SuiteConfig() marker runs under: a
// duration it leaves at zero takes DefaultSuiteConfig's, as if the marker were
// absent. A negative duration (gotest.NoDeadline) stays and disables the
// deadline. The declared value itself remains the verdict budget, so a zero is
// never enforced, exactly like a suite without a marker.
func WithSuiteDefaults(declared gotest.SuiteConfig) gotest.SuiteConfig {
	def := gotest.DefaultSuiteConfig()
	if declared.Timeout == 0 {
		declared.Timeout = def.Timeout
	}
	if declared.SetupTimeout == 0 {
		declared.SetupTimeout = def.SetupTimeout
	}
	return declared
}

// WithFixtureDefaults is [WithSuiteDefaults] for FixtureConfig() and
// SharedFixtureConfig() markers: a zero Timeout takes DefaultFixtureConfig's.
func WithFixtureDefaults(declared gotest.FixtureConfig) gotest.FixtureConfig {
	if declared.Timeout == 0 {
		declared.Timeout = gotest.DefaultFixtureConfig().Timeout
	}
	return declared
}
