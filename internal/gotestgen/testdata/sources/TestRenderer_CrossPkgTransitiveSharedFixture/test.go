package testpkg

import (
	"context"

	"testpkg/extfixtures"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SchemaSharedFixture struct {
	PG      *extfixtures.PostgresSharedFixture
	Version string
}

func (f *SchemaSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *SchemaSharedFixture) AfterAll(ctx context.Context) error  { return nil }

type MigrationTestSuite struct {
	Schema *SchemaSharedFixture
}

func (s *MigrationTestSuite) TestOne(t *gotest.T) {}
