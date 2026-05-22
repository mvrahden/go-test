package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type AFixture struct{}
func (f *AFixture) BeforeAll(ctx context.Context) error { return nil }

type BFixture struct{}
func (f *BFixture) BeforeAll(ctx context.Context) error { return nil }

type ChildFixture struct {
	*AFixture
	*BFixture
}
func (f *ChildFixture) BeforeAll(ctx context.Context) error { return nil }

type SomeTestSuite struct { *ChildFixture }
func (s *SomeTestSuite) TestOne(t *gotest.T) {}
