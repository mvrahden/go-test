// Package contract holds a generic contract suite another package's external
// test package instantiates.
package contract

import "github.com/mvrahden/go-test/pkg/gotest"

// IndexContractTestSuite is a contract every instantiation runs.
type IndexContractTestSuite[T any] struct{}

func (s *IndexContractTestSuite[T]) TestZeroValue(t *gotest.T) {
	var v T
	gotest.Equal(t, v, *new(T))
}
