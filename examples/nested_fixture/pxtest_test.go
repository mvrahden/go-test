package nestedfixture_test

import "github.com/mvrahden/go-test/pkg/gotest"

type ExternalInfraFixture struct {
	Value string
}

func (f *ExternalInfraFixture) BeforeAll(t *gotest.T) {
	f.Value = "infra-ready"
}

type ExternalLightTestSuite struct {
	*ExternalInfraFixture
}

func (s *ExternalLightTestSuite) TestSimple(t *gotest.T) {}
