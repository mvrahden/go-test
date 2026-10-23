package guardedshared

import "github.com/mvrahden/go-test/pkg/gotest"

// GuardedTestSuite would be skipped by its guard, which runs only once the
// suite runs; a fixture that never came up stops it before that.
type GuardedTestSuite struct {
	Trap *TrapSharedFixture
}

func (s *GuardedTestSuite) SuiteGuard() string { return "never here" }

func (s *GuardedTestSuite) TestNeverRuns(t *gotest.T) { gotest.Fail(t, "a guarded suite ran") }

// FreeTestSuite binds nothing and runs.
type FreeTestSuite struct{}

func (s *FreeTestSuite) TestOne(t *gotest.T) {}
