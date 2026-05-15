package fixturesuite

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SetupFixture struct {
	Value string
}

func (f *SetupFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.ContainerFixtureConfig()
}

func (f *SetupFixture) BeforeAll(ctx context.Context) error {
	f.Value = "initialized"
	return nil
}

func (f *SetupFixture) AfterAll(ctx context.Context) error { return nil }

type DemoTestSuite struct {
	*SetupFixture
}

func (s *DemoTestSuite) TestValueSet(t *gotest.T) {
	gotest.Equal(t, "initialized", s.SetupFixture.Value)
}

func (s *DemoTestSuite) TestAnotherCheck(t *gotest.T) {
	gotest.NotEmpty(t, s.SetupFixture.Value)
}
