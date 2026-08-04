package gotest

import (
	"fmt"
	"iter"
	"reflect"
	"sync"
	"testing"
)

// Each iterates over entries as sub-tests, yielding a fresh *T and the entry for each.
func Each[E any](t *T, entries []E) iter.Seq2[*T, E] {
	return func(yield func(*T, E) bool) {
		for i, entry := range entries {
			name := eachEntryName(reflect.ValueOf(entry), i)
			if !eachRun(t, name, entry, yield) {
				break
			}
		}
	}
}

// eachRun creates a named subtest and calls yield from the calling goroutine
// (not the subtest goroutine). This satisfies Go's range-over-func contract
// that yield must be called from the same goroutine as the iterator.
func eachRun[E any](parent *T, name string, entry E, yield func(*T, E) bool) bool {
	ready := make(chan *testing.T, 1)
	done := make(chan struct{})
	finished := make(chan struct{})

	go func() {
		parent.t.Run(name, func(tt *testing.T) {
			ready <- tt
			<-done
		})
		close(finished)
	}()

	// t.Run does not always run the closure: a subtest filtered out by
	// -run/-skip, or suppressed after -failfast tripped, returns immediately.
	// Waiting on ready alone deadlocked the binary until -test.timeout killed
	// it — with no AfterAll and no fixture teardown — the moment anyone reran
	// a single table entry by name. Observe testing instead of assuming it:
	// finished-first means this entry was not selected, so skip it and let the
	// iteration continue, exactly as go test treats the subtest.
	var tt *testing.T
	select {
	case tt = <-ready:
	case <-finished:
		return true
	}

	// goexited stays true when yield leaves via runtime.Goexit — a failed
	// assertion inside the entry. A panic is tracked separately: the two must
	// not be confused, because the Goexit branch below would discard an
	// in-flight panic instead of letting it propagate.
	goexited := true
	var captured *capturedPanic
	var result bool

	// Deferred, so the subtest is released and awaited even when yield leaves
	// via Goexit — that unwinds straight past the inner closure below.
	defer func() {
		close(done)
		<-finished
		if captured != nil {
			panic(captured)
		}
		if goexited && tt.Failed() {
			parent.t.FailNow()
		}
	}()

	func() {
		defer func() { captured = capturedFrom(recover(), "Each entry") }()
		result = yield(parent.sub(tt), entry)
		goexited = false
	}()
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

// Go runs fn on a new goroutine and returns a function that waits for it.
//
// A panic in a goroutine the test starts itself is unrecoverable: Go terminates
// the whole process without running any other goroutine's deferred work, so no
// AfterEach, no AfterAll and no fixture teardown happens, and the panic is
// attributed to nothing in particular. There is no way for a framework to guard
// a goroutine it did not create — so this creates it for you, captures the panic
// with the stack from where it happened, and re-raises it on the test's own
// goroutine, where the testing package handles it like any other test panic.
//
// For work that finishes on its own, wait for it where you want the panic to
// surface:
//
//	wait := gotest.Go(t, func() { report = build(input) })
//	defer wait()
//
// For a goroutine that runs until something stops it — a server, a poller —
// do not wait inside the test. The wait is registered as test cleanup, which
// runs after AfterEach and after any later t.Cleanup, so whatever stops the
// goroutine has already run by the time anything waits for it:
//
//	gotest.Go(t, func() { srv.Serve(l) })   // AfterEach closes l
//
// A `defer wait()` here would deadlock instead: the test's own defers run
// before AfterEach, so it would wait for a goroutine nothing has stopped yet.
//
// Calling the returned function more than once is safe.
func Go(t *T, fn func()) (wait func()) {
	done := make(chan struct{})
	var captured *capturedPanic

	go func() {
		defer close(done)
		defer func() { captured = capturedFrom(recover(), "goroutine started by gotest.Go") }()
		fn()
	}()

	var once sync.Once
	wait = func() {
		once.Do(func() {
			<-done
			if captured != nil {
				panic(captured)
			}
		})
	}
	t.t.Cleanup(wait)
	return wait
}
