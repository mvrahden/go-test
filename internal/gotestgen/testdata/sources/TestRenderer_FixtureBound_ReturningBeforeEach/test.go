package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{ Conn string }

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type myCtx struct{ pool string }

type QueryTestSuite struct {
	*DBFixture
}

func (s *QueryTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *QueryTestSuite) AfterEach(t *gotest.T, ctx *myCtx) {}
func (s *QueryTestSuite) TestInsert(t *gotest.T, ctx *myCtx) {}
