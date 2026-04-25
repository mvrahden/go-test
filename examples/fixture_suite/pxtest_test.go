package fixturesuite_test

import "github.com/mvrahden/go-test/pkg/gotest"

type ExternalSetupFixture struct {
	Ready bool
}

func (s *ExternalSetupFixture) BeforeAll(t *gotest.T) {
	s.Ready = true
}

type ExternalDemoTestSuite struct {
	*ExternalSetupFixture
}

func (s *ExternalDemoTestSuite) TestReady(t *gotest.T) {}
