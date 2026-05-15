package genericsuite_test

import "github.com/mvrahden/go-test/pkg/gotest"

type GenericExtTestSuite struct {
	count int
}

func (s *GenericExtTestSuite) BeforeEach(t *gotest.T) {
	s.count = 0
}

func (s *GenericExtTestSuite) TestAlpha(t *gotest.T) {
	gotest.Zero(t, s.count)
}
