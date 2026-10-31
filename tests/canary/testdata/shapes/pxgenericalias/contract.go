// Package pxgenericalias declares a generic contract suite its external test
// package instantiates, beside one the external test package declares and
// one from another package.
package pxgenericalias

import "github.com/mvrahden/go-test/pkg/gotest"

// ContractTestSuite is a contract every instantiation runs.
type ContractTestSuite[T any] struct{}

func (s *ContractTestSuite[T]) TestRuns(t *gotest.T) {}
