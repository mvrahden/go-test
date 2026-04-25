package fixturesuite_test

import "github.com/mvrahden/go-test/pkg/gotest"

//go:test fixture
type ExternalSetup struct {
	Ready bool
}

func (s *ExternalSetup) BeforeAll(t *gotest.T) {
	s.Ready = true
}

type ExternalDemoTestSuite struct {
	*ExternalSetup
}

func (s *ExternalDemoTestSuite) TestReady(t *gotest.T) {}
