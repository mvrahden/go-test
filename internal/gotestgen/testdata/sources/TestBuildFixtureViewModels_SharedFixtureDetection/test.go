package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PGSharedFixture struct {
	DSN  string
	Host string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }

type DBFixture struct {
	*PGSharedFixture
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type DBTestSuite struct {
	*DBFixture
}

func (s *DBTestSuite) TestQuery(t *gotest.T) {}
