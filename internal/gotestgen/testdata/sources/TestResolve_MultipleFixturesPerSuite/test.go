package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type AFixture struct{}
func (f *AFixture) BeforeAll(ctx context.Context) error { return nil }

type BFixture struct{}
func (f *BFixture) BeforeAll(ctx context.Context) error { return nil }

type BadTestSuite struct {
	*AFixture
	*BFixture
}
func (s *BadTestSuite) TestOne(t *gotest.T) {}
