package gotest

import (
	"fmt"
	"runtime"
)

// R is an assertion recorder that captures assertion outcomes without
// propagating them to the test runner. It satisfies the same contract
// as *testing.T for assertion functions (Errorf + FailNow), making it
// the callback type for Eventually and Consistently.
//
// Use Record to run a function with a fresh *R in a dedicated goroutine.
type R struct {
	failed  bool
	message string
	// name is the polling test's, for a poll's recorder; empty otherwise.
	name string
}

// nameOf is the test name t answers to: a recorder's polling test, else
// Name() when t has one, else "".
func nameOf(t testingT) string {
	if r, ok := t.(*R); ok {
		return r.name
	}
	if named, ok := t.(interface{ Name() string }); ok {
		return named.Name()
	}
	return ""
}

func (r *R) Errorf(format string, args ...any) {
	r.failed = true
	r.message = fmt.Sprintf(format, args...)
}

func (r *R) FailNow() {
	r.failed = true
	runtime.Goexit()
}

func (r *R) Failed() bool    { return r.failed }
func (r *R) Message() string { return r.message }

// Record is for testing an assertion helper: it runs fn with a fresh *R and
// returns it, so the test can ask whether the helper failed and with what
// message. fn runs in a dedicated goroutine because FailNow calls
// runtime.Goexit.
//
// A panic inside fn is carried back to the calling goroutine and re-raised
// there. Left unrecovered it would abort the process from a goroutine the
// testing package knows nothing about, so no cleanup would run at all — not the
// suite's AfterAll, not any package fixture's teardown. Re-raising it on the
// test's own goroutine puts it back on testing's normal panic path.
func Record(fn func(*R)) *R {
	r := &R{}
	done := make(chan struct{})
	var captured *capturedPanic
	go func() {
		defer close(done)
		defer func() { captured = capturedFrom(recover(), "poll function") }()
		fn(r)
	}()
	<-done
	if captured != nil {
		panic(captured)
	}
	return r
}
