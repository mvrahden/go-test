package gotest

import "time"

// FixtureConfig controls timeout and retry behavior for package fixtures and
// shared fixtures. Returned by the optional FixtureConfig() or
// SharedFixtureConfig() marker method on a fixture struct.
// The returned value is used as-is: a zero (or negative) Timeout means no
// deadline. Without a marker method, DefaultFixtureConfig applies.
//
// For shared fixtures, state is captured once during setup and distributed to
// all test processes as a JSON snapshot. Transfer fields (exported, not assigned
// in Hydrate) should contain stable connection parameters (host, port,
// credentials) rather than ephemeral handles. Each test process calls Hydrate()
// to establish live connections from those parameters.
type FixtureConfig struct {
	// Timeout is the deadline for each lifecycle operation (BeforeAll/AfterAll).
	// Zero or negative means no deadline. DefaultFixtureConfig uses 2m.
	Timeout time.Duration
	// Retries is how many times to retry BeforeAll on failure. Default: 0.
	Retries int
	// RetryDelay is the pause between retry attempts. Default: 0.
	RetryDelay time.Duration
}

// SuiteConfig controls timeout, parallelism, and failure behavior for a test
// suite. Returned by the optional SuiteConfig() marker method on a suite struct.
// The returned value is used as-is: a zero (or negative) duration means no
// deadline. Without a marker method, DefaultSuiteConfig applies.
// Start from a preset to combine defaults with overrides:
//
//	cfg := gotest.DefaultSuiteConfig()
//	cfg.Parallel = true
//	return cfg
type SuiteConfig struct {
	// Timeout is the per-test-method deadline. Zero or negative means no
	// deadline. DefaultSuiteConfig uses 30s.
	Timeout time.Duration
	// SetupTimeout is the deadline for BeforeAll/AfterAll. Zero or negative
	// means no deadline. DefaultSuiteConfig uses 30s.
	SetupTimeout time.Duration
	// FailFast stops the suite after the first test failure. Default: false.
	FailFast bool
	// Parallel runs test methods concurrently. Requires a returning BeforeEach
	// so each parallel test gets its own isolated state. Default: false.
	Parallel bool
}

// DefaultFixtureConfig returns a baseline configuration for package fixtures:
// 2-minute timeout, no retries.
func DefaultFixtureConfig() FixtureConfig {
	return FixtureConfig{Timeout: 2 * time.Minute}
}

// ContainerFixtureConfig returns a configuration tuned for container-based
// fixtures (e.g. testcontainers): 5-minute timeout, 1 retry with 5s delay.
func ContainerFixtureConfig() FixtureConfig {
	return FixtureConfig{Timeout: 5 * time.Minute, Retries: 1, RetryDelay: 5 * time.Second}
}

// DefaultSuiteConfig returns a baseline suite configuration: 30s test timeout,
// 30s setup timeout, no retries, sequential execution.
func DefaultSuiteConfig() SuiteConfig {
	return SuiteConfig{Timeout: 30 * time.Second, SetupTimeout: 30 * time.Second}
}

// IntegrationSuiteConfig returns a configuration for heavier integration suites:
// 2-minute test timeout, 5-minute setup timeout.
func IntegrationSuiteConfig() SuiteConfig {
	return SuiteConfig{Timeout: 2 * time.Minute, SetupTimeout: 5 * time.Minute}
}
