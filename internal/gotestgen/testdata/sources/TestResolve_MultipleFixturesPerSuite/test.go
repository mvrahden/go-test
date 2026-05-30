package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type AFixture struct{}
func (f *AFixture) BeforeAll(ctx context.Context) error { return nil }

type BFixture struct{}
func (f *BFixture) BeforeAll(ctx context.Context) error { return nil }

type MultiTestSuite struct {
	A *AFixture
	B *BFixture
}
func (s *MultiTestSuite) TestOne(t *gotest.T) {}
