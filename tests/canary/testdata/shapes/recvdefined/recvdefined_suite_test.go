package recvdefined

import "github.com/mvrahden/go-test/pkg/gotest"

// cases declares a case, which a type defined over it does not carry.
type cases struct{}

func (c *cases) TestRuns(t *gotest.T) {}

type DefinedTestSuite cases
