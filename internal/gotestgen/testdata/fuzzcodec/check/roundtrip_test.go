package fuzzcodec

import (
	"math"
	"reflect"
	"testing"
)

// intPtr exists because Go has no address-of-literal syntax for a basic
// type — "&42" is not an expression — so a *int test value needs a named
// variable to take the address of.
func intPtr(v int) *int { return &v }

func richCases() map[string]Rich {
	return map[string]Rich{
		"zero": {},
		"populated": {
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
		"nil pointer, empty slices": {
			Name: "bob", Home: nil, Tags: nil, Counters: nil, Count: nil,
		},
		"extreme numerics": {
			I: math.MaxInt, I8: math.MinInt8, I64: math.MinInt64,
			U: math.MaxUint, U64: math.MaxUint64,
			F64: math.MaxFloat64, F32: math.SmallestNonzeroFloat32,
		},
	}
}

// TestRoundTrip is the property that keeps a promoted crasher meaningful:
// whatever f.Add encoded, the decoder must hand back unchanged.
func TestRoundTrip(t *testing.T) {
	for name, want := range richCases() {
		t.Run(name, func(t *testing.T) {
			got := ƒ_fuzzdec_v1_Rich(ƒ_fuzzenc_v1_Rich(want))
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("round trip mismatch\n want: %#v\n  got: %#v", want, got)
			}
		})
	}
}

// TestRoundTripNaN checks NaN by bits — NaN != NaN under DeepEqual, but
// fuzzing must still be able to reach it.
func TestRoundTripNaN(t *testing.T) {
	got := ƒ_fuzzdec_v1_Rich(ƒ_fuzzenc_v1_Rich(Rich{F64: math.NaN(), F32: float32(math.NaN())}))
	if !math.IsNaN(got.F64) || !math.IsNaN(float64(got.F32)) {
		t.Fatalf("NaN did not survive the round trip: %v %v", got.F64, got.F32)
	}
}

// TestDecoderTotality is the constraint the whole wire format exists to
// satisfy: no byte string may make the decoder panic or refuse. A rejected
// execution is a wasted execution.
func TestDecoderTotality(t *testing.T) {
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
		ƒ_fuzzdec_v1_Rich(buf) // must not panic
	}
	// All-zero and all-0xFF inputs are the two shapes minimisation drives
	// toward; check them explicitly.
	for _, fill := range []byte{0x00, 0xFF} {
		for size := 0; size < 64; size++ {
			buf := make([]byte, size)
			for i := range buf {
				buf[i] = fill
			}
			ƒ_fuzzdec_v1_Rich(buf) // must not panic
		}
	}
}

// FuzzDecoderTotality is the same property, available to a real fuzzing run
// (`go test -fuzz=FuzzDecoderTotality`). CI covers totality via the
// deterministic loop above; this target is for when someone wants to push on
// it. gotest fuzzing gotest's own fuzz support is the honest way to prove
// the decoder never rejects.
func FuzzDecoderTotality(f *testing.F) {
	f.Add([]byte(nil))
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	f.Fuzz(func(t *testing.T, raw []byte) {
		ƒ_fuzzdec_v1_Rich(raw) // must not panic
	})
}
