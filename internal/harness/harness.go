// Package harness holds what pkg/gotest shares with pkg/gotestruntime for
// generated code only, so the user package exports none of it: the binding
// of a generated fuzz wrapper, and the constructors of values whose fields
// only pkg/gotest can set. Its types are generic in the *gotest.T they hand
// out, which this package cannot name.
package harness

import "testing"

// FuzzEach runs one execution of a fuzz target: t is the engine's, literal
// renders the input for the failure echo (nil when the input has no literal
// form), body calls the user's callback under a fresh T.
type FuzzEach[T any] func(t *testing.T, literal func() string, body func(T))

// FuzzTarget is the generated binding of one Fuzz<Suite>_<Method> wrapper:
// Register asserts the callback to the exact type the wrapper was generated
// for and registers it with the engine under the fanned leaf signature;
// Explode turns one typed f.Add tuple into those leaves.
type FuzzTarget[T any] struct {
	Signature string
	Register  func(tf *testing.F, fn any, each FuzzEach[T]) bool
	Explode   func(seed []any) ([]any, error)
}

// Constructors builds the values generated code hands a suite.
type Constructors[T, B, F any] struct {
	// EachT is the T a benchmark's BeforeEach and AfterEach get.
	EachT func(tb testing.TB) T
	// BenchB is the B a suite's benchmark method gets.
	BenchB func(b *testing.B) B
	// FuzzF is the F a suite's fuzz method gets: f with the suite's
	// per-execution hooks and the wrapper's target.
	FuzzF func(f *testing.F, beforeEach, afterEach func(T), target *FuzzTarget[T]) F
	// Count counts the assertions of t, a test method, and reports them on
	// t as it ends.
	Count func(t *testing.T)
}

// Registered is the Constructors[*gotest.T, *gotest.B, *gotest.F] pkg/gotest
// sets at init.
var Registered any
