package simplesuite_test

import "github.com/mvrahden/go-test/pkg/gotest"

type SimpleExtTestSuite struct {
	value string
}

func (s *SimpleExtTestSuite) BeforeEach(t *gotest.T) {
	s.value = "initialized"
}

func (s *SimpleExtTestSuite) TestValueIsSet(t *gotest.T) {
	gotest.NotZero(t, s.value)
}
