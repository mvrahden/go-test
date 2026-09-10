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

// FuzzAdapterLifecycle is a top-level stdlib fuzz target using gotest as a
// plain library: no generated target, so f.Fuzz binds the callback by
// reflection. It replays two seeds and proves beforeEach/afterEach
// interpose around EACH execution, not once around the whole target.
func FuzzAdapterLifecycle(f *testing.F) {
	var order []string
	gf := gotest.NewF(f,
		func(*gotest.T) { order = append(order, "before") },
		func(*gotest.T) { order = append(order, "after") },
		nil)
	gf.Add("ab")
	gf.Add("cd")

	if gf.F() != f {
		f.Fatalf("gf.F() = %p, want %p (identity passthrough broken)", gf.F(), f)
	}

	gf.Fuzz(func(t *gotest.T, s string) {
		order = append(order, "body:"+s)
		gotest.Equal(t, "before", order[len(order)-2])
		gotest.Equal(t, "body:"+s, order[len(order)-1])
		if len(order) > 3 {
			gotest.Equal(t, []string{"before", "body:ab", "after"}, order[:3])
		}
	})

	// Under go test the seeds replay synchronously, so the last afterEach
	// has run by now.
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
	gf := gotest.NewF(f, nil, nil, nil)
	gf.Add("x")
	gf.Fuzz(func(t *gotest.T, s string) {
		gotest.NotZero(t, s)
	})
}

// FuzzAdapter2Args proves the reflective binding passes every native
// argument through, seeds included.
func FuzzAdapter2Args(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, nil)
	gf.Add("a", 7)
	gf.Fuzz(func(t *gotest.T, s string, n int) {
		gotest.Equal(t, "a", s)
		gotest.Equal(t, 7, n)
	})
}

// fanReq is a struct Go's fuzzing engine cannot handle natively. fanReqTarget
// is what codegen emits for a target over it: Email stays a native string
// leaf, Age rides as an 8-byte leaf, and the callback is bound by its exact
// type.
type fanReq struct {
	Email string
	Age   int
}

func fanReqLiteral(v fanReq) string {
	return fmt.Sprintf("fanReq{Email: %q, Age: %d}", v.Email, v.Age)
}

func fanReqTarget() *gotest.FuzzTarget {
	return &gotest.FuzzTarget{
		Signature: "func(*gotest.T, fanReq)",
		Register: func(tf *testing.F, fn any, each gotest.FuzzEach) bool {
			run, ok := fn.(func(*gotest.T, fanReq))
			if !ok {
				return false
			}
			tf.Fuzz(func(t *testing.T, email string, age []byte) {
				v := fanReq{Email: email, Age: gotestfuzz.LeafInt(age)}
				each(t, func() string { return fanReqLiteral(v) }, func(tt *gotest.T) { run(tt, v) })
			})
			return true
		},
		Explode: func(seed []any) ([]any, error) {
			if len(seed) != 1 {
				return nil, fmt.Errorf("f.Add was given %d values, but this fuzz target takes 1", len(seed))
			}
			v, err := gotest.SeedArg[fanReq](seed, 0)
			if err != nil {
				return nil, err
			}
			return []any{v.Email, gotestfuzz.LeafBytesInt(v.Age)}, nil
		},
	}
}

// FuzzTargetDispatch proves the mechanism end to end: a typed seed is
// exploded into leaves, the callback is bound through the target's own
// (*testing.F).Fuzz call, and it receives the fanned-in struct with the
// lifecycle interposed per execution.
func FuzzTargetDispatch(f *testing.F) {
	var order []string
	gf := gotest.NewF(f,
		func(*gotest.T) { order = append(order, "before") },
		func(*gotest.T) { order = append(order, "after") },
		fanReqTarget())

	gf.Add(fanReq{Email: "a@b.c", Age: 30})

	gf.Fuzz(func(t *gotest.T, req fanReq) {
		order = append(order, "body")
		gotest.Equal(t, "before", order[len(order)-2])
		if len(order) == 3 {
			gotest.Equal(t, "a@b.c", req.Email)
			gotest.Equal(t, 30, req.Age)
		}
	})

	if len(order) < 3 || order[0] != "before" || order[1] != "body" || order[2] != "after" {
		f.Fatalf("order = %v, want the per-execution before/body/after triple", order)
	}
}

// FuzzTargetMixed proves a two-value target fans one position and passes
// the other through, seeds included.
func FuzzTargetMixed(f *testing.F) {
	target := &gotest.FuzzTarget{
		Signature: "func(*gotest.T, fanReq, string)",
		Register: func(tf *testing.F, fn any, each gotest.FuzzEach) bool {
			run, ok := fn.(func(*gotest.T, fanReq, string))
			if !ok {
				return false
			}
			tf.Fuzz(func(t *testing.T, email string, age []byte, topic string) {
				v := fanReq{Email: email, Age: gotestfuzz.LeafInt(age)}
				each(t, nil, func(tt *gotest.T) { run(tt, v, topic) })
			})
			return true
		},
		Explode: func(seed []any) ([]any, error) {
			if len(seed) != 2 {
				return nil, fmt.Errorf("f.Add was given %d values, but this fuzz target takes 2", len(seed))
			}
			v, err := gotest.SeedArg[fanReq](seed, 0)
			if err != nil {
				return nil, err
			}
			s, err := gotest.SeedArg[string](seed, 1)
			if err != nil {
				return nil, err
			}
			return []any{v.Email, gotestfuzz.LeafBytesInt(v.Age), s}, nil
		},
	}
	gf := gotest.NewF(f, nil, nil, target)
	gf.Add(fanReq{Email: "x@y.z", Age: 7}, "orders")
	gf.Fuzz(func(t *gotest.T, req fanReq, topic string) {
		gotest.Equal(t, "x@y.z", req.Email)
		gotest.Equal(t, 7, req.Age)
		gotest.Equal(t, "orders", topic)
	})
}

// FuzzTargetReportsDecodedInputOnFailure proves a failing execution prints
// the fanned-in literal — go test itself prints only the corpus path. Armed
// by GOTEST_TEST_FUZZ_FAIL_INPUT; unarmed it replays its seed and passes.
func FuzzTargetReportsDecodedInputOnFailure(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, fanReqTarget())
	gf.Add(fanReq{Email: "boom"})
	gf.Fuzz(func(t *gotest.T, v fanReq) {
		if v.Email == "boom" && os.Getenv("GOTEST_TEST_FUZZ_FAIL_INPUT") != "" { //nolint:fail-guard // a deliberate failure trigger, not an assertion about v
			t.Errorf("deliberate failure for input reporting")
		}
	})
}

// FuzzHandleContext proves the handle's context is the engine's own.
func FuzzHandleContext(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, nil)
	if gf.Context() != f.Context() {
		f.Fatalf("gf.Context() is not f.Context()")
	}
	gf.Add("x")
	gf.Fuzz(func(t *gotest.T, s string) {})
}

// FuzzHandleSkipf is armed by GOTEST_TEST_FUZZ_SKIP: Skipf must skip the
// whole target before any seed replays.
func FuzzHandleSkipf(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, nil)
	if os.Getenv("GOTEST_TEST_FUZZ_SKIP") != "" {
		gf.Skipf("no corpus on %s", "this host")
	}
	gf.Add("x")
	gf.Fuzz(func(t *gotest.T, s string) {
		gotest.Empty(t, os.Getenv("GOTEST_TEST_FUZZ_SKIP"), "a seed replayed after Skipf")
	})
}

// FuzzHandleErrorf is armed by GOTEST_TEST_FUZZ_ERRORF: Errorf marks the
// target failed and returns; testing.F then runs no seeds for a failed
// target, so the callback never executes.
func FuzzHandleErrorf(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, nil)
	if os.Getenv("GOTEST_TEST_FUZZ_ERRORF") != "" {
		gf.Errorf("setup broke: %d", 7)
		fmt.Println("continued after Errorf")
	}
	gf.Add("x")
	gf.Fuzz(func(t *gotest.T, s string) {
		fmt.Println("seed replayed after Errorf")
	})
}

// FuzzHandleFailNow is armed by GOTEST_TEST_FUZZ_FAILNOW: FailNow stops the
// target at once, so nothing after it runs.
func FuzzHandleFailNow(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, nil)
	if os.Getenv("GOTEST_TEST_FUZZ_FAILNOW") != "" {
		gf.Errorf("giving up")
		gf.FailNow()
		fmt.Println("unreachable after FailNow")
	}
	gf.Add("x")
	gf.Fuzz(func(t *gotest.T, s string) {})
}

// FuzzSeedTypeMismatch proves a seed of the wrong type is rejected against
// the target's own type, naming both. Armed by env.
func FuzzSeedTypeMismatch(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, fanReqTarget())
	if os.Getenv("GOTEST_TEST_FUZZ_BAD_SEED") != "" {
		gf.Add("not a fanReq")
	} else {
		gf.Add(fanReq{Email: "ok"})
	}
	gf.Fuzz(func(t *gotest.T, v fanReq) {})
}

// FuzzWrongCallbackShape proves a callback the generated target was not
// built for fails the target with the expected signature named. Armed by
// env.
func FuzzWrongCallbackShape(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, fanReqTarget())
	gf.Add(fanReq{Email: "ok"})
	if os.Getenv("GOTEST_TEST_FUZZ_WRONG_SHAPE") != "" {
		gf.Fuzz(func(t *gotest.T, s string) {})
		return
	}
	gf.Fuzz(func(t *gotest.T, v fanReq) {})
}

// FuzzAddAfterFuzz proves a late f.Add is refused with a message. Armed by env.
func FuzzAddAfterFuzz(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, nil)
	gf.Add("early")
	gf.Fuzz(func(t *gotest.T, s string) {})
	if os.Getenv("GOTEST_TEST_FUZZ_LATE_ADD") != "" {
		gf.Add("late")
	}
}

// FWrapperTestSuite covers what is assertable about *gotest.F outside a
// real fuzz target: the assertion contract and the buffered-seed logic.
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
		f := gotest.NewF(nil, nil, nil, nil)
		f.Add("a", 1)
		f.Add("b", 2)
		gotest.Equal(it, [][]any{{"a", 1}, {"b", 2}}, gotest.ExportSeeds(f))
	})

	t.It("copies the caller's slice, so f.Add(vals...) is safe to reuse", func(it *gotest.T) {
		f := gotest.NewF(nil, nil, nil, nil)
		vals := []any{"a"}
		f.Add(vals...)
		vals[0] = "mutated"
		gotest.Equal(it, [][]any{{"a"}}, gotest.ExportSeeds(f))
	})

	t.It("explodes each tuple through the target's own explode function", func(it *gotest.T) {
		f := gotest.NewF(nil, nil, nil, nil)
		f.Add(fanReq{Email: "e", Age: 3})
		out, err := gotest.ExportExplodeSeeds(f, fanReqTarget().Explode)
		gotest.NoError(it, err)
		gotest.Equal(it, [][]any{{"e", gotestfuzz.LeafBytesInt(3)}}, out)
	})

	t.It("numbers the seed an explode error came from", func(it *gotest.T) {
		f := gotest.NewF(nil, nil, nil, nil)
		f.Add("fine")
		f.Add("bad")
		_, err := gotest.ExportExplodeSeeds(f, func(seed []any) ([]any, error) {
			if seed[0] == "bad" {
				return nil, errors.New("f.Add was given string, but this fuzz target takes fanReq")
			}
			return seed, nil
		})
		gotest.ErrorContains(it, err, "seed #2: f.Add was given string, but this fuzz target takes fanReq")
		_, err = gotest.ExportExplodeSeeds(f, identity)
		gotest.NoError(it, err)
	})

	t.It("names the position and both types when a seed value has the wrong type", func(it *gotest.T) {
		_, err := gotest.SeedArg[fanReq]([]any{"x", "y"}, 1)
		gotest.ErrorContains(it, err, "value 2: f.Add was given string, but this fuzz target takes gotest_test.fanReq")
	})
}

// runArmedFuzzTarget re-runs one of the env-armed targets above in a
// subprocess with the arming variable set. The target fails deliberately
// once armed, so the non-zero exit is expected. -count=1 defeats the cache.
func runArmedFuzzTarget(target, armEnv string) string {
	cmd := exec.Command("go", "test", "-count=1", "-v", "-run", "^"+target+"$", ".") //nolint:gosec // G204: target is a test-local constant, not user input
	cmd.Env = append(os.Environ(), armEnv+"=1")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func (s *FWrapperTestSuite) TestDecodedInputReporting(t *gotest.T) {
	t.It("prints the fanned-in literal to stderr when an execution fails", func(it *gotest.T) {
		out := runArmedFuzzTarget("FuzzTargetReportsDecodedInputOnFailure", "GOTEST_TEST_FUZZ_FAIL_INPUT")
		gotest.Contains(it, out, protocol.FuzzInputPrefix+`fanReq{Email: "boom", Age: 0}`)
	})
}

func (s *FWrapperTestSuite) TestHandleDelegation(t *gotest.T) {
	t.It("skips the whole target on Skipf, before any seed replays", func(it *gotest.T) {
		out := runArmedFuzzTarget("FuzzHandleSkipf", "GOTEST_TEST_FUZZ_SKIP")
		gotest.Contains(it, out, "--- SKIP: FuzzHandleSkipf")
		gotest.Contains(it, out, "no corpus on this host")
		gotest.NotContains(it, out, "a seed replayed after Skipf")
	})

	t.It("fails the target on Errorf and returns; the engine then runs no seeds", func(it *gotest.T) {
		out := runArmedFuzzTarget("FuzzHandleErrorf", "GOTEST_TEST_FUZZ_ERRORF")
		gotest.Contains(it, out, "--- FAIL: FuzzHandleErrorf")
		gotest.Contains(it, out, "setup broke: 7")
		gotest.Contains(it, out, "continued after Errorf")
		gotest.NotContains(it, out, "seed replayed after Errorf")
	})

	t.It("stops the target at FailNow", func(it *gotest.T) {
		out := runArmedFuzzTarget("FuzzHandleFailNow", "GOTEST_TEST_FUZZ_FAILNOW")
		gotest.Contains(it, out, "--- FAIL: FuzzHandleFailNow")
		gotest.Contains(it, out, "giving up")
		gotest.NotContains(it, out, "unreachable after FailNow")
	})

	t.It("attributes a diagnostic to the caller, not to the handle", func(it *gotest.T) {
		// Without Helper() every Errorf/Skipf would read "f.go:44:", which
		// sends the user into gotest's source instead of their own.
		for _, target := range [][2]string{
			{"FuzzHandleErrorf", "GOTEST_TEST_FUZZ_ERRORF"},
			{"FuzzHandleSkipf", "GOTEST_TEST_FUZZ_SKIP"},
		} {
			out := runArmedFuzzTarget(target[0], target[1])
			gotest.Contains(it, out, "f_suite_test.go:", "%s output:\n%s", target[0], out)
			gotest.NotContains(it, out, "f.go:", "%s output:\n%s", target[0], out)
		}
	})
}

func (s *FWrapperTestSuite) TestSeedGuards(t *gotest.T) {
	t.It("fails the target when a seed is not the target's type", func(it *gotest.T) {
		out := runArmedFuzzTarget("FuzzSeedTypeMismatch", "GOTEST_TEST_FUZZ_BAD_SEED")
		gotest.Contains(it, out, "seed #1: value 1: f.Add was given string, but this fuzz target takes gotest_test.fanReq")
		gotest.Contains(it, out, "FAIL")
	})

	t.It("fails the target when f.Add is called after f.Fuzz", func(it *gotest.T) {
		out := runArmedFuzzTarget("FuzzAddAfterFuzz", "GOTEST_TEST_FUZZ_LATE_ADD")
		gotest.Contains(it, out, "f.Add called after f.Fuzz")
		gotest.Contains(it, out, "FAIL")
	})

	t.It("fails the target when the callback is not the shape it was generated for", func(it *gotest.T) {
		out := runArmedFuzzTarget("FuzzWrongCallbackShape", "GOTEST_TEST_FUZZ_WRONG_SHAPE")
		gotest.Contains(it, out, "f.Fuzz: callback is func(*gotest.T, string), but this target was generated for func(*gotest.T, fanReq)")
		gotest.Contains(it, out, "FAIL")
	})
}
