package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *DBFixture) AfterAll(ctx context.Context) error   { return nil }
func (f *DBFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *DBFixture) AfterEach(ctx context.Context) error  { return nil }

// UsesTestSuite binds the fixture.
type UsesTestSuite struct {
	Fix *DBFixture
}

func (s *UsesTestSuite) TestRuns(t *gotest.T) {}
