package skipped

import "github.com/mvrahden/go-test/pkg/gotest"

// BoundTestSuite is the only suite binding the fixture.
type BoundTestSuite struct {
	Shared *MarkSharedFixture
}

func (s *BoundTestSuite) TestOne(t *gotest.T) { gotest.Equal(t, "up", s.Shared.State) }
func (s *BoundTestSuite) TestTwo(t *gotest.T) { gotest.Equal(t, "up", s.Shared.State) }

// FreeTestSuite binds nothing.
type FreeTestSuite struct{}

func (s *FreeTestSuite) TestOne(t *gotest.T) {}
