package orphangeneric

import "github.com/mvrahden/go-test/pkg/gotest"

// OrphanContractTestSuite is generic and no alias instantiates it.
type OrphanContractTestSuite[T any] struct{}

func (s *OrphanContractTestSuite[T]) TestZero(t *gotest.T) { gotest.Fail(t, "never runs") }

// RealTestSuite runs beside it.
type RealTestSuite struct{}

func (s *RealTestSuite) TestRuns(t *gotest.T) {}
