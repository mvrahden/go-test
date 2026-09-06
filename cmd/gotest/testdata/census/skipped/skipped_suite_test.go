package skipped_test

import "github.com/mvrahden/go-test/pkg/gotest"

// GuardedTestSuite skips itself as a whole, so its methods never get their
// own verdict; the census must count them as covered by the suite's skip.
type GuardedTestSuite struct{}

func (s *GuardedTestSuite) BeforeAll(t *gotest.T) { t.Skipf("guarded: not on this machine") }

func (s *GuardedTestSuite) TestNeverReached(t *gotest.T) { gotest.True(t, false) }
