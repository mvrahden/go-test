package gotest_test

import (
	"context"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzAdapterLifecycle is a top-level stdlib fuzz target — this IS the
// legitimate integration point for gotest.Fuzz, exempt from the
// suites-only idiom. It replays two seed corpus entries and proves that
// beforeEach/afterEach interpose around EACH execution of the fuzz body
// (not just once around the whole fuzz target).
func FuzzAdapterLifecycle(f *testing.F) {
	f.Add("ab")
	f.Add("cd")
	var order []string
	gf := gotest.NewF(f,
		func(*gotest.T) { order = append(order, "before") },
		func(*gotest.T) { order = append(order, "after") })

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
		// interposition. (The after hook for THIS execution hasn't run
		// yet at this point — it's deferred until fn returns — so the
		// full triple for THIS execution is checked below, after
		// gotest.Fuzz returns.)
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
	f.Add("x")
	gf := gotest.NewF(f, nil, nil)
	gotest.Fuzz(gf, func(t *gotest.T, s string) {
		gotest.NotZero(t, s)
	})
}

// FuzzAdapter2Args proves Fuzz2 passes both arguments through correctly.
func FuzzAdapter2Args(f *testing.F) {
	f.Add("a", 7)
	gf := gotest.NewF(f, nil, nil)
	gotest.Fuzz2(gf, func(t *gotest.T, s string, n int) {
		gotest.Equal(t, "a", s)
		gotest.Equal(t, 7, n)
	})
}

// codecReq is a struct Go's fuzzing engine cannot handle natively —
// f.Fuzz would panic with "unsupported type for fuzzing" without a codec.
type codecReq struct {
	Email string
	Age   int
}

// encodeCodecReq/decodeCodecReq stand in for what codegen emits. The wire
// format is irrelevant here; what is under test is the dispatch.
func encodeCodecReq(v codecReq) []byte {
	return append([]byte{byte(v.Age)}, v.Email...)
}

func decodeCodecReq(b []byte) codecReq {
	if len(b) == 0 {
		return codecReq{}
	}
	return codecReq{Age: int(b[0]), Email: string(b[1:])}
}

func codecReqCodec() gotest.Codec[codecReq] {
	return gotest.Codec[codecReq]{Decode: decodeCodecReq, Encode: encodeCodecReq}
}

// FuzzCodecDispatch proves the whole struct-fuzzing mechanism end to end
// inside a real fuzz target: a typed f.Add seed is encoded on the way in,
// the target is rerouted to a native []byte target, and the callback still
// receives a decoded struct — with beforeEach/afterEach interposed per
// execution, exactly as on the native path.
func FuzzCodecDispatch(f *testing.F) {
	var order []string
	gf := gotest.NewF(f,
		func(*gotest.T) { order = append(order, "before") },
		func(*gotest.T) { order = append(order, "after") },
		codecReqCodec())

	gf.Add(codecReq{Email: "a@b.c", Age: 30})

	gotest.Fuzz(gf, func(t *gotest.T, req codecReq) {
		order = append(order, "body")
		gotest.Equal(t, "before", order[len(order)-2])
		// The seed must survive the encode/decode round trip intact.
		if len(order) == 3 {
			gotest.Equal(t, "a@b.c", req.Email)
			gotest.Equal(t, 30, req.Age)
		}
	})

	if len(order) < 3 || order[0] != "before" || order[1] != "body" || order[2] != "after" {
		f.Fatalf("order = %v, want the per-execution before/body/after triple", order)
	}
}

// FuzzCodecNativeUnaffected proves an attached codec does not hijack a
// target whose argument type Go fuzzes natively — Fuzz[string] must find no
// Codec[string] and take the native path, and Add must leave a string seed
// alone rather than handing testing.F a []byte for a string target.
func FuzzCodecNativeUnaffected(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, codecReqCodec())
	gf.Add("plain")
	gotest.Fuzz(gf, func(t *gotest.T, s string) {
		gotest.Equal(t, "plain", s)
	})
}

// codecOther is a second non-native type, so the package's F carries two
// codecs — the configuration in which a wrong-typed seed used to be claimed
// by its own codec and silently decoded as garbage by the other target.
type codecOther struct{ Label string }

func codecOtherCodec() gotest.Codec[codecOther] {
	return gotest.Codec[codecOther]{
		Decode: func(b []byte) codecOther { return codecOther{Label: string(b)} },
		Encode: func(v codecOther) []byte { return []byte(v.Label) },
	}
}

// FuzzCodecRightTypeSeedWithTwoCodecs is the control for
// TestSeedTypeMismatch below: with two codecs attached, a seed of the
// target's own type must still work untouched.
func FuzzCodecRightTypeSeedWithTwoCodecs(f *testing.F) {
	gf := gotest.NewF(f, nil, nil, codecReqCodec(), codecOtherCodec())
	gf.Add(codecReq{Email: "a@b.c", Age: 30})
	gotest.Fuzz(gf, func(t *gotest.T, req codecReq) {
		gotest.Equal(t, "a@b.c", req.Email)
		gotest.Equal(t, 30, req.Age)
	})
}

// FWrapperTestSuite is a normal gotest suite covering what's assertable
// about *gotest.F outside of a real fuzz target. *testing.F has no public
// constructor, so an actual *gotest.F can only be built inside a genuine
// fuzz target — F() identity, Add forwarding (via seed count driving
// executions), and the generic adapters are exercised end to end by
// FuzzAdapterLifecycle, FuzzAdapterNilHooks, and FuzzAdapter2Args above.
// What remains assertable here, without an instance, is the assertion
// contract *gotest.F promises to satisfy.
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

func (s *FWrapperTestSuite) TestCodecEncoding(t *gotest.T) {
	t.It("claims a value of its own type and re-encodes it", func(it *gotest.T) {
		c := codecReqCodec()
		got := c.Decode(c.Encode(codecReq{Email: "x@y.z", Age: 7}))
		gotest.Equal(it, codecReq{Email: "x@y.z", Age: 7}, got)
	})
}

// TestSeedTypeMismatch pins the guard that stops a seed of the wrong
// non-native type from being silently decoded as an unrelated value. Every
// codec in a package is attached to every F, so without it the wrong seed is
// claimed by its OWN codec, encoded, and handed to a target that reads those
// bytes as garbage — with the target still passing.
func (s *FWrapperTestSuite) TestSeedTypeMismatch(t *gotest.T) {
	// codec index 0 handles codecReq, index 1 handles codecOther
	newTwoCodecF := func() *gotest.F {
		return gotest.NewF(nil, nil, nil, codecReqCodec(), codecOtherCodec())
	}

	t.It("reports no mismatch when the seed matches the target's type", func(it *gotest.T) {
		f := newTwoCodecF()
		gotest.ExportEncodeSeeds(f, []any{codecReq{Email: "a@b.c", Age: 1}})
		gotest.Equal(it, -1, gotest.ExportSeedMismatch(f, 0), "a codecReq seed on a codecReq target is correct")
	})

	t.It("reports the offending codec when the seed is a different non-native type", func(it *gotest.T) {
		f := newTwoCodecF()
		gotest.ExportEncodeSeeds(f, []any{codecOther{Label: "xyz"}})
		gotest.Equal(it, 1, gotest.ExportSeedMismatch(f, 0), "a codecOther seed on a codecReq target must be caught")
	})

	t.It("reports a mismatch when a native target was given a codec-claimed seed", func(it *gotest.T) {
		f := newTwoCodecF()
		gotest.ExportEncodeSeeds(f, []any{codecOther{Label: "xyz"}})
		gotest.Equal(it, 1, gotest.ExportSeedMismatch(f, -1), "native targets must encode no seed at all")
	})

	t.It("leaves native seeds unclaimed, so they never trip the guard", func(it *gotest.T) {
		f := newTwoCodecF()
		out := gotest.ExportEncodeSeeds(f, []any{"plain", 7, []byte{1, 2}})
		gotest.Equal(it, []any{"plain", 7, []byte{1, 2}}, out, "native values must pass through untouched")
		gotest.Equal(it, -1, gotest.ExportSeedMismatch(f, -1))
	})

	t.It("encodes a claimed seed to bytes and leaves the caller's slice intact", func(it *gotest.T) {
		f := newTwoCodecF()
		args := []any{codecOther{Label: "hi"}}
		out := gotest.ExportEncodeSeeds(f, args)
		gotest.Equal[any](it, []byte("hi"), out[0])
		gotest.Equal[any](it, codecOther{Label: "hi"}, args[0], "f.Add(vals...) must not mutate the caller's slice")
	})
}
