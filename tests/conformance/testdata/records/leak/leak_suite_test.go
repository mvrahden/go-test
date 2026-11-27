package leak

import "github.com/mvrahden/go-test/pkg/gotest"

type LeakTestSuite struct{ Leak *LeakSharedFixture }

func (s *LeakTestSuite) TestHolds(t *gotest.T) { gotest.Equal(t, 1, 1) }
