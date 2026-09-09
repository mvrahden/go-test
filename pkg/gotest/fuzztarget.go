package gotest

import (
	"fmt"
	"reflect"
	"testing"
)

// FuzzEach runs one execution of a fuzz target: t is the engine's, literal
// renders the input for the failure echo (nil when the input has no literal
// form), body calls the user's callback under a fresh *T.
type FuzzEach func(t *testing.T, literal func() string, body func(*T))

// FuzzTarget is the generated binding of one Fuzz<Suite>_<Method> wrapper:
// Register asserts the callback to the exact type the wrapper was generated
// for and registers it with the engine under the fanned leaf signature;
// Explode turns one typed f.Add tuple into those leaves. You never construct
// one; the generated wrapper hands it to NewF.
type FuzzTarget struct {
	Signature string
	Register  func(tf *testing.F, fn any, each FuzzEach) bool
	Explode   func(seed []any) ([]any, error)
}

// SeedArity checks one f.Add tuple against the target's declared arity.
func SeedArity(seed []any, n int) error {
	if len(seed) != n {
		return fmt.Errorf("f.Add was given %d value%s, but this fuzz target takes %d", len(seed), plural(len(seed)), n)
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

var gotestTType = reflect.TypeOf((*T)(nil))

// fuzzWithoutTarget binds fn by reflection — the path for a top-level
// stdlib FuzzX using gotest as a plain library, where no wrapper generated a
// target. fn must be func(*gotest.T, <engine-native>...); seeds pass through
// with an arity check only, so the engine reports a bad seed type itself.
func (f *F) fuzzWithoutTarget(fn any) {
	fv := reflect.ValueOf(fn)
	ft := fv.Type()
	if ft.Kind() != reflect.Func || ft.NumIn() < 1 || ft.In(0) != gotestTType || ft.NumOut() != 0 || ft.IsVariadic() {
		f.f.Fatalf("f.Fuzz: callback is %T, want func(*gotest.T, ...)", fn)
		return
	}
	arity := ft.NumIn() - 1
	f.flushSeeds(func(seed []any) ([]any, error) {
		if len(seed) != arity {
			return nil, fmt.Errorf("f.Add was given %d value%s, but this fuzz target takes %d", len(seed), plural(len(seed)), arity)
		}
		return seed, nil
	})

	in := make([]reflect.Type, ft.NumIn())
	in[0] = reflect.TypeOf((*testing.T)(nil))
	for i := 1; i < ft.NumIn(); i++ {
		in[i] = ft.In(i)
	}
	engineFn := reflect.MakeFunc(reflect.FuncOf(in, nil, false), func(args []reflect.Value) []reflect.Value {
		t := args[0].Interface().(*testing.T)
		f.each(t, func(tt *T) {
			call := append([]reflect.Value{reflect.ValueOf(tt)}, args[1:]...)
			fv.Call(call)
		})
		return nil
	})
	f.f.Fuzz(engineFn.Interface())
}
