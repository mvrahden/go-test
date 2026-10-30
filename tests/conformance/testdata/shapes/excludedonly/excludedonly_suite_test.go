package excludedonly

import "github.com/mvrahden/go-test/pkg/gotest"

// LaterTestSuite declares a case, excluded: it runs as skipped.
type LaterTestSuite struct{}

func (s *LaterTestSuite) X_TestLater(t *gotest.T) { gotest.Fail(t, "an excluded case ran") }

type RealTestSuite struct{}

func (s *RealTestSuite) TestRuns(t *gotest.T) {}
