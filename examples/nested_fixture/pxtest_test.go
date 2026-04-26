package nestedfixture_test

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ExternalInfraFixture struct {
	Value string
}

func (f *ExternalInfraFixture) BeforeAll(ctx context.Context) error {
	f.Value = "infra-ready"
	return nil
}

type ExternalLightTestSuite struct {
	*ExternalInfraFixture
}

func (s *ExternalLightTestSuite) TestSimple(t *gotest.T) {}
