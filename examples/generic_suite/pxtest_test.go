package genericsuite_test

import "github.com/mvrahden/go-test/pkg/gotest"

type SimpleExtTestSuite struct {
	count int
}

func (s *SimpleExtTestSuite) BeforeEach(t *gotest.T) {
	s.count = 0
}

func (s *SimpleExtTestSuite) TestAlpha(t *gotest.T) {
	gotest.Zero(t, s.count)
}
