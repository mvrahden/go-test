package failingshared

import "github.com/mvrahden/go-test/pkg/gotest"

// LeakTestSuite never runs: its fixture does not come up.
type LeakTestSuite struct {
	Leak *LeakSharedFixture
}

func (s *LeakTestSuite) TestNeverRuns(t *gotest.T) { gotest.Fail(t, "a suite without its fixture ran") }
