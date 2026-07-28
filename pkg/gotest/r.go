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

// Record runs fn with a fresh *R in a dedicated goroutine and returns
// the recorder after fn completes. The goroutine is required because
// FailNow calls runtime.Goexit.
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
