package gotestfuzz_test

import (
	"math"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestfuzz"
)

// FuzzBytesTestSuite covers the total byte-cursor primitives every generated
// fuzz decoder is built from. Totality is the property that matters: a
// decoder that can reject its input wastes a coverage-guided execution.
type FuzzBytesTestSuite struct{}

func (s *FuzzBytesTestSuite) TestScalarRoundTrip(t *gotest.T) {
	t.It("round-trips every fixed-width scalar in declaration order", func(it *gotest.T) {
		w := gotestfuzz.NewWriter()
		w.Bool(true)
		w.Int8(-8)
		w.Int16(-1600)
		w.Int32(-320000)
		w.Int64(-6400000000)
		w.Int(-42)
		w.Uint8(8)
		w.Uint16(1600)
		w.Uint32(320000)
		w.Uint64(6400000000)
		w.Uint(42)
		w.Float32(1.5)
		w.Float64(-2.25)

		r := gotestfuzz.NewReader(w.Out())
		gotest.True(it, r.Bool())
		gotest.Equal(it, int8(-8), r.Int8())
		gotest.Equal(it, int16(-1600), r.Int16())
		gotest.Equal(it, int32(-320000), r.Int32())
		gotest.Equal(it, int64(-6400000000), r.Int64())
		gotest.Equal(it, -42, r.Int())
		gotest.Equal(it, uint8(8), r.Uint8())
		gotest.Equal(it, uint16(1600), r.Uint16())
		gotest.Equal(it, uint32(320000), r.Uint32())
		gotest.Equal(it, uint64(6400000000), r.Uint64())
		gotest.Equal(it, uint(42), r.Uint())
		gotest.Equal(it, float32(1.5), r.Float32())
		gotest.Equal(it, -2.25, r.Float64())
	})

	t.It("round-trips NaN and Inf bit patterns, which fuzzing must be able to reach", func(it *gotest.T) {
		w := gotestfuzz.NewWriter()
		w.Float64(math.NaN())
		w.Float64(math.Inf(-1))

		r := gotestfuzz.NewReader(w.Out())
		gotest.True(it, math.IsNaN(r.Float64()))
		gotest.True(it, math.IsInf(r.Float64(), -1))
	})
}

func (s *FuzzBytesTestSuite) TestStringAndBytes(t *gotest.T) {
	t.It("round-trips strings and byte slices", func(it *gotest.T) {
		w := gotestfuzz.NewWriter()
		w.String("hello")
		w.ByteSlice([]byte{1, 2, 3})

		r := gotestfuzz.NewReader(w.Out())
		gotest.Equal(it, "hello", r.String())
		gotest.Equal(it, []byte{1, 2, 3}, r.ByteSlice())
	})

	t.It("returns a copy, never a window onto the corpus buffer", func(it *gotest.T) {
		w := gotestfuzz.NewWriter()
		w.ByteSlice([]byte{7, 7})
		raw := w.Out()

		got := gotestfuzz.NewReader(raw).ByteSlice()
		got[0] = 99
		gotest.Equal(it, byte(7), raw[len(raw)-2], "mutating the decoded slice must not corrupt the input buffer")
	})

	t.It("clamps a length that overruns the remaining buffer instead of failing", func(it *gotest.T) {
		// length prefix says 9 bytes, only 2 follow
		r := gotestfuzz.NewReader([]byte{9, 0, 'h', 'i'})
		gotest.Equal(it, "hi", r.String())
	})
}

func (s *FuzzBytesTestSuite) TestTotality(t *gotest.T) {
	t.It("yields zero values for every reader method once the buffer is exhausted", func(it *gotest.T) {
		r := gotestfuzz.NewReader(nil)
		gotest.False(it, r.Bool())
		gotest.Equal(it, int8(0), r.Int8())
		gotest.Equal(it, int16(0), r.Int16())
		gotest.Equal(it, int32(0), r.Int32())
		gotest.Equal(it, int64(0), r.Int64())
		gotest.Equal(it, 0, r.Int())
		gotest.Equal(it, uint8(0), r.Uint8())
		gotest.Equal(it, uint16(0), r.Uint16())
		gotest.Equal(it, uint32(0), r.Uint32())
		gotest.Equal(it, uint64(0), r.Uint64())
		gotest.Equal(it, uint(0), r.Uint())
		gotest.Equal(it, float32(0), r.Float32())
		gotest.Equal(it, 0.0, r.Float64())
		gotest.Empty(it, r.String())
		gotest.Empty(it, r.ByteSlice())
		gotest.Equal(it, 0, r.Len())
	})

	t.It("never returns a partial value from a short read", func(it *gotest.T) {
		// two bytes available, an eight-byte read must yield zero
		r := gotestfuzz.NewReader([]byte{0xFF, 0xFF})
		gotest.Equal(it, int64(0), r.Int64())
	})
}

func (s *FuzzBytesTestSuite) TestLenClamping(t *gotest.T) {
	t.It("clamps a slice count to what the remaining buffer could fill", func(it *gotest.T) {
		r := gotestfuzz.NewReader([]byte{200, 1, 2, 3})
		gotest.Equal(it, 3, r.Len(), "a count of 200 with 3 bytes left must not ask for 200 elements")
	})

	t.It("clamps the written count to 255 and reports what it wrote", func(it *gotest.T) {
		w := gotestfuzz.NewWriter()
		gotest.Equal(it, 255, w.Len(1000))
	})
}
