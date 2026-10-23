package gotest

import "github.com/mvrahden/go-test/internal/carry"

// capturedPanic carries a panic across a helper goroutine, or a deferred
// function that has to finish coordinating before the panic may continue.
type capturedPanic = carry.Panic

// capturedFrom converts a recovered value into a capturedPanic, nil for none.
// Callers invoke recover() themselves, directly inside the deferred function.
func capturedFrom(v any, where string) *capturedPanic { return carry.From(v, where) }
