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
		for i, entry := range entries {
			name := eachEntryName(reflect.ValueOf(entry), i)
			if !eachRun(t.t, name, entry, yield) {
				break
			}
		}
	}
}

// eachRun creates a named subtest and calls yield from the calling goroutine
// (not the subtest goroutine). This satisfies Go's range-over-func contract
// that yield must be called from the same goroutine as the iterator.
func eachRun[E any](parent *testing.T, name string, entry E, yield func(*T, E) bool) bool {
	ready := make(chan *testing.T, 1)
	done := make(chan struct{})
	finished := make(chan struct{})

	go func() {
		parent.Run(name, func(tt *testing.T) {
			ready <- tt
			<-done
		})
		close(finished)
	}()

	tt := <-ready
	goexited := true
	defer func() {
		close(done)
		<-finished
		if goexited && tt.Failed() {
			parent.FailNow()
		}
	}()

	result := yield(NewT(tt), entry)
	goexited = false
	return result
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
