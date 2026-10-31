package pxgenericalias_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/shapes/pxgenericalias"
	"github.com/mvrahden/go-test/tests/conformance/testdata/shapes/pxgenericalias/contract"
)

// IntContractTestSuite instantiates the package's own contract.
type IntContractTestSuite = pxgenericalias.ContractTestSuite[int]

// LocalContractTestSuite is a contract the external test package declares.
type LocalContractTestSuite[T any] struct{}

func (s *LocalContractTestSuite[T]) TestLocal(t *gotest.T) {}

// IntLocalTestSuite instantiates the external test package's contract.
type IntLocalTestSuite = LocalContractTestSuite[int]

// IntIndexTestSuite instantiates a contract from another package.
type IntIndexTestSuite = contract.IndexContractTestSuite[int]
