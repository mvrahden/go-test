package gotestruntime

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// The values generated benchmark and fuzz wrappers hand to a suite. Each
// function is one call of the pkg/gotest constructor that builds it: the
// fields it sets are unexported there.

// EachT is the *gotest.T a benchmark's BeforeEach and AfterEach get: tb's
// own, with no *testing.T behind it.
func EachT(tb testing.TB) *gotest.T { return gotest.NewTFromTB(tb) }

// BenchB is the *gotest.B a suite's benchmark method gets.
func BenchB(b *testing.B) *gotest.B { return gotest.NewB(b) }

// FuzzTarget is the generated binding of one fuzz wrapper.
type FuzzTarget = gotest.FuzzTarget

// FuzzEach runs one execution of a [FuzzTarget].
type FuzzEach = gotest.FuzzEach

// FuzzF is the *gotest.F a suite's fuzz method gets: f with the suite's
// per-execution hooks and the wrapper's target, nil for a native signature.
func FuzzF(f *testing.F, beforeEach, afterEach func(*gotest.T), target *FuzzTarget) *gotest.F {
	return gotest.NewF(f, beforeEach, afterEach, target)
}

// SeedArity checks one f.Add tuple against the target's arity.
func SeedArity(seed []any, n int) error { return gotest.SeedArity(seed, n) }

// SeedArg asserts seed[i] to A, naming the 1-based position on mismatch.
func SeedArg[A any](seed []any, i int) (A, error) { return gotest.SeedArg[A](seed, i) }
