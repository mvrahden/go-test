package gotest

import (
	"fmt"
	"iter"
	"reflect"
	"testing"
)

// Each iterates over entries as sub-tests, yielding a fresh *T and the entry for each.
func Each[E any](t *T, entries []E) iter.Seq2[*T, E] {
	return func(yield func(*T, E) bool) {
		var stop bool
		for i, entry := range entries {
			if stop {
				break
			}
			name := eachEntryName(reflect.ValueOf(entry), i)
			t.t.Run(name, func(tt *testing.T) {
				if !yield(NewT(tt), entry) {
					stop = true
				}
			})
		}
	}
}

func eachEntryName(v reflect.Value, index int) string {
	if v.Kind() == reflect.Struct {
		for _, field := range []string{"Desc", "Name"} {
			f := v.FieldByName(field)
			if f.IsValid() && f.Kind() == reflect.String && f.String() != "" {
				return f.String()
			}
		}
	}
	return fmt.Sprintf("#%d", index)
}

// Must extracts the value from a (value, error) or (value, bool) pair, panicking on failure.
func Must[T any](val T, ok any) T {
	switch v := ok.(type) {
	case nil:
		return val
	case bool:
		if v {
			return val
		}
		panic("Must: got false")
	case error:
		panic(fmt.Sprintf("Must: got error: %v", v))
	default:
		panic(fmt.Sprintf("Must: unsupported ok type %T", v))
	}
}
