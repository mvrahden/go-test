// Package harness runs a contract a user's type fulfils.
package harness

import "github.com/mvrahden/go-test/pkg/gotest"

// Impl is the contract.
type Impl interface{ TestRoundTrip(t *gotest.T) }

// Run runs the contract on i.
func Run(t *gotest.T, i Impl) { i.TestRoundTrip(t) }

// RunAll runs it on each of impls.
func RunAll(t *gotest.T, impls ...Impl) {
	for _, i := range impls {
		i.TestRoundTrip(t)
	}
}
