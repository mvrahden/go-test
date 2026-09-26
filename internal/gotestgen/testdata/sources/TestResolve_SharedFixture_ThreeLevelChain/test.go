package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type PGSharedFixture struct{ ConnStr string }

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) AfterAll(ctx context.Context) error  { return nil }

// ParentFixture is the only fixture that names the shared fixture.
type ParentFixture struct {
	PG *PGSharedFixture
}

func (f *ParentFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *ParentFixture) AfterAll(ctx context.Context) error  { return nil }

type ChildFixture struct {
	Parent *ParentFixture
}

func (f *ChildFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *ChildFixture) AfterAll(ctx context.Context) error  { return nil }

// DeepTestSuite reaches PG only through Child → Parent.
type DeepTestSuite struct {
	Child *ChildFixture
}

func (s *DeepTestSuite) TestOne(t *gotest.T) {}
