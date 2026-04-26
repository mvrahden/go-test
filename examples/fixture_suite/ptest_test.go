package fixturesuite

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SetupFixture struct {
	Value string
}

func (s *SetupFixture) BeforeAll(ctx context.Context) error {
	s.Value = "initialized"
	return nil
}

func (s *SetupFixture) AfterAll(ctx context.Context) error { return nil }

type DemoTestSuite struct {
	*SetupFixture
}

func (s *DemoTestSuite) TestValueSet(t *gotest.T)      {}
func (s *DemoTestSuite) TestAnotherCheck(t *gotest.T) {}
