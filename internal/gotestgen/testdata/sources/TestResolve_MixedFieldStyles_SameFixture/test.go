package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{ Conn string }
func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type EmbeddedTestSuite struct { *DBFixture }
func (s *EmbeddedTestSuite) TestOne(t *gotest.T) {}

type NamedTestSuite struct { db *DBFixture }
func (s *NamedTestSuite) TestOne(t *gotest.T) {}
