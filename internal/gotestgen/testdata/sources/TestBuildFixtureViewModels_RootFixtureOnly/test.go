package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MyFixture struct {}

func (f *MyFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *MyFixture) AfterAll(ctx context.Context) error  { return nil }

type MyTestSuite struct {
	*MyFixture
}

func (s *MyTestSuite) TestOne(t *gotest.T) {}
