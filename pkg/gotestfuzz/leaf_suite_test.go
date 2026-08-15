package gotestfuzz_test

import (
	"math"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestfuzz"
)

// FuzzLeafTestSuite covers the fixed-width leaf helpers a fanned numeric
// field travels through. Two properties matter: every value round-trips,
// and decoding is total — a short, empty, or over-long byte slice still
// yields a value, because a fuzz execution that rejects its input is a
// wasted execution.
type FuzzLeafTestSuite struct{}

func (s *FuzzLeafTestSuite) TestRoundTrip(t *gotest.T) {
	t.It("round-trips every signed width at its extremes", func(it *gotest.T) {
		for _, v := range []int8{0, 1, -1, math.MinInt8, math.MaxInt8} {
			gotest.Equal(it, v, gotestfuzz.LeafInt8(gotestfuzz.LeafBytesInt8(v)))
		}
		for _, v := range []int16{0, -1, math.MinInt16, math.MaxInt16} {
			gotest.Equal(it, v, gotestfuzz.LeafInt16(gotestfuzz.LeafBytesInt16(v)))
		}
		for _, v := range []int32{0, -1, math.MinInt32, math.MaxInt32} {
			gotest.Equal(it, v, gotestfuzz.LeafInt32(gotestfuzz.LeafBytesInt32(v)))
		}
		for _, v := range []int64{0, -1, math.MinInt64, math.MaxInt64} {
			gotest.Equal(it, v, gotestfuzz.LeafInt64(gotestfuzz.LeafBytesInt64(v)))
		}
		for _, v := range []int{0, -1, math.MinInt, math.MaxInt} {
			gotest.Equal(it, v, gotestfuzz.LeafInt(gotestfuzz.LeafBytesInt(v)))
		}
	})

	t.It("round-trips every unsigned width at its extremes", func(it *gotest.T) {
		gotest.Equal(it, uint8(math.MaxUint8), gotestfuzz.LeafUint8(gotestfuzz.LeafBytesUint8(math.MaxUint8)))
		gotest.Equal(it, uint16(math.MaxUint16), gotestfuzz.LeafUint16(gotestfuzz.LeafBytesUint16(math.MaxUint16)))
		gotest.Equal(it, uint32(math.MaxUint32), gotestfuzz.LeafUint32(gotestfuzz.LeafBytesUint32(math.MaxUint32)))
		gotest.Equal(it, uint64(math.MaxUint64), gotestfuzz.LeafUint64(gotestfuzz.LeafBytesUint64(math.MaxUint64)))
		gotest.Equal(it, uint(math.MaxUint), gotestfuzz.LeafUint(gotestfuzz.LeafBytesUint(math.MaxUint)))
	})

	t.It("round-trips floats including the shapes native mutation cannot reach", func(it *gotest.T) {
		for _, v := range []float64{0, -2.25, math.MaxFloat64, math.SmallestNonzeroFloat64, math.Inf(1), math.Inf(-1)} {
			gotest.Equal(it, v, gotestfuzz.LeafFloat64(gotestfuzz.LeafBytesFloat64(v)))
		}
		for _, v := range []float32{0, 1.5, math.MaxFloat32, math.SmallestNonzeroFloat32} {
			gotest.Equal(it, v, gotestfuzz.LeafFloat32(gotestfuzz.LeafBytesFloat32(v)))
		}
		gotest.True(it, math.IsNaN(gotestfuzz.LeafFloat64(gotestfuzz.LeafBytesFloat64(math.NaN()))), "NaN survives by bit pattern")
		gotest.True(it, math.IsNaN(float64(gotestfuzz.LeafFloat32(gotestfuzz.LeafBytesFloat32(float32(math.NaN()))))))
	})
}

func (s *FuzzLeafTestSuite) TestWidths(t *gotest.T) {
	t.It("encodes at the natural width, with int and uint pinned to 8 bytes", func(it *gotest.T) {
		gotest.Len(it, gotestfuzz.LeafBytesInt8(1), 1)
		gotest.Len(it, gotestfuzz.LeafBytesUint16(1), 2)
		gotest.Len(it, gotestfuzz.LeafBytesInt32(1), 4)
		gotest.Len(it, gotestfuzz.LeafBytesFloat32(1), 4)
		gotest.Len(it, gotestfuzz.LeafBytesUint64(1), 8)
		gotest.Len(it, gotestfuzz.LeafBytesFloat64(1), 8)
		gotest.Len(it, gotestfuzz.LeafBytesInt(1), 8)
		gotest.Len(it, gotestfuzz.LeafBytesUint(1), 8)
	})

	t.It("is little-endian, so a single low byte carries the small values", func(it *gotest.T) {
		gotest.Equal(it, []byte{0xD2, 0x04, 0, 0, 0, 0, 0, 0}, gotestfuzz.LeafBytesUint64(1234))
	})
}

func (s *FuzzLeafTestSuite) TestTotality(t *gotest.T) {
	t.It("zero-extends short input, including nil", func(it *gotest.T) {
		gotest.Equal(it, uint64(0), gotestfuzz.LeafUint64(nil))
		gotest.Equal(it, uint64(1), gotestfuzz.LeafUint64([]byte{1}))
		gotest.Equal(it, int32(0x0201), gotestfuzz.LeafInt32([]byte{1, 2}))
		gotest.Equal(it, float64(0), gotestfuzz.LeafFloat64([]byte{}))
	})

	t.It("truncates over-long input to the leading width", func(it *gotest.T) {
		gotest.Equal(it, uint8(7), gotestfuzz.LeafUint8([]byte{7, 8, 9}))
		gotest.Equal(it, uint64(1), gotestfuzz.LeafUint64([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0xFF}))
	})

	t.It("reinterprets bits for signed kinds rather than clamping", func(it *gotest.T) {
		gotest.Equal(it, int8(-1), gotestfuzz.LeafInt8([]byte{0xFF}))
		gotest.Equal(it, int64(-1), gotestfuzz.LeafInt64([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}))
	})
}
