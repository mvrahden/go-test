package gosuite

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type UnitTestSuite struct{}

func (s *UnitTestSuite) BeforeAll(t *gotest.T)  {}
func (s *UnitTestSuite) BeforeEach(t *gotest.T) {}

// func (s *UnitTestSuite) fTestSomethingSpecific(t *gotest.T)      {} // focus
func (s *UnitTestSuite) xTestUnit(t *gotest.T) {} // skip
func (s *UnitTestSuite) TestUnitFails(t *gotest.T) {
	t.T().Fail()
}
