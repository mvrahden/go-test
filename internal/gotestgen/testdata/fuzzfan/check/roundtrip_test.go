package fuzzfan

import (
	"math"
	"reflect"
	"testing"
)

// intPtr exists because Go has no address-of-literal syntax for a basic
// type — "&42" is not an expression — so a *int test value needs a named
// variable to take the address of.
func intPtr(v int) *int { return &v }

// richCases are the seeds FuzzRichRoundTrip replays, in a slice because
// replay order is f.Add order and the target matches executions to cases
// by index. Empty slices are written as nil on purpose: the fan collapses
// an empty pass-through []byte and an empty hybrid slice to nil, so nil is
// the shape that round-trips exactly.
func richCases() []Rich {
	return []Rich{
		{},
		{
			Name: "alice", Blob: []byte{1, 2, 3}, OK: true,
			I: -42, I8: -8, I16: -1600, I32: -320000, I64: -6400000000,
			U: 42, U8: 8, U16: 1600, U32: 320000, U64: 6400000000,
			F32: 1.5, F64: -2.25,
			Prio: 3, Tag: "urgent",
			Tags:     []string{"a", "", "ccc"},
			Grid:     [3]int8{-1, 0, 1},
			Home:     &Address{Street: "Main", Zip: 12345},
			Nested:   Address{Street: "Nested", Zip: 1},
			Counters: []Address{{Street: "x", Zip: 2}, {Street: "y", Zip: 3}},
			Count:    intPtr(-17),
		},
		{Name: "bob", Home: nil, Tags: nil, Counters: nil, Count: nil},
		{
			I: math.MaxInt, I8: math.MinInt8, I64: math.MinInt64,
			U: math.MaxUint, U64: math.MaxUint64,
			F64: math.MaxFloat64, F32: math.SmallestNonzeroFloat32,
		},
		{Name: "nan", F64: math.NaN(), F32: float32(math.NaN())},
		{Name: "inf", F64: math.Inf(1), F32: float32(math.Inf(-1))},
	}
}

// equalRich is reflect.DeepEqual with NaN treated as equal to NaN — the
// engine must be able to carry NaN through a seed, and DeepEqual would
// call that a mismatch.
func equalRich(want, got Rich) bool {
	if math.IsNaN(want.F64) || math.IsNaN(float64(want.F32)) {
		if !math.IsNaN(got.F64) || !math.IsNaN(float64(got.F32)) {
			return false
		}
		want.F64, got.F64 = 0, 0
		want.F32, got.F32 = 0, 0
	}
	return reflect.DeepEqual(want, got)
}

// FuzzRichRoundTrip is the property that keeps a promoted crasher
// meaningful, proved through the engine itself: every seed is exploded into
// leaves by the generated fan-out, handed to the real (*testing.F).Fuzz via
// the generated register function, fanned back in, and must equal the seed.
// Under `go test` the seeds replay synchronously in f.Add order.
func FuzzRichRoundTrip(f *testing.F) {
	cases := richCases()
	for _, c := range cases {
		f.Add(ƒ_fuzzout_v1_Rich(c)...)
	}
	i := 0
	ƒ_fuzzreg_v1_Rich(f, func(t *testing.T, got Rich) {
		if i >= len(cases) {
			return // a live fuzzing run past the seeds; nothing to compare
		}
		want := cases[i]
		i++
		if !equalRich(want, got) {
			t.Fatalf("round trip mismatch for case %d\n want: %#v\n  got: %#v", i-1, want, got)
		}
	})
}

// TestHybridTotality is the constraint the mini-codec exists to satisfy for
// the leaves that still ride through it: no byte string may make the
// decoder panic or refuse. A rejected execution is a wasted execution.
func TestHybridTotality(t *testing.T) {
	// A deterministic LCG rather than math/rand: this test must fail the
	// same way on every machine and every run.
	state := uint64(0x2545F4914F6CDD1D)
	next := func() byte {
		state = state*6364136223846793005 + 1442695040888963407
		return byte(state >> 33)
	}
	for size := 0; size < 260; size++ {
		buf := make([]byte, size)
		for i := range buf {
			buf[i] = next()
		}
		ƒ_fuzzdec_v1_slice_Address(buf) // must not panic
		ƒ_fuzzdec_v1_slice_string(buf)  // must not panic
	}
	for _, fill := range []byte{0x00, 0xFF} {
		for size := 0; size < 64; size++ {
			buf := make([]byte, size)
			for i := range buf {
				buf[i] = fill
			}
			ƒ_fuzzdec_v1_slice_Address(buf)
		}
	}
}
