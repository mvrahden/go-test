package withdroppedcase

import "github.com/mvrahden/go-test/pkg/gotest"

// Checker is declared outside the test files: another package may call it.
type Checker struct{}

func (c *Checker) TestReady(t *gotest.T) {}

// SharedContractTestSuite is declared outside the test files.
type SharedContractTestSuite[T any] struct{}

func (s *SharedContractTestSuite[T]) TestHolds(t *gotest.T) {}
