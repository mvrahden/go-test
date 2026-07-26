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
	return &T{t: t, tb: t}
}

// NewTFromTB adapts a non-*testing.T testing type (*testing.B, *testing.F)
// so suite lifecycle hooks written against *gotest.T can run under
// benchmarks and fuzz targets. T() returns nil and It/When panic on a
// TB-backed T; the collector statically rejects suites that would hit either.
func NewTFromTB(tb testing.TB) *T { return &T{tb: tb} }

type T struct {
	t   *testing.T
	tb  testing.TB
	ctx context.Context
}

func (t *T) T() *testing.T {
	return t.t
}

func (t *T) testingTB() testing.TB {
	if t.t != nil {
		return t.t
	}
	return t.tb
}

func (t *T) Context() context.Context {
	if t.ctx != nil {
		return t.ctx
	}
	return t.testingTB().Context()
}

// NewTWithDeadline wraps t with a context that expires after timeout, so a test
// can hold work to a tighter budget than the suite's own. The context is derived
// from t.Context() and is released when the test ends.
func NewTWithDeadline(t *testing.T, timeout time.Duration) *T {
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return &T{t: t, tb: t, ctx: ctx}
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
	if t.t != nil {
		if goFrame := assert.SkipInternalFrames(t.t); goFrame != "" {
			msg = strings.TrimPrefix(msg, goFrame+": ")
		}
	}
	t.testingTB().Errorf("%s", msg)
}

func (t *T) FailNow() {
	t.testingTB().FailNow()
}

func (t *T) Skipf(format string, args ...any) {
	t.testingTB().Skipf(format, args...)
}

func (t *T) Setenv(key, value string) {
	t.testingTB().Setenv(key, value)
}

func (t *T) TempDir() string {
	return t.testingTB().TempDir()
}

//go:noinline
func execTestFn(testFn func(it *T), it *T) { testFn(it) }

// sub builds the *T for a nested behavior. A nested subtest gets its own context
// from the testing package, which carries none of the parent's deadline — so a
// suite's configured Timeout would silently stop applying the moment a test
// entered a When or an It. Deriving from the parent keeps its deadline and
// values, and registering the cancel on the child ends it with the child.
func (t *T) sub(tt *testing.T) *T {
	if t.ctx == nil {
		return NewT(tt)
	}
	ctx, cancel := context.WithCancel(t.ctx)
	tt.Cleanup(cancel)
	return NewTWithContext(tt, ctx)
}

func (t *T) It(description string, testFn func(it *T)) {
	if t.t == nil {
		panic("gotest: It/When are unavailable in benchmark/fuzz lifecycle hooks")
	}
	t.t.Run(description, func(tt *testing.T) {
		execTestFn(testFn, t.sub(tt))
	})
}

func (t *T) When(description string, fn func(w *T)) {
	if t.t == nil {
		panic("gotest: It/When are unavailable in benchmark/fuzz lifecycle hooks")
	}
	t.t.Run(description, func(tt *testing.T) {
		execTestFn(fn, t.sub(tt))
	})
}
