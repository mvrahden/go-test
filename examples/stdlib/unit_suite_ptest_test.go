package stdlib

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type UnitTestSuite struct {
	sut   *Unit
	ready bool
}

func (s *UnitTestSuite) BeforeAll(t *testing.T) {
	s.ready = true
}

func (s *UnitTestSuite) AfterAll(t *testing.T) {
	s.ready = false
}

func (s *UnitTestSuite) BeforeEach(t *testing.T) {
	s.sut = NewUnit()
}

func (s *UnitTestSuite) AfterEach(t *testing.T) {
	s.sut = nil
}

func (s *UnitTestSuite) TestUnitSuccess(t *testing.T) {
	for _, expected := range []string{"hello", "world", "foo", "bar", "baz"} {
		actual := s.sut.DoSomething()
		if actual != expected {
			t.Fatalf("wanted %q; got %q", expected, actual)
		}
	}
}

func (s *UnitTestSuite) TestReady(t *testing.T) {
	gotest.True(t, s.ready)
}
