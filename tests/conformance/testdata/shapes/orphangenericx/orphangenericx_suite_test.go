package orphangenericx

import "github.com/mvrahden/go-test/pkg/gotest"

// X_ContractTestSuite is generic, excluded and instantiated by no alias.
type X_ContractTestSuite[T any] struct{}

func (s *X_ContractTestSuite[T]) TestZero(t *gotest.T) { gotest.Fail(t, "never runs") }

// RealTestSuite runs beside it.
type RealTestSuite struct{}

func (s *RealTestSuite) TestRuns(t *gotest.T) {}
