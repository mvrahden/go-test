package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SimpleFixture struct {}

func (f *SimpleFixture) BeforeAll(ctx context.Context) error { return nil }

type BasicTestSuite struct {
	*SimpleFixture
}

func (s *BasicTestSuite) TestOne(t *gotest.T) {}
