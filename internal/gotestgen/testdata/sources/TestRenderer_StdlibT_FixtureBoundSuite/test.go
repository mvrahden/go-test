package testpkg

import (
	"context"
	"testing"
)

type DBFixture struct{}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type StdlibTestSuite struct {
	*DBFixture
}

func (s *StdlibTestSuite) BeforeAll(t *testing.T)  {}
func (s *StdlibTestSuite) AfterEach(t *testing.T)  {}
func (s *StdlibTestSuite) TestQuery(t *testing.T)  {}
