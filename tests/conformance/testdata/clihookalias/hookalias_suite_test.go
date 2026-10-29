// Package clihookalias instantiates a suite declared in another package whose
// hooks call t.T() under a benchmark wrapper; the generator refuses it.
package clihookalias

import "github.com/mvrahden/go-test/tests/conformance/testdata/clihookalias/contract"

// IntHookTestSuite instantiates the contract suite.
type IntHookTestSuite = contract.HookContractTestSuite[int]
