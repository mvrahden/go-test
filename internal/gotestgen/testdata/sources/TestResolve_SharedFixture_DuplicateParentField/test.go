package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ParentSharedFixture struct{}

func (f *ParentSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *ParentSharedFixture) AfterAll(ctx context.Context) error  { return nil }

// TwinSharedFixture declares two fields of the same parent shared-fixture
// type. Only one parent instance of a type ever exists in the DAG, so the
// wiring silently assigned the last field and left the first nil at BeforeAll.
type TwinSharedFixture struct {
	Primary   *ParentSharedFixture
	Secondary *ParentSharedFixture
}

func (f *TwinSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *TwinSharedFixture) AfterAll(ctx context.Context) error  { return nil }

type TwinTestSuite struct {
	Twin *TwinSharedFixture
}

func (s *TwinTestSuite) TestOne(t *gotest.T) {}
