package gotest_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestfuzz"
)

// FuzzAdapterLifecycle is a top-level stdlib fuzz target — this IS the
// legitimate integration point for gotest.Fuzz, exempt from the
// suites-only idiom. It replays two seed corpus entries and proves that
// beforeEach/afterEach interpose around EACH execution of the fuzz body
// (not just once around the whole fuzz target).
func FuzzAdapterLifecycle(f *testing.F) {
	var order []string
	gf := gotest.NewF(f,
		func(*gotest.T) { order = append(order, "before") },
		func(*gotest.T) { order = append(order, "after") })
	gf.Add("ab")
	gf.Add("cd")

	if gf.F() != f {
		f.Fatalf("gf.F() = %p, want %p (identity passthrough broken)", gf.F(), f)
	}

	gotest.Fuzz(gf, func(t *gotest.T, s string) {
		order = append(order, "body:"+s)

		// This execution's before must sit immediately before this
		// execution's body entry.
		gotest.Equal(t, "before", order[len(order)-2])
		gotest.Equal(t, "body:"+s, order[len(order)-1])

		// Once a second execution has begun, the FIRST execution's
		// after hook must already have run and interposed between the
		// two executions — proving per-execution (not aggregate) hook
		// interposition.
		if len(order) > 3 {
			gotest.Equal(t, []string{"before", "body:ab", "after"}, order[:3])
		}
	})

	// gotest.Fuzz has now returned: under `go test` (seed replay, not
	// live fuzzing) testing.F.Fuzz runs synchronously over the seed
	// corpus, so both executions — including the LAST one's afterEach —
	// have completed by now.
	want := []string{"before", "body:ab", "after", "before", "body:cd", "after"}
	if len(order) != len(want) {
		f.Fatalf("order = %v, want %v", order, want)
	}
	for i, w := range want {
		if order[i] != w {
			f.Fatalf("order[%d] = %q, want %q (full: %v)", i, order[i], w, order)
		}
	}
}

// FuzzAdapterNilHooks proves nil before/after hooks don't panic.
func FuzzAdapterNilHooks(f *testing.F) {
	gf := gotest.NewF(f, nil, nil)
	gf.Add("x")
	gotest.Fuzz(gf, func(t *gotest.T, s string) {
		gotest.NotZero(t, s)
	})
}

// FuzzAdapter2Args proves Fuzz2 passes both arguments through on the
// native path — no fan attached, so the seeds go to the engine as declared.
func FuzzAdapter2Args(f *testing.F) {
	gf := gotest.NewF(f, nil, nil)
	gf.Add("a", 7)
	gotest.Fuzz2(gf, func(t *gotest.T, s string, n int) {
		gotest.Equal(t, "a", s)
		gotest.Equal(t, 7, n)
	})
}

// fanReq is a struct Go's fuzzing engine cannot handle natively — f.Fuzz
// would panic with "unsupported type for fuzzing" without a fan. fanReqFan
// is what codegen emits for it: Email stays a native string leaf, Age rides
// as an 8-byte leaf.
type fanReq struct {
	Email string
	Age   int
}

func fanReqFan() gotestfuzz.Fan[fanReq] {
	return gotestfuzz.Fan[fanReq]{
		Register: func(f *testing.F, run func(*testing.T, fanReq)) {
			f.Fuzz(func(t *testing.T, email string, age []byte) {
				run(t, fanReq{Email: email, Age: gotestfuzz.LeafInt(age)})
			})
		},
		Explode: func(v fanReq) []any { return []any{v.Email, gotestfuzz.LeafBytesInt(v.Age)} },
		Literal: func(v fanReq) string { return fmt.Sprintf("fanReq{Email: %q, Age: %d}", v.Email, v.Age) },
	}
}

// FuzzFanDispatch proves the whole mechanism end to end inside a real fuzz
// target: a typed f.Add seed is exploded into leaves, the target is
// registered through the fan's own (*testing.F).Fuzz call, and the callback
// receives the fanned-in struct — with beforeEach/afterEach interposed per
// execution, exactly as on the native path.
func FuzzFanDispatch(f *testing.F) {
	var order []string
	gf := gotest.NewF(f,
		func(*gotest.T) { order = append(order, "before") },
		func(*gotest.T) { order = append(order, "after") },
		fanReqFan())

	gf.Add(fanReq{Email: "a@b.c", Age: 30})

	gotest.Fuzz(gf, func(t *gotest.T, req fanReq) {
		order = append(order, "body")
		gotest.Equal(t, "before", order[len(order)-2])
		// The seed must survive the explode/fan-in round trip intact.
		if len(order) == 3 {
			gotest.Equal(t, "a@b.c", req.Email)
			gotest.Equal(t, 30, req.Age)
		}
	})

	if len(order) < 3 || order[0] != "before" || order[1] != "body" || order[2] != "after" {
		f.Fatalf("order = %v, want the per-execution before/body/after triple", order)
	}
}

// FuzzFanNativeUnaffected proves an attached fan does not hijack a target
// whose argument type is a pass-through kind — Fuzz[string] must find no
// FuzzFan[string] and take the native path, and the string seed must reach
// the engine untouched.
func FuzzFanNativeUnaffected(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, fanReqFan())
	gf.Add("plain")
	gotest.Fuzz(gf, func(t *gotest.T, s string) {
		gotest.Equal(t, "plain", s)
	})
}

// FuzzFan2Mixed proves a two-argument target fans one position and passes
// the other through, seeds included.
func FuzzFan2Mixed(f *testing.F) {
	fan := gotestfuzz.Fan2[fanReq, string]{
		Register: func(f *testing.F, run func(*testing.T, fanReq, string)) {
			f.Fuzz(func(t *testing.T, email string, age []byte, topic string) {
				run(t, fanReq{Email: email, Age: gotestfuzz.LeafInt(age)}, topic)
			})
		},
		Explode: func(v fanReq, s string) []any { return []any{v.Email, gotestfuzz.LeafBytesInt(v.Age), s} },
		Literal: func(v fanReq, s string) string {
			return fmt.Sprintf("fanReq{Email: %q, Age: %d}, %q", v.Email, v.Age, s)
		},
	}
	gf := gotest.NewF(f, nil, nil, fan)
	gf.Add(fanReq{Email: "x@y.z", Age: 7}, "orders")
	gotest.Fuzz2(gf, func(t *gotest.T, req fanReq, topic string) {
		gotest.Equal(t, "x@y.z", req.Email)
		gotest.Equal(t, 7, req.Age)
		gotest.Equal(t, "orders", topic)
	})
}

// FuzzFanReportsDecodedInputOnFailure proves the fan path prints the
// fanned-in value when an execution fails, which is what makes a struct
// crasher readable at the crash site — go test itself prints no input
// values, only the corpus file path.
//
// The deliberate failure is armed by GOTEST_TEST_FUZZ_FAIL_INPUT (the same
// idiom as GOTEST_TEST_EACH_FAIL_FIRST): unarmed, the target replays its
// seed and passes, so a plain `go test ./pkg/gotest/` stays green. Only
// TestDecodedInputReporting's subprocess arms it to scrape the marker line.
func FuzzFanReportsDecodedInputOnFailure(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, fanReqFan())
	gf.Add(fanReq{Email: "boom"})
	gotest.Fuzz(gf, func(t *gotest.T, v fanReq) {
		if v.Email == "boom" && os.Getenv("GOTEST_TEST_FUZZ_FAIL_INPUT") != "" { //nolint:fail-guard // a deliberate failure trigger, not an assertion about v
			t.Errorf("deliberate failure for input reporting")
		}
	})
}

// FuzzSeedTypeMismatch proves a seed of the wrong type is rejected against
// the target's own type with a message naming both, instead of reaching the
// engine. Armed by env like the echo target above.
func FuzzSeedTypeMismatch(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, fanReqFan())
	if os.Getenv("GOTEST_TEST_FUZZ_BAD_SEED") != "" {
		gf.Add("not a fanReq")
	} else {
		gf.Add(fanReq{Email: "ok"})
	}
	gotest.Fuzz(gf, func(t *gotest.T, v fanReq) {})
}

// FuzzAddAfterFuzz proves a late f.Add is refused with a message, rather
// than being silently dropped from the buffer. Armed by env.
func FuzzAddAfterFuzz(f *testing.F) {
	gf := gotest.NewF(f, nil, nil)
	gf.Add("early")
	gotest.Fuzz(gf, func(t *gotest.T, s string) {})
	if os.Getenv("GOTEST_TEST_FUZZ_LATE_ADD") != "" {
		gf.Add("late")
	}
}

// FWrapperTestSuite is a normal gotest suite covering what's assertable
// about *gotest.F outside of a real fuzz target. *testing.F has no public
// constructor, so an actual *gotest.F can only be built inside a genuine
// fuzz target — the targets above cover dispatch end to end. What remains
// assertable here, without an instance, is the assertion contract and the
// buffered-seed logic.
type FWrapperTestSuite struct{}

func (s *FWrapperTestSuite) TestAssertionContract(t *gotest.T) {
	t.It("satisfies Errorf/FailNow/Skipf/Context like B and T", func(it *gotest.T) {
		var _ interface {
			Errorf(format string, args ...any)
			FailNow()
			Skipf(format string, args ...any)
			Context() context.Context
		} = (*gotest.F)(nil)
	})
}

func (s *FWrapperTestSuite) TestSeedBuffering(t *gotest.T) {
	identity := func(seed []any) ([]any, error) { return seed, nil }

	t.It("keeps every f.Add tuple in order until the target flushes it", func(it *gotest.T) {
		f := gotest.NewF(nil, nil, nil)
		f.Add("a", 1)
		f.Add("b", 2)
		gotest.Equal(it, [][]any{{"a", 1}, {"b", 2}}, gotest.ExportSeeds(f))
	})

	t.It("copies the caller's slice, so f.Add(vals...) is safe to reuse", func(it *gotest.T) {
		f := gotest.NewF(nil, nil, nil)
		vals := []any{"a"}
		f.Add(vals...)
		vals[0] = "mutated"
		gotest.Equal(it, [][]any{{"a"}}, gotest.ExportSeeds(f))
	})

	t.It("explodes each tuple through the target's own explode function", func(it *gotest.T) {
		f := gotest.NewF(nil, nil, nil)
		f.Add(fanReq{Email: "e", Age: 3})
		fan := fanReqFan()
		out, err := gotest.ExportExplodeSeeds(f, 1, func(seed []any) ([]any, error) { return fan.Explode(seed[0].(fanReq)), nil })
		gotest.NoError(it, err)
		gotest.Equal(it, [][]any{{"e", gotestfuzz.LeafBytesInt(3)}}, out)
	})

	t.It("rejects a tuple whose arity is not the target's, naming the seed", func(it *gotest.T) {
		f := gotest.NewF(nil, nil, nil)
		f.Add("a", "b")
		f.Add("only one")
		_, err := gotest.ExportExplodeSeeds(f, 2, identity)
		gotest.ErrorContains(it, err, "seed #2")
		gotest.ErrorContains(it, err, "given 1 value, but this fuzz target takes 2")
	})

	t.It("wraps an explode error with the seed number", func(it *gotest.T) {
		f := gotest.NewF(nil, nil, nil)
		f.Add("fine")
		f.Add("bad")
		_, err := gotest.ExportExplodeSeeds(f, 1, func(seed []any) ([]any, error) {
			if seed[0] == "bad" {
				return nil, errors.New("f.Add was given string, but this fuzz target takes fanReq")
			}
			return seed, nil
		})
		gotest.ErrorContains(it, err, "seed #2: f.Add was given string, but this fuzz target takes fanReq")
	})
}

// runArmedFuzzTarget re-runs one of the env-armed fuzz targets above in a
// subprocess with the arming variable set. The target fails deliberately
// once armed, so "go test" exits non-zero; the error is expected and not
// the thing under test — see e.g. e2e_suite_test.go's identical out, _ :=
// ... idiom. -count=1 defeats the test cache: a cached pass from an unarmed
// run would otherwise be replayed with no output.
func runArmedFuzzTarget(target, armEnv string) string {
	cmd := exec.Command("go", "test", "-count=1", "-run", "^"+target+"$", ".")
	cmd.Env = append(os.Environ(), armEnv+"=1")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func (s *FWrapperTestSuite) TestDecodedInputReporting(t *gotest.T) {
	t.It("prints the fanned-in literal to stderr when an execution fails", func(it *gotest.T) {
		out := runArmedFuzzTarget("FuzzFanReportsDecodedInputOnFailure", "GOTEST_TEST_FUZZ_FAIL_INPUT")
		gotest.Contains(it, out, protocol.FuzzInputPrefix+`fanReq{Email: "boom", Age: 0}`)
	})
}

func (s *FWrapperTestSuite) TestSeedGuards(t *gotest.T) {
	t.It("fails the target when a seed is not the target's type", func(it *gotest.T) {
		out := runArmedFuzzTarget("FuzzSeedTypeMismatch", "GOTEST_TEST_FUZZ_BAD_SEED")
		gotest.Contains(it, out, "seed #1: f.Add was given string, but this fuzz target takes gotest_test.fanReq")
		gotest.Contains(it, out, "FAIL")
	})

	t.It("fails the target when f.Add is called after gotest.Fuzz", func(it *gotest.T) {
		out := runArmedFuzzTarget("FuzzAddAfterFuzz", "GOTEST_TEST_FUZZ_LATE_ADD")
		gotest.Contains(it, out, "f.Add called after gotest.Fuzz")
		gotest.Contains(it, out, "FAIL")
	})
}
