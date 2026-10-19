package generic_test

import "refactormod/generic"

var _ = generic.ContractTestSuite[float64]{}

var _ *generic.IntContractTestSuite
