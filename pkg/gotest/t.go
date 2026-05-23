package gotest

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

type TestCase func(*T)

func NewT(t *testing.T) *T {
	return &T{t: t}
}

type T struct {
	t         *testing.T
	ctx       context.Context
	collector *collectingT
}

func (t *T) T() *testing.T {
	if t.t == nil {
		panic("gotest: T() called but no underlying *testing.T is available")
	}
	return t.t
}

func (t *T) Context() context.Context {
	if t.ctx != nil {
		return t.ctx
	}
	return t.t.Context()
}

func NewTWithDeadline(t *testing.T, timeout time.Duration) *T {
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return &T{t: t, ctx: ctx}
}

func (t *T) Errorf(format string, args ...any) {
	if t.collector != nil {
		t.collector.Errorf(format, args...)
		return
	}
	assert.SkipInternalFrames(t.t)
	t.t.Errorf(format, args...)
}

func (t *T) FailNow() {
	if t.collector != nil {
		t.collector.FailNow()
		return
	}
	t.t.FailNow()
}

//go:noinline
func execTestFn(testFn func(it *T), it *T) { testFn(it) }

func (t *T) It(description string, testFn func(it *T)) {
	t.t.Run(description, func(t *testing.T) {
		execTestFn(testFn, NewT(t))
	})
}

func (t *T) When(description string, fn func(w *T)) {
	t.t.Run(description, func(tt *testing.T) {
		execTestFn(fn, NewT(tt))
	})
}

func Each[E any](t *T, entries []E) iter.Seq2[*T, E] {
	return func(yield func(*T, E) bool) {
		for i, entry := range entries {
			name := eachEntryName(reflect.ValueOf(entry), i)
			t.t.Run(name, func(tt *testing.T) {
				yield(NewT(tt), entry)
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

// collectingT captures assertion failures without propagating them.
// Used by Eventually/Consistently to suppress intermediate poll failures.
type collectingT struct {
	failed  bool
	message string
}

func (c *collectingT) Errorf(format string, args ...any) {
	c.failed = true
	c.message = fmt.Sprintf(format, args...)
}
func (c *collectingT) FailNow() {
	c.failed = true
	runtime.Goexit()
}

func runCollectingPoll(fn func(poll *T)) (failed bool, message string) {
	c := &collectingT{}
	poll := &T{collector: c}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(poll)
	}()
	<-done
	return c.failed, c.message
}
