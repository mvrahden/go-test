package gotest

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

type TestCase func(*T)

func NewT(t *testing.T) *T {
	return &T{t: t}
}

type T struct {
	t *testing.T
}

func (t *T) T() *testing.T { return t.t }
func (t *T) Errorf(format string, args ...any) {
	t.t.Helper()
	t.t.Errorf(format, args...)
}
func (t *T) FailNow() { t.t.FailNow() }
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
func (t *T) Assert(v any) *assert.BaseAssertionContext {
	return assert.NewAssertionContext(v, t.t)
}
