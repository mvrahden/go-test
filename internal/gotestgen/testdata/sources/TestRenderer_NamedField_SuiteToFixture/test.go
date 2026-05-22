package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{ Conn string }
func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type QueryTestSuite struct { db *DBFixture }
func (s *QueryTestSuite) TestOne(t *gotest.T) {}
