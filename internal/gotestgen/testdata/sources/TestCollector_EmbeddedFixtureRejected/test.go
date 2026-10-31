package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type ChildTestSuite struct {
	*DBFixture
}

func (s *ChildTestSuite) TestSomething(t *gotest.T) {}
