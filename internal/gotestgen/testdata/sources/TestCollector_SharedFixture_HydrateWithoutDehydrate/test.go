package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error   { return nil }

type MyTestSuite struct{ *PGSharedFixture }

func (s *MyTestSuite) TestOne(t *gotest.T) {}
