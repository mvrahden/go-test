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

// B returns the underlying *testing.B.
func (b *B) B() *testing.B { return b.b }

// Name is the benchmark's name as testing gave it.
func (b *B) Name() string { return b.b.Name() }

// Loop reports whether the measured loop runs again: for b.Loop() { ... }.
func (b *B) Loop() bool { return b.b.Loop() }

// ReportAllocs reports the benchmark's allocations.
func (b *B) ReportAllocs() { b.b.ReportAllocs() }

// SetBytes records n bytes processed per iteration.
func (b *B) SetBytes(n int64) { b.b.SetBytes(n) }

// ReportMetric adds the metric n in unit to the benchmark's result.
func (b *B) ReportMetric(n float64, unit string) { b.b.ReportMetric(n, unit) }

// Context is the benchmark's context.
func (b *B) Context() context.Context { return b.b.Context() }

// Errorf fails the benchmark at the caller's line, not gotest's, and goes on.
func (b *B) Errorf(format string, args ...any) { report(b.b, fmt.Sprintf(format, args...)) }

// FailNow ends the benchmark now, as failed.
func (b *B) FailNow() { b.b.FailNow() }

// Skipf skips the benchmark at the caller's line, with the formatted reason.
func (b *B) Skipf(format string, args ...any) {
	assert.SkipInternalFrames(b.b)
	b.b.Skipf(format, args...)
}

// Setenv sets the environment variable key to value and restores it when the
// benchmark ends, as testing.B does.
func (b *B) Setenv(key, value string) { b.b.Setenv(key, value) }

// TempDir returns a new temporary directory, removed when the benchmark ends.
func (b *B) TempDir() string { return b.b.TempDir() }
