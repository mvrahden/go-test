package gotest

import (
	"context"
	"fmt"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

// B wraps *testing.B for benchmark methods on gotest suites. It satisfies
// the same assertion contract as *T and *R (Errorf + FailNow), so the full
// assertion library works inside benchmarks.
type B struct {
	b *testing.B
}

func NewB(b *testing.B) *B { return &B{b: b} }

// B returns the underlying *testing.B; the methods after it delegate to it.
// Errorf and Skipf name the caller's line, not gotest's.
func (b *B) B() *testing.B                       { return b.b }
func (b *B) Name() string                        { return b.b.Name() }
func (b *B) Loop() bool                          { return b.b.Loop() }
func (b *B) ReportAllocs()                       { b.b.ReportAllocs() }
func (b *B) SetBytes(n int64)                    { b.b.SetBytes(n) }
func (b *B) ReportMetric(n float64, unit string) { b.b.ReportMetric(n, unit) }
func (b *B) Context() context.Context            { return b.b.Context() }
func (b *B) Errorf(format string, args ...any)   { report(b.b, fmt.Sprintf(format, args...)) }
func (b *B) FailNow()                            { b.b.FailNow() }
func (b *B) Skipf(format string, args ...any) {
	assert.SkipInternalFrames(b.b)
	b.b.Skipf(format, args...)
}

// Setenv sets the environment variable key to value and restores it when the
// benchmark ends, as testing.B does.
func (b *B) Setenv(key, value string) { b.b.Setenv(key, value) }

// TempDir returns a new temporary directory, removed when the benchmark ends.
func (b *B) TempDir() string { return b.b.TempDir() }
