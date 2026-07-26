package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MyFixture struct{}

func (f *MyFixture) BeforeAll(ctx context.Context) error { return nil }

type MyTestSuite struct {
	Fixture *MyFixture
}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
