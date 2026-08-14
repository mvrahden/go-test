// Package fuzzing demonstrates gotest's fuzzing surface end to end, using a
// message broker's binary frame codec: struct-typed targets, native
// targets, typed seeds, harvested seeds, and the crash -> triage -> promote
// -> replay workflow.
package fuzzing

import (
	"encoding/binary"
	"errors"
	"math"
)

// Kind identifies what a Frame carries across the wire.
type Kind uint8

const (
	KindData Kind = iota
	KindControl
	KindHeartbeat
	KindError
)

// TraceID is a 128-bit distributed trace identifier. It travels behind a
// pointer so frames with tracing disabled pay nothing for it on the wire.
type TraceID struct {
	Hi, Lo uint64
}

// Header is one key/value pair attached to a Frame.
type Header struct {
	Name  string
	Value string
}

// Frame is one message-broker wire frame.
type Frame struct {
	Version uint8
	Kind    Kind
	Topic   string
	Headers []Header
	Payload []byte
	Trace   *TraceID
}

var errTruncated = errors.New("fuzzing: truncated frame")

// Encode renders f as a byte stream.
//
// Version and Kind used to be packed into a single byte to save space on
// the wire — a space-saving trick that looked safe because Kind only ever
// needs 3 bits today. It wasn't: neither field was actually bounded to fit
// its share of the byte, so a Version >= 32 or a Kind >= 8 silently
// corrupted the other field on the round trip. `gotest fuzz` found this in
// under a second (see FuzzFrameRoundTrip's promoted seed, a regression test
// for exactly this bug); the fix is to give each field its own byte.
func Encode(f Frame) []byte {
	b := make([]byte, 0, 2+len(f.Topic)+len(f.Payload))
	b = append(b, f.Version, uint8(f.Kind))
	b = putString(b, f.Topic)
	b = putHeaders(b, f.Headers)
	b = putBytes(b, f.Payload)
	b = putTrace(b, f.Trace)
	return b
}

// Decode is defensive by construction: every read is bounds-checked against
// what remains, nothing is indexed past the slice, and truncated or
// malformed input returns errTruncated rather than panicking or driving an
// unbounded allocation from an attacker-controlled length prefix.
func Decode(b []byte) (Frame, error) {
	r := &reader{b: b}

	version, ok := r.byte()
	if !ok {
		return Frame{}, errTruncated
	}
	kind, ok := r.byte()
	if !ok {
		return Frame{}, errTruncated
	}
	f := Frame{
		Version: version,
		Kind:    Kind(kind),
	}

	topic, ok := r.string()
	if !ok {
		return Frame{}, errTruncated
	}
	f.Topic = topic

	headers, ok := r.headers()
	if !ok {
		return Frame{}, errTruncated
	}
	f.Headers = headers

	payload, ok := r.bytes()
	if !ok {
		return Frame{}, errTruncated
	}
	f.Payload = payload

	trace, ok := r.trace()
	if !ok {
		return Frame{}, errTruncated
	}
	f.Trace = trace

	return f, nil
}

// Codec applies a broker-configured payload size limit on top of the wire
// format's own bounds checks — the kind of resource guard a real broker
// enforces ahead of the raw protocol.
type Codec struct {
	MaxPayload int
}

// NewCodec returns a Codec that rejects any frame whose payload exceeds
// maxPayload bytes.
func NewCodec(maxPayload int) *Codec {
	return &Codec{MaxPayload: maxPayload}
}

// ErrPayloadTooLarge is returned by Codec.Decode for an otherwise
// well-formed frame whose payload exceeds the configured limit.
var ErrPayloadTooLarge = errors.New("fuzzing: payload exceeds broker limit")

// Decode delegates to the package-level Decode and additionally rejects a
// frame whose payload exceeds c's configured limit.
func (c *Codec) Decode(b []byte) (Frame, error) {
	f, err := Decode(b)
	if err != nil {
		return Frame{}, err
	}
	if c.MaxPayload > 0 && len(f.Payload) > c.MaxPayload {
		return Frame{}, ErrPayloadTooLarge
	}
	return f, nil
}

// --- encode side ---

func putString(b []byte, s string) []byte {
	n := len(s)
	if n > math.MaxUint16 {
		n = math.MaxUint16
		s = s[:n]
	}
	var lenBuf [2]byte
	binary.LittleEndian.PutUint16(lenBuf[:], uint16(n)) //nolint:gosec // G115: clamped above
	b = append(b, lenBuf[:]...)
	return append(b, s...)
}

func putBytes(b []byte, v []byte) []byte {
	n := len(v)
	if n > math.MaxUint32 {
		n = math.MaxUint32
		v = v[:n]
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(n)) //nolint:gosec // G115: clamped above
	b = append(b, lenBuf[:]...)
	return append(b, v...)
}

func putHeaders(b []byte, hs []Header) []byte {
	n := len(hs)
	if n > math.MaxUint16 {
		n = math.MaxUint16
		hs = hs[:n]
	}
	var lenBuf [2]byte
	binary.LittleEndian.PutUint16(lenBuf[:], uint16(n)) //nolint:gosec // G115: clamped above
	b = append(b, lenBuf[:]...)
	for _, h := range hs {
		b = putString(b, h.Name)
		b = putString(b, h.Value)
	}
	return b
}

func putTrace(b []byte, tr *TraceID) []byte {
	if tr == nil {
		return append(b, 0)
	}
	b = append(b, 1)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], tr.Hi)
	b = append(b, buf[:]...)
	binary.LittleEndian.PutUint64(buf[:], tr.Lo)
	return append(b, buf[:]...)
}

// --- decode side: every method reports ok=false instead of panicking or
// reading past what remains ---

type reader struct {
	b []byte
}

func (r *reader) take(n int) ([]byte, bool) {
	if n < 0 || n > len(r.b) {
		return nil, false
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v, true
}

func (r *reader) byte() (byte, bool) {
	v, ok := r.take(1)
	if !ok {
		return 0, false
	}
	return v[0], true
}

func (r *reader) uint16() (uint16, bool) {
	v, ok := r.take(2)
	if !ok {
		return 0, false
	}
	return binary.LittleEndian.Uint16(v), true
}

func (r *reader) uint32() (uint32, bool) {
	v, ok := r.take(4)
	if !ok {
		return 0, false
	}
	return binary.LittleEndian.Uint32(v), true
}

func (r *reader) uint64() (uint64, bool) {
	v, ok := r.take(8)
	if !ok {
		return 0, false
	}
	return binary.LittleEndian.Uint64(v), true
}

func (r *reader) string() (string, bool) {
	n, ok := r.uint16()
	if !ok {
		return "", false
	}
	v, ok := r.take(int(n))
	if !ok {
		return "", false
	}
	return string(v), true
}

// bytes returns a fresh copy — never an alias of the input — so the caller
// can't corrupt Decode's argument by mutating the result.
func (r *reader) bytes() ([]byte, bool) {
	n, ok := r.uint32()
	if !ok {
		return nil, false
	}
	v, ok := r.take(int(n))
	if !ok {
		return nil, false
	}
	if len(v) == 0 {
		return nil, true
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, true
}

func (r *reader) headers() ([]Header, bool) {
	n, ok := r.uint16()
	if !ok {
		return nil, false
	}
	if n == 0 {
		return nil, true
	}
	out := make([]Header, 0, n)
	for i := 0; i < int(n); i++ {
		name, ok := r.string()
		if !ok {
			return nil, false
		}
		value, ok := r.string()
		if !ok {
			return nil, false
		}
		out = append(out, Header{Name: name, Value: value})
	}
	return out, true
}

func (r *reader) trace() (*TraceID, bool) {
	present, ok := r.byte()
	if !ok {
		return nil, false
	}
	if present == 0 {
		return nil, true
	}
	hi, ok := r.uint64()
	if !ok {
		return nil, false
	}
	lo, ok := r.uint64()
	if !ok {
		return nil, false
	}
	return &TraceID{Hi: hi, Lo: lo}, true
}
