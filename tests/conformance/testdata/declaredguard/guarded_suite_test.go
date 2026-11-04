// Package declaredguard holds a suite its guard keeps from running; see the
// spec conformance suite.
package declaredguard

import "github.com/mvrahden/go-test/pkg/gotest"

type GuardedTestSuite struct{}

func (s *GuardedTestSuite) SuiteGuard() string { return "the guard keeps this suite from running" }

func (s *GuardedTestSuite) TestGuarded(t *gotest.T) {
	t.When("the guard lets the suite run", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			gotest.True(it, true)
		})
		w.It("passes again", func(it *gotest.T) {
			gotest.True(it, true)
		})
	})
}
