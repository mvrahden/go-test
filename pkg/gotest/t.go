package gotest

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

func NewT(t *testing.T) *T {
	return &T{t: t}
}

type T struct {
	t   *testing.T
	ctx context.Context
}

func (t *T) T() *testing.T {
	return t.t
}

func (t *T) Context() context.Context {
	if t.ctx != nil {
		return t.ctx
	}
	return t.t.Context()
}

// NewTWithDeadline wraps t with a context that expires after timeout, so a test
// can hold work to a tighter budget than the suite's own. The context is derived
// from t.Context() and is released when the test ends.
func NewTWithDeadline(t *testing.T, timeout time.Duration) *T {
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return &T{t: t, ctx: ctx}
}

// NewTWithContext wraps t, overriding what [T.Context] reports. Use it when the
// context cannot be expressed as a deadline off t.Context() — injected values,
// or a lifetime that has to differ from the test's own.
//
// The caller owns the context; nothing here cancels it.
func NewTWithContext(t *testing.T, ctx context.Context) *T {
	return &T{t: t, ctx: ctx}
}

func (t *T) Errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if goFrame := assert.SkipInternalFrames(t.t); goFrame != "" {
		msg = strings.TrimPrefix(msg, goFrame+": ")
	}
	t.t.Errorf("%s", msg)
}

func (t *T) FailNow() {
	t.t.FailNow()
}

func (t *T) Skipf(format string, args ...any) {
	t.t.Skipf(format, args...)
}

func (t *T) Setenv(key, value string) {
	t.t.Setenv(key, value)
}

func (t *T) TempDir() string {
	return t.t.TempDir()
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
