// Package fzcrosspkg instantiates a suite another package declares, whose fuzz
// method's seeds replay here.
package fzcrosspkg

import "github.com/mvrahden/go-test/tests/conformance/testdata/fzcrosspkg/contract"

// IntCountTestSuite instantiates the contract suite.
type IntCountTestSuite = contract.CountContractTestSuite[int]
