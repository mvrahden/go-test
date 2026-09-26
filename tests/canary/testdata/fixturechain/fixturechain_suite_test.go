package fixturechain

import "github.com/mvrahden/go-test/pkg/gotest"

// ChainTestSuite binds only the child fixture; the shared fixture two levels
// up must still be started and its state hydrated.
type ChainTestSuite struct {
	Child *ChildFixture
}

func (s *ChainTestSuite) TestSharedStateArrives(t *gotest.T) {
	gotest.Equal(t, "hydrated", s.Child.Parent.Shared.Token)
}
