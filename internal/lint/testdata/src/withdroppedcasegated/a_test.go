package withdroppedcasegated

import "github.com/mvrahden/go-test/pkg/gotest"

// The build constraints leave gated_test.go out of the package the analyzer
// sees; what it names may run, so nothing here is reported.

// gated's case is called only from the constrained file.
type gated struct{}

func (g gated) TestGated(t *gotest.T) {}

// GatedContractTestSuite is instantiated only by the constrained file.
type GatedContractTestSuite[T any] struct{}

func (s *GatedContractTestSuite[T]) TestHolds(t *gotest.T) {}

type OpenTestSuite struct{}

func (s *OpenTestSuite) TestOpens(t *gotest.T) {}
