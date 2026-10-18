package teardownaftersetup_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/canary/testdata/teardownaftersetup"
)

// DownTestSuite reads the fixture that never comes up; it never runs.
type DownTestSuite struct {
	Down *teardownaftersetup.DownSharedFixture
}

func (s *DownTestSuite) TestNeverRuns(t *gotest.T) { gotest.Equal(t, "up", s.Down.State) }

// HeldTestSuite reads the fixture that comes up and will not release.
type HeldTestSuite struct {
	Release *teardownaftersetup.ReleaseSharedFixture
}

func (s *HeldTestSuite) TestFixtureIsUp(t *gotest.T) { gotest.Equal(t, "up", s.Release.State) }
