// Package harnessctor carries the constructors pkg/gotest lends
// pkg/gotestruntime: generated code needs a *gotest.T and a *gotest.B whose
// fields only pkg/gotest can set, and the user package exports no
// constructor for them. pkg/gotest sets both at init; the runtime asserts
// the result to its type.
package harnessctor

import "testing"

var (
	// EachT returns the *gotest.T a benchmark's BeforeEach and AfterEach get.
	EachT func(tb testing.TB) any
	// BenchB returns the *gotest.B a suite's benchmark method gets.
	BenchB func(b *testing.B) any
)
