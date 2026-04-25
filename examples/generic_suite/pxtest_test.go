package genericsuite_test

import (
	genericsuite "github.com/mvrahden/go-test/examples/generic_suite"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type SimpleExtTestSuite struct{}

func (s *SimpleExtTestSuite) BeforeEach(t *gotest.T) {
	genericsuite.Noop()
}

func (s *SimpleExtTestSuite) TestAlpha(t *gotest.T) {
	genericsuite.Noop()
}
