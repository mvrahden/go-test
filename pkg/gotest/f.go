package gotest

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotestfuzz"
)

// F wraps *testing.F for fuzz targets, pairing it with optional
// beforeEach/afterEach lifecycle hooks that the generic Fuzz adapters
// interpose around each execution (every seed replay and every generated
// input). It satisfies the same assertion contract as *T, *B, and *R
// (Errorf + FailNow), so the assertion library works against the fuzz
// target itself, e.g. to fail setup outside of any single execution.
//
// It also carries the generated fan adapters for the package's fuzz
// targets — see gotestfuzz.Fan — and buffers f.Add seeds until the
// Fuzz call, which knows the target's shape and explodes them through its
// own fan.
type F struct {
	f          *testing.F
	beforeEach func(*T)
	afterEach  func(*T)
	adapters   []gotestfuzz.Adapter
	seeds      [][]any
	fuzzed     bool
	echoAll    bool
}

// NewF wraps f, pairing it with optional beforeEach/afterEach lifecycle
// hooks. Either hook may be nil. The trailing adapters — supplied by
// generated code, one per fuzz-adapter instantiation in the package whose
// arguments are not all pass-through kinds — are what let Fuzz work with
// types Go's fuzzing engine rejects, and what give numbers a better mutator
// than the engine's own.
func NewF(f *testing.F, beforeEach, afterEach func(*T), adapters ...gotestfuzz.Adapter) *F {
	return &F{
		f: f, beforeEach: beforeEach, afterEach: afterEach, adapters: adapters,
		echoAll: os.Getenv(protocol.EnvFuzzEchoInput) == "1",
	}
}

func (f *F) F() *testing.F                     { return f.f }
func (f *F) Context() context.Context          { return f.f.Context() }
func (f *F) Errorf(format string, args ...any) { f.f.Errorf(format, args...) }
func (f *F) FailNow()                          { f.f.FailNow() }
func (f *F) Skipf(format string, args ...any)  { f.f.Skipf(format, args...) }

// Add records a seed. It is buffered rather than forwarded: only the Fuzz
// call knows which fan the target dispatches through, and a typed seed such
// as a struct must be exploded into that fan's leaves before testing.F.Add
// ever sees it (testing.F.Add panics on any non-native value). Seeds are
// forwarded, in order, at the top of Fuzz/Fuzz2/Fuzz3.
func (f *F) Add(args ...any) {
	if f.fuzzed {
		f.f.Fatalf("f.Add called after gotest.Fuzz — add every seed before the Fuzz call")
		return
	}
	seed := make([]any, len(args))
	copy(seed, args)
	f.seeds = append(f.seeds, seed)
}

// explodeSeeds runs every buffered seed through explode after checking its
// arity, returning the leaf tuples to forward or the first error, numbered
// by seed so the message points at the right f.Add call.
func (f *F) explodeSeeds(arity int, explode func(seed []any) ([]any, error)) ([][]any, error) {
	out := make([][]any, 0, len(f.seeds))
	for i, seed := range f.seeds {
		if len(seed) != arity {
			return nil, fmt.Errorf("seed #%d: f.Add was given %d value%s, but this fuzz target takes %d",
				i+1, len(seed), plural(len(seed)), arity)
		}
		vals, err := explode(seed)
		if err != nil {
			return nil, fmt.Errorf("seed #%d: %w", i+1, err)
		}
		out = append(out, vals)
	}
	return out, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// flushSeeds forwards the buffered seeds to testing.F.Add through explode,
// failing the target on the first bad seed.
func (f *F) flushSeeds(arity int, explode func(seed []any) ([]any, error)) {
	tuples, err := f.explodeSeeds(arity, explode)
	if err != nil {
		f.f.Fatalf("%s", err)
		return
	}
	for _, vals := range tuples {
		f.f.Add(vals...)
	}
	f.seeds = nil
}

// seedAs asserts one seed value to the target's declared type, with a
// message naming both sides.
func seedAs[A any](v any) (A, error) {
	a, ok := v.(A)
	if !ok {
		var zero A
		return zero, fmt.Errorf("f.Add was given %T, but this fuzz target takes %T", v, zero)
	}
	return a, nil
}

// findAdapter returns the adapter of exactly type X attached to f. The
// lookup is one type assertion per adapter per fuzz *target*, not per
// execution.
func findAdapter[X gotestfuzz.Adapter](f *F) (X, bool) {
	for _, ad := range f.adapters {
		if x, ok := ad.(X); ok {
			return x, true
		}
	}
	var zero X
	return zero, false
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

// run is each plus the failure-time echo: when literal is non-nil, the
// execution's input is reported on stderr on panic or t.Failed() — go test
// itself prints no input values on a fuzz failure, only the corpus file
// path — and on every execution when GOTEST_FUZZ_ECHO_INPUT=1, which is how
// triage and promote read a crasher back.
func (f *F) run(t *testing.T, literal func() string, body func(*T)) {
	if literal != nil {
		if f.echoAll {
			reportFuzzInput(literal())
		} else {
			// A panic aborts the process before t.Cleanup runs, so the
			// report has to happen in a defer that survives unwinding. The
			// literal is built here, not above, so the common pass path
			// never constructs it.
			defer func() {
				if r := recover(); r != nil {
					reportFuzzInput(literal())
					panic(r)
				}
				if t.Failed() {
					reportFuzzInput(literal())
				}
			}()
		}
	}
	f.each(t, body)
}

// reportFuzzInput prints literal, the input of a fuzz execution as Go
// source, to stderr for triage and promote to scrape.
func reportFuzzInput(literal string) {
	fmt.Fprintln(os.Stderr, protocol.FuzzInputPrefix+literal)
}

// Fuzz adapts fn to *testing.F.Fuzz for a single-argument fuzz target.
//
// When a gotestfuzz.Fan[A] is attached, the target is registered through the fan's
// own (*testing.F).Fuzz call — A's leaves as separate typed arguments,
// fanned back into an A per execution — and every buffered seed is exploded
// into the same leaves first. Otherwise A is one of the pass-through kinds
// (string, []byte, bool) and testing.F receives the concrete func(*testing.T,
// A) directly. Each execution gets a fresh *T; see each for the lifecycle
// interposition.
func Fuzz[A any](f *F, fn func(*T, A)) {
	f.fuzzed = true
	if fan, ok := findAdapter[gotestfuzz.Fan[A]](f); ok {
		f.flushSeeds(1, func(seed []any) ([]any, error) {
			v, err := seedAs[A](seed[0])
			if err != nil {
				return nil, err
			}
			return fan.Explode(v), nil
		})
		fan.Register(f.f, func(t *testing.T, v A) {
			f.run(t, literalOf(fan.Literal, v), func(tt *T) { fn(tt, v) })
		})
		return
	}
	f.flushSeeds(1, func(seed []any) ([]any, error) {
		if _, err := seedAs[A](seed[0]); err != nil {
			return nil, err
		}
		return seed, nil
	})
	f.f.Fuzz(func(t *testing.T, a A) {
		f.each(t, func(tt *T) { fn(tt, a) })
	})
}

// Fuzz2 is the two-argument form of Fuzz. Each position fans independently
// through the attached gotestfuzz.Fan2[A, B]; pass-through positions stay exactly
// as declared.
func Fuzz2[A, B any](f *F, fn func(*T, A, B)) {
	f.fuzzed = true
	if fan, ok := findAdapter[gotestfuzz.Fan2[A, B]](f); ok {
		f.flushSeeds(2, func(seed []any) ([]any, error) {
			a, err := seedAs[A](seed[0])
			if err != nil {
				return nil, positioned(1, err)
			}
			b, err := seedAs[B](seed[1])
			if err != nil {
				return nil, positioned(2, err)
			}
			return fan.Explode(a, b), nil
		})
		fan.Register(f.f, func(t *testing.T, a A, b B) {
			f.run(t, literalOf2(fan.Literal, a, b), func(tt *T) { fn(tt, a, b) })
		})
		return
	}
	f.flushSeeds(2, func(seed []any) ([]any, error) {
		if _, err := seedAs[A](seed[0]); err != nil {
			return nil, positioned(1, err)
		}
		if _, err := seedAs[B](seed[1]); err != nil {
			return nil, positioned(2, err)
		}
		return seed, nil
	})
	f.f.Fuzz(func(t *testing.T, a A, b B) {
		f.each(t, func(tt *T) { fn(tt, a, b) })
	})
}

// Fuzz3 is the three-argument form of Fuzz, as Fuzz2.
func Fuzz3[A, B, C any](f *F, fn func(*T, A, B, C)) {
	f.fuzzed = true
	if fan, ok := findAdapter[gotestfuzz.Fan3[A, B, C]](f); ok {
		f.flushSeeds(3, func(seed []any) ([]any, error) {
			a, err := seedAs[A](seed[0])
			if err != nil {
				return nil, positioned(1, err)
			}
			b, err := seedAs[B](seed[1])
			if err != nil {
				return nil, positioned(2, err)
			}
			c, err := seedAs[C](seed[2])
			if err != nil {
				return nil, positioned(3, err)
			}
			return fan.Explode(a, b, c), nil
		})
		fan.Register(f.f, func(t *testing.T, a A, b B, c C) {
			f.run(t, literalOf3(fan.Literal, a, b, c), func(tt *T) { fn(tt, a, b, c) })
		})
		return
	}
	f.flushSeeds(3, func(seed []any) ([]any, error) {
		if _, err := seedAs[A](seed[0]); err != nil {
			return nil, positioned(1, err)
		}
		if _, err := seedAs[B](seed[1]); err != nil {
			return nil, positioned(2, err)
		}
		if _, err := seedAs[C](seed[2]); err != nil {
			return nil, positioned(3, err)
		}
		return seed, nil
	})
	f.f.Fuzz(func(t *testing.T, a A, b B, c C) {
		f.each(t, func(tt *T) { fn(tt, a, b, c) })
	})
}

func positioned(pos int, err error) error { return fmt.Errorf("value %d: %w", pos, err) }

// literalOf* defer the literal rendering to the moment it is needed and
// map a nil Literal to no echo at all.
func literalOf[A any](lit func(A) string, a A) func() string {
	if lit == nil {
		return nil
	}
	return func() string { return lit(a) }
}

func literalOf2[A, B any](lit func(A, B) string, a A, b B) func() string {
	if lit == nil {
		return nil
	}
	return func() string { return lit(a, b) }
}

func literalOf3[A, B, C any](lit func(A, B, C) string, a A, b B, c C) func() string {
	if lit == nil {
		return nil
	}
	return func() string { return lit(a, b, c) }
}
