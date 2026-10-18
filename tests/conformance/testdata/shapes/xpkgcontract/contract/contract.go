// Package contract holds a generic contract suite other packages instantiate.
package contract

import "github.com/mvrahden/go-test/pkg/gotest"

// IndexContractTestSuite is a contract every instantiation runs.
type IndexContractTestSuite[T any] struct{}

func (s *IndexContractTestSuite[T]) TestZeroValue(t *gotest.T) {
	var v T
	gotest.Equal(t, v, *new(T))
}
