package fixturesuite

import "github.com/mvrahden/go-test/pkg/gotest"

type SetupFixture struct {
	Value string
}

func (s *SetupFixture) BeforeAll(t *gotest.T) {
	s.Value = "initialized"
}

func (s *SetupFixture) AfterAll(t *gotest.T) {}

type DemoTestSuite struct {
	*SetupFixture
}

func (s *DemoTestSuite) TestValueSet(t *gotest.T)      {}
func (s *DemoTestSuite) TestAnotherCheck(t *gotest.T) {}
