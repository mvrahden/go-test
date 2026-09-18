package gotest

import "time"

// FixtureConfig controls timeout and retry behavior for package fixtures and
// shared fixtures. Returned by the optional FixtureConfig() or
// SharedFixtureConfig() marker method on a fixture struct.
// A zero Timeout gets DefaultFixtureConfig's, as if the marker were absent;
// NoDeadline (any negative value) disables it. Retries and RetryDelay are used
// as written.
//
// For shared fixtures, state is captured once during setup and distributed to
// all test processes as a JSON snapshot. Transfer fields (exported, not assigned
// in Hydrate) should contain stable connection parameters (host, port,
// credentials) rather than ephemeral handles. Each test process calls Hydrate()
// to establish live connections from those parameters.
type FixtureConfig struct {
	// Timeout is the deadline for each lifecycle operation (BeforeAll/AfterAll).
	// Zero means the 2m default; NoDeadline disables it.
	Timeout time.Duration
	// Retries is how many times to retry BeforeAll on failure. Default: 0.
	Retries int
	// RetryDelay is the pause between retry attempts. Default: 0.
	RetryDelay time.Duration
}

// SuiteConfig controls timeout, parallelism, and failure behavior for a test
// suite. Returned by the optional SuiteConfig() marker method on a suite struct.
// A duration left at zero gets DefaultSuiteConfig's, as if the marker were
// absent; NoDeadline (any negative value) disables it. Booleans are used as
// written. Start from a preset to override its durations:
//
//	cfg := gotest.DefaultSuiteConfig()
//	cfg.Parallel = true
//	return cfg
type SuiteConfig struct {
	// Timeout is the per-test-method deadline. Zero means the 30s default;
	// NoDeadline disables it.
	Timeout time.Duration
	// SetupTimeout is the deadline for BeforeAll/AfterAll. Zero means the 30s
	// default; NoDeadline disables it.
	SetupTimeout time.Duration
	// FailFast stops the suite after the first test failure. Default: false.
	FailFast bool
	// Parallel runs test methods concurrently. Requires a returning BeforeEach
	// so each parallel test gets its own isolated state. Default: false.
	Parallel bool
	// Exclusive schedules the suite's process strictly alone: after every
	// non-exclusive suite has finished, one exclusive suite at a time, in
	// deterministic order. For suites whose verdicts depend on wall-clock
	// behavior or contended resources (timing budgets, containers, ports,
	// heavy child builds) — a budget verdict taken on a saturated machine is
	// not a verdict you can act on. Resolved statically by the generator,
	// like Parallel; scheduling-only, no effect inside the suite process.
	// Default: false.
	Exclusive bool
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

// NoDeadline disables a timeout: a SuiteConfig or FixtureConfig duration set
// to it (or any negative value) runs without a deadline, where zero would get
// the default.
const NoDeadline time.Duration = -1

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
