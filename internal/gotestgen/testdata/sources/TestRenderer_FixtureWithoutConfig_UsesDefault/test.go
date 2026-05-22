package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PlainFixture struct{}

func (f *PlainFixture) BeforeAll(ctx context.Context) error { return nil }

type PlainTestSuite struct {
	*PlainFixture
}

func (s *PlainTestSuite) TestOne(t *gotest.T) {}
