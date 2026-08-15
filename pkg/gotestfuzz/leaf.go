package gotestfuzz

import (
	"encoding/binary"
	"math"
)

// The Leaf* helpers carry one numeric leaf of a fanned fuzz argument as a
// fixed-width little-endian []byte corpus value. A number rides as bytes
// rather than as the engine's own int/float kind on purpose: Go's mutator
// gives a []byte its richest operators — interesting-value overwrites, bit
// flips, window arithmetic — and gives a native number only bounded ±100
// arithmetic, so boundary values, NaN, and Inf are one mutation away here
// and unreachable natively.
//
// Decoding is total: short input zero-extends, long input truncates to the
// leading width. int and uint are pinned to 8 bytes so the corpus format
// does not depend on the platform word size.

func leafWord(b []byte, width int) uint64 {
	var buf [8]byte
	copy(buf[:], b[:min(len(b), width)])
	return binary.LittleEndian.Uint64(buf[:])
}

func LeafUint8(b []byte) uint8     { return uint8(leafWord(b, 1)) }
func LeafUint16(b []byte) uint16   { return uint16(leafWord(b, 2)) }
func LeafUint32(b []byte) uint32   { return uint32(leafWord(b, 4)) }
func LeafUint64(b []byte) uint64   { return leafWord(b, 8) }
func LeafUint(b []byte) uint       { return uint(leafWord(b, 8)) }
func LeafInt8(b []byte) int8       { return int8(LeafUint8(b)) }   //nolint:gosec // G115: deliberate bit reinterpretation
func LeafInt16(b []byte) int16     { return int16(LeafUint16(b)) } //nolint:gosec // G115: deliberate bit reinterpretation
func LeafInt32(b []byte) int32     { return int32(LeafUint32(b)) } //nolint:gosec // G115: deliberate bit reinterpretation
func LeafInt64(b []byte) int64     { return int64(LeafUint64(b)) } //nolint:gosec // G115: deliberate bit reinterpretation
func LeafInt(b []byte) int         { return int(LeafUint64(b)) }   //nolint:gosec // G115: deliberate bit reinterpretation
func LeafFloat32(b []byte) float32 { return math.Float32frombits(LeafUint32(b)) }
func LeafFloat64(b []byte) float64 { return math.Float64frombits(LeafUint64(b)) }

func LeafBytesUint8(v uint8) []byte   { return []byte{v} }
func LeafBytesUint16(v uint16) []byte { return binary.LittleEndian.AppendUint16(nil, v) }
func LeafBytesUint32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
func LeafBytesUint64(v uint64) []byte { return binary.LittleEndian.AppendUint64(nil, v) }
func LeafBytesUint(v uint) []byte     { return LeafBytesUint64(uint64(v)) }
func LeafBytesInt8(v int8) []byte     { return LeafBytesUint8(uint8(v)) }   //nolint:gosec // G115: inverse of LeafInt8
func LeafBytesInt16(v int16) []byte   { return LeafBytesUint16(uint16(v)) } //nolint:gosec // G115: inverse of LeafInt16
func LeafBytesInt32(v int32) []byte   { return LeafBytesUint32(uint32(v)) } //nolint:gosec // G115: inverse of LeafInt32
func LeafBytesInt64(v int64) []byte   { return LeafBytesUint64(uint64(v)) } //nolint:gosec // G115: inverse of LeafInt64
func LeafBytesInt(v int) []byte       { return LeafBytesUint64(uint64(v)) } //nolint:gosec // G115: inverse of LeafInt
func LeafBytesFloat32(v float32) []byte {
	return LeafBytesUint32(math.Float32bits(v))
}
func LeafBytesFloat64(v float64) []byte {
	return LeafBytesUint64(math.Float64bits(v))
}

// LeafBytes normalises a []byte leaf on its way into a fanned value: an
// empty slice becomes nil. The engine does not preserve the nil/empty
// distinction — a nil seed comes back as []byte{} after a trip through the
// corpus format — so a struct field would otherwise compare differently
// under `go test` replay and under `-fuzz`. Collapsing on fan-in makes the
// field deterministic, matching the mini-codec's own convention. Only a
// pass-through top-level []byte position is handed over untouched.
func LeafBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}
