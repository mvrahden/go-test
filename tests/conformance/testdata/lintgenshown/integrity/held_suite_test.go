package integrity

import "github.com/mvrahden/go-test/pkg/gotest"

// HeldTestSuite has a case on a value receiver: the receiver rule and the
// generator both refuse it.
type HeldTestSuite struct{}

func (s HeldTestSuite) TestByValue(t *gotest.T) {}

func (s *HeldTestSuite) TestRuns(t *gotest.T) {}
