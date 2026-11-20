package gotestruntime

import (
	"fmt"
	"testing"

	"github.com/mvrahden/go-test/internal/harnessctor"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// The values generated benchmark and fuzz wrappers hand to a suite.

// EachT is the *gotest.T a benchmark's BeforeEach and AfterEach get: tb's
// own, with no *testing.T behind it.
func EachT(tb testing.TB) *gotest.T {
	return harnessctor.EachT(tb).(*gotest.T) //nolint:forcetypeassert // pkg/gotest builds exactly this type
}

// BenchB is the *gotest.B a suite's benchmark method gets.
func BenchB(b *testing.B) *gotest.B {
	return harnessctor.BenchB(b).(*gotest.B) //nolint:forcetypeassert // pkg/gotest builds exactly this type
}

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
func SeedArity(seed []any, n int) error {
	if len(seed) != n {
		s := "s"
		if len(seed) == 1 {
			s = ""
		}
		return fmt.Errorf("f.Add was given %d value%s, but this fuzz target takes %d", len(seed), s, n)
	}
	return nil
}

// SeedArg asserts seed[i] to A, naming the 1-based position on mismatch.
func SeedArg[A any](seed []any, i int) (A, error) {
	a, ok := seed[i].(A)
	if !ok {
		var zero A
		return zero, fmt.Errorf("value %d: f.Add was given %T, but this fuzz target takes %T", i+1, seed[i], zero)
	}
	return a, nil
}
