package stdlib

import "github.com/mvrahden/go-test/pkg/gotest"

type UnitTestSuite struct {
	sut *Unit
}

func (s *UnitTestSuite) BeforeEach(t *gotest.T) {
	s.sut = NewUnit()
}

func (s *UnitTestSuite) TestUnitSuccess(t *gotest.T) {
	for _, expected := range []string{"hello", "world", "foo", "bar", "baz"} {
		gotest.Equal(t, expected, s.sut.DoSomething())
	}
}
