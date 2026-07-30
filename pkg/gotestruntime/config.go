package gotestruntime

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// A timeout in this framework does two jobs, and they are not the same job.
//
// It bounds the context handed to a lifecycle phase, which costs nothing: code
// that watches its context gets cancelled, code that does not carries on. And it
// is a budget the phase is held to, enforced by verdict, because Go cannot
// preempt a running goroutine and a failure is the only thing left to say.
//
// The first is safe to default. The second is not: holding a suite to a number
// nobody wrote turns working suites red on upgrade, for a reason their author
// never chose and cannot find in their own code. So the resolved configs below
// keep the defaults for bounding contexts and enforce only what a suite or
// fixture asked for by name.

// SuiteConfig is a suite's configuration once the defaults and the suite's own
// SuiteConfig() have been merged, together with which budgets it asked for.
type SuiteConfig struct {
	gotest.SuiteConfig
	// TimeoutDeclared and SetupTimeoutDeclared record whether the value came
	// from the suite or from the default.
	TimeoutDeclared      bool
	SetupTimeoutDeclared bool
}

// ResolveSuiteConfig merges a suite's declared config over the defaults. It is
// variadic so a generated harness can pass nothing at all for a suite that has
// no SuiteConfig method.
//
// A zero field means "keep the default". FailFast and Parallel are one-way
// latches — once true, a later declaration with false will not reset them.
func ResolveSuiteConfig(declared ...gotest.SuiteConfig) SuiteConfig {
	cfg := SuiteConfig{SuiteConfig: gotest.DefaultSuiteConfig()}
	for _, d := range declared {
		if d.Timeout != 0 {
			cfg.Timeout = d.Timeout
			cfg.TimeoutDeclared = true
		}
		if d.SetupTimeout != 0 {
			cfg.SetupTimeout = d.SetupTimeout
			cfg.SetupTimeoutDeclared = true
		}
		if d.Retries != 0 {
			cfg.Retries = d.Retries
		}
		if d.FailFast {
			cfg.FailFast = true
		}
		if d.Parallel {
			cfg.Parallel = true
		}
	}
	return cfg
}

// TestBudget is the deadline a test method is held to, or zero when there is
// none to enforce — either because the suite never asked for one or because it
// disabled the budget with a negative Timeout.
func (c SuiteConfig) TestBudget() time.Duration {
	return budget(c.Timeout, c.TimeoutDeclared)
}

// SetupBudget is the deadline BeforeAll and AfterAll are held to, on the same
// terms as [SuiteConfig.TestBudget].
func (c SuiteConfig) SetupBudget() time.Duration {
	return budget(c.SetupTimeout, c.SetupTimeoutDeclared)
}

// FixtureConfig is a fixture's configuration once the defaults and its own
// FixtureConfig() or SharedFixtureConfig() have been merged.
type FixtureConfig struct {
	gotest.FixtureConfig
	// TimeoutDeclared records whether Timeout came from the fixture or from the
	// default.
	TimeoutDeclared bool
}

// ResolveFixtureConfig merges a fixture's declared config over the defaults, on
// the same terms as [ResolveSuiteConfig].
func ResolveFixtureConfig(declared ...gotest.FixtureConfig) FixtureConfig {
	cfg := FixtureConfig{FixtureConfig: gotest.DefaultFixtureConfig()}
	for _, d := range declared {
		if d.Timeout != 0 {
			cfg.Timeout = d.Timeout
			cfg.TimeoutDeclared = true
		}
		if d.Retries != 0 {
			cfg.Retries = d.Retries
		}
		if d.RetryDelay != 0 {
			cfg.RetryDelay = d.RetryDelay
		}
	}
	return cfg
}

// Budget is the deadline a fixture's BeforeAll and AfterAll are held to, or zero
// when there is none to enforce.
func (c FixtureConfig) Budget() time.Duration {
	return budget(c.Timeout, c.TimeoutDeclared)
}

func budget(timeout time.Duration, declared bool) time.Duration {
	if !declared || timeout <= 0 {
		return 0
	}
	return timeout
}
