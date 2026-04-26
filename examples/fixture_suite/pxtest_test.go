package fixturesuite_test

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ExternalSetupFixture struct {
	Ready bool
}

func (s *ExternalSetupFixture) BeforeAll(ctx context.Context) error {
	s.Ready = true
	return nil
}

type ExternalDemoTestSuite struct {
	*ExternalSetupFixture
}

func (s *ExternalDemoTestSuite) TestReady(t *gotest.T) {}
