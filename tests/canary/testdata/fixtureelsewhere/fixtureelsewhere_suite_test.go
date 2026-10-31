// Package fixtureelsewhere binds fixtures whose failing hooks another
// package declares; the canary reads where the failures are located.
package fixtureelsewhere

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/canary/testdata/fixtureelsewhere/elsewhere"
)

type RemoteTestSuite struct {
	Remote *elsewhere.RemoteFixture
}

func (s *RemoteTestSuite) TestBody(t *gotest.T) {}

type LeavingTestSuite struct {
	Leaving *elsewhere.LeavingFixture
}

func (s *LeavingTestSuite) TestBody(t *gotest.T) {}
