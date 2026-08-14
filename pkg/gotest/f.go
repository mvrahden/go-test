package gotest

import (
	"context"
	"testing"
)

// F wraps *testing.F for fuzz targets, pairing it with optional
// beforeEach/afterEach lifecycle hooks that the generic Fuzz adapters
// interpose around each execution (every seed replay and every generated
// input). It satisfies the same assertion contract as *T, *B, and *R
// (Errorf + FailNow), so the assertion library works against the fuzz
// target itself, e.g. to fail setup outside of any single execution.
type F struct {
	f          *testing.F
	beforeEach func(*T)
	afterEach  func(*T)
}

// NewF wraps f, pairing it with optional beforeEach/afterEach lifecycle
// hooks. Either hook may be nil.
func NewF(f *testing.F, beforeEach, afterEach func(*T)) *F {
	return &F{f: f, beforeEach: beforeEach, afterEach: afterEach}
}

func (f *F) F() *testing.F                     { return f.f }
func (f *F) Add(args ...any)                   { f.f.Add(args...) }
func (f *F) Context() context.Context          { return f.f.Context() }
func (f *F) Errorf(format string, args ...any) { f.f.Errorf(format, args...) }
func (f *F) FailNow()                          { f.f.FailNow() }
func (f *F) Skipf(format string, args ...any)  { f.f.Skipf(format, args...) }

// Fuzz adapts fn to *testing.F.Fuzz for a single-argument fuzz target.
// testing.F.Fuzz receives a concrete func(*testing.T, A); its own internal
// reflection validates the signature (A must be in Go's fuzzable type
// set), so gotest performs zero reflection here. Each execution gets a
// fresh *T: beforeEach (if non-nil) runs immediately before fn, afterEach
// (if non-nil) is deferred to run immediately after fn returns — both
// interposed per execution, not once for the whole fuzz target.
func Fuzz[A any](f *F, fn func(*T, A)) {
	f.f.Fuzz(func(t *testing.T, a A) {
		tt := NewT(t)
		if f.beforeEach != nil {
			f.beforeEach(tt)
		}
		if f.afterEach != nil {
			defer f.afterEach(tt)
		}
		fn(tt, a)
	})
}

// Fuzz2 is the two-argument form of Fuzz.
func Fuzz2[A, B any](f *F, fn func(*T, A, B)) {
	f.f.Fuzz(func(t *testing.T, a A, b B) {
		tt := NewT(t)
		if f.beforeEach != nil {
			f.beforeEach(tt)
		}
		if f.afterEach != nil {
			defer f.afterEach(tt)
		}
		fn(tt, a, b)
	})
}

// Fuzz3 is the three-argument form of Fuzz.
func Fuzz3[A, B, C any](f *F, fn func(*T, A, B, C)) {
	f.f.Fuzz(func(t *testing.T, a A, b B, c C) {
		tt := NewT(t)
		if f.beforeEach != nil {
			f.beforeEach(tt)
		}
		if f.afterEach != nil {
			defer f.afterEach(tt)
		}
		fn(tt, a, b, c)
	})
}
