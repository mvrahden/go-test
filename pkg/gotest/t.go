package gotest

import (
	"context"
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
