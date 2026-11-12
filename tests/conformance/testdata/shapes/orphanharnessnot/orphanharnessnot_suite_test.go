package orphanharnessnot

import "github.com/mvrahden/go-test/pkg/gotest"

// anyImpl is converted to any alone.
type anyImpl struct{}

func (*anyImpl) TestRoundTrip(t *gotest.T) {}

type namer interface{ Name() string }

// namedImpl is converted to an interface without its case method.
type namedImpl struct{}

func (*namedImpl) Name() string { return "named" }

func (*namedImpl) TestRoundTrip(t *gotest.T) {}

type HoldsTestSuite struct{}

func (s *HoldsTestSuite) TestHolds(t *gotest.T) {
	var held any = &anyImpl{}
	var n namer = &namedImpl{}
	gotest.NotZero(t, held)
	gotest.Equal(t, "named", n.Name())
}
