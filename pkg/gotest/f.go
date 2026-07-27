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
//
// It also carries the generated codecs for any non-native argument type the
// package fuzzes — see Codec.
type F struct {
	f          *testing.F
	beforeEach func(*T)
	afterEach  func(*T)
	codecs     []fuzzCodec
	// seeded holds the indices into codecs of every codec that claimed at
	// least one Add argument. Indices, not the codecs themselves: Codec[A]
	// has func fields, so comparing two fuzzCodec interface values holding
	// them would panic with "comparing uncomparable type".
	seeded []int
}

// NewF wraps f, pairing it with optional beforeEach/afterEach lifecycle
// hooks. Either hook may be nil. The trailing codecs — supplied by generated
// code, one per non-native fuzz argument type in the package — are what let
// Fuzz and Add work with types Go's fuzzing engine rejects.
func NewF(f *testing.F, beforeEach, afterEach func(*T), codecs ...fuzzCodec) *F {
	return &F{f: f, beforeEach: beforeEach, afterEach: afterEach, codecs: codecs}
}

func (f *F) F() *testing.F                     { return f.f }
func (f *F) Context() context.Context          { return f.f.Context() }
func (f *F) Errorf(format string, args ...any) { f.f.Errorf(format, args...) }
func (f *F) FailNow()                          { f.f.FailNow() }
func (f *F) Skipf(format string, args ...any)  { f.f.Skipf(format, args...) }

// Add forwards seed values to testing.F.Add, first encoding any argument a
// codec claims. The interception has to happen here: testing.F.Add panics
// with "unsupported type to Add" before gotest would otherwise see the
// value. Arguments no codec claims pass through untouched, so a native seed
// on a native target behaves exactly as before.
func (f *F) Add(args ...any) {
	f.f.Add(f.encodeSeeds(args)...)
}

// encodeSeeds returns args with every value a codec claims replaced by that
// codec's encoding, recording which codecs claimed something. args is never
// modified — it may be the caller's own slice via f.Add(vals...).
func (f *F) encodeSeeds(args []any) []any {
	if len(f.codecs) == 0 || len(args) == 0 {
		return args
	}
	encoded := make([]any, len(args))
	copy(encoded, args)
	for i, a := range args {
		for ci, c := range f.codecs {
			if b, ok := c.encodeAny(a); ok {
				encoded[i] = b
				f.recordSeedCodec(ci)
				break
			}
		}
	}
	return encoded
}

// recordSeedCodec notes that codecs[ci] encoded a seed, so Fuzz can later
// check the seed's type against the type the target actually fuzzes.
func (f *F) recordSeedCodec(ci int) {
	for _, seen := range f.seeded {
		if seen == ci {
			return
		}
	}
	f.seeded = append(f.seeded, ci)
}

// checkSeeds fails the target when a seed was encoded by a codec other than
// the one this target dispatches through (want == -1 for the native path,
// which must not have encoded any seed at all).
//
// Every codec in the package is attached to every F, so without this check a
// seed of the wrong type is claimed by *its own* codec, encoded, and handed
// to a target that decodes those bytes as an unrelated value — silently, and
// with the target still passing. Go's own fuzzing engine catches the
// equivalent mistake on native types by panicking; this restores that.
func (f *F) checkSeeds(want int) {
	if got := f.seedMismatch(want); got >= 0 {
		f.f.Fatalf("f.Add was given a seed of the type handled by %T, but this fuzz target takes a different type — "+
			"those bytes would decode as an unrelated value; check the f.Add calls in this fuzz method", f.codecs[got])
	}
}

// seedMismatch returns the index of the first codec that encoded a seed but
// is not the codec this target dispatches through, or -1 when every seed
// matches. Split from checkSeeds so the decision is testable without a
// *testing.F, which has no public constructor.
func (f *F) seedMismatch(want int) int {
	for _, got := range f.seeded {
		if got != want {
			return got
		}
	}
	return -1
}

// each runs body under a fresh *T with beforeEach (if non-nil) immediately
// before it and afterEach (if non-nil) deferred to immediately after —
// interposed per execution, not once for the whole fuzz target.
func (f *F) each(t *testing.T, body func(*T)) {
	tt := NewT(t)
	if f.beforeEach != nil {
		f.beforeEach(tt)
	}
	if f.afterEach != nil {
		defer f.afterEach(tt)
	}
	body(tt)
}

// Fuzz adapts fn to *testing.F.Fuzz for a single-argument fuzz target.
//
// When a Codec[A] is attached, the target is rerouted to a native []byte
// target and A is decoded per execution — this is what makes struct-typed
// callbacks work. The lookup is one interface type assertion per fuzz
// target, not per execution.
//
// Otherwise testing.F.Fuzz receives a concrete func(*testing.T, A); its own
// internal reflection validates the signature (A must be in Go's fuzzable
// type set), so gotest performs zero reflection here. Each execution gets a
// fresh *T; see each for the lifecycle interposition.
func Fuzz[A any](f *F, fn func(*T, A)) {
	for i, c := range f.codecs {
		codec, ok := c.(Codec[A])
		if !ok {
			continue
		}
		f.checkSeeds(i)
		f.f.Fuzz(func(t *testing.T, raw []byte) {
			f.each(t, func(tt *T) { fn(tt, codec.Decode(raw)) })
		})
		return
	}
	f.checkSeeds(-1)
	f.f.Fuzz(func(t *testing.T, a A) {
		f.each(t, func(tt *T) { fn(tt, a) })
	})
}

// Fuzz2 is the two-argument form of Fuzz. Multi-argument targets are native
// types only: codegen rejects a non-native argument here at generation time,
// pointing you at a single-struct Fuzz target instead.
func Fuzz2[A, B any](f *F, fn func(*T, A, B)) {
	f.checkSeeds(-1)
	f.f.Fuzz(func(t *testing.T, a A, b B) {
		f.each(t, func(tt *T) { fn(tt, a, b) })
	})
}

// Fuzz3 is the three-argument form of Fuzz. Native types only, as Fuzz2.
func Fuzz3[A, B, C any](f *F, fn func(*T, A, B, C)) {
	f.checkSeeds(-1)
	f.f.Fuzz(func(t *testing.T, a A, b B, c C) {
		f.each(t, func(tt *T) { fn(tt, a, b, c) })
	})
}
