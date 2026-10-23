// Package fixtureelsewhere binds fixtures whose failing hooks another
// package declares; the canary reads where the failures are located.
package fixtureelsewhere

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/canary/testdata/fixtureelsewhere/elsewhere"
)

// PromotedFixture's AfterEach is the one elsewhere.Hooks declares.
type PromotedFixture struct {
	elsewhere.Hooks
}

func (f *PromotedFixture) BeforeAll(ctx context.Context) error { return nil }

type RemoteTestSuite struct {
	Remote *elsewhere.RemoteFixture
}

func (s *RemoteTestSuite) TestBody(t *gotest.T) {}

type PromotedTestSuite struct {
	Promoted *PromotedFixture
}

func (s *PromotedTestSuite) TestBody(t *gotest.T) {}
