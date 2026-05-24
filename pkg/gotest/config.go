package gotest

import "time"

// FixtureConfig controls timeout and retry behavior for package fixtures and
// shared fixtures. Returned by the optional FixtureConfig() or
// SharedFixtureConfig() marker method on a fixture struct.
// A zero value means "keep the default."
type FixtureConfig struct {
	// Timeout is the deadline for each lifecycle operation (BeforeAll/AfterAll).
	// Use -1 to disable. Default: 2m (DefaultFixtureConfig).
	Timeout time.Duration
	// Retries is how many times to retry BeforeAll on failure. Default: 0.
	Retries int
	// RetryDelay is the pause between retry attempts. Default: 0.
	RetryDelay time.Duration
}

// SuiteConfig controls timeout, parallelism, and failure behavior for a test
// suite. Returned by the optional SuiteConfig() marker method on a suite struct.
// A zero value means "keep the default"; booleans latch to true and cannot be
// reset to false via overlay.
type SuiteConfig struct {
	// Timeout is the per-test-method deadline. Use -1 to disable. Default: 30s.
	Timeout time.Duration
	// SetupTimeout is the deadline for BeforeAll/AfterAll. Use -1 to disable. Default: 30s.
	SetupTimeout time.Duration
	// Retries is how many times to retry a failed test method. Default: 0.
	Retries int
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

// OverlayFixtureConfig merges overlay into base: non-zero fields in overlay
// replace the corresponding base field; zero fields are preserved.
func OverlayFixtureConfig(base *FixtureConfig, overlay FixtureConfig) {
	if overlay.Timeout != 0 {
		base.Timeout = overlay.Timeout
	}
	if overlay.Retries != 0 {
		base.Retries = overlay.Retries
	}
	if overlay.RetryDelay != 0 {
		base.RetryDelay = overlay.RetryDelay
	}
}

// OverlaySuiteConfig merges overlay into base: non-zero fields replace the
// corresponding base field. FailFast and Parallel are one-way latches — once
// true, an overlay with false will not reset them.
func OverlaySuiteConfig(base *SuiteConfig, overlay SuiteConfig) {
	if overlay.Timeout != 0 {
		base.Timeout = overlay.Timeout
	}
	if overlay.SetupTimeout != 0 {
		base.SetupTimeout = overlay.SetupTimeout
	}
	if overlay.Retries != 0 {
		base.Retries = overlay.Retries
	}
	if overlay.FailFast {
		base.FailFast = true
	}
	if overlay.Parallel {
		base.Parallel = true
	}
}
