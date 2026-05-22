package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type PGSharedFixture struct {
	ConnStr string
	Port    int
}
func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) AfterAll(ctx context.Context) error  { return nil }

type AppFixture struct { *PGSharedFixture }
func (f *AppFixture) BeforeAll(ctx context.Context) error { return nil }

type UserTestSuite struct { *AppFixture }
func (s *UserTestSuite) TestOne(t *gotest.T) {}
