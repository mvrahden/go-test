package xpkgcontract

import "github.com/mvrahden/go-test/tests/canary/testdata/shapes/xpkgcontract/contract"

// IntIndexTestSuite instantiates a contract suite declared in another package.
type IntIndexTestSuite = contract.IndexContractTestSuite[int]
