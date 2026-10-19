package gotest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/callsite"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/runstate"
	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

// F wraps *testing.F for fuzz targets, pairing it with optional
// beforeEach/afterEach lifecycle hooks that the generic Fuzz adapters
// interpose around each execution (every seed replay and every generated
// input). It satisfies the same assertion contract as *T, *B, and *R
// (Errorf + FailNow), so the assertion library works against the fuzz
// target itself, e.g. to fail setup outside of any single execution.
//
// It also carries the generated FuzzTarget of its wrapper and buffers f.Add
// seeds until the Fuzz call, which explodes them through that target.
type F struct {
	f          *testing.F
	beforeEach func(*T)
	afterEach  func(*T)
	target     *FuzzTarget
	seeds      [][]any
	seedAt     []string // the file:line of each seed's f.Add call
	fuzzed     bool
	echoAll    bool
}

// NewF wraps f, pairing it with optional beforeEach/afterEach lifecycle
// hooks (either may be nil) and the generated target of its wrapper. A nil
// target is the plain-library case: Fuzz then binds by reflection and only
// engine-native argument types work.
func NewF(f *testing.F, beforeEach, afterEach func(*T), target *FuzzTarget) *F {
	return &F{
		f: f, beforeEach: beforeEach, afterEach: afterEach, target: target,
		echoAll: os.Getenv(protocol.EnvFuzzEchoInput) == "1",
	}
}

func (f *F) F() *testing.F            { return f.f }
func (f *F) Context() context.Context { return f.f.Context() }

// Errorf and Skipf name the caller's line, not gotest's.
func (f *F) Errorf(format string, args ...any) { report(f.f, fmt.Sprintf(format, args...)) }
func (f *F) FailNow()                          { f.f.FailNow() }

// Name is the fuzz target's name, as testing gave it.
func (f *F) Name() string {
	if f.f == nil {
		return ""
	}
	return f.f.Name()
}
func (f *F) Skipf(format string, args ...any) {
	assert.SkipInternalFrames(f.f)
	f.f.Skipf(format, args...)
}

// Add records a seed. It is buffered rather than forwarded: a typed seed
// such as a struct must be exploded into the target's leaves before
// testing.F.Add ever sees it (testing.F.Add panics on any non-native
// value). Seeds are forwarded, in order, at the top of Fuzz.
func (f *F) Add(args ...any) {
	if f.fuzzed {
		assert.SkipInternalFrames(f.f)
		f.f.Fatalf("f.Add called after f.Fuzz — add every seed before the Fuzz call")
		return
	}
	seed := make([]any, len(args))
	copy(seed, args)
	f.seeds = append(f.seeds, seed)
	at := ""
	if _, file, line, ok := runtime.Caller(1); ok {
		at = callsite.Format(file, line)
	}
	f.seedAt = append(f.seedAt, at)
}

// seedError is a seed explode refused, by its index among the f.Add calls.
type seedError struct {
	index int
	err   error
}

func (e *seedError) Error() string { return fmt.Sprintf("seed #%d: %v", e.index+1, e.err) }
func (e *seedError) Unwrap() error { return e.err }

// explodeSeeds runs every buffered seed through explode (which checks the
// tuple's arity and types itself), returning the leaf tuples to forward or
// the first error, numbered by seed so the message points at the right
// f.Add call.
func (f *F) explodeSeeds(explode func(seed []any) ([]any, error)) ([][]any, error) {
	out := make([][]any, 0, len(f.seeds))
	for i, seed := range f.seeds {
		vals, err := explode(seed)
		if err != nil {
			return nil, &seedError{index: i, err: err}
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
// failing the target on the first bad seed, located at its f.Add call.
func (f *F) flushSeeds(explode func(seed []any) ([]any, error)) {
	tuples, err := f.explodeSeeds(explode)
	if err != nil {
		var se *seedError
		if errors.As(err, &se) && se.index < len(f.seedAt) && f.seedAt[se.index] != "" {
			// Undecorated: testing would name this line, not the f.Add call.
			fmt.Fprintln(f.f.Output(), strings.ReplaceAll(f.seedAt[se.index]+": "+err.Error(), "\n", "\n    "))
			f.f.FailNow()
			return
		}
		f.f.Fatalf("%s", err)
		return
	}
	for _, vals := range tuples {
		f.f.Add(vals...)
	}
	f.seeds = nil
}

// each runs body under a fresh *T with beforeEach (if non-nil) immediately
// before it and afterEach (if non-nil) deferred to immediately after —
// interposed per execution, not once for the whole fuzz target. The defer
// comes first: a beforeEach that fails, skips or panics still gets its
// afterEach, as a test method's hooks pair. A panic is re-raised after
// afterEach, which a fatal assertion there would otherwise discard. Once
// the run is stopped, an execution that has not started is skipped with the
// reason "run stopping", and one running gets the stop's grace.
func (f *F) each(t *testing.T, body func(*T)) {
	if runstate.StopContext().Err() != nil {
		fmt.Fprintln(t.Output(), "run stopping")
		t.SkipNow()
	}
	tt := NewT(t)
	if f.afterEach != nil {
		defer func() {
			if p := capturedFrom(recover(), "the fuzz execution, ahead of its AfterEach"); p != nil {
				defer panic(p)
			}
			f.afterEach(tt)
		}()
	}
	if f.beforeEach != nil {
		f.beforeEach(tt)
	}
	defer runstate.EnterBody(t, "a fuzz execution")()
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

// Fuzz binds fn, a func(*gotest.T, ...), to the engine through the
// generated target, which checks the callback's exact type. Seeds buffered
// by Add are exploded through the target first. Without a target the
// binding is reflective; see fuzzWithoutTarget.
func (f *F) Fuzz(fn any) {
	f.fuzzed = true
	f.claimInterrupt()
	if f.target == nil {
		f.fuzzWithoutTarget(fn)
		return
	}
	f.flushSeeds(f.target.Explode)
	if !f.target.Register(f.f, fn, f.run) {
		f.f.Fatalf("f.Fuzz: callback is %T, but this target was generated for %s — the generated wrapper is stale, re-run gotest", fn, f.target.Signature)
	}
}

// claimInterrupt records that Go's fuzzing engine is about to take this target:
// in the coordinator, for the target -test.fuzz selects. The engine stops on an
// interrupt by itself, saving what it found, so the runtime leaves it alone
// from here on. Before — ordinary tests, other targets' seeds — it does not.
func (f *F) claimInterrupt() {
	pattern := runstate.FuzzTarget()
	if pattern == "" || runstate.FuzzWorker() || f.f == nil {
		return
	}
	if ok, err := regexp.MatchString(pattern, f.f.Name()); err == nil && ok {
		runstate.SetFuzzEngine()
	}
}
