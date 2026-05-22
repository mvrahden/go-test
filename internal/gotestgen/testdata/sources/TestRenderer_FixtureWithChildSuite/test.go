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

type QueryTestSuite struct {
	*DBFixture
}

func (s *QueryTestSuite) BeforeEach(t *gotest.T) {}
func (s *QueryTestSuite) AfterEach(t *gotest.T)  {}
func (s *QueryTestSuite) TestInsert(t *gotest.T) {}
func (s *QueryTestSuite) TestSelect(t *gotest.T) {}
