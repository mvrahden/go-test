package gotest

import (
	"fmt"
	"testing"
)

// Harnesses an earlier gotest generated call these names. They stay so such
// a harness keeps compiling; this tree's generator calls the runtime's.

// NewTFromTB adapts a *testing.B or *testing.F for a suite's hooks.
//
// Deprecated: generated code calls gotestruntime.EachT. The name stays for
// harnesses written by gotest before v1.39.
func NewTFromTB(tb testing.TB) *T { return &T{tb: tb} }

// NewB wraps b for a suite's benchmark method.
//
// Deprecated: generated code calls gotestruntime.BenchB. The name stays for
// harnesses written by gotest before v1.39. An assertion takes a *testing.B
// as it is.
func NewB(b *testing.B) *B { return &B{b: b} }

// Deprecated: generated code calls gotestruntime.SeedArity. The name stays
// for harnesses written by gotest before v1.39.
func SeedArity(seed []any, n int) error {
	if len(seed) != n {
		return fmt.Errorf("f.Add was given %d value%s, but this fuzz target takes %d", len(seed), plural(len(seed)), n)
	}
	return nil
}

// Deprecated: generated code calls gotestruntime.SeedArg. The name stays for
// harnesses written by gotest before v1.39.
func SeedArg[A any](seed []any, i int) (A, error) {
	a, ok := seed[i].(A)
	if !ok {
		var zero A
		return zero, fmt.Errorf("value %d: f.Add was given %T, but this fuzz target takes %T", i+1, seed[i], zero)
	}
	return a, nil
}
