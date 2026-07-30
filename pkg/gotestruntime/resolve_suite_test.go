package gotestruntime_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// ResolveConfigTestSuite covers the difference between a budget a suite asked
// for and one the framework picked on its behalf.
//
// The distinction matters because a budget is now enforced by verdict: a method
// that outlives it is failed. Holding a suite to a number it never wrote turns
// working suites red on upgrade, for no reason the author would recognise. The
// default still bounds t.Context(), which costs nothing and cancels code that
// bothers to watch it — it just is not grounds for failing anyone.
type ResolveConfigTestSuite struct{}

func (s *ResolveConfigTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *ResolveConfigTestSuite) TestUndeclaredBudgets(t *gotest.T) {
	t.When("a suite declares nothing", func(w *gotest.T) {
		cfg := gotestruntime.ResolveSuiteConfig()

		w.It("still bounds the contexts with the defaults", func(it *gotest.T) {
			gotest.Equal(it, 30*time.Second, cfg.Timeout)
			gotest.Equal(it, 30*time.Second, cfg.SetupTimeout)
		})

		w.It("enforces neither", func(it *gotest.T) {
			gotest.Equal(it, time.Duration(0), cfg.TestBudget())
			gotest.Equal(it, time.Duration(0), cfg.SetupBudget())
		})
	})

	t.When("a suite declares only unrelated fields", func(w *gotest.T) {
		cfg := gotestruntime.ResolveSuiteConfig(gotest.SuiteConfig{Parallel: true, FailFast: true})

		w.It("carries them over", func(it *gotest.T) {
			gotest.True(it, cfg.Parallel)
			gotest.True(it, cfg.FailFast)
		})

		w.It("still enforces no budget", func(it *gotest.T) {
			gotest.Equal(it, time.Duration(0), cfg.TestBudget())
			gotest.Equal(it, time.Duration(0), cfg.SetupBudget())
		})
	})
}

func (s *ResolveConfigTestSuite) TestDeclaredBudgets(t *gotest.T) {
	t.When("a suite declares a Timeout", func(w *gotest.T) {
		cfg := gotestruntime.ResolveSuiteConfig(gotest.SuiteConfig{Timeout: 5 * time.Second})

		w.It("enforces exactly that", func(it *gotest.T) {
			gotest.Equal(it, 5*time.Second, cfg.TestBudget())
		})

		w.It("leaves the setup budget unenforced", func(it *gotest.T) {
			gotest.Equal(it, 30*time.Second, cfg.SetupTimeout)
			gotest.Equal(it, time.Duration(0), cfg.SetupBudget())
		})
	})

	t.When("a suite disables a budget with -1", func(w *gotest.T) {
		cfg := gotestruntime.ResolveSuiteConfig(gotest.SuiteConfig{Timeout: -1, SetupTimeout: -1})

		w.It("enforces nothing and leaves the contexts unbounded", func(it *gotest.T) {
			// A negative timeout is the documented "off". testScopedT already
			// reads it that way, and a budget of -1 must not be enforced either.
			gotest.Equal(it, time.Duration(0), cfg.TestBudget())
			gotest.Equal(it, time.Duration(0), cfg.SetupBudget())
		})
	})

	t.When("a suite declares a SetupTimeout only", func(w *gotest.T) {
		cfg := gotestruntime.ResolveSuiteConfig(gotest.SuiteConfig{SetupTimeout: time.Minute})

		w.It("enforces the setup budget alone", func(it *gotest.T) {
			gotest.Equal(it, time.Minute, cfg.SetupBudget())
			gotest.Equal(it, time.Duration(0), cfg.TestBudget())
		})
	})
}

func (s *ResolveConfigTestSuite) TestResolveFixtureConfig(t *gotest.T) {
	t.When("a fixture declares nothing", func(w *gotest.T) {
		cfg := gotestruntime.ResolveFixtureConfig()

		w.It("bounds its lifecycle with the default but enforces no budget", func(it *gotest.T) {
			gotest.Equal(it, 2*time.Minute, cfg.Timeout)
			gotest.Equal(it, time.Duration(0), cfg.Budget())
		})
	})

	t.When("a fixture declares a Timeout and retries", func(w *gotest.T) {
		cfg := gotestruntime.ResolveFixtureConfig(gotest.FixtureConfig{
			Timeout:    5 * time.Minute,
			Retries:    1,
			RetryDelay: 5 * time.Second,
		})

		w.It("enforces the declared timeout and carries the retry policy", func(it *gotest.T) {
			gotest.Equal(it, 5*time.Minute, cfg.Budget())
			gotest.Equal(it, 1, cfg.Retries)
			gotest.Equal(it, 5*time.Second, cfg.RetryDelay)
		})
	})
}
