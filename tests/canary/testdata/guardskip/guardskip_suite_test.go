package guardskip

import (
	"context"
	"errors"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// TrapFixture fails to come up, so a suite that sets it up fails.
type TrapFixture struct{}

func (f *TrapFixture) BeforeAll(ctx context.Context) error {
	return errors.New("a guarded suite set up its fixture")
}

// GuardedTestSuite is skipped by its guard, which runs before its fixtures:
// the trap is never set up and the suite reports a skip, not a failure.
type GuardedTestSuite struct {
	Trap *TrapFixture
}

func (s *GuardedTestSuite) SuiteGuard() string { return "not in the canary" }

func (s *GuardedTestSuite) TestNeverRuns(t *gotest.T) {
	gotest.Fail(t, "a guarded suite ran")
}
