package gotestruntime

import (
	"encoding/binary"
	"math"
)

// maxFuzzLen bounds a string/[]byte length prefix (2 bytes) and maxFuzzCount
// bounds a slice element count (1 byte). Fixed-width prefixes rather than
// varints: a single byte flip perturbs exactly one field, which is the
// mutation locality that makes coverage guidance converge.
const (
	maxFuzzLen   = 0xFFFF
	maxFuzzCount = 0xFF
)

// FuzzReader is the total, consuming byte cursor the decoders gotest
// generates for non-native fuzz argument types are built from. Every method
// is total: reading past the end of the buffer is not an error, it yields
// the zero value. That is what keeps a fuzz execution from ever rejecting
// its input — a rejected execution is a wasted execution, and coverage-guided
// fuzzing pays for every one of them with zero learning.
//
// Fields are read in declaration order; see docs/design/fuzz-structs.md for
// the wire format.
type FuzzReader struct {
	buf []byte
	pos int
}

// NewFuzzReader returns a reader positioned at the start of b. b is never
// modified, and no value the reader returns aliases it.
func NewFuzzReader(b []byte) *FuzzReader { return &FuzzReader{buf: b} }

// take consumes and returns the next n bytes, or nil when fewer than n
// remain — consuming whatever was left, so a short read can never be
// re-interpreted as a partial value.
func (r *FuzzReader) take(n int) []byte {
	if n > len(r.buf)-r.pos {
		r.pos = len(r.buf)
		return nil
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *FuzzReader) Bool() bool {
	b := r.take(1)
	return b != nil && b[0] != 0
}

func (r *FuzzReader) Uint8() uint8 {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (r *FuzzReader) Uint16() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

func (r *FuzzReader) Uint32() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (r *FuzzReader) Uint64() uint64 {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

// The signed readers below reinterpret the same fixed-width bits rather than
// converting a value: two's-complement reinterpretation is exactly what the
// wire format specifies, it is lossless in both directions, and it is what
// lets a single byte flip reach every negative value. gosec's G115
// overflow warning is therefore describing the intent, not a defect.
//
// int and uint are always eight bytes on the wire. On a 32-bit platform the
// top four are dropped on read and sign-extended on write, so a round trip
// still holds for every value that platform's int can hold.

func (r *FuzzReader) Uint() uint       { return uint(r.Uint64()) }
func (r *FuzzReader) Int8() int8       { return int8(r.Uint8()) }   //nolint:gosec // G115: deliberate bit reinterpretation
func (r *FuzzReader) Int16() int16     { return int16(r.Uint16()) } //nolint:gosec // G115: deliberate bit reinterpretation
func (r *FuzzReader) Int32() int32     { return int32(r.Uint32()) } //nolint:gosec // G115: deliberate bit reinterpretation
func (r *FuzzReader) Int64() int64     { return int64(r.Uint64()) } //nolint:gosec // G115: deliberate bit reinterpretation
func (r *FuzzReader) Int() int         { return int(r.Uint64()) }   //nolint:gosec // G115: deliberate bit reinterpretation
func (r *FuzzReader) Float32() float32 { return math.Float32frombits(r.Uint32()) }
func (r *FuzzReader) Float64() float64 { return math.Float64frombits(r.Uint64()) }

// ByteSlice reads a 2-byte length, clamps it to what remains, and returns a
// fresh copy of that many bytes. The copy matters: the corpus buffer must
// stay intact even if the target mutates what it was handed.
func (r *FuzzReader) ByteSlice() []byte {
	n := int(r.Uint16())
	if rem := len(r.buf) - r.pos; n > rem {
		n = rem
	}
	if n <= 0 {
		return nil
	}
	out := make([]byte, n)
	copy(out, r.buf[r.pos:r.pos+n])
	r.pos += n
	return out
}

func (r *FuzzReader) String() string { return string(r.ByteSlice()) }

// Len reads a 1-byte slice element count, clamped to the number of bytes
// still available — one byte is the smallest any element can consume, so a
// truncated input can never drive a huge allocation.
func (r *FuzzReader) Len() int {
	n := int(r.Uint8())
	if rem := len(r.buf) - r.pos; n > rem {
		n = rem
	}
	return n
}

// FuzzWriter is FuzzReader's encoding counterpart, used by the generated
// encoders that turn a typed f.Add(...) seed into the bytes the rerouted
// []byte target expects.
type FuzzWriter struct {
	buf []byte
}

func NewFuzzWriter() *FuzzWriter { return &FuzzWriter{} }

// Out returns the encoded bytes.
func (w *FuzzWriter) Out() []byte { return w.buf }

func (w *FuzzWriter) Bool(v bool) {
	var b byte
	if v {
		b = 1
	}
	w.buf = append(w.buf, b)
}

func (w *FuzzWriter) Uint8(v uint8)   { w.buf = append(w.buf, v) }
func (w *FuzzWriter) Uint16(v uint16) { w.buf = binary.LittleEndian.AppendUint16(w.buf, v) }
func (w *FuzzWriter) Uint32(v uint32) { w.buf = binary.LittleEndian.AppendUint32(w.buf, v) }
func (w *FuzzWriter) Uint64(v uint64) { w.buf = binary.LittleEndian.AppendUint64(w.buf, v) }
func (w *FuzzWriter) Uint(v uint)     { w.Uint64(uint64(v)) }
func (w *FuzzWriter) Int8(v int8)     { w.Uint8(uint8(v)) }   //nolint:gosec // G115: deliberate bit reinterpretation, inverse of FuzzReader.Int8
func (w *FuzzWriter) Int16(v int16)   { w.Uint16(uint16(v)) } //nolint:gosec // G115: deliberate bit reinterpretation, inverse of FuzzReader.Int16
func (w *FuzzWriter) Int32(v int32)   { w.Uint32(uint32(v)) } //nolint:gosec // G115: deliberate bit reinterpretation, inverse of FuzzReader.Int32
func (w *FuzzWriter) Int64(v int64)   { w.Uint64(uint64(v)) } //nolint:gosec // G115: deliberate bit reinterpretation, inverse of FuzzReader.Int64
func (w *FuzzWriter) Int(v int)       { w.Uint64(uint64(v)) } //nolint:gosec // G115: deliberate bit reinterpretation, inverse of FuzzReader.Int

func (w *FuzzWriter) Float32(v float32) { w.Uint32(math.Float32bits(v)) }
func (w *FuzzWriter) Float64(v float64) { w.Uint64(math.Float64bits(v)) }

// ByteSlice writes a 2-byte length followed by the bytes, truncating past
// maxFuzzLen. Truncation is the documented cost of a fixed-width prefix; a
// seed that long is not a seed worth keeping verbatim.
func (w *FuzzWriter) ByteSlice(v []byte) {
	if len(v) > maxFuzzLen {
		v = v[:maxFuzzLen]
	}
	w.Uint16(uint16(len(v))) //nolint:gosec // G115: clamped to maxFuzzLen on the line above
	w.buf = append(w.buf, v...)
}

func (w *FuzzWriter) String(v string) {
	if len(v) > maxFuzzLen {
		v = v[:maxFuzzLen]
	}
	w.Uint16(uint16(len(v))) //nolint:gosec // G115: clamped to maxFuzzLen on the line above
	w.buf = append(w.buf, v...)
}

// Len writes a 1-byte element count and returns the count actually written,
// so the caller's element loop covers exactly what the decoder will read.
func (w *FuzzWriter) Len(n int) int {
	if n > maxFuzzCount {
		n = maxFuzzCount
	}
	if n < 0 {
		n = 0
	}
	w.Uint8(uint8(n)) //nolint:gosec // G115: clamped to [0, maxFuzzCount] just above
	return n
}
