package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CFGFixture struct{}

func (f *CFGFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *CFGFixture) AfterAll(ctx context.Context) error  { return nil }
func (f *CFGFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.ContainerFixtureConfig()
}

type CFGTestSuite struct {
	*CFGFixture
}

func (s *CFGTestSuite) TestOne(t *gotest.T) {}
