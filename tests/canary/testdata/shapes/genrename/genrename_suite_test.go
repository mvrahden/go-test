package genrename

import "github.com/mvrahden/go-test/pkg/gotest"

// OldNameTestSuite is renamed after the first generate.
type OldNameTestSuite struct{}

func (s *OldNameTestSuite) TestRuns(t *gotest.T) { gotest.True(t, true) }
