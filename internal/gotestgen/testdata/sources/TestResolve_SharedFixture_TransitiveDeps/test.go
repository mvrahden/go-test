package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type PGSharedFixture struct{ ConnStr string }

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) AfterAll(ctx context.Context) error  { return nil }

type SchemaSharedFixture struct {
	PG *PGSharedFixture
}

func (f *SchemaSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *SchemaSharedFixture) AfterAll(ctx context.Context) error  { return nil }

// Suite references only Schema, but transitively needs PG too
type UserTestSuite struct {
	Schema *SchemaSharedFixture
}

func (s *UserTestSuite) TestOne(t *gotest.T) {}

// Suite references only PG directly
type SimpleTestSuite struct {
	PG *PGSharedFixture
}

func (s *SimpleTestSuite) TestOne(t *gotest.T) {}
