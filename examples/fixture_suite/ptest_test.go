package fixturesuite

import "github.com/mvrahden/go-test/pkg/gotest"

//go:test fixture
type Setup struct {
	Value string
}

func (s *Setup) BeforeAll(t *gotest.T) {
	s.Value = "initialized"
}

func (s *Setup) AfterAll(t *gotest.T) {}

type DemoTestSuite struct {
	*Setup
}

func (s *DemoTestSuite) TestValueSet(t *gotest.T)      {}
func (s *DemoTestSuite) TestAnotherCheck(t *gotest.T) {}
