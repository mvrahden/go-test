package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type MyTestSuite struct {
	*DBFixture
}

func (s *MyTestSuite) TestSomething(t *gotest.T) {}
