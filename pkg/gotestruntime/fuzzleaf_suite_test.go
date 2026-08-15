package gotestruntime_test

import (
	"math"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
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
			gotest.Equal(it, v, gotestruntime.LeafInt8(gotestruntime.LeafBytesInt8(v)))
		}
		for _, v := range []int16{0, -1, math.MinInt16, math.MaxInt16} {
			gotest.Equal(it, v, gotestruntime.LeafInt16(gotestruntime.LeafBytesInt16(v)))
		}
		for _, v := range []int32{0, -1, math.MinInt32, math.MaxInt32} {
			gotest.Equal(it, v, gotestruntime.LeafInt32(gotestruntime.LeafBytesInt32(v)))
		}
		for _, v := range []int64{0, -1, math.MinInt64, math.MaxInt64} {
			gotest.Equal(it, v, gotestruntime.LeafInt64(gotestruntime.LeafBytesInt64(v)))
		}
		for _, v := range []int{0, -1, math.MinInt, math.MaxInt} {
			gotest.Equal(it, v, gotestruntime.LeafInt(gotestruntime.LeafBytesInt(v)))
		}
	})

	t.It("round-trips every unsigned width at its extremes", func(it *gotest.T) {
		gotest.Equal(it, uint8(math.MaxUint8), gotestruntime.LeafUint8(gotestruntime.LeafBytesUint8(math.MaxUint8)))
		gotest.Equal(it, uint16(math.MaxUint16), gotestruntime.LeafUint16(gotestruntime.LeafBytesUint16(math.MaxUint16)))
		gotest.Equal(it, uint32(math.MaxUint32), gotestruntime.LeafUint32(gotestruntime.LeafBytesUint32(math.MaxUint32)))
		gotest.Equal(it, uint64(math.MaxUint64), gotestruntime.LeafUint64(gotestruntime.LeafBytesUint64(math.MaxUint64)))
		gotest.Equal(it, uint(math.MaxUint), gotestruntime.LeafUint(gotestruntime.LeafBytesUint(math.MaxUint)))
	})

	t.It("round-trips floats including the shapes native mutation cannot reach", func(it *gotest.T) {
		for _, v := range []float64{0, -2.25, math.MaxFloat64, math.SmallestNonzeroFloat64, math.Inf(1), math.Inf(-1)} {
			gotest.Equal(it, v, gotestruntime.LeafFloat64(gotestruntime.LeafBytesFloat64(v)))
		}
		for _, v := range []float32{0, 1.5, math.MaxFloat32, math.SmallestNonzeroFloat32} {
			gotest.Equal(it, v, gotestruntime.LeafFloat32(gotestruntime.LeafBytesFloat32(v)))
		}
		gotest.True(it, math.IsNaN(gotestruntime.LeafFloat64(gotestruntime.LeafBytesFloat64(math.NaN()))), "NaN survives by bit pattern")
		gotest.True(it, math.IsNaN(float64(gotestruntime.LeafFloat32(gotestruntime.LeafBytesFloat32(float32(math.NaN()))))))
	})
}

func (s *FuzzLeafTestSuite) TestWidths(t *gotest.T) {
	t.It("encodes at the natural width, with int and uint pinned to 8 bytes", func(it *gotest.T) {
		gotest.Len(it, gotestruntime.LeafBytesInt8(1), 1)
		gotest.Len(it, gotestruntime.LeafBytesUint16(1), 2)
		gotest.Len(it, gotestruntime.LeafBytesInt32(1), 4)
		gotest.Len(it, gotestruntime.LeafBytesFloat32(1), 4)
		gotest.Len(it, gotestruntime.LeafBytesUint64(1), 8)
		gotest.Len(it, gotestruntime.LeafBytesFloat64(1), 8)
		gotest.Len(it, gotestruntime.LeafBytesInt(1), 8)
		gotest.Len(it, gotestruntime.LeafBytesUint(1), 8)
	})

	t.It("is little-endian, so a single low byte carries the small values", func(it *gotest.T) {
		gotest.Equal(it, []byte{0xD2, 0x04, 0, 0, 0, 0, 0, 0}, gotestruntime.LeafBytesUint64(1234))
	})
}

func (s *FuzzLeafTestSuite) TestTotality(t *gotest.T) {
	t.It("zero-extends short input, including nil", func(it *gotest.T) {
		gotest.Equal(it, uint64(0), gotestruntime.LeafUint64(nil))
		gotest.Equal(it, uint64(1), gotestruntime.LeafUint64([]byte{1}))
		gotest.Equal(it, int32(0x0201), gotestruntime.LeafInt32([]byte{1, 2}))
		gotest.Equal(it, float64(0), gotestruntime.LeafFloat64([]byte{}))
	})

	t.It("truncates over-long input to the leading width", func(it *gotest.T) {
		gotest.Equal(it, uint8(7), gotestruntime.LeafUint8([]byte{7, 8, 9}))
		gotest.Equal(it, uint64(1), gotestruntime.LeafUint64([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0xFF}))
	})

	t.It("reinterprets bits for signed kinds rather than clamping", func(it *gotest.T) {
		gotest.Equal(it, int8(-1), gotestruntime.LeafInt8([]byte{0xFF}))
		gotest.Equal(it, int64(-1), gotestruntime.LeafInt64([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}))
	})
}
