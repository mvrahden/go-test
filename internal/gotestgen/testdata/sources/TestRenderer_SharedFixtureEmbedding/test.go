package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PostgresSharedFixture struct {
	DSN string
}

func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error { return nil }

type E2EFixture struct {
	*PostgresSharedFixture
	Pool string
}

func (f *E2EFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *E2EFixture) AfterAll(ctx context.Context) error  { return nil }

type QueryTestSuite struct {
	*E2EFixture
}

func (s *QueryTestSuite) TestInsert(t *gotest.T) {}
