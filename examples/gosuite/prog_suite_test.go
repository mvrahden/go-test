package gosuite

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type UnitTestSuite struct {
	sut *Unit
}

func (s *UnitTestSuite) BeforeAll(t *gotest.T) {
	t.T().Logf("BeforeAll")
}
func (s *UnitTestSuite) BeforeEach(t *gotest.T) {
	t.T().Logf("BeforeEach")
	s.sut = NewUnit()
}

func (s *UnitTestSuite) xTestUnit(t *gotest.T) {} // skip
func (s *UnitTestSuite) TestUnitSuccess(t *gotest.T) {
	t.T().Logf("TestUnitSuccess")
}
func (s *UnitTestSuite) F_TestUnitFails(t *gotest.T) {
	t.T().Fail()
}
