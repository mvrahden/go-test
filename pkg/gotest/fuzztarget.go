package gotest

import (
	"fmt"
	"reflect"
	"testing"
)

var gotestTType = reflect.TypeOf((*T)(nil))

// fuzzWithoutTarget binds fn by reflection — the path for a top-level
// stdlib FuzzX using gotest as a plain library, where no wrapper generated a
// target. fn must be func(*gotest.T, <engine-native>...); each seed's arity
// and value types are checked against it, so a bad seed is reported at its
// f.Add call.
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
		for i, v := range seed {
			if want := ft.In(i + 1); reflect.TypeOf(v) != want {
				return nil, fmt.Errorf("value %d: f.Add was given %T, but this fuzz target takes %s", i+1, v, want)
			}
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
