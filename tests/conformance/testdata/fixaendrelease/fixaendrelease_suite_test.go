package fixaendrelease

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// QuickTestSuite is the alpha fixture's only reader.
type QuickTestSuite struct {
	Alpha *AlphaSharedFixture
}

func (s *QuickTestSuite) TestReadsAlpha(t *gotest.T) { gotest.Equal(t, "up", s.Alpha.State) }

// SlowerTestSuite is the beta fixture's only reader and ends the run.
type SlowerTestSuite struct {
	Beta *BetaSharedFixture
}

func (s *SlowerTestSuite) TestReadsBeta(t *gotest.T) {
	gotest.Equal(t, "up", s.Beta.State)
	time.Sleep(time.Second)
}
