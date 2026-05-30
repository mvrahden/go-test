package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type PGSharedFixture struct{ ConnStr string }

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) AfterAll(ctx context.Context) error  { return nil }

type SchemaSharedFixture struct {
	PG      *PGSharedFixture
	Version string
}

func (f *SchemaSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *SchemaSharedFixture) AfterAll(ctx context.Context) error  { return nil }

type MigrationTestSuite struct {
	Schema *SchemaSharedFixture
}

func (s *MigrationTestSuite) TestOne(t *gotest.T) {}
