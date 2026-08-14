package gotest

import (
	"context"
	"testing"
)

// B wraps *testing.B for benchmark methods on gotest suites. It satisfies
// the same assertion contract as *T and *R (Errorf + FailNow), so the full
// assertion library works inside benchmarks.
type B struct {
	b *testing.B
}

func NewB(b *testing.B) *B { return &B{b: b} }

func (b *B) B() *testing.B                       { return b.b }
func (b *B) Loop() bool                          { return b.b.Loop() }
func (b *B) ReportAllocs()                       { b.b.ReportAllocs() }
func (b *B) SetBytes(n int64)                    { b.b.SetBytes(n) }
func (b *B) ReportMetric(n float64, unit string) { b.b.ReportMetric(n, unit) }
func (b *B) Context() context.Context            { return b.b.Context() }
func (b *B) Errorf(format string, args ...any)   { b.b.Errorf(format, args...) }
func (b *B) FailNow()                            { b.b.FailNow() }
func (b *B) Skipf(format string, args ...any)    { b.b.Skipf(format, args...) }
