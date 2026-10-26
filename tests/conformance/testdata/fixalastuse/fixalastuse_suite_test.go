package fixalastuse

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FastTestSuite is the fixture's only reader.
type FastTestSuite struct {
	A *ASharedFixture
}

func (s *FastTestSuite) TestReadsA(t *gotest.T) {
	gotest.Equal(t, "up", s.A.State)
	mark("fast ran")
}

// SlowTestSuite reads no fixture and runs well past the fast suite.
type SlowTestSuite struct{}

func (s *SlowTestSuite) TestTakesItsTime(t *gotest.T) {
	time.Sleep(3 * time.Second)
	mark("slow ends")
}
