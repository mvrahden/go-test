package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PGSharedFixture struct {
	DSN string `gotest:"env=PG_DSN"`
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }

type E2EFixture struct {
	*PGSharedFixture
}

func (f *E2EFixture) BeforeAll(ctx context.Context) error { return nil }

type QueryTestSuite struct {
	*E2EFixture
}

func (s *QueryTestSuite) TestInsert(t *gotest.T) {}
