package gotestruntime

import "github.com/mvrahden/go-test/internal/fuzzcodec"

// The byte codecs a generated fuzz wrapper uses for arguments Go's fuzzing
// engine has no type for. Decoding is total: no input is ever rejected.

// FuzzReader is the consuming byte cursor a generated decoder reads a value
// from, fields in declaration order.
type FuzzReader = fuzzcodec.Reader

// NewFuzzReader returns a reader positioned at the start of b.
func NewFuzzReader(b []byte) *FuzzReader { return fuzzcodec.NewReader(b) }

// FuzzWriter is FuzzReader's encoding counterpart, which turns a seed into
// the bytes the reader decodes back to it.
type FuzzWriter = fuzzcodec.Writer

// NewFuzzWriter returns an empty writer.
func NewFuzzWriter() *FuzzWriter { return fuzzcodec.NewWriter() }

// fuzzLeaf is a number the engine carries as one fixed-width leaf.
type fuzzLeaf interface {
	uint8 | uint16 | uint32 | uint64 | uint | int8 | int16 | int32 | int64 | int | float32 | float64
}

// FuzzLeaf decodes one numeric leaf: short input zero-extends, long input
// truncates to the width of V.
func FuzzLeaf[V fuzzLeaf](b []byte) V {
	var v V
	switch p := any(&v).(type) {
	case *uint8:
		*p = fuzzcodec.LeafUint8(b)
	case *uint16:
		*p = fuzzcodec.LeafUint16(b)
	case *uint32:
		*p = fuzzcodec.LeafUint32(b)
	case *uint64:
		*p = fuzzcodec.LeafUint64(b)
	case *uint:
		*p = fuzzcodec.LeafUint(b)
	case *int8:
		*p = fuzzcodec.LeafInt8(b)
	case *int16:
		*p = fuzzcodec.LeafInt16(b)
	case *int32:
		*p = fuzzcodec.LeafInt32(b)
	case *int64:
		*p = fuzzcodec.LeafInt64(b)
	case *int:
		*p = fuzzcodec.LeafInt(b)
	case *float32:
		*p = fuzzcodec.LeafFloat32(b)
	case *float64:
		*p = fuzzcodec.LeafFloat64(b)
	}
	return v
}

// FuzzLeafBytes encodes v as the leaf FuzzLeaf decodes back to it.
func FuzzLeafBytes[V fuzzLeaf](v V) []byte {
	switch x := any(v).(type) {
	case uint8:
		return fuzzcodec.LeafBytesUint8(x)
	case uint16:
		return fuzzcodec.LeafBytesUint16(x)
	case uint32:
		return fuzzcodec.LeafBytesUint32(x)
	case uint64:
		return fuzzcodec.LeafBytesUint64(x)
	case uint:
		return fuzzcodec.LeafBytesUint(x)
	case int8:
		return fuzzcodec.LeafBytesInt8(x)
	case int16:
		return fuzzcodec.LeafBytesInt16(x)
	case int32:
		return fuzzcodec.LeafBytesInt32(x)
	case int64:
		return fuzzcodec.LeafBytesInt64(x)
	case int:
		return fuzzcodec.LeafBytesInt(x)
	case float32:
		return fuzzcodec.LeafBytesFloat32(x)
	case float64:
		return fuzzcodec.LeafBytesFloat64(x)
	}
	return nil
}

// FuzzBytes normalises a []byte leaf on its way into a fanned value: an
// empty slice becomes nil, since the engine does not keep nil and empty
// apart.
func FuzzBytes(b []byte) []byte { return fuzzcodec.LeafBytes(b) }
