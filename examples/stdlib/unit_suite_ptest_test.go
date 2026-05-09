package stdlib

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// UnitTestSuite is a testsuite-style test of the Unit.
type UnitTestSuite struct {
	sut *Unit
}

func (s *UnitTestSuite) BeforeEach(t *gotest.T) {
	s.sut = NewUnit()
}

func (s *UnitTestSuite) TestUnitSuccess(t *gotest.T) {
	for idx, expected := range []string{"hello", "world", "foo", "bar", "baz"} {
		actual := s.sut.DoSomething()
		if actual != expected {
			t.T().Fatalf("not equal@%d. wanted %q; got %q", idx, expected, actual)
		}
	}
}
