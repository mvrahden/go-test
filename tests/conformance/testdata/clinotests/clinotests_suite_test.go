package clinotests_test

import "github.com/mvrahden/go-test/pkg/gotest"

// GuardedTestSuite is skipped by its guard, so its function runs and skips.
type GuardedTestSuite struct{}

func (s *GuardedTestSuite) SuiteGuard() string    { return "not here" }
func (s *GuardedTestSuite) TestNever(t *gotest.T) { gotest.Fail(t, "a guarded suite ran") }

// AllExcludedTestSuite has only excluded methods and is never dispatched.
type AllExcludedTestSuite struct{}

func (s *AllExcludedTestSuite) X_TestOne(t *gotest.T) { gotest.Fail(t, "an excluded method ran") }

// RunsTestSuite runs.
type RunsTestSuite struct{}

func (s *RunsTestSuite) TestOne(t *gotest.T) { gotest.True(t, true) }
