package orphangoos

import "github.com/mvrahden/go-test/pkg/gotest"

// sigChecks is called only from a file of another platform.
type sigChecks struct{}

func (c *sigChecks) TestSignals(t *gotest.T) { gotest.True(t, true) }

type UnitTestSuite struct{}

func (s *UnitTestSuite) TestA(t *gotest.T) {}
