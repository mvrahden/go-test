package fixadeps

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DirectTestSuite binds Parent itself and is done first.
type DirectTestSuite struct {
	Parent *ParentSharedFixture
}

func (s *DirectTestSuite) TestReadsParent(t *gotest.T) {
	gotest.Equal(t, "up", s.Parent.State)
	mark("direct ran")
}

// ChildReaderTestSuite reaches Parent only through Child, and runs longer.
type ChildReaderTestSuite struct {
	Child *ChildSharedFixture
}

func (s *ChildReaderTestSuite) TestReadsChild(t *gotest.T) {
	time.Sleep(2 * time.Second)
	gotest.Equal(t, "up", s.Child.State)
	mark("child reader ends")
}
