// Package fzpromote is the package the promote clauses stage outside the
// repository: one target that fails on every input but its seed, one that
// holds, one that decodes a struct from several leaves, and decoding targets
// that fail only on values plain Go literals cannot spell.
package fzpromote

import (
	"bytes"
	"math"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PromoteTestSuite struct{}

// FuzzOnlySeed fails on every input but its seed, so a session finds a
// crasher within its first mutations.
func (s *PromoteTestSuite) FuzzOnlySeed(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {
		if in != "seed" {
			t.Errorf("unexpected input %q", in)
		}
	})
}

// FuzzTrim holds on every input.
func (s *PromoteTestSuite) FuzzTrim(f *gotest.F) {
	f.Add(" x ")
	f.Fuzz(func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

// Message fans out to one leaf per field.
type Message struct {
	To      string
	Subject string
	Count   int
}

// FuzzMessage decodes a Message from three leaves.
func (s *PromoteTestSuite) FuzzMessage(f *gotest.F) {
	f.Add(Message{To: "a@b.c", Subject: "hi", Count: 1})
	f.Fuzz(func(t *gotest.T, m Message) {
		gotest.GreaterOrEqual(t, len(m.To+m.Subject), 0)
	})
}

// lossy64 and lossy32 report a negative zero or a NaN of other bits than
// math.NaN's: values a plain Go literal cannot spell.
func lossy64(v float64) bool {
	return v == 0 && math.Signbit(v) || math.IsNaN(v) && math.Float64bits(v) != math.Float64bits(math.NaN())
}

func lossy32(v float32) bool {
	return v == 0 && math.Signbit(float64(v)) || math.IsNaN(float64(v)) && math.Float32bits(v) != math.Float32bits(float32(math.NaN()))
}

// exact reports the values at the edge of their literal spelling: the
// integer limits, a rune that is no character, invalid UTF-8.
func exact(u uint64, i int64, r rune, s string, b []byte) bool {
	return u == math.MaxUint64 && i == math.MinInt64 && r == -1 && s == "\xff\xfe" && bytes.Equal(b, []byte("\x00\xff"))
}

// Point fans out to one leaf per field.
type Point struct {
	X float64
	Y float32
}

// FuzzPoint decodes a Point and fails on a lossy float.
func (s *PromoteTestSuite) FuzzPoint(f *gotest.F) {
	f.Add(Point{X: 1, Y: 1})
	f.Fuzz(func(t *gotest.T, p Point) {
		gotest.False(t, lossy64(p.X) || lossy32(p.Y), "a lossy float")
	})
}

// FuzzPointAt decodes a Point beside two values of its own.
func (s *PromoteTestSuite) FuzzPointAt(f *gotest.F) {
	f.Add(Point{X: 1, Y: 1}, int32(1), byte('a'))
	f.Fuzz(func(t *gotest.T, p Point, r int32, b byte) {
		gotest.False(t, lossy64(p.X) && r == -1 && b == 0xff, "a lossy point at the edge")
	})
}

// Edge fans out to one leaf per field.
type Edge struct {
	U uint64
	I int64
	R rune
	S string
	B []byte
}

// FuzzEdge decodes an Edge and fails on the edge values.
func (s *PromoteTestSuite) FuzzEdge(f *gotest.F) {
	f.Add(Edge{U: 1, I: 1, R: 'a', S: "a", B: []byte("a")})
	f.Fuzz(func(t *gotest.T, e Edge) {
		gotest.False(t, exact(e.U, e.I, e.R, e.S, e.B), "the edge values")
	})
}
